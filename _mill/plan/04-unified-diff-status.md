# Batch: unified-diff-status

```yaml
task: 'fabric: Fabric.Commit classify+dispatch + unified diff/status'
batch: unified-diff-status
number: 4
cards: 3
verify: go test -tags integration ./internal/gitrepo/ ./internal/fabricengine/
depends-on: []
```

## Batch Scope

This batch adds the Go-internal unified `Fabric.Diff`/`Status`, independent of the commit path (new files plus a single doc-only addendum to `gogit.go`, none of which any other batch touches), so it is a root batch. It delivers the new read-only `gitrepo.Repo` worktree-changed-files primitive that `Fabric.Status` needs (backed by go-git's `Worktree.Status()`, a pure on-disk read that stays in go-git's half of the **gitrepo Client Boundary Invariant** — no invariant change, no pinned-CLI-set edit, no new `gitexec` call), the nearest-older weft-anchor bridge `Fabric.Diff` uses instead of the exact `WeftSHAForWarpSHA`, and the two methods plus their side-labelled result type. Batch-local decision: the new unified types (`ChangeSide`/`ChangeEntry`/`DiffResult`) are named to avoid colliding with `status.go`'s existing `StatusResult`/`PairStatus` (the paired host↔weft topology view), which is a **different** thing from both `StatusWeft` (a dirty/ahead/behind bool view) and this new "what changed in my worktree" view — three distinct surfaces, per `_mill/discussion.md`'s Technical context.

## Cards

### Card 11: gitrepo worktree-changed-files primitive

- **Context:**
  - `internal/gitrepo/gitrepo.go`
  - `internal/gitrepo/gitrepo_test.go`
- **Edits:**
  - `internal/gitrepo/gogit.go`
- **Creates:**
  - `internal/gitrepo/worktree.go`
  - `internal/gitrepo/worktree_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** In a new `internal/gitrepo/worktree.go` add `func (r *Repo) WorktreeChangedFiles() ([]string, error)` — a read-only working-tree read via go-git: obtain the handle with `r.goGit()`, then `repo.Worktree()` and `wt.Status()`, holding `r.goGitMu.Lock()` for the `Status()` call (it builds/uses go-git's lazy object index, so it follows the write-lock discipline `gogit.go` documents for object-touching reads, not the plain-ref `RLock` path). Return the set of repo-relative paths whose `git.FileStatus` shows `Staging != git.Unmodified` OR `Worktree != git.Unmodified` (any uncommitted change — tracked, staged, or untracked), de-duplicated; the returned order is not contractual (mirror `ChangedFilesSince`'s set posture). `wt.Status()` internally calls `gitignore.ReadPatterns`, which reads `.git/info/exclude` first (confirmed in the vendored go-git v5.19.1: `plumbing/format/gitignore/dir.go`'s `ReadPatterns` → `readIgnoreFile(fs, path, infoExcludeFile)` where `infoExcludeFile = ".git/info/exclude"`), so files fabric git-excludes there (the `.weft/` lock dir, `.gitrepo-push.lock`) are already omitted from `Status()` — no separate exclude-file filtering is needed in `WorktreeChangedFiles`. This introduces **no** `gitexec` call inside the `gitrepo` package, so the Client Boundary Invariant's pinned set and the constraints doc stay unchanged. Add a `//go:build integration` `worktree_test.go` **in package `gitrepo_test`** (the same external test package as the existing `gitrepo_test.go`), reusing that file's `newRepo`/`writeFile`/`commitAll` fixture helpers rather than redeclaring same-named helpers (a same-package compile collision) — build fixtures with those helpers plus `lyxtest.MustRun` for the staged-file case: a clean repo returns an empty set; a repo with a modified tracked file, a newly-created untracked file, and a separately-staged file returns exactly those paths. Additionally, extend `gogit.go`'s package locking-discipline doc comment (the two-bullet list under "The package's locking discipline, stated once here …", covering the plain-ref `RLock` read and the `lookupObjectRetrying` object-lookup categories) with a **third** bullet: a working-tree scan (`Worktree.Status()`, via `WorktreeChangedFiles`) acquires `r.goGitMu.Lock` directly (not `RLock`, and not routed through `lookupObjectRetrying`) for the duration of the single `Status()` call — a write-locked, non-retried third category, since `Status()` builds/uses the lazy object index once with no reindex-retry loop.
- **Commit:** `feat(gitrepo): add WorktreeChangedFiles working-tree read via go-git`

### Card 12: Nearest-older weft anchor + Fabric.Diff/Status

