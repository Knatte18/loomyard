MILL_REVIEW_BEGIN
# Review: Adopt quarry's glyph alphabet as the plan alphabet

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-09-06
```

## Findings

### [BLOCKING:design] No disposition for a quarry call that errors
**Section:** `resolve-status-policy` / `blocking-policy` / `delta-call-site`
**Issue:** The policy covers the four per-target resolve *statuses* only, but `quarry.Open(root)` and `(*Repo).Resolve([]string) ([]ResolveResult, error)` (verified `quarry/repo.go:29,87`) each return an error, as does `DeltaGit`; the discussion never says whether a non-nil error at `Plan-Revalidate`, `begin-batch` or `record-batch` blocks the gate, degrades to format-only validation, or is informational.
**Fix:** State the infrastructure-error disposition per boundary (block vs. degrade vs. informational), distinct from the per-target status policy.

### [BLOCKING:consistency] Parity pair's shared function left unnamed
**Section:** `drift-boundaries` / Testing ("Gate parity and CLI") / Technical context
**Issue:** `package-ownership` puts the resolve pass in `planglyph`, so the `Plan-Revalidate` ↔ `validate-plan --require-approved` pair can no longer call `planparser.Validate`; the discussion only says "that pair's function gains the batched `Resolve`" and never names the new shared function, while CONSTRAINTS.md's Gate Self-Check Parity bullet spells `planparser.Validate` for that pair — a fourth CONSTRAINTS edit the "**three** separate edits, not one" enumeration excludes.
**Fix:** Name the single function both `loomshed.NewPlanValidate`'s row and the `--require-approved` verb call after the change, and add the parity bullet to the CONSTRAINTS edit list.

### [NIT:consistency] CLI/Cobra module-count line not in the edit list
**Section:** Technical context (CONSTRAINTS edits) / `cli-verb-surface`
**Issue:** CONSTRAINTS.md's CLI/Cobra Invariant reads "eleven of twelve also carry `RunCLIIn`"; `cmd/lyx/main.go` registers twelve subtrees today, so adding `quarrycli` makes the count stale, but the enumerated edits name only the `stencilcli` deviation line.
**Fix:** Add the count line to the CLI/Cobra edit named for the same commit.

## Verdict

REQUEST_CHANGES
Two decisions missing: quarry-error disposition and the parity pair's shared function.
MILL_REVIEW_END
