# Review-setup audit, 2026-10-28

What each part of the review setup is worth, read from the records of landed loom runs.
Every figure derives from the scripts and TSV tables beside this file; the sample size `n` stands beside each number.
Each question ends in a recommendation.
A recommendation is not a decision: it changes no profile, `loom.yaml`, lens set or stencil.

## Sample and method

- Source: the weft repository's archive tags created at or after `79bab47a0` (2026-10-24) that hold `*.lens-<lens>.md` files.
  74 archive tags are newer than that commit, and 50 of them hold lens files (`runs.txt`); `unpack-runs.sh` unpacks them.
- No tag is an OSL-1046 hub run; the records repository is Neve's, and OSL-1046 appears only as a downstream target in some decision records.
- 8 of the 50 are full loom runs with Discussion and Plan perches: `beam-start-and-ownership`, `discussion-scope-in-lenses`, `dotnet-scoped-verify`, `gate-runs-once`, `regression-baseline-verify`, `umbrella-anchor`, `umbrella-daemon`, `umbrella-groundwork`.
  Questions 1 and 3 and 4 and the Discussion and Plan lens figures therefore rest on `n = 8` runs, most of them large umbrella tasks.
- The other 42 hold no Discussion or Plan review; most are darn runs with one code review.
  Code figures cover 60 code review rounds (43 first-round fans, 2 solo second rounds, 15 archived re-reviews).
- Tokens are `fresh_tokens` from each round's `round-<n>-usage.yaml`: uncached input plus output, review and fixer together.
  Cache reads are left out; they run roughly 10 to 65 times the fresh count and no pricing is in the records.
  A round's lens forks are inside its review figure.
  Minutes are the review's and the fixer's `wall_seconds`, summed.
- `lym report convergence` and `lym report timing` refuse outside the hub's prime worktree and this task works in its own, so neither ran.
  `tools/tokencount/tokencount.sh` was not run either: it reads the same usage records through `go run`, which this session may not do.
  The usage records are the source for every token and minute figure.
- Gating means severity BLOCKING or MEDIUM.
- Real-or-noise is the writer's judgement, by these rules:
  - A finding a round review kept counts as real; the review's `## Rejected` list holds what its lead dropped.
  - A finding's fix holds when the next round's bouncer ledger lists its key as resolved; a key from a perch's last round has no later ledger and stays unverified.
  - A twill lens item counts as real when the writer's own disposition list says `fixed`, and as noise when it says `rejected` or `disputed`.
  - A review finding carries one `origin`, so a finding two lenses raised is credited to one of them; lens counts are a lower bound on what a lens can find.

| File | Content |
| --- | --- |
| `findings.tsv` | every kept finding: run, perch, round, severity, class, origin (`extract-findings.sh`) |
| `rounds.tsv` | per round: fan, model, tokens, minutes, verdicts, finding counts (`extract-rounds.sh`) |
| `keys.tsv` | per bouncer-ledger key: first round, class, fix kind, final status (`extract-keys.sh`) |
| `rejected.tsv` | lens items a review dropped or folded (`extract-rejected.sh`) |
| `twill.tsv` | writer-side lens items and their disposition (`extract-twill.sh`) |
| `lens-hit-rate.tsv` | per review lens: rounds run, rounds with a finding, gating, blocking (`lens-hit-rate.sh`) |
| `delta.tsv` | per round-2-or-later finding: whether the previous fixer touched its text (`extract-delta.sh`) |
| `themes.tsv` | per gating finding: keyword theme (`classify-findings.sh`) |

## 1. Rounds per perch

