All 4 cards (35, 36, 37, 38) are committed, matching the batch file's `## Cards` `Commit:` messages. Verify passes clean, and `git status` shows no uncommitted tracked changes (only the pre-existing untracked brief file, out of scope).

{"status":"success","commit_sha":"de920a3a","session_id":"c11157f8-ad6f-45fe-b20c-5f21c016255c","cards_done":[35,36,37,38]}

Summary of work completed (4 of 4 cards committed, verified via `git log 3cb985d9..HEAD --oneline`):

- **Card 35** (`4d793b34`): Added `internal/websterengine/integration-template.md` (the integration fork's prompt, no per-card/commit instructions), `RenderIntegrationPrompt`/`IntegrationTemplate` in `internal/websterengine/render.go`, an integration-fork bracket section in `internal/websterengine/master-template.md`, and pinning tests in `internal/websterengine/template_test.go`.
- **Card 36** (`d21329c7`): Added `internal/websterengine/integration.go` with `ShouldRunIntegration`, `AwaitIntegration`, and `RunIntegration` (await/parse plumbing only, no bisect).
- **Card 37** (`962a9451`): Extended `internal/websterengine/integration.go` with `bisect` (in-process binary search via `gitrepo.CheckoutDetached`/`RestoreBranch` + `os/exec`), `RecordIntegrationFailure`, and `BisectAndEscalate`; extended `internal/websterengine/summary.go` with `AppendIntegrationFailure`.
- **Card 38** (`de920a3a`): Wired `runIntegrationStage`/`accumulatedCardSHAs` into `internal/websterengine/runlevel.go`'s `Run`, adjusted `internal/websterengine/runlevel_test.go`'s `seedRunPlanDir` fixture to keep pre-existing tests unaffected, and added `internal/websterengine/integration_test.go` (skip / pass / fail-with-bisect-and-HEAD-restore coverage against a real scratch repo).

`go test -tags integration ./internal/websterengine/...` passes cleanly (confirmed with `-count=1`), and `git status --porcelain --untracked-files=no` shows no uncommitted tracked changes.
