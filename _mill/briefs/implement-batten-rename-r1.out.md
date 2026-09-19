All 5 cards (7, 8, 9, 10, 11) are committed, matching the batch file's declared Commit: messages. No tracked uncommitted changes remain. The full verify suite passed: `go build ./...`, all six package test trees, and the integration-tagged `./internal/shedcli/...` run.

Summary of work:
- Card 7: renamed `internal/lifecycleshed` -> `internal/battenshed`, retargeted `internal/shedrecipe/recipe.go` and `registry.go`.
- Card 8: renamed `internal/lifecyclerecipe` -> `internal/battenrecipe`, `contracts/recipes/lifecycle-recipe.yaml` -> `batten-recipe.yaml`, `LifecycleRecipe` -> `BattenRecipe`, and `NameLoomRun`/`"Loom-Run"` -> `NameRunShed`/`"Run-Shed"` (durable-identity value change, sanctioned by the no-migration Shared Decision).
- Card 9: renamed `internal/lifecyclecli` -> `internal/battencli`, the `lyx lifecycle` verb -> `lyx batten`, `LifecycleDir` -> `BattenDir` (directory value `"lifecycle"` deliberately left unchanged), and updated `cmd/lyx/main.go`.
- Card 10: renamed `entries_lifecycle.go`/`_test.go` -> `entries_batten.go`/`_test.go`, `shedcli`'s `"lifecycle"` table key -> `"batten"`. Discovered two files not listed in any card (`internal/shedcli/table_test.go`, `cli_test.go`) that the key rename broke -- added them to card 10's plan scope via a plan-edit commit before touching them, per the brief's rule 2.
- Card 11: retargeted cross-cutting tests and the four falsified `CONSTRAINTS.md` lines (Told-Geometry bound-packages, CLI/Cobra interactive-handoff exception and package-naming deviations, `Lifecycle Bookend Invariant` -> `Batten Bookend Invariant`). Discovered the batch's own `cmd/lyx/...` verify failing against `tools/sandbox/SANDBOX-FABRIC-SUITE.md`'s stale `Covers: lifecycle` tag (module F22) -- added that file to card 11's scope via a second plan-edit commit, then retargeted its heading/tag/prose/CLI examples to `batten`, leaving its `.lyx/lifecycle/<slug>/...` fixture paths untouched since that relocation is batch 6's.

Plan file amended: `_mill/plan/03-batten-rename.md` (two scope-extension edits, each committed separately before the corresponding code change).

{"status":"success","commit_sha":"c0ac05af41eeaf1606918e7da4b662fe8ea1e2bd","session_id":"2b87ce82-4eb2-4fb5-9d44-6fbc5804dbd3","cards_done":[7,8,9,10,11]}

All 5 of 5 cards declared in this batch are committed and verified -- no partial completion.

{"status":"success","commit_sha":"c0ac05af41eeaf1606918e7da4b662fe8ea1e2bd","session_id":"2b87ce82-4eb2-4fb5-9d44-6fbc5804dbd3","cards_done":[7,8,9,10,11]}
