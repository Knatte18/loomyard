MILL_REVIEW_BEGIN
# Review: fabric: shrink hubgeometry to the minimal illusion primitive (slice 7)

```yaml
verdict: GAPS_FOUND
reviewer_model: sonnetxhigh
reviewer_self_id: claude-sonnet-5 (Sonnet 5, Anthropic)
reviewed_file: _mill/discussion.md
date: 2026-08-05
```

## Findings

### [GAP] clone.go:112 "fails every time" claim doesn't hold under the stated fallback rules
**Section:** `anchor-read-ownership` ("Correction to that ordering, and a required fix")
**Issue:** The decision asserts `hubgeometry.Resolve(hostWorktreePath)` at `clone.go:112` "fails every time" under the strict gate. Traced against the actual code and this discussion's own rules: at step 5 no marker exists yet, so per `anchor-naming` `AnchorRel` falls back to `"."`, making `AnchorPath() == WorktreePath()`. `hostWorktreePath` is a freshly-cloned repo root, so `git rev-parse --show-toplevel` run at that same path (the literal `cwd` argument) returns that same path — `cwd == AnchorPath()` holds, and the strict-equality gate should pass, not fail, contradicting the "fails every time" premise this required fix is built on.
**Fix:** Verify the actual pass/fail outcome (spike or a quick table trace) before batch 1 locks in a fix scoped as "required" for a failure; if the call in fact succeeds, reframe the `clone.go:112` change as a spawn-avoidance/clarity simplification rather than a correctness fix, and adjust the "Clone still succeeds end-to-end..." test scenario's assumed before-state accordingly.

### [NOTE] `boardengine` misattributed as a BoardDir production caller; "ten" undercounts
**Section:** `boarddir-ownership`
**Issue:** `internal/boardengine` is listed among BoardDir's "ten production callers outside it," but grep shows `hubgeometry.BoardDir` appears only in `boardengine`'s comments (`config.go:21`, several `_test.go` files), never in its production call sites. Separately, `fabriccli` alone already contributes 9 call sites (`fabric.go` ×8, `weft_verbs.go` ×1) — matching "`fabriccli` ×9" — but adding `boardcli` (2), `configsync` (1), `fabricengine` (4+ across `clone.go`/`reconcile.go`/`junctionnames.go`), and `ideengine` (1) pushes the real total well past ten.
**Fix:** Recount and correct the caller list/number before it lands in the module doc or `CONSTRAINTS.md`; drop `boardengine` from the list unless a real production call site exists.

### [NOTE] "envsource is a stdlib-only leaf today" is inaccurate as stated
**Section:** `config-path-move`
**Issue:** `envsource.go:12` currently imports `internal/hubgeometry` (for today's `hubgeometry.DotEnv` call at `:22`), so envsource is not stdlib-only "today" — it becomes stdlib-only only after this task's own "Call-site consequence" bullet removes that import. The two statements in the same decision are in tension.
**Fix:** Reword to "envsource becomes a stdlib-only leaf after this move" (matching the accurate "loses an import rather than gaining one" bullet already present).

## Verdict

GAPS_FOUND
One required-fix's failure premise appears technically wrong; verify before batch 1 locks scope.
MILL_REVIEW_END
