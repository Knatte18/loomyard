Good, 40 hex chars as expected — earlier miscount. Final sha: `5d04e1885e9575c09a0bb2681ffc7e6ff506d44c`.

All four cards for batch `suite-doc-and-guard` are implemented and committed:

- `C:\Code\loomyard\wts\mux-sandbox-suite\tools\sandbox\MUX-SANDBOX-SUITE.md` (created, card 1)
- `C:\Code\loomyard\wts\mux-sandbox-suite\cmd\lyx\sandbox_coverage_test.go` (generalized to glob, card 2)
- `C:\Code\loomyard\wts\mux-sandbox-suite\tools\sandbox\SANDBOX-SUITE.md` (S9 removed, pointer added, card 3)
- `C:\Code\loomyard\wts\mux-sandbox-suite\CONSTRAINTS.md` (Sandbox Suite Coverage invariant updated, card 4)

`verify: go test ./cmd/lyx/` passes (all tests green, including `TestSandboxCoverage_AllModulesCoveredOrExcluded`). No tracked uncommitted changes remain.

{"status":"success","commit_sha":"5d04e1885e9575c09a0bb2681ffc7e6ff506d44c","session_id":"64f5e151-79ad-43de-b7c5-07df79d8ec48"}

{"status":"success","commit_sha":"5d04e1885e9575c09a0bb2681ffc7e6ff506d44c","session_id":"64f5e151-79ad-43de-b7c5-07df79d8ec48"}
