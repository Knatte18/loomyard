MILL_REVIEW_BEGIN
# Review: Replace reed's header pane with a status-line and Selvage — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: claude-sonnet-5 (system-reported as "Sonnet 5")
reviewed_file: plan/
date: 2026-09-18
```

## Findings

### [BLOCKING:consistency] Card 44 closes only four of the design doc's five Open items
**Location:** batch 7 / card 44 (`manifest/designs/reed-header-selvage.md`)
**Issue:** The current `## Open items` section of `manifest/designs/reed-header-selvage.md` has five bullets: (1) the `worktree` token, (2) the `apply.go` band flip, (3) Selvage's shell, (4) the daemon's exact lifecycle, and (5) "Whether the daemon should keep being spawned as a detached child of whichever `reed up` starts it, or move to a proper OS-level service/supervisor pattern, is not yet decided." Card 44's text says "Close the four `Open items`" and enumerates closures for exactly items 1–4 only; no card in the plan (checked all seven batch files) ever instructs closing item 5. The shipped mechanism already answers it: card 33 (batch 5) spawns the daemon via `os.Executable()` + `exec.Command` + `proc.Detach`, matching `internal/boardengine/spawn.go`/`internal/fabricengine/spawn.go` exactly, from three call sites (`up`, `resume`, `attach`) — i.e. it stays a detached child, never an OS-level service. If card 44 is executed as written, the rewritten doc's "The design" section will describe those three spawn sites as settled fact while its own "Open items" section still calls the same question "not yet decided," a self-contradiction within one document.
**Fix:** Reword card 44 to close all five items (not four), adding a sentence closing item 5: the daemon stays a detached child spawned from `up`/`resume`/`attach`, never an OS-level service/supervisor.

## Verdict

REQUEST_CHANGES
One card leaves a now-decided design-doc open item unclosed, shipping a self-contradictory doc; every other mechanism claim, rename, Move pair, DAG edge, and file citation checked against source verified clean.
MILL_REVIEW_END
