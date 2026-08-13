MILL_REVIEW_BEGIN
# Review: gitexec: add the checked entry point and migrate the call sites

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-13
```

## Findings

### [BLOCKING:consistency] gitexecTotal == 2 is false after migration
**Section:** `client-boundary-guard-keys-on-both-helpers`
**Issue:** The guard counts raw occurrences of the substring `"gitexec."` in the whole rendered file (`gitrepoboundary_test.go:159`), and the migration adds `var gitErr *gitexec.GitError` to at least six `gitrepo` methods (`IsAncestor`, `StageAndCommit`, `CommitEmpty`, `StageAllAndCommit`, `pushWithRebaseRetry`, `PushRebaseFree`), so the count becomes ~8, not 2, and "one inside `run`, one inside `runChecked`" cannot hold.
**Fix:** Decide the replacement assertion explicitly — count only `gitexec.Run`/`gitexec.RunGit` *call expressions* via the existing AST walk, or declare a package-local alias for the error type — rather than restating the occurrence count.

### [BLOCKING:decision] roadmap entry itself has no stated disposition
**Section:** Scope / `package-header-carries-the-durable-rationale`
**Issue:** Scope says only "remove `manifest/roadmap.md`'s link to it", but `roadmap.md:47-51` is a **Planned** entry for this very task, and the file has a `## Done` section; leaving a shipped item under Planned is exactly the move CLAUDE.md says the roadmap does make on completing a planned item.
**Fix:** State whether the entry moves to `## Done` (with what wording) or is deleted, and that this lands in the same commit as the design-doc deletion.

### [BLOCKING:design] `run`'s own surviving RunGit call is unclassified
**Section:** `the-checked-call-invariant` / gitrepo disposition table
**Issue:** The invariant reads "every remaining raw `gitexec.RunGit` / `r.run` call site in non-test source carries a marker", which literally covers the `gitexec.RunGit` call inside `gitrepo.run`'s body — but the table's "3 raw" counts only `Pull`/`Fetch`/`HasUnpushed`, leaving both the marker requirement and `gitrepo`'s per-package pinned count ambiguous.
**Fix:** State explicitly whether the `run` helper's own body is a marked raw site and what number the per-package map pins for `internal/gitrepo`.

### [NIT:scope] new guard file will trip the guards it teaches
**Section:** Scope / `token-guards-key-on-the-shorter-prefix`
**Issue:** `cmd/lyx/checkedcall_test.go` is untagged and will carry `gitexec.RunGit` and `exec.Command` (`go env GOMOD`) as its own scan data, which `tierpurity_test.go`'s `bannedTokens` flags unless added to `allowedSpawners` — every sibling guard already has such an entry.
**Fix:** Add the `allowedSpawners` entry (with reason) to the scope list beside the three token-set edits.

### [NIT:design] guard scans `internal/` only, `cmd/` unscanned
**Section:** `the-checked-call-invariant`
**Issue:** Zero production sites live under `cmd/` today, so the restriction is harmless now, but a future raw call in `cmd/` escapes the marker requirement and it is not among the honestly-named blind spots.
**Fix:** Either widen the walk to `internal/` + `cmd/` or name this as a third blind spot in the test header and the CONSTRAINTS entry.

### [NIT:design] Run-vs-RunGit implementation relationship unstated
**Section:** Constraints "Discovered during discussion"
**Issue:** "On an exec-level failure … stdout is whatever was captured" is unachievable if `Run` delegates to `RunGit`, whose exec path returns `("", "", -1, err)` (`gitexec.go:32`); the discussion never says whether `Run` wraps `RunGit` or duplicates the exec body.
**Fix:** State the intended structure, or reword the contract to "stdout is empty when git never ran".

## Verdict

REQUEST_CHANGES
Guard-count claim is factually wrong; roadmap entry and `run`'s own site undecided.
MILL_REVIEW_END
