# Do the reach and mutation reports change what lands? 2026-11-21

Since `6dbc7a05` every darn gate writes `reach.json` and `mutation.json`.
A reach turn can send the writer back for unreached functions,
failure reports carry triage lines,
and the code review rubric gets `reach_report` and `mutation_report` markers.
This report reads the 10 most recent darn runs that landed after `6dbc7a05` and counts what each of those four things changed.
It records one reading of the records on 2026-11-21 and is not rewritten when the code changes.
No test and no agent ran for it.

## Verdict

Keep the reach turn and the mutation step, tune both, and turn off the triage verdict line.

- **Reach turn: keep, tune.**
  7 turns in 4 runs led to tests in 3 turns, in 2 runs.
  Of the 4 turns that changed nothing, 3 named functions that tier-1 tests call but no coverage map holds;
  in the fourth the writer judged the functions too costly to test.
  The turn costs 5 to 60 seconds of agent time; when it adds tests, a gate pass of 3 to 12 minutes follows.
  The reach report also gave Code-Review 3 findings in 2 runs, all fixed,
  one of them on a function the writer had declined to test after its turn.
  Tune: leave out of the turn a function whose package has a tier-1 test the maps do not hold,
  or map tier-1 tests, since that gap caused most of the turns that changed nothing.
- **Mutation step: keep, tune.**
  Code-Review saw 27 survivors in 6 runs, raised 6 findings from them in 3 runs, and a fixer fixed all 6.
  The step added 41 seconds of gate wall time per run on average, 17 at the median.
  Of the 20 survivors no finding came from, 7 were killed by a tier-1 test the mutation run does not run,
  8 sat on lines a removal restored verbatim, and 5 no test could reasonably kill.
  Tune: run each mutant against the package's tier-1 tests too, or mark a survivor whose line a tier-1 test covers,
  so the reviewer stops re-deriving that by hand.
- **Triage lines: turn off "reaches none of this run's changes".**
  No fixer used a triage line to call a failure unrelated.
  All 6 failed tests that drew "reaches none of this run's changes" had failed because of the run's own change,
  through a text scan of the tree, an embedded markdown file or a new nil seam: changes that are not functions in a map.
  A fixer who believed the line would have dismissed a real failure every time.
  The other line, "not in the coverage maps", stood beside the two failures the change did not cause, and decided neither.

## Method

The runs are the newest 10 archive tags of the records repository whose seed names the `darn` recipe, landed after `mutation-report-step` (`runs.tsv`).
`pocock-skills-survey` and older darn runs landed earlier and fall outside the 10; `mutation-report-step` itself is left out because its own gate predates its step.
Each run was read once from four sources:

- the records at its archive tag: shed status with gate spans, code review rounds, fixer reports;
- the hub's per-run log archive: each verify log, with its reach note, failure report and mutation summary line, and each gate call's trace;
- the Claude Code session transcripts of the run: the reach turn's findings, what the session did next, and the cold fixer replies;
- the bouncer and review files, for findings that cite a survivor or the reach report.

The scripts beside this file print each figure: `verify.sh` the reports, `timing.sh` the step times, `turns.sh` and `after.sh` the reach turns and what followed, `roles.sh` the sessions.

Limits:

- The step times come from the traces of the row gate calls, the calls the shed waits on.
  A session's own `lym gate test --gate` call detaches to a front run whose trace the archive does not hold, so those calls' report steps are not counted.
- A call's time after its tiers holds the map refresh, the reach analysis, the redundancy step and the mutation step.
  The redundancy step predates `6dbc7a05`, so its span is subtracted; the reach analysis has no log line of its own and is not split from the refresh.
- Two calls, in `raddle-slug-length-check` and `harden-reap-lym-link-cwd`, span a 41-hour pause of the hub; their tier time is left out, their after-tier time is kept.
- Spend is in tokens of the session stretch a turn opened, read from the transcripts; no price is applied.

## Per run

### What each step changed

