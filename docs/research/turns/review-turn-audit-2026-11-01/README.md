# Review turn audit

The scripts and tables behind [report.md](report.md): the turns, context and cost of the Discussion and Plan review segment (reviewer, fixer, bouncer seed and judge, and their forks) on three full tapestry runs.
The tables record one run on one machine and are not claiming to stay true.

## Rerunning

Run `run-all.sh [out-dir]` from any working directory; the repo root is found from the script's location.
`out-dir` defaults to this folder.
Everything is read-only apart from the tables and the work directory.

Environment:

- `PROJECTS` names the transcript root; it defaults to `~/.claude/projects`.
- `SCRATCH` names the work directory; it defaults to `<repo root>/.scratch/review-turn-audit/`.

The scripts need `bash`, `jq` and `awk`.
No Go build is involved.

## Adding a run

Add the task slug to `slugs.txt`; its transcripts must be under `$PROJECTS/-home-knatte-Code-loomyard-LYMHUB-<slug>/`.
Then rerun `run-all.sh` and update the report's tables.

## Scripts

- `run-all.sh` runs the rest in order.
  It runs a copy of the turn audit's `extract-turns.sh` beside this folder's `slugs.txt`, keeps the segment's roles, drops the replayed first turn of every fork, and reuses the turn audit's `session-metrics.sh`, `role-summary.sh`, `merge-kinds.sh` and `kinds-total.sh`.
- `session-costs.sh` prices every segment session at the list price of its model and again at the Sonnet row.
  It stops on any model other than Opus 5.5 and Sonnet 5.5.
- `role-table.sh`, `rounds-table.sh`, `merge-saving.sh` and `fixer-cost.sh` aggregate the tables the report quotes.
- `where-turns-go.sh` splits each role's weight and time by activity group.