| Perch and round | Runs reaching it | Kept findings per round | Gating per round | Fresh tokens per round | Minutes per round | Fresh tokens per gating finding |
| --- | --- | --- | --- | --- | --- | --- |
| Discussion 1, lens fan | 8 | 15.4 | 8.4 | 485k | 17.8 | 58k |
| Discussion 2, lens fan | 8 | 12.5 | 7.5 | 468k | 15.8 | 62k |
| Discussion 3, solo | 7 | 3.9 | 2.3 | 312k | 10.0 | 136k |
| Discussion 4, solo | 1 | 4.0 | 1.0 | 285k | 7.6 | 285k |
| Plan 1, lens fan | 8 | 14.0 | 5.9 | 698k | 24.1 | 118k |
| Plan 2, solo | 8 | 3.4 | 2.1 | 504k | 16.9 | 240k |
| Plan 3, solo | 4 | 5.8 | 3.0 | 453k | 14.1 | 151k |
| Code 1, lens fan | 43 | 2.6 | 0.7 | 226k | 5.6 | 323k |

What stays fixed, by the next round's ledger:

| Perch and round | Keys the round opened | Resolved by the next round | Gating keys resolved |
| --- | --- | --- | --- |
| Discussion 1 | 123 | 123 | 67 of 67 |
| Discussion 2, where a round 3 followed | 70 | 65 | 40 of 44 |
| Plan 1 | 112 | 108 | 44 of 47 |
| Plan 2, where a round 3 followed | 11 | 10 | 8 of 9 |

Do the late rounds still find new defects?

- Discussion round 3 found at least one gating finding in 6 of the 7 runs that reached it, and a BLOCKING one in 3 of the 7 (`regression-baseline-verify`, `umbrella-anchor`, `umbrella-daemon`).
  The 7th run (`gate-runs-once`) found nothing and converged.
- Discussion round 4 ran once (`umbrella-daemon`, n = 1) and found 1 gating finding.
- Plan round 2 found a gating finding in 7 of 8 runs, BLOCKING in 3 of 8 (`gate-runs-once`, `regression-baseline-verify`, `umbrella-anchor`).
- Plan round 3 found a gating finding in 4 of 4 runs, BLOCKING in 3 of 4.
- Round 2's gating count does not predict round 3's: the seven Discussion pairs (round 2 gating to round 3 gating) are 7 to 2, 6 to 2, 8 to 0, 11 to 3, 10 to 1, 8 to 6 and 5 to 2.

What the lens fan adds, against a solo reviewer:

- Discussion round 2 kept 60 gating findings across 8 runs; 57 came from a lens and 3 from the lead's own focus pass (95%).
  Round 1 kept 67 gating findings: 62 from lenses, 5 from the lead (93%).
- Plan round 1 kept 47 gating findings across 8 runs: 27 from lenses (57%), 20 from the lead.
- The lead's focus pass is the nearest thing in the records to a solo round: it kept 18 Discussion findings (8 gating) over 16 fan rounds, 1.1 per round.
- The forks are 26% of a Discussion round-2 review's fresh tokens (77k of 301k, mean over 8) and 16% of a Plan round-1 review's (73k of 449k).
  A fan round costs less per gating finding than the solo rounds after it, but the records hold no solo round run on the same text, so the gap mixes the fan with a record that is already cleaner.
- The code fan finds no gating finding in 28 of 43 first rounds (65%), and no finding at all in 13 of 43.

Recommendation, not a decision:

- Keep the Discussion fan in rounds 1 and 2 and the Plan fan in round 1; keep a solo Discussion round 3 and solo Plan rounds 2 and 3 in the heavy profiles.
  No stopping signal in the sample says a round can be skipped: round 2's gating count predicts nothing about round 3's.
- Make Discussion round 3 and Plan round 2 the last rounds a light profile pays for; stop a light profile there, and run a second solo Plan round only when Plan round 2 kept a BLOCKING finding (3 of 8 runs).
- Default Code to a solo review in the light and docs profiles and keep the fan for the heavy profile.
  The records support this only through cost per gating finding (323k fresh tokens) and the 65% empty rounds, not through a measured solo code round.

## 2. Lenses

### twill-plan (writer-side, n = 8 runs, 293 items)

