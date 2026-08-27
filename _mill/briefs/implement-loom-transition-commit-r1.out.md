All 4 cards (29, 30, 31, 32) committed and verified, plus one additional fix commit correcting a Fabric Vocabulary Invariant violation found during self-verify. 4 of 4 cards committed — all complete.

{"status":"success","commit_sha":"bf2fd48a9d6fea882815e8726a607324f3f5ccea","session_id":"fd1d6dd7-2ece-49a0-b9eb-cb69148aadd0","cards_done":[29,30,31,32]}

Summary of files touched (all absolute paths under `/home/knatte/Code/loomyard/wts/weft-local-only-files`):
- `internal/loomcli/wiring.go` — added `commitStatusDeps`, `loomCommitStatusDeps`, `commitStatusMessage`, `newCommitStatusSeam`, and filled `CommitStatus` at both `wireStatusPathsOnly` and `wire()` ShedPaths sites.
- `manifest/designs/loom.md` — rewrote the resume and landing-checkpoint paragraphs to reflect the new per-transition commit/push behavior.
- `internal/loomcli/wiring_commitstatus_test.go` — new Tier 1 test file covering all seven required properties.

Note: during self-verify I found and fixed a Fabric Vocabulary Invariant violation (bare "weft" tokens in `internal/loomcli` comments/log strings, which is not in the owner set) via an extra commit `bf6b9743`, caught by `TestEnforcement_FabricVocabulary` in `internal/lyxcwd`. `go build ./cmd/lyx && go test ./internal/loomcli/... ./internal/lyxcwd/...` passes cleanly.

{"status":"success","commit_sha":"bf2fd48a9d6fea882815e8726a607324f3f5ccea","session_id":"fd1d6dd7-2ece-49a0-b9eb-cb69148aadd0","cards_done":[29,30,31,32]}
