All 5 cards (Card 2 through Card 6, as numbered in the batch file) are committed successfully. `go test ./internal/shedrecipe/...` passes cleanly.

Summary of work:
- `/home/knatte/Code/loomyard/wts/shed-recipe-engine-registry/internal/shedrecipe/doc.go` — package doc (Card 2)
- `/home/knatte/Code/loomyard/wts/shed-recipe-engine-registry/internal/shedrecipe/recipe.go` — `Constructor`, `Config`, `Env` types (Card 3)
- `/home/knatte/Code/loomyard/wts/shed-recipe-engine-registry/internal/shedrecipe/config.go` — typed accessors + `configRejectUnknown` (Card 4)
- `/home/knatte/Code/loomyard/wts/shed-recipe-engine-registry/internal/shedrecipe/env.go` and `.../paths.go` — Env validators and `resolveUnderRoot` (Card 5)
- `/home/knatte/Code/loomyard/wts/shed-recipe-engine-registry/internal/shedrecipe/config_test.go`, `env_test.go`, `paths_test.go` — full test coverage (Card 6)

5 of 5 cards committed this turn; all committed and pushed to `shed-recipe-engine-registry`.

{"status":"success","commit_sha":"00321f120393cf412564c2184cbe2e3396909771","session_id":"33dc8d52-8441-48c5-8afb-6b30fd23815f","cards_done":[2,3,4,5,6]}
