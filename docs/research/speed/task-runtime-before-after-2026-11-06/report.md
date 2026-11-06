# Task run time before and after turn-savings, 2026-11-06

Whether landed runs take less active time since project `turn-savings` (P1: the `agent-turns` changes and the batching work), read from the records of the runs that landed.
No agent ran and no lym code changed; `extract.sh` reads the reports and the archive tags, `analyze.sh` prints every table below.

## Answer

- **Darn: not faster, slower.**
  Active time of a code run is 31.7 min on average before (median 31.3, n = 52) and 45.0 min after (median 39.7, n = 130).
  The slowdown holds in every size band, 1.3 to 1.6 times, and in the fixed-plus-per-line fit.
- **Tapestry: no saving to show.**
  A whole run with a Discussion takes 188.8 min on average before (n = 7) and 244.8 min after (n = 6), for larger runs; per 100 changed lines it is 3.2 and 3.1 min.
  The sample is small and the report makes no claim of significance.
- **P1's share: none that the data can show.**
  The phases `agent-turns` touched did not shrink: darn write went from 16.6 to 20.0 min, tapestry Webster from 74.4 to 111.6 min.
  Several other things changed the same week and pull the other way (see "What else changed"), and these records cannot separate them from P1.
  The per-turn effect of P1 is in [turn-savings-measure-2026-11-04](../../turns/turn-savings-measure-2026-11-04/report.md); a saving per turn and no saving per run are consistent when the runs got more turns elsewhere.

## Sample and method

- Window: every landed run in the timing report, which starts 2026-10-27.
  One look at the archive tags before that date: 469 tags hold `_lyx/shed/<slug>/status.json` with a stamp per row, but no phase, sleep or wait record, and the timing report reads none of them.
  The window stays.
- Periods, by the run's start (UTC): **Before** is a start before `f08df88b` (2026-11-01 15:04), **After** a start after `cfad47e6` (2026-11-01 16:53), and a run between the two belongs to neither (12 darn code runs, none tapestry).
  After splits at the batching hook `eea96fe2` (2026-11-02 12:51), its widening `119c1c05` (2026-11-04 15:15) and the Discussion exploration change `3085f451` (2026-11-05 05:35).
  The commits of P1's members (`9dc4fada`, `a1715fa5`, `8016f5c4`, `1993acac`, `76dd5fb7`, `9d2c1adc`) all lie between the two cut commits, so the cut stands.
- Active time is `active_seconds` of `lym report timing`: the span less machine sleep and less the union of every wait and the paused phase, a wait inside a pause taken out once.
  Phase seconds exclude sleep but include waits, so a phase table shows time with waits inside; the waits are shown apart below.
