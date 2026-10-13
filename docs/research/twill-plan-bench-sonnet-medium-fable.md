# twill-plan bench, sonnet[medium] twill and single Fable

2026-10-12, prime `/home/knatte/Code/loomyard-LYXHUB/loomyard`, board note `twill-plan-sonnet-medium`.
Two arms, two runs per reference each, set beside the arms already run (`twill-plan-bench.md` rounds 1-3, `twill-plan-bench-sonnet.md`):

1. Twill with the plan writer on `sonnet[medium]`, forks inheriting it; cap about $20.
2. A single Fable 5.1 plan writer, `fable[high]`, with no directive and no forks; cap about $30.

## Setup

- Exactly round 2's setup and mechanics as `twill-plan-bench-sonnet.md` "Setup" gives them: one `twb-*` note per pair, `lyx fabric add` in the prime, code branch reset to the base, frozen records checked out with `git checkout <weft sha> -- _lyx/discussion _lyx/reviews/discussion`, `lyx fabric commit`, `lyx shed seed self --recipe loom --driver go --param parent=main`, two `lyx loom step` calls, `status.json` written onto Plan-Write with Discussion-Write and Discussion-Bouncer done, `lyx fabric commit`, `lyx loom pause --after Plan-Write`, `lyx loom step --until-stop`.
- Reference 1 (`shuttle-startup-ready`): base `a35c8c2b1`, records `995a85ba`, pairs `twb-sm-s-{1,2}` and `twb-fab-s-{1,2}`.
  Reference 2 (`run-time-by-role`): base `c00b2e524`, records `8f5c017a`, pairs `twb-sm-r-{1,2}` and `twb-fab-r-{1,2}`.
  Every pair's frozen paths matched the records commit (`git diff --stat` empty).
- Pair config was set with `lyx config <module> --set` in the pair, which edits and syncs the pair's `_lyx/config`.
- Sonnet arm: pair `loom.yaml` `plan: sonnet[medium]`, `shuttle.yaml` `claude:` the twill wrapper and `claude_deny_agent_tool: false`.
  The directive is byte-identical to `twill-plan-bench/round-2-directive.md` and the wrapper is round 2's with its paths under the scratchpad's `rsm/` (copies in `round-sm-fable/`); each wrapper log shows Step 2b spliced before Step 3.
  Writers ran `--model sonnet --effort medium`; every writer and fork message is `claude-sonnet-5-5`.
- Fable arm: pair `loom.yaml` `plan: fable[high]`, `shuttle.yaml` `claude:` round 2's single-arm wrapper (`round-sm-fable/wrap-plain.sh`, the code-base notice only), `claude_deny_agent_tool` left `true`.
  The model key `fable` is the built-in alias in `internal/modelspec/registry.go`; the hub's `models.yaml` gives it the default effort `high`, so high was used.
  Writers ran `--model fable --effort high`; every message is `claude-fable-5-1`.
- Wave 1 (`*-s-1`, `*-r-1` of both arms) ran from 08:23 local; wave 2 from 08:35, once wave 1's $5.66 (sonnet) and $11.40 (Fable) showed both caps would hold.
- Prod `lyx` was `414d8b713` throughout; `internal/planparser`, `loom-template-plan.md`, `loom-rubric-plan-review.md` and `loom-plan-spec.md` are unchanged since `3a627c00d`, the sonnet[high] arm's wave-2 binary.

## Cost

Cost and writer turn time from `go run ./tools/tokencount` (report in `twill-plan-bench/tokens-sonnet-medium-fable.md`), orch excluded.

| Ref | Arm | Run | Writer | Forks | Total | Writer time | Messages | Cards |
|---|---|---|---|---|---|---|---|---|
| 1 | twill `sonnet[medium]` | sm-s-1 | $2.15 | $0.87 | $3.02 | 6m58s | 48 | 6 |
| 1 | twill `sonnet[medium]` | sm-s-2 | $1.92 | $0.73 | $2.65 | 6m14s | 42 | 6 |
| 2 | twill `sonnet[medium]` | sm-r-1 | $2.04 | $0.60 | $2.64 | 6m34s | 46 | 7 |
| 2 | twill `sonnet[medium]` | sm-r-2 | $2.80 | $0.87 | $3.67 | 9m52s | 52 | 7 |
| 1 | single `fable[high]` | fab-s-1 | $5.46 | | $5.46 | 9m14s | 20 | 5 |
| 1 | single `fable[high]` | fab-s-2 | $5.27 | | $5.27 | 10m23s | 18 | 5 |
| 2 | single `fable[high]` | fab-r-1 | $5.94 | | $5.94 | 10m14s | 21 | 7 |
| 2 | single `fable[high]` | fab-r-2 | $5.99 | | $5.99 | 10m39s | 19 | 7 |