| Lens | Items | Fixed | Rejected | BLOCKING | MEDIUM | Raised with another lens |
| --- | --- | --- | --- | --- | --- | --- |
| coverage | 114 | 109 | 5 | 2 | 61 | 2 |
| forced-edits | 52 | 50 | 2 | 16 | 14 | 0 |
| row-validity | 60 | 58 | 1 | 2 | 32 | 3 |
| way-forward | 39 | 39 | 0 | 2 | 20 | 2 |
| guard-reach | 28 | 28 | 0 | 3 | 8 | 2 |

All five lenses clear 95% fixed and share almost nothing (9 of 293 items name another lens).
`forced-edits` leads on BLOCKING (16 of 52); `coverage` leads on volume, with 51 of its 114 items LOW.

Recommendation: keep the five; no merge or drop is supported by overlap or noise.

### twill-discussion (writer-side, n = 8 runs, 336 lens slots)

| Lens | Items | Fixed | Rejected | BLOCKING items | Gating items fixed | Raised with another lens |
| --- | --- | --- | --- | --- | --- | --- |
| bound-breaker | 69 | 66 | 2 | 9 | 20 | 7 |
| existing-paths | 43 | 41 | 2 | 5 | 12 | 3 |
| interactions | 57 | 57 | 0 | 5 | 13 | 6 |
| walkthrough | 37 | 36 | 1 | 7 | 12 | 7 |
| goal-scope | 46 | 35 | 9 | 0 | 6 | 4 |
| board-fit | 38 | 24 | 14 | 0 | 7 | 1 |
| leanness | 46 | 41 | 3 | 0 | 4 | 1 |

The writer's lists give a severity for 35 to 58% of items per lens, so the BLOCKING and gating columns undercount.
`board-fit` and `goal-scope` carry the noise (14 of 38 and 9 of 46 rejected, against at most 3 per lens for the other five) and produced no BLOCKING item.
Only 29 lens slots of 336 (9%) sit on an item another lens also raised.

Recommendation: merge `goal-scope` and `board-fit` into one board-and-scope lens that reports only what contradicts the board entry or another entry, and keep the other five.
That makes six lenses, not the four the 2026-10-22 baseline proposed: `bound-breaker`, `existing-paths`, `interactions` and `walkthrough` carry every BLOCKING item, but `leanness` fixed 41 of 46 items, and 18 of the 22 that carry a severity are LOW trims of the record.

### review-discussion (n = 16 fan rounds in 8 runs)

| Lens | Rounds run | Rounds with a gating finding | Rounds with BLOCKING | Kept findings | Dropped | Folded into another |
| --- | --- | --- | --- | --- | --- | --- |
| bound-breaker | 16 | 16 | 8 | 71 | 5 | 1 |
| interactions | 16 | 15 | 9 | 59 | 2 | 5 |
| existing-paths | 16 | 15 | 4 | 38 | 1 | 1 |
| walkthrough | 16 | 9 | 3 | 31 | 1 | 2 |

`goal-scope` (6 rounds, 1 gating) and `board-fit` (4 rounds, 1 gating) ran only in the earliest fans of the sample and found little, which matches the twill-side result.
Only part of the reviews carry a `## Rejected` list, so the dropped and folded counts are a floor.

Recommendation: keep the four.
`walkthrough` is the weakest (gating in 9 of 16 rounds) but still raised 3 BLOCKING rounds; nothing in the sample supports dropping it.

### review-plan (n = 8 fan rounds in 8 runs)

| Lens | Rounds run | Rounds with a gating finding | Rounds with BLOCKING | Kept findings | Dropped |
| --- | --- | --- | --- | --- | --- |
| affected-set | 8 | 6 | 2 | 15 | 2 |
| guard-reach | 8 | 6 | 3 | 25 | 7 |
| way-forward | 8 | 4 | 0 | 31 | 7 |