| Run | Reach turns | Tests added after a turn | Reach findings in review, fixed | Survivors at review | Survivor findings, fixed | Failure reports with triage | Triage used to call a failure unrelated |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `verify-flaky-triage` | 0 | - | 0 | 0 | 0 | 0 | - |
| `harden-reap-lym-link-cwd` | 0 | - | 0 | 0 | 0 | 2 | 0 |
| `finalize-mergeable-unknown` | 0 | - | 0 | 0 | 0 | 0 | - |
| `raddle-slug-length-check` | 0 | - | 0 | 0 | 0 | 1 | 0 |
| `single-repo-code-records-removal` | 2 (writer, fixer) | 0 | 0 | 10 | 0 | 0 | - |
| `bug-red-first-gate` | 2 (writer, fixer) | 2 | 1, 1 | 6 | 2, 2 | 2 | 0 |
| `shellguard-sh-c-rm-prompt` | 0 | - | 0 | 0 (0 mutants run) | 0 | 0 | - |
| `hardenshed-test-stray-lock` | 2 (writer, fixer) | 1 | 0 | 4 | 1, 1 | 0 | - |
| `merge-danger-tautology-glossary` | 0 | - | 0 | 5 | 3, 3 | 1 | 0 |
| `agent-questions-to-parent` | 1 (writer) | 0 | 2, 2 | 2 | 0 | 0 | - |
| **All** | **7 in 4 runs** | **3 turns** | **3, 3** | **27** | **6, 6** | **6** | **0** |

"Writer" is the Darn-Write session, "fixer" the Code-Burler-Fix session; both get the turn through their row's verify gate.
"Survivors at review" counts the survivors of the mutation report that matched HEAD when Code-Review ran.

### Time the steps added

Seconds of gate wall time, summed over the run's row gate calls of keys that list the reports (Darn-Write and Code-Burler-Fix; Finalize runs none).

| Run | Row gate calls | Tier time | After the tiers | Redundancy step | Reach and mutation | of which mutation | Mutants at review |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `verify-flaky-triage` | 3 | 1104 | 129 | 10 | 119 | 48 | 29 |
| `harden-reap-lym-link-cwd` | 3 | 213 | 23 | 0 | 23 | 3 | 3 |
| `finalize-mergeable-unknown` | 2 | 551 | 99 | 3 | 96 | 14 | 11 |
| `raddle-slug-length-check` | 2 | - | 243 | 12 | 231 | 33 | 7 |
| `single-repo-code-records-removal` | 2 | 183 | 430 | 37 | 393 | 248 | 40 |
| `bug-red-first-gate` | 5 | 732 | 199 | 24 | 175 | 22 | 38 |
| `shellguard-sh-c-rm-prompt` | 2 | 268 | 14 | 0 | 14 | 1 | 0 |
| `hardenshed-test-stray-lock` | 3 | 307 | 61 | 3 | 58 | 18 | 8 |
| `merge-danger-tautology-glossary` | 2 | 221 | 61 | 10 | 51 | 15 | 20 |
| `agent-questions-to-parent` | 2 | 248 | 69 | 6 | 63 | 8 | 12 |
| **All** | **26** | | **1328** | **105** | **1223** | **410** | |

Reach and mutation add 122 seconds per run on average and 80 at the median.
The mutation step's share is 41 seconds on average and 17 at the median.
`single-repo-code-records-removal` is the outlier: a removal diff of 124 changed functions, whose mutation step ran 569 test runs in 188 seconds in one call.
A call whose tiers were all carried, as on the first verify after a cold fixer, still runs every report step; in this sample that cost 8 to 224 seconds.

### Reach turns

| Run | Session | Unreached functions named | Turn seconds | Requests | Output tokens | What followed |
| --- | --- | --- | --- | --- | --- | --- |
| `single-repo-code-records-removal` | writer | 13 | 18 | 4 | 1527 | No change: the 13 lost only their one-repository branches, and integration tests call six of the seven named |
| `single-repo-code-records-removal` | fixer | as above | 5 | 3 | 639 | No change: the fixer read the note as listing reached functions only; a note in the fixer report |
| `bug-red-first-gate` | writer | 4 | 35 | 6 | 3328 | Test for `changedTestsAt`; the other three left |
| `bug-red-first-gate` | fixer | 3 | 18 | 4 | 1409 | Test for `noSeamWayForward`; `runRedFirst` left as covered by tier-1 tests, `runMutate` and `verbDiffBase` as not this round's code |
| `hardenshed-test-stray-lock` | writer | 2 | 25 | 5 | 1958 | No change: the tier-1 `TestRunTierStrayFiles` exercises both |
| `hardenshed-test-stray-lock` | fixer | 3 | 60 | 9 | 7686 | Tier-0 tests for all three, each already exercised by a tier-1 test |
| `agent-questions-to-parent` | writer | 3 | 7 | 2 | 835 | No change: tests would need a full hub; Code-Review later raised `newShed` as F2 from the reach report, and it was fixed |

