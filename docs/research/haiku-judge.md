# Haiku 5.5 for the Bouncer judge, per row

Analysis step of board note `haiku-roles`, written 2026-10-10.
It decides, per Bouncer row, whether the judge's ruling is simple enough for Haiku 5.5 (`claude-haiku-5-5`, alias `haiku` in `models.yaml`) instead of Sonnet 5.5, and names the trial.
No config, code or board entry was changed.

## Verdicts

| Row | Judge verdict | Seed pass | Reason in one line |
|---|---|---|---|
| Discussion-Bouncer | keep Sonnet | Haiku-safe | Design-level key mapping decides most verdicts, it escalates most often, and an error feeds the Plan's answer key. |
| Plan-Bouncer | trial first | Haiku-safe | Findings are card-level and concrete, but the old-or-new key call still decides convergence, and a miss reaches Webster, the most expensive phase. |
| Webster-Bouncer | trial first, strongest candidate | Haiku-safe | Findings cite files, tests and PATTERN entries, most gating ones are BLOCKING and never downgraded, and the PR description and PR-Gate catch what it lets through. |

No row is "safe for Haiku" without a trial: the rule is mostly mechanical, but two parts of every ruling need judgement (below), and Go does not cross-check the verdict against the facts.
The saving is about $1.2 per task, while one wrongly bounced round costs about three times that, so the trial has to show Haiku adds no rounds.

## The rows

The shipped recipes hold three Bouncer rows, all in `contracts/recipes/loom-recipe.yaml`; `darn-recipe.yaml` and `batten-recipe.yaml` have none.
Each pairs with a `BurlerRound` row through mutual `on_stuck` (the "perch") and shares its run directory.

| Row | Segment | `artifact_paths` | Rubric stencil | Seams | On CONVERGED |
|---|---|---|---|---|---|
| Discussion-Bouncer | Discussion-Review | `_lyx/discussion/decision-record.md`, `support-log.md` | `loom-rubric-discussion-review` (49 lines) | `commit_seam: discussion`, `carry_over` | Plan-Write |
| Plan-Bouncer | Plan-Review | `_lyx/plan` | `loom-rubric-plan-review` (110 lines) | `commit_seam: plan`, `approve_seam: plan`, `skip_seam: rework-exempt`, `carry_over` | Batchifier, then Webster |
| Webster-Bouncer | Webster-Review | `_lyx/plan` (the diff is named by the rubric) | `loom-rubric-webster-review` (95 lines) | `carry_over` only | Describe, Publish, PR-Gate |

Each row runs two kinds of pass, both from `contracts/stencils/bouncer/`:

- **Seed** (`bouncer-template-seed.md`, role `bouncer-seed`): runs once before round 1, reads the rubric and every artifact in `artifact_paths`, and writes one file, `round-1-focus.md`, a list of places to look.
  It rules on nothing; the reviewer still reviews everything, and a focus entry may never cap severity.
- **Judge** (`bouncer-template-judge.md`, role `bouncer-judge`): runs after every review-and-fix round.
  It never opens the artifacts.
  It reads the facts file Go renders (`bouncerfacts.go`: per-round counts by severity and class, recurring ledger keys, keys open in an earlier round), the latest review, and the previous ledger, with the rubric for reference.
  It writes three files: the verdict (`CONVERGED`, `CONTINUE` or `CIRCLING` with a one-line quoted rationale), a lossless ledger of finding keys, and the next round's focus file.

The question it rules on is fixed, rendered by `decisionRuleMarker` in `internal/shedadapters/bouncerprompt.go`:

- A finding gates when its class is `design`; any BLOCKING finding also gates.
- Round 1: `CONTINUE` on a gating finding at MEDIUM or worse or any BLOCKING, else `CONVERGED`.
- Round 2 on: `CONTINUE` only on a BLOCKING finding or a gating MEDIUM that the judge maps to a key open in an earlier round; a gating MEDIUM on a key first raised in this round converges, and Go carries it into the decision record's `## Open risks`.
- From `review_circling_checkpoint` (3) on: `CIRCLING` when a gating finding sits on a key open in an earlier round and still open.

