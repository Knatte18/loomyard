# twill-plan bench, round 1

2026-10-11, prime `/home/knatte/Code/loomyard-LYXHUB/loomyard`, board note `twill-plan`.
One run each for the single arms and two for twill `opus[medium]`; the $15 cap left no room for more (see Verdict).

## Method

- Reference: `shuttle-startup-ready` (landed as `567170457`, records tag `archive/shuttle-startup-ready/cf03187ce146`).
  Small (five cards), recent, and its plan had real defects: four Plan-Review round-1 findings, one round-2 finding, and a Webster done-check failure caused by the plan.
- Frozen inputs: code base `a35c8c2b1` (main when the reference pair was added at 15:08 local), and the decision record, support log and `_lyx/reviews/discussion/` from weft `995a85ba` (the review carry-over commit immediately before the reference Plan-Write).
  The reference Plan-Write draft is weft `75c4f889`, written by single `opus[high]` for $2.39 in 6m05s; the plan after review is `57e562ce`.
- Pairs `twb-pl-{oh-1,om-1,tw-1,tw-2}`, each from `lyx fabric add` in the prime with a `twb-*` note, code branch reset to `a35c8c2b1`, and the frozen discussion files checked out into the weft side with `git checkout 995a85ba -- ...`.
- Run: `lyx fabric commit`, `lyx shed seed self --recipe loom --driver go --param parent=main`, two `lyx loom step` calls (Preflight, Loom-Preflight), then `status.json` rewritten to put the run on Plan-Write with Discussion-Write and Discussion-Bouncer recorded done, `lyx fabric commit`, `lyx loom pause --after Plan-Write`, `lyx loom step --until-stop`.
  No Discussion-Write ran, so every arm planned from the same record byte for byte.
- Pair `loom.yaml` sets `plan:` per arm; pair `shuttle.yaml` points `claude:` at a scratchpad wrapper.
  Every arm's wrapper appends one code-base notice to the system prompt: the code base is HEAD, never read another ref such as `main` (main holds the landed solution).
- Prototype twill (no Go change, no stencil touched): the twill wrapper exports `CLAUDE_CODE_FORK_SUBAGENT=1` and appends the twill directive, rendered by hand from `contracts/stencils/twill/twill-directive.md`, to the system prompt; pair `shuttle.yaml` sets `claude_deny_agent_tool: false`.
  Rendered values: reviewed files the pair's `_lyx/plan/`; ready marker `<pair>/.lyx/twill-plan/plan.ready`; rubric `loom-rubric-plan-review.md`; format step "Step 3 and Step 4"; self-check `lyx loom validate-plan` per Step 5; dispositions in `<pair>/.lyx/twill-plan/lens-review.md` (Plan has no support log).
- Plan-Write's own gate holds a turn end while a fork is outstanding (`expiredTurnEnd` waits on every task for a gated run), so the existing shuttle wait needed no change.
- `twill-plan` fan, five lenses, carried in the directive (pair `burler.yaml` not used):
  - `decision-coverage`, `card-structure`, `pattern-fit`: the template lenses verbatim.
  - `edit-targets` (new): each card's `Create:`, `Edit:` and `Uses:` lists against the code, every touched file, member, test, fake and fixture listed with its real glyph, every `Create` handle in the package and file the Intent puts it in; rubric "Member glyphs on an `Edit` card" and "The re-sign arrow".
  - `tests-budget` (new): rubric "Verify coverage", "Test economy" and "Reachability of a prescribed refusal" on every planned test row, opening the test and its fakes, plus each new top-level test against `cmd/lyx/testdata/test-budget.yaml`.
- Cost: `go run ./tools/tokencount` (report in `twill-plan-bench/tokens.md`), orch excluded; wall is the writer's turn time.
- All four ran partly in parallel: oh-1 and om-1 with tw-1, tw-2 started once tw-1's forks had spawned.
- Prod `lyx` was `c279a38ba` ("Glyphs end to end"), newer than the reference run's binary, so the plan stencil, validator and glyph rules differ from the reference's.

## Results

| Arm | Run | Writer | Forks | Total | Wall | Messages | Cards |
|---|---|---|---|---|---|---|---|
| reference draft, single `opus[high]` | `75c4f889` | $2.39 | | $2.39 | 6m05s | 41 | 5 |
| single `opus[high]` | oh-1 | $2.67 | | $2.67 | 6m24s | 55 | 5 |
| single `opus[medium]` | om-1 | $2.77 | | $2.77 | 6m12s | 61 | 6 |
| twill `opus[medium]` | tw-1 | $3.47 | $0.97 (5 forks, 22 msgs) | $4.44 | 8m16s | 66 | 6 |
| twill `opus[medium]` | tw-2 | $3.75 | $0.90 (5 forks, 21 msgs) | $4.65 | 8m19s | 69 | 6 |

Round spend $14.53 of the $15 cap, orch excluded (tokencount attributes $0.47-$2.02 of orch per run).
Twill `opus[medium]` averages $4.55 and 8m18s: 1.6x single `opus[medium]` and 1.7x single `opus[high]` on cost, about two minutes longer.
`opus[medium]` single cost no less than `opus[high]` here (more messages, more cache reads).
Each twill writer spawned its five forks in one message, and every fork returned before its writer ended; forks ran about 1 minute each.
Both twill writers spawned the forks after writing the plan, not before as the directive says (tw-1 recorded it); the forks still waited on the marker, so prefix sharing was smaller but the review was intact.

## Grading

Reference defects (the grading key):

