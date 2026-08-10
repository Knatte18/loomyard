MILL_REVIEW_BEGIN
# Review: fabric: one ownership-and-dirtiness gate for all destruction (slice 12)

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-10
```

## Findings

### [BLOCKING:design] Clone's two gate sites have no `*lyxcwd.Location`
**Section:** `gate-call-shape` / `rollback-paths-go-through-the-gate`
**Issue:** Both request shapes make `l *lyxcwd.Location` a required field ("`l` is passed, never derived"), but `resetHub` (`clone.go:569`, called at `:159`/`:204`) and `teardownHub` (`clone.go:605`, earliest call `:243`/`:245`) live in `CloneHub`, which holds only `cwd string` — `lyxcwd.Resolve` is not reached until `clone.go:369`, long after both; the only prior Location is the synthetic partial `&lyxcwd.Location{HubPath, WorktreeName}` at `clone.go:260`, itself after the first teardown sites.
**Fix:** State whether these two sites pass nil, a synthetic partial Location, or the gate makes `l` optional, and say which gate predicates may then be resolved — otherwise the "required field ⇒ omitted check is a compile error" property does not hold at two of the six `RemoveAll(` sites.

### [BLOCKING:design] `resetHardTo` has no ownership kind, container, or target
**Section:** `ownership-is-a-closed-enum` / Technical context "five primitives"
**Issue:** `ResetHard` is assigned to `pathRequest` (required `container`, `target`, `ownership`), but no decision names what those are for a reset, and none of the four path-shaped kinds fits the warp worktree `pull` resets: `isRegisteredLinkedWorktreeIn` explicitly skips `entry.Main` (`remove.go:230`), so `ownedRegisteredLinkedWorktree` would refuse `lyx fabric pull` run in the hub's prime warp worktree — the main worktree of the warp clone.
**Fix:** Name the ownership kind (or a new ref-like/self-worktree kind) and the container/target semantics for `resetHardTo`, and state explicitly that pull in the prime worktree still passes.

### [NIT:consistency] `Fabric.ResetHard(sha)` cannot carry a request
**Demoted-from:** BLOCKING
**Section:** Scope In / Technical context "five primitives"
**Issue:** The document says both that executors are request-taking (`resetHardTo(pathRequest)`) and that the exported one-argument `Fabric.ResetHard(sha string) error` (`warpforward.go:33-35`) "becomes the gated executor" called as `f.ResetHard(...)` from `pull.go:233,267,285`; a one-arg exported method cannot let its three call sites declare ownership/dirtiness/force, and `warpforward.go:1-4` documents these delegations as a public API for out-of-package callers whose behaviour would silently change.
**Fix:** Decide whether `Fabric.ResetHard` is retained as a thin wrapper over `resetHardTo` with hardcoded declarations (and say which), or replaced, and state the effect on the exported surface.

### [NIT:scope] New `doc.go` is scanned but not allowlisted
**Section:** "Guard test template" / Scope In (docs)
**Issue:** `internal/fabricengine/doc.go` is added by this slice to hold the destruction rationale, sits in the scanned tree, and is absent from the complete allowlist — raw substring matching means a prose mention such as `os.RemoveAll(` or `fslink.Remove(` in that doc trips the guard.
**Fix:** State the constraint on doc.go's wording (no trailing-paren spellings) or add it to the allowlist with a reason.

### [NIT:scope] Markdown Link Integrity not acknowledged
**Section:** Constraints
**Issue:** The slice edits `manifest/designs/fabric-crucible-followups.md` and `manifest/roadmap.md` — both scan sources for `TestEnforcement_MarkdownLinks` — and adds `doc.go` references, but the Constraints list omits the invariant.
**Fix:** Add a one-line entry noting any new link (including `#anchor` and `../../internal/...` targets) must resolve.

## Verdict

REQUEST_CHANGES
Three unresolved gate-shape questions around clone's Location and the ResetHard executor.
MILL_REVIEW_END
