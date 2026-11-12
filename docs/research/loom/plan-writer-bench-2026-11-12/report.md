# Plan writer bench: twill against a solo Opus and a solo Fable on one plan

2026-11-12, board task `plan-writer-context`.
One reference, three arms, one plan each; a direction from one sample, not an effect size.

## Reference

`machine-name-verb`, the most recent landed tapestry run whose Plan-Write was a twill plan (it carries lens reviews).
Its records are the weft tag `archive/machine-name-verb/fdb791c78e4b`: the Discussion record and the run's usage and review files.
The base commit is `0604f2ea`, the commit that landed the task it depends on, 11 seconds before the run began.
The record's `Takes` line names that task's functions, and both new pairs checked out `0604f2ea`.

## Arms

| Arm | Plan writer | Directive and forks | Source of its figures |
|---|---|---|---|
| A, twill | `claude-opus-5-5` (the landed run, default `plan: opus[medium]`) | twill lens forks, then Plan-Burler review rounds | the tag's `usage/session-*.json` and `status.json` |
| B, solo Opus | `claude-opus-5-5`, `opus[high]` | none | pair `pwc-opus`, `go run ./tools/tokencount` |
| C, solo Fable | `claude-fable-5-2`, `fable[medium]` | none | pair `pwc-fable` |

B and C ran once each from the frozen Discussion record on the base commit, as hand-seeded pairs stopped after Plan-Write, with `plan_lenses` empty.
The orch created and ran the two pairs, because `lym fabric add` is refused from a task session.
Records show the models only; the efforts are as the orch's helper set them.
The B writer made lookup subagent calls of its own (15 Sonnet messages, $0.38 of its total): they are the writer's own, not twill forks.

## Cost and time

Wall time runs from the start of the Plan-Write row to its end.
For C it leaves out 31m19s in which the plan session waited for an orch line the orch forgot to send (the gap in its transcript, 17:29:19Z to 18:00:38Z).
Turn time is the writer's working time, from `tokencount`.

| Measure | A twill: Plan-Write | A twill: whole Plan phase | B solo Opus | C solo Fable |
|---|---|---|---|---|
| Plan cost | $4.86 | $12.41 | $2.87 | about $6.73 (estimate) |
| Plan rounds (review rounds after the write) | 0 | 2 | 0 | 0 |
| Plan wall time | 14m13s | 41m11s | 7m19s | 9m00s |
| Writer turn time | 14m02s | | 6m43s | 6m12s |
| Plan cards | 4 | 4 | 5 | 5 |

- A's whole Plan phase is Plan-Write, three Plan-Bouncer sessions, two Plan-Burler-Review sessions and two Plan-Burler-Fix sessions: $12.41 from 10:51:20Z to 11:32:31Z.
  `tokencount` read none of A's Plan sessions on this machine, so A's dollars are the `cost_usd` of the tag's usage records.
  B's total is its two Plan-Write usage records ($0.48 and $2.39).
  The first starts at 17:22:37Z, before the row's 17:25:04Z start; the wall time above starts at the row.
- C's dollars are an estimate.
  `lym` recorded `cost_usd` null for the Fable sessions, and `claude-fable-5-2` is not in `internal/shuttleengine/claudeengine/pricing.go`.
  The $6.59 for the two Plan-Write sessions uses the `claude-fable-5-1` list price ($10 input, $12.50 cache write, $0.25 cache read, $50 output per million) on the recorded token counts, with every cache write at the five-minute rate.
  The $0.14 is C's Sonnet gate fixer, which `tokencount` priced.
  B ran no gate fixer.
- A's plan-phase gate ran with no attempt, as for B and C.

## Grading

The nine keys are fixed in `keys.md`, committed before any solo plan was read.
They are the findings of A's own Plan review, round 1 (L1 to L6) and round 2 (L7 to L9).
R repeated, P partly, A avoided; the plans are the files under `_lym/plan/` of each arm.

| Key | Severity in A's review | A landed plan | B solo Opus | C solo Fable |
|---|---|---|---|---|
| L1 helpers tested directly, rows repeat verb rows | BLOCKING | A | A: only verb-level tests | R: `TestNextFile` and `TestSweptHubs` on unexported functions; its rows repeat `TestNameSet` rows |
| L2 competing-write subtest in `boardengine` | BLOCKING | A | A: none in `boardengine` | R: a `BeforePush` subtest reports `boxed` lost, as `CompetingWriteLosesToOrigins` does |
| L3 Takeover docs false after the replace set | MEDIUM | A | P: reworks the `doc.go` passage only | P: adds the replace set to `SetOwners` and `doc.go`, keeps the Takeover sentences |
| L4 `unpushed` read error without outcome | LOW | A | R: no outcome | A: a failed read fails the hub, no write |
| L5 failed-hub way forward on an untestable condition | LOW | A | A | A |
| L6 seam count in prose | NIT | A | A | R: "four seams", "two seams" |
| L7 hostname-name export without a bound, comparative comment | LOW | A | P: bound missing, comment standalone | P: bound stated, comment names `Name` |
| L8 row without input repeating a sibling | LOW | A | A | A |
| L9 tier 1 subtest repeats tier 0 | LOW | A | R: `TestName_Sweep` re-runs a failed hub on real hubs | A: re-run at tier 0, tier 1 pins origin-held owners |
| **R / P** | | 0 / 0 | **2 / 2** | **3 / 2** |

- A's column is the plan after two review rounds and their fixes.
  The keys come from A's own findings, so A scores clean by construction: its first draft carried L1 to L6, and the draft after round 1 carried L7 to L9.
  The comparison that means something is B against C, and each against the cost A paid to reach its clean plan.
- B carries no BLOCKING key: its repeated keys are LOW (L4, L9) and its partial ones MEDIUM (L3) and LOW (L7).
  C carries both BLOCKING keys, L1 and L2, the test-economy findings that A's review rounds removed.

## Verdict

- Best plan: A's landed plan, by construction of the keys; between the solo arms, B (2 R and 2 P, no BLOCKING key) over C (3 R and 2 P, both BLOCKING keys).
- Cheapest: B, $2.87 and 7m19s, 59% of A's Plan-Write cost and half its wall time.
- Twill earns its cost on this sample only if a defect in the plan costs more downstream than finding it costs.
  The lens forks and two review rounds cost $7.55 and 27m on top of A's writer.
  They cleared the four keys B kept (L3, L4, L7, L9), all LOW or MEDIUM, and the five C kept (L1, L2, L3, L6, L7), two of them BLOCKING.
  This bench cannot say what a Webster run would have paid for the defects left in B's plan.
  A solo Opus plan followed by the same review rounds is the arm this bench did not run.
- Solo Fable cost about $3.86 more than solo Opus and made the worse plan on these keys.

## Deviations and limits

- One reference, one run per solo arm, one grader; the keys are A's own findings, which favour A.
- C's cost is an estimate from a neighbouring model's price; its wall time has 31m19s taken out on the orch's report of a forgotten line.
- B's first Plan-Write session started before the row's recorded start; its $0.48 is in B's total.
- The orch's helper, not this task, created, ran and tears down the pairs; no repeat run was made, and the cost cap of $15 held (about $9.60 for B and C).
- `tokencount` found no Plan transcripts for A on this machine, so A's figures come from the tag's usage records.
  Those records do not say whether A's Plan-Write cost includes its lens forks.
