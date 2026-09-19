MILL_REVIEW_BEGIN
# Review: reed: born-as-strand for loom start's operator attach

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: claude-opus-4-class (self-reported; exact build unverifiable from inside)
reviewed_file: _mill/discussion.md
date: 2026-09-19
```

## Findings

### [BLOCKING:design] Watchdog call-site test needs a seam the design excludes
**Section:** Testing ("In-process … through an injected seam or a recorded call") vs `watchdog-seam-lives-in-reedengine`
**Issue:** The decision has `start.go`'s RunE call an exported `reedengine` package function directly, so there is no injection point and no recorder; under `go test` `suppressWatchdogSpawn` is true and the call leaves no observable trace, making the named Tier 1 assertion ("RunE reaches the seam with the expected hub path and tmux path, outside the `mustAttach` gate") unwritable without a structural addition (e.g. a func field on `loomCLI`) the Decisions section never sanctions.
**Fix:** Decide it here — either add a named seam field on `loomCLI` as part of the design, or drop the in-process assertion and state that the call site is covered by review plus the out-of-process smoke tier only.

### [NIT:scope] LYX_REED_WATCHDOG=off does not suppress the spawn — settle it
**Section:** Testing, "the plan writer should confirm whether that gate suppresses the *spawn* or only the daemon's behaviour"
**Issue:** Source answers this: `reedcli.ensureWatchdogSpawned` consults only `suppressWatchdogSpawn`/`hubPath` and ignores config; the env key reaches `cfg.Watchdog`, which `enterSession` (`internal/reedcli/watchdog.go:149`) uses only to skip a worktree's watch goroutine. The daemon process still spawns.
**Fix:** State the answer and carry the teardown-reap branch as unconditional inventory for `internal/loomcli`'s smoke tier, matching the round-2 principle this document already adopted ("a conditional work inventory is not a decision").

### [NIT:design] reedengine gains a hub-only dependency shared with standalone mode
**Section:** `watchdog-seam-lives-in-reedengine`, rejected alternatives
**Issue:** The alternatives weigh only import direction and cycles (verified: no `fabricengine → reedengine` edge exists, so no cycle). Not weighed: `reedengine` is also the standalone engine (`internal/standalonegeom/reedgeom.go`, `internal/burlercli/wiring.go`), and this move puts hub-only geometry (`fabricengine.HubScratchDir`) plus `proc`/`os/exec` into the package standalone links — `internal/burlercli`'s `TestProductionFiles_NeverReferenceHubWatchdogMechanism` scans only burlercli's own files, so nothing pins the new boundary.
**Fix:** Record the standalone consequence explicitly (one sentence) so the placement is a considered trade rather than an unexamined one.

## Verdict

REQUEST_CHANGES
One testability decision missing; the rest is well-grounded and matches source.
MILL_REVIEW_END
