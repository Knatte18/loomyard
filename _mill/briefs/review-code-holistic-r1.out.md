MILL_REVIEW_BEGIN
# Review: native clients: migrate gitrepo to go-git + selfreport gh-CLI to go-github — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-07-28
```

## Findings

### [BLOCKING] CONSTRAINTS.md's gitrepo Client Boundary Invariant omits hasUnpushed
**Location:** `CONSTRAINTS.md:160`
**Issue:** Card 21's reversal criterion fired — `internal/gitrepo/push.go`'s `hasUnpushed` is CLI-bound (`r.run("rev-list", ...)`, confirmed) and is correctly in `gitrepoboundary_test.go`'s pinned `r.run`-bound set — but the invariant's exhaustive CLI-bound list ("used for `StageAndCommit`, ..., `SnapshotSHA`'s fetch") never names `hasUnpushed`. Card 21 explicitly required it to "join the CLI-bound set the CONSTRAINTS.md entry names," and the entry's own text says "widening the CLI-bound set without editing this list is itself a violation."
**Fix:** Add `hasUnpushed` to the named CLI-bound set in the invariant's Statement bullet.

### [BLOCKING] gitrepo/doc.go misclassifies hasUnpushed as go-git-backed
**Location:** `internal/gitrepo/doc.go:16-23`
**Issue:** The opening "two-backend boundary" section lists `hasUnpushed` in the go-git read surface ("resolves state entirely through go-git's own object and ref access ... bypassing run and gitexec.RunGit completely") and omits it from the CLI-bound list a few lines later. Both are false: `push.go`'s own godoc documents the card-21 measured reversal back to the CLI (`r.run("rev-list", "--count", "@{u}..HEAD")`).
**Fix:** Move `hasUnpushed` from the go-git list to the CLI-bound list in doc.go's opening paragraph.

### [BLOCKING] roadmap.md's Done entry claims hasUnpushed migrated to go-git
**Location:** `manifest/roadmap.md:65`
**Issue:** The task's Done entry lists `hasUnpushed` among the methods that "migrated to go-git," contradicting the actual reverted-to-CLI implementation in `push.go` and its own measured-numbers godoc (go-git ~9.4x slower on the walking path).
**Fix:** Remove `hasUnpushed` from the migrated list and add a clause noting it was measured and reverted to the CLI per card 21's reversal criterion.

## Verdict

REQUEST_CHANGES
Three doc sites (CONSTRAINTS.md, gitrepo/doc.go, roadmap.md) still describe hasUnpushed as go-git-migrated despite card 21's measured CLI reversal.
MILL_REVIEW_END
