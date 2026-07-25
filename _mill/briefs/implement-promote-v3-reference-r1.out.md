All 3 cards are committed. Card 1 → `cab6c919`, Card 2 → `e15c38d4`, Card 3 → split across two commits (`9b631b2e` and `5e7f13d3`, both correctly prefixed `3: delete design doc, repoint remaining inbound links` — a `git add` pathspec typo mid-card caused the split, but the working tree is fully clean and all card-3 edits landed under the card's own commit subject). `go build ./...` passes, and all of the batch's grep/consistency checks pass.

Card-count self-check (mandatory, done honestly): 3 of 3 cards committed. All `## Cards` entries from `_mill/plan/01-promote-v3-reference.md` are complete.

Relevant files:
- `/home/knatte/Code/loomyard/wts/plan-format-v3/docs/reference/plan-format-v3.md` (created)
- `/home/knatte/Code/loomyard/wts/plan-format-v3/manifest/designs/webster-rewrite.md` (edited)
- `/home/knatte/Code/loomyard/wts/plan-format-v3/manifest/designs/plan-format-v3.md` (deleted)
- `/home/knatte/Code/loomyard/wts/plan-format-v3/manifest/designs/loom.md` (edited)
- `/home/knatte/Code/loomyard/wts/plan-format-v3/manifest/designs/loom-planner.md` (edited)
- `/home/knatte/Code/loomyard/wts/plan-format-v3/manifest/designs/codeintel-redesign.md` (edited)
- `/home/knatte/Code/loomyard/wts/plan-format-v3/manifest/designs/webster-parallel-execution.md` (edited)

{"status":"success","commit_sha":"5e7f13d3","session_id":"cda6bc45-34ea-41e1-9674-445a1644d9d9"}
