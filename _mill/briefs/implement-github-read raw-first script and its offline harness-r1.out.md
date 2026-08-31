All 8 declared cards (8, 9, 10, 11, 12, 13, 14, 15) have matching commits — 8 of 8 cards committed, all complete.

{"status":"success","commit_sha":"a3c610b8ef0ea58b1d942fc70432aacc5edf0a7a","session_id":"73f7cccf-8345-4178-87c6-9e48f639d81c","cards_done":[8,9,10,11,12,13,14,15]}

Summary: all 8 cards (8 through 15) of batch 2 ("github-read raw-first script and its offline harness") were completed and committed, and the batch's verify command (`bash plugins/prowler/scripts/github-read-selftest.sh`) passes with 57/57 assertions.

Key files created:
- `/home/knatte/Code/loomyard/wts/raw-fetch-tree-scoping/plugins/prowler/scripts/github-read.sh` — the raw-first read script
- `/home/knatte/Code/loomyard/wts/raw-fetch-tree-scoping/plugins/prowler/scripts/github-read-selftest.sh` — the offline harness
- `/home/knatte/Code/loomyard/wts/raw-fetch-tree-scoping/plugins/prowler/scripts/testdata/github-read/bin/gh` and `.../bin/curl` — stubs
- `/home/knatte/Code/loomyard/wts/raw-fetch-tree-scoping/plugins/prowler/scripts/testdata/github-read/bodies/*` — fixtures
- `/home/knatte/Code/loomyard/wts/raw-fetch-tree-scoping/plugins/prowler/scripts/testdata/github-read/CAPTURE.md` — live-capture audit trail (private repo names redacted; symlink/submodule/>1MB-file captures documented as not observed and derived, per the plan's disposition)

{"status":"success","commit_sha":"a3c610b8ef0ea58b1d942fc70432aacc5edf0a7a","session_id":"73f7cccf-8345-4178-87c6-9e48f639d81c","cards_done":[8,9,10,11,12,13,14,15]}