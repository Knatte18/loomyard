MILL_REVIEW_BEGIN
# Review: finalize: fold Raddle into its own contract and repair the dead links in raddle.md, finalize.md and self-report.md

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opus
reviewer_self_id: Claude (Opus-class, Anthropic)
reviewed_file: _mill/discussion.md
date: 2026-08-09
```

## Findings

### [NIT:scope] raddle.md residue list is a bound, not an inventory
**Demoted-from:** BLOCKING
**Section:** `### raddle-md-three-slot-references` / Scope "In"
**Issue:** `finalize.md` gets an explicit end-to-end re-read ("not just the six spec-listed line items"), but `raddle.md` gets a closed three-line list (`:3`, `:54`, `:85`) — and `raddle.md:7` carries the same retired framing the fold removes: "living in `weft`: an always-run step after Webster", which also already contradicts that file's own `## When it runs: deferred to merge-time` section.
**Fix:** Apply the same end-to-end re-read rule to `raddle.md` (and state it for `self-report.md`), rather than enumerating three lines; the enumeration method, not the specific line, is what needs fixing.

### [NIT:consistency] Honesty paragraph may read as un-guarding the CONSTRAINTS.md anchor
**Section:** `### constraints-entry` point 4 vs `### weft-git-invariant-citation`
**Issue:** Point 4 lists as "not reached … everything outside `manifest/` and `docs/`", while the citation decision relies on the anchor in `../../CONSTRAINTS.md` (outside those roots) being "machine-verified from the first run" — an implementer can legitimately read point 4 as "don't resolve anchors in out-of-root targets" and leave exactly the r1-fixed break class unguarded.
**Fix:** State that the root restriction is source-side only (which files are scanned), and that any `.md` target — inside the roots or not — has its file and anchor resolved.

### [NIT:design] Fixture strategy for the mandated scenario list is undecided
**Section:** `## Testing`, "Scenarios the test must cover"
**Issue:** Several mandated scenarios (missing file, missing anchor, fenced-code skip, duplicate-heading `-1`, stale case (b) "keyed file renamed away") need a synthetic tree, and the reused `walkEnforcementRoots` skips any directory whose name contains `testdata` (`internal/lyxcwd/enforcement_test.go:128`) — a `testdata/` fixture tree walks to zero files and every such subtest passes vacuously.
**Fix:** Decide the fixture seam — e.g. pure helpers plus `walkEnforcementRoots(t, t.TempDir(), []string{"."}, …)`, whose `repoRoot` parameter accepts a temp root — rather than leaving it to mill-plan.

## Verdict

REQUEST_CHANGES
raddle.md residue enumeration is closed where finalize.md's is open; `raddle.md:7` escapes it.
MILL_REVIEW_END
