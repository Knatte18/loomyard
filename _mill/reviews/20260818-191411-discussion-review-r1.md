MILL_REVIEW_BEGIN
# Review: invariants and docs for the told-geometry rule

```yaml
duration_s: 221.0
verdict: APPROVE
reviewer_model: opus
reviewer_self_id: Claude Opus 5 (Anthropic)
reviewed_file: _mill/discussion.md
date: 2026-08-18
```

## Findings

### [NIT:consistency] Vocabulary walk does not reach the edited .md files
**Demoted-from:** BLOCKING
**Section:** Technical context → "Guards this task must not trip", and Testing → Documentation verification
**Issue:** Both say `TestEnforcement_FabricVocabulary` gates "the new `.md` prose", but `internal/lyxcwd/enforcement_test.go:940` walks `.md` under `{"internal", "contracts/stencils"}` only — `CONSTRAINTS.md`, `docs/overview.md`, and `manifest/roadmap.md` are all outside it, so none of this task's `.md` prose is machine-checked.
**Fix:** Restate the guard's real reach — it covers only the `doc.go` edits (via the `{"internal","cmd"}` `.go` walk at line 907) — and record Fabric vocabulary in the three `.md` files as a review obligation.

### [NIT:scope] "Named exactly" package sets have no membership predicate
**Demoted-from:** BLOCKING
**Section:** Decisions → "Enforcement basis — named honestly, per package"; "`doc.go` audit"
**Issue:** The six/ten enumeration claims to be exact but omits packages with the same told-geometry property: `internal/batcher` (`config.go:31-33` — "baseDir must already be resolved by the caller — Active never resolves cwd itself (see the Cwd Resolution Invariant)", has a `doc.go`), `internal/stencilstore` (`doc.go:4` — "caller-supplied absolute stencils directory"), and `internal/shedadapters`; no predicate defines "converted package", so the sets are not reproducible by the plan writer or a future task.
**Fix:** State the membership predicate the sets are derived from (e.g. "engine/producer package that receives absolute paths and imports no `internal/lyxcwd`") and re-derive both sets from it, or scope the invariant text to the enumerated set explicitly and say it is not exhaustive.

### [NIT:design] "Two sole constructors" is broader than the pair supports
**Section:** Decisions → "Told-Geometry Invariant — content", point 3
**Issue:** Verified true for the four `*engine.Geometry` structs (`hubgeom` and `standalonegeom` are the only `Geometry{` sites), but `standalonegeom.StencilsDir` has no `hubgeom` sibling, and `treadleengine`/`shedengine` are told plain values (`runDir`, `Profile.GateDir`, `StatusPath`) rather than a `Geometry` struct — so "a new engine adds a sibling constructor in each" is already not literally true.
**Fix:** Scope the rule to engine `Geometry` structs and say plainly that plain told values (treadle, shed) are the other permitted shape.

### [NIT:consistency] "Only real validation" contradicts the sub-points beside it
**Section:** Decisions → "Cwd Resolution Invariant — the reword"
**Issue:** Sub-point 1 calls the `rev-parse` success "its only real validation", while sub-points 2–3 in the same bullet describe two further error returns `Resolve` genuinely makes (`ErrStaleAnchorMarker`, `ErrCwdOutsideAnchor`), both confirmed in `internal/lyxcwd/lyxcwd.go`.
**Fix:** Reword to "the only validation of the repository itself", keeping the marker and cwd-gate checks as the two additional, non-initialization checks.

### [NIT:scope] docslink allowlist's stale-entry failure mode unmentioned
**Section:** Technical context → "Guards this task must not trip"
**Issue:** `docsLinkAllowlist` (`docslink_test.go:396-399`) carries a live entry keyed `{docs/overview.md, ../CONSTRAINTS.md#package-naming}`; an unmatched key is a test *failure* ("stale allowlist entry, delete it"), so an incidental edit to that link while touching `docs/overview.md` fails the build.
**Fix:** Note the allowlist and that the `#package-naming` link must be left as-is unless its entry is deleted in the same commit.

## Verdict

APPROVE
Two blocking: a false guard-coverage claim and an unreproducible package-set enumeration.
_Note: 2 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 0._
MILL_REVIEW_END
