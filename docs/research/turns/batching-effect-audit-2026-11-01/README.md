# Batching effect audit

The scripts and tables behind [report.md](report.md): the Plan and Discussion writers of loom runs before and after the batching sentences of `bf00a043`.
The tables record one run on one machine and are not claiming to stay true.

## Rerunning

Run `run-all.sh [out-dir]` from any working directory; the repo root is found from the script's location.
`out-dir` defaults to this folder.
Everything is read-only apart from the tables and the work directory.

Environment:

- `PROJECTS` names the transcript root; it defaults to `~/.claude/projects`.
- `WEFT` names the records repo clone for `task-size.sh`; it defaults to `<repo root>/../loomyard-weft`.
- `SCRATCH` names the work directory; it defaults to `<repo root>/.scratch/batching-effect-audit/`.

The scripts need `bash`, `git`, `tar`, `jq` and `awk`.
No Go build is involved.

## Adding a run

1. Add the task slug to `slugs.txt`.
   Its transcripts must be under `$PROJECTS/-home-knatte-Code-loomyard-LYMHUB-<slug>/`.
2. Add a line to `runs.tsv`: the slug, a tab, and the run's `archive/<slug>/<hash>` tag in the records repo (`git -C <records repo> tag -l 'archive/*'`).
   A run that has not landed takes `-` and is read from its weft worktree `<hub>/<slug>-weft`; it appears in `writers.tsv` but not in `groups.tsv`.
3. Rerun `run-all.sh` and update the report's tables.

## Scripts

- `run-all.sh` runs the rest in order.
  It runs a copy of the turn audit's `extract-turns.sh` beside this folder's `slugs.txt`, keeps the `plan`, `plan+fork`, `discussion` and `discussion+fork` roles, and reuses the turn audit's `session-metrics.sh` for the serial-turn counts.
- `list-price.sh` prices every writer session at the Claude API list price of `claude-opus-5-5` and assigns the group: a session is after when its transcript holds the stencils' batching sentence.
  It stops on any other model, since the price row is fixed.
- `task-size.sh` reads the decision record and the plan cards of each run.
- `writers-table.sh` joins sessions, turns, prices and sizes into `writers.tsv`; `group-means.sh` averages the landed rows into `groups.tsv`.
