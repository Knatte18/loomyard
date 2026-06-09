I have verified all the load-bearing claims. The discussion is thorough and well-grounded. Let me identify the genuine gaps and notes.

Key findings:

1. **init.go signature**: Discussion says `generateCommentedBoardYAML` becomes static literals "not derived from `DefaultConfig()`". Current signature is `generateCommentedBoardYAML(defaults Config)` called with `defaults := DefaultConfig()` at init.go:67-68. The discussion doesn't state whether the parameter is dropped (changing signature + caller) or kept-but-ignored. Minor ambiguity for plan writer.

2. **`.env` baseDir vs cwd**: The Decision says `Load` reads `<baseDir>/.env`. But `LoadConfig` resolves `baseDir = cwd` (cli.go:68-72). For the detached sync child path (cli.go:63-65), `LoadConfig` is bypassed entirely, so `.env` won't be read there — acceptable but unstated.

3. **`envOptRe` regex `(.*)$` and embedded required tokens**: The expansion order step 2 says required tokens "outside the optional match" get expanded. But after step 1 substitutes the optional portion, step 2 runs `envReqRe.ReplaceAllStringFunc` on the *result* — if the fallback literal itself contains a `$env:NAME` substring, it would be re-expanded, contradicting "fallback is literal". The discussion explicitly claims fallbacks are NOT expanded, but the described two-pass algorithm (apply envReqRe to the whole result-of-step-1 string) would expand a `$env:` inside the substituted fallback. This is a real algorithmic gap.

Let me confirm finding 3 is genuine by re-reading the expansion order text — step 2 says "expanding only the `$env:NAME` tokens that remain (i.e. those that were in the original value outside the optional match)". The parenthetical states intent, but `envReqRe.ReplaceAllStringFunc(result)` operates on the full string with no positional filtering. This is a contradiction between stated intent and described mechanism.

MILL_REVIEW_BEGIN
# Review: extract-shared-infra

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: C:\Code\mhgo\wts\extract-shared-infra\_mill\discussion.md
date: 2026-06-09
```

## Findings

### [GAP] Step-2 expansion can re-expand fallback literal
**Section:** Technical context → "Env expansion regex" + Decision "$env:NAME ? fallback"
**Issue:** Step 2 runs `envReqRe.ReplaceAllStringFunc` on the whole step-1 result, but the Decision guarantees `$env:` inside a fallback is NOT expanded; if a fallback literal contains `$env:X` (e.g. `... ? $env:HOME/x`), the described mechanism would still expand it, contradicting the rule.
**Fix:** Specify positional handling (expand only the non-matched prefix span, then concatenate the literal fallback) rather than re-scanning the full substituted string.

### [GAP] init.go template: signature change unspecified
**Section:** Decision "init.go template — static literals" / package-layout init.go row
**Issue:** Current `generateCommentedBoardYAML(defaults Config)` is called with `DefaultConfig()` (init.go:67-68); the discussion says it becomes static literals "not derived from `DefaultConfig()`" but never states whether the `defaults` parameter (and its caller) is dropped or kept-and-ignored.
**Fix:** State the target signature (drop the param and update the init.go:67-68 caller) so the plan writer does not leave a dead argument.

### [NOTE] `.env` only loaded on the LoadConfig path
**Section:** Decision ".env file loading"
**Issue:** `.env` is read in `config.Load`, reached only via `board.LoadConfig`; the detached sync child sets `cfg` directly (cli.go:63-65) bypassing `LoadConfig`, so `.env` never applies there — unstated but likely intended.
**Fix:** Note that `.env` resolution is scoped to the `LoadConfig`/cwd path and intentionally not applied to the `--board-path` child.

### [NOTE] `envOptRe` multiline `$` anchor
**Section:** Technical context → "Env expansion regex"
**Issue:** `(.*)$` without `(?s)` is fine for single-line YAML scalars, but if a value ever contains a newline the `$` could anchor mid-string; values are flat strings today so this is currently safe.
**Fix:** State the assumption that expanded values are single-line, or anchor with `\z`.

## Verdict

GAPS_FOUND
Two gaps: fallback re-expansion mechanism and init.go template signature need clarifying before planning.
MILL_REVIEW_END
