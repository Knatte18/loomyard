MILL_REVIEW_BEGIN
# Review: reed: extract Selvage-pane lifecycle

```yaml
duration_s: 217.0
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: claude-opus-4-1 (best-effort; brief dictates "opusmedium")
reviewed_file: _mill/discussion.md
date: 2026-09-19
```

## Findings

### [BLOCKING:design] Policy type name defeats the enforcement rule
**Section:** `narrow-seam-shapes` (Reconcile policy) + `enforcement-test-is-the-teeth`
**Issue:** The reconcile seam is "a small unexported value type" that `planReconcile` takes as a parameter, so `reconcile.go` must name that type in its signature — a non-call, non-composite-key `*ast.Ident`, which the stated rule flags if the name contains `selvage`; the same applies to the local `reconcileLocked` holds it in. The discussion leaves exact names to mill-plan and asserts case-insensitivity "costs mill-plan no naming freedom, because helper *names* only ever appear in non-allowlisted files at call positions" — false for a type in a parameter list and for the host-side local.
**Fix:** State explicitly that the policy type and any host-side local/parameter holding it must carry a non-Selvage-named identifier (reconcile-scoped naming), or add a third exemption for type positions and say why it cannot smuggle logic.

### [NIT:scope] Verification tier omits the smoke suites
**Demoted-from:** BLOCKING
**Section:** Testing → "Full run"
**Issue:** The run names `go build ./...`, `go test ./...` and "the integration-tagged reed suites", but the end-to-end live-tmux Selvage coverage sits behind `//go:build smoke` in `internal/reedcli` — `smoke_selvage_keepalive_test.go` (32 `Selvage` lines), plus `smoke_lifecycle_test.go` (95), `smoke_staterecovery_test.go` (15), `smoke_panecwd_test.go` (5), `smoke_dotfill_test.go` (3). None of these five files appears anywhere in the discussion, so the behavior-preservation claim rests on a tier the plan is not told to run.
**Fix:** Name the `smoke` tier (specifically `internal/reedcli`'s Selvage smoke files) alongside the integration tier in "Full run", and state that they are expected to pass unedited.

### [NIT:consistency] planPaneTarget's existing test: move or edit in place
**Section:** `tests-follow-their-subject` vs Testing → "Existing suites, untouched"
**Issue:** `planPaneTarget` moves, so its subject test (`spawn_test.go:21-137`) should move per `tests-follow-their-subject`; the Testing section instead treats the `planPaneTarget` signature change as one of two "sanctioned test edits" in the existing suites, implying it is edited where it sits. `bottommostPaneID`'s test (`lifecycle_test.go:661`) has the same double reading.
**Fix:** Say which file the existing `planPaneTarget`/`bottommostPaneID` tables end up in, and whether the signature edit happens before or after the move.

### [NIT:design] Exemption (i)'s "cannot be abused" is overstated
**Section:** `enforcement-test-is-the-teeth`, exemption (i)
**Issue:** The argument — a call must resolve to an in-package declaration the declaration rule already flags — holds only for same-package calls. A cross-package Selvage-named function would be exempt at the call site and unscanned at its declaration, since the check deliberately does not reach `internal/reedengine/render`. Today `render` exports the type `Selvage` but no such function, so the hole is latent, not open.
**Fix:** Record this as the test's honest residual (the pattern the cliwire/gitkit precedents already carry) instead of claiming the exemption is unabusable.

## Verdict

REQUEST_CHANGES
Enforcement rule is unsatisfiable for the policy type name; smoke-tier Selvage coverage unnamed.
_Note: 1 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 1._
MILL_REVIEW_END
