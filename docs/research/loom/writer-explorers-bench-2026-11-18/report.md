# Writer explorers bench: Discussion and Plan writers with and without explorer subagents

2026-11-18, board task `plan-writer-explorer-forks`.
One reference, two arms, one run each; a direction from one sample, not an effect size.
The tables record one run on one machine and are not claiming to stay true.

## Question

The Discussion-Write and Plan-Write stencils tell the writer to hand its exploration and lookups to fresh subagents through the Agent tool.
They are not the twill forks, which stay.
Do the explorers pay for themselves in cost, time and artifact quality?

## Share of the writers' spend

`share.sh` reads the Plan-Write and Discussion-Write transcripts of the landed tapestry runs in `slugs.txt` and splits each by the `agentType` of the subagent's meta file:
`fork` is a twill or lens fork, `general-purpose` or `Explore` an explorer or lookup subagent.
The runs are every landed tapestry run since 2026-11-03 that has transcripts on this machine, bench pairs left out.
The earlier "21 runs" count of `tools/tokencount` is not reproduced: this table counts the runs of `slugs.txt`.
Prices are the list prices of `internal/shuttleengine/claudeengine/pricing.go` as the earlier lookups measure used them, so the totals differ from `tokencount`'s.

| Writer | Runs | Writer itself | Forks | Explorers (subagents) | Total | Explorer share of the total | Explorer share of forks plus explorers |
|---|---|---|---|---|---|---|---|
| Discussion-Write | 7 | $20.80 | $12.07 | $3.22 (17) | $36.08 | 8.9% | 21% |
| Plan-Write | 23 | $141.06 | $158.90 | $18.62 (82) | $318.58 | 5.8% | 10% |

- `tokencount`'s "Plan writer forks" line counts the forks and explorers together; the explorers are about a tenth of it.
- Raw rows: `share-rows.tsv`; the table above is `share-totals.tsv`.
- Three Plan messages ran on a model with no list price and count as zero.
- Only seven runs have a Discussion-Write session, since later runs skipped it.

## Bench

### Reference

`pawl-checklist-hidden-in-beam`, weft tag `archive/pawl-checklist-hidden-in-beam/fb0f145ec02c`, base commit `f98a1ba0`.
It is the smallest landed tapestry run with both a Discussion review and a twill Plan with its review.
The grading keys are in `keys.md`, committed before any arm ran.
The board entry body each arm started from is the reference's, from the bolt repository at `08c5342d1b`.

### Arms

Both arms are full runs from a draft board entry through Discussion-Write, its review and approval, and Plan-Write with twill on, each paused before Plan-Bouncer.
Each Plan therefore rests on its own arm's decision record.
Pairs `wxb-plan-with` and `wxb-plan-without` started from `f98a1ba0`, with the hub's board stencils pinned at the run's first start.
The without arm was started on board copies patched with `no-explorers.patch`, which the orch restored right after the pin.
Pins differ for `loom-template-discussion` and `loom-template-plan` only.
Model and effort are the hub defaults.

The orch ran the pairs' decisions: both Discussion reviews escalated that commit `7c23591`, which fixes the same bug and sits on the parent branch but not on the base, contradicts the design.
The recorded answer was to ignore it and design against `f98a1ba0`.
The with arm accepted its round; the without arm continued into a second round.

### Cost and time

Cost covers the writer session, its twill and lens forks, and its explorer subagents, priced as in `share.sh`.
Review and fix sessions are not counted.
Wall time runs from the row's start to its end in the run's status history.

| Measure | Discussion with | Discussion without | Plan with | Plan without |
|---|---|---|---|---|
| Writer session | $1.81 | $1.75 | $2.68 | $2.36 |
| Forks | $1.60 (6) | $1.15 (6) | $2.91 (6) | $2.86 (6) |
| Explorers | $0.00 (0) | $0.42 (4) | $0.38 (3) | $0.14 (1) |
| Total | $3.40 | $3.31 | $5.97 | $5.37 |
| Wall time | 9m45s | 7m34s | 13m05s | 10m53s |

