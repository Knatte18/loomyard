MILL_REVIEW_BEGIN
# Review: codeintel V1 — LSP-backed lookups (Go-only, CLI + EnsureServer)

```yaml
verdict: GAPS_FOUND
reviewer_model: opus
reviewed_file: _mill/discussion.md
date: 2026-07-29
```

## Findings

### [GAP] State-file/spawn-lock machinery unmapped to native
**Section:** `state-file-location-and-content`, `concurrency-locking`, `native-strategy-wire-compatibility`
**Issue:** `native` (the sole V1 production path for Go) respawns `gopls -remote=auto` fresh per call, dials no recorded address, and relies on gopls's own daemon dedup — so it owns no lyx daemon PID, yet the state-file, worktree spawn-lock, and two-part staleness decisions are all written as generic "EnsureServer" behavior, leaving it unstated whether native writes a state file or takes the lock at all.
**Fix:** State per-strategy which machinery each uses — that native uses only the probe (no state file, no spawn-lock, no staleness/restart), and the state-file + `internal/lock` spawn-race + staleness apparatus is exercised solely by the `supervised` proof path.

### [NOTE] Batch-vs-single discriminant and shape discontinuity unstated
**Section:** `batch-mode-cli`
**Issue:** With `MinimumNArgs(1)`, nothing names the trigger that selects the old single-symbol envelope (`{"references":[...]}`, engine error → `output.Err`) vs. the new per-symbol batch shape; the natural rule (len==1 single, len>1 batch) implies a JSON-shape discontinuity at the 1→2 arg boundary.
**Fix:** State that arg count is the discriminant and that single-arg calls keep the legacy top-level shape while 2+ args switch to the per-symbol array.

### [NOTE] `net/http` for toolchain manager ungrounded
**Section:** Constraints (Codeintelengine Leaf Invariant), `toolchain-manager-authority`
**Issue:** The Constraints note lists "`os/exec` and `net/http` for the toolchain manager," but every described step installs via `go install ...@<pinned>` (which does its own network) against a static registry `PinnedVersion` — no Decision names an HTTP call.
**Fix:** Either name the HTTP fetch the manager needs (e.g. version discovery) or drop `net/http` from the stated dependency set.

## Verdict

GAPS_FOUND
One strategy-to-machinery mapping gap for native; two minor clarifications.
MILL_REVIEW_END
