MILL_REVIEW_BEGIN
# Review: fabric: one ownership-and-dirtiness gate for all destruction (slice 12)

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Anthropic Claude, Opus-class (env reports claude-opus-5)
reviewed_file: _mill/discussion.md
date: 2026-08-10
```

## Findings

### [BLOCKING:design] Request structs cannot carry the checks' own inputs
**Section:** `gate-call-shape` / `ownership-is-a-closed-enum` / `r6-validation-asymmetry-folds-into-the-gate`
**Issue:** Both declared shapes list only `{what, container, target, slug, ownership, dirtiness, force}` and `{what, repoDir, branch, …}`, but the gate is required to resolve `primaryWeftBranch(l *lyxcwd.Location)` (`cleanup.go:206`) and `validateWorktreeSlug(slug, junctionNames []string)` (`slug.go:30`) internally — neither `l` nor the junction-name set is in either struct, and the Constraints section's "the gate takes a `*lyxcwd.Location`" contradicts the struct definitions.
**Fix:** State the full required field set (Location and/or the config-derived junction names) for each shape, and say whether that leaves the hermetic "pure logic over the request struct, needs no git" tests still pure.

### [BLOCKING:design] Link-removal enumeration is unreliable
**Section:** Technical context, "The five primitives and their current sites"
**Issue:** The `link removal` row lists `weftwiring.go:156`, `portals.go:57`, `junction.go:259` — but `junction.go:259` is `os.Remove(link)`, not `fslink.Remove`, and the production `fslink.Remove(` sites at `unwire.go:143` (slug-derived, `WorktreePath(l, slug)/…/_board`), `junction.go:161`, `junction.go:311` and `junction.go:461` are absent entirely, so the enumeration method missed a whole file and mislabelled a listed site.
**Fix:** Name the enumeration method used (and re-run it), then restate the primitive's site list with each site's disposition, since the In-scope promise is "every destructive call site".

### [BLOCKING:decision] Sites the banned tokens hit with no stated disposition
**Section:** "Guard test template" / "Not all of these are gate candidates"
**Issue:** `warpforward.go:34` (`f.warp.ResetHard(sha)`, the exported `Fabric.ResetHard` delegation) matches `.ResetHard(`, and `hook.go:160` and `junction.go:259` match `os.Remove(`; none is allowlisted (`destroy.go`, `gitexclude.go`, `warpprobe.go`, `ancestors.go`, `index.go`) and none is named a gate candidate, so the one-commit slice lands with a red guard.
**Fix:** Give each a disposition — gate executor or allowlist entry with its reason — in the discussion, before plan writing.

### [NIT:consistency] `teardownHub` call-site count is 13, not 14
**Section:** `rollback-paths-go-through-the-gate`, gap #2, Q&A log
**Issue:** The document says "14 call sites" three times while its own enumeration (`:243,245,268,279,288,308,325,335,346,349,361,371,379`) lists 13, which grep of `clone.go` confirms.
**Fix:** Correct the count to 13 in all three places.

## Verdict

REQUEST_CHANGES
Gate input fields, link-removal enumeration, and three undispositioned guard-matching sites must be settled first.
MILL_REVIEW_END
