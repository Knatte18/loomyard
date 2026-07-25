# Batch: fabric-weft-git

```yaml
task: 'fabric: unify warp + weft into one git-coordination module'
batch: fabric-weft-git
number: 5
cards: 5
verify: go test -tags integration ./internal/fabricengine
depends-on: [1, 2]
```

## Batch Scope

Implements fabric's weft-git and coordination surface on the `Fabric` handle: the index
git-wiring (`RecordCorrespondence`/`WeftSHAForWarpSHA`/`RebuildIndex`), the parity verbs
(`StatusWeft`/`CommitWeft`/`PushWeft`/`PullWeft` plus package-level `PushWeftAt` for the
detached-push child), and the two genuinely cross-repo operations `SyncWeft` and
`RevertWithWeft` — plus the CONSTRAINTS.md Weft Git Invariant amendment in the same
commit as the first weft-touching code. Runs parallel to batches 3/4 (disjoint files;
depends only on gitrepo growth and fabric-core). External interface for batch 6: all
`Fabric` methods above, `PushWeftAt`, and the typed errors/results. Batch-local
decision: typed sentinel errors (`ErrNoCorrespondence`, `ErrStaleSHA`,
`ErrRevertRollbackFailed`) are package-level `errors.New` values wrapped with context,
matching gitrepo's `ErrInvalidSHA` idiom.

## Cards

### Card 22: index git-wiring on Fabric

- **Context:**
  - `internal/fabricengine/corrindex.go`
  - `internal/fabricengine/trailer.go`
  - `internal/fabricengine/fabric.go`
  - `internal/gitrepo/gitrepo.go`
  - `internal/gitexec/gitexec.go`
  - `internal/state/state.go`
  - `manifest/designs/fabric.md`
  - `_mill/discussion.md`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/index.go`
  - `internal/fabricengine/index_integration_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** `index.go` — the fabric layer that owns everything git the pure
  component must not touch. Unexported plumbing: `weftGitDir()` resolving
  `git rev-parse --git-dir` in the weft worktree via gitexec (absolutized against the
  weft path when relative — in a linked worktree this names the per-worktree gitdir,
  which is exactly the wanted per-pair scope), index path
  `filepath.Join(<gitdir>, "fabric-corrindex.json")`; `warpSeq(sha)` running
  `git rev-list --count --first-parent <sha>` in the warp repo. Exported `Fabric`
  methods per the design doc: `RecordCorrespondence(warpSHA, weftSHA string) error`
  (compute seq, upsert via the corrindex component); `WeftSHAForWarpSHA(warpSHA
  string) (string, error)` — exact lookup; on cache hit whose weft SHA fails
  `f.Weft.SHAExists`, run `RebuildIndex` once and retry (index self-correction per
  the overview decision); if the rebuilt answer still fails `SHAExists` return a
  wrapped `ErrStaleSHA` naming both SHAs; on no entry return wrapped
  `ErrNoCorrespondence`; `RebuildIndex() error` — scan the current weft branch's
  history extracting `Warp-SHA` trailers via git's trailer machinery (the design
  doc's `git interpret-trailers` scan; a single `git log` invocation with a
  trailers-extracting format is the accepted one-pass implementation), rebuild all
  entries (seq per warp SHA; trailer values failing `f.Warp.SHAExists` are recorded
  anyway — staleness surfaces at use, per the stale-SHA decision), and atomically
  replace the index file. Integration-tagged `index_integration_test.go`
  (`lyxtest.CopyWeft` + a plain host repo fixture): gitdir resolution lands inside
  the weft gitdir; record→lookup round-trip; `RebuildIndex` on a branch with
  hand-crafted trailer commits reproduces the recorded entries.
- **Commit:** `feat(fabricengine): correspondence index git wiring on Fabric`

### Card 23: weft-git parity verbs and Weft Git Invariant amendment

- **Context:**
  - `internal/weftengine/weft.go`
  - `internal/weftengine/sync.go`
  - `internal/weftengine/status.go`
  - `internal/boardengine/sync.go`
  - `internal/lock/lock.go`
  - `internal/gitrepo/gitrepo.go`
  - `internal/gitrepo/push.go`
  - `internal/gitrepo/pull.go`
  - `internal/gitexec/gitexec.go`
  - `internal/fabricengine/fabric.go`
  - `internal/fabricengine/trailer.go`
  - `internal/fabricengine/index.go`
  - `_mill/discussion.md`
- **Edits:**
  - `CONSTRAINTS.md`
