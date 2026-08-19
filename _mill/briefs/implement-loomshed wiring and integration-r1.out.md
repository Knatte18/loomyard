All 5 cards (31-35) have matching commits in the log, plus the plan-edit commit. 5 of 5 cards committed. Verify passed fully (both untagged and integration tiers). No uncommitted tracked changes.

{"status":"success","commit_sha":"6b0303fc69593ae02bebe0285031fe68c89730f2","session_id":"0af3fbd3-bbc9-4e77-9795-824f5e2d3992","cards_done":[31,32,33,34,35]}

## Summary

All 5 cards in batch 05 (loomshed wiring and integration) are committed, 5 of 5 -- verified honestly against the commit log.

Key files touched:
- `/home/knatte/Code/loomyard/wts/landing-publish-finalize-producers/internal/loomshed/loomshed.go` -- added `Deps.Landing`, wired real `NewPublish`/`NewFinalize` into rows 12/13.
- `/home/knatte/Code/loomyard/wts/landing-publish-finalize-producers/internal/loomshed/seam_enforcement_test.go` -- allowlisted the new `landingshed` import.
- `/home/knatte/Code/loomyard/wts/landing-publish-finalize-producers/internal/loomshed/stub.go`, `stub_test.go` -- corrected the stub inventory to six rows.
- `/home/knatte/Code/loomyard/wts/landing-publish-finalize-producers/internal/loomshed/loomshed_test.go`, `fixture_test.go`, `sequence_test.go`, `resume_test.go` -- new wiring assertions plus a necessary redesign of the full-sequence/resume tests' terminal point (now `RunBlocked` at Publish's own told-skip gate rather than `RunDone` through a stub-backed Finalize, since the real Finalize needs a genuine git pair that the untagged tier may not spawn).
- `/home/knatte/Code/loomyard/wts/landing-publish-finalize-producers/internal/landingshed/testmain_integration_test.go`, `finalize_integration_test.go`, `publish_integration_test.go` -- new integration coverage against real hubforge pairs, only the model/GitHub seams faked.
- `/home/knatte/Code/loomyard/wts/landing-publish-finalize-producers/_mill/plan/05-loomshed-wiring-and-integration.md` -- extended card 34's `Edits:` scope (with its own commit) to cover the fixture files the wiring change collaterally forced open, per the brief's file-discovery protocol.

Notable finding worth flagging to the reviewer: wiring the real Publish/Finalize producers into loomshed's list made the pre-existing `TestSequence_FullRunReachesDone` and three `resume_test.go` suites unable to reach `Done` through a real `Finalize.Call()` in the untagged tier (it needs a genuine two-worktree pair and therefore git). I resolved this by configuring the shared fixture's landing passthrough so Publish's own told-skip gate reports `Stuck` before ever touching its resolver -- the run now legitimately blocks at Publish instead -- and updated those tests' terminal-state assertions accordingly rather than weakening any coverage.

Full `verify:` command (both the untagged and `-tags integration` halves) passed. No uncommitted tracked changes remain.
