# PR #1607 follow-up evidence (5e4dbbfc8)

Captured from the local dev environment (Compose project `memoh-pr1607`, web http://localhost:18082) running commit 5e4dbbfc8 with Grok Build 1.0.62.

- `status-config-only.png`, `status-run-views.txt`: `/status` from the composer; the run view frame carries `configuration_only: true` (the earlier status run from before the fix did not).
- `recycle-wait.log`, `recycle-wait-B.png`: Agent config change while another conversation's turn runs; the new turn waits for the old process and completes.
- `recycle-busy.log`, `recycle-busy-B.png`, `recycle-busy-B-retry.png`: the old turn runs past 30 s; the new turn answers `external_runtime.busy`, the old turn still completes, and a resend succeeds.
- `status-busy-running.png`: `/status` while a turn runs answers 409 `session_runtime.session_busy`; the running turn (`sleep 20`) is untouched and completes.
