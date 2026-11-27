# Weaver against Webster on landed plans

2026-11-27.
The figures record runs on one machine and are not claiming to stay true.

## Result

Weaver implemented all three plans: every card committed, every plan-level verify passed on its first run, and no stop.
On the one plan both implementers ran today, Weaver took the same time as Webster and cost 18% less,
and its result drew half as many review findings.

**Recommendation:** Weaver can replace Webster as tapestry's implementer, but not as the default yet.
Switch it on per task first, and make these changes before it becomes the default:

1. Give a Weaver attempt a timeout of its own, sized to a batch.
   An attempt runs under `shuttle.run_timeout_min`, 30 minutes, which its gate waits and gate fixers count against.
   The first batch of the calibration plan hit it with three of four cards committed, and the retry climbed to Opus.
2. Name each batch's deviations and departures in Weaver's `summary.md`.
   Webster's summary lists them; Weaver records the deviations in `state.json` and its summary lists only attempt counts.
3. Record gate spans on the row.
   The Webster rows hold `gate_spans` for the plan-level verify; the Weaver rows hold none.
4. Watch the paths this bench did not reach: a lost strand, a resume, a failed plan-level verify and the third rung.

The evidence is thin on quality: one run per plan, one review per result, and one same-day comparison.

## Plans

Each plan was implemented by Webster and landed.
A run is clean when its Webster row has one start and one end, with no machine sleep, restarted Master or recovery strand inside it.

| Plan | Base commit | Landed commit | Cards, Webster batches | Recorded Webster run | Main packages |
|---|---|---|---|---|---|
| `board-viewer-server` | `6d3a916f4` | `7bdb031ed` | 9 in 4+2+3 | 2026-11-19 | `boardengine`, `boardserve`, `boardcli`, `vscode` |
| `launch-manifest-parity` | `40c66912c` | `3bbc92a0e` | 9 in 3+3+3 | 2026-11-17 | `shuttleparity`, `shuttleengine`, `shuttlecli`, `heddleengine` |
| `gate-test-map-mode` | `5469ff91d` | `b92c95ee5` | 5 in 1+2+2 | 2026-11-17 | `gatecli`, `testcover` |

Why these three:

- Each recorded run is clean.
- Their main packages differ.
- Each base commit is an ancestor of `main` and builds `cmd/lym` today, and each plan passes `lym webster validate`.
- `board-viewer-server` has the newest clean recorded run, so it is the calibration plan.

Newer runs left out:

- `dobby-run-watch-build`: the machine hibernated for over eight hours inside the Webster row.
- `async-run-pusher`: two Master sessions, and the row's history holds only the later start.
- `single-repo-namespace-layout`: the row stopped once, and one batch took two recoveries.
- `review-result-checks`: the plan-level verify failed once and the row did rework.
- `slack-machine-bridge-build`: one batch took a recovery.
- `adopt-foreground`: its base commit is no longer in the repository.

Weaver cut `gate-test-map-mode` into batches of 1+1+3 cards; the other partitions equal Webster's.

## How the runs were made

Each run had a worktree pair of its own: the base commit's tree committed on top of a branch,
the landed run's archived plan and decision record copied in, the run moved straight to the implementer row,
and a pause after that row.
Weaver ran its default ladder (`sonnet[medium]`, `opus[medium]`, `opus[high]`), Webster its default models.

| Run | Plan | Implementer | Result head | Started, CET |
|---|---|---|---|---|
| `b1` | `board-viewer-server` | Weaver | `18ad1c92f` | 12:58, with `b2` |
| `b2` | `board-viewer-server` | Webster | `68c4add1c` | 12:58, with `b1` |
| `b3` | `launch-manifest-parity` | Weaver | `44e4bc47a` | 14:30, with `b4` |
| `b4` | `gate-test-map-mode` | Weaver | `ebdb1317d` | 14:30, with `b3` |

No run was lost to the machine, and none was repeated.
In the tables, "recorded" is the landed Webster run of the same plan.

### Questions and stops

- `b1` and `b2` both asked how cards 1–3 could pass the test-only-code guard, and both got the same answer:
  temporary allow-list entries with a `Pattern-Departure` trailer.
- `b3` asked about a tree-sitter link its card 7 forced, and was told to add the allow-list entry the landed run had added.
- `b2` stopped once after its verify passed: the run-exit audit flagged the Master's own friction note.
  The arbiter cleared it and the resumed row ended 1m47s later.

### Limits

- The archived decision records are the landed runs' final ones.
  They hold the entries added while those runs ran, and name the findings their reviews left open.
  Every session of today's runs could read them; the recorded runs started without them.
