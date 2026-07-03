41 chars including newline = 40 hex chars, correct SHA-1 length. Good, sha is `2a5c9797158a98dc14800b59e91ae7283d655f8d`.

{"status":"success","commit_sha":"2a5c9797158a98dc14800b59e91ae7283d655f8d","session_id":"658b94af-515e-40e2-b366-8ece2fedfe41"}

Summary of work done in this batch (docs, batch 3 of the mux-sandbox-suite task):

Card 8 — commit `cae5ed6` (docs(sandbox): document the mux suite runbook and launcher):
- `C:\Code\loomyard\wts\mux-sandbox-suite\docs\sandbox-howto.md` — added `mux-sandbox-suite.cmd` to the launcher intro sentence, added a new "### 4b. Run the mux suite (optional, needs live psmux)" subsection after "### 4. Run the suite", and added `tools/sandbox/MUX-SANDBOX-SUITE.md` to the "See also" list.
- `C:\Code\loomyard\wts\mux-sandbox-suite\docs\sandbox-hub.md` — added a new "## Running the mux suite" section mirroring the main-suite description, and added the `mux-sandbox-suite.cmd` row to the launcher/subcommand mapping block (now "four subcommands").

Card 9 — commit `2a5c979` (docs(mux): point module doc and overview at the mux sandbox suite):
- `C:\Code\loomyard\wts\mux-sandbox-suite\docs\modules\mux.md` — added a "## Manual test surface: the mux sandbox suite" subsection immediately after "## What actually works (empirical guardrails)" and before "## Completion and hooks live in shuttle, not mux".
- `C:\Code\loomyard\wts\mux-sandbox-suite\docs\overview.md` — extended the "## Sandbox Hub" paragraph to name `mux-sandbox-suite.cmd` and note the shared `sandbox-fetch.cmd` report pipeline.

Verify: `go test ./...` — all packages pass (including `internal/muxcli`, `internal/muxengine`, `internal/muxengine/render`, `cmd/lyx`, `tools/sandbox`).

Pre-report self-check: `git status --porcelain --untracked-files=no` is clean (no uncommitted tracked changes). Both commits pushed to `mux-sandbox-suite` branch.

{"status":"success","commit_sha":"2a5c9797158a98dc14800b59e91ae7283d655f8d","session_id":"658b94af-515e-40e2-b366-8ece2fedfe41"}
