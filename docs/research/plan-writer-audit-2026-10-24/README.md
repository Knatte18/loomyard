# Plan writer audit: cost, rounds, wall time and relevant.md coverage

The scripts and tables of audit `plan-writer-context` (A8), with the measured baseline of 2026-10-24.
The scripts measure what the loom Plan phase costs and how much of the code the Plan writer read the Discussion's `relevant.md` named, so the audit reruns once more loom runs have landed.
The tables here record one run on one machine and are not claiming to stay true; the findings are in [results.md](results.md).

## What the scripts measure

- `run-all.sh` writes one `tools/tokencount` report per loom slug in `slugs.txt` (cost, time and sessions per segment) into `tc/`.
- `run-priority.sh` is a second pass over the slugs of `sample-input.tsv` (runs started 2026-10-15 or later) and `gate-runs-once`, skipping reports already written.
- `q1-table.sh` builds `q1-table.tsv`, one row per loom run: Plan phase cost, Plan-Write session cost with and without its twill forks, fork sessions, Plan-Write elapsed and active time, and review rounds.
- `q1-summary.sh` groups `q1-table.tsv` by period of the run's seed start (before 10-15, 10-15 to 10-19, 10-20 to the five-lens cut `c62274be5`, after it) and writes the group means to `q1-summary.tsv`.
- `reads.sh <slug> <archive-tag>` finds the repo files the run's Plan-Write session (transcript title `ly:<slug>:plan`) read and the files its `relevant.md` named, and counts the overlap.
- `q2-table.sh` joins those counts into `q2-table.tsv`: the share of read files `relevant.md` named, and the share of named files read, per run and pooled, with and without `PATTERN.md`, `CLAUDE.md`, `pattern/` and `docs/`.

## Running it

Run from any working directory; the repo root is found from the script's location, so any worktree of this repo works.

1. `run-all.sh`, then `run-priority.sh`.
2. `lyx report timing --since 2026-10-08 --format markdown`, saved by hand; its loom `plan` column is `timing-plan-phase.tsv`.
   No script reads it.
3. `q1-table.sh`, then `q1-summary.sh`.
4. `reads.sh <slug> <archive-tag>` for each row of `sample-input.tsv`, for example `while IFS=$'\t' read -r s t; do ./reads.sh "$s" "$t"; done < sample-input.tsv`, then `q2-table.sh`.

Environment:

- `OUT=<dir>` names the work directory; it defaults to `<repo root>/.scratch/plan-writer-audit/`.
- `WEFT=<records repo>` names the records repo clone for `q1-table.sh` and `reads.sh`; it defaults to `<repo root>/../loomyard-weft`.

Every script writes under the work directory (`tc/`, `reads/`, `slugs-priority.txt`, `slugs-todo.txt`, `slug-tag.tsv`, `q1-table.tsv`, `q1-summary.tsv`, `q2-table.tsv`) and never into `docs/`.
`run-all.sh` and `run-priority.sh` build `tools/tokencount` into `$OUT/tokencount-bin` on first use, which needs `CGO_ENABLED=1` and a C compiler.
The scripts need `git`, `jq`, `awk` and `go` on `PATH` and only read the repos.

## Inputs

- The local Claude Code transcripts under `~/.claude/projects/`, read through `tools/tokencount` for cost and time and by `reads.sh` for the files the Plan writer read.
  A run whose transcripts are not on the machine yields no row.
- `lyx report timing`, for the Plan phase wall time per loom run.
- The records repo: `archive/*` tags, each run's `_lyx/shed/<slug>/seed.json` for its start time and `_lyx/discussion/relevant.md`.
  List the tags with `git -C <records repo> tag -l 'archive/*'`.
- `docs/research/token-audit-2026-10-22/runcost.tsv`, whose loom rows give `q1-table.sh` its run list and its review round counts.
- `slugs.txt` and `sample-input.tsv` in this folder: the loom slugs of the 2026-10-24 run, and the slug and archive tag of the runs `reads.sh` samples.
- `$OUT/extra-loom.tsv`, optional: `slug`, `archive tag` and `loom` as tab-separated columns 2, 3 and 4 for loom archive tags missing from `runcost.tsv`.
  The baseline used one; without it a rerun lists only the runs of `runcost.tsv`.

## Baseline

`results.md` is the write-up of the 2026-10-24 run.
`q1-table.tsv`, `q1-summary.tsv`, `q2-table.tsv`, `reads-summary.tsv` and `timing-plan-phase.tsv` are its tables, kept unchanged.
`results.md` cites the commands as they ran from the prime's `.scratch/a8/`; the commands here replace them.
