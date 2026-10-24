# Batcher budget trial: cautious versus wide

The comparison script of audit `batch-budget-300k-trial`, with the measured baseline of 2026-10-24.
The script compares Webster runs under the `cautious` batcher profile with runs under `wide`, so the check reruns once more `wide` runs have landed.
The tables below record one run of it on one machine and are not claiming to stay true.

## What the script compares

`compare.sh` walks the records repo's `archive/<slug>/<n>` tags from a start date and groups each run by the profile its own `_lyx/webster/state.json` partition names, the profile Webster actually used, not one guessed from dates.
The switch to `wide` is commit `216c22a12` in the records repo (`batcher.yaml`, `active: wide`), 2026-10-22T15:50:17+02:00.

Per run it records cards, batches, cards per fork, the median estimate, the measured median and maximum fork peak, Webster cost and wall time, recoveries, batches not done, verify-gate reports, review rounds and findings.
Per fork it records the batcher's estimate against the peak the transcript measured.
The summary gives group medians for `cautious`, `cautious` runs of ten cards or more, and `wide`, then fork-level counts of peaks over 200k and 300k tokens.

## Running it

```
docs/research/batch-budget-trial-2026-10-24/compare.sh [FROM-date]
```

- `FROM-date` defaults to `2026-10-15`.
- The repo root is found from the script's location, so any working directory and any worktree of this repo works.
- `WEFT=<records repo>` names the records repo clone; it defaults to `<repo root>/../loomyard-weft`.
- `OUT=<dir>` names the work directory; it defaults to `<repo root>/.scratch/batch-budget-trial/`.
- `FORCE=1` re-runs `tokencount` for runs whose report is already cached.

The script builds `tools/tokencount` into the work directory on first use, which needs `CGO_ENABLED=1` and a C compiler, and needs `git`, `jq` and `awk` on `PATH`.
It writes the cache (`tc/`), the binary, `runs.tsv`, `forks.tsv` and `summary.md` under the work directory, never into `docs/`.
It only reads the repos.

## Inputs

- The records repo's `archive/*` tags: the run's `state.json`, its Webster verify-gate reports and its review round files.
- The local Claude Code transcripts, read through `tools/tokencount`, for cost, wall time and fork peaks.
  A run whose transcripts are not on the machine has `NA` for those columns and no fork rows.

## Baseline

`runs.tsv` (one row per run) and `forks.tsv` (one row per fork) in this folder are the outputs of the 2026-10-24 run, kept unchanged.
A rerun writes its own copies under the work directory; compare against these.

At that run only two `wide` runs had landed, one with transcripts and three forks with an estimate and a peak, so the `wide` column is thin.

| metric (median) | cautious | cautious >=10 cards | wide |
|---|---|---|---|
| runs | 51 | 18 | 2 |
| cards | 6 (n=51) | 14 (n=18) | 32.5 (n=2) |
| batches | 3 (n=51) | 6.5 (n=18) | 15 (n=2) |
| cards/fork | 3.00 (n=51) | 2.145 (n=18) | 3.13 (n=2) |
| est_median_peak | 177798 (n=51) | 185025 (n=18) | 250563 (n=2) |
| measured_median_peak | 170984 (n=48) | 191997 (n=16) | 309570 (n=1) |
| measured_max_peak | 232948 (n=48) | 276642 (n=16) | 315152 (n=1) |
| webster_cost$ | 9.84 (n=51) | 19.6 (n=18) | 14.56 (n=1) |
| webster_forks_cost$ | 6.88 (n=51) | 16.685 (n=18) | 12.95 (n=1) |
| webster_min | 26.6 (n=51) | 51.65 (n=18) | 38.2 (n=1) |
| $/card | 1.15 (n=51) | 1.495 (n=18) | 1.12 (n=1) |
| recoveries | 0 (n=51) | 0 (n=18) | 1 (n=2) |
| notdone_batches | 0 (n=51) | 0 (n=18) | 0 (n=2) |
| verify_gate_reports | 0 (n=51) | 0 (n=18) | 0.5 (n=2) |
| webster_review_rounds | 1 (n=51) | 1 (n=18) | 1 (n=2) |
| review_findings | 1 (n=51) | 2 (n=18) | 2 (n=2) |
| gating_findings | 0 (n=51) | 0 (n=18) | 0.5 (n=2) |

| fork-level | cautious | wide |
|---|---|---|
| forks with estimate+peak | 173 | 3 |
| mean peak/estimate | 0.99 | 1.17 |
| forks peaking over 200k | 59 | 2 |
| forks peaking over 300k | 19 | 2 |
| forks peaking above their estimate | 68 | 2 |