## How the judge's model is set

One key covers every row: `judge` in `loom.yaml` (`_lyx/config/loom.yaml`, template `internal/loomengine/template.yaml`), today `sonnet[medium]`.
`loomengine.ResolveJudge` resolves it through `models.yaml` once per process, and `shedrecipe.Env.JudgeModel`/`JudgeEffort`/`JudgeVersion` carry it to every Bouncer row, for both the seed and the judge pass.

Per row, `bouncerEntry` (`internal/shedrecipe/entries_bouncer.go`) does accept `model`, `effort` and `version` keys in a Bouncer row's recipe `config`, overriding the Env value.
That is not a config-level choice today:

- the recipe is embedded in the binary (`contracts/recipes/recipes.go`), so a per-row model is a recipe edit plus a rebuild;
- the row values are provider strings (`haiku`, `medium`), not model-specs, so they bypass `models.yaml`;
- every row's recipe comment says the key is deliberately absent so the row stays tunable from `loom.yaml`.

So the operator can switch the judge for all three rows (seeds included), not for one row.
A per-row switch needs a code change, for example per-segment `discussion_judge`, `plan_judge` and `webster_judge` keys mirroring `webster_review`.

`loom` is not a hub-wide module, so each task pair has its own `_lyx/config/loom.yaml`: a trial can be confined to one pair.

## What the judge actually decides

Most of the ruling is a fixed-list check that Haiku's benchmark result (a precise spec implemented exactly) supports:

- reading BLOCKING and MEDIUM-design counts off the facts table;
- writing three files in a strict YAML shape, with the rationale double-quoted on one line;
- carrying every previous ledger key forward as `open` or `resolved`;
- writing the focus file's `round:` as the next round.

Two parts need judgement, and they decide the verdict from round 2 on:

1. **Key mapping.**
   Whether a finding is the same defect as a key open in an earlier round, or a new one, is a semantic call across rounds, and it alone separates `CONVERGED` from `CONTINUE` for a gating MEDIUM.
   Examples from the records:
   - `webster-stability`, Discussion round 3: the judge mapped F1 (D6's way-forward names a per-file Create the plan format refuses) to a new key rather than the open `d6-rename-partitioned-declaration-underived`, because "F1 attacks the way-forward sentence, which round 2's fix introduced", and ruled `CONVERGED`.
   - `git-robustness`, Discussion round 2: "the round-1 lock-order question is answered and the race is a separate gap", so new key, `CONVERGED`.
   - `board-labels-and-skills`, `bugfix-fabric`, `shuttle-turns` and `run-recovery`, Webster rounds 3–4: hard-wrapped comments found at new sites each round were mapped onto one recurring key, giving `CIRCLING`.
2. **Relabels and departures.**
   The judge may relabel a finding's class or severity where its own review entry shows the mislabel, and must ratify or reject each `## Focus departures` entry; it also writes the next round's focus, checked against the rubric's `Do not flag` list.
   In the records relabels are rare (the rationales mostly say "no relabel"), and BLOCKING is never relabelled down.

Go does not check the verdict against the facts: `CONVERGED` over a BLOCKING row is accepted, and the carry-over into `## Open risks` is built from the judge's own ledger.
A key the judge drops or wrongly marks `resolved` is therefore lost silently, while a key it maps as new is carried and stays visible.

## Past verdicts

Source: the review records in the weft history (`git -C loomyard-weft log --all`), 88 tasks, 581 distinct verdict files from 2026-09-30 to 2026-10-10, plus 610 judge and seed transcripts under `~/.claude/projects/-home-knatte-Code-loomyard-LYXHUB-*`.
Every judge and seed pass in them ran on Sonnet 5.5 medium.
The rule changed twice in that window (48ae1b3cc on 2026-10-03: sit-rep judge with a facts file; 0ebdea27f on 2026-10-08: convergence from round 2 with carry-over), so only verdicts from 2026-10-08 on (29 tasks) show today's rule.
Earlier verdicts use `APPROVED`/`BLOCKING`, and some earlier `CIRCLING` verdicts follow a retired rule ("the gating count rose … which the decision rule reads as no progress").

Verdict files from 2026-10-08 on, counting each distinct version:

| Segment | CONVERGED | CONTINUE | CIRCLING | Escalations (cause) |
|---|---|---|---|---|
| Discussion | 14 | 56 | 7 | 14 (budget) |
| Plan | 26 | 44 | 2 | 7 (budget) |
| Webster | 44 | 36 | 2 | 2 (circling) |

Findings:

- **No overturned verdict is recorded.**
  Of 44 escalation briefs over the whole window, the parent ended the segment with `accept` in most and ran one more round with `continue` in nine (`board-labels-and-skills`, `board-model`, `bugfix-fabric`, `driver-context`, `loom-bouncer-rounds` twice, `skills-in-one-turn`, `test-suite-weight`, `webster-replan-state`), three of them under the retired circling rule; none of the decision records says the judge misruled.
  An `accept` at budget agrees with a `CONTINUE` judge that open findings remain, and only decides they may pass.
- **No mechanical failure shows in the surviving records.**
  No facts file carries a parse-error row, and the logs of the two live task pairs carry no degraded or re-judged pass; logs of torn-down pairs are gone, so the rate is unknown, not zero.
- **Hub interventions concentrate on Discussion-Review.**
  Fourteen of the 23 escalations since 2026-10-08 came from it, against seven from Plan-Review and two from Webster-Review.
  That is the segment where a judge error most often turns into a parent fork.
- **Webster-Review rulings are the most mechanical.**
  Its gating findings are mostly BLOCKING because they cite a PATTERN entry or a failing test, and a BLOCKING finding gates without any mapping; its `CIRCLING` cases are one defect family at new sites.
- **Plan- and Discussion-Review rulings lean on the key mapping.**
  Twenty-one of the recent `CONVERGED` rationales rest on "a key first raised in this round", and the Discussion ones map design questions, where a rewritten decision is often a fix and a new defect at once.

## Cost per pass

Sonnet transcripts (435 judge and 167 seed passes, 55 tasks), deduplicated by message id; prices from `docs/benchmarks/haiku-5-5.md`, 5-minute cache writes at 1.25× input (the judge's TTL in `shuttle.yaml`).

| Pass | Calls | Cache write | Cache read | Output | Largest context p50 / p90 / p99 | Cost per pass |
|---|---|---|---|---|---|---|
| Judge | 6.0 | 34k | 163k | 3.0k | 41k / 45k / 55k | $0.13 |
| Seed | 6.7 | 29k | 160k | 2.1k | 37k max 62k | $0.11 |

That is about $1.38 per task for all judge and seed passes, about $76 across the 55 tasks.
The same tokens on Haiku 5.5 cost about $0.08 per task; with four times the output and a doubled context, as in the benchmark, about $0.15, still under the 100k price tier at p99.
One Opus review-and-fix round costs about $3.5 before output in a recent `round-N-usage.yaml` record (`_lyx/reviews/plan/round-2-usage.yaml`, 2026-10-10), so one extra round per task costs more than the whole saving.
`round-N-usage.yaml` has no judge or seed half, so the judge's cost is measured from transcripts only.

## What Haiku 5.5 is known to do

From `docs/benchmarks/haiku-5-5.md`: on one fully specified coding task Haiku 5.5 medium matched Sonnet and Opus (66/66, no fuzz deviation) at about a seventh of Sonnet's cost, took about twice as long, wrote about four times the output and grew its context to 74–132k.
It did not measure review, ambiguous requirements or long loops, and the benchmark itself names the judge as a candidate only after its verdicts are compared against Sonnet's on the same rounds.
In use, a Haiku 5.5 driver once ended its first turn asking to start (`bolt-and-store`, fixed in 189fd59c8); a judge pass that ends without its three files is discarded and re-judged, spending budget.

## Per row: cost of a wrong verdict

| Row | Wrong CONVERGED lets through | Wrong CONTINUE or CIRCLING sends back | Verdict |
|---|---|---|---|
| Discussion-Bouncer | A design flaw becomes the Plan's answer key; the Plan-Review rubric keeps carry-over out of that key, so Plan-Review checks the plan against the flaw, not the flaw itself. | One Opus round (about $3.5 and 10 min), or a parent escalation fork, on the segment that already escalates most. | keep Sonnet |
| Plan-Bouncer | A card defect reaches Webster, the most expensive phase; Webster-Review sees only the code against the cards, and a carried key becomes a check-by-hand item in the PR description. | One Opus round or a budget escalation; Plan-Review escalated seven times since 2026-10-08. | trial first |
| Webster-Bouncer | A code defect reaches Describe and the PR; a carried key becomes a check-by-hand item in the PR description, and PR-Gate is the operator's review. A dropped ledger key is silent. | One review round (`webster_review: sonnet[medium]`) plus an Opus fix and a verify gate run. | trial first |

The seed pass of each row writes only a focus list, never caps severity and never gates; a weak focus costs round-1 depth, not a wrong outcome, so it is Haiku-safe.
It shares the `judge` key with the judge pass, so it moves only with it.

## Trial

### What to switch

Config can only switch all three rows at once, so the first live run switches all three, seeds included, and is read per row:

- In the trial task's own pair, before `lyx loom start`, run `lyx config loom --set 'judge=haiku[medium]'`.
- Do not set it in the prime: other pairs keep `sonnet[medium]` and are the comparison.
- `haiku[medium]` is written out although the `models.yaml` alias already defaults to medium, as the benchmark advises.
- Pick a task with a real Webster phase (several cards), so all three segments run at least two rounds.

Discussion-Bouncer runs on Haiku in this trial only because no config isolates it; its verdicts are read but do not count toward switching it.
To trial Plan and Webster alone, the recipe would need `model: haiku` and `effort: medium` on those two rows, or per-segment judge keys: a code change, out of scope here.

### Check every Haiku verdict on the run

The hub reads each Haiku round's facts, review, previous ledger and the three written files, and records per round:

- whether the verdict follows from the facts (a BLOCKING row with `CONVERGED` is a defect let through);
- whether every previous ledger key reappears;
- each old-or-new key mapping, and whether Sonnet's reading would differ;
- whether the focus file kept its round and was not replaced by the fallback;
- any degraded pass, re-judge, missing file, or turn that ended on a question.

### Numbers to compare against the Sonnet runs since 2026-10-08

| Measure | Sonnet baseline | Source |
|---|---|---|
| Verdicts later overturned: parent `continue` after `CIRCLING`, a later segment or PR-Gate raising a defect the judge closed or dropped | none recorded | escalation briefs, circling decisions, decision record, PR-Gate findings |
| Judged rounds to settle, per segment | Discussion mostly 2–3, Plan 2–3, Webster 1–4 | `round-N-bouncer-verdict.md` count per segment |
| Verdict mix per segment | table above | verdict files |
| Escalations and parent decisions per segment | Discussion 14, Plan 7, Webster 2 over 29 tasks | `round-N-escalation.md`, `round-N-circling-decision.md` |
| Hub interventions | orch messages, parent forks, `resume`/`goto` | orch and pair logs |
| Judge and seed tokens per pass: calls, cache write, cache read, output, largest context | judge 6 calls, 34k, 163k, 3.0k, 41k; seed 6.7, 29k, 160k, 2.1k, 37k | pair transcripts, deduplicated by message id |
| Cost per task for judge and seed, and passes crossing 100k | $1.38, none | transcripts at the price table |
| Wall time per judge pass | measure on both | transcript timestamps |

Switch a row only when its Haiku verdicts match the hub's reading on every round and the run adds no round or escalation that Sonnet would not have; one run is one sample, so repeat on a second task before switching.
A Go check that refuses `CONVERGED` over a BLOCKING facts row, and a ledger that drops a previous key, would make any judge model safer, and is worth its own board entry.
