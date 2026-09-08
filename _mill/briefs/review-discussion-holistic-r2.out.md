MILL_REVIEW_BEGIN
# Review: Unify webster/burler CLI wiring into a shared module

```yaml
verdict: APPROVE
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-09-08
```

## Findings

### [NIT:consistency] Derive test call-site enumeration is incomplete
**Section:** Mechanical enforcement (the "six test call sites" paragraph) **Issue:** `internal/webstercli/wiring_test.go:165` also calls `standalonestate.Derive` and is not listed, and `standalonegeom/reedgeom_symlink_integration_test.go` holds two calls (:45, :49), so the real count is eight, not six. **Fix:** Correct the count/list, or state it as illustrative rather than exhaustive — the `_test.go` skip makes the design unaffected either way.

### [NIT:consistency] `ResolvePlanDir` is a method but violates the stated method rule
**Section:** API shape / Exported surface **Issue:** The decision states methods are for "the fallible and message-producing steps" and package functions for pure helpers, yet `(Module).ResolvePlanDir(toldPlanDir, defaultPlanDir string) (string, bool)` is infallible, produces no message, and reads no `Module` field. **Fix:** Either move it to a package-level function or state why the descriptor receiver is wanted here.

### [NIT:design] `StandaloneRequest` field semantics left implicit
**Section:** Exported surface **Issue:** `StencilsDirFlag`/`PlanDirFlag` are named as raw flag values, but today `wire` resolves both via `resolveToldDir(cwd, …)` *before* the mode branch (webstercli/wiring.go:82-88, burlercli/wiring.go:73-78) and must keep the resolved plan dir for `wireHub`; who resolves is unstated. **Fix:** Say explicitly that the request carries raw flag values and `ResolveStandalone` resolves them (harmless-because-idempotent double resolution otherwise).

### [NIT:design] Missing-plan refusal text shape unspecified
**Section:** Plan opt-in is a nil descriptor field **Issue:** `PlanRules` is said to carry "the missing-plan refusal's text", but webster's live message interpolates two distinct paths in order (`geom.PlanDir` and `standaloneDefaultPlanDir(...)`, wiring.go:243); a bare string cannot reproduce it byte-for-byte. **Fix:** Name the shape (format string with a stated argument order, or a `func(planDir, defaultPlanDir string) string`).

### [NIT:scope] `internal/preflight` not named among rejected homes
**Section:** Module name and location **Issue:** `preflight` already owns `ResolveMode` and is described in `docs/overview.md:388` as the mode selector for standalone-capable CLIs, making it the nearest neighbour a plan writer would ask about; it is disqualified (it imports `lyxcwd` and `fabricengine`) but never mentioned. **Fix:** Add one line rejecting `preflight` on its import set.

### [NIT:scope] "Only webstercli and burlercli are standalone-capable" is slightly overbroad
**Section:** Scope / Out **Issue:** `internal/quarrycli/cli.go:58-73` also branches on `preflight.ResolveMode`'s hub-vs-standalone verdict; it derives no state dir and no target, so it is correctly out of scope, but the claim as written is not literally true. **Fix:** Narrow the wording to "the only CLIs that derive standalone state geometry".

## Verdict

APPROVE
Round-1 blockers resolved; claims verified against source; remaining items are cosmetic.
MILL_REVIEW_END
