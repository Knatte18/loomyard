7 of 7 cards committed (Cards 3 through 9), matching the batch's declared card count of 7. All committed, verify passed, no dirty tracked files.

{"status":"success","commit_sha":"7068ac90b35362b18d638a8038856fc5870330d6","session_id":"b2b46593-3c51-4cc4-b2ef-1ee048c56d86","cards_done":[3,4,5,6,7,8,9]}

Summary: All 7 cards in batch `directive-read-path` (cards 3-9) were implemented and committed:

- `/home/knatte/Code/loomyard/wts/pattern-directive-stencils/internal/pattern/pattern.go` — `Directive` now reads stencils from `stencilsDir` and strips the banner; three directive constants deleted.
- `/home/knatte/Code/loomyard/wts/pattern-directive-stencils/internal/pattern/leaf_enforcement_test.go` and `/home/knatte/Code/loomyard/wts/pattern-directive-stencils/CONSTRAINTS.md` — Pattern Leaf Invariant allowlist extended.
- `/home/knatte/Code/loomyard/wts/pattern-directive-stencils/internal/pattern/pattern_test.go` — stamped fixture helper, 10 migrated tests, 4 new property tests.
- `/home/knatte/Code/loomyard/wts/pattern-directive-stencils/internal/loomengine/plan.go`, `internal/loomengine/prompt_test.go` — loom call site.
- `/home/knatte/Code/loomyard/wts/pattern-directive-stencils/internal/burlerengine/engine.go`, `internal/burlerengine/prompt_test.go` — burler call site.
- `/home/knatte/Code/loomyard/wts/pattern-directive-stencils/internal/websterengine/render.go`, `internal/websterengine/template_test.go` — webster's two hoisted call sites plus error-path tests.

Note: `go fmt ./...` initially touched two out-of-scope files (`internal/lyxcwd/docslink_test.go`, `internal/shell/posix.go`) with pre-existing formatting drift unrelated to this batch (one edit even mangled a Unicode quote in a comment); both were reverted via `git checkout --` before committing, and only in-scope files were formatted/staged. Full `go build ./... && go test ./...` passes cleanly with no uncommitted tracked changes remaining.