- Each pair's history holds the landed solution.
  No tool result in any session of the four runs held the landed commit's subject or short hash.
- `lym` was rebuilt at 14:08 CET, from `80df63359` to `2326ef4da`, and the hub's gate config was edited at 14:08 and 14:23.
  Both calibration runs span these changes; `b3` and `b4` ran after them.
- Load differed.
  Sampled load averages on this 12-core machine were 4–45 during `b1` and `b2`, and 6–23 during `b3` and `b4`.
  Other runs of the hub worked at the same time.

## Wall time

| Plan | Recorded Webster | Webster today | Weaver |
|---|---|---|---|
| `board-viewer-server` | 63m00s | 89m28s | 91m57s |
| `launch-manifest-parity` | 52m29s | — | 57m56s |
| `gate-test-map-mode` | 53m36s | — | 50m52s |

Webster today is measured to its stop; to the end of the resumed row it is 91m14s.

## Calibration and adjusted figures

The factor is Webster today divided by the recorded Webster run of `board-viewer-server`.
The adjusted figure is the recorded Webster figure of the other two plans multiplied by it.

| Measure | Recorded | Webster today | Factor |
|---|---|---|---|
| Row wall time | 3780 s | 5368 s | 1.42 |
| Gate tier runs (card, batch, plan-level) | 1518 s | 3376 s | 2.22 |
| Gate fixer sessions, summed | 566 s | 1105 s | 1.95 |
| Dollars, without the stop | $11.75 | $13.41 | 1.14 |

| Plan | Measure | Recorded | Adjusted | Weaver | Weaver / recorded | Weaver / adjusted |
|---|---|---|---|---|---|---|
| `launch-manifest-parity` | Wall time | 52m29s | 74m32s | 57m56s | 1.10 | 0.78 |
| `launch-manifest-parity` | Gate tier runs | 976 s | 2171 s | 1624 s | 1.66 | 0.75 |
| `launch-manifest-parity` | Gate fixers | 600 s | 1171 s | 769 s | 1.28 | 0.66 |
| `launch-manifest-parity` | Dollars | $17.00 | $19.40 | $12.73 | 0.75 | 0.66 |
| `gate-test-map-mode` | Wall time | 53m36s | 76m07s | 50m52s | 0.95 | 0.67 |
| `gate-test-map-mode` | Gate tier runs | 965 s | 2146 s | 1018 s | 1.05 | 0.47 |
| `gate-test-map-mode` | Gate fixers | 336 s | 656 s | 378 s | 1.13 | 0.58 |
| `gate-test-map-mode` | Dollars | $14.91 | $17.02 | $8.60 | 0.58 | 0.51 |

The adjusted column favours Weaver on time: `b3` and `b4` ran under less load than the pair that set the factor.
Read the time ratios of these two plans as lying between the two ratio columns.

## Time in batches, gates and fixers

Batch time runs from a batch's start to the next batch's start, so it holds the batch's gates and fixers.
The last batch ends where the plan-level verify starts, or, for Weaver, where its last session ends.
"Verify and close" is the rest of the row.

| Plan | Run | Batch 1 | Batch 2 | Batch 3 | Verify and close | Row |
|---|---|---|---|---|---|---|
| `board-viewer-server` | Recorded Webster | 25m49s | 13m48s | 16m18s | 7m05s | 63m00s |
| `board-viewer-server` | Webster today | 39m58s | 24m04s | 21m20s | 4m05s | 89m28s |
| `board-viewer-server` | Weaver | 39m44s | 25m34s | 22m16s | 4m23s | 91m57s |
| `launch-manifest-parity` | Recorded Webster | 16m49s | 20m58s | 11m42s | 3m00s | 52m29s |
| `launch-manifest-parity` | Weaver | 22m36s | 20m25s | 11m40s | 3m15s | 57m56s |
| `gate-test-map-mode` | Recorded Webster | 24m28s | 15m59s | 10m16s | 2m53s | 53m36s |
| `gate-test-map-mode` | Weaver | 21m28s | 3m18s | 22m53s | 3m13s | 50m52s |

Gate times are the summed wall time of the tier runs started inside the row, in seconds, from the gate run records.
"Direct" is tier runs a session started outside a keyed gate; it is not in the sum.

