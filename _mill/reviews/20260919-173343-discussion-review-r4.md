MILL_REVIEW_BEGIN
# Review: Seeded driver choice: ly-drive strand as the child's driver

```yaml
duration_s: 229.0
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic) — Opus-class model, as invoked
reviewed_file: _mill/discussion.md
date: 2026-09-19
```

## Findings

### [BLOCKING:design] Driver model-spec resolution site undecided
**Section:** `driver-is-a-loom-config-key` + Testing (`internal/loomcli` spec composition)
**Issue:** `modelspec.Parse` only validates grammar; every other loom role goes on to `registry.Resolve` and fills three shuttle fields (`internal/loomengine/review.go:33-49`: `Model`, `Effort: resolved.Params["effort"]`, `Version: resolved.Params["version"]`), and that resolution lives in `loomengine` (`DiscussionSpec`, `PlanSpec`, `ResolveReview`), not in `loomcli`. The discussion names no resolver for `driver`, never mentions `Spec.Version`, and its test bullet ("the model taken from the loom config's `driver` key") would pin the raw alias rather than the resolved model id.
**Fix:** Decide where the driver spec is composed and resolved (a `loomengine` resolver alongside `ResolveReview`, using the already-wired `c.registry`, vs. inline in `loomcli`), and state that `Effort`/`Version` come from `resolved.Params`.

### [NIT:consistency] Sandbox exclusion has no sub-module slot
**Demoted-from:** BLOCKING
**Section:** Constraints — Sandbox Suite Coverage
**Issue:** The gate (`cmd/lyx/sandbox_coverage_test.go`) is module-keyed: a module is either `**Covers:**`-tagged in `tools/sandbox/*SUITE.md` or listed in `excludedModules`, and an allowlist entry must name a registered module. `loom` is already covered, so "the llm driver path takes the explicit exclusion, recorded in the suite document" is not a state that gate can hold or check — it would be prose only.
**Fix:** State plainly that the exclusion is a prose note in the suite document with no mechanical enforcement (and that `loom` stays covered, never added to `excludedModules`), or name a mechanism that the gate actually models.

### [NIT:consistency] SKILL.md cap test cites the wrong precedent
**Section:** `ly-drive-gains-an-autonomous-mode` + Testing (`plugins/ly/skills/ly-drive`)
**Issue:** `discussiontemplate_test.go` lives in `contracts/stencils/`, not `internal/loomcli`, and it asserts against an embedded stencil (`LoomTemplateDiscussion`), not a file read from the repo tree; no Go file today reads `plugins/ly/skills`. The test's owning package and the constant's visibility are also unstated — a test under `plugins/` cannot read an unexported `loomcli` constant.
**Fix:** Cite the actual repo-tree-reading precedent (`runtime.Caller`-derived repoRoot, as in `cmd/lyx/sandbox_coverage_test.go`) and say which package owns the constant and the test.

### [NIT:decision] No disposition for a driver that finishes normally
**Section:** `llm-driver-launches-through-shuttle` / `dead-driver-is-an-operator-case`
**Issue:** `Wait` is what performs `RemoveStrand` + run-dir cleanup on a done outcome (`Spec.KeepPane`'s own doc comment), so a path that never `Wait`s leaves the driver strand and its run directory behind on the *success* path too — reclaimed only by the next bootstrap's corpse removal and `Start`'s opportunistic orphan sweep. `KeepPane`/`AwaitOperator` are likewise unpinned by the "every other spec field" enumeration.
**Fix:** Record the post-completion disposition as an accepted residual naming the two reclaimers, and note that `KeepPane`/`AwaitOperator` are inert because `Wait` is never called.

## Verdict

REQUEST_CHANGES
Model resolution site and the sandbox-exclusion mechanism need deciding before plan writing.
_Note: 1 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 1._
MILL_REVIEW_END
