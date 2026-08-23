# Batch: fabricengine-parent-resolution

```yaml
task: 'landing: parent-fabric resolution chain'
batch: fabricengine-parent-resolution
number: 1
cards: 8
verify: go test ./internal/fabricengine/... && go test -tags integration ./internal/fabricengine/...
depends-on: []
```

## Batch Scope

This batch builds the whole parent-fabric resolution chain inside `internal/fabricengine`, the package the Cwd Resolution Invariant and the `chain-lives-in-fabricengine` decision both assign this territory to: the new exported `OpenParent(l, parentBranch)` entry point, the `Prunable` porcelain field its matcher depends on, and the two vocabulary-neutral `Fabric` methods (`OriginURL`, `PushBranch`) `internal/loomcli` needs but cannot build itself (see the `fabric-vocabulary-owner-confinement` Shared Decision). It also updates this package's own doc comment to describe the new public surface, and adds full test coverage: an untagged unit test for the pure matcher and an integration test for `OpenParent` end to end, both driven through `hubforge` fixtures. Batch 4 (`loomcli-landing-wiring`) consumes `OpenParent`, `Fabric.OriginURL`, and `Fabric.PushBranch` as its external interface from this batch — nothing in this batch calls into `internal/loomcli`.

No card in this batch has a non-empty `Moves:` — every path is a plain edit or a new file.

## Cards

### Card 1: Parse the `prunable` porcelain line into `WorktreeEntry.Prunable`

- **Context:** none
- **Edits:**
  - `internal/fabricengine/worktreelist.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add a `Prunable bool` field, tagged `` `json:"prunable"` ``, to the `WorktreeEntry` struct (worktreelist.go:17-22), after the existing `Main bool` field.
  In `parseWorktreePorcelain`'s per-line loop (worktreelist.go:55-73), add a new branch that sets `entry.Prunable = true` whenever the trimmed line starts with the literal `"prunable"`.
  Use `strings.HasPrefix(line, "prunable")`, matching the existing `strings.HasPrefix(line, "branch ")`/`strings.HasPrefix(line, "worktree ")` style in the same loop — never `line == "prunable"`.
  Real `git worktree list --porcelain` output emits `prunable <reason>` (a reason string follows the keyword, e.g. `prunable gitdir file points to non-existent location`), never a bare `prunable` line, so an exact-equality match would silently never fire.
  Place the new branch anywhere among the existing `else if` chain; it does not overlap any existing prefix.
- **Commit:** `fabric: parse the prunable porcelain line into WorktreeEntry.Prunable`

### Card 2: Add the matcher and `OpenParent`

- **Context:**
  - `internal/lyxcwd/lyxcwd.go`
  - `internal/fabricengine/open.go`
  - `internal/fabricengine/fabric.go`
- **Edits:**
  - `internal/fabricengine/worktreelist.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add an unexported function `matchParentBranch(entries []WorktreeEntry, parentBranch string) (path string, ok bool)` to `worktreelist.go`, placed after `parseWorktreePorcelain`.
  It iterates `entries` in order, skips any entry with `Prunable == true` before checking its branch (a prunable entry is never a match, per the `prunable-entries-are-skipped-and-resolve-failures-name-the-branch` design decision), and returns the first entry whose `Branch == parentBranch`, with its `Path` normalized via `filepath.FromSlash` (mirroring `PrimeName`'s own normalization at worktreelist.go:95).
  It special-cases nothing else: a match whose `Branch` equals the caller's own acting worktree branch is an ordinary match, not an error (`self-parent-is-loom-policy-not-fabric-policy` — that refusal belongs to a future `internal/loomcli` caller, never here) and `Main` plays no role in matching.
  Returns `("", false)` when no live entry matches.

  Add an exported function `OpenParent(l *lyxcwd.Location, parentBranch string) (*Fabric, error)` to `worktreelist.go`, placed after `matchParentBranch`.
  It implements the four-step chain: call `List(l.AnchorPath())`; on error, wrap as `` fmt.Errorf("fabricengine: open parent fabric for branch %q: %w", parentBranch, err) ``.
  Call `matchParentBranch(entries, parentBranch)`; when `ok` is false, return a plain (unwrapped) error: `` fmt.Errorf("fabricengine: no live pair for parent branch %q", parentBranch) ``.
  Otherwise call `lyxcwd.ResolveWorktree(matchPath)` — never `lyxcwd.Resolve`, since the matched path is a worktree root, not an acting cwd, and `Resolve`'s strict cwd gate would spuriously reject it (see `lyxcwd.ResolveWorktree`'s own doc comment).
  On a `ResolveWorktree` error, or on a subsequent `Open(parentLoc)` error, wrap identically: `` fmt.Errorf("fabricengine: open parent fabric for branch %q at %q: %w", parentBranch, matchPath, err) `` — the same wrap for both step-3 and step-4 failures, always naming both the branch and the resolved path, never letting `Open`'s bare `*ErrMissingPath` or `ResolveWorktree`'s bare `lyxcwd.ErrNotAGitRepo` escape unwrapped.
  On success, return the opened `*Fabric` and a nil error.
  `OpenParent` performs no wiring and creates nothing — it is a pure read-and-open chain, exactly like the existing `Open(l)`.
