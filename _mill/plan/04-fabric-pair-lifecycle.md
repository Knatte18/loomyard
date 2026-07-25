# Batch: fabric-pair-lifecycle

```yaml
task: 'fabric: unify warp + weft into one git-coordination module'
batch: fabric-pair-lifecycle
number: 4
cards: 9
verify: go test -tags integration ./internal/fabricengine
depends-on: [3]
```

## Batch Scope

Implements fabric's complete topology surface — the `Topology` holder and every verb
warp's `Worktree` has (Add, Remove, Checkout, Reconcile, Status, Prune, Cleanup, List)
plus the consumer-facing preflight helpers `PairInSync` and `HostClean` — and proves it
with the differential back-to-back lifecycle tests that are this task's central
validation. Every weft branch is derived via `WeftBranchName`; result types mirror
warp's exactly (same field names and JSON tags) so the differential comparisons and the
future cutover are mechanical. External interface for batch 6: `Topology`/`NewTopology`
and all methods, `PairInSync`, `HostClean`. Batch-local decision: differential tests
compare structured end state read from disk/git (worktree lists, branches, links,
result structs), never raw command transcripts.

## Cards

### Card 13: Topology holder and transactional Add

- **Context:**
  - `internal/warpengine/worktreelifecycle.go`
  - `internal/warpengine/add.go`
  - `internal/warpengine/add_test.go`
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/fabricengine/branchname.go`
  - `internal/fabricengine/weftwiring.go`
  - `internal/fabricengine/portals.go`
  - `internal/fabricengine/launchers.go`
  - `internal/gitexec/gitexec.go`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/topology.go`
  - `internal/fabricengine/add.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** `topology.go`: `type Topology struct` holding an unexported
  `cfg Config`; `func NewTopology(cfg Config) *Topology`; godoc explaining the split
  from `Fabric` (hub-scoped verbs vs per-pair coordination — the overview's
  vocabulary decision). `add.go`: adapted copy of warp's `Add` with `AddOptions{SkipGit,
  SkipPush bool}`, `AddResult{Slug, Branch, Path string; Pushed bool}` (same JSON tags
  as warp's), `addOptionsFromEnv`, and `rollbackAdd`. Flow parity with warp's
  transactional sequence (clean-check → host branch+worktree → weft worktree
  create-or-adopt → portal → launchers → push both, best-effort rollback preserving
  the original error) with the branch delta: host branch is `cfg.BranchPrefix + slug`
  exactly as warp derives it (`add.go` line-89 pattern), weft branch is
  `WeftBranchName(hostBranch)`, adoption matches an existing suffixed weft branch.
- **Commit:** `feat(fabricengine): Topology holder and transactional Add`

### Card 14: Remove and List

- **Context:**
  - `internal/warpengine/remove.go`
  - `internal/warpengine/remove_test.go`
  - `internal/warpengine/list.go`
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/hubgeometry/worktreelist.go`
  - `internal/fabricengine/branchname.go`
  - `internal/fabricengine/weftwiring.go`
  - `internal/fabricengine/portals.go`
  - `internal/fabricengine/launchers.go`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/remove.go`
  - `internal/fabricengine/list.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Adapted copies: `Remove(l, slug, force) (RemoveResult, error)` with
  warp's early portal/launcher teardown ordering and `RemoveResult{Slug, Path string;
  LinksRemoved int}`; the weft branch it removes is `WeftBranchName(cfg.BranchPrefix +
  slug)`. `List(sourceDir) ([]WorktreeEntry, error)` with `type WorktreeEntry =
  hubgeometry.WorktreeEntry`, thin wrapper over `hubgeometry` listing exactly like
  warp's.
- **Commit:** `feat(fabricengine): Remove and List`

### Card 15: all-or-nothing Checkout

- **Context:**
  - `internal/warpengine/checkout.go`
  - `internal/warpengine/checkout_test.go`
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/fabricengine/branchname.go`
  - `internal/fabricengine/weftwiring.go`
  - `internal/fabricengine/junction.go`
  - `internal/gitexec/gitexec.go`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/checkout.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Adapted copy of `Checkout(l, branch) (CheckoutResult, error)`
  preserving warp's discipline: dirty-weft refusal, host switch, weft switch with
  `rollbackHostSwitch` on weft failure, junction re-pointing, `CheckoutResult{Branch,
  WeftWorktree string}`. Branch deltas: the weft target is `WeftBranchName(branch)`;
  when that weft branch is missing, `switchOrForkWeft`'s fork start point is the weft
  branch corresponding to the branch the weft was on before the switch (i.e. the
  suffixed sibling of the previous host branch), mirroring warp's fork-from-parent
  logic one suffix over. Unmanaged-branch handling matches warp's.
