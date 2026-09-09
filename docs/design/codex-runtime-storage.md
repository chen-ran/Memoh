# Codex native storage isolation

Status: revised after real NFSv3 fault injection. The implementation requires a
provider-managed persistent local volume for network-backed workspaces. It does
not claim that an ephemeral local directory makes native state recoverable.

## Problem and verified startup path

Memoh addresses each Codex home as `/data/.codex/agents/<agent-id>`. If `/data`
is NFS, native bootstrap locks and SQLite locks depend on network lock RPCs.
A nonblocking `flock` can still wait inside an NFS RPC before returning to the
application. A healthy bridge stream then sees an initialize timeout.

The first implementation moved only `CODEX_HOME/tmp` to `/tmp`. Real Docker
NFSv3 testing with the pinned **Codex 0.151.0**, a kernel NFS mount with
`local_lock=none`, and NFS-Ganesha 4.3 disproved that as a sufficient fix:

| NLM unavailable, ordinary NFS I/O still available | Observed result |
| --- | --- |
| Original home | `tmp/arg0/.../.lock` blocks in `flock(LOCK_EX\|LOCK_NB)` for over 60 seconds. |
| Only tmp local | arg0 lock succeeds in about 28 microseconds; `state_5.sqlite` blocks in `fcntl(F_SETLK)`, then app-server exits after about 30 seconds with SQLite initialization failure. |
| tmp and SQLite local (diagnostic) | Initialization still waits on `CODEX_HOME/installation_id` via `flock(LOCK_EX)`. |
| tmp, SQLite and installation_id local (diagnostic) | Initialization completes in 0.789 seconds and native `apply_patch` works. |

The diagnostic cases were disposable experiments, not migration recipes.
Timings document one observation, not performance guarantees.

Pinned upstream evidence:

- [arg0 aliases and lifetime locks](https://github.com/openai/codex/blob/rust-v0.151.0/codex-rs/arg0/src/lib.rs): helpers live below home/tmp/arg0; TMPDIR does not redirect them. Release builds reject a home under the system temporary directory.
- [SQLite configuration](https://github.com/openai/codex/blob/rust-v0.151.0/codex-rs/state/src/sqlite.rs): state, logs, goals, memories and queue databases use SQLite WAL. They cannot all be assumed rebuildable indexes.
- [installation identity](https://github.com/openai/codex/blob/rust-v0.151.0/codex-rs/core/src/installation_id.rs): `resolve_installation_id` locks the file before reading even an existing UUID; prepopulating it does not remove this lock.
- [SQLite WAL restrictions](https://sqlite.org/wal.html): moving only the bootstrap lock does not make an NFS database safe.

## Storage decision

Keep workspace files on their existing `/data` mount, but place the **entire
native Codex home on a persistent local filesystem** when that mount is not
known local. Keeping all native files together preserves credentials, SQLite
DB/WAL relationships, installation identity, sessions, and future native lock
files. There is no whole-home live copier or per-turn backup protocol.

The externally addressed home stays stable:

```text
/data/                                      existing workspace files (may be NFS)
  .codex/agents/<agent-id> -> /var/lib/memoh/codex-state/<volume-id>/<home-hash>

/var/lib/memoh/codex-state/                   dedicated persistent local mount
  .memoh-volume.json                        immutable volume identity
  .memoh-prepare.lock                       local preparation lock
  <volume-id>/<sha256-of-addressed-home>/    private native home, mode 0700
    .memoh-native-home.json                 addressed home + volume identity
    config.toml, auth.json                  same credential/configuration paths
    installation_id                        ordinary durable local file
    state_*.sqlite, *-wal, *-shm             ordinary local SQLite files
    goals_*.sqlite, memories_*.sqlite, ...   same native state boundary
    sessions/, archived_sessions/, ...     durable native transcripts
    tmp/arg0/codex-arg0*/                   Codex-owned helper directories
```

For an existing real home on a known local filesystem, keep the existing layout.
The v0.1 managed tmp link remains supported on such local homes, including
recreation of its transient target. New local homes let Codex create tmp itself.

The image/adapter must mount the persistent volume **inside each bot workspace**
at `/var/lib/memoh/codex-state`; mounting it on the Memoh server does not help.
A separate local block-backed filesystem/volume is required. The helper accepts
ext2/3/4, xfs, btrfs, zfs or f2fs and verifies a distinct mount ID. It rejects
NFS, unknown/FUSE filesystems, container overlay and tmpfs for this volume.
A Docker named volume backed by the VM's ext4 filesystem qualifies. A Docker
volume backed by NFS does not. A bind mount of a disposable local directory
could pass the filesystem test: retention is an explicit deployment contract,
not something filesystem detection can prove.

The adapter owns volume identity, single attachment, retention across workspace
replacement, backup, and restore. The same persistent native volume must be
reattached before restoring/replacing a workspace. This PR does not add an
automatic volume provisioner or implement provider-specific storage APIs.
Deployments offering only NFS and an ephemeral root must provide this storage
capability before enabling the direct Codex runtime; silently losing native
state is not an acceptable fallback.

## Preparation and identity contract

- Preparation runs through the existing bridge and embedded pinned-toolkit
  Python, with isolated Python mode, clean environment and a ten-second RPC
  and process deadline. It uses descriptor-relative, no-follow traversal.
- Preparation happens **before credential materialization**. It creates parent
  directories, then chooses a local home or publishes a new managed home link.
  Credential and configuration writes retain their addressed paths and resolve
  into the same home as Codex. Login, refresh, logout and encrypted credential
  CAS therefore keep their existing ownership boundary.
- Existing network home directories are never migrated by startup, even if
  empty. They return `native_home_requires_drain` before launching Codex.
- The persistent volume has an atomically created immutable UUID. Managed link
  text includes that UUID; a replacement empty/wrong volume is rejected rather
  than regenerating installation identity or creating a fresh SQLite database.
- A managed link whose native home or home marker is missing is rejected.
  Only the legacy **temporary** target is recreatable; durable native state is
  not. Orphaned nonempty native homes are not silently reattached to a missing
  anchor.
- A local preparation lock serializes publication/migration on the native
  volume. No lock is acquired on NFS. It is not distributed fencing: the
  provider must enforce single attachment of the native volume.
- The native home is not deleted at app-server Close. Codex owns its arg0
  children and janitor; concurrent/recycled app-servers retain distinct helper
  directories on the same local home.

Config/credential materialization also has bounded I/O deadlines. Those bounds
are separate from initialize, and a cancelled RPC is not itself evidence of
remote process exit. Generic provider process termination remains its own
contract; the storage helper does not change detached/background exec behavior.

## Offline migration of existing NFS homes

An operator must first stop admissions for the exact Agent, drain **all**
server owners, and confirm native processes have exited. This condition cannot
be inferred from local PID inspection alone. Keep admission stopped until
migration finishes. In the target workspace, run the reviewed repository helper
with the toolkit Python:

```sh
/opt/memoh/toolkit/bin/python3 -I runtime_storage.py \
  --migrate-drained /data/.codex/agents/<agent-id>
```

`--migrate-drained` explicitly declares that the operator established the
cross-instance drain. The helper additionally rejects visible local processes
with this CODEX_HOME and refuses to proceed when it cannot inspect them.

Migration checks the dedicated persistent volume and copies the offline native
tree into a private staging directory on it. It never follows source symlinks
or copies special files; resolve unsupported layouts explicitly first. Only the
top-level tmp tree is omitted from the new home, while the original copy stays
in the archive. Other nested directories named tmp are preserved.

SQLite DB and WAL files are copied only as a **quiescent set**, after draining;
there is no live DB/WAL/SHM copier. Validation and WAL checkpoint run against the
local staged databases, never against the NFS source, so broken NLM does not
block the migration's SQLite validation. Database validation failure leaves the
source untouched. The target and marker are fsynced before publication.

After rechecking local owners and the source directory identity, migration
renames the original directory to
`<agent-id>.before-native-storage-<unique-id>` and installs the managed link.
The complete original directory, including tmp and credentials, remains there.
No source data is deleted. If the operation fails between rename and link
publication, preserve the target/archive and reconcile them while still drained;
startup refuses an orphaned nonempty target instead of guessing.

There is no unattended migration and no automatic stale-owner takeover.

## Recovery and rollback

- Reattach the **same persistent native volume** during workspace replacement.
  Fixed addressed home paths and intact SQLite/transcripts allow native resume,
  fork and compaction to use the same state. An empty substitute volume fails
  its identity check.
- On `thread/resume` failure, Memoh now reports the stable external-runtime
  unavailable error instead of automatically creating a new native thread.
  Explicit `ForceFreshRuntime` behavior remains available where already intended
  (for example Discuss). Recovery must not silently discard native context or
  replay side-effecting tools.
- An immediate rollback before admitting any new work can restore the archived
  directory after all processes are drained. After new work has been admitted,
  the archive is stale: rollback needs a quiescent copy of the **current** native
  home and a compatibility check against the old CLI. Restoring the old archive
  blindly would lose newly created native state.
- Reverting the Memoh code does not undo storage publication. The addressed home
  still resolves to the persistent native directory; retain the volume and
  verify old-version compatibility. Do not remove the volume as part of an image
  rollback.

## Verification and rollout boundaries

Unit tests cover local-layout preservation, concurrent home publication,
volume/marker identity, wrong or missing volumes, symlink and permission checks,
refusal of live/legacy layouts, orphan detection, and offline migration with a
committed WAL, credential sentinel and native transcript preservation.

```sh
go test ./internal/agent/runtime/codex/... ./internal/agent/runtime/agentprocess/...
python3 -I internal/agent/runtime/codex/runtime_storage_test.py -v
```

The optional real CLI test must run in a disposable Linux workspace with Codex
0.151.0, no credentials and networking restricted to the test NFS server:

```sh
python3 -I internal/agent/runtime/codex/runtime_storage_live_test.py /path/to/codex /data
```

When /data is NFS, mount the dedicated persistent local volume first. Verify
actual NFSv3/NLM fault injection, native helpers, migration under broken NLM,
bridge start/close, and workspace replacement with both correct and wrong
volumes. A successful initialize alone does not establish real-model behavior.

The original real-NFS experiment also verified the bridge helper timeout during
full NFS outage, normal bridge process lifecycle, and tmp recreation after
remount/replacement. An extra NFSv4.1 attempt hit ordinary directory I/O errors
in the test environment, so it was not counted as compatibility evidence.

Before production rollout, the deployment owner must validate persistent-volume
provisioning/retention/single attachment and its own backup/restore path. Real
OAuth refresh, multi-step model turns, provider-specific terminate/wait,
cross-instance fencing and production rollback require separate verification.
No local filesystem check or offline smoke test substitutes for those gates.

### Revised implementation: local verification

The revised implementation was exercised with real NFSv3 plus a separate Docker
named volume backed by ext4, using Codex 0.151.0:

| Check | Result |
| --- | --- |
| NLM TCP/UDP 32803 blocked, two fresh managed app-servers | Both initialize; native apply_patch aliases work; clean EOF exit. |
| Existing NFS home with a completed native turn, NLM blocked during offline migration | Same thread ID, transcript and installation ID; resumed turn and native fork succeed. |
| Entire workspace container replaced, same NFS data and native ext4 volumes reattached, NLM blocked | Same thread and installation ID recovered; another turn and native fork succeed. |
| Existing addressed home with a different empty native volume | Refused with `native_volume_identity_mismatch`; no fresh native thread is substituted. |
| Actual Go driver preparation / bridge Exec / pinned CLI | Three starts pass; stale helper is reaped while the other live helper survives; no matching live native process remains after Close. |
| Python filesystem suite | 26 tests pass, including committed WAL migration and corruption refusal. |
| Go runtime, error catalog, server and handlers | Targeted suites pass with race instrumentation; Codex golangci-lint passes. |

Recovery turns used a deterministic loopback Responses fixture, not a real
model provider. This proves native protocol/state round trips, not model quality,
OAuth refresh, or compaction correctness. The fixture is available as
`runtime_storage_recovery_test.py`; use `create`, `migrate`, and `verify` in
sequence in disposable workspaces, keeping `/results` for its test thread ID.
Do not run its fixed test home against user data. The live filesystem detector
cannot verify retention policies or physical volume single attachment.
