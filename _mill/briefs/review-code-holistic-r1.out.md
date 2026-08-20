MILL_REVIEW_BEGIN
# Review: loom: session bootstrap — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-08-20
```

## Findings

### [BLOCKING:design] Legacy origin-record commit is not retried after a crash between write and commit
**Location:** `internal/loomcli/run.go:76-118` (`resolveParentBranch` gate at `internal/loomcli/seedinput.go:35-54`)
**Issue:** For a legacy pair repaired via `--parent`, step 1 writes `origin.json` to disk only when `resolveParentBranch` reports `write == true`; step 3's `commitPaths` includes `fabricengine.OriginRecordRel()` only when *this invocation's* `writeOrigin` was true. If the process dies/errors between step 1's successful write and step 3's commit (crash, Ctrl-C, disk full, a failing step 2), the next `loom run` re-reads the already-written-but-uncommitted record via `ReadOrigin`, finds it present with a matching value, and `resolveParentBranch` now returns `write == false` — so the origin record is permanently excluded from every future `commitPaths` on that worktree. The record stays an untracked file in the weft forever, and (per this file's own comments) `fabricengine.Clean`/`CheckWorktreeClean` scans the weft including untracked files, so the machine's first Preflight precondition row is permanently blocked with no built-in remedy — `--parent` again is a silent no-op since the value already matches. Contrast with the status file: `commitPaths` always includes `loomengine.LoomStatusRel()` unconditionally every invocation, so that half self-heals; the origin-record half does not, which reads as an asymmetry/oversight rather than an intentional design choice.
**Fix:** Base step 3's origin-record inclusion on whether the record is actually absent from the weft's committed tree (or track "needs-commit" state independent of "needs-write" state), not solely on whether *this* invocation's write occurred — so a crash between steps 1 and 3 self-heals on the next `loom run` exactly as the status file already does.

### [NIT:consistency] Stale file-header comment omits the run launcher
**Location:** `internal/fabricengine/launcher_content.go:1-5`
**Issue:** The header still says the file "builds the byte content and file mode for launcher scripts (ide, fabric-checkout, ide-menu)", but `launcherScript` is now also the builder for the `run<ext>` script (`launchers.go`'s `writeLaunchers`, card 6). The plan's card 6 scoped this file as `Context:` only (not `Edits:`), so the comment was left as-is.
**Fix:** Add "run" to the enumerated script set in this header comment in a follow-up touch of the file.

## Verdict

REQUEST_CHANGES
The origin-record commit path has a real, untested self-healing gap on the legacy `--parent` repair flow; everything else matches the plan precisely.
MILL_REVIEW_END
