MILL_REVIEW_BEGIN
# Review: landing: parent-fabric resolution chain

```yaml
duration_s: 180.0
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-23
```

## Findings

### [BLOCKING:design] ReadOrigin's found bool has no disposition
**Section:** Decisions → `parent-branch-from-origin-record`; Technical context → Deps table
**Issue:** `fabricengine.ReadOrigin` is `(Origin, bool, error)` (origin.go:75) and a false bool is the documented legacy-worktree case, but the discussion states only "`Origin.ParentBranch`" — it never says what `drive` does when `found` is false or `ParentBranch` is empty; `drive.go` has no `--parent` flag, so `run.go`'s `resolveParentBranch` remedy is unavailable there, and an empty `ParentBranch` would silently reach `OpenParent` and surface much later as a Finalize `Stuck`.
**Fix:** State the disposition for `found == false` / empty `ParentBranch` in `drive.go` — refuse on the envelope naming `lyx loom run --parent`, or an explicit alternative.

### [BLOCKING:design] Self-parent behaviour left explicitly undecided
**Section:** Testing → `internal/fabricengine` — the matcher
**Issue:** The scenario list says "Decide and pin the behaviour here rather than leaving it implicit" for the case where the parent branch equals the acting worktree's own branch — a decision deferred out of the Decisions section into the test plan, leaving a plan writer to choose between matching self and refusing.
**Fix:** Add a `### Decision:` fixing the self-parent outcome (match, or refuse with a named error) so the test pins a decided behaviour rather than discovering one.

### [NIT:consistency] "All fourteen fields non-zero" is impossible
**Demoted-from:** BLOCKING
**Section:** Testing → `internal/loomcli`
**Issue:** `Deps` does have fourteen fields (deps.go), but `PushSkipped bool` is `false` in every normal invocation (`EnvSyncOptions().SkipPush` reads `WEFT_SKIP_PUSH`), so an assertion that every field is non-zero cannot pass as written.
**Fix:** Restate the assertion as "every field except `PushSkipped` non-zero", or specify the env-set fixture, and say how the drift guard covers the bool.

### [NIT:design] Two-opens rationale is nominal, not substantive
**Section:** Decisions → `two-opens-in-drive-rather-than-a-shared-handle`
**Issue:** `drive.go` opens a handle eagerly at the same site it would have cached, and `NewPublish`/`NewFinalize` call `deps.OpenFabric()` eagerly at construction (publish.go:68, finalize.go:77), so the "laziness is a documented contract" argument does not distinguish the two options in this call path.
**Fix:** Either re-ground the decision on where the contract actually still binds, or note the deps.go laziness comment as a third comment to correct.

### [NIT:scope] Files-touched list omits the docs the Scope requires
**Section:** Technical context → "Files that will be touched"
**Issue:** Scope-In and the Constraints section both require `manifest/designs/loom.md`, `manifest/roadmap.md`, and the affected package docs in the same commit, but none appears in the eight-item file list.
**Fix:** Add the doc files to the list so the plan's work inventory matches the stated scope.

### [NIT:consistency] websterGeom is not reachable from drive.go
**Section:** Technical context → Deps table (`WebsterDir` row)
**Issue:** The parenthetical "already built in `wire()` as `websterGeom`" is a local variable never stored on `c` (wiring.go:64); only `c.runDeps.Geom` carries it, so the note conflicts with the claim that `registry`/`runner` are "the one structural change to `wire()`".
**Fix:** Say which reach drive uses — a fresh `hubgeom.WebsterGeometry(c.location)` call or `c.runDeps.Geom.WebsterDir`.

## Verdict

REQUEST_CHANGES
Three decisions missing or undecidable; the rest of the grounding checks out.
_Note: 1 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 2._
MILL_REVIEW_END