- **Commit:** `fabric: add OpenParent parent-fabric resolution chain`

### Card 3: Re-export the matcher for unit testing

- **Context:** none
- **Edits:**
  - `internal/fabricengine/export_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add `var MatchParentBranchForTest = matchParentBranch` to `export_test.go`, following the file's existing re-export idiom (e.g. `var ExcludePatternForTest = excludePatternFor` — a bare var alias, no wrapper function), so `package fabricengine_test` files can drive `matchParentBranch` directly with hand-built `WorktreeEntry` slices, without a `hubforge` fixture.
- **Commit:** `fabric: re-export matchParentBranch for unit testing`

### Card 4: Add `Fabric.OriginURL` and `Fabric.PushBranch`

- **Context:**
  - `internal/gitrepo/remote.go`
  - `internal/fabricengine/spawn.go`
- **Edits:**
  - `internal/fabricengine/fabric.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add two new methods to `Fabric` in `fabric.go`, placed after `WeftLyxDir` (the last existing single-sided accessor in the file).

  `func (f *Fabric) OriginURL() (string, error)` delegates to the warp side's `RemoteURL`: `return f.warp.RemoteURL("origin")`.
  This is the single-sided-op-callable-from-outside-the-package carve-out `fabric.go`'s own package doc comment already states (the same carve-out `WeftWorktree` and the warp-only accessors named in that comment use), since `internal/loomcli` needs the origin URL and is not a Fabric Vocabulary Invariant owner.

  `func (f *Fabric) PushBranch(opts SyncOptions) (PushResult, error)` delegates to the package-level primitive: `return PushWarpRebaseFreeAt(f.warpPath, opts)`.
  This is the vocabulary-neutral spelling `internal/loomcli` must call instead of naming `PushWarpRebaseFreeAt` directly — see the `fabric-vocabulary-owner-confinement` Shared Decision.
  The method itself performs no discarding of the `PushResult`; that is the caller's choice (batch 4's `drive.go` discards it).
- **Commit:** `fabric: add Fabric.OriginURL and Fabric.PushBranch`

### Card 5: Document the new public surface

