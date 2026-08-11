41 chars including newline = 40-char SHA. Good, that's the correct full SHA.

All 3 cards committed (3 of 3 cards committed), verify passed.

{"status":"success","commit_sha":"86edec587d5dc3c7bff556681c9ccd653d895165","session_id":"7fbfc33c-d7c4-40ce-a6c3-f2e24da3eaea","cards_done":[1,2,3]}

Summary: All 3 of 3 cards in batch 01-mutation-vocabulary committed and verified.

- `/home/knatte/Code/loomyard/wts/fabric-mutation-record-envelope/internal/fabricengine/mutation.go` (new) — Card 1, commit `073c3ef7`
- `/home/knatte/Code/loomyard/wts/fabric-mutation-record-envelope/internal/fabricengine/mutation_test.go` (new) — Card 2, commit `b0f194dc`
- `/home/knatte/Code/loomyard/wts/fabric-mutation-record-envelope/internal/output/output.go` and `/home/knatte/Code/loomyard/wts/fabric-mutation-record-envelope/internal/output/output_test.go` (edited) — Card 3, commit `86edec58`

`go test ./internal/fabricengine/ ./internal/output/` passes. Working tree has no uncommitted tracked changes (only the untracked brief file remains, which is out of scope).

{"status":"success","commit_sha":"86edec587d5dc3c7bff556681c9ccd653d895165","session_id":"7fbfc33c-d7c4-40ce-a6c3-f2e24da3eaea","cards_done":[1,2,3]}
