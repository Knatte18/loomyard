# Review: Seeded driver choice: ly-drive strand as the child's driver

```yaml
verdict: APPROVE
reviewer_model: orchestrator
reviewer_self_id: claude-fable-5
reviewed_file: _mill/discussion.md
date: 2026-09-19
```

## Findings

### [NIT:consistency] Invariant's mechanical proxy contradicts the task's own scope
**Demoted-from:** BLOCKING
**Section:** `new-invariant-driver-choice-single-site`
**Issue:** The proxy — "`shedrun`'s driver constants have exactly one production consumer outside `shedrun` itself, in `internal/loomcli`" — is falsified by this discussion's own scope: the lifted acceptance on `lyx shed seed --driver llm` (recipe-gated, so `shedcli` compares against the constant), the lifted `--child-driver llm` on `lyx batten run|step`, and the *rewritten* batten `--driver llm` refusal all consume the constants in production code, and that batten refusal is itself "a code path gating a refusal on it".
**Suggested fix:** Reword the invariant to govern the seed's *recorded* `driver` field (read at exactly one site per recipe, its bootstrap verb) as distinct from flag validation at seeding sites, and restate the proxy against that split — e.g. the only production reader of `Seed.Driver` outside `shedrun` is `internal/loomcli` — or enumerate the permitted seeding-site consumers explicitly.

### [NIT:decision] Autonomous step cap has a derivation but no number
**Demoted-from:** BLOCKING
**Section:** `ly-drive-gains-an-autonomous-mode`, item 3
**Issue:** The cap "is stated as its own number derived from loom's own worst case (near a hundred steps)" — but the number itself is never stated, so a plan writer must invent it (100? 120? the inherited 40?), and the skill's worst-case arithmetic ("near a hundred") brackets rather than fixes it.
**Suggested fix:** State the number in the decision (and whether it lands in the SKILL.md text, the launch prompt, or both), e.g. 120 = worst case ~100 plus margin, so the skill edit and the Go prompt composer pin the same value.

### [NIT:design] Timestamped report path can collide on a fast relaunch
**Section:** `driver-report-is-the-run-s-output-file` / Testing, spec composition
**Issue:** A second-granularity `drive-report-<compact-timestamp>.md` collides when corpse-removal-plus-relaunch lands in the same second (the smoke test's stub pane exits instantly), and `Spec.validate`'s must-not-exist check then refuses the relaunch — the exact case the timestamp exists to permit, and the "two launches produce two different paths" test only proves it under a faked clock.
**Suggested fix:** Add a uniqueness fallback to the derivation (sub-second component or short random suffix), or state that a same-second collision's refusal is an accepted, self-describing residual.

## Verdict

APPROVE
Consistent with seeded-shed-core's contract and verified against the code, but the new invariant's proxy contradicts the task's own scope and the autonomous cap is left numberless.
_Note: 2 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 0._
