MILL_REVIEW_BEGIN
# Review: Built-in CLI help: lyx self-documents modules & commands

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-06-27
```

## Findings

### [GAP] "Only muxpoc has shared pre-dispatch" is false
**Section:** Technical context (muxpoc gotcha) / integration-style-c-preserve-seam
**Issue:** board (`internal/board/cli.go:67-101`: builds `cfg`/`b := New(cfg)`), weft (`internal/weft/cli.go:82-104`: `l`/`cfg`/`pathspec`), and ide (`internal/ide/cli.go:31-41`: `l`) each resolve cwd-dependent config/layout once and share it across all subcommands — conditional on `--board-path`/`--weft-path` — yet the discussion states "only muxpoc has shared pre-dispatch across multiple subcommands" and that handlers otherwise "move into RunE essentially verbatim." board and ide even resolve before the no-arg check today, so the same "no-arg listing must not require a git repo/config" hazard the muxpoc gotcha guards against applies to all three.
**Fix:** Correct the "only muxpoc" claim; specify that board/weft/ide also lift their shared resolution into a `PersistentPreRunE` (skipped on the no-`Run` parent's listing path) and preserve the `--board-path`/`--weft-path` bypass branch.

### [NOTE] How PreRunE-built cfg/layout reaches each RunE is unspecified
**Section:** exit-and-error-contract / muxpoc gotcha
**Issue:** The design threads `exitState` via context, but never says how the `PersistentPreRunE`-built `cfg`/layout (muxpoc, plus board/weft/ide per the GAP) is handed to each subcommand's `RunE` (e.g. `cmdUp(out, cfg)` needs `cfg`).
**Fix:** State the resolved value lives in a `Command()`-closure variable the PreRunE populates and each RunE closes over (or is carried in context like `exitState`).

## Verdict

GAPS_FOUND
Shared cwd-dependent pre-dispatch in board/weft/ide is unaddressed; "only muxpoc" claim is inaccurate.
MILL_REVIEW_END