# Writer cost remeasure

The scripts and tables behind [report.md](report.md): the writer-lookups measurement and the plan writer audit,
re-run on the tapestry runs that landed after `3085f451`.
The tables record one run on one machine and are not claiming to stay true.

## Rerunning

Run each script from any working directory; the repo root is found from the script's location.
Environment:

- `PROJECTS` names the transcript root; it defaults to `~/.claude/projects`.
- `SCRATCH` (`run-all.sh`) and `OUT` (`plan-table.sh`) name the work directory;
  they default to `<repo root>/.scratch/writer-cost-remeasure`.
- `CUTOFF` names the first session date of `run-all.sh`; it defaults to 2026-11-05.
- `WEFT` names the records repo clone; it defaults to `<repo root>/../loomyard-weft`.

The scripts need `bash`, `git`, `jq` and `awk`; `plan-table.sh` also needs `go` with cgo to build `tools/tokencount`.
Everything is read-only apart from the tables and the work directory.

## Scripts and tables

- `run-all.sh` extracts turns and calls of the slugs in `slugs.txt` with the turn audit's `extract-turns.sh`,
  then runs `writers.sh` (`writer-turns.tsv`, `writer-sessions.tsv`, `writer-chains.tsv`)
  and `subagents.sh` (`writer-subagents.tsv`); it also leaves `turns.tsv` and `calls.tsv`.
- `carry.sh <label> <dir>` prices the carry of lookup results per session;
  `carry.tsv` holds this sample and the 2026-11-04 sample.
- `session-table.sh <label> <dir>` writes one row per session (`session-table.tsv`);
  `lookup-context.sh <slugs-file> <cutoff>` the context each lookup subagent read in (`lookup-context.tsv`);
  `plan-reads.sh <slugs-file> <cutoff>` the size of the Plan writer's own lookup results by call kind (`plan-reads.tsv`).
- `plan-table.sh` writes `plan-table.tsv`, one row per run of `plan-slugs.txt`:
  Plan phase cost by part, rounds, Plan-Write time, plan cards and changed lines.
- `audit-sizes.sh` writes `audit-sizes.tsv`: the sizes of the 2026-10-24 audit's runs beside its committed costs.
- `group-means.sh` writes `group-means.tsv`: group means and size-normalised figures.
- `timing-plan-phase.tsv` holds the per-run phase times of `lym report timing --format json`, saved by hand;
  that verb runs from the prime worktree only.
