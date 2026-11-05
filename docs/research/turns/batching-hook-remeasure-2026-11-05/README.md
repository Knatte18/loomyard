# Batching hook re-measure

The scripts and tables behind [report.md](report.md): the batching breach hook and the edit-round rule, read from the transcripts of the darn and tapestry runs that landed after the hook reached forks, against the sample of `docs/research/turns/turn-savings-measure-2026-11-04`.
The tables record one run on one machine and are not claiming to stay true.

## Rerunning

Run `run-all.sh [out-dir]` from any working directory; the repo root is found from the script's location, and the turn audit scripts it reuses are read from the sibling folders of `docs/research/turns`.
`out-dir` defaults to this folder, with one subfolder per sample: `new` and `baseline`.

Environment:

- `PROJECTS` names the transcript root; it defaults to `~/.claude/projects`.
- `SCRATCH` names the work directory; it defaults to `<repo root>/.scratch/batching-hook-remeasure/`.
- `NEW_CUTOFF` and `BASELINE_CUTOFF` name the first session date of each sample; they default to 2026-11-04 and 2026-11-02.

The scripts need `bash`, `jq`, `awk`, `git` and `go`.
Everything is read-only apart from the tables and the work directory.

## Adding a run

Add the slug to `slugs.txt` and a line with its recipe to `recipes.tsv`; its transcripts must be under `$PROJECTS/-home-knatte-Code-loomyard-LYMHUB-<slug>/`.
The baseline takes `baseline-slugs.txt` and `baseline-recipes.tsv` the same way.
Then rerun `run-all.sh` and update the report's tables.

## Scripts

- `run-all.sh` runs the rest for both samples; the extractors it runs come from `docs/research/turns/turn-audit-2026-10-28` and `docs/research/turns/review-turn-audit-2026-11-01`, plus `tools/forkaudit/forkaudit.sh` and `tools/tokencount`.
- `events.sh` with `events.jq` writes `events.tsv`: each batching hook firing, each call a guard refused, and each `lym gate test`, validator and lint result, by session and turn.
- `breaches.sh` writes `breaches.tsv`, the turns that make the hook's breaches, and `summary.sh` sums them into `breach-summary.tsv` by recipe and role.
- `edit-chains.sh` writes `edit-chains.tsv`, the edit chains that hold an edit follow with what preceded each: a failed gate, a failed validator or lint call, or neither; `edit-chains-summary.sh` sums them into `edit-chains-summary.tsv` by session kind.
- `darn-writers.sh` writes `darn-writers.tsv`: turns, calls, Edit-tool and shell edits and check turns per darn writer.
- `gate-loops.sh` writes `gate-loops.tsv`, the card gate fix loops; `PENDING_AS_FAIL=1` gives `gate-loops-upper.tsv`, the upper bound that counts a call with no recorded verdict as a failure.