`guard-reach` and `way-forward` share names with twill-plan lenses, whose findings the writer had already fixed (28 and 39 items) before the review ran.
`way-forward` yields the least: 8 gating of 31 kept, no BLOCKING, 7 dropped.
`guard-reach` still raised 3 BLOCKING rounds after the writer's own pass.

Recommendation: replace `way-forward` with a lens the writer fan lacks, aimed at repository invariants a card breaks without touching the line that breaks (test seams and tier tags, `doc.go` and PATTERN text a card falsifies, card order); question 3 and 4 show these as what plan rounds 2 and 3 find late.
Keep `guard-reach` and `affected-set`.
This is the 2026-10-22 baseline's "give `review-plan` the lenses the writer fan lacks" proposal, now with a named replacement.

### review-code (n = 58 fan rounds across the code-reviewed runs)

| Lens | Rounds with a finding | Rounds with a gating finding | Rounds with BLOCKING | Kept findings | Dropped |
| --- | --- | --- | --- | --- | --- |
| code-correctness | 23 | 11 | 2 | 43 | 3 |
| code-affected-set | 23 | 8 | 0 | 28 | 4 |
| code-tests | 29 | 4 | 0 | 48 | 9 |
| code-pattern-fit | 13 | 3 | 1 | 17 | 1 |

The baseline's proposal to fold `code-affected-set` into `code-pattern-fit` is not supported: `code-affected-set` found a gating finding in 8 rounds and `code-pattern-fit` in 3.
4 of the 5 gating `code-pattern-fit` findings are stale prose a change left behind (a help example naming a removed verb, tier-tag rules and a PATTERN entry no longer true), which is an affected-set question.
`code-tests` is the noisiest: 48 kept findings with 4 gating rounds, and 9 dropped (16% of what it reported).