- Sonnet arm: $11.98 of the $20 cap ($8.91 writers, $3.07 forks), $2.99 per plan.
- Fable arm: $22.66 of the $30 cap, $5.67 per plan.
- tokencount attributes $3.11 of orch to the eight runs, excluded above.
- `sonnet[medium]` cut the token volume against `sonnet[high]`: writer output 47k-78k against 65k-107k, writer cache reads 5.0M-7.5M against 8.5M-11.3M, forks $0.60-$0.87 against $1.09-$1.30.
- Fable used about single `opus[high]`'s output (42k-51k against 35k-49k) and cache writes (212k-228k against 153k-171k), but less than half its cache reads (2.0M-2.7M against 3.9M-5.5M) over 18-21 messages against 38-51.
  At 2.5x Opus's output and cache-write price, that lands at 2.2x single `opus[high]`'s cost per plan, inside the note's $5.50-6.50 estimate, with about half of it cache writes.
  Its turns are slow: writer time 9m14s-10m39s against 5m56s-8m08s for single `opus[high]`.

## Grading

Same keys and method as round 2 (`twill-plan-bench.md`): R repeated, P partly, A avoided, n/a another route.

### Reference 1 (`shuttle-startup-ready`)

| Key | sm-s-1 | sm-s-2 | fab-s-1 | fab-s-2 |
|---|---|---|---|---|
| K1 no-tail rows | R (skill-load death row on `skillReed`, which the card claims has a ready-screen capture but never gives one; retry-exit row rejected, see Rejections) | P (skill-load row gets a queued non-blank capture; retry-exit row rejected) | R (skill-load and status-cap absence asserts on empty captures) | R (same) |
| K2 new top-level test | R (new `TestStartup_DismissalGap` beside `TestStartup_UndismissableGateUntilWindowExpires`) | R (new `TestStartup_GateDismissalGap`, never-clears row dropped in favor of the old test) | R (new `TestStartup_GateDismissalGap` with a never-clears row beside the old test) | R (new `TestStartup_DismissalGap`, same) |
| K3 capture until first send | A (constant ready capture with a lone `%`) | n/a | n/a | n/a |
| K4 middle state | A | n/a | n/a | n/a |
| K5 stale keep reason | A (rewritten) | A (reworded) | P (doc comment rewritten, keep reason not named) | R (row replaced, doc comment and keep reason not mentioned) |
| K6 test package vs glyph | A (file-only `Create`, package `claudeengine_test`) | n/a | n/a | n/a |
| N1 start-level criterion | P (start test drives the color and prompt sends; the skill-load send is argued, not tested) | R (rejected with citations: the box rows stand for it) | R (no start-level test) | R (no start-level test) |
| **R / P** | **2 / 1** | **2 / 1** | **3 / 1** | **4 / 0** |

### Reference 2 (`run-time-by-role`)

| Key | sm-r-1 | sm-r-2 | fab-r-1 | fab-r-2 |
|---|---|---|---|---|
| J1 repeated seed rows | P (`DecodeTreeSeed` subtests in `TestSeed_LegacyParentKey` repeat `parent` and unknown key) | P (`DecodeSeed` rows repeat `parent` and absent driver; the unknown-key row was dropped on a test-economy finding) | P (`DecodeSeed` rows repeat `parent`, unknown key and absent driver) | P (`DecodeSeed` subtest repeats `parent` and unknown key) |
| J2 parse abort, seed-less status | P (bad `at`, bad JSON, no-seed rows; no bad `started_at` or `last_at` row) | A (bad `at`, bad `started_at`, no seed) | P (bad `at`, no seed; no `started_at` row) | P (bad `at`, no seed; no `started_at` row) |
| J3 "All runs" sum vs union | P (two runs summed, no overlap) | P (same) | P (same) | P (same) |
| J4 shedrun constructor claim | R (`doc.go` sentence added, claim stays, `paths.go` header unlisted) | R (same) | R (same) | R (same) |
| J5 run-id error wrapping | P (slug and tag only) | P (slug, tag and the refused id; the criteria-coverage finding asking for the file was rejected) | A (slug, tag and file) | A (slug, tag and file) |
| J6 cwd parent vs worktree parent | R | R | R | R |
| J7 missing-commit skip row | n/a (no parser skip) | A (malformed `refs` line aborts, with a row) | R (go-git `TagsUnder` skips a non-commit tag; no row reaches it) | A (`TestArchivedSlugs` asserts the skipped lines) |
| J8 `orch_test.go` unlisted | A (`TestCountOrch` edited) | A (same) | A (same) | R (card 5 says no test asserts the header; `TestCountOrch` unlisted) |
| J9 bare `weft` in tests | P (no name with it; the row asserts "the flag spelling with its dash") | R (`TestShedSourceFromWeft`) | R (`TestRunRefusesAnotherHubWithoutWeft`) | R (`TestRunRefusesAForeignHubWithoutWeft`, `TestRunReadsArchivedStatusFromTheWeft`) |
| **R / P** | **2 / 5** | **3 / 3** | **4 / 3** | **4 / 3** |

