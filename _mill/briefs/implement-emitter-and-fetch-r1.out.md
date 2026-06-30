All 6 cards are committed (753ca2e, 8200d66, 6f5b89a, 120acdf, b486b4b, d6a3420), pushed, working tree clean, and verify passes.

{"status":"success","commit_sha":"d6a34201e8072534576765d4e6c8860e74955aa7","session_id":"6af578a8-45c7-4f3d-828b-afeeb8134b38"}

Summary of work for batch `emitter-and-fetch`:

- `C:\Code\loomyard\wts\sandbox-report-json\tools\sandbox\report.go` (new) — defines the `sandboxReport`/`reportMeta`/`reportFingerprint`/`reportItem` contract types and `fetchReport`.
- `C:\Code\loomyard\wts\sandbox-report-json\tools\sandbox\main.go` — added top-level `-loomyard` flag, required for the `suite` subcommand, resolved via `filepath.Clean`+`filepath.Abs`.
- `C:\Code\loomyard\wts\sandbox-report-json\tools\sandbox\suite.go` — `runSuite` signature now takes `loomyardRoot`; removes stale `sandbox-report.json` before launch, excludes it via `.git/info/exclude`, and fetches it on clean exit.
- `C:\Code\loomyard\wts\sandbox-report-json\tools\sandbox\SANDBOX-SUITE.md` — rewritten to instruct the agent to emit `sandbox-report.json` instead of filing GitHub issues via `lyx selfreport create`; dropped the `gh` pre-condition.
- `C:\Code\loomyard\wts\sandbox-report-json\sandbox.cmd` — passes `-loomyard "%~dp0."` to the tool.
- `C:\Code\loomyard\wts\sandbox-report-json\tools\sandbox\report_test.go` (new) — covers `fetchReport`'s validate/stamp/fetch behavior (happy path, empty items, absent items, malformed JSON, wrong/missing source, missing report, scratch dir creation).
- `C:\Code\loomyard\wts\sandbox-report-json\tools\sandbox\suite_test.go` / `main_test.go` — updated for the new `runSuite`/`run` signatures and flag, plus new tests for fetch, stale-removal, and exclude behavior.

Verify command `go build ./... && go test ./tools/sandbox/... ./internal/paths/...` passes.
