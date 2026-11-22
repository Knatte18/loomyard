# Where darn runs lost time since 2026-11-18

A measurement report: it records one analysis on 2026-11-22 and is not kept current.

## Result

The slowdown does not reproduce in the archived rows.
Full darn runs (a Code-Burler-Review row present) took a median of 45.0 min on 2026-11-16 and 2026-11-17 and 45.3 min on 2026-11-21 and 2026-11-22.
The entry's 24 to 47 min figures could not be recovered from the archive in any cut tried.
The archive shows three real changes, each small and mostly offset by a shrinking one:

| Change after 2026-11-18 | Size per full darn run | Share of a 45 min run |
| --- | --- | --- |
| Row gaps: the records push at each transition | +56 s (75 s to 131 s median) | about 2 % |
| Code-Burler-Review row | +2.8 min median (5.0 to 7.8) | about 6 % |
| Verify tail after the tests: reach, redundancy and mutation reports | about +75 to +90 s per verify gate | about 4 to 7 % per run before offsets |
| Offset: tests per verify gate | Darn-Write verify tests 341 s to 298 s, Code-Burler-Fix 197 s to 187 s | about -2 % |
| Offset: writer row | -1.7 min median (17.8 to 16.1) | about -4 % |

Medians do not add; the means are 51.6 min before and 49.9 min after.

## Method

- Rows: the `history` of `_lym/shed/<slug>/status.json` in each `refs/tags/archive/<slug>/<sha>` tag of `loomyard-weft`.
  A row's time is `at` minus `started_at`; a gap is a row's `started_at` minus the previous row's `at`; skipped rows are left out.
- Gates: `_bolt/.lym/gate/runs/*.json`, one record per `lym gate test` tier run, with `wait_start`, `start`, `end`, tests discovered, and the gate key.
  Verify logs come from `_bolt/.lym/log-archive/<slug>/<stamp>/verify/`.
- Periods: archive tags dated 2026-11-16 and 2026-11-17 against 2026-11-21 and 2026-11-22.
- Sample: runs with a Code-Burler-Review row, no gap between rows over 30 min, no row over 2 h and no run over 4 h.
  That leaves 33 runs before and 16 after.
  Over all darn runs, 7-row research and docs runs included, the sample is 43 before and 21 after.
  Medians are 44.6 and 31.5 min, so the docs and research runs do not explain a rise either.
- Excluded after: the runs that sat paused for days, such as `finalize-mergeable-unknown`, `gate-dotnet-fixes` and `verify-flaky-triage`.
- Median full-darn run by archive day: 2026-11-11 34 min, 2026-11-15 50, 2026-11-16 53, 2026-11-17 45, 2026-11-18 45, 2026-11-19 60, 2026-11-21 44, 2026-11-22 45.
  The step up from about 34 to about 50 min lies between 2026-11-11 and 2026-11-15, before the period the entry names.

## Per row

Medians of the 33 and 16 runs.

| Row | Before | After |
| --- | --- | --- |
| Whole run | 45.0 min | 45.3 min |
| Sum of gaps between rows | 75 s | 131 s |
| Darn-Write | 17.8 min | 16.1 min |
| Code-Burler-Review | 5.0 min | 7.8 min |
| Code-Burler-Fix | 7.8 min | 8.2 min |
| Code-Bouncer (both) | 48 s | 44 s |
| Describe | 27 s | 27 s |
| Finalize | 3.9 min | 4.3 min |

The longest single gap per run rose from 22 s to 33 s median.
Code review wrote 3.5 lens reviews per run before and 4.0 after, in 1.24 and 1.12 rounds.

## Agents against gates

A row's agent time is the row minus the verify gate inside it.
In Darn-Write the verify gates took about 7.5 min before and 7.0 min after of a 17.8 and 16.1 min row, so the writer itself worked about 10 min in both.
The agent rows are not slower, except Code-Burler-Review.
Its row holds no gate, so the extra 2.8 min is agent time.
Code review began to receive mutation survivors (`6dbc7a058`, 2026-11-19) and a tautological-test lens (`6d3a916f4`, 2026-11-19); this report did not time which of them costs the minutes.

## Verify

Per run, the verify gate spans sum to 950 s before and 928 s after, in 2.8 and 2.5 gates.
The gate count did not rise.
What changed is where a gate's time goes.

| Gate | Tests, before | Tests, after | After the tests, before | After the tests, after |
| --- | --- | --- | --- | --- |
| Darn-Write verify | 341 s | 298 s | 111 s, trailing 22 s | 125 s, trailing 83 s |
| Code-Burler-Fix verify | 197 s | 187 s | 22 s, trailing 7 s | 114 s, trailing 111 s |