J6 survives every arm again.
No plan of either arm opened `TestEnforcement_FabricVocabulary`; three of four plans name a test with `Weft`.

## Arm comparison

R / P are means over two runs per reference; cost per plan and writer time are means over the same runs.

| Ref | Arm | Runs (R / P) | Mean R / P | Cost per plan | Writer time |
|---|---|---|---|---|---|
| 1 | single `opus[high]` (r1 oh-1, r2 p2s-oh-2) | 1 / 1, 1 / 0 | 1.0 / 0.5 | $2.58 | 6m10s |
| 1 | twill `opus[medium]` (r2) | 0 / 1, 1 / 0 | 0.5 / 0.5 | $5.56 | 9m40s |
| 1 | twill `opus[high]` (r3) | 1 / 0, 0 / 0 | 0.5 / 0.0 | $6.06 | 11m8s |
| 1 | twill `sonnet[high]` | 1 / 1, 1 / 0 | 1.0 / 0.5 | $4.47 | 8m42s |
| 1 | twill `sonnet[medium]` | 2 / 1, 2 / 1 | 2.0 / 1.0 | $2.84 | 6m36s |
| 1 | single `fable[high]` | 3 / 1, 4 / 0 | 3.5 / 0.5 | $5.37 | 9m49s |
| 2 | single `opus[high]` (r2) | 6 / 1, 3 / 4 | 4.5 / 2.5 | $2.68 | 7m28s |
| 2 | twill `opus[medium]` (r2) | 2 / 3, 2 / 4 | 2.0 / 3.5 | $4.93 | 10m41s |
| 2 | twill `opus[high]` (r3) | 1 / 4, 1 / 3 | 1.0 / 3.5 | $5.67 | 13m50s |
| 2 | twill `sonnet[high]` | 4 / 2, 1 / 2 | 2.5 / 2.0 | $5.08 | 12m12s |
| 2 | twill `sonnet[medium]` | 2 / 5, 3 / 3 | 2.5 / 4.0 | $3.16 | 8m13s |
| 2 | single `fable[high]` | 4 / 3, 4 / 3 | 4.0 / 3.0 | $5.97 | 10m27s |

Over both references: twill `sonnet[medium]` $2.99 and 7m25s per plan, single `fable[high]` $5.67 and 10m8s, against twill `opus[medium]` $5.24 and 10m10s and single `opus[high]` $2.63 and 6m49s.

## Lens finding counts (sonnet arm)

Severity words in each fork's final message (B BLOCKING, M MEDIUM, total), counted as in round 2.

| Lens | sm-s-1 | sm-s-2 | sm-r-1 | sm-r-2 |
|---|---|---|---|---|
| criteria-coverage | 1 / 3 / 8 | 0 / 2 / 5 | 0 / 2 / 6 | 0 / 2 / 8 |
| row-reachability | 0 / 2 / 8 | 1 / 0 / 3 | 0 / 6 / 12 | 0 / 1 / 6 |
| forced-edits | 1 / 0 / 3 | 1 / 1 / 3 | 0 / 0 / 6 | 1 / 0 / 5 |
| test-economy | 0 / 0 / 5 | 0 / 1 / 5 | 0 / 0 / 5 | 0 / 1 / 5 |
| Total | 24 | 16 | 29 | 24 |
| Dispositions (fixed / rejected or kept) | 17 / 6 | 12 / 4 | 24 / 2 | 12 / 9 |

Every fork returned a report; the counts sit between round 2's opus forks (14-23) and the `sonnet[high]` forks (21-33).

## Behavior

- Spawn point: every sonnet writer spawned its four forks in one message after Step 2 and before its first plan write.

| Run | First Agent call | Fourth Agent call | First write under `_lyx/plan/` |
|---|---|---|---|
| sm-s-1 | 06:24:49 | 06:25:02 | 06:27:40 |
| sm-s-2 | 06:38:12 | 06:38:25 | 06:39:56 |
| sm-r-1 | 06:24:57 | 06:25:10 | 06:26:56 |
| sm-r-2 | 06:39:13 | 06:39:26 | 06:44:17 |

  Times are UTC.
- `validate-plan`: every writer ended on a passing run.
  The first run passed in both reference-1 plans of each arm; on reference 2 every first run failed on `card-fabric-reference` (a `-weft` mention reaching the fabric repository), sm-r-2 also on `card-missing-field` and fab-r-2 on `handle-dangling`, each fixed in one more run.
  That is 2 of 4 first passes for `sonnet[medium]` against 1 of 4 for `sonnet[high]`.
