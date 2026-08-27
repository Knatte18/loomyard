# Batch: push-and-mergestate-probe

```yaml
task: "Add a local-only file category to weft"
batch: "push-and-mergestate-probe"
number: 5
cards: 4
verify: go build ./cmd/lyx && go test ./internal/fabricengine/... && go test -tags integration ./internal/fabricengine/...
depends-on: [3]
```

## Batch Scope

This batch adds the two fabric-vocabulary-neutral entry points the loom-side per-transition closure will consume in batch 7, and nothing else: `PushAnchored(l, opts)`, the synchronous push mirroring `CommitAnchoredPaths`' shape, and `MergeStateActive(l)`, the weft-only mid-merge probe the closure consults before committing.
Both take a `*lyxcwd.Location` and return no path, so a caller outside the Fabric Vocabulary Invariant's owner set never learns the weft exists.
Neither has a production caller until batch 7;
that is deliberate, so the two seams land with their own direct tests rather than only being exercised through a wiring closure.

The `depends-on: [3]` edge exists for `internal/fabricengine/doc.go`, which batches 1, 2, 3 and this batch all edit — not for any behavioural dependency.

Batch-local decisions beyond `## Shared Decisions`:

- `PushAnchored`'s underlying primitive is `gitrepo.PushRebaseFree`, never `gitrepo.PushCoalesced`.
  `PushCoalesced`'s `pushWithRebaseRetry` path runs `git pull --rebase` on a rejected push, rewriting this side's SHAs and invalidating the correspondence index — contradicting `correspondence-unchanged` and turning a rejection into a silent history rewrite of a *running* weft — and it takes a repo-root push-lock file that would contend with `SpawnDetachedPush` children and landing-time pushes on every transition.
  This is the same choice `PushWarpRebaseFreeAt` already made, for the same two reasons.
- `MergeStateActive` probes the **weft alone**, deliberately, and is not the two-sided form the unexported `foreignMergeStatePresent` uses.
  Warp and weft are independent clones with independent `.git`, and the status commit runs in the weft worktree, so warp-side git state cannot block it.
  Inheriting the two-sided form would freeze every status commit for the whole duration of a live warp conflict-resolution session — the one moment a resuming machine most needs to know the run is Stuck and since when.
- `MergeStateActive` surfaces a probe error rather than swallowing it.
  The closure, not the probe, decides that an error means skip.

## Cards

### Card 21: add PushAnchored

- **Context:**
  - `internal/fabricengine/commitweftpaths.go`
  - `internal/fabricengine/spawn.go`
  - `internal/fabricengine/weftgit.go`
  - `internal/fabricengine/fabric.go`
  - `internal/gitrepo/push.go`
  - `internal/lyxcwd/lyxcwd.go`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/pushanchored.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/fabricengine/pushanchored.go` in `package fabricengine`, with a file-header comment in the package's existing style stating that it holds `PushAnchored`, the vocabulary-neutral synchronous push beside `CommitAnchoredPaths`.
  Declare `func PushAnchored(l *lyxcwd.Location, opts SyncOptions) (res PushResult, err error)`.
  Its body, mirroring `PushWarpRebaseFreeAt` (`internal/fabricengine/spawn.go`) exactly:
  resolve the target as `WeftWorktree(l)`;
  build `rec := NewMutations(filepath.Dir(<that path>))` and stamp `res.Mutations = rec.Snapshot()` from a deferred call, so every return carries a record;
  return `(PushResult{}, nil)` immediately when `opts.SkipGit || opts.SkipPush` is true, with no lock taken and nothing recorded;
  build `repo := gitrepo.New(<that path>)`, sample `hadUnpushed, hadUnpushedErr := repo.HasUnpushed()`, call `repo.PushRebaseFree()` and return its error unwrapped on failure, then call `recordPushIfAdvanced(rec, repo, hadUnpushed, hadUnpushedErr)`.
  Returning the error unwrapped is load-bearing and must be stated in the doc comment: `gitrepo.PushRebaseFree` returns the `gitrepo.ErrPushRejected` sentinel bare rather than wrapped, and batch 7's closure matches exactly that sentinel with `errors.Is` to warn-and-continue on a rejection while treating every other push error differently.
  Write a doc comment mirroring `CommitAnchoredPaths`' own shape — `l`-in, no path out, anchor-resolved target — and naming, as `PushWarpRebaseFreeAt`'s comment does, why `PushCoalesced` is disqualified twice over.
  Take no lock: `PushRebaseFree` is lock-free by construction, which is half of why it was chosen.
  `PushResult` already embeds `MutationRecord`, so this satisfies the Mutation Record Invariant without a `rec *Mutations` parameter — matching `PushWarpAt`/`PushWarpRebaseFreeAt`, neither of which takes a recorder.
- **Commit:** `feat(fabricengine): add PushAnchored, a vocabulary-neutral synchronous push`

### Card 22: add MergeStateActive

- **Context:**
  - `internal/fabricengine/mergestate.go`
  - `internal/fabricengine/mergelifecycle.go`
  - `internal/fabricengine/commitweftpaths.go`
  - `internal/fabricengine/fabric.go`
  - `internal/gitrepo/merge.go`
  - `internal/lyxcwd/lyxcwd.go`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/mergestateactive.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/fabricengine/mergestateactive.go` in `package fabricengine`, with a file-header comment in the package's existing style.
  Declare `func MergeStateActive(l *lyxcwd.Location) (bool, error)`.
  Its body: build `repo := gitrepo.New(WeftWorktree(l))`, call `repo.MergeHeadPresent()` and `repo.ConflictedFiles()`, evaluating both unconditionally before combining, wrap each error as `fmt.Errorf("fabricengine: check merge head: %w", err)` and `fmt.Errorf("fabricengine: check conflicted files: %w", err)` respectively, matching `foreignMergeStatePresent`'s existing wording, and return `mergeHeadPresent || len(conflicted) > 0`.
  The doc comment must state four things a reader cannot infer from the body: that it answers "is the weft mid-merge at the git level" rather than "does fabric have a merge in progress";
  that it is weft-only by design, with the independent-`.git` and frozen-warp-session reasoning;
  that `Fabric.MergeInProgress` cannot serve as this probe, because it is `mergeRecordExists()`'s bare boolean, never consults `foreignMergeStatePresent`, is therefore false in precisely the foreign-state case this probe exists for, and needs an open `*Fabric` a caller closure does not hold;
  and that a non-nil error is surfaced rather than swallowed, leaving the decision that an unreadable probe means skip to the caller.
  It takes an `l *lyxcwd.Location`, not an open `*Fabric`, matching `CommitAnchoredPaths` and `PushAnchored`.
  This is a read-only probe, so it returns no result type and embeds no `MutationRecord`, per the Mutation Record Invariant's "a read-only one must not" clause.
