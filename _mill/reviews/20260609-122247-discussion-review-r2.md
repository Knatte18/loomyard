The bench uses only `_mhgo/board.yaml` and the CLI path, confirming "no changes needed." Now I have enough to evaluate. Let me assess the key analytical concerns:

1. **Optional regex fallback whitespace**: `envOptRe = \$env:([A-Za-z_][A-Za-z0-9_]*)\s*\?\s*(.*)$` — the `\s*` after `?` means a fallback of `Home.md` works, but the `(.*)$` captures everything to end including any trailing whitespace already trimmed by `\s*`. But what about leading whitespace in the fallback being stripped? The discussion says "fallback (literal, runs to end of value)" but `\s*\?\s*` strips whitespace around `?`. This is a minor ambiguity.

2. **Template env-var names** are net-new and underived.

3. **`envOptRe` with multiple `$env:` tokens**: The regex `\$env:NAME\s*\?\s*(.*)$` is greedy. With value `$env:A ? $env:B ? c`, the `(.*)$ ` captures `$env:B ? c` as literal fallback — but the decision says "only one ?-form token per value, and it must be last" and required tokens fall through. There's an interaction: if optional matches, do required tokens inside the fallback get expanded? The discussion says expansion order step 2 expands "remaining required tokens" — ambiguous whether tokens inside a captured fallback are expanded.

Let me finalize my review.

MILL_REVIEW_BEGIN
# Review: extract-shared-infra

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: C:\Code\mhgo\wts\extract-shared-infra\_mill\discussion.md
date: 2026-06-09
```

## Findings

### [GAP] Template introduces undefined env var names
**Section:** Technical context → init.go template comment update
**Issue:** The new template hardcodes `MHGO_BOARD_PATH`, `MHGO_HOME`, `MHGO_SIDEBAR`, `MHGO_PROPOSAL_PREFIX` (lines 254-257), but these names exist nowhere in the codebase and no decision defines them; current `generateCommentedBoardYAML` derives lines from `defaults Config` fields (init.go:111-114) rather than static strings.
**Fix:** State the canonical env var names as a decision and clarify whether the template becomes static literals or stays derived from `DefaultConfig()`.

### [GAP] Fallback whitespace handling is ambiguous
**Section:** Decision: `$env:NAME ? fallback` optional syntax
**Issue:** `envOptRe` uses `\s*\?\s*(.*)$`, so whitespace around `?` is consumed, but the prose says the fallback "runs to end of value" — unclear whether leading/trailing fallback whitespace (e.g. `$env:N ?  ../_board `) is preserved or trimmed, affecting path values.
**Fix:** Specify exact fallback trimming semantics (trim both sides / preserve as-is).

### [GAP] Required-token expansion inside captured fallback undefined
**Section:** Env expansion regex / Decision: optional syntax
**Issue:** `envOptRe` greedily captures `(.*)$` as the fallback; for a value like `$env:A ? $env:B`, step-2 required expansion ("expand remaining required tokens") could re-expand `$env:B` inside the fallback, but behaviour is unstated. "Only one ?-form token, must be last" does not forbid a required token in the fallback text.
**Fix:** State whether `$env:` tokens inside a fallback are expanded or treated literally.

### [NOTE] Prefix-text-only path resolution interaction unverified
**Section:** Decision: board.LoadConfig stays exported
**Issue:** Board resolves relative `Path` against baseDir after expansion; a fallback like `? ../_board` yields a relative path that must still be joined — fine, but no test in the plan asserts the fallback-then-path-resolution combination.
**Fix:** Add a board-level test for `path: $env:X ? ../_board` resolving against baseDir.

### [NOTE] spawn_windows.go lacks explicit build tag
**Section:** Technical context → Package layout (spawn files)
**Issue:** `spawn_windows.go` has no `//go:build windows` line; it relies solely on the `_windows.go` filename suffix (valid Go), while `spawn_other.go` uses an explicit `!windows` tag — the discussion does not note this asymmetry but it does not affect correctness.
**Fix:** None required; record that the filename suffix is the intended constraint mechanism.

## Verdict

GAPS_FOUND
Env-fallback semantics and undefined template env var names need clarification before plan writing.
MILL_REVIEW_END
