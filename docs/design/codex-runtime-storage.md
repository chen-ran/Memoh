# Codex runtime storage isolation (draft v0.1)

Status: first implementation for review, **not a complete NFS compatibility fix**.
No production migration is performed by this change.

## Problem and upstream audit

The direct Codex driver places `CODEX_HOME` at
`/data/.codex/agents/<agent-id>`. That preserves native thread data, but also
puts Codex's bootstrap locks on whatever filesystem backs `/data`.

With a network-backed home, `flock(LOCK_EX | LOCK_NB)` on an arg0 lock can wait
in the kernel's network-filesystem RPC path. The app-server has not reached
`initialize` yet, so a healthy stdio transport still produces a handshake
timeout. This failure class is not specific to a hosted provider.

Audited source: [Codex rust-v0.151.0 arg0](https://github.com/openai/codex/blob/rust-v0.151.0/codex-rs/arg0/src/lib.rs).
It creates helper aliases in `CODEX_HOME/tmp/arg0/codex-arg0*`, holds `.lock`
for the process lifetime, and tries to lock old directories before deleting
them. The path is not redirected by `TMPDIR`. Release builds also reject an
entire `CODEX_HOME` under the system temporary directory: initialization may
continue with a warning and missing aliases. Therefore an initialize-only
test with a fresh `/tmp` home does not prove tool compatibility.

## Scope of this iteration

Keep the durable home, configuration, encrypted credential store, native
thread identifiers, sessions, and SQLite placement unchanged. Prepare only
the bootstrap temporary directory before starting Codex:

```text
/data/.codex/agents/<agent-id>/
  config.toml                    unchanged
  auth.json                      unchanged
  sessions/ and native state     unchanged
  tmp -> /tmp/memoh-codex-runtime-<uid>/<sha256-of-home>/
           arg0/codex-arg0*/      created and locked by Codex, not Memoh
```

This is a narrow mapping **from a durable tmp entry to local transient
storage**, not a link from an entire runtime home back to a persistent volume.
The location is inside the workspace, never the Memoh server's local /tmp.

### Preparation contract

- A missing `tmp` gets the managed link. Concurrent prepares accept only the
  same mapping. No existing entry is replaced or deleted.
- Existing real `tmp` directories on known local filesystems remain unchanged.
- Existing real `tmp` on an unsupported/network filesystem fails with
  `legacy_tmp_requires_drain`. Even an apparently empty directory may be in use;
  startup must not rename it, unlink locks, or probe them with another flock.
- Unknown links, symlinked parents, non-directory entries, and unsafe local
  ownership/permissions fail closed. A missing target of the exact managed
  link can be recreated after workspace replacement.
- Directory walks use `O_NOFOLLOW` and descriptor-relative operations. The
  private local root and agent directory are owned by the effective UID with
  mode 0700. Filesystem checks resolve `/proc/self/fdinfo` mount IDs rather than
  guessing by pathname; unknown filesystems are not assumed safe.
- The local-filesystem allowlist is conservative, not proof of every backing
  storage topology. In particular a custom overlay can hide a remote upperdir;
  supported workspace images must ensure the transient root is really local.
- The helper is embedded in the server and passed through bridge stdin to
  the toolkit's pinned Python, in isolated mode with a clean environment.
  This avoids an extra image helper file or a new privileged mount capability.
  Both RPC and process preparation have a ten-second deadline. A broken NFS
  metadata operation can still stall in the kernel: backend process cleanup
  remains a separate requirement.

The managed link target is stable per home/UID in a workspace. A per-process
UUID **in the durable symlink target** would allow a second process to repoint
the first process's helper path. Instead Codex already allocates and locks
its own unique subdirectories below the stable target. Identical link text
across workspace generations resolves to separate local filesystems. This
requires the managed workspace UID to stay stable; changing UID needs a
reviewed, drained layout migration.

Memoh does not delete this shared tmp parent on app-server Close. Codex owns
the lifetime and janitor cleanup of each arg0 child. Deleting the parent could
break another live/draining process. A few empty parent directories can remain
until workspace disposal. This is not a distributed ownership mechanism:
cross-server startup coordination and fencing must not rely on local flock.

## What is deliberately not solved yet

1. **SQLite/WAL and complete native recovery.** Moving arg0 does not make an
   SQLite database on NFS safe. Audit the pinned CLI's complete state graph
   before enabling this as a full NFS support claim. Distinguish rebuildable
   indexes from irrecoverable state. Prefer supported path configuration and
   explicit native-state persistence; evaluate the existing agentstate port
   rather than inventing a whole-home bidirectional copier. If database backup
   is necessary, use a consistent snapshot and reconcile its boundary with
   native transcripts; do not independently copy a live DB/WAL/SHM set.
2. **Confirmed remote process termination.** Stream closure, stdin EOF, and
   process exit are different events. The generic process contract and backend
   implementations need separate review, including PID-not-yet-known races,
   natural exit, process groups, stale identities, and cancellation. Do not
   globally change detached/background-task semantics to fix managed Codex.
3. **Credential and owner races.** Continue using the existing encrypted
   credential store/CAS. A later home change must update login, refresh,
   logout, and materialization together. A stale owner must not republish
   credentials or native state. This patch does not add distributed fencing.
4. **Existing NFS layouts.** The first implementation protects new homes and
   fails clearly on unsafe legacy tmp directories; it does not automatically
   recover already blocked agents. Do not roll it out as an unattended repair.

## Migration and rollback gates

Before a follow-up migration, stop admitting new work for the exact Agent,
drain every app-server owner, and confirm the relevant processes have exited.
Only then archive the old transient directory with a recoverable, bounded
operation and publish the new layout. Do not infer ownership from PID alone,
or recurse through symlinks. Credentials, sessions and user files must remain
unchanged. No automated migration command is provided in this first patch.

Persist the latest recoverable native state before workspace replacement.
If a future change alters native storage format, rollback requires proving
the old version can read the new state, not merely switching an image tag.
Missing native state must not be presented as a successful original-thread
resume. Do not replay side-effecting tools automatically to rebuild state.

## OSS versus provider integration

| Concern | Owner |
| --- | --- |
| Codex layout policy, safe preparation, helper compatibility | OSS driver |
| Generic managed-process lifecycle and error reporting | OSS process/bridge contract, follow-up |
| Native session persistence and credential invariants | OSS driver and existing state store, follow-up |
| Provider-specific terminate/wait, sandbox generation validation | Provider adapter, not Codex driver |
| NFS mount configuration, template distribution, pause/resume tests | Deployment/provider integration |

Do not add provider names, credentials, production identifiers, or conditional
`if hosted` behavior to the generic driver. No schema migration, template
change, frontend API change, or other Agent's storage policy is included here.

## Verification

Automated tests cover missing/existing/dangling layouts, immutable durable
sentinels, repeated and concurrent preparation, distinct agents, unknown
filesystems, mocked network mounts, symlink attacks, unsafe permissions,
clean execution environment, sanitized errors, and bounded preparation.

```sh
go test ./internal/agent/runtime/codex/... ./internal/agent/runtime/agentprocess/...
python3 -I internal/agent/runtime/codex/runtime_tmp_test.py -v
```

The optional offline smoke test must run in a disposable Linux workspace with
the pinned Codex binary and a writable `/data`; it creates only a fresh test
home. Run it with networking disabled and no credentials mounted:

```sh
python3 -I internal/agent/runtime/codex/runtime_tmp_live_test.py /path/to/codex /data
```

It starts two app-servers, checks both initialize, confirms the second janitor
does not remove the first process's helper, executes the native apply_patch
aliases, checks durable sentinels, and verifies EOF exit/arg0 cleanup. It does
not exercise the bridge, OAuth, model/tool turns, native resume, or real NFS.

Before ready-for-review/release, separately validate real NFSv3 failure and
remount behavior; OAuth refresh; complete multi-step turns; native
resume/fork/compaction across server restart and workspace replacement;
multiple server owners; failure cleanup; and rollback. Mocked filesystem
classification and passing initialize are not substitutes for these gates.

## Related prior art

- [VS Code Remote-SSH local lockfiles](https://github.com/microsoft/vscode-docs/blob/main/remote-release-notes/v1_37.md)
- [IPython local history database configuration](https://ipython.readthedocs.io/en/stable/api/generated/IPython.core.history.html)
- [OpenCode NFS proposal, closed without merge](https://github.com/anomalyco/opencode/pull/15131)
- [SQLite WAL restrictions](https://sqlite.org/wal.html)
- [SQLite consistent backup API](https://sqlite.org/backup.html)
- [OpenHands explicit conversation persistence](https://docs.openhands.dev/sdk/guides/convo-persistence)
