All three cards of batch `registration-and-suite` are implemented, committed, and pushed, and the batch's `verify:` command is green.

Summary of work:
- `C:\Code\loomyard\wts\internal-burler\tools\sandbox\SANDBOX-BURLER-SUITE.md` (new) — sandbox suite doc with S1 (toy BLOCKING round, `**Covers:** burler`), S2 (APPROVED path), S3 (error paths).
- `C:\Code\loomyard\wts\internal-burler\sandbox-burler-suite.cmd` (new) — launcher.
- `C:\Code\loomyard\wts\internal-burler\tools\sandbox\suite.go` — added `burlerSandboxSuiteMD` embed and `burlerSuite` spec.
- `C:\Code\loomyard\wts\internal-burler\tools\sandbox\main.go` — added `case "burler-suite":` dispatch, updated top doc comment.
- `C:\Code\loomyard\wts\internal-burler\tools\sandbox\main_test.go` — added `TestRun_BurlerSuiteRoutesToLaunch`.
- `C:\Code\loomyard\wts\internal-burler\cmd\lyx\main.go` — registered `burlercli.Command()`, updated root `Long` module list.
- `C:\Code\loomyard\wts\internal-burler\cmd\lyx\helptree_test.go` — added `"burler"` to pinned module list and a `{name: "burler", ...}` table entry.
- `C:\Code\loomyard\wts\internal-burler\internal\burlerengine\smoke_round_test.go` (new) — `//go:build smoke` opt-in test wiring the real muxengine/claudeengine/shuttleengine.Runner/burlerengine.Engine stack over a toy chair/table fixture.

Commits (all pushed to `internal-burler`):
- `379003e` sandbox: add SANDBOX-BURLER-SUITE (doc, burler-suite subcommand, launcher)
- `3afc4ea` lyx: register burler module at the cobra root
- `2055ffa` burler: add opt-in smoke test driving one real toy round

Note: `goimports` is not installed in this environment, so the git-commit skill's import-formatting step was skipped for those files; `go build`/`go vet`/`go test` all pass and import ordering was kept consistent with neighboring code by hand. The repo also uses CRLF line endings repo-wide (`core.autocrlf=true`), which makes `gofmt -l` flag pre-existing untouched files too (e.g. `report.go`) — not something introduced by this batch.

{"status":"success","commit_sha":"2055ffaf0414180e46befc9d700435b839061c3b","session_id":"25084724-45ee-4d96-a2ec-678392a23183"}