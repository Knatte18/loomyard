# Batch: docs

```yaml
task: 'fabric: Fabric.Commit classify+dispatch + unified diff/status'
batch: docs
number: 5
cards: 1
verify: go build ./internal/fabricengine/
depends-on: [1, 2, 3, 4]
```

## Batch Scope

This batch lands the documentation the task owes, describing the full landed behavior of `Fabric.Commit`/`Diff`/`Status`, so it depends on all four code batches. It is the exact doc set `_mill/discussion.md`'s Scope names — `internal/fabricengine/doc.go`, the `fabric-unified-view.md` slice-2 line (mark DONE), and the `CONSTRAINTS.md` Weft Git Invariant forward-reference — and no more (this slice adds no CLI verb and no new module, so `docs/overview.md`'s module table is unchanged). The project's "docs land in the same commit" rule is satisfied at task granularity: every batch of this task lands on one branch merged atomically, and a dedicated final docs card is the only way to describe `doc.go`'s cross-batch surface (`Commit` from batch 3, `Diff`/`Status` from batch 4) coherently in one place.

## Cards

### Card 14: Document Fabric.Commit/Diff/Status; mark slice 2 done; discharge the Weft Git Invariant forward-reference

- **Context:**
  - `internal/fabricengine/commit.go`
  - `internal/fabricengine/diff.go`
- **Edits:**
  - `internal/fabricengine/doc.go`
  - `manifest/designs/fabric-unified-view.md`
  - `CONSTRAINTS.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/fabricengine/doc.go`'s package comment, add paragraphs (markdown-in-godoc, one line per paragraph, matching the file's existing style) describing: `Fabric.Commit` — classify-and-dispatch, warp-first ordering, the plain-git warp property, the report-not-rollback partial-failure story with its three weft outcomes (`CommitResult`/`*PartialCommitError`), the detached both-sides async push, and that `opts.SkipGit` is **weft-scoped** for `Fabric.Commit` (it gates only the weft commit; the warp commit and the env-var-gated async push proceed regardless — a deliberate narrowing of `SyncOptions.SkipGit`'s general "skip all git operations if true" contract for this one entry point); the Go-internal `Fabric.Diff` (nearest-older weft-anchor bridge, degrades to an empty weft side with `NoWeftCorrespondence` rather than erroring) and `Fabric.Status` (its new go-git worktree-status primitive); one sentence distinguishing the three status surfaces — `Topology.Status` (paired topology), `StatusWeft` (dirty/ahead/behind), and the new unified `Fabric.Status` (merged uncommitted worktree changes); and one sentence recording the known asymmetry that `Fabric.Status` may surface the `.gitrepo-push.lock` operational artifact (gitrepo's single-pusher lock, left in a repo's worktree root by `PushCoalesced` because `lock.FileLock.Release` unlocks without deleting the file) on the **warp/host** side but not the weft side — fabric seeds a `.git/info/exclude` entry for it only on the weft it owns and deliberately does not manage the host repo's git config, so a host push's lingering lock is reported like any other untracked host file. In `manifest/designs/fabric-unified-view.md`, rewrite build-order item 2 in slice-1's DONE format (prefix `2. **DONE — Fabric.Commit (classify+dispatch) + unified Fabric.Diff/Status**` and note it landed as this task), and mark the two now-answered "Open questions" entries resolved in place: the "Partial-failure semantics for a two-sided Fabric.Commit" question (resolved — warp-first, report-not-rollback, three-outcome result/error) and the "Whether Fabric.Diff is a CLI verb or Go-internal only" question (resolved — Go-internal only). Do **not** delete `fabric-unified-view.md` (it is deleted only when the whole slices-1-through-5 campaign lands). In `CONSTRAINTS.md`'s Weft Git Invariant, discharge the forward-reference sentence ("The general 'who may time a weft commit' question gets a fuller treatment once `fabric-unified-view.md`'s `Fabric.Commit` work lands (sequenced after this task).") by rewriting it to state that `Fabric.Commit` has landed as a Go API called by the orchestration layer, that an LLM agent never invokes it (deliberate policy, not a code guard — the old accidental `git add`-fails guardrail is intentionally not reintroduced), and that the weft side of `Fabric.Commit` satisfies module ownership by routing through `commitWeftLocked`/`CommitWeft`.
- **Commit:** `docs(fabric): document Fabric.Commit/Diff/Status and mark slice 2 done`

## Batch Tests

`verify: go build ./internal/fabricengine/` confirms the edited `doc.go` package comment still compiles; the other two edits are pure markdown (`manifest/designs/fabric-unified-view.md`, `CONSTRAINTS.md`) with no runnable surface. The task's module-wide `done_gate` (`go test ./...`, run from git_root by mill-go before marking the task done) is the final regression gate across every package this task touched.
