MILL_REVIEW_BEGIN
# Review: webster standalone mode: run refuses to start Master; logs write untracked into target repo

```yaml
duration_s: 200.0
verdict: REQUEST_CHANGES
reviewer_model: opus
reviewer_self_id: Claude (Anthropic), Opus-class; exact version not self-verifiable
reviewed_file: _mill/discussion.md
date: 2026-09-06
```

## Findings

### [NIT:consistency] logger sink-API decision adopts what it rejects
**Demoted-from:** BLOCKING
**Section:** `### logger-production-sink-api`
**Issue:** The block carries a second, superseded `Rationale:`/`Rejected:` pair (lines 179–184) whose rejected **(a)** is "Add a separate production wrapper delegating to the same body — two names for one behaviour", which is exactly the decision stated at the top of the same block ("one new production entry point … sharing `SetDurableSinkDir`'s existing reset body"); the Q&A entry likewise answers "Promote it — rewrite the doc comment, keep the name and body" with no mention of a second entry point.
**Fix:** Delete the stale second Rationale/Rejected pair, fold its still-live content (the ordering-obligation doc sentence) into the surviving rationale, and restate the Q&A answer as "add an atomic entry point; keep `SetDurableSinkDir`'s name, signature and body as the no-worktree-root shorthand".

### [BLOCKING:design] Detached constructor's third told path has no validation disposition
**Section:** `### detached-pair-validation` / `### fork-audit-pane-cwd`
**Issue:** `fork-audit-pane-cwd` makes `paneCwd` an explicit third parameter of the detached constructor, but `detached-pair-validation` decides only the anchor/worktree pair ("non-empty and absolute … plus strict disjointness") and the Testing table's refused-shapes rows name no `paneCwd` case — yet `paneCwd` becomes the fork audit's workdir (`wait.go:431`), the exact consumer whose failure mode `validateToldPaths` exists to catch: an empty or relative value does not fail, it resolves against the process cwd.
**Fix:** State whether the detached constructor validates `paneCwd` (non-empty + absolute, and whether any relation to the other two is asserted), and add the corresponding refused-shape rows to the `internal/shuttleengine` testing list.

### [BLOCKING:design] CONSTRAINTS.md host entry for the new clause does not exist
**Section:** `### documentation-surface`
**Issue:** The decision says to record the detached-anchor rule "extending the shuttle seam's entry rather than opening a new top-level invariant", but `CONSTRAINTS.md`'s only shuttle entry is the **Shuttle Provider-Seam Invariant**, which is exclusively about provider specifics living under `claudeengine` and has no anchor/worktree-geometry content — the clause has no stated home, while the Told-Geometry Invariant (which already binds `shuttleengine` and already names `hubgeom`/`standalonegeom`) is the topical candidate the decision never considers.
**Fix:** Name the exact `CONSTRAINTS.md` heading the clause is appended to and why, or state that a new top-level invariant is opened.

## Verdict

REQUEST_CHANGES
Sound analysis and verified source claims; three unresolved decisions block plan writing.
_Note: 1 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 2._
MILL_REVIEW_END