Two of the three test-adding turns closed a real gap, and one of them closed it weakly.
Code-Review found that the `changedTestsAt` test the writer added survived a negated condition (F10), and the fixer strengthened it.
The third, in `hardenshed-test-stray-lock`, added tier-0 tests for functions a tier-1 test already exercised, so the map would see them.

### Mutation survivors at review

| Run | Survivors | Raised as a finding | Not raised, and why |
| --- | --- | --- | --- |
| `single-repo-code-records-removal` | 10 | 0 | 2 killed by a tier-1 test the mutation run skipped; 8 on lines restored verbatim from before `f06acf39` |
| `bug-red-first-gate` | 6 | 2 (F10 `redfirst.go:287`; F11's `ToLower` row) | 2 killed by tier-1 `TestGateRedFirst`; 1 equivalent; 1 a file-mode constant |
| `hardenshed-test-stray-lock` | 4 | 1 (F4, the refusal half of `tierrun.go:83`) | 3 killed by tier-1 `TestRunTierStrayFiles` |
| `merge-danger-tautology-glossary` | 5 | 3 (F4 for `description.go:127` and `:131`, F5, F6) | 1 newline constant normalized again downstream |
| `agent-questions-to-parent` | 2 | 0 | 1 a log line only; 1 an increment any positive constant satisfies |
| **All** | **27** | **6 findings from 7 survivors** | **7 tier-1, 8 restored lines, 5 unkillable** |

Every survivor finding was LOW and fixed by one table row or assertion.
Code-Review also cited a report's uncovered changed block in two runs:
`finalize-mergeable-unknown` as support for F2, an untested error path, and `raddle-slug-length-check` in a lens finding that the round's synthesis dropped as a NIT.

### Triage lines

| Run | Failure report | Triage lines | Cause of the failure | Fixer's call |
| --- | --- | --- | --- | --- |
| `harden-reap-lym-link-cwd` | 1st Darn-Write | "reaches none" for `TestEnforcement_FabricVocabulary`; "not in the coverage maps" for `TestDobby_ReexecsOntoAReplacedBinary` | The change: a variable named `weft` in a test; the second test flaky | Fixed the first, left the second as flaky after a passing re-run |
| `harden-reap-lym-link-cwd` | 2nd Darn-Write | "not in the coverage maps" for every failed test | A missing prebuilt `lym` binary | Environment fault, from the log; no change |
| `raddle-slug-length-check` | Darn-Write | "reaches none" for `TestEnforcement_FabricVocabulary` | The change: a test importing `weftname` | Fixed |
| `bug-red-first-gate` | 1st Darn-Write | "reaches none" for `TestWire_EverySeamFilled`; "not in the coverage maps" for others | The change: two new seams left nil | Fixed |
| `bug-red-first-gate` | 2nd Darn-Write | "not in the coverage maps" for `TestGateMutate` | Load: it passed alone | Ran it alone; cited the line beside the diff not touching `reach.go`; no change |
| `merge-danger-tautology-glossary` | Darn-Write | "reaches none" for `TestRender`, `TestStore`, `TestEnforcement_FabricVocabulary` | The change: a new glossary passage and its bare `weft` token | Fixed all three |

Every "reaches none" line used a map from commit `5dc7a587` and judged against the diff's changed Go functions.
The failures it misjudged came through files and data the maps do not record as functions.

## What to change

1. **Triage.**
   Drop the "reaches none of this run's changes" line and its `lym gate reach` follow-up, or print it only when the diff changes Go functions alone.
   Six of six such lines were wrong in this sample, and the failures they described were the run's own.
2. **Reach turn.**
   Skip a function in the turn when a tier-1 test of its package exists and no map holds tier-1 tests,
   or build maps for tier-1 tests.
   Four of seven turns ended with no change, and three of those four named such functions.
3. **Mutation step.**
   Count a survivor as killed, or mark it, when a tier-1 test of the package covers its line,
   since 7 of the 20 survivors that drew no finding fell to a tier-1 test.
   A survivor on a line the diff restores from an older commit is no gap of the diff, and could be marked so too.
