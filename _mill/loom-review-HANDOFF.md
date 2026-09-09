# loom crucible campaign — handoff note

Campaign: confirm live that `centralize-glyph-shape-enum` and
`quarry-bump-v0-2-0-status-helpers` (both merged, both unit/integration-tested) are genuinely
behavior-preserving when driven through loom's real built binary — not just under the unit suite.
See `_mill/loom-crucible-orchestrator-kickoff.md` for the full campaign brief.

## Current state
No round has run yet. `_mill/loom-review-prompt.md` is seeded for **round 1** with the
safety-verification dummy-task mission (Create+canonicalize, Rename exact-tier auto-bind,
deliberate drift, fail-closed `lookup`, `Status.Known()`/`Rejected()` call-site audit — all via the
real `validate-plan`/`record-batch` CLI verbs, no real LLM subprocess required per the review
prompt's cost declaration).

Waiting on the operator's explicit model + effort-tier pick before round 1 can spawn (Hard Rule 2 —
never defaulted, never falls back to `general-purpose`).

## CLOSED-AND-VERIFIED
None yet.

## RESIDUAL currently seeded
Round 1's full mission (see `_mill/loom-review-prompt.md`'s "Round context seeded from
prior-round verification" — there is no residual yet, this is the initial mission).

## DEFERRED list
None yet.

## Next action
Get the operator's model + effort pick, then spawn `subagent_type: crucible-reviewer-<effort>`
with `model: <pick>`, prompt: "Read `_mill/loom-review-prompt.md` and do exactly what it says.",
tagged `<model>-<effort>-r1`.