- **Creates:**
  - `internal/fabricengine/weftgit.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** `weftgit.go`, behavior parity with weftengine plus the trailer
  delta: `func (f *Fabric) StatusWeft(pathspec []string) (map[string]any, error)`
  (same keys as `weftengine.Status`: `weft_worktree`, `branch`, `dirty`, `ahead`,
  `behind`, nil ahead/behind without upstream). `func (f *Fabric)
  CommitWeft(pathspec []string, message string, opts SyncOptions) (sha string,
  committed bool, err error)`: SkipGit gate; acquire the `internal/lock` flock at
  `<weftPath>/.weft/weft.write.lock` (same path as weftengine's, boardengine-style
  lock-around-gitrepo per the overview decision; ensure the `.weft` dir exists);
  read `f.Warp.CurrentSHA()`; commit via `f.Weft.StageAndCommit(appendWarpSHATrailer
  (message, warpSHA), pathspec)` — NEVER `StageAllAndCommit`; when committed, call
  `f.RecordCorrespondence(warpSHA, sha)` (this is the immediate pre-push record the
  CLI detached path relies on; stale entries self-correct at lookup).
  `func (f *Fabric) PushWeft(opts SyncOptions) error`: SkipGit/SkipPush gates, then
  `f.Weft.PushCoalesced()` — gitrepo's `.gitrepo-push.lock` is the push
  serialization; no ported weft push lock. `func (f *Fabric) PullWeft(opts
  SyncOptions) error`: SkipGit gate, then `f.Weft.Pull()`. Package-level
  `func PushWeftAt(weftPath string, opts SyncOptions) error` (the detached-push
  child's entry: gates, `gitrepo.New(weftPath).PushCoalesced()` — no `Fabric`, no
  warp path, mirroring weftcli's bypass push). CONSTRAINTS.md, same commit: amend the
  Weft Git Invariant's module-ownership bullet to "goes through `internal/weftengine`
  **or** `internal/fabricengine`" and "through `internal/warpengine` **or**
  `internal/fabricengine`", with an explicit parallel-build note ("fabric is the
  in-progress unified replacement; this dual ownership lasts until the warp/weft
  cutover task and is then collapsed") — the agent-never-drives-weft-git half applies
  to fabric identically and is left unchanged.
- **Commit:** `feat(fabricengine): weft-git verbs under fabric-layer lock; amend Weft Git Invariant`

### Card 24: SyncWeft and RevertWithWeft

- **Context:**
  - `internal/fabricengine/fabric.go`
  - `internal/fabricengine/weftgit.go`
  - `internal/fabricengine/index.go`
  - `internal/fabricengine/corrindex.go`
  - `internal/gitrepo/gitrepo.go`
  - `internal/gitrepo/push.go`
  - `internal/gitrepo/reset.go`
  - `manifest/designs/fabric.md`
  - `_mill/discussion.md`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/syncweft.go`
  - `internal/fabricengine/revert.go`
  - `internal/fabricengine/revert_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** `syncweft.go`: `type SyncResult struct { Committed bool; Pushed
  bool; WarpSHA string; WeftSHA string }` (JSON tags snake_case);
  `func (f *Fabric) SyncWeft(message string, pathspec []string, opts SyncOptions)
  (SyncResult, error)` — the canonical synchronous coordinated operation:
  `CommitWeft` (trailer + pre-push record inside); if committed and not
  SkipGit/SkipPush: `f.Weft.Push()` in-process, then RE-READ `f.Weft.CurrentSHA()`
  (a rebase-recovered push rewrites local SHAs — gitrepo's documented contract) and
  `RecordCorrespondence(warpSHA, postPushSHA)` so the index holds the post-push SHA.
  `revert.go`: `type RevertResult struct { Exact bool; WarpSHA, WeftSHA, GapFrom,
  GapTo string }`; `func (f *Fabric) RevertWithWeft(warpSHA string) (RevertResult,
  error)`. Ordering per the discussion: resolution FIRST, mutating nothing —
  `f.Warp.SHAExists(warpSHA)` else wrapped `ErrStaleSHA`; exact index hit, else
  nearest-older via `warpSeq(warpSHA)` + the component's `nearestAtOrBefore`
  (resolution logic extracted as a pure helper `classifyCorrespondence(ix *corrIndex,
  targetSeq int, targetSHA string) (revertResolution, error)` so gap classification
  is untagged-testable); resolved weft SHA validated with `f.Weft.SHAExists`
  (stale → one `RebuildIndex` retry → wrapped `ErrStaleSHA`); no at-or-older entry →
  wrapped `ErrNoCorrespondence`. Then mutate: capture pre-revert
  `f.Warp.CurrentSHA()`; `f.Warp.ResetHard(warpSHA)`; `f.Weft.ResetHard(weftSHA)`;
  on weft failure roll warp back via `f.Warp.ResetHard(preRevertSHA)` (Checkout's
  all-or-nothing discipline); if the rollback itself fails return wrapped
  `ErrRevertRollbackFailed` reporting both repos' current SHAs loudly. Gap result:
  `Exact=false` with `GapFrom` = the resolved entry's warp SHA and `GapTo` = the
  requested warp SHA so the caller can flag weft/raddle as stale. Untagged
  `revert_test.go`: `classifyCorrespondence` exact / gap / no-older-error cases
  against a hand-built index — no git.
- **Commit:** `feat(fabricengine): SyncWeft and all-or-nothing RevertWithWeft`

### Card 25: differential weft-git tests

- **Context:**
  - `internal/weftengine/weft.go`
  - `internal/weftengine/sync.go`
  - `internal/weftengine/status.go`
  - `internal/weftengine/sync_test.go`
  - `internal/weftengine/status_test.go`
  - `internal/weftengine/weft_integration_test.go`
  - `internal/lyxtest/lyxtest.go`
  - `internal/fabricengine/fabric.go`
  - `internal/fabricengine/weftgit.go`
  - `internal/fabricengine/trailer.go`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/weftgit_differential_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Integration-tagged, package `fabricengine_test`. Twin
  `lyxtest.CopyWeft` fixtures per case (weftengine on side A with a host-repo stand-in
  for fabric's warp on side B — fabric needs a warp repo for the trailer's
  `CurrentSHA`): `StatusWeft` vs `weftengine.Status` map equality on clean and dirty
  trees; `CommitWeft` vs `weftengine.Commit` — same committed flag, same staged scope
  (stray out-of-pathspec file untracked on both), same commit subject after
  normalizing side B's message by stripping the `Warp-SHA` trailer (via
  `parseWarpSHATrailer`-guided removal), and side B's trailer names side B's warp
  HEAD; already-removed pathspec returns `(false, nil)` on both; `PushWeft` vs
  `weftengine.Push` — commit lands on the bare remote on both, broken-remote error
  contains the repo path and no `fatal:` on both; `PullWeft` vs `weftengine.Pull`
  fast-forward restoration; env-gating parity via `t.Setenv` (`WEFT_SKIP_GIT`,
  `WEFT_SKIP_PUSH`) using `EnvSyncOptions` on both sides.
- **Commit:** `test(fabricengine): differential weft-git equivalence against weftengine`

### Card 26: trailer, index, and staleness integration tests

- **Context:**
  - `internal/gitrepo/push.go`
  - `internal/gitrepo/push_test.go`
  - `internal/lyxtest/lyxtest.go`
  - `internal/fabricengine/fabric.go`
  - `internal/fabricengine/weftgit.go`
  - `internal/fabricengine/syncweft.go`
  - `internal/fabricengine/revert.go`
  - `internal/fabricengine/index.go`
  - `internal/fabricengine/trailer.go`
  - `internal/fabricengine/corrindex.go`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/syncweft_integration_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Integration-tagged, package `fabricengine` (needs the unexported
  index plumbing). Per the discussion's Testing list: (1) `SyncWeft` writes the
  trailer — verify by reading the commit message from git and via
  `git interpret-trailers --parse`; (2) `RebuildIndex` reconstructs an index equal to
  the incrementally-built one (`entries()` equality after several `SyncWeft` rounds);
  (3) synchronous `SyncWeft` records the post-push SHA: advance the bare remote from
  a second clone so the push recovers via rebase (the `push_test.go`
  cross-clone-rebase technique), then assert the recorded weft SHA equals the
  post-push `CurrentSHA` and `SHAExists`, NOT the pre-push SHA; (4) detached-path
  self-correction: record a pre-push SHA, rewrite it (amend), and assert
  `WeftSHAForWarpSHA` heals via `RebuildIndex` to the surviving trailer commit;
  (5) staleness: after a history rewrite that orphans the resolved SHA beyond
  rebuild, `WeftSHAForWarpSHA` and `RevertWithWeft` surface wrapped `ErrStaleSHA`
  (assert with `errors.Is`), and `RevertWithWeft` mutated neither repo (both
  `CurrentSHA`s unchanged); plus `RevertWithWeft` end-to-end: exact-match revert
  resets both repos; gap revert resets to nearest-older and reports the range;
  weft-reset failure rolls warp back to the pre-revert SHA (force the weft
  `ResetHard` to fail while resolution still succeeds by pre-creating the weft
  gitdir's `index.lock` — `SHAExists`/`rev-parse` are unaffected, `reset --hard`
  refuses).
- **Commit:** `test(fabricengine): trailer, index rebuild, post-push SHA, and staleness coverage`

## Batch Tests

`verify: go test -tags integration ./internal/fabricengine` runs the differential
weft-git suite, the index/trailer/staleness integration suite, the untagged
classification tests, and everything from earlier fabricengine batches. Justification
for package-wide scope: batches 3–5 all land in this one package; the union is exactly
the affected test set.