- K1 (Plan R1-F1, MEDIUM): card 3's no-tail rows do not reach their conditions: the skill-load-death row runs on `skillReed`, whose startup capture is empty, and no row covers a reed-status retry exit with a capture.
- K2 (R1-F2, LOW): new top-level `TestStartup_GateDismissalGap` where `TestStartup_UndismissableGateUntilWindowExpires` should become the table.
- K3 (R1-F3, LOW): the start-level test serves the lone-`%` capture only until the first send, so the skill-load send never sees it.
- K4 (R2-F1, LOW): that test's capture sequence has an unspecified, circular middle state.
- K5 (R1-F4, NIT): `TestStartup_ReadyProbeTiming`'s `//testtiming:keep` reason left stale.
- K6 (Webster): the start-level test is wired "as `corpus_test.go` does" (package `claudeengine_test`) while its `Create` glyph names package `claudeengine`; the batch's done-check failed and a recovery session moved the test.
  `lyx quarry resolve` answers `not_found` for an external-test-package function such as `claudeengine#TestCorpus_ReplaysEachTurnEnd` even though `lyx quarry glyphs` lists it under that id.

R repeated, P partly, A avoided, n/a the plan took another route.

| Key | oh-1 | om-1 | tw-1 | tw-2 |
|---|---|---|---|---|
| K1 no-tail rows | P (adds the retry-exit row with a capture; skill-load row still on an empty capture) | R | R (decision-coverage fork raised the retry exit; writer rejected it, wrongly saying no fake can script it) | P (edit-targets fork raised the empty `skillReed` capture as BLOCKING; fixed with a `Screen` field; no retry-exit row) |
| K2 new top-level test | R | R | R (`TestStartup_GateDismissalSpacing`) | A (widens `TestStartup_UndismissableGateUntilWindowExpires`) |
| K3 capture until first send | A (capture always the lone-`%` pane) | A (same) | n/a | n/a |
| K4 middle state | A | A | n/a | n/a |
| K5 stale keep reason | A (reworded) | R | R | R (reworded only for the gate test) |
| K6 test package vs glyph | A (file-only `Create`, no member glyph) | R (`claudeengine_test` with a `claudeengine#` glyph) | n/a | n/a |
| **R / P** | **1 / 1** | **4 / 0** | **3 / 0** | **1 / 1** |

