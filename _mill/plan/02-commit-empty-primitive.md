# Batch: commit-empty-primitive

```yaml
task: 'fabric: fold snapshot-tracking into the Warp-SHA trailer'
batch: commit-empty-primitive
number: 2
cards: 4
verify: go test -tags integration -count=1 ./internal/gitrepo/... ./cmd/lyx/...
depends-on: [1]
```

## Batch Scope

This batch adds one new primitive to `internal/gitrepo` — `CommitEmpty(msg string) (sha string, err error)` — together with its typed `ErrIndexNotEmpty`, its integration coverage, its entry in the boundary guard's pinned method set, its clause in `CONSTRAINTS.md`'s CLI-bound set, and its section in the package doc. The external interface batch 4 consumes is exactly that method signature and that error value.

It depends on batch 1 because both batches edit `cmd/lyx/gitrepoboundary_test.go`, `CONSTRAINTS.md`, and `internal/gitrepo/doc.go`, and because batch 1 removes three names from the pinned set while this batch adds one — doing them in the wrong order leaves the guard red for a reason unrelated to the card being worked.

**Batch-local decision — `CommitEmpty` lives in `gitrepo.go`, beside `StageAndCommit`, not in a new file.** A new file would change the package's non-test file count, which `gitrepoBoundaryMinScannedFiles`'s doc comment enumerates and batch 1 has just corrected. Keeping it in `gitrepo.go` also puts it next to the primitive whose never-sweep norm it is approximating, where a reader comparing the two contracts will find both.

**`internal/gitrepo` is geometry-blind and stays so.** `CommitEmpty` takes only a message. It knows nothing about warp, weft, trailers, or snapshot tags; all trailer composition stays in `fabricengine`.

## Cards

### Card 6: Add ErrIndexNotEmpty and CommitEmpty to gitrepo

- **Context:**
  - `internal/gitrepo/gogit.go`
  - `internal/gitrepo/reset.go`
- **Edits:**
  - `internal/gitrepo/gitrepo.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add `var ErrIndexNotEmpty` beside the existing `ErrNoCommits` and `ErrInvalidSHA` sentinels, checkable via `errors.Is`. Add `func (r *Repo) CommitEmpty(msg string) (sha string, err error)` immediately after `StageAndCommit`. It returns no `committed bool` — an empty commit cannot no-op, so it always commits when it commits at all. The method has two steps. **Step one, the index pre-check**, branches on the born/unborn state using the package's existing **typed** detection: call `r.CurrentSHA()` and treat a `ErrNoCommits` result (via `errors.Is`) as unborn. Never match git's stderr text to decide this — `CurrentSHA` migrated to go-git's typed `plumbing.ErrReferenceNotFound` precisely so callers stop depending on git's English output. On a **born** HEAD, run `git diff --cached --quiet` through `r.run` and use the exact exit-code mapping `StageAndCommit` already uses in its own `switch`: exit 0 means the index matches HEAD, so proceed; exit 1 means staged differences exist, so return `ErrIndexNotEmpty`; any other exit is a genuine git failure and is returned as an error including stderr. On an **unborn** HEAD there is no HEAD to diff against, so do **not** use `diff --cached` at all — run `git ls-files --cached` instead and treat empty output as "proceed" and any output as `ErrIndexNotEmpty`. That form is exact, needs no empty-tree object constant (which is hash-algorithm-dependent and would be a latent SHA-256 bug), and does not bet the contract on `diff --cached`'s unborn-HEAD semantics. The unborn path is reachable in production, not hypothetical: batch 4's unborn-weft case routes straight through it. **Step two** runs `git commit --allow-empty -m <msg>` through `r.run` and then reads the new SHA via `r.CurrentSHA()`, mirroring `StageAndCommit`'s own tail. Both git invocations go through `r.run` — never `gitexec.RunGit` directly — because the boundary guard asserts the bare token `gitexec.` appears exactly once in the package's non-test source, inside `run`'s own body. Write `CommitEmpty`'s godoc to say it **refuses if the index is dirty when checked**, and state the residual race plainly rather than claiming the guarantee `StageAndCommit` has: `StageAndCommit` is structurally safe because it scopes its commit with `git commit ... -- <files>`, which cannot commit an unlisted path no matter what races it, whereas `CommitEmpty` has no pathspec to scope with and is check-then-commit across two git spawns, so an index write landing between them is still swept. The window is milliseconds and fabric's own callers serialise under the combined write lock, but the doc must not over-claim; a future reader must not build on a guarantee that is not there.
- **Commit:** `fabric: add gitrepo.CommitEmpty with a dirty-index refusal`

### Card 7: Integration coverage for CommitEmpty

- **Context:**
  - `internal/gitrepo/gitrepo.go`
  - `internal/gitrepo/testmain_test.go`
  - `internal/gitrepo/gitrepo_test.go`
  - `internal/gitrepo/reset_test.go`
- **Edits:** none
- **Creates:**
  - `internal/gitrepo/commitempty_integration_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** New file, first non-empty line `//go:build integration`, in whichever of the package's two test packages the existing git-spawning fixtures it reuses already live — read `gitrepo_test.go` and `reset_test.go` first and follow their package and fixture conventions rather than inventing a new harness. The file spawns git, so the Test Tier Purity Invariant requires the build tag; `internal/gitrepo/testmain_test.go` already provides `lyxtest.HermeticGitEnv()` for the package, so no new `TestMain` is needed. Cover five cases. **Born HEAD, clean index:** `CommitEmpty` returns a SHA that `SHAExists` confirms, and the new commit's tree equals its parent's — assert the tree identity directly, because batch 4's `empty-commits-take-over-the-correspondence-entry` decision rests on it. **Two successive calls** produce two distinct SHAs, pinning that an empty commit is never deduplicated into a no-op. **Unborn HEAD, clean index:** `CommitEmpty` creates the repository's root commit, empty. This is a specified contract, not incidental behaviour — `fabricengine` reaches it in batch 4 whenever weft has no commits yet. **Unborn HEAD with a staged file:** `ErrIndexNotEmpty`, checkable via `errors.Is`, and nothing committed. This is the case that proves the pre-check was specified for both states rather than only the born one, and it is the only test that exercises the `git ls-files --cached` branch. **Born HEAD with a staged-but-uncommitted file:** `ErrIndexNotEmpty` and nothing committed, so the never-sweep intent holds on the ordinary path too.
