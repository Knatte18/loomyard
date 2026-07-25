Both findings from the holistic review were processed under the mill-receiving-review protocol and fixed (both were NIT-severity but valid, no legitimate pushback grounds):

1. **`findingsEnvelope`'s `"card"` key untested** - added `TestValidateCmd_FindingsUseCardKey` in `internal/webstercli/cli_test.go`, driving a plan that trips `planparser.Validate`'s `card-missing-field` check through `validateCmd` and asserting the emitted findings carry `"card":"1-only"` with no stale `"batch"` key. Commit `fbcdc81e`.

2. **`bisect`'s empty-`shas` case was a hard error, not a graceful degrade** - per card 37's own prose ("Guard the bisect so an empty `shas` slice or a single-SHA plan degrades gracefully"), changed `bisect` in `internal/websterengine/integration.go` to return a `-1` sentinel for the empty case instead of erroring, and guarded `BisectAndEscalate` to fall back to `"unknown"` for both card and SHA rather than indexing into the empty slice (which would have panicked). Added `TestBisectAndEscalate_EmptySHAsDegradesGracefully` in `internal/websterengine/integration_test.go`. Commit `e5b9b1dc`.

All ten batch `verify:` commands ran clean, in order, from `/home/knatte/Code/loomyard/wts/webster-rewrite`:
- `go test ./internal/planparser/...` (x2), `go test ./internal/batcher/...`, `go test -tags integration ./internal/gitrepo/...`, `go test -tags integration ./internal/websterengine/...` (x4), `go test -tags integration ./internal/webstercli/...`, `go build ./...` - all passed.

Baseline HEAD was `ad352224` (mill-go: holistic fix housekeeping commit); final HEAD `e5b9b1dc` differs, and `git status --porcelain --untracked-files=no` is clean (no uncommitted tracked changes).

Files touched:
- `/home/knatte/Code/loomyard/wts/webster-rewrite/internal/webstercli/cli_test.go`
- `/home/knatte/Code/loomyard/wts/webster-rewrite/internal/websterengine/integration.go`
- `/home/knatte/Code/loomyard/wts/webster-rewrite/internal/websterengine/integration_test.go`

{"status":"success","commit_sha":"e5b9b1dcea901ac9188481d2348d263b39dd819e","session_id":"7adec8c7-4d15-4824-aaf0-5cf5312e2c48"}
