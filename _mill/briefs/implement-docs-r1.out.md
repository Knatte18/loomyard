No dirty tracked files. One commit matches Card 14's `Commit:` message exactly. This batch declares 1 card, and it is fully committed — 1 of 1 cards committed.

{"status":"success","commit_sha":"175bf2a5","session_id":"0ede54e7-7a74-48b5-8580-8a1add767476","cards_done":[14]}

Summary: This batch had exactly 1 card (Card 14), and it is fully committed as `175bf2a5`. 1 of 1 cards committed — all complete.

Files changed:
- `/home/knatte/Code/loomyard/wts/fabric-commit-api/internal/fabricengine/doc.go` — added package-comment paragraphs documenting `Fabric.Commit`'s classify-and-dispatch/warp-first/report-not-rollback behavior, the async push's `SkipGit`/`SkipPush` scoping, `Fabric.Diff`/`Fabric.Status`, the three status surfaces, and the `.gitrepo-push.lock` warp/weft asymmetry.
- `/home/knatte/Code/loomyard/wts/fabric-commit-api/manifest/designs/fabric-unified-view.md` — marked build-order item 2 DONE and resolved its two open questions (partial-failure semantics, Diff CLI-vs-Go-internal).
- `/home/knatte/Code/loomyard/wts/fabric-commit-api/CONSTRAINTS.md` — discharged the Weft Git Invariant's `Fabric.Commit` forward-reference.

Verify (`go build ./internal/fabricengine/`) passed. Commit was pushed to `fabric-commit-api` (remote `cb718bf7..175bf2a5`).

{"status":"success","commit_sha":"175bf2a5","session_id":"0ede54e7-7a74-48b5-8580-8a1add767476","cards_done":[14]}