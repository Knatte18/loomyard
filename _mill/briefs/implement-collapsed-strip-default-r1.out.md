1 of 1 card committed. Batch fully complete: the single card (Card 1, the only card in this batch) has been committed with the exact `Commit:` message, verify passes, and there are no uncommitted tracked changes.

Summary of work:
- `/home/knatte/Code/loomyard/wts/reed-collapsed-strip-readability/internal/reedengine/config_test.go` — both `CollapsedStripRows` default assertions moved from 3 to 6 (explicit-template test and degrade-to-embedded-template test), including the `want` text in the error format strings.
- `/home/knatte/Code/loomyard/wts/reed-collapsed-strip-readability/internal/reedengine/template_posix.yaml` and `template_windows.yaml` — `collapsed_strip_rows` raised from 3 to 6 with an extended inline comment carrying the readability rationale and the reconcile-adoption caveat; both files remain byte-identical on this line.
- `/home/knatte/Code/loomyard/wts/reed-collapsed-strip-readability/internal/reedengine/doc.go` — the "Silent layout rescale" anecdote's `3` reworded to note it was the then-default (now 6), with only that bullet reflowed.

Verify (`go test ./internal/reedengine/...`) is green. Commit `328a8abf90415e538e1ec808a62dcb29edfd4272` pushed to branch `reed-collapsed-strip-readability`.

{"status":"success","commit_sha":"328a8abf90415e538e1ec808a62dcb29edfd4272","session_id":"737a74a4-c346-4c5a-8962-f7e1da90be00","cards_done":[1]}
