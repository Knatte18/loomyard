{"status":"success","commit_sha":"7e303f68a5e2225eb953b0481e330fb8e4577124","session_id":"f5ff46f7-1eb5-4366-b8ac-33919541b9d6","cards_done":[21,22,23,24,25,26]}

Summary: All 6 of 6 cards in batch `06-recipe-wiring-and-regression.md` are committed (cards 21-26). Verify command `go test ./internal/loomrecipe/...` passes. Working tree is clean.

Files touched:
- `/home/knatte/Code/loomyard/wts/loom-plan-approval-gate/contracts/recipes/loom-recipe.yaml` — added `approve_seam: plan` to `Plan-Bouncer`, corrected `Plan-Burler`'s `fasit.instructions`.
- `/home/knatte/Code/loomyard/wts/loom-plan-approval-gate/internal/loomrecipe/fixture_test.go` — filled `Env.ApprovePlan` with a real `planparser.SetApproved` closure, flipped the fake `Plan-Write` writer and seed to unapproved, renamed `fakeLoomBurler`'s corruption field to `injectOrphanCardDir`.
- `/home/knatte/Code/loomyard/wts/loom-plan-approval-gate/internal/loomrecipe/sequence_test.go` — added the post-run `Approved == true` assertion and updated doc comments.
- `/home/knatte/Code/loomyard/wts/loom-plan-approval-gate/internal/loomrecipe/revalidate_test.go` — re-pointed the regression at the orphan-card-file corruption (`index-file-mismatch`).
- `/home/knatte/Code/loomyard/wts/loom-plan-approval-gate/internal/loomrecipe/approveseam_test.go` (new) — dynamic and static negative-case tests for the approval seam wiring.
- `/home/knatte/Code/loomyard/wts/loom-plan-approval-gate/internal/loomrecipe/shape_test.go` — filled `ApprovePlan` with a no-op closure in `testEnv`.

Commits: `2ca75a19`..`7e303f68` on branch `loom-plan-approval-gate`, all pushed.
