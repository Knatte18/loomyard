All 1 card (Card 11) committed successfully. Verify (`go test ./internal/loomcli/...`) passes. Working tree clean.

Files touched:
- `/home/knatte/Code/loomyard/wts/loom-plan-write-producer/internal/loomcli/wiring.go` — added `PlanSpec` and `CommitPlan` closures to the `shedrecipe.Env` literal, plus the `planparser` import.
- `/home/knatte/Code/loomyard/wts/loom-plan-write-producer/internal/loomcli/wiring_test.go` — added `seedPlanStencil` helper, wired it into `hubLocation`, added `TestWire_PlanSeamsFilled` and `TestWire_PlanSpecEvaluatesToExpectedShape`.

Card-count self-check: batch declares 1 card (Card 11); 1 card committed this turn (commit `922cb5ea`). 1 of 1 cards committed — fully complete.

{"status":"success","commit_sha":"922cb5ea1e3543c38d98f0b63868353bfb138902","session_id":"5fd7211e-e216-4b6c-b4f4-d97e8295ee67","cards_done":[11]}
