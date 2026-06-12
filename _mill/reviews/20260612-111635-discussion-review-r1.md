Confirmed: of the six subcommands, only `up`, `review`, `daemon`, `status` (partially, via stripped-env) are described. `down` and `attach` behavior are never specified beyond being listed. `status` output is only partially specified (stripped env keys). This is a scope/ambiguity gap for a plan writer.

I have sufficient grounding to produce the review.

MILL_REVIEW_BEGIN
# Review: mhgo-mux-design

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-06-12
```

## Findings

### [GAP] State file in committed dir, holds machine-local IDs
**Section:** Scope (In) / Decisions
**Issue:** State is placed at `<cwd>/_mhgo/muxpoc-state.json`, but `_mhgo/` is the committed config layer (init.go); only `.mhgo/` is gitignored (init.go line 119), and `docs/shared-libs/state.md` says session/pane IDs are machine-local and must be gitignored.
**Fix:** Decide the path explicitly — either `<cwd>/.mhgo/` (gitignored, matches state.md) or extend the gitignore block — and state it in the discussion.

### [GAP] `down` and `attach` subcommand behavior unspecified
**Section:** Scope (In), line 41
**Issue:** Six subcommands are listed but only `up`/`review`/`daemon` have described behavior; `down` (kill-server? clear state? leave state for later resume?) and `attach` (pop maximized terminal?) are never defined.
**Fix:** Add a one-line contract per subcommand, especially whether `down` deletes muxpoc-state.json or preserves it.

### [GAP] No concurrency/partial-write handling for state
**Section:** Decisions / Testing
**Issue:** The foreground daemon writes state on recovery while interactive subcommands (`up`, `review`) also write it; `internal/lock` is listed as available but the discussion never says the state writes are locked/atomic, and no partial-write/corrupt-state failure mode is addressed.
**Fix:** State that muxpoc-state.json uses atomic write + `internal/lock`, and name the corrupt/missing-state recovery behavior.

### [NOTE] Daemon poll interval and crash-loop guard unstated
**Section:** Decisions (foreground poller)
**Issue:** The poller has no named interval (mux.md suggests ~500ms) and no crash-loop guard, which the real mux design explicitly includes; unbounded respawn could loop on a permanently-failing pane.
**Fix:** Name the poll interval (configurable) and either include a simple loop cap or explicitly defer it with rationale.

### [NOTE] `status` output contract only partially specified
**Section:** Testing / Scope
**Issue:** `status` is required to surface `strippedEnvKeys`, but its full output (pane liveness, session ids, dead-pane state) is otherwise undefined.
**Fix:** Sketch the `status` JSON fields so the plan writer has a target shape.

## Verdict

GAPS_FOUND
State-file location/gitignore conflict, and `down`/`attach` plus state-concurrency behavior need definition before planning.
MILL_REVIEW_END
