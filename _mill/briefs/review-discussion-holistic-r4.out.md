MILL_REVIEW_BEGIN
# Review: Deploy cited spec/design docs to target repos like stencils

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude Opus 4.x-class model (exact build unknown; self-reported ID "Opus 5")
reviewed_file: _mill/discussion.md
date: 2026-09-13
```

## Findings

### [BLOCKING:consistency] Scope says four rubric sites, decision says three
**Section:** Scope bullet 6 vs `rubric-marker-allowlist` / Q&A line 351
**Issue:** Scope says the helper is "routed through by all four rubric-value sites", while the decision and the Q&A log fix the count at **three** stencil-sourced sites and explicitly reject `internal/burlercli/run.go:63` (verified: its `Rubric` comes from `profileYAML.Rubric`, no stencil route) — a plan writer following Scope would wire in the literal site the decision rejects outright.
**Fix:** Correct the Scope bullet to "three stencil-sourced rubric sites".

### [BLOCKING:decision] `lyx stencil sync`/`validate` coverage left undecided
**Section:** Technical context, "Existing call sites to extend for seeding"
**Issue:** "`stencil sync`/`validate` covering specs is a judgment call for the plan" is an explicit TBD on observable CLI behaviour; it also hides that `internal/stencilcli/cli.go:184` is a **second** production caller of `CommitSeededStencils` (the decision names only `cmd/lyx/stencilseed.go:129`), so the generalisation lands on it either way, and `stencilstore.Validate` compares marker sets — semantics a non-template spec does not have.
**Fix:** Decide now whether `sync` and `validate` operate on the specs registry, and state the disposition of the `stencilcli` commit caller.

### [BLOCKING:design] Specs seeding under a told `--stencils-dir` undecided
**Section:** `specs-dir-marker`; Testing, "Seeding — integration"
**Issue:** `internal/cliwire/standalone.go:91-101` seeds only when `stencilsDir == ""`, a deliberately documented asymmetry; the discussion establishes only that the specs dir is *resolved* independently, never whether the specs `Reconcile` runs in the told-override branch — if it inherits the skip, the specs dir resolves but is empty and `{{.specs_dir}}` points at nothing, reproducing the dead reference.
**Fix:** State explicitly that the specs reconcile runs unconditionally in standalone (and why the "never rewrite a curated set" rationale does not extend to specs), or that it shares the skip and what the consequence is.

### [NIT:consistency] "nine sites" does not match the audit table
**Section:** `citation-enforcement-test`; Testing, first block
**Issue:** The audit lists 8 normative rows plus 5 non-normative rows, and `bouncer-template-judge.md` carries the path at three lines (61, 90, 116) — so the scan's initial-failure count is 13 occurrences, not nine; "must fail on all nine current sites, which is the proof it works" is therefore an unusable TDD criterion.
**Fix:** Restate the count from the audit table, or drop the exact number in favour of "every occurrence the audit lists".

### [NIT:design] Agent reachability of the deployed path never stated
**Section:** Problem / Constraints
**Issue:** Deployed specs live under `<hub>/_board/_lyx/specs/`, and the Hub Containment Invariant bars junctioning `_board` into a worktree, so an agent must read an absolute path outside its own worktree; this premise (and the fact that it holds only for autonomous specs, which alone get `--dangerously-skip-permissions` — `claudeengine/command.go:87-89`) is never recorded, and Hub Containment is absent from the Constraints list.
**Fix:** Record the reachability premise and list the Hub Containment Invariant among the binding constraints.

### [NIT:scope] `CommitSeededStencils` hardcodes the dir twice, not once
**Section:** `specs-dir-mirrors-stencils-dir`, weft-commit half
**Issue:** Besides the pathspec join at `stencilcommit.go:56`, line 65 records each mutation as `filepath.Join(StencilsDir(hub), relPath)`; parameterising only the prefix would file seeded specs under the stencils dir in the `*Mutations` record, against the Mutation Record Invariant — so "differing only in one constant" is inaccurate.
**Fix:** Note that both the pathspec prefix and the absolute-dir used for the mutation record are generalised together.

## Verdict

REQUEST_CHANGES
Three blocking items: a stale site count, an undecided CLI-verb disposition, and undecided seeding under `--stencils-dir`.
MILL_REVIEW_END
