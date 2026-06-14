MILL_REVIEW_BEGIN
# Review: Extend worktree module: portals and launchers — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-06-14
```

## Findings

### [BLOCKING] remove_test.go missing teardown-before-exists-check test case

**Location:** `internal/worktree/remove_test.go`

**Issue:** Card 11 explicitly requires a test case where the worktree dir is deleted before `Remove` is called, asserting portal/launcher cleanup still runs before the "not found" return. The `NonexistentSlug` case with slug "ghost" only asserts `wantErr: true` — it does not create a portal/launcher before the call and does not assert they were cleaned up. The sequence the plan is guarding against (directory gone but portal still dangling) is untested.

**Fix:** Add a test case that pre-creates a portal (or launcher on Windows) for a slug whose worktree directory does not exist, calls `Remove`, and asserts the portal/launcher is gone — regardless of whether an error is returned for the missing directory.

### [BLOCKING] gitignore.go splits on "\n" but does not strip "\r" — CRLF corruption risk

**Location:** `internal/gitignore/gitignore.go:56`

**Issue:** `strings.Split(existingContent, "\n")` on a CRLF file leaves trailing `\r` on each captured line. This causes the old-entry set (built from trimmed lines) to mismatch the sorted new entries (which have no `\r`), so `entriesEqual` always returns false — the file is rewritten on every call and `changed=true` is returned spuriously even for idempotent adds. On Windows, git-checked-out `.gitignore` files commonly have CRLF.

**Fix:** After reading the file, normalize line endings before splitting: `existingContent = strings.ReplaceAll(existingContent, "\r\n", "\n")`.

### [NIT] add.go line 119: exec error discarded on push failure, replaced with generic message

**Location:** `internal/worktree/add.go:119`

**Issue:** When `git.RunGit` returns a non-`ExitError` failure on the push step (e.g., psmux binary not found, permission denied), the actual error `err` is discarded and the caller receives the generic string `"cwd is not a valid git worktree"` instead of the push-specific cause.

**Fix:** Return `fmt.Errorf("push: %w", err)` instead of the generic message, consistent with the non-zero-exit case below it that correctly reports `"push failed: %s"`.

### [NIT] worktree/portals_test.go: os.Readlink is unreliable for NTFS junctions

**Location:** `internal/worktree/portals_test.go:51`

**Issue:** `os.Readlink` on a Windows NTFS junction does not return the target the way it does for POSIX symlinks; the resolved path may include a `\??\` device prefix, causing the path comparison to fail even when the junction is correctly created.

**Fix:** Replace the `os.Readlink` comparison with an existence check on `filepath.Join(portalLink, "some-file-known-to-be-in-target")`, or use `os.Stat` through the junction to verify it resolves correctly, which is platform-agnostic.

## Verdict

REQUEST_CHANGES
Two blocking issues: missing mandatory remove test case and CRLF-unsafe gitignore parser.
MILL_REVIEW_END
