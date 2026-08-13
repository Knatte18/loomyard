All 4 cards (35, 36, 37, 38) committed — matching the batch's declared 4 cards. Full completion confirmed: 4 of 4 cards committed, verify passed.

{"status":"success","commit_sha":"119bf2e67e017c85ec1b1aaa14848254373d5571","session_id":"a63cb2f0-095b-473c-af35-2b13172e53fd","cards_done":[35,36,37,38]}

Summary of work done in this batch (06-stuck-packages):

- `/home/hanf/Code/loomyard/wts/lyxtest-real-hubs/internal/loomengine/export_test.go` (created, card 35) — re-exports `checkResolved` as `CheckResolvedForTest`.
- `/home/hanf/Code/loomyard/wts/lyxtest-real-hubs/internal/loomengine/preflight_integration_test.go` (card 36) — flipped to `package loomengine_test`, built on `hubforge.NewHub`, retargeted `seedRepoWideFabricConfig` onto `hubforge.SeedFabricConfig` (preserving the load-bearing `pathspec: _extra` override), dropped the redundant `gitkit.SeedConfig` call, and removed the now-erroring `git checkout -b WeftBranchName("main")` line.
- `/home/hanf/Code/loomyard/wts/lyxtest-real-hubs/internal/treadleengine/export_test.go` (created, cards 37 and 38) — re-exports `runCircling`/`judgeInputs` (card 37), plus `framingCircling` as `FramingCirclingForTest`, added during card 38 because the compiler required it for `ParseJudgeVerdict`'s call in the flipped smoke test (noted in the card 38 commit message).
- `/home/hanf/Code/loomyard/wts/lyxtest-real-hubs/internal/treadleengine/smoke_judge_test.go` (card 38) — flipped to `package treadleengine_test`, built on `hubforge.NewHub`, dropped the redundant `gitkit.SeedConfig` call.

Verify command (`go vet -tags integration ./... && go vet -tags smoke ./... && go test -tags integration ./internal/loomengine/... ./internal/treadleengine/...`) passed cleanly. All 4 cards committed and pushed to `origin/lyxtest-real-hubs`; no uncommitted tracked changes remain.
