MILL_REVIEW_BEGIN
# Review: reed: extract Selvage-pane lifecycle

```yaml
duration_s: 165.0
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic) — Opus-class model, as invoked
reviewed_file: _mill/discussion.md
date: 2026-09-19
```

## Findings

### [NIT:consistency] Enforcement rule (a) forbids the seam's own call sites
**Demoted-from:** BLOCKING
**Section:** `### enforcement-test-is-the-teeth` vs `### narrow-seam-shapes`
**Issue:** The reconcile-policy type is "built once from `(live, selvagePaneID)`" and `planPaneTarget` keeps taking an id, so the hosts must still read `st.SelvagePaneID` (verified: `reconcile.go:213`, `spawn.go:149`) — exactly the selector expression rule (a) bans outside `selvagepane.go`/`state.go`/`config.go`.
**Fix:** Decide one way — the seam helpers take `*ReedState` (so no host reads the field), or the spec names the surviving call-site reads as allowlisted; state it in both decisions.

### [NIT:decision] `clearConflictingPaneBindings` has no stated disposition
**Demoted-from:** BLOCKING
**Section:** `## Scope` / `## Technical context` (`reconcile.go:188`)
**Issue:** It is named in Technical context as claiming `st.SelvagePaneID` (verified `reconcile.go:190-191`) but appears in neither the In list, the Out list, nor any seam contract — and it encodes Selvage-specific policy ("a strand can never own Selvage's pane"), so the enforcement test would flag it.
**Fix:** State whether the Selvage claim moves into a `selvagepane.go` helper, stays with an allowlist carve-out, or is out of scope with a reason.

### [NIT:consistency] `planPaneTarget` is both rejected and adopted
**Demoted-from:** BLOCKING
**Section:** `### policy-moves-not-just-lifecycle` (Rejected) vs `### narrow-seam-shapes`
**Issue:** The first decision rejects "moving `planReconcile`/`planPaneTarget`/`toRenderInputs` wholesale into the new file" as inverting the scatter; the second decides `planPaneTarget` moves into `selvagepane.go` wholesale.
**Fix:** Remove `planPaneTarget` from the rejected list and state why it differs from the other two (its whole subject is Selvage-relative targeting, unlike the reconcile schedule and the render mapping).

### [BLOCKING:design] Blanking seam leaves Selvage code in `apply.go`
**Section:** `### narrow-seam-shapes` (Render blanking) vs `### policy-moves-not-just-lifecycle`
**Issue:** The helper returns only the blanked pane id, so `apply.go:94`'s `render.Selvage{PaneID: …, HeightRows: e.cfg.Selvage.HeightRows}` — a `render.Selvage` construction plus an `e.cfg.Selvage` read — stays behind, contradicting "zero Selvage-specific code / comment-only hits", and the test polices neither expression.
**Fix:** Fix the seam's return type (the whole `render.Selvage` value vs the bare id) and say explicitly whether non-`SelvagePaneID` Selvage selectors are in the goal or deliberately unpoliced.

### [NIT:consistency] Test-file counts use an unstated, different method
**Section:** `## Testing` / `### pure-refactor-no-behavior-change`
**Issue:** "Why now" mandates stating the method wherever counts are republished, then the test figures (80/51/73/45/17, "~350 assertions") are case-*insensitive* line counts — case-sensitive values are 50/27/60/37/11 — and `template.go`'s single hit is lowercase `selvage` (a yaml-key comment), not `Selvage`.
**Fix:** Label these figures as case-insensitive line counts, not "assertions", and correct `template.go`'s listing.

## Verdict

REQUEST_CHANGES
Enforcement rule collides with the seam contracts; three artifacts lack a consistent disposition.
_Note: 3 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 1._
MILL_REVIEW_END
