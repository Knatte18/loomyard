# Model downgrade per role: what was tried and what was decided

Closing report for board note `model-downgrade`, which has been removed from the board.
It records what was tested, what came out of it and what the hub config is on 2026-10-15.
Every measurement is stated as it was measured on its date.

## The idea

On 2026-10-13 the operator noted that lyx spawns many agents a day, and most roles run on Opus or Sonnet.
The goal was to find roles that can move to a cheaper model (Sonnet 5.5 or Haiku 5.5) without losing quality.
The planned first step was a bench: replay landed runs with one role on a cheaper model, run each arm more than once, and change no model setting until the bench shows it holds.

## The study (2026-10-13)

`docs/research/model-downgrade.md` lists every role with its model, where the model is set, its cost share over the six runs in `docs/research/token-usage-by-role.md`, a proposal and the risk.
Its savings are estimates from those six runs, not measurements.

- Cost is dominated by cache reads in long sessions.
  Sonnet 5.5 reads cache at the same price as Opus ($0.20 per million), so moving a role from Opus to Sonnet saves about 30-40% of that role's cost.
  The large saving is Haiku.
- The hub's driver already ran on `haiku[medium]` and cost about $0.20 over the six runs.

Ranked candidates (savings estimated over the six runs):

| # | Candidate | Estimated saving | Status |
|---|---|---|---|
| 1 | Webster Master to `haiku[high]` | about $30 | rejected, not tried |
| 2 | Plan-Review and Discussion-Review reviewers to `sonnet[high]` from a later round | about $10 | acted on: per-round lists in the hub default since 2026-10-15, not measured |
| 3 | Bouncer judge and seed to `haiku[medium]` | about $7.5 | Webster-Bouncer judge replayed (partial); judge stays `sonnet[medium]` |
| 4 | Plan and Discussion fixers to `sonnet[medium]` | about $4 | not tried; moved to `burler-fixer-fork` |
| 5 | Fork the fixer from the reviewer | part of $1.9 per run | not built; moved to `burler-fixer-fork` |
| 6 | Small items: conflict resolver, friction, describe, driver template default | under $1 each | not acted on |

The study kept Opus for plan, discussion, arbiter, alignment and recovery.

An earlier analysis, `docs/research/haiku-judge.md` (2026-10-10, note `haiku-roles`), had looked at the judge row by row.
Its conclusions:

- The seed pass is Haiku-safe on every row.
- Discussion-Bouncer's judge stays on Sonnet.
- Plan-Bouncer needs a trial first.
- Webster-Bouncer is the strongest candidate.
- One wrongly bounced round costs about three times the per-task saving of about $1.2, so a trial has to show that Haiku adds no rounds.

## Per-round review models

Hub default since 2026-10-15, chosen by the operator:

- `plan_review: ["opus[medium]", "sonnet[high]"]`: Opus in round 1, Sonnet from round 2.
- `discussion_review: ["opus[medium]", "opus[medium]", "sonnet[high]"]`: Opus in rounds 1 and 2, Sonnet from round 3.

The last entry of each list serves every later round.
The split follows the operator's views of 2026-10-15:

- Discussion is where to invest, because a watertight Discussion saves the most later.
  This is the operator's experience from Millhouse.
- The plan matters less.
- `sonnet[high]` can review from round 3.

`discussion_max_bounces` is 2 in the hub default, so a Discussion-Review run reaches its Sonnet round only past the budget, on a parent `continue`.

This change has not yet been measured on real runs.
No bench compared these lists against all-Opus review.
The risk named in the note still has to be checked: round 1 finds the most, so a weaker round can push findings into later rounds that have less budget left.

For context on review models, the 2026-10-15 Discussion-Write bench (`docs/research/twill-discussion-fork0-bench.md`) kept today's twill `opus[medium]`.
The critical-fixer arm and the fork0 arm each cost more and repeated more key items.

## Bouncer judge on Haiku

The operator approved a bench of the Webster-Bouncer judge on Haiku.
On 2026-10-15 it was replayed on archived rounds with the same inputs Sonnet had, and stopped after 7 of 25 cases.
It stopped because every replay window took focus from the orch pane.
The evidence was kept in the prime's git-ignored scratch, not in the repository.

