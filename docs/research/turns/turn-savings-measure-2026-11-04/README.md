# Turn savings measure

The scripts and tables behind [report.md](report.md): the effect of project `agent-turns` and of the batching breach hook, read from the transcripts of darn and tapestry runs that landed since 2026-11-02, against the runs `agent-turns` was measured on.
The tables record one run on one machine and are not claiming to stay true.

## Rerunning

Run `run-all.sh [out-dir]` from any working directory; the repo root is found from the script's location.
`out-dir` defaults to this folder, with one subfolder per sample: `new` and `baseline`.

Environment:

- `PROJECTS` names the transcript root; it defaults to `~/.claude/projects`.
- `SCRATCH` names the work directory; it defaults to `<repo root>/.scratch/turn-savings-measure/`.
- `NEW_CUTOFF` and `BASELINE_CUTOFF` name the first session date of each sample; they default to 2026-11-02 and 2026-10-27.

The scripts need `bash`, `jq`, `awk`, `git` and `go`.
Everything is read-only apart from the tables and the work directory.

## Adding a run

Add the slug to `slugs.txt` and a line with its recipe to `recipes.tsv`; its transcripts must be under `$PROJECTS/-home-knatte-Code-loomyard-LYMHUB-<slug>/`.
The baseline takes `baseline-slugs.txt` and `baseline-recipes.tsv` the same way.
Then rerun `run-all.sh` and update the report's tables.

## Scripts

- `run-all.sh` runs the rest for both samples.
  It runs copies of the turn audit's `extract-turns.sh` and of `events.sh` beside each sample's slugs, and reuses the turn audit's `session-metrics.sh` and `role-summary.sh`, `tools/forkaudit/forkaudit.sh`, `tools/tokencount` and the review turn audit's scripts on a staged copy.
- `events.sh` with `events.jq` writes `events.tsv`: each batching hook firing, each call a guard refused and each `lym gate test` result, by session and turn.
- `breaches.sh` writes `breaches.tsv`, the turns that make the hook's breaches, and `summary.sh` sums them into `breach-summary.tsv` by recipe and role.
- `darn-writers.sh` writes `darn-writers.tsv`: turns, calls, Edit-tool and shell edits and check turns per darn writer.
- `gate-loops.sh` writes `gate-loops.tsv`, the card gate fix loops; `PENDING_AS_FAIL=1` gives `gate-loops-upper.tsv`, the upper bound that counts a call with no recorded verdict as a failure.
