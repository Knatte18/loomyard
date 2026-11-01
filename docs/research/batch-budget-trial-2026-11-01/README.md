# Batcher budget trial: cautious versus wide, recheck of 2026-11-01

The recheck of audit `batch-budget-300k-trial`, due once many `wide` runs had landed.
The figures record one run of `compare.sh` on one machine and are not claiming to stay true.
The baseline of 2026-10-24 sits in `../batch-budget-trial-2026-10-24/`.

## Method

`compare.sh` here is the baseline script with one change: it reads both archive layouts, `_lyx/` (runs to 2026-10-26) and `_lym/` (later runs), including the newer `batches` object in `state.json` and the review rounds under `reviews/code/`.
It ran as `compare.sh 2026-10-15` after `git fetch --tags` in the records repo clone.
`runs.tsv` and `forks.tsv` beside this file are its outputs.

Two limits of the data:

- The Claude transcripts of the `cautious` runs, which ended 2026-10-22, are no longer on this machine, so this run measured nothing for `cautious`.
  The `cautious` measured figures below are the 2026-10-24 baseline's, from the same script.
- `wide` runs have a measured fork peak only when their transcripts are on this machine: 8 of 23 runs, 34 forks with an estimate and a peak.
  The other runs ran on another machine or lost their transcripts.
  Their structural columns (cards, batches, estimate, review rounds and findings, recoveries) are complete.

## Runs

23 `wide` runs against 59 `cautious` runs (51 in the baseline; the fetch added older tags).
The `wide` runs span 2026-10-22 to 2026-11-01.

| metric (median) | cautious | cautious >=10 cards | wide |
|---|---|---|---|
| runs | 59 | 22 | 23 |
| cards | 7 | 14 | 18 |
| batches | 3 | 7 | 5 |
| cards/fork | 2.29 | 1.92 | 3.50 |
| est_median_peak | 177798 | 185025 | 254718 |
| measured_median_peak | 170984 (baseline, n=48) | 191997 (baseline, n=16) | 174488 (n=8) |
| measured_max_peak | 232948 (baseline, n=48) | 276642 (baseline, n=16) | 309870 (n=8) |
| webster_cost$ | 9.84 (baseline, n=51) | 19.6 (baseline, n=18) | 16.13 (n=9) |
| $/card | 1.15 (baseline, n=51) | 1.495 (baseline, n=18) | 1.13 (n=9) |
| webster_min | 26.6 (baseline, n=51) | 51.65 (baseline, n=18) | 49.7 (n=9) |
| recoveries | 0 | 0 | 0 |
| notdone_batches | 0 | 0 | 0 |
| webster_review_rounds | 1 | 1 | 1 |
| review_findings | 1 | 2 | 3 |
| gating_findings | 0 | 0 | 0 |

Cost per card of the six `wide` runs of 10 cards or more has median 1.13 (range 1.01 to 2.60), against 1.495 for the `cautious` runs of that size.
The `wide` runs are bigger runs (median 18 cards against 7), which accounts for part of their higher median finding count.

## Fork peak against the batcher's estimate

Forks with an estimate and a measured peak: 34 `wide` here; 173 `cautious` and 3 `wide` in the baseline.

| forks estimated above 200k | cautious (baseline) | wide |
|---|---|---|
| forks | 51 | 29 |
| median peak/estimate | 0.89 | 0.89 |
| mean peak/estimate | 0.92 | 0.86 |
| 90th percentile peak/estimate | not computed | 1.27 |
| highest peak/estimate | 1.95 | 1.34 |
| forks peaking above their estimate | 15 | 9 |
| forks peaking over 300k | 16 | 4 |

Within the `wide` forks estimated above 200k:

- The 20 multi-card forks have mean peak/estimate 1.01; 9 peaked above their estimate and the 4 over 300k are among them.
- The 9 single-card forks have mean 0.61, none above the estimate: the estimate of a lone large card is high.
- The multi-card overshoots reach 1.34 of the estimate; the highest peak is 381k.

`wide` forks overall: mean peak/estimate 0.89, 10 of 34 above their estimate, 20 peaking over 200k, 4 over 300k.

## Findings

- No quality regression: no recoveries, no batch left not done, one review round at the median, no gating finding at the median.
- Cost per card is level with `cautious` overall and lower than `cautious` on runs of 10 cards or more, where the identity-batcher fork cost hurt most.
- The first check's worry, forks 31-39% over the estimate, does not hold up: that was one run.
  Across 29 forks above 200k the median peak is 0.89 of the estimate and the 90th percentile 1.27.
- The 300k budget is overrun by the multi-card tail: 4 of 20 multi-card forks peaked over 300k, the highest at 381k.

## Recommendation

1. **Put `wide` into `internal/batcher/template.yaml`: yes, as the active profile.**
   It costs no more per card, shows no quality loss over 23 runs, and the hub has run it since 2026-10-22 without a recovery.
   Keep `cautious` in the template as the fallback profile.
2. **Calibrate the estimator above 200k: no scaling change now.**
   The median estimate is about 11% high and the mean for multi-card forks is 1.01, so a global factor would shift the tail the wrong way.
   The error is the spread, not the bias: single-card forks run about 40% under their estimate and multi-card forks reach 1.3 to 1.4 over.
   If a calibration is wanted, it is a correction of about -40% to the estimate of a lone card above 200k, which would let the batcher pair such cards; the multi-card tail needs headroom of about 30%, not a lower estimate.
   Recheck both with `compare.sh` once the transcripts of the 15 `wide` runs without measured peaks, or 10 more measured runs, are available.
