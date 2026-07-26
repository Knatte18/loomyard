Both cards in this batch (Card 10 and Card 11) are committed, with an extra plan-edit commit for a mechanical test-fallout addition to card 10's scope. Verify (`go test ./internal/treadleengine/... ./internal/perchengine/... ./internal/perchcli/... ./cmd/lyx/...`) passes. Working tree is clean (no uncommitted tracked changes).

Summary of files touched:
- `/home/knatte/Code/loomyard/wts/treadle/internal/treadleengine/profile.go` — added `Profile.PreRoundTargeting`
- `/home/knatte/Code/loomyard/wts/treadle/internal/treadleengine/runner.go` — added `AttemptInput.SeedPath`
- `/home/knatte/Code/loomyard/wts/treadle/internal/treadleengine/roundfiles.go` — added `roundArtifactPaths.Seed`
- `/home/knatte/Code/loomyard/wts/treadle/internal/treadleengine/roundfiles_test.go` — mechanical fallout fix (added to plan scope via `9e785085`)
- `/home/knatte/Code/loomyard/wts/treadle/internal/treadleengine/state.go` — `roundRecord.SeedPath`, `moveStaleArtifacts` now covers the seed path
- `/home/knatte/Code/loomyard/wts/treadle/internal/treadleengine/run.go` — pre-round targeting wiring (`runPreRoundTargeting`, restructured `runRound`/stale-move ordering)
- `/home/knatte/Code/loomyard/wts/treadle/internal/treadleengine/template.go` — embeds `targeting-template.md`
- `/home/knatte/Code/loomyard/wts/treadle/internal/treadleengine/targeting.go` — new: `runTargeting`
- `/home/knatte/Code/loomyard/wts/treadle/internal/treadleengine/targeting-template.md` — new template
- `/home/knatte/Code/loomyard/wts/treadle/internal/treadleengine/engine_test.go`, `template_test.go`, `state_test.go` — card 11 test coverage
- `/home/knatte/Code/loomyard/wts/treadle/_mill/plan/03-preround-targeting.md` — scope extension for `roundfiles_test.go`

4 of 4 units of work complete (2 cards + the required plan-edit commit that preceded card 10's file edit). No cards remain in this batch.

{"status":"success","commit_sha":"be76eb008654c1d5822e76c2343ff7192b7b1dad","session_id":"abe7575d-69ed-4c9b-994e-d198abf37633","cards_done":[10,11]}
