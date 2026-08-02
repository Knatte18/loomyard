1 of 1 card committed (Card 1), matching the batch's declared card total exactly.

Summary: 1 of 1 cards committed — batch complete.

- `/home/knatte/Code/loomyard/wts/webster-fork-context-hygiene/internal/hubgeometry/hubgeometry.go` — added `PlanDirRel()` accessor.
- `/home/knatte/Code/loomyard/wts/webster-fork-context-hygiene/internal/planparser/plan.go` — added `Card.SourcePath` field.
- `/home/knatte/Code/loomyard/wts/webster-fork-context-hygiene/internal/planparser/parse.go` — populated `SourcePath` at parse time; renamed shadowed local `path` var to `filePath` to avoid colliding with the newly imported stdlib `path` package.
- `/home/knatte/Code/loomyard/wts/webster-fork-context-hygiene/internal/planparser/parse_test.go` — added `TestParsePlan_Card_SourcePath` (single- and multi-card cases).

Verify (`go build ./... && go test ./internal/planparser/... ./internal/hubgeometry/...`) passed. Commit `3d15d34ec3b97dfeaf7fd3243ff2d2a8f617c9fa` pushed to `webster-fork-context-hygiene`.

{"status":"success","commit_sha":"3d15d34ec3b97dfeaf7fd3243ff2d2a8f617c9fa","session_id":"8d6cb5cd-0206-4806-89d3-21a472263b6a","cards_done":[1]}
