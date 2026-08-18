MILL_REVIEW_BEGIN
# Review: invariants and docs for the told-geometry rule

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-18
```

## Findings

### [BLOCKING:consistency] hubgeom fails the invariant's own predicate
**Section:** Decisions → "Enforcement basis — named honestly, per package"
**Issue:** The membership predicate binds a package only when it "imports `internal/lyxcwd` in production not at all", yet `internal/hubgeom` is listed in the review-obligation (bound) set while `internal/hubgeom/hubgeom.go:9` and `webstergeom.go:8` import it in production — and the discussion's own Technical context (line 203) lists hubgeom under the tier that "legitimately imports it".
**Fix:** State how the two geometry-adapter packages sit relative to the predicate (e.g. a named carve-out: adapters are *tellers*, bound by point 3's direction rule rather than by the non-import predicate) and remove `internal/hubgeom` from the non-import-derived list.

### [BLOCKING:design] preflight/hubgeom placed in the shared-infrastructure list
**Section:** Decisions → "`docs/overview.md` — three targeted edits", item 2
**Issue:** `docs/overview.md:318` describes "a thin layer of shared infrastructure" the user-facing modules sit *on*; `internal/preflight` imports `internal/fabricengine` and `internal/hubgeom` imports `reedengine`/`burlerengine`/`perchengine`/`websterengine`, so both sit *above* the engines, not beneath them — adding them there states a false layering.
**Fix:** Decide where the two adapters are mapped (a separate sentence in `## Modules` or the new Execution-stack paragraph) and keep only `internal/standalonegeom` as a candidate for the shared-infrastructure list.

### [BLOCKING:scope] docs/shared-libs/README.md has no stated disposition
**Section:** Decisions → "`docs/overview.md` — three targeted edits"
**Issue:** The overview sentence being edited says the list is "defined in [shared-libs/README.md]", whose own package list (lines 28–42) also lacks `preflight`, `hubgeom`, `standalonegeom`; the discussion never says whether that file is in or out of scope, and its "the list is a map and three packages are missing from it" rationale applies to it verbatim.
**Fix:** Name `docs/shared-libs/README.md` explicitly as in-scope or out-of-scope, with the reason.

### [BLOCKING:scope] doc.go audit subject set is stated as an unverifiable count
**Section:** Decisions → "`doc.go` audit — additive, not a rewrite"
**Issue:** The subject set is "the same fifteen the brief's Files list names", but the discussion's own enumeration totals sixteen packages (6 machine-enforced + 10 review obligation), and it then also assigns audit dispositions to `internal/webstercli` and `internal/scoutcli`, which appear in neither list — so a plan writer cannot reconstruct the set from this document.
**Fix:** Enumerate the audit's subject packages inline rather than by count and by reference to the wiki brief.

### [NIT:consistency] Cited line numbers drifted by 1–2
**Section:** Technical context → "Guards this task must not trip"; Decisions → design-doc deletion
**Issue:** `docsLinkAllowlist` is at `docslink_test.go:396-399`, not `394-397`; the four roadmap `See` lines are 108/111/114/117, not ~107/110/113/116. Content and counts (five links, fourteen `allowedSpawners` entries, `enforcement_test.go:907/940`, `tierpurity_test.go:28-43`, pinned config sets) all verified correct.
**Fix:** Refresh the four line citations or drop the numbers in favour of the keys.

## Verdict

REQUEST_CHANGES
Predicate contradiction, false layering claim, and two unresolved scope boundaries.
MILL_REVIEW_END
