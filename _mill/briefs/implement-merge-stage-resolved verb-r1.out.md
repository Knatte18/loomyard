All 6 cards committed (matching all 6 `Commit:` messages in the batch file), working tree is clean, and both verify sub-invocations passed.

6 of 6 cards committed. All batch commits are present in the log, `git status --porcelain --untracked-files=no` shows no tracked in-scope modifications, and the full verify command (`go test ./internal/gitrepo/... ./internal/fabricengine/... ./cmd/lyx/...` followed by `go test -tags integration ./internal/gitrepo/... ./internal/fabricengine/...`) passed.

Relevant files:
- `/home/knatte/Code/loomyard/wts/landing-publish-finalize-producers/internal/gitrepo/merge.go` (Card 1: `StageResolved`)
- `/home/knatte/Code/loomyard/wts/landing-publish-finalize-producers/cmd/lyx/gitrepoboundary_test.go` and `/home/knatte/Code/loomyard/wts/landing-publish-finalize-producers/CONSTRAINTS.md` (Card 2)
- `/home/knatte/Code/loomyard/wts/landing-publish-finalize-producers/internal/fabricengine/mutation.go` (Card 3: `KindMergeResolvedStaged`)
- `/home/knatte/Code/loomyard/wts/landing-publish-finalize-producers/internal/fabricengine/mergestage.go` (Card 4: `StageResult`, `MergeStageResolved`)
- `/home/knatte/Code/loomyard/wts/landing-publish-finalize-producers/cmd/lyx/destructiveguard_test.go` (Card 5)
- `/home/knatte/Code/loomyard/wts/landing-publish-finalize-producers/internal/fabricengine/mergestage_integration_test.go` (Card 6)

{"status":"success","commit_sha":"cb3289108dbbc78280db30deb7d52b4d8e60ea6e","session_id":"5b55eff4-e53b-40d8-959f-41bccd8f2cdf","cards_done":[1,2,3,4,5,6]}
