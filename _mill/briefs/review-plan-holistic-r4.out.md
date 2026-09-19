MILL_REVIEW_BEGIN
# Review: reed: per-hub daemon reaps orphaned sessions — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: Claude Sonnet 5 (claude-sonnet-5)
reviewed_file: plan/
date: 2026-09-19
```

## Findings

### [BLOCKING:design] ReapSession's throwaway Geometry literal bypasses the sole-constructor rule
**Location:** batch 1, card 2 **Issue:** Card 2 has `ReapSession` build `geom: Geometry{SocketKey: socketKey, SessionName: sessionName}` directly in `internal/reedengine/overlay.go`, but CONSTRAINTS.md's Told-Geometry Invariant states "`internal/hubgeom`/`internal/standalonegeom` are the only `Geometry`-struct constructors." Verified: no production (non-test) file anywhere in the repo builds a `Geometry{}` literal outside `internal/hubgeom/*.go` and `internal/standalonegeom/*.go` — every other hit is a `_test.go` fixture. Batch 5 (card 20/21) explicitly declines to touch CONSTRAINTS.md ("this batch touches neither `docs/overview.md` nor `CONSTRAINTS.md`"), so no carve-out is proposed for this new exception, and this invariant has no named AST enforcement test (unlike sibling invariants), so review discipline is the only backstop. **Fix:** either route the throwaway engine's Geometry through a sanctioned constructor path, or add an explicit CONSTRAINTS.md exemption for this shape in the same task (as other narrow exemptions, e.g. Config Strictness's degrading-package list, are recorded inline).

### [BLOCKING:scope] Card 13 calls Engine.ShellPath() without lock.go in Context
**Location:** batch 3, card 13 **Issue:** Card 13's Requirements direct updating four call sites to `runWatchdogLoop(ctx, h.Path, tmuxPath, eng1.ShellPath(), watchdogDefaultTiming())`, but `Engine.ShellPath()` is declared in `internal/reedengine/lock.go` (card 1), and card 13's `Context:` lists only `internal/reedcli/watchdog.go`. Card 12, wiring the same accessor at the spawn site, correctly lists `lock.go` in its own Context — card 13 does not, so per the Context Completeness rule this is a cold-start gap. **Fix:** add `internal/reedengine/lock.go` to card 13's `Context:`.

### [NIT:consistency] Card 18's Windows-degradation rationale doesn't hold on the platform the test runs on
**Location:** batch 4, card 18 **Issue:** Card 18 justifies asserting only pane root pids ("an empty shell is exactly the configuration whose Windows descendant walk degrades to returning the roots unchanged") — but `internal/reedengine/proctree_linux.go`'s `descendantClosurePIDs` never reads `e.cfg.Shell` at all and always attempts the full `/proc`-based closure regardless of `shellPath`; the roadmap's "Real-Linux validation" item confirms this suite runs on Linux, where an empty shell has no effect on this call. The test itself remains valid, but its stated justification is factually wrong for the platform it actually executes on. **Fix:** reword the rationale as a precautionary match to Windows semantics rather than a claim about this platform's own behavior.

## Verdict

REQUEST_CHANGES
Two BLOCKING findings: a Told-Geometry sole-constructor conflict in card 2, and a Context gap in card 13.
MILL_REVIEW_END