- **Commit:** `feat(fabricengine): coordinated all-or-nothing Checkout`

### Card 16: Reconcile

- **Context:**
  - `internal/warpengine/reconcile.go`
  - `internal/warpengine/reconcile_test.go`
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/fabricengine/branchname.go`
  - `internal/fabricengine/weftwiring.go`
  - `internal/fabricengine/junction.go`
  - `internal/fabricengine/hostlayout.go`
  - `internal/gitexec/gitexec.go`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/reconcile.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Adapted copy of `Reconcile(l) (ReconcileResult, error)` with the
  full action vocabulary (`ReconcileAction` consts `weft_recreated`,
  `junction_repointed`, `raw_adopted`, `unmanaged_reported`, `already_healthy`) and
  `ReconcilePairResult`/`ReconcileResult` mirroring warp's fields. Delta: where warp
  mirrors the host branch name onto the weft side (`reconcileMissingWeft`,
  `adoptWeftWorktree`, `createDormantWeftForRawHost`), fabric uses
  `WeftBranchName(hostBranch)`. Inner git failures land in the result `Error` fields,
  never leak raw stderr (warp's `_NoStderrLeak` posture).
- **Commit:** `feat(fabricengine): repair-and-adopt Reconcile`

### Card 17: Status (pairs) with drift and pollution

- **Context:**
  - `internal/warpengine/status.go`
  - `internal/warpengine/status_test.go`
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/fabricengine/branchname.go`
  - `internal/fabricengine/hostlayout.go`
  - `internal/gitexec/gitexec.go`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/status.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Adapted copy of `Status(l) (StatusResult, error)` with
  `PairStatus`/`StatusResult`/`PollutionEntry` mirroring warp's fields (JSON tags
  included). Delta: a pair is `InSync` when `weftBranch == WeftBranchName(hostBranch)`
  (warp requires equal names); `DriftReason` texts state the expected suffixed branch.
  Junction health and `_lyx` pollution detection copied unchanged.
- **Commit:** `feat(fabricengine): pair Status with suffix-aware drift`

### Card 18: Prune and Cleanup

