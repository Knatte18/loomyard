MILL_REVIEW_BEGIN
# Review: websterengine + webstercli told-geometry, and Webster standalone entry

```yaml
duration_s: 244.0
verdict: REQUEST_CHANGES
reviewer_model: opus
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-18
```

## Findings

### [BLOCKING:design] Eager fabricengine.Open regresses unwired hubs
**Section:** `websterCLI.layout is removed outright` (`c.fabric` opened once in the wiring function)
**Issue:** `fabricengine.Open` = `newPaired(l.WorktreePath(), WeftWorktree(l))` and hard-errors `*ErrMissingPath` when the weft sibling is absent (`open.go:12`, `fabric.go:67`) — exactly the three healthy-but-unwired locations (`<hub>/_board`, unpaired sibling, pair-removed worktree) the mode-selection Decision preserves; today `sync.go:26` opens lazily and only when `!opts.SkipGit`, so `validate`/`status`/`run` still work there and under `LYX_SKIP_GIT`.
**Fix:** state the disposition of an `Open` failure in the wiring function (tolerate → `nil` handle, or open lazily at the commit point) and say explicitly that `EnvSyncOptions().SkipGit` still means "never open".

### [NIT:consistency] c.fabric cannot be the RefMatcher source
**Demoted-from:** BLOCKING
**Section:** `websterCLI.layout is removed outright` — "single source for all three fabric-bound needs"
**Issue:** `*fabricengine.Fabric` exposes no scanner and no `Location`; the matcher is only obtainable from `fabricengine.NewRefScanner(l *lyxcwd.Location)` (`refscanner.go:24`), so the claim is false and no `c.refMatcher` field appears among the enumerated replacements for the deleted `layout`.
**Fix:** name the third replacement field and say it is built from the local `layout` in the wiring function before `layout` goes out of scope.

### [BLOCKING:design] Standalone RefMatcher supplier left as two options
**Section:** `standalone-has-no-fabric`
**Issue:** "a `websterengine`-local zero value **or** a `standalonegeom`-supplied one" is an unresolved alternative, and it is load-bearing: `CheckFork`/`CheckParent` call `fabricRef.Matches(cmd)` unguarded (`audit.go:114,183`), so a `nil` interface panics on the first standalone `record-batch`.
**Fix:** pick one supplier, name the exported type, and state that the seam is never `nil` in either mode.

### [NIT:design] AuditWorkdir is redundant with WorktreeRoot
**Section:** `webster-geometry-struct` / `the-two-roots`
**Issue:** in both pinned modes `AuditWorkdir == WorktreeRoot` (hub: both `l.AnchorPath()`; standalone: both the absolute target), so the ninth field is provably equal to an existing one everywhere and invites silent drift.
**Fix:** either justify the field by a mode where the two differ, or drop it and pin `WorktreeRoot` as the audit workdir.

### [NIT:consistency] WorktreeRoot means two different paths
**Section:** `the-two-roots` / Technical context
**Issue:** `websterengine.Geometry.WorktreeRoot` is `AnchorPath()` in hub mode while `reedengine.Geometry.WorktreeRoot` is `l.WorktreePath()` (`hubgeom.go:22`); the two-roots table also never states what `shuttleengine.NewRunner`'s `anchorPath`/`worktreeRoot` arguments (`cli.go:184`, fed from `reedGeom`) become in standalone.
**Fix:** note the deliberate name collision in the Geometry doc obligation and add a row for the Runner's two told arguments.

### [NIT:decision] audit_test.go's RefScanner tests have no disposition
**Section:** Testing → `internal/websterengine`
**Issue:** `audit_test.go:41,90,199` build a real `fabricengine.NewRefScanner` and `WeftWorktree` to pin the fabric-reference guard CONSTRAINTS cites as machine-checked; the discussion only says the file's `Location` fixtures need conversion, not whether those cases stay, move, or are re-expressed against `RefMatcher`.
**Fix:** state the disposition of `TestRefScannerMatches` once `CheckFork`/`CheckParent` no longer take `*RefScanner`.

### [NIT:consistency] Q&A log says six values, Decision says nine
**Section:** Q&A log ("keeps the four Deps structs from drifting on the same six values")
**Issue:** contradicts `webster-geometry-struct`'s nine-field list.
**Fix:** correct the Q&A entry to nine.

## Verdict

REQUEST_CHANGES
Fabric-handle lifecycle and the RefMatcher seam are unresolved; the rest is sound.
_Note: 1 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 2._
MILL_REVIEW_END