- The with arms spent $9.37 and the without arms $8.68 on writers, forks and explorers; the explorers' own spend is $0.38 against $0.56.
- The without arm's writers still launched explorers, four in the Discussion and one in the Plan, though the stencil lines were gone.
  The with arm's Discussion writer launched none though its stencil told it to start with explorers.
  The stencil line therefore did not decide whether a writer used explorers in this sample.
- Round 2 of the without arm's Discussion review, its fix and its second bouncer pass are outside the figures above.
  The orch's own spend on the pairs and the review and fix sessions are not measured here; the writer sessions' total of about $18 leaves the $40 cap far from reached.

### Grading

Grades are R repeated, P partly, A avoided, as in the 2026-11-12 plan bench.
The Discussion keys grade the record as the writer wrote it, before any review; the Plan keys grade the plan before Plan-Bouncer.

| Key | With | Without |
|---|---|---|
| DK1 old-build driver bound | R: the record's Out line names an older build that dropped the strand, never a live old driver | R: no old driver named; a race with `lym loom start` is named instead |
| DK2 viewed-window-ending criterion | A, by design: no second overview window, the viewer ends on the beam overview | A, by design: no reed change |
| DK3 tie focus consequence | A, by design: no order key | A, by design: no order key |
| PK1 both `docs/overview.md` statements | A: the orch bullet and the `lym beam start` paragraph | A: the orch and batten entries |
| PK2 comment sweep of one-overview statements | A, by design: reed is untouched | A, by design: reed is untouched |
| PK3 `IfAbsent` relaunch row | A: no relaunch row | A: no relaunch row |
| R / P | 1 / 0 | 1 / 0 |

Neither arm reproduced the reference's design of several overview strands.
Both put the runs strand in the overview window and the checklist in a window of its own, because reed on the base refuses a second overview strand and the orch's decision pointed them at the base.
Five of the six keys defend a defect of the reference's own design, so they did not discriminate between the arms.
Round 1 of the Discussion review found 6 findings for the with arm (1 BLOCKING, 4 LOW, 1 NIT) and 9 for the without arm (2 BLOCKING, 3 MEDIUM, 3 LOW, 1 NIT); round 2 of the without arm raised one new BLOCKING finding.
The designs differ, so the counts are not a measure of the explorers.

## Recommendation

Keep the explorers; this bench gives no reason to drop them from either writer.

- Cost: the explorers are about 6% of the Plan-Write spend and 9% of the Discussion-Write spend over the landed runs, and $0.14 to $0.42 in the bench arms; removing the lines saves less than a dollar a run.
- Behavior: the stencil lines did not control the writers in this sample, so removing them would not remove the explorers.
- Time: the without arms' write rows were 2m11s and 2m12s shorter, one sample each, with different designs.
- Quality: the keys did not discriminate, so no quality effect is shown either way.

A bench that could say more needs a reference whose design the arms reproduce, or keys drawn from findings that do not depend on one design, and several runs per arm.

## Deviations and limits

- One reference, one run per arm, one grader; five of the six keys did not apply to either arm's design.
- The first four pairs were set up for four arms; the orch removed the two Discussion-only arms after no verb could skip Discussion-Write for the plan pairs, and removed `wxb-disc-with` by mistake.
- The pair branches were reset to `f98a1ba0` and force-pushed to origin, since a run's sync with origin undid a local reset.
- The review of each arm saw the parent commit `7c23591` and escalated it, which a hub with no such commit would not.
- Review and fix sessions are not priced; the 21-run count of `tokencount` is not reproduced.

## Files

- `keys.md`: the grading keys, committed before any arm ran.
- `no-explorers.patch`: the stencil edits of the without arms, against `main` at the time of the bench.
- `share.sh`, `slugs.txt`, `share-rows.tsv`, `share-totals.tsv`: the share measurement.
- `arms-rows.tsv`: the bench arms' transcripts, priced by the same script.
