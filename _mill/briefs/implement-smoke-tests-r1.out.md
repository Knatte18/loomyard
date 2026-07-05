No tracked in-scope modifications remain dirty. All three cards are committed and pushed, and verify passes cleanly.

Summary of work:
- `C:\Code\loomyard\wts\internal-shuttle\internal\shuttlecli\smoke_run_test.go` — Card 22: `TestSmokeShuttleRunWritesOutputAndCleans`, plus the shared smoke helpers (`claudeBinaryPath`, `smokePwshPath`, `hubHolder`/`hubHolders`, `deferHubRelease`, `muxStatusStrand`) reused by the other two smoke files.
- `C:\Code\loomyard\wts\internal-shuttle\internal\shuttlecli\smoke_guardrail_test.go` — Card 23: `TestSmokeGuardrailDeniesAgentTool` and `TestSmokeGuardrailAskingSurfacesQuestion`.
- `C:\Code\loomyard\wts\internal-shuttle\internal\shuttlecli\smoke_interrupt_test.go` — Card 24: `TestSmokeInterruptSendContinues`.

Commits: `2436264`, `b20073b`, `76d11d3` (all pushed to `internal-shuttle`).

{"status":"success","commit_sha":"76d11d3","session_id":"ac7b9e68-0308-4e9a-8c85-93abfaa96eee"}
