# loom-step + self-report crucible campaign — orchestrator handoff

## Current state
No round has been spawned yet. `_mill/loom-review-prompt.md` has been seeded for **round 1** (the campaign kickoff mission: dummy-task drive of `lyx loom step` through discussion→plan→implement→review, an interrupted-and-resumed repro, a genuine Tier-1 anomaly trigger, a genuine Tier-2 friction/reflection cycle, and exactly one deliberate live-fire GitHub issue against `Knatte18/loomyard`).

Waiting on the operator's explicit model + effort pick for round 1 before spawning (crucible hard rule 2 — never default, never fall back to `general-purpose`).

## CLOSED-AND-VERIFIED
Nothing yet — no round has run.

## Residual currently seeded in `_mill/loom-review-prompt.md`
Not a residual — it's the campaign's round-1 mission seed (see the file's own "Round context seeded from prior-round verification" section). Full text lives there, not duplicated here.

## Deferred list
Empty — round 1 hasn't run yet.

## Live-fire self-report tracking
Not yet triggered. Per hard rule 7: only one deliberate live-fire issue for the whole campaign; once round 1 (or whichever round) confirms a real filing, record the issue number/URL here immediately, and campaign wrap-up must close it (labeled as a deliberate crucible test-fire) before hand-off.

- Issue: (none filed yet)
- Closed: n/a

## Next action
Ask the operator for round 1's model + effort tier pick, then spawn `subagent_type: crucible-reviewer-<effort>` with `model: <pick>`, prompt: "Read `_mill/loom-review-prompt.md` and do exactly what it says." Tag it `<model>-<effort>-r1`.
