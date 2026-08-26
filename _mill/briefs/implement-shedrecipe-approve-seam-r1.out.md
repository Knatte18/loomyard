All 3 cards (14, 15, 16) declared in the batch file have been committed and confirmed against the log. This is the honest count: 3 of 3 cards committed.

Summary:
- Card 14: added `ApprovePlan func() error` to `Env` in `/home/knatte/Code/loomyard/wts/loom-plan-approval-gate/internal/shedrecipe/recipe.go`, placed after `CommitPlan`.
- Card 15: resolved the optional `approve_seam` config key in `/home/knatte/Code/loomyard/wts/loom-plan-approval-gate/internal/shedrecipe/entries_bouncer.go`, mirroring `commit_seam`'s switch, added to the `configRejectUnknown` allowlist, and wired `Approve` into `shedadapters.BouncerConfig`.
- Card 16: added `TestBouncerEntry_ApproveSeam` in `/home/knatte/Code/loomyard/wts/loom-plan-approval-gate/internal/shedrecipe/entries_bouncer_test.go` covering the plan-resolves case, absent-key default, unrecognised-value error, missing-Env-closure guard, and the hyphenated-typo rejection.

Verify (`go test ./internal/shedrecipe/...`) passes. Working tree is clean.

{"status":"success","commit_sha":"a52e69dc236570e799a37c6dab29664b00b957c6","session_id":"3546ca75-750e-4e75-8427-48022f9b3bb6","cards_done":[14,15,16]}
