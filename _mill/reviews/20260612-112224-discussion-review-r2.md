The shared infrastructure matches the discussion's claims. The `config.Load` reads from `_mhgo/<module>.yaml`, consistent with the discussion's note that `_mhgo/` is the committed config layer. The discussion proposes `.mhgo/` (gitignored) for machine-local state — a distinct layer not currently handled by `config.Load`, which the discussion acknowledges.

I have enough verification. The discussion is well-grounded, decisions have rationale and rejected alternatives, failure modes (corrupt state, crash-loop, concurrency) are addressed, and testing is concrete.

MILL_REVIEW_BEGIN
# Review: mhgo-mux-design

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-06-12
```

## Findings

### [NOTE] Launch-command config source unspecified
**Section:** Testing / Supporting decisions
**Issue:** Testing requires a configurable launch command (`%CLAUDE%`/`%SID%` templating, cheap placeholder for tests) but the discussion never says where that config is read from — `internal/config.Load` only reads the committed `_mhgo/<module>.yaml` layer, while muxpoc state lives in gitignored `.mhgo/`.
**Fix:** State whether the launch command comes from `_mhgo/muxpoc.yaml` (via `config.Load`), an env var, or a flag, so the planner picks one consistently.

### [NOTE] Spawn helpers are package-private to board
**Section:** Technical context (Codebase conventions)
**Issue:** The discussion says to reuse build-tagged spawn helpers, but `spawnSync`/`HideWindow` logic in `internal/board/spawn_windows.go` is unexported and board-specific (hardcodes `mhgo board sync`), so muxpoc cannot import it.
**Fix:** Clarify that muxpoc gets its own `spawn_windows.go`/`spawn_other.go` mirroring board's pattern (not shared code), as already implied by "mirror internal/board".

### [NOTE] Crash-loop guard window persistence undefined
**Section:** Decisions / daemon foreground poller
**Issue:** The crash-loop guard ("3 recoveries / 60 s") is in-memory in the daemon process; whether the rolling-window counter survives a daemon restart, and what `status` shows after "giving up", is unstated.
**Fix:** Note the counter is daemon-process-local (resets on daemon restart) and whether the give-up state is reflected anywhere observable.

## Verdict

APPROVE
Decisions, failure modes, and testing are well-grounded; remaining items are non-blocking clarifications.
MILL_REVIEW_END