- Size is the added plus deleted lines of the run's landing commit against its first parent (`git diff --numstat -M`), or the run's own landed-size record where it holds one.
  The convergence report maps a landing commit to most darn runs; a run it leaves unmapped, as it does every tapestry run, takes the one first-parent commit of the parent branch up to 12 minutes before its last stamp (the mapped runs' commits lie 2 to 662 seconds before it).
  A fast-forwarded branch of several commits is measured by its last commit only.
- Darn runs apart: a run whose slug names a bench, trial, audit, measurement, survey or spike, or whose profile is `docs`, is not a code run (43 runs, listed in `runs.tsv`; `codex-astra-fan-bench` is one).
  Profile is recorded only for the 19 runs that hold a usage record, all After, so every other run is `unknown` and the periods are not compared by profile.
  Of the code runs, 25 have their whole span inside machine sleep, so they have no active time and are left out of every active figure; they are counted below.

| Recipe | Before | Between | After | After 1 (to hook) | After 2 (hook) | After 3 (widened) | After 4 (exploration) |
| --- | --- | --- | --- | --- | --- | --- | --- |
| darn, all | 98 | 12 | 152 | 18 | 95 | 4 | 35 |
| darn, bench or docs by name | 28 | 0 | 15 | 3 | 5 | 0 | 7 |
| darn, code, whole span asleep | 18 | 0 | 7 | 4 | 3 | 0 | 0 |
| darn, code, active time measured | 52 | 12 | 130 | 11 | 87 | 4 | 28 |
| darn, code, no machine sleep at all | 51 | 12 | 121 | 11 | 85 | 0 | 25 |
| tapestry | 7 | 0 | 8 | 1 | 3 | 0 | 4 |

## Darn

Code runs, minutes; n beside every row, lines are added plus deleted.
The fit is active minutes on lines per 100, over the runs of the row.

| Period | n | Active mean | median | range | Lines median | Min per 100 lines, pooled | Median of ratios | Fit fixed (min) | Fit per 100 lines (min) | r |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| Before | 52 | 31.7 | 31.3 | 4.2–93.1 | 381.5 | 5.2 | 7.1 | 27.3 | 0.7 | 0.30 |
| After | 130 | 45.0 | 39.7 | 6.3–133.7 | 404 | 7.6 | 11.2 | 41.0 | 0.7 | 0.30 |
| After 1 | 11 | 49.4 | 53.3 | 17.2–66.1 | 357 | 3.3 | 13.6 | 48.1 | 0.1 | 0.21 |
| After 2 | 87 | 44.1 | 40.5 | 7.4–124.6 | 392.5 | 8.6 | 11.3 | 29.0 | 2.9 | 0.69 |
| After 3 | 4 | 47.5 | 30.5 | 12.2–116.9 | 349 | 10.3 | 12.7 | 5.2 | 9.2 | 0.95 |
| After 4 | 28 | 45.6 | 38.5 | 6.3–133.7 | 545.5 | 9.3 | 11.0 | 36.3 | 1.9 | 0.26 |

Size explains little of the time (r about 0.3), so the raw comparison is not misled by size; the same holds inside size bands.
The fit is dominated by a fixed cost per run, and that fixed cost grew from 27 to 41 minutes.
Runs with no machine sleep at all (n = 51 before, 121 after) give the same picture: 31.8 and 45.0 minutes.
After 3 has n = 4 and no sleepless run, so it shows nothing.

| Lines changed | Median active before (n) | Median active after (n) | After / Before |
| --- | --- | --- | --- |
| < 100 | 17.8 (6) | 28.4 (21) | 1.59 |
| 100–299 | 21.9 (14) | 27.9 (32) | 1.27 |
| 300–999 | 34.2 (24) | 51.7 (56) | 1.51 |
| ≥ 1000 | 41.4 (8) | 63.2 (16) | 1.53 |

### Phases

Setup, Finalize and Land were not touched by `agent-turns`; write is the writer, and review the review of the committed diff.
No darn run holds a merge phase.

| Phase | Before n | mean | median | range | After n | mean | median | range |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| setup | 52 | 0.1 | 0.1 | 0–0.1 | 130 | 0.8 | 0.5 | 0–6.2 |
| write (touched) | 52 | 16.6 | 15.1 | 0.5–40.2 | 130 | 20.0 | 16.8 | 0–132.5 |
| review | 52 | 9.8 | 9.6 | 0–30.6 | 130 | 15.1 | 12.6 | 0–69.4 |
| publish and Finalize | 52 | 5.6 | 3.6 | 0.6–51.7 | 130 | 8.6 | 6.4 | 0.8–36 |
| land | 46 | 0.5 | 0.3 | 0.2–8.7 | 129 | 0.6 | 0.5 | 0.1–2.1 |

The writer's median is 15.1 before and 16.8 after, so the writer did not get faster even where `agent-turns` acts on it.
Review and Finalize, which P1 did not touch, grew more than write did.

### What active time leaves out

Mean minutes per run: before 57.6 span, 25.2 sleep, 0.8 halt wait, no escalation, parent-review or PR-gate wait; after 95.9 span and 50.9 sleep, and waits near zero.
Darn runs wait on nobody, so the removed time is almost all machine sleep, and it differs between the periods (the machine slept more over the later days).
Active time is the figure that is comparable.

### Gate time

From `lym gate report`, over the runs that have a row (50 before, 124 after).
Gate time is the summed wall time of the run's tier runs; the slot wait is shown apart.

| Period | Gate wall mean | Slot wait mean | Writer gate wall mean | Active mean | Active less gate wall, mean | median |
| --- | --- | --- | --- | --- | --- | --- |
| Before | 12.1 | 1.2 | 3.9 | 32.1 | 20.0 | 17.7 |
| After | 18.3 | 2.6 | 8.1 | 45.2 | 26.9 | 24.1 |

Gate time grew by about 6 minutes, and the change remains after it is taken out: 20.0 to 26.9 minutes, 17.7 to 24.1 at the median.
The gate report holds no row for 62 of the runs, so these two figures cover about two thirds of them.

### Bench and docs runs

Listed in `runs.tsv` with `bench` true by `analyze.sh`, and not in any mean above.
Their write time is that of what they measure; the 43 runs take 6 to 76 minutes, and one, `turn-savings-measure`, changes 33504 lines of data.
Profile is recorded for 4 of them (`docs`, 11.7 min on average).
Where it is recorded, the profiles differ much: light 29.1 min (n = 7), solid 57.2 (n = 5), speed 16.4 (n = 2), docs 11.7 (n = 4), all After.
A shift in the mix of profiles would move the figures either way, and no Before run records its profile.

## Tapestry

Each run with its phases, minutes; waits and sleep are shown apart, and lines come from the landing commit.

| Run | Start | Period | Class | Lines | Active | Setup | Discussion | Plan | Webster | Merge | Publish and Finalize | Land | Waits | Sleep | Gate wall |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| run-verbs-shared | 10-31 14:10 | Before | whole run | 9150 | 252.8 | 0.1 | 58.8 | 84.1 | 103.6 | 5.2 | 1.9 | 0.3 | 1.1 | 0 | 27.9 |
| interactive-recipe | 10-31 19:22 | Before | whole run | 3630 | 137.7 | 0 | 35.4 | 48.6 | 47.8 | 5 | 1.1 | 0.3 | 0.5 | 557.1 | 19 |
| warp-visibility | 11-01 07:07 | Before | whole run | 7365 | 200.7 | 0.5 | 44.7 | 73.7 | 74.4 | 4.6 | 2.6 | 0.5 | 0.3 | 0 | 30.6 |
| plan-split-producers | 11-01 11:01 | Before | whole run | 7089 | 180.5 | 0.1 | 43.7 | 61.2 | 73.5 | 6 | 1.1 | 0.3 | 5.2 | 0 | 28.8 |
| machine-switch | 11-01 11:53 | Before | whole run | 6734 | 215.1 | 0.1 | 51.4 | 53.6 | 87.5 | 8.9 | 13 | 0.5 | 0 | 0 | 38.6 |
| reed-status-buttons | 11-01 12:03 | Before | whole run | 2695 | 154.2 | 0.1 | 43.2 | 52.4 | 53.6 | 6 | 2.1 | 0.5 | 3.7 | 0 | 30 |
| gate-run-keeps-cache-warm | 11-01 12:21 | Before | whole run | 5068 | 180.8 | 0.1 | 42.6 | 42.1 | 80.6 | 7 | 8.7 | 0.6 | 0.8 | 0 | 38.1 |
| cold-gate-fixer | 11-02 11:20 | After 1 | excluded | - | 153.2 | 0 | 0 | 0 | 16.5 | 37.7 | 98.7 | 0.4 | 0 | 400 | 13.6 |
| decision-record-on-board | 11-03 07:18 | After 2 | whole run | 1969 | 142.6 | 0.2 | 23.1 | 42.5 | 57 | 9.1 | 11.4 | 0.9 | 1.5 | 0 | 44.9 |
| shed-sub-run-block | 11-03 07:21 | After 2 | whole run | 2775 | 121.4 | 0.2 | 31.7 | 36.3 | 48.9 | 7.1 | 6.1 | 1 | 9.8 | 0 | 27.3 |
| batten-remote-order | 11-03 07:28 | After 2 | whole run | 5261 | 296.1 | 0.8 | 55.5 | 88.3 | 140.5 | 6.8 | 4.2 | 0.9 | 0.9 | 0 | 51.7 |
| pawl-orch-landing | 11-05 07:10 | After 4 | whole run | 8484 | 225.4 | 1.7 | 32 | 69.9 | 114.9 | 6 | 1.2 | 0.2 | 0.5 | 0 | 31.9 |
| raddle-recipe | 11-05 08:32 | After 4 | whole run | 11648 | 310.6 | 0.1 | 56.7 | 97.1 | 144.6 | 9 | 3.6 | 0.2 | 0.7 | 0 | 41.6 |
| drift-rework | 11-05 13:15 | After 4 | whole run | 16913 | 373 | 0.3 | 60.9 | 107.1 | 189.4 | 10.6 | 5.6 | 0.3 | 1 | 0 | 52 |
| slack-relay | 11-05 17:13 | After 4 | Sketch-fed | 7416 | 177.4 | 0.2 | - | 72.5 | 86.4 | 9.9 | 8.9 | 0.3 | 0.8 | 0 | 33.3 |

A run starts from a Sketch, and so skips Discussion, when it has no Discussion phase and a stored decision record: `slack-relay` is the one such run.
It is listed by name and enters no mean of whole-run time.
`cold-gate-fixer` shows no Discussion or Plan time (it is a fixer run that starts at its Webster row), so it is left out of both comparisons.
`creel-cross-hub-build` has not landed and is not in the window.

| Measure (minutes) | Before n | mean | median | range | After n | mean | median | range |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| Whole run, active; Discussion runs only | 7 | 188.8 | 180.8 | 137.7–252.8 | 6 | 244.8 | 260.7 | 121.4–373 |
| Discussion phase | 7 | 45.7 | 43.7 | 35.4–58.8 | 6 | 43.3 | 43.8 | 23.1–60.9 |
| Plan through Land, every run but `cold-gate-fixer` | 7 | 144.7 | 142.0 | 102.8–195 | 7 | 199.7 | 192.1 | 99.3–312.8 |
| Plan | 7 | 59.4 | 53.6 | 42.1–84.1 | 7 | 73.4 | 72.5 | 36.3–107.1 |
| Webster | 7 | 74.4 | 74.4 | 47.8–103.6 | 7 | 111.6 | 114.9 | 48.9–189.4 |
| Merge | 7 | 6.1 | 6.0 | 4.6–8.9 | 7 | 8.4 | 9.0 | 6–10.6 |
| Publish and Finalize | 7 | 4.4 | 2.1 | 1.1–13 | 7 | 5.8 | 5.6 | 1.2–11.4 |

Plan through Land adds the phase seconds, waits inside; a tapestry run's waits are 0 to 10 minutes.
The Plan-onward comparison is like for like, Sketch-fed run included, and shows the same direction as the whole run.

| Period | n | Lines mean | median | Min per 100 lines, pooled | Median of ratios | Fit fixed (min) | Fit per 100 lines (min) | r |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| Before | 7 | 5961.6 | 6734 | 3.2 | 3.2 | 97.9 | 1.5 | 0.90 |
| After | 6 | 7841.7 | 6872.5 | 3.1 | 3.5 | 125.6 | 1.5 | 0.88 |

The slower mean is size: the After runs `raddle-recipe` and `drift-rework` change 11648 and 16913 lines, and per 100 lines the two periods are level.
Of the After runs, only 4 started after the Discussion exploration change, so its own effect on Discussion cannot be told from the 6 runs: Discussion takes 43.8 minutes at the median in both periods.

## How much is P1

Phases `agent-turns` touched are write, plan and Webster; the untouched ones are setup, Finalize and Land.

| Recipe | Phase | Before median | After median | Touched |
| --- | --- | --- | --- | --- |
| darn | write | 15.1 | 16.8 | yes |
| darn | review | 9.6 | 12.6 | no |
| darn | publish and Finalize | 3.6 | 6.4 | no |
| tapestry | plan | 53.6 | 72.5 | yes |
| tapestry | Webster | 74.4 | 114.9 | yes |
| tapestry | publish and Finalize | 2.1 | 5.6 | no |

Every phase is slower or level at the median, touched or not, and the writer, plan and Webster show no saving.

### What else changed in the window

Dates are commit times, local.
Each could move the figures either way.

- Recipe rows: `Record-Write` is in 29 % of darn code runs before and 100 % after; `Drift-Recheck` (`843a03f1`, 2026-11-03) is in 42 % after and none before; `Drift-Await` is in 88 % before and 100 % after.
  `Code-Bouncer`, `Code-Burler-Review` and `Code-Burler-Fix` are held by the same share of runs in both periods, with the same visits per run.
- Drift: the capture and judge rework (`925431bd` 2026-11-04 as a design, `3882070d` 2026-11-05 as code, and `98bf70b0` 2026-11-06).
- The arbiter (`e45949b2`, 2026-11-05) and added review: Discussion exploration by subagents (`2d319fe5` 2026-11-02 and `3085f451` 2026-11-05), `Parts-Write` (`02d0f3fa`, 2026-11-01).
- Gate: one failure report (`3a8b8612`), one test binary per package (`6c5727df`, both 2026-11-01), a cold gate fixer (`a9536680`, 2026-11-02) and the darn writer running no gate (`5abbc19f`, 2026-11-03).
- Model: no record in these data says which model each run used; a model change cannot be read from them.
- Runs in parallel: other landed runs in flight at a run's start, 1.8 on average before (n = 105) and 3.4 after (n = 160), and the mean slot wait of a darn run in the gate doubled, 1.2 to 2.6 minutes.
  More parallel runs share the machine, the gate slots and the API.

### What the data supports

- Darn took longer after, by about 13 minutes at the mean and 8 at the median, in each size band; a growth in gate time of about 6 minutes does not account for it.
- Tapestry took longer in the mean for larger runs, and is level per 100 changed lines; with 7 and 6 runs it supports no claim either way.
- Nothing here shows P1 shortening a run.
  A run holds time that `agent-turns` does not act on (review, Finalize, gates, Drift), and parallel load and recipe rows grew at the same time.
- The data cannot separate P1's effect from those changes: the periods differ in recipe, concurrency and size at once, and no run was repeated across the cut.
  A fair test is the same task twice on one version, which is an agent run and outside this report's scope.

## Reproduce

- `extract.sh` runs the lym reports from the prime and writes `runs.tsv` (one row per landed run) and `gate-by-slug.tsv` (via `join-gate.sh`); `producers.sh > producers.tsv` writes the recipe rows each run went through.
- `analyze.sh` prints every table above from `runs.tsv` and `producers.tsv`, by `analyze.jq`.
- The cost report holds a usage record for few runs, all After, so it gives no Before figure; `lym report cost` is not used for a comparison.
