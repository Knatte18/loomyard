All 4 cards match their declared Commit: messages exactly. 4 of 4 cards committed, all complete.

{"status":"success","commit_sha":"fdd7e6d1","session_id":"bde17b03-58f2-4d09-9448-0025b8cb1c3d","cards_done":[1,2,3,4]}

Summary: 4 of 4 cards committed for batch `registry-and-state-foundations`.

- `/home/knatte/Code/loomyard/wts/codeintel-v1/internal/codeintelengine/registry.go` — added `Entry.PinnedVersion`/`Entry.HasNativeDaemon` (yaml-tagged), set on the `go` builtin only (`v0.23.0`, `true`); other four builtins left zero-valued. Commit `e60abaa5`.
- `/home/knatte/Code/loomyard/wts/codeintel-v1/internal/codeintelengine/template.yaml` — mirrored the same two keys onto the seeded `go:` block. Same commit `e60abaa5`.
- `/home/knatte/Code/loomyard/wts/codeintel-v1/internal/codeintelengine/registry_test.go` — added coverage for the new fields on `go` and the four non-Go builtins, plus a `validateEntry` regression guard over actual `builtins()` entries. Commit `045afb40`.
- `/home/knatte/Code/loomyard/wts/codeintel-v1/internal/hubgeometry/hubgeometry.go` — added `Layout.CodeintelDaemonStateFile(lang string) string` and `Layout.CodeintelDaemonLock(lang string) string`, WorktreeRoot-anchored, under `dotLyxDirName`, placed after `LoomStatusLock`. Commit `92052e88`.
- `/home/knatte/Code/loomyard/wts/codeintel-v1/internal/hubgeometry/codeinteldaemon_test.go` (new) — mirrors `loomstatus_test.go`'s pattern plus a distinct-per-language pair. Commit `fdd7e6d1`.

Batch verify (`go test -count=1 ./internal/codeintelengine/... ./internal/hubgeometry/...`) passes; `gofmt -l` clean on both packages; working tree clean at report time.
