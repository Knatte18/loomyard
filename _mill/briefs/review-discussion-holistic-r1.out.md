MILL_REVIEW_BEGIN
# Review: Scout owns its own lyxcwd-based geometry accessors (drop Options.AnchorRoot threading)

```yaml
verdict: GAPS_FOUND
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-08
```

## Findings

### [GAP] Byte-identical rule contradicts the assert-no-callers fix
**Section:** Constraints ("Every resolved path must be byte-identical before and after … in-hub and out-of-hub") vs `fix-assert-no-callers-literal` + Scope Out ("Any observable CLI behaviour change").
**Issue:** Verified at `internal/scoutcli/cli.go:576-599`: out-of-hub `assert-no-callers` today leaves `worktreeRoot` empty, so the daemon state path *does* change (relative → absolute) — an observable behaviour change the discussion elsewhere forbids absolutely.
**Fix:** State the byte-identical rule with an explicit carve-out naming out-of-hub `assert-no-callers` as the one intended delta, so a plan writer does not read the rule as grounds to back out the fix.

### [GAP] assert-no-callers pin test has no defined seam
**Section:** Testing item 2 vs Technical context ("Whether the hand-built literal is replaced by a `buildOptions` call or merely gains a `Layout` field is mill-plan's call").
**Issue:** The test is specified as asserting the constructed `Options.Layout`, but the construction site is a closure-local literal inside `RunE`; the discussion leaves both the seam ("may require a small seam") and the literal-vs-`buildOptions` choice undecided, so the pin test's target is undefined.
**Fix:** Decide the seam (e.g. a `baseOptions(registry, dir, layout, lang, timeout)` helper `buildOptions` also calls) and state what the test calls.

### [GAP] docs-in-same-commit misses two worktreeRoot mentions
**Section:** `docs-in-same-commit` ("Nothing else").
**Issue:** `doc.go:138-139` documents the seam as `ensureServer(ctx, lang, entry, targetDir, worktreeRoot, timeout)`, `doc.go:236` repeats "(worktreeRoot, lang)", and `ensureserver.go:1` opens "the EnsureServer(lang, worktreeRoot) -> LSPConn seam" — none are in the decision's list, and the decision's "Nothing else" conflicts with Technical context's separate instruction to reword `ensureserver.go:301-304`.
**Fix:** Enumerate all four sites (doc.go:136-142, doc.go:215-219, doc.go:236, ensureserver.go:1 and 301-304) in the decision.

### [NOTE] no-nil-layout rationale weaker than stated
**Section:** `no-nil-layout-check`.
**Issue:** `Layout` is dereferenced only in `ensureSupervised`, reached only when `entry.HasNativeDaemon` (`refs.go:64-65`); a nil `Layout` on any non-native language never panics, so "a nil deref is the correct, immediate signal" holds only for Go.
**Fix:** Record that the fail-fast argument is language-conditional, or note it as accepted.

### [NOTE] Synthesized Location is a fabricated geometry
**Section:** `out-of-hub-synthesized-location`.
**Issue:** The only production precedents for hand-built `lyxcwd.Location` are `fabricengine/clone.go:125` and `hostlayout.go:30` — the geometry owner; the synthesized value's `HubPath`/`RepoName`/`AnchorPath()` are fictions any accessor other than `WorktreePath()` would read wrong.
**Fix:** State that the synthesized `Location` is consumed only by `DaemonStateFile`/`DaemonLock`, and say so in the helper's doc comment.

## Verdict

GAPS_FOUND
Three gaps: behaviour-change contradiction, undefined test seam, incomplete doc-update list.
MILL_REVIEW_END
