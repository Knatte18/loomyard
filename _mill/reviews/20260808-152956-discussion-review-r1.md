MILL_REVIEW_BEGIN
# Review: Collapse _pattern into _lyx, and un-reserve _raddle as a hub-level name

```yaml
verdict: GAPS_FOUND
reviewer_model: opus
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-08
```

## Findings

### [GAP] Template pathspec change does not reach deployed configs
**Section:** "No migration" note (line 119) + `template.yaml`'s `pathspec` becomes empty
**Issue:** `yamlengine.applyExistingOverrides` (`internal/yamlengine/reconcile.go:117`) copies each *existing* leaf value onto the template, so a deployed `fabric.yaml` keeps `pathspec: _pattern`; `Config.Dirs()` still yields `_pattern`, `RepoWiredNames` still contains it, and `applyStaleRemoval` never sees it as stale.
**Fix:** State explicitly that the teardown claim holds only for freshly-cloned repos, and decide what (if anything) happens to an already-deployed `pathspec: _pattern` value.

### [GAP] `ReportOnly` has a second writer the decision does not name
**Section:** `HostPollution.ReportOnly` is deleted with `_raddle`
**Issue:** The `_raddle` branch is not its only writer — `Status` sets `ReportOnly: true` on the synthetic `<scan error: %v>` entry at `internal/fabricengine/status.go:149-152`, which has no remedy either.
**Fix:** Decide what the scan-error entry becomes once the field is gone (bare `Path`, a new error field, or a returned error) and record it.

### [GAP] PATTERN content silently joins `_lyx` commit routing
**Section:** Scope / `PatternResidue` is re-scoped
**Issue:** Every round-loop caller passes `ScopedPathspec(rel, ["_lyx"])` (see `committed_lyxonly_integration_test.go:29`), so after the move, hand-authored PATTERN edits are swept into automated weft commits that previously could not touch `_pattern` — a behavioural change interacting with the Fabric Git Invariant's positive-only cross-module exclusions and with what `PatternResidue` will now flag.
**Fix:** State whether that widening is intended and what it means for residue volume, or scope a carve-out.

### [GAP] Ownership of the new `_lyx/...` pathspec literals is unstated
**Section:** `PatternResidue` is re-scoped
**Issue:** `patternDirName` is replaced by strings beginning `_lyx`, but `_lyx`'s sole registered owner is `internal/lyxdirs` (`enforcement_test.go:287`); the discussion never says whether fabricengine builds them from `lyxdirs.LyxDirName` or as bare literals, nor that exact-equality matching means bare literals go unpoliced.
**Fix:** Name the construction form and note that `TestEnforcement_GeometryLiterals` will not catch a bare `"_lyx/PATTERN.md"`.

### [GAP] Wrong type name for the pollution entry
**Section:** Scope bullet, decision heading, Q&A
**Issue:** The type is `PollutionEntry` (`status.go:30`); no `HostPollution` type exists, so three references point at a non-existent identifier.
**Fix:** Rename to `PollutionEntry.ReportOnly` throughout.

### [GAP] Reserved-name arithmetic is wrong, and `_pattern` un-reservation is unaddressed
**Section:** Testing → `_raddle` un-reservation
**Issue:** `junctionnames_test.go:232-246` reserves six names; both `_pattern` and `_raddle` leave, so it drops to four, not five. Separately, `_pattern` also stops being a reserved slug — `internal/lyxcwd/lyxcwd_test.go:132-141`'s `TestIsReservedHubName_Pattern` pins the opposite and is never mentioned, and a slug named `_pattern` colliding with a stale on-disk junction is unconsidered.
**Fix:** Correct the count and add the `_pattern` un-reservation (test inversion plus collision risk) to scope.

### [NOTE] Constraint cited under a name that does not exist
**Section:** Constraints; Pattern Leaf Invariant decision
**Issue:** "Lyx Directory Token Invariant" is not a `CONSTRAINTS.md` heading — the actual heading is "Lyxdirs Single-Declarer Invariant"; the same passage calls `lyxdirs` "stdlib-free" where the invariant says stdlib-only.
**Fix:** Use the real heading and the "stdlib-only, zero-import" wording.

### [NOTE] "Four hub-structural tokens" sites not enumerated
**Section:** Technical context → `internal/fabricengine`
**Issue:** `HubReservedNames()` returns four today; `structuraldirs_test.go:99-110`'s `TestHubReservedNames_StillReturnsExactlyTheFourHubStructuralTokens` (name included), `junctionnames_test.go:73-77`, and `doc.go:84` all say "four" and must become three.
**Fix:** List those three sites, including the test-function rename.

## Verdict

GAPS_FOUND
Migration claim, ReportOnly's second writer, and commit-routing widening need resolution before planning.
MILL_REVIEW_END
