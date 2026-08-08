Both cards (7 and 8) are committed, matching the batch's declared 2 cards. Verify passes.

{"status":"success","commit_sha":"a5ce4b01e8448b2188b246a56011dc968afe7b36","session_id":"208ba83f-b1c5-4790-a5d9-7cfa05ee41a5","cards_done":[7,8]}

Summary: 2 of 2 cards committed (all complete, verified against the git log range `81082839..HEAD`).

File paths touched:
- `/home/knatte/Code/loomyard/wts/pattern-into-lyx-consolidation/internal/fabricengine/pull.go` (Card 7: deleted `patternDirName` const, added `internal/pattern` import, re-scoped `patternResidueCommits` pathspec to `pattern.PathspecFile`/`pattern.PathspecDir`, retargeted all `_pattern`-naming doc comments)
- `/home/knatte/Code/loomyard/wts/pattern-into-lyx-consolidation/internal/fabricengine/pull_integration_test.go` (Card 8: moved the seeded fixture to `_lyx/PATTERN.md`, replaced `pattern.DirName` usage with `lyxdirs.LyxDirName`, added negative `_lyx/config/fabric.yaml` and positive `_lyx/pattern/detail.md` sub-cases)

Both commits pushed. Batch verify command (`go test -tags integration -run 'Pull|Residue' ./internal/fabricengine/`) passes, and `go vet -tags integration ./internal/fabricengine/...` is clean.

{"status":"success","commit_sha":"a5ce4b01e8448b2188b246a56011dc968afe7b36","session_id":"208ba83f-b1c5-4790-a5d9-7cfa05ee41a5","cards_done":[7,8]}