"Trailing" is the span from the last test run's end to the gate's end.
It holds the reach, redundancy and mutation reports (`49c494cb4` and `6dbc7a058`, 2026-11-19), and in Darn-Write the red-first gate (`d35c62607`, 2026-11-19) adds about 30 s of its own beside the verify span.
The tail grew about 75 to 105 s per gate.
Tests per gate fell because the diff scope discovered fewer tier 0 tests, 20 524 before and 15 697 after at the median, and ran 27 s against 36 s.

Tier wall times of `darn-write` and `code-burler-fix` gate runs, median seconds:

| Tier | Before | After |
| --- | --- | --- |
| Tier 0 | 36 to 40 | 27 to 36 |
| Tier 1 | 90 and 97 | 92 and 97 |
| Tier 2 | 53 and 52 | 52 and 52 |

Tier 1 discovers about 5 500 to 6 400 tests and tier 2 about 650 to 960 in both periods.
Tier 2 discovered 960 before and 650 after, with 280 and 135 passed, so the later scope runs fewer tier 2 tests that actually execute.
The entry's rise in full-verify time (9 to 12 min, then 11 to 16 min) is not in these records for the same runs.
The first verify span rose 7.6 to 8.7 min and later ones 3.5 to 4.5 min at the median, which is the tail above.

Cache: no `ok` line of any archived verify log in either period reads `(cached)` (31 969 lines before, 20 385 after).
Test results are never served from the test cache in a verify gate in either period, so the cache did not change.
This report did not measure the Go build cache apart from the tests.

## Load

- Gate slots are not contended.
  The mean wait for a slot on a `darn-write` tier 0 run was 10.4 s before and 1.5 s after, and the median wait is 0 s for every key and tier.
- Gate runs overlapped 1.3 to 1.6 on average while any ran, in both periods, with the three slots all busy for 0.1 to 1.4 h on 2026-11-16 and 2026-11-17 and 0.2 h on each of 2026-11-21 and 2026-11-22.
- The hardener live tests (`harden-live`, 21 runs on 2026-11-22, tier 1 only) ran about 80 s each and never made another key wait.
- The 2026-11-19 gate records showed a run end stamped far from its start, so that day is not in the overlap figures.

## Gate failures within a second

These are failures of a gate step, found in the verify logs.
Two kinds account for those logs:

- A test package that did not compile: the tier reports `0 discovered`, exit 1, in 0.2 s, and a cold fixer reworks it for 10 to 47 s.
  The rework is needed; the code does not build.
- Red-first refusals before 2026-11-21 evening: a diff that changes a non-Go file, such as a stencil or spec, made the gate refuse at once, and the writer's retries refused the same way.
  `gate-dotnet-fixes`, `raddle-slug-length-check`, `red-first-overlay-non-go-files` and `verify-flaky-triage` each logged three to four such refusals.
  The rework after one cannot help, since the diff stays as it is; `d35c62607`'s follow-up `d35d65565` (2026-11-21) restores embedded non-Go files instead.
  Counts of verify gates under 5 s per run rose from 0.8 to 1.4, and the rework time per run from 28 s to 38 s at the median: under a minute per run.

## Timed run

One run, on a machine that was not idle (load average 1.5 from two other runs).
`lym gate test --tier 0 --tier 1 ./internal/runfsm ./internal/gitrepo ./internal/summaryparser`: tier 0 247 tests in 0.4 s, tier 1 264 tests in 6.8 s, 8.2 s with the call.
The second, idle run was not made: those tiers ran from a warm build cache, and the 3 000 gate records above already span quiet and busy hours.

## Causes and fixes

None of these is the whole of a doubling, because the archive shows no doubling.
The shares are of a 45 min run.

1. **Verify tail after the tests**, 4 to 7 % before offsets: the reach, redundancy and mutation reports and the red-first gate run after the tests, one to two minutes per gate.
   Candidate note: run the three reports of a verify gate concurrently with its tier 2 tests, or only on the last verify gate before a handoff.
2. **Code-Burler-Review**, 6 %: 2.8 min more, with 0.5 more lens reviews per run.
   Candidate note: measure which input of the reviewer, mutation survivors or the tautological-test lens, costs the minutes, and cap or skip it on a diff with no survivor.
3. **Records push at each transition**, 2 %: 56 s of gaps and a 33 s longest gap.
   `later-transition-push-wait` already covers the push after the start rows; its fix removes about half of this.
4. **Red-first refusals on non-Go diffs**, under 1 % and gone since `d35d65565`: no further action.
5. **Not a cause**: gate slots, the other runs' load, hardener tests, tier wall times and the number of full verify runs.
   The test cache never hits in either period; a candidate note could ask whether a verify gate should pass `-count=1` knowingly, but it changes no time.

The entry's figure of 24 min before was not found.
A re-run of this measure on runs from 2026-11-23 on, with the verify-tail and review figures split out per row in the run record, would settle whether a later change did slow darn.
