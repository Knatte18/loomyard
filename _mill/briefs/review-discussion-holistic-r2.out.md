MILL_REVIEW_BEGIN
# Review: Scout owns its own lyxcwd-based geometry accessors (drop Options.AnchorRoot threading)

```yaml
verdict: GAPS_FOUND
reviewer_model: opusmedium
reviewer_self_id: Claude (Opus-class, Anthropic)
reviewed_file: _mill/discussion.md
date: 2026-08-08
```

## Findings

### [GAP] assert-no-callers pin test is tautological
**Section:** Testing §2 / `fix-assert-no-callers-literal`
**Issue:** `baseOptions(registry, targetDir, layout, lang, timeout)` takes `layout` as a parameter and `buildOptions` merely wraps it, so asserting `baseOptions(...).Layout.WorktreePath() == buildOptions(...).Layout.WorktreePath()` for the same inputs is true by construction; the actual defect lives in the `RunE` closure (`cli.go:576-584`, `worktreeRoot` left empty when `lyxcwd.Resolve` fails), which the test never exercises.
**Fix:** Name a seam or test shape that actually observes the command's own resolution (e.g. a command-level run asserting the out-of-hub daemon path, or a helper that resolves *and* builds), or state explicitly that the pin is compile-time only.

### [GAP] "exactly six sites and nothing beyond them" contradicts three other mandates
**Section:** `docs-in-same-commit` vs `options-carries-layout` / `no-nil-layout-check` / `out-of-hub-synthesized-location` / Technical context
**Issue:** All six enumerated sites are in `scoutengine`, yet other decisions require: a doc on `Options.Layout` saying it is required (`refs.go:45-53` has no per-field docs today), `resolveLocation`'s new doc comment carrying the fiction limit, and rewording the three `resolveWorktreeRoot` reference blocks at `cli.go:147-150`, `295-296`, `416-417` — none on the list the decision closes with "and nothing beyond them".
**Fix:** Extend the enumeration to the `scoutcli` sites plus `refs.go`'s `Options`/`Layout` field doc, or reword the closure to bind only `scoutengine` comment sites.

### [GAP] Affected-test enumeration lists accessor calls only, not the threaded call sites
**Section:** Technical context → "Affected test files (nine, not five)"
**Issue:** Beyond the cited `DaemonStateFile`/`DaemonLock` lines, the same files pass `worktreeRoot` into `ensureServer`/`ensureSupervised` and must change too: `ensureserver_test.go:380`, `supervised_test.go:96,153,318`, `supervised_scout_test.go:41,91`, `supervised_integration_test.go:60,91,114`, `ensureserver_integration_test.go:145,176` — an under-count precisely in the four `//go:build scout` files the discussion calls the largest correctness risk.
**Fix:** Complete the per-file site list to include every `ensureServer`/`ensureSupervised` call, not just accessor calls.

### [NOTE] `worktreeRoot` doubles as `targetDir` in three untagged fixtures
**Section:** Technical context → "Test fixture shape"
**Issue:** `supervised_test.go:96,153,318` and `supervised_scout_test.go:41,91` call `ensureSupervised(…, lang, worktreeRoot, worktreeRoot, …)`, so the fixture must keep the plain directory string alongside the new `l`, not replace it.
**Fix:** State that the fixture yields both `dir` (string, for `targetDir`) and `l` (`*lyxcwd.Location`).

### [NOTE] Line numbers drift in the affected-file list
**Section:** Technical context → item 2
**Issue:** `supervised_test.go`'s `worktreeRoot := t.TempDir()` fixtures are at 63, 127, 205 (65-66/129-130/207-208 are the accessor calls), and a fourth usage sits at 318 under the third fixture.
**Fix:** Correct the cited line ranges or cite symbols instead of lines.

## Verdict

GAPS_FOUND
Pin test proves nothing, doc-site list self-contradicts, and scout-tagged call sites are under-enumerated.
MILL_REVIEW_END