Recommendation: fold `code-pattern-fit` into `code-affected-set` (reverse of the baseline's proposal), and narrow `code-tests` to missing coverage of changed behavior so it stops reporting style.
The sample is 3 gating rounds for `code-pattern-fit`; treat the fold as low-confidence.

## 3. Finding classes

Gating findings in Discussion and Plan reviews, all rounds (n = 220: 144 Discussion, 76 Plan):

| Reviewer class | Discussion | Plan |
| --- | --- | --- |
| design | 122 | 37 |
| consistency | 17 | 21 |
| decision | 5 | 4 |
| scope | 0 | 14 |

By keyword theme (`classify-findings.sh`, a heuristic that gives each finding one theme, first match wins):

| Theme | Discussion | Plan | Cheapest stop |
| --- | --- | --- | --- |
| design-interaction (no keyword; state, order, race and ownership defects) | 55 | 39 | the reviewer: no check or rule reaches it |
| unbounded edge or guard | 27 | 3 | a writer rule exists in the discussion spec; `bound-breaker` catches the rest |
| code claim wrong | 17 | 4 | a machine check in `validate-discussion` that every cited path and symbol exists; the rest stays with `existing-paths` |
| coverage gap (no acceptance line, card or row) | 14 | 7 | a machine check: every Decision id appears in an acceptance line; every acceptance line maps to a card |
| refusal without a way forward | 11 | 4 | a machine check that a refusal sentence names a command |
| PATTERN rule | 9 | 5 | an input the writer lacked: the `pattern/` background file of each entry a card touches |
| test seam or tier tag | 6 | 6 | a machine check in `validate-plan` |
| docs sync | 1 | 3 | a machine check in `validate-plan` that a card falsifying a package behavior lists its `doc.go` under Extends |
| cross-section inconsistency | 3 | 0 | the reviewer |
| card order or dependency | 1 | 5 | the reviewer |

The "cheapest stop" column is the writer's judgement of where each theme could be stopped; no check was built or tried.
About 43% of gating findings (94 of 220) match no keyword, and a sample of them is state-and-order defects of the kind only a reader of the whole design finds.

Recommendation:

- Add to `validate-plan` the checks the records show recur at Plan rounds 2 and 3: a card that adds a field to a shared `Deps` or `Env` struct lists the reflective seam tests; a new test file carries exactly one tier tag; a card whose Intent falsifies a package behavior lists its `doc.go`.
  The 16 gating Plan findings in the three machine-checkable themes (test seam, docs sync, coverage gap) are the ceiling of what these checks stop.
- Add to `validate-discussion` a citation check (every cited path and symbol exists) and an acceptance check (every Decision id appears in an acceptance line).
  The 31 gating Discussion findings in the code-claim and coverage themes are the ceiling.
- Leave the design-interaction and unbounded-edge classes (82 Discussion and 42 Plan findings) to the lenses; no cheaper stop is visible.

## 4. Fix-delta review

`delta.tsv` marks each round-2-or-later finding as touching text the previous round's fixer changed (`yes`), text it did not change (`no`), or neither (`unkeyed`).
A Discussion finding is keyed by the decision ids (`D<n>`) in its location and summary and matched against the previous fixer report; a Plan finding by its card number, matched against the cards that fixer report names.
The match is generous: a fixer report that mentions many decisions marks most findings touched, so `yes` is an upper bound on what a delta review catches.

| Perch and round | Findings | Touched | Untouched | Unkeyed | Gating untouched of gating |
| --- | --- | --- | --- | --- | --- |
| Discussion 2 | 100 | 70 | 2 | 28 | 2 of 60 |
| Discussion 3 | 27 | 19 | 0 | 8 | 0 of 16 |
| Discussion 4 | 4 | 3 | 0 | 1 | 0 of 1 |
| Plan 2 | 27 | 24 | 2 | 1 | 2 of 17 |
| Plan 3 | 23 | 9 | 14 | 0 | 8 of 12 |

- Discussion rounds 2 to 4 (n = 8 runs): 92 of 94 keyed findings touch fixed text, and the 2 that do not are `gate-runs-once` D11 (a rationale that stayed false, BLOCKING) and `beam-start-and-ownership` D10 (MEDIUM); a delta review would have caught the other 92, apart from the 37 unkeyed section-level findings (Acceptance criteria, Constraints, PATTERN changes, Open risks).
  The fixer edits those sections by rule on every round, so they are likely inside a delta; the records do not prove it.
- Plan round 2: 24 of 26 keyed findings touched; the 2 untouched are `umbrella-anchor` card 14 (a `reset --keep` outside `destroy.go`, BLOCKING) and `gate-runs-once` card 20.
- Plan round 3 (n = 4 runs): 14 of 23 findings point at text the round-2 fixer left alone, 8 of the 12 gating.
  They are repository-invariant misses in cards no earlier fix touched: reflective seam tests and tier tags (`beam-start-and-ownership` cards 14 to 16), `doc.go` files a card falsifies (`gate-runs-once` cards 15 and 17), a card that cannot pass its own gate (`beam-start-and-ownership` card 10).

Recommendation: review Discussion rounds 2 and later, and Plan round 2, as a delta: the decisions or cards the fixer changed, plus the sections that cite them.
Keep the final Plan round a full read, because that is where the untouched-text findings surface.
The records measure which findings a delta would have caught, not the tokens a delta saves, and the reviewer today still reads the whole record; the saving is unmeasured.
Moving the repository-invariant checks into `validate-plan` (question 3) removes most of what the full late Plan round finds in untouched text, which would let the delta rule apply to every round.

## Limits

- 8 full loom runs, mostly large umbrella tasks; the figures describe those and not a typical small task.
- No solo round was run on the same text as a fan round, so every fan-against-solo figure compares different record states.
- A review finding carries one origin lens, so overlap between review lenses is hidden; the twill lists show 9% overlap and the review `## Rejected` lists are incomplete.
- Delta touching and finding themes are keyword heuristics.
- Cost is fresh tokens and minutes; no dollars, since the records hold no pricing.
- Ledger keys of a perch's last round have no later verdict, so "stays fixed" is measured only for rounds a later round followed.