- **Context:**
  - `internal/fabricengine/revert.go`
  - `internal/fabricengine/index.go`
  - `internal/fabricengine/corrindex.go`
  - `internal/fabricengine/fabric.go`
  - `internal/fabricengine/status.go`
  - `internal/gitrepo/gitrepo.go`
  - `internal/gitrepo/worktree.go`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/diff.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** In a new `internal/fabricengine/diff.go` add the nearest-older bridge `func (f *Fabric) weftAnchorForWarpSHA(warpSHA string) (weftSHA string, found bool, err error)`: compute `targetSeq, err := f.warpSeq(warpSHA)`; call the existing `res, err := f.resolveRevertTarget(warpSHA, targetSeq)` (in `revert.go` — it already does exact-then-nearest resolution via `classifyCorrespondence`/`nearestAtOrBefore` plus the `SHAExists`→`RebuildIndex`→retry stale-heal); on `errors.Is(err, ErrNoCorrespondence)` return `("", false, nil)` (a `warpSHA` older than the first weft commit is a valid pre-lyx state, not an error); on any other error return it; otherwise return `(res.Entry.WeftSHA, true, nil)`. Add the side-labelled result types (chosen to not collide with `status.go`'s `StatusResult`/`PairStatus`): `type ChangeSide string` with `const (SideWarp ChangeSide = "warp"; SideWeft ChangeSide = "weft")`, `type ChangeEntry struct { Path string; Side ChangeSide }`, and `type DiffResult struct { Entries []ChangeEntry; NoWeftCorrespondence bool }`. Add `func (f *Fabric) Diff(sinceWarpSHA string) (DiffResult, error)`: warp changes from `f.Warp.ChangedFilesSince(sinceWarpSHA)`; resolve the weft anchor via `weftAnchorForWarpSHA` — when `found`, weft changes from `f.Weft.ChangedFilesSince(weftAnchor)`; when not found, no weft entries and `NoWeftCorrespondence = true` (not an error); merge into one `[]ChangeEntry` labelled by side. Add `func (f *Fabric) Status() ([]ChangeEntry, error)`: merge `f.Warp.WorktreeChangedFiles()` and `f.Weft.WorktreeChangedFiles()` into one side-labelled `[]ChangeEntry` (no correspondence anchor, so no `NoWeftCorrespondence`). Give the file a godoc header distinguishing this unified `Fabric.Status` from `Topology.Status` and `StatusWeft`.
- **Commit:** `feat(fabric): add Go-internal unified Fabric.Diff and Fabric.Status`

### Card 13: Integration tests — unified diff and status

- **Context:**
  - `internal/fabricengine/diff.go`
  - `internal/fabricengine/weftgit.go`
  - `internal/fabricengine/syncweft.go`
  - `internal/fabricengine/index_integration_test.go`
  - `internal/fabricengine/syncweft_integration_test.go`
  - `internal/gitrepo/worktree.go`
  - `internal/lyxtest/lyxtest.go`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/diff_integration_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add a `//go:build integration` file (package `fabricengine`) reusing the existing fixtures. For `Fabric.Diff`: after two `SyncWeft` rounds on a warp/weft pair (which record correspondence), assert `Diff(earlierWarpSHA)` returns a merged `[]ChangeEntry` with warp-side paths labelled `SideWarp` and weft-side paths labelled `SideWeft`, anchored via the nearest-older weft SHA. Cover the **nearest-older** path with a warp-only anchor that has no exact correspondence (advance warp with an un-synced commit, à la `TestRevertWithWeft_Gap_ResetsToNearestOlderAndReportsRange`) and assert the weft side resolves to the nearest older synced weft baseline rather than empty. Cover the **no-correspondence** path with a `sinceWarpSHA` older than the first weft commit and assert warp entries present, weft entries empty, `NoWeftCorrespondence == true`, and no error. For `Fabric.Status`: run one `CommitWeft`/`SyncWeft` first (so `ensureWeftLockDir` seeds the weft `.git/info/exclude`), then create uncommitted changes in both the warp and weft worktrees and assert the merged `[]ChangeEntry` reports both sides' uncommitted paths with correct side labels, and that fabric's own operational artifacts (the `.weft/` lock dir, the `.gitrepo-push.lock` file) do **not** appear (they are git-excluded) — verifying the go-git primitive honors `.git/info/exclude`. (That go-git behavior is confirmed against v5.19.1 in Card 11's Requirements, so this assertion is a regression guard over a verified behavior, not a leap on an unexercised library assumption.)
- **Commit:** `test(fabric): cover unified Diff nearest-older bridge and Status merge`

## Batch Tests

`verify: go test -tags integration ./internal/gitrepo/ ./internal/fabricengine/` runs the new `gitrepo` primitive test (`worktree_test.go`) and the new fabric diff/status integration file, plus both packages' existing suites. The `gitrepo` package already has a `HermeticGitEnv` `TestMain` (`internal/gitrepo/testmain_test.go`). No fabric-config seeding is needed here — `Fabric.Diff`/`Status` do not classify paths and therefore never call `WiredNames`; the diff tests drive correspondence through `SyncWeft`/`CommitWeft`'s explicit pathspec, exactly like the existing `syncweft_integration_test.go` cases.
