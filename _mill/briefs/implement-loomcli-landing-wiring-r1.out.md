All 8 cards (12-19) committed in 7 commits (cards 12 and 13 combined into one commit named after the later card, "loom: test resolveLandingParent"), and `go test ./internal/loomcli/...` passes.

Summary of files touched:
- `/home/knatte/Code/loomyard/wts/landing-parent-fabric-resolution-chain/internal/loomcli/seedinput.go` — added `resolveLandingParent`
- `/home/knatte/Code/loomyard/wts/landing-parent-fabric-resolution-chain/internal/loomcli/seedinput_test.go` — added `TestResolveLandingParent`
- `/home/knatte/Code/loomyard/wts/landing-parent-fabric-resolution-chain/internal/loomcli/landingdeps.go` — new file, `landingDeps` assembly seam
- `/home/knatte/Code/loomyard/wts/landing-parent-fabric-resolution-chain/internal/loomcli/landingdeps_test.go` — new file, reflection drift-guard test
- `/home/knatte/Code/loomyard/wts/landing-parent-fabric-resolution-chain/internal/loomcli/cli.go` — added `registry`, `runner`, `landingCfg` fields to `loomCLI`
- `/home/knatte/Code/loomyard/wts/landing-parent-fabric-resolution-chain/internal/loomcli/wiring.go` — loads `landing.yaml`, carries `registry`/`runner`/`landingCfg` onto the struct, updated comment
- `/home/knatte/Code/loomyard/wts/landing-parent-fabric-resolution-chain/internal/loomcli/wiring_test.go` — `seedLandingConfig`, `hubLocation` update, `TestWire_LandingSeamFieldsPopulated`
- `/home/knatte/Code/loomyard/wts/landing-parent-fabric-resolution-chain/internal/loomcli/drive.go` — fills `Env.Landing` before `loomrecipe.New`

Card-count self-check: 8 of 8 cards committed (cards 12-19), verified against `git log 54109ae8..HEAD --oneline` matching all `Commit:` messages from the batch file. No `Commit: none` cards existed in this batch. `verify: go test ./internal/loomcli/...` passed.

{"status":"success","commit_sha":"a471ff48b8ed9716d1276560a5071befd0a2f7f6","session_id":"92eb9be0-873d-4b62-a26f-7d9ddc098bea","cards_done":[12,13,14,15,16,17,18,19]}