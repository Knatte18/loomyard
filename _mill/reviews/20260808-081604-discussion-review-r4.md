MILL_REVIEW_BEGIN
# Review: Scout owns its own lyxcwd-based geometry accessors (drop Options.AnchorRoot threading)

```yaml
verdict: GAPS_FOUND
reviewer_model: opus
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-08
```

## Findings

### [GAP] Test-file comments naming WorktreeRoot are unassigned
**Section:** Technical context § "Affected test files (nine, not five)" + `docs-in-same-commit`
**Issue:** The enumeration claims each entry lists "every site that must change", but lists only compile-breaking code sites; `refs_integration_test.go:77,178,193`, `ensureserver_integration_test.go:127,136,174` and `supervised_integration_test.go:51,89` carry prose comments naming `WorktreeRoot`/`worktreeRoot` that become false after the rename, and `docs-in-same-commit` is explicitly scoped to `scoutengine` prose docs plus mandated new code doc comments — so no decision covers them, and no gate compiles those four files.
**Fix:** State whether the nine files' comment mentions of `WorktreeRoot`/`worktreeRoot` are reworded in this commit or explicitly left alone, and if reworded, add them to the per-file site lists.

### [NOTE] "Registry is the built-in one" assertion has no stated mechanism
**Section:** Testing § TDD candidate 2
**Issue:** `scoutengine.Registry` is `map[string]Entry` (`registry.go:46`), so it is not `==`-comparable against `scoutengine.BuiltinRegistry()`; the pin test needs `reflect.DeepEqual` or a keyed spot-check, which the discussion does not name.
**Fix:** Name the comparison shape for the registry half of the `lookupContext` assertion.

### [NOTE] Untagged scoutcli test does spawn git, indirectly
**Section:** Constraints § Test Tier Purity / Hermetic Git Test Environment
**Issue:** "This spawns no git" is true of the scoutengine fixture shape but not of file 4 (`internal/scoutcli/cli_test.go`): both the existing out-of-hub test and the new `lookupContext` test reach `lyxcwd.Resolve`, which calls `gitexec.RunGit([]string{"rev-parse","--show-toplevel"}, cwd)` (`lyxcwd.go:143`); `internal/scoutcli` has no `TestMain`/`HermeticGitEnv`. Both guards still pass (direct-token / lyxtest-helper based) and this is pre-existing, but the out-of-hub premise depends on `t.TempDir()` not sitting inside a git repo.
**Fix:** Scope the no-git claim to files 1-3 and 5, and note the scoutcli tests' inherited dependence on an ambient non-repo TMPDIR.

## Verdict

GAPS_FOUND
One unassigned doc-hygiene site set; the rest of the design verifies clean against source.
MILL_REVIEW_END
