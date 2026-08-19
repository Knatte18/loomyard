MILL_REVIEW_BEGIN
# Review: fabric: merge-conflict primitive

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-19
```

## Findings

### [BLOCKING:consistency] Sync guard mutates while guards claim no mutation
**Section:** `safety-guards-are-aggregated-and-side-free`
**Issue:** `Merge`'s upstream-sync guard performs a fetch and `merge --ff-only` — real ref movement on both sides — yet the same decision states "A guard failure halts before any mutation, so there is nothing to roll back", and the aggregate-every-guard rule implies the sync still runs even after the dirty-worktree guard has already failed.
**Fix:** State whether the sync is a guard or a pre-merge mutating step, fix its position relative to the other guards (evaluated but only executed once all pass, or moved out of the guard set), and say whether the ref movement is recorded as a mutation and whether `MergeAbort`'s pre-merge SHAs are captured before or after it.

### [BLOCKING:consistency] Wrong config base named for the wired name-set
**Section:** `conflicts-are-reported-as-unified-worktree-relative-paths`
**Issue:** The decision names the config base as "derived from the hub, `filepath.Dir(f.warpPath)`" and then equates it with `LoadConfig(repoWideFabricBase(l))`; these are different values — `repoWideFabricBase(l)` is `BoardDir(l.HubPath)` (`junctionnames.go:279`), i.e. `<hub>/_board`, while `filepath.Dir(f.warpPath)` is `<hub>` itself (`destroy.go:1143`, container only, never a config base). A literal implementation reads fabric.yaml at the wrong base.
**Fix:** Name one derivation — the merge path already calls `lyxcwd.ResolveWorktree(f.warpPath)`, so `RepoWiredNames(l)` (`junctionnames.go:287`) is the exact single call — and drop the `filepath.Dir(f.warpPath)` equivalence.

### [BLOCKING:design] MergeConclude's empty-message path invokes an editor
**Section:** `public-surface-shapes` (`MergeConclude`)
**Issue:** The pinned command is `git commit [-m <msg>]` with the empty-message case "falling back to git's prepared MERGE_MSG/SQUASH_MSG, exactly as a non-interactive `git commit` would" — but `git commit` with no `-m` and no `--no-edit`/`-F` launches the configured editor for both a `--no-commit` merge conclusion and a `--squash` commit; every existing `gitrepo` commit site (`gitrepo.go:118,159,189`) passes `-m` and no code path sets `GIT_EDITOR`.
**Fix:** Pin the non-interactive spelling explicitly (`git commit --no-edit`, or `-F <gitdir>/MERGE_MSG`/`SQUASH_MSG`) in the `MergeConclude` contract.

### [NIT:design] Which mechanism detects both-sides-already-up-to-date
**Section:** `already-up-to-date-is-a-result-not-a-fabrication`
**Issue:** The no-op result is specified as taking no lock and writing no record, but `MergeStart` also returns a `MergeAlreadyUpToDate` outcome, which is only observable after the lock is taken and the record written; the discussion never says which path decides.
**Fix:** State that the both-sides case is decided by a pre-lock `IsAncestor` probe on each side, with `MergeStart`'s outcome used only for the one-side case.

## Verdict

REQUEST_CHANGES
Three concrete defects: mutating guard, wrong config base, editor-invoking conclude command.
MILL_REVIEW_END
