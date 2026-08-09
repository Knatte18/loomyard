MILL_REVIEW_BEGIN
# Review: fabric: store the warp-URL binding in weft:main; fold bootstrap into clone (slice 10)

```yaml
verdict: APPROVE
reviewer_model: opus
reviewer_self_id: Claude (Opus-class, Anthropic)
reviewed_file: _mill/discussion.md
date: 2026-08-09
```

## Findings

### [NIT:design] Re-point conflict has no stated remedy
**Section:** conflict-rule / Scope "Out" **Issue:** The `present, differs` row is a hard error and no verb re-points a bound weft, but neither the error text nor the discussion names what an operator does when a warp repo is genuinely renamed or moved (`--force-bootstrap` is explicitly ignored when a record is present). **Fix:** State the intended remedy (e.g. hand-edit `.lyx-warp` in `_board` and commit, or "wait for a future bind verb") and require the conflict message to name it, as the unbound-weft message already does.

### [NIT:consistency] Derived-URL provenance absent from error wording
**Section:** clonehub-signature / Technical context step 1 **Issue:** In the one-argument form the warp URL comes from the record, yet existing messages read as operator-supplied — `could not derive repo name from warp URL %s` (`internal/fabricengine/clone.go:92`) and the step-5 clone failure both surface a URL the operator never typed. **Fix:** Say in the plan that on the derive path these errors state the URL came from the recorded `.lyx-warp` binding.

### [NIT:design] Probe reorders the offline failure surface
**Section:** pre-hub-probe / reset-folding **Issue:** The probe becomes step 0, ahead of today's step-3 hub-exists check (`internal/fabricengine/clone.go:98-101`), so a re-clone against an already-existing hub, or any offline invocation, now fails with a `probe weft <url>:` git error rather than `hub already exists`; the discussion never states this ordering consequence. **Fix:** Record the ordering explicitly (probe always precedes the hub-exists check) as accepted, or state that hub-exists is re-checked cheaply before probing in the two-argument form.

## Verdict

APPROVE
Decisions, taxonomy, guard, reconcile split and tests all verified against source; only wording nits remain.
MILL_REVIEW_END
