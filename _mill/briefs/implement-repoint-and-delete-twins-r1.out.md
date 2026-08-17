All 12 cards (Card 3 through Card 14) are committed, one commit per card, matching the batch's declared 12 cards. All committed to remote. Verify passes fully.

12 of 12 cards committed (Cards 3–14). Both verify sub-invocations pass, gofmt clean, git status clean of tracked in-scope changes.

Key files touched:
- `/home/knatte/Code/loomyard/wts/planparser-plan-dir/internal/loomengine/plan.go`
- `/home/knatte/Code/loomyard/wts/planparser-plan-dir/internal/loomengine/plan_test.go`
- `/home/knatte/Code/loomyard/wts/planparser-plan-dir/internal/loomengine/config.go`
- `/home/knatte/Code/loomyard/wts/planparser-plan-dir/internal/loomengine/planpath_test.go` (deleted)
- `/home/knatte/Code/loomyard/wts/planparser-plan-dir/internal/webstercli/cli.go`
- `/home/knatte/Code/loomyard/wts/planparser-plan-dir/internal/webstercli/cli_test.go`
- `/home/knatte/Code/loomyard/wts/planparser-plan-dir/internal/webstercli/verbs_test.go`
- `/home/knatte/Code/loomyard/wts/planparser-plan-dir/cmd/lyx/constructoranchoring_test.go`
- `/home/knatte/Code/loomyard/wts/planparser-plan-dir/cmd/lyx/notransients_test.go`
- `/home/knatte/Code/loomyard/wts/planparser-plan-dir/internal/websterengine/runlevel.go`
- `/home/knatte/Code/loomyard/wts/planparser-plan-dir/CONSTRAINTS.md`
- `/home/knatte/Code/loomyard/wts/planparser-plan-dir/docs/overview.md`

One notable fixture fix beyond the plan's literal wording: in `internal/webstercli/verbs_test.go`'s `newVerbsFixture`, flipping `AnchorRel` to `"backend"` broke `TestBeginBatchCmd_HappyPath`/`TestRecordBatchCmd_Envelope`/`TestRecoverBatchCmd_RunningThenTerminal` because `RunDeps.WorktreeRoot` (`= layout.AnchorPath()`, unchanged production code, out of this batch's scope) is passed to `gitrepo.Repo.CurrentSHA()`, which requires a literal `.git` at that exact path (go-git's `PlainOpenWithOptions` with no `DetectDotGit`). Per the card's own guidance ("the fix is to make the fixture create the anchored directory it needs — never to revert the flip"), I added a `seedAnchoredGitLink` helper that writes a `.git` gitlink file at the anchored subdirectory pointing straight at the real worktree's `.git`, so reads through it see the same live HEAD rather than a stale detached SHA.
