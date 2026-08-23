MILL_REVIEW_BEGIN
# Review: landing: parent-fabric resolution chain

```yaml
duration_s: 374.0
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Opus-class); system identifies the exact model as claude-opus-5
reviewed_file: _mill/discussion.md
date: 2026-08-23
```

## Findings

### [BLOCKING:design] Drive-only refusals are unseen on the `run` path
**Section:** `scalar-read-errors-refuse-or-defer-by-consumer` (the `LoadConfig`/`Open` paragraph)
**Issue:** The decision justifies the new hard failures by "refusing early names the cause precisely" on `drive`'s envelope, but `drive` is normally the detached child `run.go:208` spawns — its envelope goes to `LoomDriverLog`, and `awaitRunLock` returns `awaitRunLockChildDied`, so the operator running `lyx loom run` sees only `"loom: driver did not take the run lock; see <log>"` (run.go:260). Every pre-existing `drive` precondition is guaranteed by `run` beforehand (`run` seeds the status file; `wire()` loads the other configs); `landing.yaml` is the first one that is not, since `wire()` never loads it.
**Fix:** State the disposition explicitly — either accept the log-only symptom as a named consequence, or decide to load `landingshed.LoadConfig` in `wire()` (alongside the one existing strict load, `loomengine.LoadConfig`) and name the cost that imposes on `status`/`pause` and on `wiring_test.go`'s seed set.

### [BLOCKING:design] Refusal-guard tests claimed pure with no seam to hold them
**Section:** `## Testing` → `internal/loomcli` ("The two refusal paths … both are pure-function tests needing no fixture")
**Issue:** `assembly-seam-takes-plain-values` places both guards in `drive.go` *above* the `landingDeps` call ("`drive.go` therefore owns, above this call: … and both refusal guards"), i.e. inline in the `RunE` closure, which is reachable only through the CLI with a resolved `*lyxcwd.Location`. `resolveParentBranch`'s existing tests do not cover what the discussion itself says is new ("`drive`'s assembly *calls* it with an empty flag and propagates"), and the `parentBranch == taskBranch` clause has no pure function at all.
**Fix:** Name a second extracted pure helper holding both clauses (the same extraction rationale `wire`/`landingDeps` already carry), or drop the "pure-function tests needing no fixture" claim and state what tier those two assertions actually land in.

### [NIT:consistency] Roadmap Done entry's forward reference goes stale
**Section:** `## Technical context` → "Files that will be touched" (docs list)
**Issue:** `manifest/roadmap.md:183` (the `loom: convert to a Shed recipe` Done entry) says "`Env.Landing` is deliberately left unfilled by `internal/loomcli`, preserving the pre-existing gap the new `landing: parent-fabric resolution chain` Planned item above closes" — false once this lands, and the docs list names only the item's own stale "no worktree-listing helper exists" claim. `manifest/designs/loom.md:67`'s "swaps in the real … producers once `landing: Publish + Finalize producers` lands" is the same shape.
**Fix:** Add both cross-references to the roadmap/loom.md correction list so the same commit clears them.

## Verdict

REQUEST_CHANGES
Two decisions missing: who sees the new refusals, and where the guards live for testing.
MILL_REVIEW_END
