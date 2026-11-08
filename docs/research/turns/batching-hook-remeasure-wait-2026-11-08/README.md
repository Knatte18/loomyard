# Batching hook re-measure after the transcript wait

The scripts and tables behind [report.md](report.md): the batching breach hook read from the transcripts of the darn runs that landed after it began to wait for its call to reach the transcript,
against the sample of `docs/research/turns/batching-hook-remeasure-2026-11-05`.
The tables record one run on one machine and are not claiming to stay true.

## Rerunning

Run `run-all.sh [out-dir]` from any working directory; the repo root is found from the script's location.
It runs the extractors and table scripts of `batching-hook-remeasure-2026-11-05` and `turn-audit-2026-10-28` unchanged, and writes one subfolder per sample: `new` and `before`.

Environment:

- `PROJECTS` names the transcript root; it defaults to `~/.claude/projects`.
- `SCRATCH` names the work directory; it defaults to `<repo root>/.scratch/batching-hook-remeasure-wait/`.
- `NEW_CUTOFF` and `BEFORE_CUTOFF` name the first session timestamp of each sample; they default to 2026-11-07T13:15:00 and 2026-11-05.

The scripts need `bash`, `jq`, `awk` and `git`.
Everything is read-only apart from the tables and the work directory.

## Adding a run

Add the slug to `slugs.txt` and a line with its recipe to `recipes.tsv`; its transcripts must be under `$PROJECTS/-home-knatte-Code-loomyard-LYMHUB-<slug>/`.
Then rerun `run-all.sh` and update the report's tables.

## Scripts

- `latency.sh` writes `latency.tsv`: the time from each Read, Grep, Glob, Edit and Write call's transcript line to its result's line, with the call's context and its message's call count.
  `latency-summary.sh` sums it by context band into `latency-summary.tsv`, which prices the hook's wait.
- `nudge-effect.sh` writes `nudge-effect.tsv`: for the breach turns the hook's rule defines, whether the next message batched, split by whether the hook nudged the turn.
- The other tables come from the sibling folder's scripts: `events.tsv`, `breaches.tsv`, `breach-summary.tsv`, `edit-chains.tsv`, `edit-chains-summary.tsv`, and the turn audit's `turns.tsv`, `calls.tsv`, `sessions.tsv` and `role-summary.tsv`.
