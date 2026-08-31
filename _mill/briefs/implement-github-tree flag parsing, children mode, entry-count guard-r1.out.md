That's the brief file itself, out of scope (not a card edit). No tracked modifications remain.

Card-count self-check: 7 of 7 cards committed (cards 1-7), matching the batch file's declared 7 cards. Verify (`bash plugins/prowler/scripts/github-tree-selftest.sh`) passes with all 44 assertions green.

{"status":"success","commit_sha":"feba7ea332249e7516aa45e2a51342e434859b94","session_id":"7ce70db0-a862-4a6b-a274-cfaea6c48b5b","cards_done":[1,2,3,4,5,6,7]}

Summary of work: implemented all 7 cards of batch 1 in `/home/knatte/Code/loomyard/wts/raw-fetch-tree-scoping/plugins/prowler/scripts/github-tree.sh` (leading-flag parser with `--` terminator, incremental entry-count guard, `--children` non-recursive mode) and its offline harness (`plugins/prowler/scripts/github-tree-selftest.sh`, `plugins/prowler/scripts/testdata/github-tree/bin/gh`, plus new fixtures `children-src-nonrec.json` and `children-empty-nonrec.json`). All 44 harness assertions pass (22 pre-existing + 22 new), and the working tree is clean aside from the pre-existing untracked brief file, which is outside this batch's scope.

{"status":"success","commit_sha":"feba7ea332249e7516aa45e2a51342e434859b94","session_id":"7ce70db0-a862-4a6b-a274-cfaea6c48b5b","cards_done":[1,2,3,4,5,6,7]}