| Plan | Run | Card | Batch | Plan-level | Sum | Direct | Fixer sessions | Fixer time |
|---|---|---|---|---|---|---|---|---|
| `board-viewer-server` | Recorded Webster | 878 | 233 | 407 | 1518 | 0 | 8 | 566 s |
| `board-viewer-server` | Webster today | 2352 | 782 | 242 | 3376 | 26 | 9 | 1105 s |
| `board-viewer-server` | Weaver | 2448 | 743 | 219 | 3410 | 37 | 9 | 1051 s |
| `launch-manifest-parity` | Recorded Webster | 668 | 140 | 168 | 976 | 9 | 8 | 600 s |
| `launch-manifest-parity` | Weaver | 1199 | 268 | 157 | 1624 | 23 | 9 | 769 s |
| `gate-test-map-mode` | Recorded Webster | 364 | 432 | 169 | 965 | 253 | 3 | 336 s |
| `gate-test-map-mode` | Weaver | 465 | 393 | 160 | 1018 | 398 | 3 | 378 s |

## Cost

Counted from the session transcripts, subagents included, one count per message, at the prices in `claudeengine/pricing.go`.
The usage records alone leave out the gate fixers.
The `b2` figures include its stop: the arbiter (the only Opus there, $0.32) and the resumed Master ($0.19).

| Plan | Run | Model | Input | Cache write | Cache read | Output | Dollars |
|---|---|---|---|---|---|---|---|
| `board-viewer-server` | Recorded Webster | Sonnet | 476 | 1,227,640 | 29,227,456 | 283,224 | $11.75 |
| `board-viewer-server` | Webster today | Sonnet | 500 | 1,586,257 | 29,191,469 | 245,928 | $13.60 |
| `board-viewer-server` | Webster today | Opus | 16 | 37,494 | 224,262 | 4,208 | $0.32 |
| `board-viewer-server` | Weaver | Sonnet | 400 | 1,395,439 | 22,679,047 | 240,949 | $10.43 |
| `board-viewer-server` | Weaver | Opus | 34 | 56,900 | 756,831 | 4,049 | $0.52 |
| `launch-manifest-parity` | Recorded Webster | Sonnet | 566 | 1,557,085 | 50,832,097 | 294,206 | $17.00 |
| `launch-manifest-parity` | Weaver | Sonnet | 442 | 1,684,732 | 30,627,716 | 239,617 | $12.73 |
| `gate-test-map-mode` | Recorded Webster | Sonnet | 460 | 1,212,552 | 44,685,448 | 293,885 | $14.91 |
| `gate-test-map-mode` | Weaver | Sonnet | 252 | 1,054,821 | 17,789,104 | 240,876 | $8.60 |

| Plan | Run | Total | Of which gate fixers |
|---|---|---|---|
| `board-viewer-server` | Recorded Webster | $11.75 | $1.65 |
| `board-viewer-server` | Webster today | $13.92 | $2.43 |
| `board-viewer-server` | Weaver | $10.95 | $2.37 |
| `launch-manifest-parity` | Recorded Webster | $17.00 | $1.62 |
| `launch-manifest-parity` | Weaver | $12.73 | $2.61 |
| `gate-test-map-mode` | Recorded Webster | $14.91 | $0.90 |
| `gate-test-map-mode` | Weaver | $8.60 | $1.24 |

Most of Weaver's saving is cache reads.
A likely cause, not measured here, is that each batch starts cold, where a Webster fork carries the Master's context.

## Ladder and stops

| Run | Attempts | Climbed past the first rung | Stops reaching the stop chain | Questions |
|---|---|---|---|---|
| Weaver, `board-viewer-server` | 4 | 1 (batch 1, timeout, finished on `opus[medium]`) | 0 | 1 |
| Weaver, `launch-manifest-parity` | 3 | 0 | 0 | 1 |
| Weaver, `gate-test-map-mode` | 3 | 0 | 0 | 0 |
| Webster today, `board-viewer-server` | — | — | 1 (run-exit audit) | 1 |
| Recorded Webster, all three | — | — | 0 | not measured |

## Quality

### Verify, cards and departures

"Files outside targets" is the scope guard's deviation count, summed over the batches.
Departures are `Pattern-Departure` trailers; for a recorded run they are what its summary reports.

| Plan | Run | Plan-level verify | Verify fixer rounds | Cards committed | Files outside targets | Departures |
|---|---|---|---|---|---|---|
| `board-viewer-server` | Recorded Webster | passed | 0 | 9/9 | 16 | 0 |
| `board-viewer-server` | Webster today | passed | 0 | 9/9 | 16 | 4 |
| `board-viewer-server` | Weaver | passed | 0 | 9/9 | 19 | 3 |
| `launch-manifest-parity` | Recorded Webster | passed | 0 | 9/9 | 17 | 0 |
| `launch-manifest-parity` | Weaver | passed | 0 | 9/9 | 21 | 3 |
| `gate-test-map-mode` | Recorded Webster | passed | 0 | 5/5 | 8 | 0 |
| `gate-test-map-mode` | Weaver | passed | 0 | 5/5 | 9 | 0 |

