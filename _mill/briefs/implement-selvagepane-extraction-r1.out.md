40 chars + newline, correct. All 9 cards committed, verify passed. Final report.

All 9 cards from the batch plan are committed and `go test ./internal/reedengine/` passes in full (including the enforcement test, which is red at commit 1 and green from commit 2 onward). The working tree is clean with no uncommitted in-scope changes.

Card-count self-check: 9 of 9 cards committed (cards 1–9, one commit each).

Key files touched:
- `/home/knatte/Code/loomyard/wts/reed-selvage-pane-extraction/internal/reedengine/selvagepane_enforcement_test.go` (new)
- `/home/knatte/Code/loomyard/wts/reed-selvage-pane-extraction/internal/reedengine/selvagepane.go` (new)
- `/home/knatte/Code/loomyard/wts/reed-selvage-pane-extraction/internal/reedengine/selvagepane_test.go` (new)
- `/home/knatte/Code/loomyard/wts/reed-selvage-pane-extraction/internal/reedengine/lifecycle.go`
- `/home/knatte/Code/loomyard/wts/reed-selvage-pane-extraction/internal/reedengine/lifecycle_test.go`
- `/home/knatte/Code/loomyard/wts/reed-selvage-pane-extraction/internal/reedengine/reconcile.go`
- `/home/knatte/Code/loomyard/wts/reed-selvage-pane-extraction/internal/reedengine/reconcile_test.go`
- `/home/knatte/Code/loomyard/wts/reed-selvage-pane-extraction/internal/reedengine/spawn.go`
- `/home/knatte/Code/loomyard/wts/reed-selvage-pane-extraction/internal/reedengine/spawn_test.go`
- `/home/knatte/Code/loomyard/wts/reed-selvage-pane-extraction/internal/reedengine/apply.go`
- `/home/knatte/Code/loomyard/wts/reed-selvage-pane-extraction/internal/reedengine/generation.go`

One deviation from the plan's card boundaries worth flagging for review: `planPaneTarget`'s `insertAbove` table cases were added during card 7's relocation (a natural side effect of adapting the signature) rather than deferred to card 9 as the plan's card 7 text specified ("the new insertAbove return is covered by card 9 rather than here"). Card 9's commit notes this and adds the remaining seam-helper tests (`newReapPolicy`, `selvageRenderParams`, `seedSelvageClaim`, `clearSelvagePaneBinding`) as planned. Functionally all required coverage exists by the end of the batch.

{"status":"success","commit_sha":"a336fcca783daf6984580ea0e59046c16f8dc552","session_id":"9431660f-20cf-48d6-8576-fb216d12d565","cards_done":[1,2,3,4,5,6,7,8,9]}
