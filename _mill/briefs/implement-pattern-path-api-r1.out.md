40 chars + newline, correct full SHA: `4683955de6bd615a73a9ab120118823fca50e09e`.

{"status":"success","commit_sha":"4683955de6bd615a73a9ab120118823fca50e09e","session_id":"59690091-99c2-4ec7-b93f-f86da3546a0a","cards_done":[1,2,3,4,5,6]}

All 6 of 6 cards committed this turn — full completion, honestly verified against the batch file's `## Cards` list and the commit log since the batch-start commit `4301f2e1446cb0901d67159f426a17dfba0c54f7`.

Summary of work:
- `internal/pattern/leaf_enforcement_test.go`, `CONSTRAINTS.md` — widened Pattern Leaf Invariant allowlist to admit `internal/lyxdirs`.
- `internal/pattern/pattern.go` — `File` now builds from `lyxdirs.LyxDirName`; added `PathspecFile`/`PathspecDir` exported consts; rewrote the three directive constants' literal pointers from `_pattern/...` to `_lyx/...`.
- `internal/pattern/doc.go` — updated package godoc for the new `_lyx` layout and documented `PathspecFile`/`PathspecDir`.
- `internal/pattern/patternpath_test.go`, `internal/pattern/pattern_test.go` — converged path/directive expectations onto `_lyx`, added pathspec exact-string and forward-slash tests.
- `cmd/lyx/constructoranchoring_test.go` — anchored `pattern.FileHere` assertions on `lyxdirs.LyxDirName` instead of `pattern.DirName`.
- `internal/websterengine/template_test.go`, `internal/builderengine/template_test.go`, `internal/burlerengine/template_test.go`, `internal/loomengine/plan_test.go` — moved PATTERN fixture/directive-stub expectations to `_lyx`.

Verify command (`go test ./internal/pattern/ ./internal/burlerengine/ ./internal/websterengine/ ./internal/loomengine/ ./cmd/lyx/ && go test -tags integration ./internal/builderengine/`) passed, and `go vet -tags integration ./...` (the overview-level cross-batch check) is clean. Working tree has no uncommitted tracked changes.

{"status":"success","commit_sha":"4683955de6bd615a73a9ab120118823fca50e09e","session_id":"59690091-99c2-4ec7-b93f-f86da3546a0a","cards_done":[1,2,3,4,5,6]}
