I have all the evidence. Let me compile the findings. The key issues:

1. **`RunGit` signature is wrong** in the discussion — it claims `RunGit(dir string, args ...string) (string, error)` but actual is `RunGit(args []string, cwd string) (stdout, stderr string, exitCode int, err error)`. This is load-bearing for dirty-check, push, existence-check, and remove logic — all of which the discussion describes in terms of the wrong signature (e.g., "exits 0 if branch exists" relies on inspecting exitCode, which the discussion's signature hides).

2. **`LoadConfig` signature mismatch** — the discussion's "Board module as structural template" prescribes `LoadConfig(baseDir string) (Config, error)`, but board's actual signature is `LoadConfig(baseDir, module string) (Config, error)`.

3. **Error message string mismatch** — config_test scenario expects error containing `run "mhgo init"`, but board wraps it as `not initialized here; run "mhgo init"` while `internal/config` emits `not initialized: _mhgo/ directory not found`. The worktree package must replicate board's wrapping; the discussion doesn't note this.

4. **`git -C` dirty-check** — discussion's "Dirty-check implementation" uses `git -C <path> status`, but RunGit already takes a cwd; mixing `-C` with cwd is redundant/inconsistent with the codebase convention.

5. **main.go module doc** — the doc comment lists `muxpoc` not `worktree`; minor.

MILL_REVIEW_BEGIN
# Review: mhgo-worktree-module

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-06-13
```

## Findings

### [GAP] RunGit signature in Technical context is wrong
**Section:** Technical context → Existing shared infrastructure; Dirty-check; existence checks
**Issue:** Discussion states `internal/git.RunGit(dir string, args ...string) (string, error)`, but the actual signature is `RunGit(args []string, cwd string) (stdout, stderr string, exitCode int, err error)` — args first, plus stderr and exitCode returns.
**Fix:** Correct the signature, and rewrite the existence-check ("`rev-parse --verify` exits 0") and push/error-handling descriptions in terms of the returned `exitCode`/`stderr`, since `err` is nil for non-zero git exits.

### [GAP] LoadConfig template signature mismatch
**Section:** Technical context → Board module as structural template
**Issue:** Prescribed `LoadConfig(baseDir string) (Config, error)` does not match board's actual `LoadConfig(baseDir, module string) (Config, error)` (the module name `"worktree"` must be passed).
**Fix:** State the signature as `LoadConfig(baseDir, module string)` (or hardcode `"worktree"` internally) to match the established pattern.

### [GAP] "run mhgo init" error wrapping not specified
**Section:** Testing → config_test.go ("error containing run \"mhgo init\"")
**Issue:** `internal/config.Load` emits `not initialized: _mhgo/ directory not found`; the `run "mhgo init"` phrasing only exists because board's `LoadConfig` re-wraps it. The discussion does not state worktree must replicate that wrapping, so the test as written would fail against the raw config error.
**Fix:** Specify that worktree's `LoadConfig` must detect the "not initialized" error and re-wrap with the `run "mhgo init"` message, mirroring `internal/board/config.go`.

### [NOTE] Redundant `git -C` in dirty-check
**Section:** Technical context → Dirty-check implementation
**Issue:** Shows `git -C <worktree_path> status --porcelain`, but `RunGit` already sets `cmd.Dir = cwd`; passing both `-C` and a cwd is redundant and diverges from board's convention of relying on the cwd argument.
**Fix:** Pass the path via RunGit's `cwd` argument and drop `-C` (or note explicitly why `-C` is preferred).

### [NOTE] main.go doc comment lists muxpoc, not worktree
**Section:** Technical context → cmd/mhgo/main.go dispatch
**Issue:** Current `main.go` module list documents `init`, `board`, `muxpoc`; the discussion's "Modules" intent should add `worktree` to both the switch and the doc block, which it does not call out.
**Fix:** Note that the package doc-comment Modules list must also gain a `worktree` line alongside the new `case`.

## Verdict

GAPS_FOUND
Reused-API signatures (RunGit, LoadConfig) and the init-error wrapping are misstated; correct before planning.
MILL_REVIEW_END
