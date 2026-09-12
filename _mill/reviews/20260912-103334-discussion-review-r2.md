MILL_REVIEW_BEGIN
# Review: self-report Tier 1: Go-detected structural anomalies

```yaml
duration_s: 678.0
verdict: REQUEST_CHANGES
reviewer_model: opus
reviewer_self_id: Claude (Anthropic) — Opus-class model, exact build unverifiable from inside the session
reviewed_file: _mill/discussion.md
date: 2026-09-12
```

## Findings

### [BLOCKING:design] Bouncer-row discriminator for ledger discovery unnamed
**Section:** `### ledger-discovery` **Issue:** Non-Bouncer rows also publish non-empty `output` (`internal/shedadapters/singlellm.go:165` returns `OutputPointer{Path: spec.OutputFiles[0]}`, e.g. `discussion.md`/`plan.md`), so "every non-empty `output` belonging to a Bouncer row" needs a discriminator the discussion never states — and the two candidates are exactly the rejected options: row-name/recipe knowledge, or the `shedadapters`-owned filename shape. **Fix:** Pin how a history entry is classified as a ledger pointer, and state what happens when a non-ledger path is handed to the fail-loud accessor (skip-on-parse-failure vs. never-offered).

### [BLOCKING:design] Detect-and-file is unreachable on drive's error paths
**Section:** `### detection-site` / `## Constraints` (discovered) **Issue:** `internal/loomcli/drive.go:133-136` returns an error envelope immediately on any non-nil `shed.Run` error, so the post-`Run` step never executes for a producer failure (`state: failed`, `run.go:219`) — and because the entry observation lives only in this process's memory, a crash-resume followed by a failing producer is lost permanently, never re-detectable by the next `drive`. **Fix:** Decide explicitly whether the detect-and-file step also runs on the `err != nil && !errors.Is(err, ErrShedBusy)` path, and say so in the ordering the plan must implement.

### [BLOCKING:design] Title-keyed dedupe suppresses later distinct anomalies
**Section:** `### one-issue-per-anomaly` + `### dedupe-marker` **Issue:** The title is `loom anomaly: <kind> — <slug>` with no producer, round, or timestamp except for the recurring-finding case, and the marker holds titles — so a second, genuinely separate crash-resume, or an escalation at a different producer later in the same task, is permanently suppressed as a duplicate; the discussion only reasons about the resume-of-one-halt case. **Fix:** State the intended dedupe granularity (once-per-task-per-kind vs. per-producer/per-occurrence) and put the discriminator into the title shape if the latter.

### [NIT:consistency] "loomcli already imports shedadapters" is test-only
**Section:** `### ledger-access` / Q&A #3 **Issue:** Only `internal/loomcli/smoke_attachprobe_test.go:42` imports `shedadapters`; production `loomcli` does not, so this is a new production edge, not an existing one (the no-cycle claim does hold — `shedadapters` imports no `loom*` package). **Fix:** Restate the rationale as "new edge, no cycle" rather than "already imported".

### [NIT:consistency] Ledger filename misstated
**Section:** `## Problem` **Issue:** The file is `round-%d-bouncer-ledger.md` (`internal/shedadapters/round.go:55`), not `round-<N>-ledger.md`; this matters if filename shape becomes the discovery discriminator. **Fix:** Correct the name.

### [NIT:decision] `bug` label default owner not stated
**Section:** `### one-issue-per-anomaly` **Issue:** "the module's existing `bug` default" lives in `internal/selfreportcli/cli.go:111`, not in `selfreportengine`, so a `loomcli` caller of `CreateIssue` re-declares the literal. **Fix:** Say which package owns the label list for the automatic path.

### [NIT:decision] CLI/Cobra deviation question is answerable now
**Section:** Q&A final entry **Issue:** Handed to the plan as the one open item, but production `loomcli` already imports `burlerengine`, `websterengine`, `reedengine`, `fabricengine`, `landingshed`, `batcher` (`internal/loomcli/wiring.go`), so the invariant's deviation list plainly covers `<module>cli`↔`<module>engine` naming, not an edge ledger. **Fix:** Settle it in the discussion — no CONSTRAINTS.md entry needed — instead of deferring.

## Verdict

REQUEST_CHANGES
Ledger discovery, error-path reachability, and dedupe granularity each need pinning before planning.
MILL_REVIEW_END