### Diff against the landed commit

The landed commit holds the recorded run's review fixes, which the results do not, and its base can be newer than the plan's base.
The recorded runs' implementer heads are no longer in the repository.
"Go added" is production / test lines.

| Plan | Diff | Files | Lines added / removed | Go added | Files shared with landed |
|---|---|---|---|---|---|
| `board-viewer-server` | Landed | 63 | 6048 / 87 | 2221 / 2867 | — |
| `board-viewer-server` | Webster today | 61 | 4768 / 124 | 1948 / 2169 | 55 |
| `board-viewer-server` | Weaver | 60 | 4931 / 144 | 2015 / 2144 | 58 |
| `launch-manifest-parity` | Landed | 63 | 4931 / 118 | 2118 / 2739 | — |
| `launch-manifest-parity` | Weaver | 61 | 3985 / 118 | 1859 / 2052 | 54 |
| `gate-test-map-mode` | Landed | 40 | 1406 / 3862 | 598 / 766 | — |
| `gate-test-map-mode` | Weaver | 40 | 1263 / 3881 | 572 / 641 | 40 |

### Review findings

One Burler round per result: a `sonnet[medium]` reviewer with the code lens fan, over the commits above the base,
with the archived decision record and plan as the answer key.
The recorded figures are round 1 of each landed run's Code-Review.

| Plan | Run | Verdict | Blocking | Medium | Low | Nit | Total |
|---|---|---|---|---|---|---|---|
| `board-viewer-server` | Recorded Webster | blocking | 1 | 3 | 8 | 0 | 12 |
| `board-viewer-server` | Webster today | blocking | 3 | 11 | 6 | 0 | 20 |
| `board-viewer-server` | Weaver | blocking | 1 | 4 | 4 | 1 | 10 |
| `launch-manifest-parity` | Recorded Webster | blocking | 2 | 6 | 4 | 1 | 13 |
| `launch-manifest-parity` | Weaver | blocking | 3 | 7 | 6 | 0 | 16 |
| `gate-test-map-mode` | Recorded Webster | approved | 0 | 0 | 2 | 1 | 3 |
| `gate-test-map-mode` | Weaver | approved | 0 | 4 | 9 | 0 | 13 |

Only the two `board-viewer-server` runs of today compare like with like.
The recorded rounds were reviewed by older prompts on other days,
and the same plan drew 12 findings recorded and 20 today from Webster.
The higher Weaver counts on the other two plans are therefore not evidence either way.

## Findings per run

The full reviews are in `reviews/`.

### Weaver, `board-viewer-server` ([review](reviews/weaver-board-viewer-server.md))

- Blocking: the opener's exit is waited on and not logged.
- Medium: a failed server start loses its error; an action moves a dirty edit form onto a new revision,
  so Save overwrites another writer; server behaviours and verb wiring lack tests.
- One of the four findings the decision record names as open recurred: untested verb wiring.
- Beyond its cards, it exported `boardengine.EntryStoreFile` and updated two tests outside its targets.

### Webster today, `board-viewer-server` ([review](reviews/webster-board-viewer-server.md))

- Blocking: editing a task's dependencies silently deletes the met ones; the watch loop has no one-second floor;
  the opener's exit is not logged.
  The Weaver result and the landed commit leave the dependency field blank in the edit form and do not have the first.
- Medium: the dirty-form overwrite; a kind change discards the unsent edit; the server's create makes projects;
  a lost server error; `board.html` not pinned to LF; missing tests.
- Three of the four named findings recurred: the watch loop floor, the LF pin and the verb wiring.

### Weaver, `launch-manifest-parity` ([review](reviews/weaver-launch-manifest-parity.md))

- Blocking: the seat catalog never builds the unassisted burler reviewer, which an acceptance criterion names;
  `CLAUDE.local.md` and the engine names are repeated literals.
- Medium: the verb refuses the burler roles real runs carry; an empty `CLAUDE.md` reads as a Codex gap;
  five behaviours are unpinned by tests.
- The two literal findings and the untested production passage reader
  are ones the decision record names from the recorded run's review.

### Weaver, `gate-test-map-mode` ([review](reviews/weaver-gate-test-map-mode.md))

- Approved.
- Medium: `docs/overview.md` still describes the deleted planning; two acceptance behaviours are unpinned;
  two removed tests have no `Loosened-Check` trailer.
- Low: a leftover of the old planning, an unbounded scope computation, stale texts and missing test rows.