- Fable called `validate-plan` once or twice per plan and wrote all cards in one burst after its reading; it never re-read its plan against a review, having none.
  Three Fable plans and one sonnet plan use a `**Prosa:**` field the validator accepts; fab-r-2 also carries a "Rename mechanic" section for its `decodeSeed` rename.
- Rejections: the sonnet writers cited `path:line` for every rejection, and the spot-checked lines match the bases (`startup_test.go:49-55`, `testbudget_test.go:33`, `corpus_test.go:200-236`, `wait.go:966-997`, `session.go:61-94`, `send.go:430-432`, `main.go:180-187`, `main.go:222`, `archive.go:58-61`, `gitkit.go:26`).
  Four of 21 rejections kept a key defect, against two of 18 for `sonnet[high]` and none for round 2's `opus[medium]`, and one fix was wrong:
  - sm-s-1 and sm-s-2 both rejected the retry-exit tail row, citing that the status errors fail before `CapturePane` in that tick; the lines are accurate, but `run.lastStartupCapture` survives from an earlier successful probe, so a row with a capture first and status errors after (round 2's fix) reaches the condition.
  - sm-s-2 rejected criteria-coverage's MEDIUM on the start-level criterion (N1), citing the box methods as the only route; round 2's plans test it end to end.
  - sm-s-1 "fixed" row-reachability's finding that the skill-load death row is vacuous by asserting a non-empty capture that `skillReed` never holds, so the assertion would fail against the base fake; the fix is wrong, not a rejection.
  - sm-r-2 rejected criteria-coverage's J5 finding (no file in the run-id error) citing `runid.go:25`, keeping J5 at P.

## Verdict

- Twill `sonnet[medium]` is the cheapest twill arm by far: $2.99 per plan, 43% below twill `opus[medium]` and within 14% of single `opus[high]`, with writer time (7m25s) close to the single arm's.
  Medium effort cut the token volume that made `sonnet[high]` no cheaper than opus.
- Its quality does not hold up: it repeats more key defects than twill `opus[medium]` on both references (2.0 / 1.0 against 0.5 / 0.5; 2.5 / 4.0 against 2.0 / 3.5), the worst reference-1 score of any twill arm, and four of its 21 rejections kept a key defect, three on premises a fuller code read would have refuted.
  On reference 2 it sits level with `sonnet[high]` on R and worse on P.
- Single `fable[high]` does not match twill `opus[medium]`: on reference 1 it scores 3.5 / 0.5 (no start-level test in either plan, K1 and K2 repeated), the worst of any arm; on reference 2 it scores 4.0 / 3.0, level with single `opus[high]` (4.5 / 2.5) and behind twill `opus[medium]` (2.0 / 3.5).
  It costs $5.67 per plan, 8% more than twill `opus[medium]` and 2.2x single `opus[high]`, and its writer takes as long as the twill writer while running a third of the turns.
  Fewer turns did not buy more findings: Fable reads less (half single opus's cache reads) and writes once.
- Recommendation: keep twill `opus[medium]` for Plan-Write.
  Neither new arm reaches its quality: `sonnet[medium]` is cheap but repeats more and judges worse, and single Fable costs more than twill `opus[medium]` for single-writer quality.
  If cost matters more than the first-draft quality, the next arm worth a bench is twill with an `opus[medium]` writer and `sonnet[medium]` forks, since the lens findings, not the writer's own reading, closed most keys; this bench did not run it.
- Two runs per reference and one grader: a direction, not an effect size.

## Deviations

- Pair config went through `lyx config <module> --set` instead of a hand edit; the resulting lines match the earlier arms' (`plan:`, `claude:`, `claude_deny_agent_tool:`), and each set synced the pair's weft, so the later `lyx fabric commit` had nothing left to commit.
- `status.json` was written whole with the editor rather than patched, with the same entries and shape as the earlier arms' (checked against `archive/twb-son-r-1`).
- Grading is one grader's reading against round 2's keys; P marks a key a plan addresses with a gap the key's finding still names.
  K5 for fab-s-1 is P because the card rewrites `TestStartup_ReadyProbeTiming`'s doc comment, which holds the `//testtiming:keep` line, without naming the reason.
  J7 is graded R for fab-r-1 because its new tag listing has a skip branch no row reaches, and n/a for sm-r-1, which plans no skip.
- Torn down: all eight pairs with `lyx fabric remove --remote` (no worktree, portal, launcher or branch left locally or on either origin; records under `archive/twb-sm-*` and `archive/twb-fab-*` tags in the weft repository), all eight notes with `lyx board remove`, no claude process of these pairs left.
  Plans, lens-review files, the directive and both wrappers are under `round-sm-fable/`, the token report in `twill-plan-bench/tokens-sonnet-medium-fable.md`.
- Afterwards `lyx stencil list` showed every stencil and spec `untouched`, and the prime's working tree is clean.