- **Context:**
  - `internal/warpengine/prune.go`
  - `internal/warpengine/prune_test.go`
  - `internal/warpengine/cleanup.go`
  - `internal/warpengine/cleanup_test.go`
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/fabricengine/branchname.go`
  - `internal/fabricengine/weftwiring.go`
  - `internal/gitexec/gitexec.go`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/prune.go`
  - `internal/fabricengine/cleanup.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Adapted copies of `Prune(l, apply) (PruneResult, error)` (stale
  weft dirs whose host worktree is gone; `PruneEntry` fields mirrored; live pairs never
  touched) and `Cleanup(l, apply, force) (CleanupResult, error)` (weft branches without
  a host sibling; dry-run/apply/force matrix; raddle-fold-back gate copied). Deltas: a
  weft branch's host sibling is recovered via `hubgeometry.WeftHostSlug(branch)` (a
  non-suffixed weft branch is by definition not fabric-managed — report, never delete,
  matching warp's unmanaged posture) followed by warp's `strings.TrimPrefix(...,
  cfg.BranchPrefix)` slug inverse; the protected-branch set is warp's protected set
  mapped through `WeftBranchName` (e.g. `main-weft` protected).
- **Commit:** `feat(fabricengine): Prune and Cleanup with suffix inverse`

### Card 19: PairInSync and HostClean preflight helpers

- **Context:**
  - `internal/warpengine/drift.go`
  - `internal/warpengine/drift_test.go`
  - `internal/warpengine/hostclean.go`
  - `internal/warpengine/hostclean_test.go`
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/fabricengine/branchname.go`
  - `internal/gitexec/gitexec.go`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/drift.go`
  - `internal/fabricengine/hostclean.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Adapted copies of the package-level loom-preflight helpers:
  `PairInSync(l) (ok bool, reason string, err error)` (branch correspondence uses
  `WeftBranchName`; broken-junction detection copied) and `HostClean(l) (clean bool,
  reason string, err error)` (strict `git status --porcelain`, no untracked-files
  flag — byte-identical behavior to warp's). These are dead-until-cutover exports by
  design (discussion's "Complete exported surface" decision); the differential tests
  in card 21 are their consumers now.
- **Commit:** `feat(fabricengine): PairInSync and HostClean preflight helpers`

### Card 20: differential lifecycle tests — add, remove, checkout, list

- **Context:**
  - `internal/warpengine/worktreelifecycle.go`
  - `internal/warpengine/add.go`
  - `internal/warpengine/add_test.go`
  - `internal/warpengine/remove_test.go`
  - `internal/warpengine/checkout_test.go`
  - `internal/warpengine/weftwiring_test.go`
  - `internal/lyxtest/lyxtest.go`
  - `internal/lyxtest/hermetic.go`
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/fabricengine/topology.go`
  - `internal/fabricengine/add.go`
  - `internal/fabricengine/remove.go`
  - `internal/fabricengine/checkout.go`
  - `internal/fabricengine/list.go`
  - `internal/fabricengine/branchname.go`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/lifecycle_differential_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Integration-tagged, package `fabricengine_test`. Shared
  differential harness for the batch: copy the same lyxtest fixture twice
  (`lyxtest.CopyPairedLocal` per side; seed each with its module's config via
  `lyxtest.SeedConfig` using the same `branch_prefix`), run the warp operation
  (`warpengine.New(cfg).Add/...`) on side A and the fabric equivalent
  (`fabricengine.NewTopology(cfg).Add/...`) on side B, then assert equivalent end
  state via a normalizing comparator: worktree lists match, host branches identical,
  weft branch on side B equals `WeftBranchName` of side A's, junction/portal targets
  correspond, launcher sets correspond modulo `warp-checkout`↔`fabric-checkout`.
  Cover per the discussion's list: add (fresh + adopt-existing-weft-branch), rollback
  on injected weft failure leaves both sides clean, remove (incl. links-removed
  count), checkout to an existing pair branch, checkout that must fork a missing weft
  branch (fork-point isolation mirrored from `weftwiring_test.go`'s subtask case),
  list output equality. The harness helpers (fixture-pair builder, comparator) are
  written for reuse by card 21.
- **Commit:** `test(fabricengine): differential add/remove/checkout/list equivalence`

### Card 21: differential tests — reconcile, pairs, prune, cleanup, preflight

- **Context:**
  - `internal/warpengine/worktreelifecycle.go`
  - `internal/warpengine/reconcile_test.go`
  - `internal/warpengine/status_test.go`
  - `internal/warpengine/prune_test.go`
  - `internal/warpengine/cleanup_test.go`
  - `internal/warpengine/drift_test.go`
  - `internal/warpengine/hostclean_test.go`
  - `internal/lyxtest/lyxtest.go`
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/fabricengine/reconcile.go`
  - `internal/fabricengine/status.go`
  - `internal/fabricengine/prune.go`
  - `internal/fabricengine/cleanup.go`
  - `internal/fabricengine/drift.go`
  - `internal/fabricengine/hostclean.go`
  - `internal/fabricengine/lifecycle_differential_test.go`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/reconcile_differential_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Integration-tagged, package `fabricengine_test`, reusing card 20's
  harness. Differential coverage: reconcile on a healthy pair (both report
  already-healthy, mutate nothing), reconcile recreating a deleted weft worktree
  (actions correspond; fabric recreates the suffixed branch), reconcile re-pointing a
  broken junction, status/pairs on in-sync and deliberately-drifted pairs (fabric's
  `InSync` true exactly when warp's is, given corresponding fixtures), prune of a stale
  weft dir (live pair untouched on both sides), cleanup dry-run/apply/force matrix
  (protected branches skipped on both — `main` vs `main-weft`; entries correspond),
  and `PairInSync`/`HostClean` equivalence on clean, drifted, and dirty fixtures.
- **Commit:** `test(fabricengine): differential reconcile/pairs/prune/cleanup equivalence`

## Batch Tests

`verify: go test -tags integration ./internal/fabricengine` runs the two differential
suites plus all earlier fabricengine tests. The differential files ARE this batch's
test story — every verb is asserted equivalent to its warp reference on twin fixtures,
which is the discussion's designated validation for the parallel build; separate
unit-test copies of warp's per-verb tests are deliberately omitted (the references are
listed as Context on each card instead).