- **Context:** none
- **Edits:**
  - `internal/fabricengine/doc.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In the "`# The one-repo illusion at the public API boundary`" section of `doc.go`, immediately after the sentence ending `` "...performing no wiring of its own (wiring is Topology's job: Add/Checkout/Reconcile/Remove/Prune/Cleanup)." `` and before the sentence beginning `` "`Fabric.Commit`'s `CommitResult.Committed() bool` is the one commit result..." ``, insert a new sentence documenting the three additions this batch made: `OpenParent` is the parent-fabric resolution chain built on `List` — it matches the hub's worktrees against a caller-supplied branch, skipping any `Prunable` entry (git's own signal that a worktree's directory is gone but its administrative entry survives), resolves the match via `lyxcwd.ResolveWorktree`, and opens it through `Open`, returning a plain error a caller turns into `Stuck` when no live pair matches; `Fabric.OriginURL()` and `Fabric.PushBranch(opts)` are the two single-sided methods a caller like `internal/loomcli` reaches for under the same carve-out `Open`'s own bullet already states, `OriginURL` wrapping the warp side's `RemoteURL("origin")` and `PushBranch` wrapping `PushWarpRebaseFreeAt` under a vocabulary-neutral name `internal/loomcli` is not permitted to say itself.
  Write this as ordinary prose in this file's own style (long sentences, `` ` ``-quoted identifiers), not a bullet list.
- **Commit:** `fabric: document OpenParent, OriginURL, and PushBranch`

### Card 6: Unit test the matcher

