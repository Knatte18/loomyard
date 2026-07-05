I've verified the discussion's key source claims: `Config.Claude` (config.go:24), the config-test assertion (config_test.go:53-54), `AddSpec` lacking `SessionID`, `Strand.SessionID` present-but-unwritten (state.go:32), `sendKeysLiteralArg`/send-keys mechanics (spawn.go:75-146), `Status` read-only op (lifecycle.go:789), and `AddStrand` (strand.go:305). All accurate. No `SendKeys`/`CapturePane` exported ops exist yet, matching the discussion's "additions" framing.

MILL_REVIEW_BEGIN
# Review: Build internal/shuttle: one LLM agent via a swappable engine

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-05
```

## Findings

### [NOTE] OutputFiles path base unspecified
**Section:** Outcome classification / Spec surface
**Issue:** `done` hinges on "all expected output files exist," but the discussion never states whether `spec.OutputFiles` entries are absolute or relative, and if relative, relative to what (Go process cwd vs the pane/worktree cwd) — the caller's prompt and shuttle's `Stat` must agree on the base.
**Fix:** Pin the OutputFiles path contract (recommend absolute, or explicitly relative-to-worktree-root) in the classification decision so the plan encodes one convention.

## Verdict

APPROVE
Thorough, source-grounded, r1/r2 gaps resolved; only a minor path-contract clarification remains.
MILL_REVIEW_END
