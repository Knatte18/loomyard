{"status":"success","commit_sha":"4fe31a7e01d0f3ca023ed6ec274c35bc9b10a636","session_id":"e5cfd6ba-919e-47aa-a449-52fad74b9e18"}

Summary of work completed for batch `02-engine-round`:

- **Card 3** (`internal/burlerengine/review-prompt-template.md`, `template.go`, `template_test.go`; `CONSTRAINTS.md`): embedded the generic A-review/B-fix round prompt template with all eight top-level stencil markers, pinned round-discipline substrings via `TestTemplate_StatesRoundDiscipline`, proved fill behavior via `TestTemplate_FillsWithAllMarkers`, and added the short `## Review Round Invariant` section to `CONSTRAINTS.md`.
- **Card 4** (`internal/burlerengine/prompt.go`, `prompt_test.go`): implemented `composePrompt(p *Profile) (string, error)`, composing target/fasit path blocks (with directory annotation), fix-scope rules (source vs overlay), tool-use rules, and the prior-rounds clean-room block.
- **Card 5** (`internal/burlerengine/engine.go`, `engine_test.go`): implemented the `Shuttle` interface, `Engine`/`New`, `Result`, and `(*Engine).Run`'s full sequence (validate -> compose -> build Spec -> run -> outcome/verdict mapping), tested against a same-package `fakeShuttle`.
- **Card 6** (`internal/burlerengine/doc.go`): wrote the package doc comment folding in the durable design content from `docs/modules/burler.md` ahead of that file's later deletion.

Batch verify (`go build ./... && go test ./internal/burlerengine/...`) passes. All four cards are committed and pushed on branch `internal-burler`, working tree clean of tracked in-scope changes. One untracked file remains outside scope: `C:\Code\loomyard\wts\internal-burler\_mill\briefs\implement-engine-round-r1.md` (the brief itself, not part of this batch's edit surface).