The n/a cells hide a new defect in both twill plans:
- N1: neither twill plan carries the acceptance criterion "a start whose pane shows a ready box with a lone `%` passes the color send and the skill-load send and returns a run" with a test.
  Both writers decided against a start-level test before review (claudeengine sits at its test-budget row and no shuttle test drives Claude's own box reading); in each run the decision-coverage fork flagged it MEDIUM, and the writer kept the waiver (tw-1 in the overview's Shared Decisions, tw-2 in card 1's Intent).
  Counting N1 as a repeat, twill scores 4 / 0 and 2 / 1, against 1 / 1 for single `opus[high]` and 4 / 0 for single `opus[medium]`.

Other differences:
- Both twill plans split D3 over two cards (shuttleengine and shedadapters), tw-2 through a card-structure finding; the reference and both singles keep one card.
- tw-2's tests-budget fork caught a weak assertion (first-key-fails row compared against the wrong count) and a missing `RunTimeoutMin`; tw-1's tests-budget fork made card 5 assert the window-expiry tail.

## Lens finding counts

Counts are severity words (`BLOCKING`, `MEDIUM`, `LOW`) in each fork's final message; the writer's merged dispositions follow.

| Lens | tw-1 | tw-2 |
|---|---|---|
| decision-coverage | 3 | 4 |
| edit-targets | 5 | 3 |
| card-structure | 4 | 3 |
| tests-budget | 4 | 4 |
| pattern-fit | 6 | 3 |
| Total | 22 | 17 |
| Dispositions (fixed / rejected / no action) | 9 / 4 / 2 | 12 / 2 / 1 |

Every fork returned a report in both runs.
The findings that touched the key: tw-1 decision-coverage on K1's retry exit (rejected), tw-2 edit-targets on K1's empty capture (fixed); decision-coverage flagged N1 in both runs (rejected or waived).

## Verdict

- Twill `opus[medium]` for Plan-Write does not show a measurable gain in this round: its two runs score 3 / 0 and 1 / 1 on the key (4 / 0 and 2 / 1 counting N1), against 1 / 1 for single `opus[high]` and 4 / 0 for single `opus[medium]`.
- It costs 1.6-1.7x ($4.55 against $2.67-$2.77) and about two minutes more per plan.
- The lenses do find real plan defects (tw-2's fix of K1's empty capture is the reference's own fix), but the writer judges them: the two findings that would have closed K1 and N1 in tw-1 were rejected on a wrong premise or kept as a waiver.
- One run per single arm and two for twill: the spread inside the twill arm (1-3 R) is as wide as the gap between arms, so this is a direction, not an effect.
- A round 2 is worth it only with changes: run the forks before writing as the directive says (both writers wrote first), make a rejected BLOCKING or MEDIUM lens finding need a cited code read, and add a second reference task, since one five-card plan gives six keys and three of them went n/a for twill.
  Without such changes, more runs of the same arms would mostly measure spread; Plan-Review's two rounds caught K1-K5 anyway, and the defect that reached Webster (K6) was avoided by single `opus[high]`, not by twill.

## Deviations

- The twill directive reaches the writer through `--append-system-prompt` (merged into Claude's own deny-notice argument by the wrapper) instead of a `{{.twill_directive}}` marker inside the plan stencil; no stencil was edited, so nothing could leak to Bolt.
  Afterwards `lyx stencil list` showed all 71 stencils and specs `untouched`, and `_board` had no stencil or spec commit since the bench began.
- The fork env and the Agent tool came from the pair-local wrapper and `claude_deny_agent_tool: false`, as in the Discussion bench's round 4, so no Go change was needed; with that setting the writer's Agent tool was unrestricted, and it spawned only forks.
- The run reached Plan-Write through a hand-written `status.json`, not through Discussion-Write and Discussion-Review.
- The two new lens texts were written after reading the grading key; they cite rubric sections rather than the key's defects, but `edit-targets`' "package and file the Intent puts it in" sits close to K6.
- The code-base notice is an addition to the reference's prompt, the same for every arm.
- oh-1 ran `git log --all -- _lyx/plan/00-overview.md` in the code repository; it exited 1 with no output, so nothing leaked.
- "status push failed ... push rejected (remote diverged)" on each pair's code side after the reset to `a35c8c2b1`, as in the Discussion bench; harmless.
- Spend left no room for the second single runs or a twill `opus[high]` arm.
- Torn down: all four pairs with `lyx fabric remove --remote` (no branch left locally or on origin; records kept under `archive/twb-pl-*` tags in the weft repository), all four notes, no reed strand left.
  The lens-review files lived in each pair's untracked `.lyx` and went with the pair; their content is summarized above.

# Round 2

2026-10-11, prime `/home/knatte/Code/loomyard-LYXHUB/loomyard`, prod `lyx` at `c279a38ba`, as in round 1.
Arms: single `opus[high]` (the hub default) against improved twill `opus[medium]`, two runs per arm per reference.
Round 1's single `opus[high]` run `oh-1` on reference 1 counts as one of that arm's two runs there; round 1's twill runs do not, since the prototype changed.

## Setup and changes

- Mechanics as round 1: one `twb-*` note per pair, `lyx fabric add` in the prime, code branch reset to the base, the frozen discussion files checked out with `git checkout <weft sha> -- _lyx/discussion _lyx/reviews/discussion`, `lyx fabric commit`, `lyx shed seed self --recipe loom --driver go --param parent=main`, two `lyx loom step` calls, `status.json` edited onto Plan-Write with Discussion-Write and Discussion-Bouncer done, `lyx fabric commit`, `lyx loom pause --after Plan-Write`, `lyx loom step --until-stop`.
- Pair `shuttle.yaml` points `claude:` at a scratchpad wrapper; twill pairs also set `claude_deny_agent_tool: false` and pair `loom.yaml` `plan: opus[medium]`.
  Every arm keeps round 1's code-base notice.
- Reference 1: `shuttle-startup-ready`, base `a35c8c2b1`, records `995a85ba`, keys K1-K6 plus N1 as in round 1.
- Reference 2: `run-time-by-role` (landed as `6ea291720`, records tag `archive/run-time-by-role/1113f9cf4d58`).
  Code base `c00b2e524`, main when the pair was added at 13:11 local and the parent of its first card commit `64e6e90fe`.
  Frozen records: weft `8f5c017a`, the discussion artifacts commit right after the review carry-over and before Plan-Write.
  The reference draft is weft `e79c7931`, single `opus[high]`, $2.62 and 7m37s by tokencount; the plan after two Plan-Review rounds is `f8e2ada7`.
  Chosen over `per-role-models` (14-minute Plan-Write, a mid-review record change) and `plan-gaps` (the prod plan stencil names its `insert-card` verbs, a solution leak).
- Pairs: `twb-p2s-tw-{1,2}` and `twb-p2s-oh-2` on reference 1; `twb-p2r-tw-{1,2}` and `twb-p2r-oh-{1,2}` on reference 2.
  Wave 1 ran the four twill pairs and `twb-p2r-oh-1` in parallel from 19:44; wave 2 ran the two `oh-2` pairs in parallel from 19:57, once wave 1's cost was known.

Changes to the prototype:

1. Spawn point.
   The wrapper now splices the directive into the run's own `prompt.md` (beside the `--settings` file) as `## Step 2b — Lens review by forks: spawn before you write`, just before `## Step 3 — Write the plan into`, so it sits in the plan task's own step order instead of the system prompt.
   Its text says the first Write of any file under `_lyx/plan/` comes after the Agent call, has the writer state its cards in a text reply first, and then spawn in the same turn.
   The system prompt keeps only a one-line pointer to Step 2b.
2. Cited rejections.
   Step 6 now reads: a rejection cites the code read to reject it, a `path:line` or glyph opened in this session and what it shows; an uncited rejection is not allowed, a finding not refuted from code read is fixed; a kept waiver, deferral or skipped test counts as a rejection and needs the same citation.
   The lens-review file records each finding's lens, severity and `fixed` or `rejected` with its citation.
3. Lenses, tuned from round 1 only (the text is in `twill-plan-bench/round-2-directive.md`):
   - Dropped `card-structure` and `pattern-fit`: in round 1 they raised 16 findings, none touching the key, mostly LOW confirmations or granularity opinions.
   - `criteria-coverage` (was `decision-coverage`, which found K1's retry exit and N1 twice): every decision and acceptance criterion mapped to a card and a test row that would fail; a criterion argued, waived or deferred instead of tested is a finding.
   - `row-reachability` (from `edit-targets`' BLOCKING K1 find and `tests-budget`'s weak-assertion find): open each named test and its fakes, check the row's input reaches the claimed branch, the assertion can fail, and the needed fake fields are listed.
   - `forced-edits` (from round 1's K5 and K6 misses): callers, output-asserting tests, comments, docs and stated reasons such as `//testtiming:keep` that a change makes wrong; each `Create` handle's package against the file's package clause, `_test` packages included.
   - `test-economy` (from round 1's K2 miss): a new top-level test that should be a row, a row that repeats existing coverage, and the test-budget row.
   - The fork boilerplate adds: report defects only, never a passed check; a severity scale; evidence as a `path:line` or glyph.

## Results

Cost and turn time from `go run ./tools/tokencount` (report in `twill-plan-bench/tokens-round-2.md`), orch excluded.

| Ref | Arm | Run | Writer | Forks | Total | Writer time | Messages | Cards |
|---|---|---|---|---|---|---|---|---|
| 1 | single `opus[high]` | oh-1 (round 1) | $2.67 | | $2.67 | 6m24s | 55 | 5 |
| 1 | single `opus[high]` | p2s-oh-2 | $2.48 | | $2.48 | 5m56s | 51 | 5 |
| 1 | twill `opus[medium]` | p2s-tw-1 | $4.38 | $1.35 | $5.73 | 9m41s | 64 | 5 |
| 1 | twill `opus[medium]` | p2s-tw-2 | $4.17 | $1.22 | $5.39 | 9m38s | 75 | 5 |
| 2 | reference draft, single `opus[high]` | `e79c7931` | $2.62 | | $2.62 | 7m37s | 43 | 6 |
| 2 | single `opus[high]` | p2r-oh-1 | $2.41 | | $2.41 | 6m48s | 38 | 4 |
| 2 | single `opus[high]` | p2r-oh-2 | $2.95 | | $2.95 | 8m08s | 49 | 6 |
| 2 | twill `opus[medium]` | p2r-tw-1 | $3.63 | $1.26 | $4.89 | 10m12s | 53 | 5 |
| 2 | twill `opus[medium]` | p2r-tw-2 | $3.88 | $1.08 | $4.96 | 11m10s | 59 | 5 |

Round spend $28.82 of the $30 cap ($23.91 writers, $4.91 forks), orch excluded (tokencount attributes $2.56 of orch to the round).
Twill averages $5.24 against $2.63 for single `opus[high]` (2.0x) and about 3m30s more writer time.
The forks cost more than in round 1 ($1.08-$1.35 against $0.90-$0.97 for five), since four forks now inherit a context of Step 2's reading and each waited about five minutes while the writer wrote.

Spawn point: every twill writer spawned its four forks after Step 2 and before its first plan write.

| Run | First Agent call | Fourth Agent call | First write under `_lyx/plan/` |
|---|---|---|---|
| p2s-tw-1 | 17:49:09 | 17:49:27 | 17:51:42 |
| p2s-tw-2 | 17:49:53 | 17:50:11 | 17:52:12 |
| p2r-tw-1 | 17:49:02 | 17:49:20 | 17:49:38 |
| p2r-tw-2 | 17:48:21 | 17:48:39 | 17:48:54 |

Times are UTC.
Every fork returned a report before its writer ended.

## Grading

### Reference 1 (`shuttle-startup-ready`)

Keys K1-K6 and N1 as in round 1; R repeated, P partly, A avoided, n/a the plan took another route.

| Key | oh-1 (r1) | p2s-oh-2 | p2s-tw-1 | p2s-tw-2 |
|---|---|---|---|---|
| K1 no-tail rows | P | R (skill-load death row on `skillReed`'s empty capture; no retry-exit row with a capture) | A (`skillReed.Screen` set to a ready screen; new `TestStartup_MechanismFailure` row "reed stops tracking the strand after a capture") | A (same two fixes) |
| K2 new top-level test | R | A (`TestStartup_TrustPromptThenReady` becomes the table) | A (new `TestStartup_GateDismissalGap` replaces and deletes `TestStartup_UndismissableGateUntilWindowExpires`, budget unchanged) | R (new `TestStartup_DismissalGap` beside the existing gate test) |
| K3 capture until first send | A | A | A | A |
| K4 middle state | A | A | A | A |
| K5 stale keep reason | A | A | A | A |
| K6 test package vs glyph | A | A (no package named for the start test) | P (`Create` glyphs spelled `claudeengine_test#...` for an external test file; the package matches the file, but that path names no directory, and whether Webster binds it is untested) | A (file-only `Create`, package `claudeengine_test`) |
| N1 start-level criterion | A | A | A | A |
| **R / P** | **1 / 1** | **1 / 0** | **0 / 1** | **1 / 0** |

Means: single `opus[high]` 1.0 / 0.5, twill `opus[medium]` 0.5 / 0.5; round 1's twill was 3.0 / 0.5 counting N1.
Both new single and twill plans now carry a start-level test, which round 1's twill plans had waived.

### Reference 2 (`run-time-by-role`)

The grading key, from the reference's Plan-Review rounds against its draft and from its Webster record:

- J1 (Plan R1-F1, BLOCKING): seed-decoder tests repeat behaviors existing `ReadSeed` tests reach once `ReadSeed` delegates (`started_at` kept, retired `parent` key, absent driver, unknown key).
- J2 (R1-F2, MEDIUM): no row reaches the timestamp-parse abort (`started_at`, `at`, `last_at`), nor a tag holding `status.json` and no `seed.json`.
- J3 (R1-F3, LOW): no row tells the "All runs" sum from a union, two runs overlapping in wall-clock time.
- J4 (R1-F4 with R2-F1, LOW): `internal/shedrun`'s `doc.go` and `paths.go` header keep the claim that every constructor joins onto a told `Location`, which the tree-relative paths break, as `RunsRootRel`, `SeedRel`, `StatusRel` and `DriveReportsRel` already do.
- J5 (R2-F3, LOW): the tree-path run-id error is not wrapped with the slug, the tag and the file Decision 9 names.
- J6 (R2-F4, LOW): the foreign-hub check compares against `filepath.Dir(wd)`, the cwd's parent, not the worktree's parent Decision 8 names.
- J7 (R2-F5, LOW): no row reaches the archive-tag parser's missing-commit skip.
- J8 (Webster deviation): `tools/tokencount/orch_test.go` (`TestCountOrch`) asserts the role-table rows the time columns change, and no card lists it.
- J9 (Webster verify-gate failure): tests in `tools/tokencount` named or spelled with the bare `weft` token, which `TestEnforcement_FabricVocabulary` bans in every test file (case-insensitive substring); the fixer renamed it to `records`.

R2-F2 (`last_at` parsed only on folded entries) is left out: it arose from the round-1 fix's wording, and the draft already defines an entry's end by `Repeats`.
The Webster review's one NIT (`max2`/`min2` doc comments) and the informational `shed.go` file-placement deviation are code or placement notes, not plan defects.
The reference draft repeats all nine.

| Key | p2r-oh-1 | p2r-oh-2 | p2r-tw-1 | p2r-tw-2 |
|---|---|---|---|---|
| J1 repeated seed rows | R (new `TestTreeRelPaths` plus `DecodeSeed` rows for absent driver, `parent`, unknown key, `started_at`) | R (new `TestDecodeSeed` with the same four rows) | A (test-economy fork; keeps only rows no `ReadSeed` test reaches and names the covering tests) | P (rows land in existing tests, but repeat `parent`, unknown-key and absent-driver behaviors) |
| J2 parse abort, seed-less status | R (decode rows only) | P (`at` abort row only) | A (`started_at` `yesterday`, bad `at`, status with no seed; row-reachability fork) | A (bad entry end, bad `started_at`, no seed file; row-reachability fork) |
| J3 "All runs" sum vs union | P | P | P (two runs share a role and the sum is asserted; no wall-clock overlap) | P (same) |
| J4 shedrun constructor claim | R (`doc.go` gains a paragraph, the claim stays, `paths.go` unlisted) | P (`doc.go` gains an exception list; `paths.go` header unlisted) | P (both files fixed by forced-edits, but name the new pair as "the one exception") | P (`paths.go` and `doc.go` reworded by forced-edits; the closing claim's other counterexamples unaddressed) |
| J5 run-id error wrapping | A | P (row "a slug `..` is refused", wrapping unstated) | P (names the slug only) | P (slug and tag, no file) |
| J6 cwd parent vs worktree parent | R | R | R | R |
| J7 missing-commit skip row | n/a (date error, no commit branch) | n/a | A ("missing a field" skip reached by a row) | n/a |
| J8 `orch_test.go` unlisted | R | A | A | A (forced-edits made `TestCountOrch`'s rows hold) |
| J9 bare `weft` in tests | R (`TestCheckHubWeft`) | R (`TestRunRefusesAForeignHubWithoutWeft`, `errHubNeedsWeft` in assertions) | R (`TestHubNeedsWeft`) | R (`errHubNeedsWeft` in assertions, `main_test.go` comment) |
| **R / P** | **6 / 1** | **3 / 4** | **2 / 3** | **2 / 4** |

Means: single `opus[high]` 4.5 / 2.5, twill `opus[medium]` 2.0 / 3.5, reference draft 9 / 0.
New defect outside the key: p2r-oh-1 adds the top-level `TestTreeRelPaths` to `internal/shedrun`, which sits at its test-budget row (22 of 22), without listing `cmd/lyx/testdata/test-budget.yaml` on card 1.
J6 and J9 survived every arm and every lens; J6 was caught only in the reference's second Plan-Review round, and J9 by no review, only by the verify gate.

## Lens finding counts

Counts are severity words in each fork's final message (B BLOCKING, M MEDIUM, total).

| Lens | p2s-tw-1 | p2s-tw-2 | p2r-tw-1 | p2r-tw-2 |
|---|---|---|---|---|
| criteria-coverage | 0 / 1 / 5 | 0 / 0 / 3 | 0 / 2 / 6 | 0 / 2 / 5 |
| row-reachability | 0 / 1 / 3 | 0 / 0 / 3 | 2 / 4 / 9 | 1 / 1 / 3 |
| forced-edits | 1 / 0 / 3 | 1 / 0 / 6 | 0 / 0 / 4 | 2 / 0 / 6 |
| test-economy | 1 / 0 / 3 | 1 / 1 / 3 | 0 / 0 / 4 | 0 / 0 / 4 |
| Total | 14 | 15 | 23 | 18 |
| Dispositions (fixed / rejected) | 13 / 0 | 8 / 3 | 18 / 1 | 15 / 1 partly |

Round 1's five lenses gave 22 and 17 per run; the four tuned lenses give 14-23, with no "covered" confirmations.

Which lenses found real defects:
- row-reachability: J2 in both reference-2 runs (the parse-abort rows), p2r-tw-1's BLOCKING (a second run with no transcript makes `CountRun` abort, `tools/tokencount/count.go:488-490`), the `RunTimeoutMin` gap in both reference-1 runs, as round 1's tests-budget lens had found.
- forced-edits: J4 in both reference-2 runs, J8's consequence in p2r-tw-2 (two BLOCKING: `TestCountOrch`'s rows could not hold with a zero span, and a span line broke its exact-text match), stale doc comments in every run, and in p2s-tw-1, merged with test-economy, a BLOCKING outside the key: the new `fmt.Errorf` return in `wait.go` trips the `auditedNegativeVerdictReturns` tripwire unless listed.
- test-economy: J1 in p2r-tw-1 (fully) and p2r-tw-2 (partly), K2 in p2s-tw-1; it missed K2 in p2s-tw-2.
- criteria-coverage: K1's tail exits in both reference-1 runs (tick-cap and run-deadline rows), J3 partly in both reference-2 runs (the "All runs" sum asserted, without overlap).
- None found J6 or J9; no lens text names Decision-wording fidelity of a comparison or PATTERN vocabulary scans, since `pattern-fit` was dropped as round-1 noise.

## Rejections and their citations

Every rejection cites code, and the cited lines match the base:
- p2s-tw-2, test-economy LOW, the window-expiry tail row repeats the pane-died one: rejected, citing pane death at `internal/shuttleengine/wait.go:544` and window expiry through `Run.classifyStartupWindow` (`wait.go:1054`) as two distinct exits.
- p2s-tw-2, criteria-coverage LOW, no row for `RunGated`'s other error return: rejected, citing `Runner.RunGated` (`internal/shuttleengine/run.go:746-756`), whose non-`ErrNotStarted` branch returns `start`'s result unchanged.
- p2s-tw-2, forced-edits LOW, absence assertions share a substring with `withPaneTail`: rejected, citing the status-cap errors at `wait.go:531-535` and the skill-load death's nil cause in `loadSkillsThenPrompt`.
- p2r-tw-1, forced-edits LOW, list `tools/tokencount/main.go` beside `run`: rejected, citing `lyx loom validate-plan`'s `redundant-file-target` refusal in the session, a tool answer rather than a code read.
- p2r-tw-1, criteria-coverage LOW, seed read errors other than absence: fixed in the Intent, the missing row justified by `fakeHistory.FileAtRevision` (`tools/tokencount/calibrate_test.go:32-38`), which returns only `ErrPathNotAtRevision`.
- p2r-tw-2, test-economy LOW, the end-to-end block repeats rendering: rejected in part, citing the title written only by `run` at `tools/tokencount/main.go:222`.
None of the rejections dropped a key defect, unlike round 1, where tw-1 rejected K1's retry exit on a wrong premise and both runs waived N1.

## Verdict

- Improved twill `opus[medium]` beats single `opus[high]` on both references: 0.5 / 0.5 against 1.0 / 0.5 on reference 1, and 2.0 / 3.5 against 4.5 / 2.5 on reference 2.
  On reference 2 the gap (J1, J2, J4 and J8 closed by named lens findings) is wider than the spread inside either arm (2 R both twill runs, 3-6 R single).
- It costs 2.0x ($5.24 against $2.63 per plan) and about 3m30s more writer time; this round did not measure Plan-Review's cost per round, so whether the extra $2.61 pays back depends on Plan-Review running fewer rounds after it.
- The three changes worked: all four writers spawned before writing, every rejection carried a citation that checks out, and the four tuned lenses gave fewer, sharper findings, every key closure tracing to a named lens finding.
- Reference 1 is a biased test: `forced-edits` names `//testtiming:keep` and `_test` packages and `test-economy` names K2's shape, all from round 1's misses on it; reference 2 is the fair test, and its gain holds.
- The remaining misses (J6 and J9) are a Decision-wording comparison and a PATTERN vocabulary scan, which no tuned lens covers; a short `pattern-scans` lens (each identifier, literal and comment a card prescribes against the repo's enforcement tests) is the candidate for a round 3.
- Two runs per arm per reference and one grader: a direction with a consistent sign on both references, not a measured effect size.
- Recommendation: build twill for Plan-Write as a real producer at `opus[medium]`, with the directive as a marker between Step 2 and Step 3 of the plan stencil, these four lenses and the cited-rejection rule, and bench whether Plan-Review can then run one round.
  Twill `opus[high]` was not run: the round's $28.82 left no room for it.

## Deviations

- The wrapper rewrites the run's `prompt.md` after shuttle writes it and before Claude reads it, through bash parameter expansion and `printf`, to splice Step 2b in; this stands in for a `{{.twill_directive}}` stencil marker and touched no stencil.
  It ran on the four twill pairs only (log `wrap-<pair>.log`), and each spliced prompt had the Step 2b heading right before Step 3 and no unrendered `PAIRDIR`.
- Reference 2's key was read before the lens texts were written; the lens texts cite only round-1 evidence and rubric sections, and none names a J key.
- Reference 1's `forced-edits` and `test-economy` texts were written from that reference's own misses, so its twill result is not independent; see Verdict.
- Grading is one grader's reading of each plan against the key; P marks a key a plan addresses with a gap the key's finding still names.
- The status edits, pause and notice are round 1's; the frozen discussion record carries its `lyx:carry-over` block, as it did for the reference writer.
- No "status push failed" warning this round.
- The single arm's second reference-1 run is round 1's `oh-1`, run the day's same binary and mechanics, under round 1's notice.
- Torn down: all seven pairs with `lyx fabric remove --remote` (no worktree or branch left locally or on either origin; records under `archive/twb-p2*` tags in the weft repository), all seven notes, no reed strand left.
  Plans, lens-review files, the directive and the wrapper are copied under `round-2/`, the token report in `twill-plan-bench/tokens-round-2.md`.
- Afterwards `lyx stencil list` showed all 71 stencils and specs `untouched`, and `_board` had no stencil or spec commit since the round began.

# Round 3: twill opus[high]

2026-10-11, prime `/home/knatte/Code/loomyard-LYXHUB/loomyard`.
One arm, twill `opus[high]`, two runs per reference, so it compares directly with round 2's twill `opus[medium]` and single `opus[high]` arms.

## Setup

- Round 2's setup unchanged: the same directive (byte-identical copy), the same four lenses (`criteria-coverage`, `row-reachability`, `forced-edits`, `test-economy`), the same cited-rejection rule, the same Step 2b splice, the same code-base notice, bases (`a35c8c2b1` with records `995a85ba`, `c00b2e524` with records `8f5c017a`), status edits, pause and grading keys.
- The wrapper is round 2's with only its log and directive paths moved under `r3/` (copy in `round-3/wrap-twill.sh`).
- Pair `loom.yaml` keeps the shipped `plan: opus[high]`; pair `shuttle.yaml` points `claude:` at the wrapper and sets `claude_deny_agent_tool: false`.
  Each writer ran `claude --model opus --effort high`; forks inherit it.
- Pairs `twb-p3s-th-{1,2}` on reference 1, `twb-p3r-th-{1,2}` on reference 2.
  To guard the cap, wave 1 ran `th-1` on both references in parallel from 20:16 local, wave 2 the two `th-2` pairs from 20:33 once wave 1's $10.68 was known.

## Results

Cost and turn time from `go run ./tools/tokencount` (report in `twill-plan-bench/tokens-round-3.md`), orch excluded.

| Ref | Run | Writer | Forks | Total | Writer time | Messages | Cards |
|---|---|---|---|---|---|---|---|
| 1 | p3s-th-1 | $4.12 | $1.24 | $5.36 | 9m53s | 67 | 5 |
| 1 | p3s-th-2 | $5.37 | $1.38 | $6.75 | 12m23s | 83 | 5 |
| 2 | p3r-th-1 | $4.10 | $1.22 | $5.32 | 15m6s | 51 | 4 |
| 2 | p3r-th-2 | $4.60 | $1.41 | $6.01 | 12m33s | 63 | 5 |

Round spend $23.44 of the $30 cap ($18.20 writers, $5.24 forks), orch excluded (tokencount attributes $6.79 of orch to the round).
Every writer spawned its four forks after Step 2 and before its first plan write (first Agent call 38-66 s before the first write, all four within 19 s), and every fork returned a report.

## Three arms

Repeated / partly repeated, mean of two runs; cost per plan and writer time are means over the same two runs.

| Ref | Arm | R / P | Cost per plan | Writer time |
|---|---|---|---|---|
| 1 | single `opus[high]` (r1 oh-1, r2 p2s-oh-2) | 1.0 / 0.5 | $2.58 | 6m10s |
| 1 | twill `opus[medium]` (r2) | 0.5 / 0.5 | $5.56 | 9m40s |
| 1 | twill `opus[high]` (r3) | 0.5 / 0.0 | $6.06 | 11m8s |
| 2 | single `opus[high]` (r2) | 4.5 / 2.5 | $2.68 | 7m28s |
| 2 | twill `opus[medium]` (r2) | 2.0 / 3.5 | $4.93 | 10m41s |
| 2 | twill `opus[high]` (r3) | 1.0 / 3.5 | $5.67 | 13m50s |

Over both references twill `opus[high]` averages $5.86 and 12m29s writer time per plan, against $5.24 and 10m10s for twill `opus[medium]` (+12%, +2m19s) and $2.63 and 6m49s for single `opus[high]` (2.2x).

## Grading

### Reference 1 (`shuttle-startup-ready`)

| Key | p3s-th-1 | p3s-th-2 |
|---|---|---|
| K1 no-tail rows | A (`skillReed.StartupCapture` set to a ready screen for the died row; `TestStartup_MechanismFailure` row with a pending capture, then status errors to the retry cap; row-reachability fork) | A (`skillReed.Screen`; row "reed stops tracking the strand after a capture") |
| K2 new top-level test | R (new `TestStartup_DismissalGap` beside `TestStartup_UndismissableGateUntilWindowExpires`; the test-economy fork raised it, the writer rejected it citing `startup_test.go:297-341` as a different behavior) | A (the new table absorbs the old test as a row and deletes it, package count unchanged; test-economy fork) |
| K3 capture until first send | A | A |
| K4 middle state | A | A |
| K5 stale keep reason | A | A |
| K6 test package vs glyph | A (file-only `Create`, package `claudeengine_test`) | A (package `claudeengine` with `claudeengine#` glyphs) |
| N1 start-level criterion | A | A |
| **R / P** | **1 / 0** | **0 / 0** |

### Reference 2 (`run-time-by-role`)

| Key | p3r-th-1 | p3r-th-2 |
|---|---|---|
| J1 repeated seed rows | P (`DecodeSeed` rows land in existing tests but repeat absent driver, `started_at` and unknown key) | P (a `DecodeSeed` subtest repeats `parent`, `started_at` and unknown key) |
| J2 parse abort, seed-less status | A (`at` `yesterday`, `started_at` `soon`, no seed file; row-reachability fork) | A (bad `at`, bad `started_at`, no seed; test-economy and row-reachability forks) |
| J3 "All runs" sum vs union | A (the same run rendered under two slugs, so two runs overlap in wall-clock time, and the "All runs" Producers row asserts the 2h sum) | A (same shape, "All runs" `Webster` 1h20m0s) |
| J4 shedrun constructor claim | P (`doc.go` gains the `RunsRootRel` and `RunFilesRel` exceptions; `paths.go` header unlisted) | P (`doc.go` and `paths.go` headers except only the new pair; `SeedRel`, `StatusRel`, `DriveReportsRel` unaddressed) |
| J5 run-id error wrapping | P (slug only, stated as a deviation from Decision 9 after a criteria-coverage finding) | A (slug, tag and `status.json`; criteria-coverage fork) |
| J6 cwd parent vs worktree parent | R | R |
| J7 missing-commit skip row | A (a line without a date reaches the field-count skip) | A (a line with no sha) |
| J8 `orch_test.go` unlisted | A (`TestCountOrch` on card 2) | A (`TestCountOrch` on card 2) |
| J9 bare `weft` in tests | P (no test or identifier named with it, but a row asserts the refusal names "the weft flag's name, dash included") | P (no test or identifier named with it, but a row asserts "the message names -weft") |
| **R / P** | **1 / 4** | **1 / 3** |

J9 is graded P, not A: neither writer nor any fork opened `TestEnforcement_FabricVocabulary`, so the plans avoid round 2's `TestHubNeedsWeft` and `errHubNeedsWeft` by wording, and each leaves the spelling of the flag-name assertion to the implementer; existing code builds it as `"-" + repositoryFlag` (`tools/tokencount/calibrate_integration_test.go:165`).
Against round 2's twill `opus[medium]`, the R drop on reference 2 is J9 alone (R to P); the P count holds at 3.5 with J3 and J7 closed in both runs and J5 in one, offset by J9.
J6 survives every arm of all three rounds.

## Lens finding counts

| Lens | p3s-th-1 | p3s-th-2 | p3r-th-1 | p3r-th-2 |
|---|---|---|---|---|
| criteria-coverage | 0 / 3 / 5 | 0 / 2 / 4 | 0 / 6 / 10 | 1 / 1 / 6 |
| row-reachability | 0 / 2 / 4 | 1 / 0 / 3 | 0 / 1 / 7 | 1 / 2 / 9 |
| forced-edits | 0 / 0 / 3 | 0 / 0 / 3 | 0 / 0 / 6 | 0 / 0 / 4 |
| test-economy | 0 / 0 / 3 | 0 / 0 / 5 | 0 / 0 / 5 | 0 / 1 / 4 |
| Total | 15 | 15 | 28 | 23 |
| Dispositions (fixed / rejected) | 14 / 1 | 13 / 2 | 21 / 0 | 17 / 0 |

Cells are B BLOCKING / M MEDIUM / total, counted as in round 2.
Reference 1 gives the same 14-15 findings as round 2; reference 2 gives 23-28 against 18-23, with more MEDIUM criteria-coverage findings.
All three rejections cite code that matches the base: `internal/shuttleengine/startup_test.go:297-341` (the undismissable-gate test) and `cmd/lyx/testdata/test-budget.yaml:96` (161) in p3s-th-1; `claudeengine/session.go:36,197-208` (`boxInteriorChars`, `isBlankBoxInterior`), `startup_test.go:191-222` and `wait.go:1125-1131` (`identity`) in p3s-th-2.
The p3s-th-1 rejection keeps K2: cited and accurate, but the reference's Plan-Review judged the same choice a defect.

## Verdict

- Twill `opus[high]` does not clearly earn its extra cost over twill `opus[medium]`.
  It scores 0.5 / 0.0 against 0.5 / 0.5 on reference 1, and 1.0 / 3.5 against 2.0 / 3.5 on reference 2, where the whole R gain is J9, avoided by wording rather than by a writer or lens that read the vocabulary scan.
  These gaps are no wider than the spread inside either arm over two runs.
- The extra cost is small: +$0.62 per plan (+12%) and about 2m20s more writer time, mostly from a longer writer (more messages and cache reads on reference 1, longer turns on reference 2); the forks cost about the same ($1.22-$1.41 against $1.08-$1.35).
- Both twill arms still beat single `opus[high]` on reference 2 (4.5 / 2.5) at about 2.2x its cost; the round-2 recommendation stands: build twill for Plan-Write at `opus[medium]`.
- J6 (a Decision-wording comparison) and J9 (a PATTERN vocabulary scan) remain the misses no lens covers, so round 2's `pattern-scans` lens is a better next bench than more writer effort.

## Deviations

- Production `lyx` moved during the round: wave 1 started on `86982fa43`, the binary installed at 19:43, which also ran round 2 (its waves began at 19:44, so round 2's "`c279a38ba`" understates it); the operator's landing installed `b2e4cf913` at 20:30 and `3a627c00d` at 20:37.
  Wave 2's `lyx loom step` ran on `b2e4cf913`, and every `lyx` the writers and forks called after 20:37 (`validate-plan`, `shuttle await`) on `3a627c00d`.
  That landing re-seeded `_board`'s `loom-rubric-plan-review.md` and `loom-plan-spec.md` at 20:30; both diffs only rename the review rows (`Plan-Burler` to `Plan-Burler-Fix` or `-Review`, `**Fix:**` line to `fix` key), so wave 1 and wave 2 writers read the same rubric content, and the plan stencil was untouched.
- Wave 2 started 17 minutes after wave 1 instead of all pairs in one wave.
- Grading is one grader's reading against round 2's keys; J9's P, a judgment on prescribed wording, is explained above.
- Torn down: all four pairs with `lyx fabric remove --remote` (no worktree or branch left locally; records under `archive/twb-p3*` tags in the weft repository), all four notes, no reed strand of these pairs left.
  The `twb-son-*` pairs and strands present at the end are not this round's.
  Plans, lens-review files and the wrapper are under `round-3/`, the token report in `twill-plan-bench/tokens-round-3.md`.
- Afterwards `lyx stencil list` showed all 72 stencils and specs `untouched` (the landing added one); `_board`'s only stencil or spec commits since the round began are the landing's three `lyx: seed` commits, none by the bench.
