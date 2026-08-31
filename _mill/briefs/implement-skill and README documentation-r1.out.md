Both card commits are present in the log matching the batch's `Commit:` messages exactly. 2 of 2 cards committed — all complete.

{"status":"success","commit_sha":"380d8e19573d5caf760f73598ec607bd001b1dd7","session_id":"00730930-d951-4b24-a74b-3fcd254ed5fa","cards_done":[16,17]}

Summary: 2 of 2 cards committed this turn (card 16 and card 17), matching the batch's declared card count exactly. `verify: null` for this batch, so no verify step was run, per the batch frontmatter.

Files touched:
- `/home/knatte/Code/loomyard/wts/raw-fetch-tree-scoping/plugins/prowler/skills/github-repo-explorer/SKILL.md` — replaced the base64/raw-alternative read instructions with a single `github-read.sh` call resolved alongside `github-tree.sh`'s path; documented the three tree-listing modes (whole-repo, scoped recursive, `--children`), the children mode's trailing-slash convention, the entry-count guard's default ceiling and abort shape, and extended the exit-code-checking guidance to cover both. Per `CAPTURE.md` capture 8 ("Unresolved"), the conditional symlink sentence was omitted, since the raw host's symlink behavior was never observed.
- `/home/knatte/Code/loomyard/wts/raw-fetch-tree-scoping/plugins/prowler/README.md` — extended the tree-listing section with `--children` and the entry-count guard, rescoped the `gh`-only dependency claim to the tree script, and added a new `github-read.sh` section covering its raw-first behavior, measured cost gap, `gh`-hard/`curl`-optional prerequisite split, and its offline harness (noting neither harness is wired into CI).

Commits: `978deffab` and `380d8e195` on branch `raw-fetch-tree-scoping`, both pushed.

{"status":"success","commit_sha":"380d8e19573d5caf760f73598ec607bd001b1dd7","session_id":"00730930-d951-4b24-a74b-3fcd254ed5fa","cards_done":[16,17]}