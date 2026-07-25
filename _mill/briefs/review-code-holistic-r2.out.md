MILL_REVIEW_BEGIN
# Review: board: use gitrepo as its git operator — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-07-25
```

## Findings

No findings. Verified end-to-end against both batch files, the overview's Shared
Decisions, and CONSTRAINTS.md:

- `gitrepo.StageAllAndCommit` (`internal/gitrepo/gitrepo.go:176-216`) matches Card 1's
  step-by-step requirements exactly (add -A → diff --cached --quiet unscoped →
  commit → CurrentSHA, with the specified return/error shapes), placed immediately
  after `StageAndCommit`, and the file-header enumeration at `gitrepo.go:1-3` lists it.
- `doc.go` reconciles both "never wildcard" assertions (Scope boundaries at
  `doc.go:74-85`, Push surface at `doc.go:87-96`), adds `StageAllAndCommit` to the
  Repo API list (`doc.go:35-36`), and the dangling `sync.go:pushUnpushed` /
  `hasUnpushed` cross-references are gone from both `doc.go` and `push.go`. The
  remaining `hasUnpushed` mentions in `push.go` (lines 60, 125-153) refer to
  gitrepo's own unexported method, not board's deleted function — not a stale
  reference.
- `gitrepo_test.go` adds all three required `TestStageAllAndCommit_*` cases
  (untracked+modified capture, clean-tree no-op, capturing a file an explicit list
  would miss), reusing the existing fixture helpers.
- `boardengine/sync.go` routes exclusively through `gitrepo.New`/`StageAllAndCommit`/
  `Push`, no `gitexec` import remains, the top-level `pushLockFile` is still acquired
  once and held across the whole loop per the option-1 locking decision, and the
  loop/`commitDirty` shapes match Card 4's spec line for line. `BoardPushError`,
  `pushUnpushed`, `hasUnpushed` are fully removed from the package (grep confirms
  zero remaining references anywhere in the repo).
- `git.go` and `boardtest/git_test.go` are deleted; no other file references
  `boardengine.Pull`/`CommitPush`.
- `board.go`'s package doc folds in the gitrepo-backed sync summary without
  restating the full lock protocol (Card 6).
- `manifest/designs/board-use-gitrepo.md` is deleted (Card 7); `manifest/roadmap.md`
  moves the item to Done referencing `internal/boardengine` (not the deleted design
  doc), and the `git-native-library` Planned item's bold-name cross-reference to
  `board-use-gitrepo` is left intact (Card 8).
- No out-of-plan files present; the "All Files Touched" union in `00-overview.md`
  matches the actual touched set exactly; `board`'s decision to use `Push` (never
  `PushCoalesced`/`CurrentSHA`/`SHAExists`/`ChangedFilesSince`/snapshot surface) holds
  throughout `boardengine` (grep confirms).

## Verdict

APPROVE
Implementation fully matches the plan, shared decisions, and constraints across both batches; no findings.
MILL_REVIEW_END
