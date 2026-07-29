MILL_REVIEW_BEGIN
# Review: Give codeintel a persistent, session-long daemon

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewer_self_id: Claude Opus 4.8 (claude-opus-4-8)
reviewed_file: _mill/discussion.md
date: 2026-07-29
```

## Findings

### [NOTE] --in-file × batch-mode interaction unspecified
**Section:** Decisions / item4-flag-shape
**Issue:** The decision describes `--in-file` only for a single positional ("the positional argument"); nothing prevents `refs --in-file foo.go A B`, and batch behaviour (reject, or resolve each arg in the same file) is undefined — the buildOptions refactor touches exactly these batch closures.
**Fix:** Add one sentence stating whether `--in-file` composes with batch mode (natural reading: each arg resolved via documentSymbol in the same file) or is rejected pre-flight; pin it at plan time.

### [NOTE] Query third-form field unnamed
**Section:** Technical context (refs.go) / item4-documentsymbol-mechanism
**Issue:** The discussion says `Query{Symbol, Pos}` "gains a third form" for `--in-file` but never names the field the engine reads to select the documentSymbol resolve branch in `resolvePosition`.
**Fix:** Note the intended field (e.g. an `InFile`/scoped-name form) so the CLI→engine handoff and the new `resolvePosition` branch are unambiguous.

## Verdict

APPROVE
Scope, decisions, and both r1 GAPs are resolved and source-grounded; two non-blocking NOTEs only.
MILL_REVIEW_END
