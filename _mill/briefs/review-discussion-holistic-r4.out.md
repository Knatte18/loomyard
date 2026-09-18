MILL_REVIEW_BEGIN
# Review: Launch ly-supervise and orchestrator via lyx reed add

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: claude-opus-4-1-20250805 (best-effort; presented to me as Opus 5)
reviewed_file: _mill/discussion.md
date: 2026-09-18
```

## Findings

### [BLOCKING:design] Templated name can never match under default template
**Section:** `### add-if-absent-flag` (name resolution) + `## Testing` / `internal/reedengine`
**Issue:** `resolveStrandName` (`internal/reedengine/strand.go:125-139`) falls back to `guid[:8]`, and the shipped default template is `strand_name: '<ROLE>:<ROUND>:<SHORT_GUID>'` (`internal/reedengine/template_posix.yaml:7`, `template_windows.yaml:7`) — so any `--if-absent` invocation without `--name` resolves to a fresh, never-matching name, making the stated test obligation "an `--if-absent` add driven by `--role`/`--round` must match a strand of the same templated name" unsatisfiable and silently re-creating duplicates, the exact failure the flag exists to prevent.
**Fix:** State the disposition explicitly — either `--if-absent` requires a guid-free resolved name (reject/error when `--name` is absent or the template contains `<SHORT_GUID>`), or the guid-derived case is documented as always-adds and the contradictory test case is dropped.

### [BLOCKING:consistency] Hidden branch order contradicts the selection rule
**Section:** `### add-if-absent-flag`, branch "name present and hidden" vs "Selection rule when the resolved name is ambiguous"
**Issue:** The branch text says hidden is "checked *before* the alive/not-alive split", while the selection rule ranks candidates "first alive non-hidden, else first non-hidden, else hidden no-op"; for a table holding one hidden and one not-alive non-hidden strand of the same name, the first reading no-ops and the second relaunches — two different behaviours from one decision.
**Fix:** Pick one ordering and restate both passages in those terms (hidden strands excluded from candidacy, hidden no-op only when no non-hidden candidate exists, or the reverse).

### [NIT:consistency] Chain passes no `--anchor`, but the decision cites it
**Section:** `### add-if-absent-flag`, alive no-op branch
**Issue:** The no-op rationale says the no-op protects "the `--focus` and `--anchor` the launch chain passes", but the chain in `### vscode-task-is-the-launch-surface` is `--if-absent --cmd claude --name claude --focus` with no `--anchor` (`reedcli` defaults it to `below-parent`).
**Fix:** Drop `--anchor` from that sentence or add it to the literal chain.

### [NIT:decision] `--cmd` required-ness under `--if-absent` unstated
**Section:** `### add-if-absent-flag` / `## Testing` (`internal/reedcli`)
**Issue:** `--cmd` is `MarkFlagRequired` (`internal/reedcli/add.go:100`), yet both matched branches ignore the supplied value; the discussion never says whether it stays required.
**Fix:** One sentence stating `--cmd` stays required under `--if-absent` (bare `add` unchanged) so the plan writer does not relax the requirement.

## Verdict

REQUEST_CHANGES
Name-resolution matching is unreachable without `--name`, and the hidden-branch ordering self-contradicts.
MILL_REVIEW_END
