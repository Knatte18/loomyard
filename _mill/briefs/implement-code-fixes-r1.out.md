All 3 of 3 cards from the batch have been committed, pushed, and both verify commands pass.

{"status":"success","commit_sha":"2da5ed2","session_id":"f73bf23e-deb9-4497-a53f-8deb63e6c6fe"}

Summary of work (3 of 3 cards committed, confirmed via `git log a56d334..HEAD --oneline`):

1. **Card 1** — `C:\Code\loomyard\wts\restore-tier1-floor\internal\clihelp\exec.go`: added a package-level `func init()` that sets `cobra.MousetrapHelpText = ""` with the required doc-comment rationale. Commit `ab1df3f`.

2. **Card 2** — Relocated `TestExecGateCommand_LingeringChildDoesNotHangPastWaitDelay` verbatim from `C:\Code\loomyard\wts\restore-tier1-floor\internal\perchengine\gate_test.go` into new file `C:\Code\loomyard\wts\restore-tier1-floor\internal\perchengine\gate_lingering_test.go` (tagged `//go:build integration`), dropped the now-unused `runtime` import in `gate_test.go`, and updated its file-level doc comment. `testmain_test.go` was left untouched. Commit `ce0e6e1`.

3. **Card 3** — `C:\Code\loomyard\wts\restore-tier1-floor\internal\boardengine\boardtest\concurrency_test.go`: reduced `writes` from `50` to `10` in `TestConcurrentReadsDuringUpserts`. Measured isolated warm package time before (`1.731s`) vs after (`0.642s`/`0.381s` across two follow-up runs) — a drop well past the ~1s win criterion, so the reduction was kept (not reverted) with reasoning recorded in the comment above the const. `readers = 8` and `seedWiki(t, 100)` were left unchanged. Commit `2da5ed2`.

Verify results:
- `go test ./internal/clihelp ./internal/perchengine ./internal/boardengine/boardtest ./cmd/lyx -count=1` — all packages pass (`internal/clihelp` ~1.4s, `internal/perchengine` ~1.15s, both matching the expected speedups).
- `go test -tags integration ./internal/perchengine -run TestExecGateCommand_LingeringChildDoesNotHangPastWaitDelay -count=1 -v` — passes (~12.3s), proving the relocated test compiles and behaves correctly under the integration tag.
- Guard tests `TestTierPurity_UntaggedTestsSpawnNothing` and `TestHermeticGitEnv_GitSpawningPackagesHaveTestMain` in `cmd/lyx` both pass with the relocated file present.

No uncommitted tracked changes remain (`git status --porcelain --untracked-files=no` is empty). The only untracked file in the tree is `_mill/briefs/implement-code-fixes-r1.md`, which is outside this batch's scope and was left alone.
