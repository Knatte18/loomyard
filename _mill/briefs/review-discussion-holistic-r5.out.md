MILL_REVIEW_BEGIN
# Review: Diagnostic tracing (trace) on the logger module

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewer_self_id: Claude Opus 4.8 (claude-opus-4-8[1m])
reviewed_file: _mill/discussion.md
date: 2026-07-31
```

## Findings

### [GAP] Header record source undefined without the root hook
**Section:** `sink-open-triggers` / `no-arming-under-test` / Testing ("Sink naming and lazy open")
**Issue:** The header record is "composed by the root hook at startup," but `no-arming-under-test` suppresses that composition under `go test`, yet the untagged unit test asserts the first sink line is a header record — the seam/`LYX_TRACE=1` open path has no defined header source.
**Fix:** State who composes the header when the sink opens without the root hook (e.g. `logger` self-composes a minimal header from trace-ID/pid it can resolve, root hook enriches with command/argv), and reflect that in the seam contract.

### [GAP] Header carries worktree-root but is "composed at startup" vs lazy geometry
**Section:** `sink-open-triggers` vs `lazy-sink-open`
**Issue:** The header includes "worktree root," but `lazy-sink-open` defers all geometry resolution to the first sink-open trigger to avoid a git spawn per command — so worktree root is not available when the header is "composed at startup," a contradiction.
**Fix:** Split the header's static fields (command/argv/trace-ID/pid, known at startup) from geometry-derived fields (worktree root, filled at sink-open in `sinkOnce`), and say so explicitly.

## Verdict

GAPS_FOUND
Header-record composition timing and ownership are underspecified across the two sink-open paths.
MILL_REVIEW_END