- **Context:** none
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/matchparent_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create `matchparent_test.go`, package `fabricengine_test`, untagged (no `//go:build` line — this is a pure-function test needing no fixture).
  Write one table-driven test, `TestMatchParentBranch`, over `fabricengine.MatchParentBranchForTest` (card 3's re-export), covering these scenarios:
  - Exactly one entry matches the parent branch → that entry's `Path` is returned, `ok` is `true`.
  - No entry matches → `ok` is `false`, `Path` is empty.
  - The only candidate is detached (`Branch: "(detached)"`) → no match, no panic.
  - An entry's `Branch` equals the given `parentBranch` value even though nothing distinguishes it as the "acting worktree's own" branch in this pure-function view → it still matches (`ok` is `true`); comment this case explaining it is the `self-parent-is-loom-policy-not-fabric-policy` decision's own assertion that the matcher special-cases nothing, so a future edit does not "fix" it into an error.
  - An entry whose `Path` uses forward slashes (e.g. `"C:/hub/wt1"` or a Unix path with a doubled separator) is returned with `filepath.FromSlash` applied.
  - Given two entries where the matching one has `Main: false` and a non-matching one has `Main: true`, the match still returns correctly — `Main` plays no role in matching.
  - An entry with `Prunable: true` whose `Branch` equals the parent branch → no match (`ok` is `false`), even though it is the only entry with that branch.
- **Commit:** `fabric: unit test matchParentBranch`

### Card 7: Integration-test `OpenParent` end to end

- **Context:**
  - `internal/fabricengine/open_integration_test.go`
  - `internal/hubforge/hub.go`
  - `internal/fabricengine/export_test.go`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/openparent_integration_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create `openparent_integration_test.go`, `//go:build integration`, package `fabricengine_test`, mirroring `open_integration_test.go`'s style (imports, `hubforge.NewHub(t, ".")` fixture pattern, one `Test...` function per scenario, no subtests).
  Write these five test functions:

  `TestOpenParent_HappyPath` — build `h := hubforge.NewHub(t, ".")`, add a task pair via `res := hubforge.AddPair(t, h, "task1")`, resolve its location via `taskLoc, err := lyxcwd.ResolveWorktree(res.Path)` (fail the test on error), call `f, err := fabricengine.OpenParent(taskLoc, "main")` (`h`'s prime worktree is on branch `"main"`), assert `err` is nil and `f` is non-nil, then prove `f` is the *parent's* pair and not the task's by asserting `fabricengine.WarpForTest(f).CurrentBranch()` returns `"main"`.
  Also assert `f.OriginURL()` returns `filepath.ToSlash(h.WarpBare)` with a nil error — folding the `OriginURL` delegation assertion into this test rather than writing it a test of its own, per the plan's design.

  `TestOpenParent_NoLivePairForBranch` — build a hub, create a branch with no worktree via `gitkit.MustRun(t, h.PrimeWorktree(), "git", "branch", "orphan-branch")`, call `fabricengine.OpenParent(h.Location, "orphan-branch")`, assert the error is non-nil and its message contains `"orphan-branch"`.

  `TestOpenParent_ParentSiblingMissing` — build a hub, add a task pair via `hubforge.AddPair`, resolve its location, delete the *hub's own* weft sibling via `os.RemoveAll(h.PrimeWeft())` (leaving the task pair itself untouched), call `fabricengine.OpenParent(taskLoc, "main")`, assert the error is non-nil, unwraps via `errors.As` to `*fabricengine.ErrMissingPath`, and that its `Path` field equals `h.PrimeWeft()`.

  `TestOpenParent_PrunableParentDirRemoved` — build a hub, add a second pair via `res := hubforge.AddPair(t, h, "task2")`, delete its directory without pruning (`os.RemoveAll(res.Path)`), call `fabricengine.OpenParent(h.Location, res.Branch)`, assert the error is non-nil, its message contains `res.Branch`, and it does **not** unwrap to `*fabricengine.ErrMissingPath` (`errors.As` returns `false`) — proving the prunable entry is reported as "no live pair," never as a broken pair.

  `TestOpenParent_ResolveFailureNamesBranchAndPath` — build a hub, add a pair via `res := hubforge.AddPair(t, h, "task3")`, run `gitkit.MustRun(t, h.PrimeWorktree(), "git", "worktree", "lock", res.Path)` to mark it locked (which suppresses git's own `prunable` porcelain line even once its directory is gone — the deterministic way to construct "the path existed at `List` time, is gone by `ResolveWorktree` time" without a real race), then `os.RemoveAll(res.Path)`, then call `fabricengine.OpenParent(h.Location, res.Branch)`.
  Assert the error is non-nil and its message contains both `res.Branch` and `res.Path` — proving the wrap in card 2's `OpenParent` names the branch and the resolved path on a step-3 resolve failure, never surfacing git's own bare "not a git repository" text unqualified.
  Comment this test explaining the locking trick, since it is not obvious from the assertions alone.
- **Commit:** `fabric: integration-test OpenParent end to end`

### Card 8: Cover the prunable porcelain-parser case

- **Context:**
  - `internal/hubforge/hub.go`
- **Edits:**
  - `internal/fabricengine/worktreelist_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add a new test function `TestList_ParsesPrunable` to `worktreelist_test.go` (same file, same `//go:build integration` tag, same `fabricengine_test` package as `TestList`), covering `parseWorktreePorcelain`'s new `Prunable` branch (card 1) through the public `List` entry point — not a new file, per the plan's discussion of where this coverage belongs.
  Build a hub via `hubforge.NewHub(t, ".")`, add a second worktree via `gitkit.MustRun(t, hub, "git", "worktree", "add", wtPath)` (mirroring `TestList`'s own `TwoWorktrees` case), delete `wtPath` with `os.RemoveAll` (no `git worktree prune`), then call `fabricengine.List(hub)`.
  Assert the returned entries contain exactly one with `Prunable == true` (the deleted one) and that the hub's own prime entry has `Prunable == false` — covering both the "a block containing a `prunable` line sets `Prunable`" and "one without it leaves the field false" cases in a single call.
- **Commit:** `fabric: cover prunable porcelain parsing in List`

## Batch Tests

`verify:` runs `go test ./internal/fabricengine/...` (covers cards 1-6, all untagged) chained with `go test -tags integration ./internal/fabricengine/...` (covers cards 7-8, the two `//go:build integration` files), per the `verify-scoped-per-package` Shared Decision.
Card 6's `matchparent_test.go` exercises `matchParentBranch` directly with no fixture; cards 7-8 exercise `OpenParent` and the porcelain parser's new field against real `hubforge` fixtures.
