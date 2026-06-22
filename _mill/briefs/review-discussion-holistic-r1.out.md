The board read ops are consumed by `internal/ide/menu.go` and `internal/board/cli.go` (the CLI contract). After the change, a corrupt `tasks.json` will now propagate an error to these read paths where previously it silently yielded an empty board. The discussion declares this intended ("failing loudly forces the corrupt file to be dealt with") but only details the muxpoc read-consumer fallout; the board-side read consumers are not enumerated. This is borderline — the decision covers the principle, but the plan writer doesn't get the board-side blast radius spelled out the way the muxpoc side is. I'll note this rather than gap it, since the decision is explicit that corruption-as-error applies to board.Load too.

Let me do a final check: the discussion says "Update all six callers" for the LoadState signature, and lists `attach.go, down.go, review.go, status.go` plus `up.go` and `daemon.go` = six callers, plus the smoke test. Grep confirmed exactly those. Good. I now have a complete picture.

The discussion is unusually thorough and well-grounded. All file references, line numbers, function signatures, decision rationales, and rejected alternatives check out against source. I found no GAPs that would block plan writing. I have two NOTE-level observations.

MILL_REVIEW_BEGIN
# Review: Adopt internal/state in board and muxpoc

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-06-22
```

## Findings

### [NOTE] Board read-consumer fallout not enumerated
**Section:** Decisions › corruption-surfaces-as-error
**Issue:** The decision details corruption-as-error consequences for muxpoc's six callers but not for board's read ops (`GetTask`, `ListTasksBrief`, `ListTasksFull` in board.go:192-233), which all call `store.Load()` and will now surface errors to `internal/ide/menu.go` and the CLI (`cli.go`) where they previously returned an empty/silent board.
**Fix:** Add one line noting the board-side read consumers now propagate corruption errors, mirroring the muxpoc behavioral-consequence list.

### [NOTE] HealthCheck unaffected — worth stating explicitly
**Section:** Scope (Out) / Technical context
**Issue:** `board.HealthCheck` (board.go:180) and its `TestHealthCheckPassesCorruptFile` (board_test.go:160) deliberately tolerate corrupt-but-readable tasks.json, but it reads via raw `os.ReadFile`, not `store.Load`, so the corruption change does not touch it; the discussion does not mention this nearby test that superficially appears to conflict with corruption-as-error.
**Fix:** Note that HealthCheck bypasses Load and its corrupt-file-passes test stays unchanged, so a plan writer doesn't mistakenly "fix" it.

## Verdict
APPROVE
Thoroughly source-grounded; no blocking gaps, two non-blocking notes on read-path fallout.
MILL_REVIEW_END