- **Commit:** `feat(fabricengine): add MergeStateActive, the weft-only mid-merge probe`

### Card 23: name the two new seams in the package narrative

- **Context:**
  - `internal/fabricengine/pushanchored.go`
  - `internal/fabricengine/mergestateactive.go`
  - `internal/fabricengine/commitweftpaths.go`
  - `_mill/discussion.md`
- **Edits:**
  - `internal/fabricengine/doc.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/fabricengine/doc.go`, extend the section describing the vocabulary-neutral, `l`-in entry points — the one that already covers `CommitAnchoredPaths`, `ReadOrigin` and `WriteOrigin` — with `PushAnchored` and `MergeStateActive`.
  If no such section exists, add the two to the `# The one-repo illusion at the public API boundary` section instead, which is where the `l`-in shape is already explained.
  State for each what it is for in one or two sentences: `PushAnchored` is the synchronous, rebase-free counterpart to `CommitAnchoredPaths`, whose `gitrepo.ErrPushRejected` a caller is expected to treat as a human-decidable condition rather than retrying;
  `MergeStateActive` is the weft-only git-level mid-merge probe a path-scoped commit must consult, distinct from both `Fabric.MergeInProgress` and the two-sided `foreignMergeStatePresent`.
  Keep every existing heading text in the file unchanged, since `docs/` and `manifest/` link into this package's documentation.
- **Commit:** `docs(fabricengine): document PushAnchored and MergeStateActive`

### Card 24: direct tests for the two new seams

- **Context:**
  - `internal/fabricengine/pushanchored.go`
  - `internal/fabricengine/mergestateactive.go`
  - `internal/fabricengine/commitweftpaths_test.go`
  - `internal/fabricengine/mergein_integration_test.go`
  - `internal/fabricengine/mergein_recovery_integration_test.go`
  - `internal/fabricengine/export_test.go`
  - `internal/fabricengine/testmain_test.go`
  - `internal/gitrepo/push.go`
  - `internal/hubforge/hub.go`
  - `internal/gitkit/gitkit.go`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/pushanchored_integration_test.go`
  - `internal/fabricengine/mergestateactive_integration_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create both files in `package fabricengine_test`, each opening with the `//go:build integration` constraint and a file-header comment.
  `pushanchored_integration_test.go` covers four properties, one test function each:
  (1) `PushAnchored` with `SyncOptions{SkipGit: true}` returns a nil error, pushes nothing, and yields an empty mutation record;
  (2) the same for `SyncOptions{SkipPush: true}`;
  (3) a weft carrying an unpushed commit is genuinely pushed, the remote tip advances to it, and the result's mutation record carries exactly one `KindBranchPushed` entry;
  (4) a weft diverged from its own upstream surfaces `gitrepo.ErrPushRejected` such that `errors.Is(err, gitrepo.ErrPushRejected)` is true — the unwrapped-sentinel property batch 7's closure depends on — and that a push error of a different kind does NOT match that sentinel.
  `mergestateactive_integration_test.go` covers four properties, one test function each:
  (1) a clean weft reports `false`;
  (2) a weft with a live `MERGE_HEAD` reports `true`;
  (3) a weft with a non-empty conflicted index and no `MERGE_HEAD` — a conflicted `git merge --squash` — reports `true`, so neither probe kind is redundant;
  (4) a warp alone mid-merge, with the weft clean, reports `false`, which is the case pinning the weft-only scope.
  Reuse the package's existing fixture helpers rather than adding new ones.
- **Commit:** `test(fabricengine): pin PushAnchored and MergeStateActive`

## Batch Tests

`verify:` runs `go build ./cmd/lyx`, then the untagged tier over `./internal/fabricengine/...`, then the `integration` tier over the same package.

- The `integration` tier is chained separately because card 24 creates two `//go:build integration` files;
  they are the whole of this batch's behavioural proof, since neither new function has a production caller until batch 7.
- Scenario 4 of `pushanchored_integration_test.go` is the load-bearing one: batch 7's closure warns and continues on exactly `gitrepo.ErrPushRejected` and on no other error, so a `PushAnchored` that wrapped the sentinel would silently turn a routine rejection into a run-halting error.
- Scenario 4 of `mergestateactive_integration_test.go` is the load-bearing one for the probe: a two-sided implementation passes the other three scenarios and fails only this one.
- The scope stays one package;
  `pipeline.done_gate` runs the repo-wide sweep at the end of the run.
