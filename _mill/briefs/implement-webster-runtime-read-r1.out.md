All 4 cards for the `webster-runtime-read` batch are committed and verified. Summary of files touched:

- `/home/knatte/Code/loomyard/wts/stencils-directory-reorg/stencils/stencils.go` — registered webster's five stencils (card 25)
- `/home/knatte/Code/loomyard/wts/stencils-directory-reorg/.gitattributes` — LF pins for the five relocated files (card 25)
- `internal/websterengine/master-template.md` → `stencils/webster/webster-template-master.md` (git mv, card 25)
- `internal/websterengine/integration-template.md` → `stencils/webster/webster-template-integration.md` (git mv, card 25)
- `internal/websterengine/fork-prefix.md` → `stencils/webster/webster-prefix-fork.md` (git mv, card 25)
- `internal/websterengine/recovery-prefix.md` → `stencils/webster/webster-prefix-recovery.md` (git mv, card 25)
- `internal/websterengine/implementer-body.md` → `stencils/webster/webster-body-implementer.md` (git mv, card 25)
- `/home/knatte/Code/loomyard/wts/stencils-directory-reorg/internal/websterengine/render.go` — call-time read via `stencilstore.Read`, `joinTemplateAssets` now strips every asset's banner via `stencil.StripLeadingComment`, accessors take `stencilsDir`/return errors (card 26)
- `/home/knatte/Code/loomyard/wts/stencils-directory-reorg/internal/websterengine/runlevel.go` — `RenderIntegrationPrompt` call site supplies `fabricengine.StencilsDir(deps.Layout.HubPath)` (card 27)
- `/home/knatte/Code/loomyard/wts/stencils-directory-reorg/internal/websterengine/template_test.go` — rewired to seed real hub-rooted stencils directories, plus three new tests: banner-leak guard, composed-runtime-read, missing-board hard error (card 28)

Verify passed in full: `go build ./...` and `go test ./stencils/... ./internal/websterengine/... ./internal/webstercli/... ./internal/lyxcwd/...` all green.

Card-count self-check: 4 of 4 cards committed (25, 26, 27, 28), matching the batch's declared card count exactly.

{"status":"success","commit_sha":"976d3b22d6665e106e7d2385a26bbd3820c7678c","session_id":"3a56da65-b15b-4630-98ac-933d92727f0c","cards_done":[25,26,27,28]}
