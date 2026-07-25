Card-count check: 7 of 7 cards committed (verified via `git log <range-start>..HEAD --oneline`, all 7 commit subjects match the batch file's `## Cards` `Commit:` messages exactly). All work is complete, verify passed, and `git status --porcelain` is clean.

{"status":"success","commit_sha":"7da444d9","session_id":"9adc0d8e-7ba1-4ece-8764-13c8c209c64b"}

Summary of work done in this batch (sandbox-wire-and-guard, 7/7 cards committed):

- `tools/sandbox/suite.go` — `binaryInfo` gained `Source`; `binaryFingerprint(path, source string)`; `header()` renders `- Source: %s`; `runSuite` resolves via `resolveLyx()` instead of bare `lookPath("lyx")`; `launchAgent` seam gained trailing `binDir string`, prepending it to the child's PATH via `prependPath` when a dev binary is resolved. `muxDown` left unchanged.
- `tools/sandbox/report.go` — `reportFingerprint` gained `Source string \`json:"source"\``; `runFetch` resolves via `resolveLyx()`; `fetchReport` stamps `Source: info.Source`.
- `tools/sandbox/main.go` — `cloneRun` seam is now `func(parentDir, lyxPath string) error`; `decideClone` resolves the binary lazily at the clone step only (`lyxPath, _, err := resolveLyx()`), keeping the Hub-exists/no-reset no-op path free of any resolution.
- `tools/sandbox/suite_test.go`, `report_test.go`, `main_test.go` — updated to the new signatures; added `stubSuiteSeams`/`stubLyxLookPath`/`stubResolveLyxProd` devBinPath stubbing so existing tests keep resolving `sourceProd`; added dedicated dev-path tests (`TestRunSuite_DevBinaryPrependsBinDir`, `TestRunFetch_DevBinary`) and a `Source:`-line header test.
- `tools/sandbox/pathresolve_guard_test.go` (new) — guard test scanning non-test `*.go` files in `tools/sandbox` for banned bare-PATH `lyx` literals outside `resolve.go`, with vacuous-scan protection (`< 3` files fails).
- `cmd/lyx/tierpurity_test.go`, `cmd/lyx/hermeticenv_test.go` — added self-exclusion allowlist entries for the new guard file.

Verify command `go test ./tools/sandbox/ ./cmd/lyx/` passes (`go test -count=1`, both packages ok). `git status --porcelain --untracked-files=no` is clean (no uncommitted tracked changes).
