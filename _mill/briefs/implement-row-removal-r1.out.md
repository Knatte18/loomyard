All 9 cards (28 through 36) committed and verified: 8 real content commits plus one plan-scope-extension commit, and card 36's verification-only step completed with no file changes as its `Commit: none` declaration requires. The batch's full verify command passed, and module-wide `go vet ./...` is clean.

{"status":"success","commit_sha":"6d0e082d356dbb8a9e14821721097245bd2cc4d9","session_id":"b463eac3-64f5-447c-9b1c-42513ba2a433","cards_done":[28,29,30,31,32,33,34,35,36]}

Key files touched this batch:
- `/home/knatte/Code/loomyard/wts/producer-gates/contracts/recipes/loom-recipe.yaml` — removed the three validate rows, rewired edges, rewrote header prose
- `/home/knatte/Code/loomyard/wts/producer-gates/internal/loomshed/gates.go`, `loomshed.go`, `interruptpolicy.go`, `fixture_test.go` (new), `gatefindings_test.go`, `cancellation_test.go` — deleted producers, moved helpers, retargeted guards
- `/home/knatte/Code/loomyard/wts/producer-gates/internal/shedrecipe/entries_simple.go`, `registry.go`, `recipe.go`, and their test files — deleted registry entries
- `/home/knatte/Code/loomyard/wts/producer-gates/internal/loomrecipe/coverage_guard_test.go`, `shape_test.go`, `recipe_test.go`, `sequence_test.go`, `approveseam_test.go`, `resume_test.go`, `fixture_test.go`, `gatequiescence_test.go` (new) — re-pointed at the fourteen-row recipe
- `/home/knatte/Code/loomyard/wts/producer-gates/internal/loomcli/parity_test.go` — rewritten against the two gate closures
- `/home/knatte/Code/loomyard/wts/producer-gates/_mill/plan/05-row-removal.md` — extended card 29's Edits list for the discovered `gates_test.go` fixture-rename fallout

{"status":"success","commit_sha":"6d0e082d356dbb8a9e14821721097245bd2cc4d9","session_id":"b463eac3-64f5-447c-9b1c-42513ba2a433","cards_done":[28,29,30,31,32,33,34,35,36]}
