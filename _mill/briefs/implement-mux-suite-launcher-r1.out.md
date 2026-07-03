All three cards for the `mux-suite-launcher` batch are implemented and committed, and the batch verify command passes.

Summary of work:
- `C:\Code\loomyard\wts\mux-sandbox-suite\tools\sandbox\suite.go` — introduced `suiteSpec{fileName, doc, instruction}`, `mainSuite`/`muxSuite` spec values (absorbing `suiteFileName`/`defaultInstruction`), embedded `MUX-SANDBOX-SUITE.md` via `muxSandboxSuiteMD`, parameterized `renderScheme(info, doc)` and `runSuite(parentDir, claudeOverride, promptOverride, spec)`.
- `C:\Code\loomyard\wts\mux-sandbox-suite\tools\sandbox\main.go` — added the `mux-suite` dispatch case mirroring `suite` (own `-claude`/`-prompt` flagset), calling `runSuite(absParent, *claudeFlag, *promptFlag, muxSuite)`; updated file-header comment.
- `C:\Code\loomyard\wts\mux-sandbox-suite\tools\sandbox\suite_test.go` — mechanically retargeted existing tests to the new signature/spec fields, added `TestRunSuite_MuxSpec_*` coverage (writes mux file, excludes both files, deletes stale report, default instruction, prompt override).
- `C:\Code\loomyard\wts\mux-sandbox-suite\tools\sandbox\main_test.go` — added `TestRun_MuxSuiteRoutesToLaunch`, `TestRun_MuxSuiteFlagsRoutedAfterToken`, `TestRun_MuxSuiteErrorPropagation`.
- `C:\Code\loomyard\wts\mux-sandbox-suite\mux-sandbox-suite.cmd` — new launcher, structural mirror of `sandbox-suite.cmd`, invoking `go run ./tools/sandbox -parent C:\Code mux-suite %*`.

Commits (all pushed to `mux-sandbox-suite` branch):
- `3fe3c52` sandbox: add mux-suite subcommand via parameterized runSuite
- `2c964ed` test(sandbox): cover mux-suite subcommand
- `8536dd7` sandbox: add mux-sandbox-suite.cmd launcher

`go test ./tools/sandbox/` passes (all pre-existing and new tests green). Working tree is clean of tracked modifications.

{"status":"success","commit_sha":"8536dd745d299a47c1796f999888def5fefbc320","session_id":"f15e059f-ceca-4698-b584-2a5b7f08fbda"}
