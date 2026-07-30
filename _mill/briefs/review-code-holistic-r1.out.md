MILL_REVIEW_BEGIN
# Review: fabric: warp-side commit lock + push coalescing — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-07-30
```

## Findings

### [BLOCKING] CoalescePushBothAt reads warp HEAD via cwd when warpPath=="", a real production path
**Location:** `internal/fabricengine/coalesce.go:116-124` (the `step` closure calling `headOrEmpty(warpPath)` unconditionally); `internal/fabriccli/spawn.go:14-16` (`spawnPush` calls `fabricengine.SpawnDetachedPush("", weftPath)`).
**Issue:** `lyx fabric sync`'s detached child is spawned with `--weft-path` only (`warpPath == ""`); this is a genuine, documented-as-supported production case ("warpPath may still be empty when weftPath is present, which pushes only the weft side"), not merely the batch's noted latent `weftPath==""` edge. Inside `CoalescePushBothAt`'s step, `headOrEmpty(warpPath)` is called unconditionally even when `warpPath == ""`, which calls `gitrepo.New("").CurrentSHA()`. `gitrepo`'s go-git open uses `PlainOpenWithOptions(r.path, &git.PlainOpenOptions{EnableDotGitCommonDir: true})` with `DetectDotGit` left false (no upward search), and `filepath.Abs("")` resolves to the process's cwd — the *inherited* cwd of the detached child (`exec.Command` sets no `cmd.Dir`), i.e. whatever directory the operator ran `lyx fabric sync` from. Whenever that cwd is not exactly the warp worktree root (a nested hub at `RelPath != "."`, a scenario this same file's own `crossModuleMachineLocalExcludes` comment says is supported — "multiple hubs at different RelPath depths share one weft checkout"), `CurrentSHA` fails with a plain wrapped error (not `gitrepo.ErrNoCommits`), which `headOrEmpty` propagates, aborting `CoalescePush`'s loop with an error. Because this runs inside a detached, un-waited, stdio-nil child (`SpawnDetachedPush`), the failure is completely silent — the weft side's genuinely-unpushed commit is never pushed and no operator or log line ever reports it. Even when cwd happens to coincide with the warp root, the "success" is an accidental coincidence, not the documented contract ("its empty-string HEAD is trivially stable" — false; a real git open against cwd occurs). No test in `coalesce_test.go`/`coalesce_integration_test.go` exercises `CoalescePushBothAt` with an empty `warpPath` at all — both integration tests always supply a real `warpPath`.
**Fix:** In `headOrEmpty` (or its call sites in the `step` closure), short-circuit to `("", nil)` when `path == ""` before ever calling `gitrepo.New(path).CurrentSHA()`, so an empty path is a true no-op read matching the doc comment's stated contract; add an integration test exercising `CoalescePushBothAt("", weftFixturePath, ...)` from a cwd that is not the (nonexistent) warp root, asserting the weft side still advances and no error is returned.

## Verdict

REQUEST_CHANGES
Silent weft-push failure when warpPath is empty (the real `lyx fabric sync` async path) and cwd isn't the warp root.
MILL_REVIEW_END
