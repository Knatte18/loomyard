MILL_REVIEW_BEGIN
# Review: Pin the plan format (Builder input contract)

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-10
```

## Findings

### [NOTE] Chain membership / chain-start derivation not pinned
**Section:** Principle #0 exceptions (deferred-verify chains)
**Issue:** `chain-end: NN` is pinned, but the rule for enumerating a chain's members and locating the chain-start SHA (roll-back target) is only implied, and `chain-end` referencing a renumbered `NN` is in tension with "robust against reordering."
**Fix:** Have the doc state chain membership = all batches whose `chain-end` equals the same `NN` (plus batch `NN`), chain-start = lowest-numbered such batch, and note how `chain-end` survives renumbering.

### [NOTE] Role list conflates stack-wide roles with builder.yaml roles
**Section:** Model-spec notation
**Issue:** The role set lists `evaluator`, but the consumer-model correction removed builder's Go on-demand evaluator; a plan writer documenting builder.yaml roles cannot tell whether `evaluator` is a builder role or a perch/burler one.
**Fix:** Doc should separate stack-wide roles (evaluator belongs to perch/burler) from the roles builder.yaml actually holds (orchestrator, implementer, implementer_oversized, fixer).

### [NOTE] Roadmap plan-format pointer targets the wrong doc
**Section:** Scope (same-commit roadmap update) / Where the docs land
**Issue:** `docs/roadmap.md:180-181` states the plan-format contract "is pinned in the module doc ([modules/loom.md])", but the contract now lands in `docs/modules/plan-format.md`.
**Fix:** Redirect that roadmap pointer to `modules/plan-format.md` as part of the same-commit roadmap update.

## Verdict

APPROVE
All decisions closed with rationale and rejected alternatives; only minor doc-precision NOTEs remain.
MILL_REVIEW_END