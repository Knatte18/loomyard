MILL_REVIEW_BEGIN
# Review: Unify webster/burler CLI wiring into a shared module

```yaml
duration_s: 187.0
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude Opus 4-class model (self-assessment; exact build unknown)
reviewed_file: _mill/discussion.md
date: 2026-09-08
```

## Findings

### [BLOCKING:design] Derive caller-set pin flags legitimate test callers
**Section:** Mechanical enforcement / Testing — enforcement tests
**Issue:** The pin is stated as "only *production* caller" but modelled on `internal/gitkit/callerset_enforcement_test.go`, which walks every `.go` file with no `_test.go` exclusion; `internal/burlercli/wiring_test.go:143,211`, `internal/webstercli/wiring_test.go:165`, both `cli_integration_test.go`s and `internal/standalonegeom/reedgeom_symlink_integration_test.go` all call `standalonestate.Derive` and would fail it, yet the discussion keeps those tests where they are.
**Fix:** State that the pin skips `_test.go` (the `treadleengine/seam_enforcement_test.go` style) or name the test call sites that must move, and say which.

### [BLOCKING:design] Source of the default plan dir is unspecified
**Section:** Plan-dir logic moves in / Dependencies / Values the result struct must carry
**Issue:** Today the default is `standalonegeom.WebsterGeometry(target, stateDir).PlanDir` (= `planparser.PlanDir(stateDir)`) in standalone and `hubgeom.WebsterGeometry(loc).PlanDir` in hub, but the discussion says `cliwire` constructs no `Geometry` and lists a dependency set containing neither `planparser` nor the geometry builders — so nothing tells `cliwire` what the default is that `SamePlanDir` and the missing-plan refusal compare against.
**Fix:** Decide whether the default arrives as a request/`PlanRules` field from the caller or `cliwire` imports `planparser`, and record it in the dependency list.

### [NIT:consistency] Verify command never runs the named regression surface
**Demoted-from:** BLOCKING
**Section:** Testing — verify command / Regression surface to watch
**Issue:** `internal/webstercli/verbs_test.go` and both `cli_integration_test.go`s carry `//go:build integration` and `smoke_test.go` carries `//go:build smoke`, so `go test ./...` alone runs none of them; the Q&A's "adding `-tags integration` costs runtime without new coverage of the moved code" is contradicted by `webstercli/cli_integration_test.go`'s own header, which says it exists to exercise the real `standalonestate.Derive` and the real standalone stencil seed end-to-end.
**Fix:** Either add a tagged run to the done gate or drop the tagged files from the regression-surface claim and remove the "no new coverage" rationale.

### [NIT:consistency] `manifest/designs/cli-wiring.md` contradicts the doc lifecycle
**Demoted-from:** BLOCKING
**Section:** Scope (In) / Q&A "Documentation?"
**Issue:** `docs/overview.md#documentation-lifecycle` (the authority CONSTRAINTS.md points at) says `manifest/designs/<module>.md` are drafts for **planned, not-yet-built** modules, deleted when the module lands, with rationale moving into the Go package header — so creating one in the landing commit produces a doc that is immediately deletable; the discussion also promises a "module-table row" in `docs/overview.md`, which lists shared packages in a tree, not a table.
**Fix:** Replace the designs doc with a `cliwire` package header comment plus the shared-lib tree entry, or state why this module is an exception.

### [NIT:consistency] Banned-declaration list omits the plan-dir names
**Demoted-from:** BLOCKING
**Section:** Mechanical enforcement — banned-declaration check
**Issue:** `samePlanDir`, `standalonePlanDirHasContent` and `(*websterCLI).standaloneDefaultPlanDir` all move into `cliwire` per the plan-dir decision, yet the banned list names only `resolveStandaloneTarget`, `repositoryRootOf`, `refuseNestedStandaloneGeometry`, `normalizeForContainment`, `pathContains`, `resolveToldDir` — leaving the exact R6-8/R6-9 bug class the plan-dir move was justified by unguarded.
**Fix:** Extend the banned list to the plan-dir declarations, or state why they are deliberately excluded.

### [NIT:decision] "A plan-dir override resolver" is unnamed
**Section:** Exported surface
**Issue:** Every other exported member is named with a signature; the plan-dir override resolver is described only as a phrase, with no statement of whether it is a `Module` method or a package function.
**Fix:** Name it and give its signature alongside the others.

### [NIT:consistency] Enforcement test names the callers `cliwire` must not name
**Section:** Descriptor values are declared by their owners / Mechanical enforcement
**Issue:** The decision states flatly that "`cliwire` names neither caller", while the banned-declaration test lives in `internal/cliwire` and hardcodes `internal/webstercli` and `internal/burlercli`.
**Fix:** Scope the "names neither caller" rule to production files explicitly.

## Verdict

REQUEST_CHANGES
Enforcement model, plan-dir default source, verify command and doc placement need settling first.
_Note: 3 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 2._
MILL_REVIEW_END
