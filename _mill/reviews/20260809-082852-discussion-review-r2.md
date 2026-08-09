MILL_REVIEW_BEGIN
# Review: fabric: store the warp-URL binding in weft:main; fold bootstrap into clone (slice 10)

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opus
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-09
```

## Findings

### [BLOCKING:design] Reconcile envelope never gains the new keys
**Section:** Decisions § reconcile-backfill (line 123)
**Issue:** "Both are `omitempty` on the JSON envelope" is false — `runReconcile` (`internal/fabriccli/fabric.go:458-460`) emits `map[string]any{"pairs": r.Pairs}` and never serializes `ReconcileResult` itself, so struct tags on the two new fields have no effect on output; the discussion never names the CLI keys the way it did concretely for clone.
**Fix:** State the reconcile envelope keys explicitly (e.g. `"warp_binding"`/`"warp_binding_detail"`) and the condition under which each is added to the map.

### [BLOCKING:design] Who sets `record_failed` is unspecified
**Section:** Decisions § reconcile-backfill
**Issue:** `WarpBinding` is a field on the engine's `ReconcileResult`, but the commit and push that can fail happen CLI-side after `Reconcile` returns — the discussion never says whether the engine returns `recorded` optimistically and the CLI overwrites it to `record_failed`, or whether the outcome is a CLI-owned value entirely; the engine cannot know a push failed.
**Fix:** Name the owner of the final `WarpBinding`/`WarpBindingDetail` value and what the engine returns before the commit is attempted.

### [NIT:consistency] "five positionals" misstates today's signature
**Section:** Scope § In (line 33)
**Issue:** `CloneHub(cwd, warpURL, weftURL, subpath string)` (`internal/fabricengine/clone.go:85`) is four positionals, not five; five is the rejected *hypothetical* with `reset` added.
**Fix:** Say "four positionals" in Scope, or reword to reference the rejected five-positional shape.

### [NIT:scope] Call-site list names two non-call-sites
**Section:** Technical context § Call sites; Constraints § Hermetic Git
**Issue:** `internal/configcli/configcli_integration_test.go` mentions `CloneHub` only in a comment and never calls it; `internal/fabricengine/add_rollback_adopt_test.go` has no `CloneHub` call either; `clone_adopt_test.go` has 11 calls, not 9. The Hermetic-Git bullet's "`internal/configcli` must be checked if its `CloneHub` call site moves" rests on the same phantom.
**Fix:** Re-derive the list from a grep of `CloneHub(` and drop the two phantom entries, or state the count as approximate and unverified.

### [NIT:design] `normalizeWarpURL` table omits the local-path shape
**Section:** Testing § TDD candidate 1
**Issue:** Every integration fixture supplies a local absolute path (`filepath.ToSlash(warpSrc)`, e.g. `clone_test.go:123`), yet the table covers only https/scp URLs; "lowercases the scheme and host portion" is undefined for `/tmp/.../bare` and for a Windows `C:/Code/...` drive letter.
**Fix:** Add a local-filesystem-path row asserting the function is a no-op on it (or state the intended behaviour for a drive letter).

## Verdict

REQUEST_CHANGES
Reconcile's CLI reporting seam — envelope keys and `record_failed` ownership — is unspecified.
MILL_REVIEW_END
