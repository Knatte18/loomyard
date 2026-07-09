MILL_REVIEW_BEGIN
# Review: Facilitate Linux support (Win11-side prep)

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-09
```

## Findings

### [NOTE] posix `$(cat)` vs `Get-Content -Raw` newline semantics
**Section:** Decisions › shell-abstraction
**Issue:** The pinned posix form `"$(cat <promptPath>)"` claims to reproduce `Get-Content -Raw` semantics, but it only addresses word-splitting; command substitution strips *all* trailing newlines whereas `Get-Content -Raw` preserves the file bytes exactly — a subtle content divergence.
**Fix:** Note in the plan that trailing-newline stripping is a benign difference for prompt text (or handle it), so the "reproduces" claim isn't silently overstated.

### [NOTE] shell seam boundary vs. flags in the posix example
**Section:** Decisions › shell-abstraction
**Issue:** The posix-form example folds `--session-id`/`--settings` into what `internal/shell` builds, while provider-seam-compliance says those flags are Claude *content* that stays in `claudeengine` — the illustrative line conflicts with the stated seam.
**Fix:** Clarify that `internal/shell` returns only the `bin + prompt-read` mechanics (quoting/chaining/file-read idiom) and `claudeengine` appends all `--` flags, so the plan doesn't leak Claude flags into the invariant-bound leaf.

## Verdict

APPROVE
Scope, decisions, constraints, and testing are complete and source-accurate; only two non-blocking clarifications remain.
MILL_REVIEW_END