Result: Haiku agreed with Sonnet on 4 of 7.
The agreeing cases were `hibernate-trigger-r1`, `loom-bouncer-rounds-r4`, `bugfix-fabric-r4` and `bugfix-webster-r3`.

| Case | Sonnet | Haiku | Parent's decision |
|---|---|---|---|
| `bugfix-fabric-r3` | CIRCLING | CONTINUE | continue (round 4 circled again and was accepted) |
| `bugfix-sessions-r4` | CIRCLING | CONTINUE | accept |
| `run-pushes-r3` | CIRCLING | CONTINUE | accept |

All three misses follow one pattern.
Haiku read fixed sites and a narrower finding count as progress.
It also did not count BLOCKING findings outside the design class toward circling.
The decision rule says a BLOCKING finding gates and decides circling on its own, so Sonnet followed the prompt and Haiku did not.

Measured cost per pass:

| Model | Cost per pass | Notes |
|---|---|---|
| Haiku | $0.006-$0.012 | 4-6 calls, largest context 42-52k tokens |
| Sonnet | $0.10-$0.19 | includes one skill-loading turn the replays lacked |

That makes Haiku about 15 times cheaper per pass.
A wrong CONTINUE, however, buys an extra review and fix round worth several dollars, so one miss outweighs many cheap passes.

Operator decision, 2026-10-15: the judge stays `sonnet[medium]`.

Operator view, not a decision: the judge's inputs, the review and fix reports, should be written plainly enough that Haiku can judge them.
Deciding whether a loop converges or circles should not need a large model.
One candidate route is for lyx to count open BLOCKING findings itself, so the judge no longer carries that part of the rule.

The replay also surfaced two reed defects, filed as issues #627 and #628 (board notes `reed-launch-race` and `reed-focus-false`).

## Webster Master on Haiku

The study's largest lever, about $30, was rejected without a trial.
Implementer forks inherit the Master's model and write the code.
The operator found Haiku (an older version, in Millhouse) often too weak as an implementer.
Haiku 5.5 is also cheap only below 100k context, and Webster forks run at 50-410k.

## Not tried, now on `burler-fixer-fork`

These items moved to board note `burler-fixer-fork`:

- Plan and Discussion fixers to `sonnet[medium]`.
- The fan turned on or off per round, which needs code.
  The fan was already benched and parked in `docs/research/lens-fan-lab-phase1.md`.
- A per-batch Webster model, with a cheap model only for mechanical cards, which needs code in `internal/websterengine`.
- Forking the fixer from the reviewer's session, so it does not read the context again.
  Skipping the fixer on an empty review is already built in `internal/burlerengine/fix.go`.

A separate model for twill lens forks, parent-review forks and Bouncer escalation forks is not possible.
Forks inherit the parent's model, and a non-fork subagent loses the shared cache prefix.

## Hub config on 2026-10-15

From `lyx config loom --print --layer default`, plus `webster.yaml` and `landing.yaml`:

| Key | Value |
|---|---|
| `review` | `opus[medium]` |
| `discussion_review` | `["opus[medium]", "opus[medium]", "sonnet[high]"]` |
| `plan_review` | `["opus[medium]", "sonnet[high]"]` |
| `webster_review` | `sonnet[medium]` |
| `fix` | `opus[medium]` |
| `discussion_fix`, `plan_fix` | empty (take `fix`, `opus[medium]`) |
| `webster_fix` | `sonnet[medium]` |
| `fan_review` | `sonnet[high]` (no fan is on: `discussion_fan` and `plan_fan` are empty) |
| `judge` | `sonnet[medium]` |
| `discussion_judge`, `plan_judge`, `webster_judge` | empty (take `judge`) |
| `seed` and per-segment seed keys | empty (take the segment's judge, `sonnet[medium]`) |
| `driver` | `haiku[medium]` (the embedded template still says `sonnet[medium]`) |
| `webster.yaml` `master` | `sonnet[medium]` |
| `landing.yaml` `conflict`, `describe` | `opus[high]`, `sonnet[medium]` |

The only model change that came out of this note is the per-round review lists.

## Sources

- `docs/research/model-downgrade.md` (2026-10-13 study)
- `docs/research/haiku-judge.md` (2026-10-10)
- `docs/research/twill-discussion-fork0-bench.md` (2026-10-15)
- `docs/research/lens-fan-lab-phase1.md`
- `docs/research/token-usage-by-role.md`
