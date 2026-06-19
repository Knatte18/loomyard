MILL_REVIEW_BEGIN
# Review: weft engine: paths geometry, paired worktrees, lyx weft

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-06-19
```

## Findings

### [NOTE] Detached `lyx weft push` cwd-resolution unspecified
**Section:** detached-coalesced-push / lyx-weft-surface (`sync`)
**Issue:** Board's `spawnSync` passes the target path explicitly (`--board-path <abs>`) and does not set `cmd.Dir`; the ported weft `sync` spawns `lyx weft push` detached, but `lyx weft push` re-resolves the weft worktree from cwd, and the decision does not state how the detached child learns which worktree to push (it would rely on inherited parent cwd, unlike board's explicit flag).
**Fix:** State that the detached `lyx weft push` either inherits the parent's cwd or receives an explicit path arg (mirroring board's `--board-path`), so a cwd-independent spawn is well-defined.

### [NOTE] Host pristine vs. worktree's own config load not reconciled
**Section:** §2 / host-pristine-enforced
**Issue:** `worktree.RunCLI` loads its `branch_prefix` via `LoadConfig(cwd, ...)`, which requires `<cwd>/_lyx` to exist (`config.FindBaseDir`); the weft model makes the host pristine, so on a fresh host the operator's cwd `_lyx` is itself a junction the hub-creator must have seeded — the discussion never states that paired spawn presumes a pre-seeded host `_lyx` junction in the Prime to even load its own config.
**Fix:** Note that `lyx worktree add` still requires a resolvable `_lyx` (junction) at the operator's cwd for its own config load, and that this is the hub-creator's responsibility (consistent with hard-require).

### [NOTE] `status` resilience covers broken junction, not missing weft worktree
**Section:** weft-config-pathspec-only / weft-status-semantics
**Issue:** Config reads from `Join(WeftWorktree(), RelPath)` so a broken *junction* never blocks `status`, but `config.Load` still requires that weft worktree's `_lyx` to exist; a missing/removed weft worktree (not just junction) would fail `status` startup, which the "status always reports drift" claim does not bound.
**Fix:** Clarify that the status-still-runs guarantee covers a broken junction with an intact weft worktree, and is not claimed for a missing weft worktree.

## Verdict
APPROVE
Decisions are well-grounded against source; only minor clarifying notes, no blocking gaps.
MILL_REVIEW_END
