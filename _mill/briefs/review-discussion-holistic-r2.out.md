MILL_REVIEW_BEGIN
# Review: scoutengine told-geometry (optional uniformity pass)

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-18
```

## Findings

### [BLOCKING:design] Hub-mode test cannot distinguish AnchorPath from WorktreePath
**Section:** Testing → "the hub-mode branch, in a new `cli_integration_test.go`" (+ Constraints → "Zero behavioural change")
**Issue:** The acceptance property demands byte-identical paths "in hub mode … at both unanchored and subpath-anchored geometries", but the only named hub test uses `hubforge.NewHub(t, ".")`, where `AnchorPath() == WorktreePath()`, so an implementation writing `layout.WorktreePath()` instead of `layout.AnchorPath()` passes; `cmd/lyx/constructoranchoring_test.go:132-139` already states in-source that rows passing `l.AnchorPath()` in are tautological w.r.t. anchoring and that the real proof must live at the production call site (perch/webster each named such a subpath-anchored test), and scout's rows become exactly that shape after this change.
**Fix:** Name a subpath-anchored hub case too — `hubforge.NewHub(t, "backend")` with `lookupContext(h.Location.AnchorPath(), …)` (note `PrimeWorktree()` is `WorktreePath()` and would trip `ErrCwdOutsideAnchor`), asserting the anchor root is the anchor, not the worktree root.

### [BLOCKING:scope] New hubforge test forces a `testmain_test.go` that is not in scope
**Section:** Scope → In (`internal/scoutcli/cli_integration_test.go`); Constraints
**Issue:** `internal/scoutcli` has no `TestMain`/`HermeticGitEnv` today; `cmd/lyx/hermeticenv_test.go` walks every `*_test.go` tag-agnostically, treats `hubforge.NewHub` as a git-spawn token, and `internal/scoutcli` is not on `allowedNonHermetic` — so the new file fails `TestHermeticGitEnv_GitSpawningPackagesHaveTestMain` (perchcli carries `internal/perchcli/testmain_test.go` precisely for this).
**Fix:** Add `internal/scoutcli/testmain_test.go` to the Files list and acknowledge the Hermetic Git Test Environment Invariant in Constraints.

### [BLOCKING:scope] Two stale-comment sites omitted from the doc enumeration
**Section:** Scope → In (`ensureserver.go`, `doc.go`); Decisions → "documentation lands in doc.go and file headers"
**Issue:** Scope names only `ensureserver.go`'s socket-path comment and `doc.go`'s "Daemon state and concurrency" section, but `internal/scoutengine/ensureserver.go:1` ("the EnsureServer(lang, layout) -> LSPConn seam") and `internal/scoutengine/doc.go:138-139` (in the "# The EnsureServer seam" section, spelling the signature as `…targetDir, layout, timeout)`) both describe the shape being deleted, contradicting the section's own "must be deleted or rewritten" Note.
**Fix:** Add both sites to the Files list, or state the doc rule as "every `layout` mention in `scoutengine` production comments", so the enumeration is closed rather than sampled.

### [NIT:scope] Test prose still says "layout" after conversion
**Section:** Testing → mechanical conversions
**Issue:** `refs_integration_test.go:79/181`, `supervised_integration_test.go:93`, `ensureserver_integration_test.go:130/178` carry "same layout"/"explicit layout" comments that survive a purely mechanical `Location`→string swap.
**Fix:** Note that these comment lines reword with the conversion.

## Verdict

REQUEST_CHANGES
Anchored hub evidence, a required TestMain file, and two stale-comment sites are missing.
MILL_REVIEW_END
