MILL_REVIEW_BEGIN
# Review: Producer gates: mechanical gates before session release

```yaml
duration_s: 181.0
verdict: REQUEST_CHANGES
reviewer_model: opus
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-09-20
```

## Findings

### [BLOCKING:design] One BurlerRound constructor, two different validators
**Section:** "The four sites and how each gets its gate"
**Issue:** All three burler rows share `engine: BurlerRound` (loom-recipe.yaml:70,147,253) and one constructor `burlerRoundEntry` (entries_burler.go:21); the discussion never says how that single constructor picks `discussionparser.Validate` for Discussion-Burler, `planglyph.ValidateFormat` for Plan-Burler, and nothing for Webster-Burler.
**Fix:** Decide the selector — row name switch, a new `gate:` config key, or split registry entries — and state it, including what a `gate_attempts` on Webster-Burler means.

### [NIT:consistency] Wrong burler allowlist named for `gate_attempts`
**Demoted-from:** BLOCKING
**Section:** "The four sites…" (Discussion-Burler / Plan-Burler bullet)
**Issue:** The cited allowlist `target, fasit, rubric, rubric_stencil, fix-scope, tool-use, cluster-fan` is `burlerRoundProfile`'s nested `profile:` sub-map (entries_burler.go:189); the row's own allowlist is entries_burler.go:42 — `run_subdir, profile, model, effort, timeout_s`. As written the key would land under `config.profile:`, contradicting "the row's own `config:` block".
**Fix:** Point the change at the row-level `configRejectUnknown` at entries_burler.go:42 and keep the key out of the profile map.

### [BLOCKING:design] Where the gate closure body lives is unstated, and two import allowlists depend on it
**Section:** Scope / "The four sites…" vs Testing (`internal/loomshed`)
**Issue:** Scope has `shedrecipe` entry constructors building the closures, while Testing carries `gatefindings_test.go` and `hasBlockingFinding` on "the gate closures" in `loomshed`. `shedrecipe`'s allowlist (seam_enforcement_test.go:33) admits neither `planglyph` nor `discussionparser`; `loomshed`'s (seam_enforcement_test.go:29) admits neither `shuttleengine`. Either home needs an allowlist edit that Scope does not list.
**Fix:** State that the closures are constructed in `loomshed` (or `shedrecipe`) and name the exact allowlist entries added, in Scope.

### [BLOCKING:design] Stencil-refresh premise is false as stated
**Section:** "Row removal and resume" — stale-mention sweep, stencils bullet
**Issue:** "a hash-mismatched file … is never overwritten, with no force-sync carve-out" does not match `reconcileOne` (reconcile.go:86): `StateUntouched` with a newer shipped body IS rewritten in prod mode and only warns in dev mode; only `StateEdited` is never overwritten; and `stencilstore.ForceRefresh` / `lyx stencil sync` (stencilcli/cli.go:138-181) is exactly a force route. The question is left open on a wrong premise.
**Fix:** Restate the mechanism per `Classify` states and decide the answer (dev-build operators run `lyx stencil sync`; edited copies stay stale by design).

### [NIT:scope] Startup-window Done path missing from the no-live-session test list
**Section:** Testing — `internal/shuttleengine`
**Issue:** The enumerated no-session Done paths omit `classifyStartupWindow` → `classifyDeadlineExpiry(OutcomeDied)` → `OutcomeDone` (wait.go:456), a fifth way a Done reaches `finalize` via `checkLivenessTick`.
**Fix:** Add it as its own case, or state that it is covered by the not-tracked/not-live cases because it shares the same `finalize` site.

## Verdict

REQUEST_CHANGES
Four grounded blockers: burler validator selection, wrong allowlist, closure home/imports, stencil premise.
_Note: 1 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 3._
MILL_REVIEW_END
