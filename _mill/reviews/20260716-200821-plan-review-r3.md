MILL_REVIEW_BEGIN
# Review: Fork-based cluster review in burler — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-07-16
```

## Findings

### [BLOCKING] AuditForks workdir is WorktreeRoot, must be Cwd
**Location:** Batch 2, Card 5 (point 3) — and the `AuditForks(sessionID, workdir)` seam it pins
**Issue:** The card passes `run.runner.layout.WorktreeRoot` as the workdir that encodes the `~/.claude/projects/<encoded-cwd>` transcript dir, but mux launches every pane at `layout.Cwd` (muxengine `new-session`/`split-window` always use `-c e.layout.Cwd`: lifecycle.go:332,541; spawn.go's `launchStrandLocked` sets no per-strand `-c`), so the claude process cwd — hence the encoded project dir — is `Cwd`, not `WorktreeRoot`. `hubgeometry.Layout` diverges the two whenever the operator invokes from a subdirectory (`RelPath != "."`), which burlercli explicitly anchors on `Cwd` "never WorktreeRoot" (cli.go:91). When they differ, AuditForks reads the wrong project dir: missing parent transcript → hard error, or missing `subagents/` → zero forks → spurious `ErrClusterForksMissing` on a clean round.
**Fix:** Pass `run.runner.layout.Cwd` (the pane's actual cwd) to `AuditForks`, and derive the transcript dir from that. The card-17 smoke will NOT catch this (fresh hub invoked at root, so Cwd==WorktreeRoot).

### [NIT] Card 9 leaves template_test.go "eight markers" comments stale
**Location:** Batch 4, Card 9
**Issue:** The card enumerates the eight→nine marker-count comment updates for `template.go` and `prompt.go` but omits `template_test.go`, which carries the same stale "eight required top-level markers" prose (lines 4-5, 42-43, 58-59) and a hard-coded required-marker list (~line 67) that must gain `cluster_rules` or the "all markers supplied" subtest fails.
**Fix:** Add `template_test.go`'s marker-list/count updates to Card 9's requirements alongside the other two files.

## Verdict

REQUEST_CHANGES
Sound plan; one wrong directory anchor in the fork-audit seam must be corrected.
MILL_REVIEW_END
