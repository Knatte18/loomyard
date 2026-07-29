MILL_REVIEW_BEGIN
# Review: prowler: installable Claude Code plugin (Go), hosted in LoomYard

```yaml
verdict: GAPS_FOUND
reviewer_model: opus
reviewer_self_id: Claude Opus 4.8 (claude-opus-4-8)
reviewed_file: _mill/discussion.md
date: 2026-07-29
```

## Findings

### [GAP] Guard enumeration omits ghguard (a third module-root walker)
**Section:** Decisions › separate-nested-go-module; Constraints
**Issue:** The decision names exactly "two disk-walking grep guards" (tierpurity, hermeticenv), but `cmd/lyx/ghguard_test.go` (GitHub Auth Invariant) also `filepath.WalkDir`s the whole module root, skipping only `tierPuritySkipDirs`, and scans every **non-test** `.go` — i.e. it reads prowler's **production** source, a scan surface the discussion asserts is out of reach; the GitHub Auth Invariant is not addressed in the Constraints section at all.
**Fix:** Add the GitHub Auth Invariant to the per-invariant Constraints list, noting ghguard walks into `plugins/prowler/1.0.0/` production `.go` and that prowler stays green by never containing `LookPath("gh")` or a same-line `exec.Command(..., "gh")`; correct "two disk-walking guards" to three (boardguard/gitrepoboundary/pathresolve stay single-dir and do not reach prowler).

## Verdict

GAPS_FOUND
Guard enumeration is incomplete: ghguard also scans prowler's production source; GitHub Auth Invariant unaddressed.
MILL_REVIEW_END