- **Commit:** `fabric: integration coverage for gitrepo.CommitEmpty`

### Card 8: Pin CommitEmpty in the boundary guard and CONSTRAINTS.md

- **Context:**
  - `internal/gitrepo/gitrepo.go`
- **Edits:**
  - `cmd/lyx/gitrepoboundary_test.go`
  - `CONSTRAINTS.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add `"CommitEmpty"` to `gitrepoPinnedRunBoundMethods` in `cmd/lyx/gitrepoboundary_test.go`. This is mandatory, not optional: the guard asserts the set by equality, so without it `TestGitrepoBoundary_PinnedRunCallSites` fails with "r.run-bound but not pinned". `CommitEmpty` contains two `r.run` call sites (the pre-check and the commit itself), which the guard counts as one method either way — but that makes it a second worked example of the blind spot batch 1's Card 3 re-documented on `StageAndCommit`, so say so where that comment now sits. In `CONSTRAINTS.md`'s `## gitrepo Client Boundary Invariant`, add `CommitEmpty` to the **Statement** bullet's exhaustively-named CLI-bound set. That entry's own rule requires this: any new `gitexec` call added inside the gitrepo package must come with an updated entry in the same commit, and widening the CLI-bound set without editing the list is itself a violation. Justify it in one clause — `CommitEmpty` mutates the repository's history, which is squarely on the `gitexec` side of the split, and go-git offers no equivalent that respects the pre-check.
- **Commit:** `fabric: pin CommitEmpty on the gitrepo client boundary`

### Card 9: Document CommitEmpty in the gitrepo package doc

- **Context:**
  - `internal/gitrepo/gitrepo.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/gitrepo/doc.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add `CommitEmpty` to the `# The Repo API` section's method inventory, and describe the primitive in the same register the rest of that file uses: what it is for (recording a commit whose only content is its message and trailers), why neither existing primitive can be coaxed into it (`StageAndCommit` returns the no-op signal on an empty file list by deliberate design, and its `diff --cached --quiet` gate returns the same no-op on unchanged content), the two-state pre-check and why the unborn branch uses `git ls-files --cached` rather than `diff --cached`, and the honest statement of the residual check-then-commit window from Card 6's godoc. State the never-sweeping norm as this file already frames it — a package-wide norm that `StageAndCommit` enforces structurally through pathspec scoping and that `CommitEmpty`, having no pathspec, can only approximate by refusing. Do not describe warp, weft, trailers, or snapshots here: the gitrepo package is geometry-blind and this doc must stay so.
- **Commit:** `fabric: document CommitEmpty in gitrepo's package doc`

## Batch Tests

`verify: go test -tags integration -count=1 ./internal/gitrepo/... ./cmd/lyx/...` — the same scope batch 1 uses, for the same two reasons: `cmd/lyx` hosts the set-equality boundary guard this batch modifies, and the module-walking tier-purity and hermetic-env guards can fire on any test file added under `internal/gitrepo`. The `-tags integration` build also compiles and runs the package's untagged tests, so one command covers both tiers.

The new coverage is `internal/gitrepo/commitempty_integration_test.go`'s five cases. Two of them carry more weight than their size suggests. The unborn-HEAD-with-a-staged-file case is the only exercise of the `git ls-files --cached` branch, and that branch exists solely because batch 4 reaches the unborn path in production — without this test the branch would ship unwitnessed. The born-HEAD tree-identity assertion is what batch 4's correspondence-overwrite decision rests on: if an empty commit's tree ever differed from its parent's, resolving a revert target to the empty commit would silently restore a different weft tree, and the argument that the overwrite is benign would collapse.
