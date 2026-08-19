# Batch: merge state, errors, and path mapping

```yaml
task: 'fabric: merge-conflict primitive'
batch: merge state, errors, and path mapping
number: 2
cards: 4
verify: go test ./internal/fabricengine/ ./cmd/lyx/ ./internal/lyxcwd/ && go test -tags integration -run Merge ./internal/fabricengine/
depends-on: [1]
```

## Batch Scope

Delivers the fabricengine foundation the merge verbs (batches 3–4) stand on, none of it reachable from a public verb yet: the typed error surface with the closed guard-reason set, the on-disk per-pair merge-state record with foreign-state probes, the unified conflict-path mapping, and the gated two-sided abort resets inside `destroy.go`.
The external interface the next batch consumes: `mergeState` load/save/delete + `mergeRecordExists` + `foreignMergeStatePresent`, `newMergeGuardError` + the `mergeReason*` constants + the six typed errors, `resolveMergeGeometry` + `unifyConflictPaths`, and `(*Fabric).resetMergeSides`.
Batch-local decision: unexported helpers throughout — nothing in this batch is exported, so the Fabric Vocabulary Invariant's owner-set carve-out applies and internal warp/weft naming is free.

## Cards

### Card 3: mergeerrors.go — typed errors and the closed reason set

- **Context:**
  - `_mill/discussion.md`
  - `internal/fabricengine/fabric.go`
  - `internal/fabricengine/pull.go`
  - `internal/fabricengine/commit.go`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/mergeerrors.go`
  - `internal/fabricengine/mergeerrors_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create `internal/fabricengine/mergeerrors.go` with the error surface pinned verbatim by `_mill/discussion.md`'s `public-surface-shapes` decision:

  ```go
  type MergeGuardError struct{ Reasons []string }
  type ErrMergeInRequired struct{ Source string }
  type ErrForeignMergeState struct{}
  type ErrNoMergeInProgress struct{}
  type ErrMergeIncomplete struct{}
  type ErrUnmergeableState struct{}
  ```

  plus the sibling-refusal error from the lock decision's consequence:

  ```go
  type ErrMergeInProgress struct{}
  ```

  All messages are fixed, side-free, and never interpolate a branch name, path, or git error (causes go to the internal log at the call site, never into the string).
  Pin these exact `Error()` strings:
  - `MergeGuardError`: `"fabricengine: merge preconditions failed: "` + `strings.Join(e.Reasons, "; ")`.
  - `ErrMergeInRequired`: `"fabricengine: merge produced conflicts and was aborted; run MergeIn in the source branch's worktree first, then retry"` (the offending branch travels only in the `Source` field).
  - `ErrForeignMergeState`: `"fabricengine: git merge state exists that fabric did not start; conclude or abort it with plain git, then retry"`.
  - `ErrNoMergeInProgress`: `"fabricengine: no merge in progress"`.
  - `ErrMergeIncomplete`: `"fabricengine: merge conclude did not finish; run MergeContinue again"`.
  - `ErrUnmergeableState`: `"fabricengine: merge produced conflicts outside the fabric-managed tree; operator intervention required"`.
  - `ErrMergeInProgress`: `"fabricengine: a merge is in progress; run MergeContinue or MergeAbort first"`.

  These strings name Go identifiers (`MergeIn`, `MergeContinue`, `MergeAbort`) while the shipped CLI spells the same operations `lyx fabric merge-in`, `merge --continue`, and `merge --abort`, and batch 6's envelope routes the fixed strings to the operator unchanged.
  Do not remap or reword them here: they are pinned verbatim by the discussion's `public-surface-shapes` decision, and the vocabulary test in batch 5 asserts them byte-exactly.
  The mismatch is closed on the CLI side instead — batch 6's card 16 requires each merge verb's `Long` help to state the identifier-to-verb mapping explicitly.

  Declare the closed guard-reason set as unexported constants, values verbatim from the discussion's `safety-guards-are-aggregated-and-side-free` decision:
  `mergeReasonAlreadyInProgress = "merge already in progress"`, `mergeReasonUnresolvedConflicts = "unresolved conflicts remain"`, `mergeReasonNoMergeInProgress = "no merge in progress"`, `mergeReasonWorktreeDirty = "worktree dirty"`, `mergeReasonNotSynced = "branch not synced to upstream"`, `mergeReasonSourceNotFound = "source branch not found"`, `mergeReasonNotFabricManaged = "source branch is not fabric-managed"`.
  Add a constructor `newMergeGuardError(reasons []string) *MergeGuardError` that sorts and deduplicates before storing, so the reported list never reveals evaluation order or arity;
  its godoc states the closed-set rule: adding a member is a same-commit change to the constant list and to the vocabulary assertion covering it (`mergevocab_test.go`, batch 5), and no member may name a side, carry a path, or imply an order.

  `mergeerrors_test.go` is untagged (pure logic, Tier 1, `package fabricengine`), test names containing `Merge`:
  assert each pinned `Error()` string byte-exactly;
  assert `newMergeGuardError` sorts and deduplicates;
  assert every closed-set constant and every `Error()` output contains no `"warp"`, `"weft"`, or `"host "`-phrase token (case-insensitive).
- **Commit:** `feat(fabricengine): merge error surface and closed guard-reason set`

### Card 4: mergestate.go — the recorded merge

- **Context:**
  - `internal/fabricengine/index.go`
  - `internal/fabricengine/corrindex.go`
  - `internal/state/state.go`
  - `internal/fabricengine/fabric.go`
  - `internal/fabricengine/weftgit.go`
  - `internal/gitrepo/merge.go`
  - `internal/fabricengine/mergeerrors.go`
  - `internal/hubforge/hub.go`
  - `internal/gitkit/gitkit.go`
  - `internal/fabricengine/index_integration_test.go`
- **Edits:**
  - `cmd/lyx/destructiveguard_test.go`
  - `internal/fabricengine/export_test.go`
- **Creates:**
  - `internal/fabricengine/mergestate.go`
  - `internal/fabricengine/mergestate_integration_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create `internal/fabricengine/mergestate.go` with the record type verbatim from the discussion (never exported, never serialized into a result — warp/weft field naming is legal here, `fabricengine` is in the owner set):

  ```go
  type mergeState struct {
      Verb          string    `json:"verb"`
      Source        string    `json:"source"`
      Squash        bool      `json:"squash"`
      Message       string    `json:"message"`
      WarpStart     string    `json:"warp_start"`
      WeftStart     string    `json:"weft_start"`
      WarpOutcome   string    `json:"warp_outcome"`
      WeftOutcome   string    `json:"weft_outcome"`
      WarpCommitted string    `json:"warp_committed"`
      WeftCommitted string    `json:"weft_committed"`
      StartedAt     time.Time `json:"started_at"`
  }
  ```

  Outcome strings: `staged`, `conflicted`, `fast_forwarded`, `up_to_date` — add an unexported mapping from `gitrepo.MergeOutcome`.
  Helpers, all on `*Fabric`:
  - `mergeStatePath() (string, error)` — `filepath.Join(gitDir, "fabric-merge.json")` with `gitDir` from the existing `weftGitDir()` (the correspondence index's placement precedent, `index.go`).
  Reads and writes go through `internal/state`'s locked, atomic-replace helpers under `<path>.lock`, exactly as `corrindex.go` does for the correspondence index in this same weft gitdir — never raw `json.Marshal` + `os.WriteFile`.
  Raw `os.WriteFile` truncates before writing, so a concurrent reader can observe a torn or empty file;
  card 13's sibling-verb guards read this record from other processes with no shared lock (the combined write lock the merge verbs hold does not cover them), and a torn read would hit `loadMergeState`'s corrupt-file clause and surface a spurious hard error instead of a clean guard answer.
  - `loadMergeState() (*mergeState, error)` — `state.ReadJSON[mergeState](path, path+".lock")`, returning `(nil, nil)` on the not-found signal and a decoded record otherwise; a corrupt file is an error, never silently adopted.
  - `saveMergeState(st *mergeState) error` — `state.WriteJSON(path, path+".lock", *st)`.
  - `deleteMergeState() error` — `os.Remove`, tolerating absence. `internal/state` has no delete primitive, so this one call stays raw, matching `index.go`'s own precedent.
  - `mergeRecordExists() (bool, error)` — thin wrapper over `loadMergeState`.
  - `foreignMergeStatePresent() (bool, error)` — git-level merge state on either side: `f.warp.MergeHeadPresent() || len(f.warp.ConflictedFiles()) > 0 || f.weft.MergeHeadPresent() || len(f.weft.ConflictedFiles()) > 0` (evaluate all four, then combine, so no short-circuit ordering leaks into timing-observable behaviour; errors wrap and return).

  Guard-test obligations, same commit:
  - `cmd/lyx/destructiveguard_test.go` `destructiveGuardAllowlist`: add `internal/fabricengine/mergestate.go` with a reason mirroring `index.go`'s entry — the `os.Remove` deletes fabric's own merge-state record inside the weft gitdir, fabric-internal metadata, never operator content.
  Do not add an allowlist entry to `cmd/lyx/uncontainedwrite_test.go` — routing the write through `internal/state.WriteJSON` leaves `mergestate.go` with no `os.WriteFile` call for that guard to see, the same reason `index.go` has no entry there.

  Extend `internal/fabricengine/export_test.go` with re-exports the `fabricengine_test` integration tests need: the state helpers (load/save/delete/exists/foreign) and the record type's fields as needed — follow the file's existing seam style.

  `mergestate_integration_test.go` (`//go:build integration`, `package fabricengine_test`, hubforge fixture, test names containing `Merge`):
  - save → load roundtrip preserves every field; the record lands at `<weft gitdir>/fabric-merge.json` and is invisible to `git status` on both sides.
  - absent record: `loadMergeState` nil, `mergeRecordExists` false.
  - `deleteMergeState` removes it and tolerates a second call.
  - `foreignMergeStatePresent` true after a plain-git conflicted `git merge` staged directly in the warp checkout (drive with `gitkit.MustRun`), false on a clean pair; the foreign state is left untouched by the probe.
- **Commit:** `feat(fabricengine): on-disk merge-state record with foreign-state probes`

### Card 5: mergepaths.go — unified conflict-path mapping

- **Context:**
  - `internal/fabricengine/junctionnames.go`
  - `internal/lyxcwd/lyxcwd.go`
  - `internal/fabricengine/mergeerrors.go`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/mergepaths.go`
  - `internal/fabricengine/mergepaths_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create `internal/fabricengine/mergepaths.go` implementing the `conflicts-are-reported-as-unified-worktree-relative-paths` decision:
  - `resolveMergeGeometry(warpPath string) (anchorRel string, wiredNames []string, err error)` — `lyxcwd.ResolveWorktree(warpPath)` for `AnchorRel`, then `RepoWiredNames(l)` for the wired set.
    Called once per merge call, before any mutation, never cached on the handle (godoc states the `Commit` re-read-per-call precedent and the reconcile-staleness rationale).
    `filepath.Dir(warpPath)` is NOT the config base — `RepoWiredNames` derives it itself.
  - `unifyConflictPaths(warpConflicts, weftConflicts []string, anchorRel string, wiredNames []string) (unified []string, unmappable bool)` — pure function:
    warp paths (already repo-root-relative) pass through unchanged;
    a weft path maps by identity iff it lies under `<anchorRel>/<name>/` for some wired `name` (with `anchorRel == "."` meaning `<name>/` directly);
    any weft path outside that set sets `unmappable` true;
    a unified path produced by both sides (the theoretical collision) also sets `unmappable` true;
    the result is lexically sorted, deduplicate-free by construction, empty-never-nil.
    Path comparison uses forward-slash git-style paths throughout (git's own output form) — normalize `anchorRel` with `path.Join` semantics, no `filepath` OS-dependence.

  `mergepaths_test.go` is untagged (pure logic, Tier 1, `package fabricengine`), table-driven, test names containing `Merge`:
  warp pass-through;
  weft identity mapping at `anchorRel "."` and at a subpath anchor (`backend/_lyx/...`);
  weft path outside the wired set → unmappable;
  weft repo-root file (e.g. a warp-binding record name) → unmappable;
  both-sides collision on one unified path → unmappable;
  merged list lexically sorted with per-side ordering destroyed;
  empty inputs → empty non-nil result.
- **Commit:** `feat(fabricengine): unified worktree-relative conflict-path mapping`

### Card 6: gated two-sided merge-abort resets in destroy.go

- **Context:**
  - `internal/fabricengine/mutation.go`
  - `internal/fabricengine/dirtiness.go`
  - `internal/fabricengine/fabric.go`
  - `internal/fabricengine/mergestate.go`
  - `internal/fabricengine/clone.go`
  - `internal/gitrepo/merge.go`
  - `internal/hubforge/hub.go`
  - `internal/gitkit/gitkit.go`
- **Edits:**
  - `internal/fabricengine/destroy.go`
  - `internal/fabricengine/destroy_test.go`
  - `internal/fabricengine/mergestate_integration_test.go`
  - `internal/fabricengine/export_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Inside `internal/fabricengine/destroy.go` (the only file permitted these primitives, per the Fabric Destruction Chokepoint Invariant), add:

  ```go
  func (f *Fabric) resetMergeSides(rec *Mutations, warpSHA, weftSHA string) error
  ```

  It builds two `pathRequest` values, warp first then weft (fixed order), and runs each through the existing `resetHardTo` executor — no new executor:
  - warp: `what: "reset warp checkout for merge abort"`, `container: filepath.Dir(f.warpPath)`, `target: f.warpPath`, `ownership: ownedWarpCheckout(f.warpPath)`, `dirtiness: dirtyScopeTracked()`, `force: true`.
  - weft: `what: "reset weft checkout for merge abort"`, `container: filepath.Dir(f.weftPath)`, `target: f.weftPath`, `ownership: ownedWeftCheckout(f.weftPath)`, `dirtiness: dirtyScopeTracked()`, `force: true`.

  `force: true` is the one deliberate divergence from `Fabric.ResetHard`'s hardcoded `force: false`: an abort's entire purpose is to discard the intentionally dirty state accumulated since the merge started (unresolved conflict markers are tracked-file modifications), and it is safe only because every caller is record-gated — the godoc must state this rationale.
  The first side's failure aborts the call (return the error, do not attempt the second) — a gate refusal is never discarded, matching `surfaceRefusal`'s rule.
  Both resets record through `resetHardTo`'s existing `KindWorktreeReset` append — never any recording outside `destroy.go`.

  Add the new ownership kind the weft request needs, per the Shared Decision on the weft abort reset's ownership kind — `ownedRegisteredLinkedWorktree` cannot be used, since its own godoc pins it to worktrees other than the main one and `clone.go`'s `cloneRepo(opts.WeftURL, weftPath)` makes the prime pair's weft checkout the weft repo's *main* worktree:
  - a `pathOwnershipWeftCheckout` member appended to the `pathOwnershipKind` constant block;
  - a constructor `ownedWeftCheckout(repoDir string) pathOwnership` with godoc mirroring `ownedWarpCheckout`'s — ANY worktree of the weft repo at `repoDir`, prime included — and stating why the registered-linked kind is wrong here;
  - a `resolvePathOwnership` arm with the refusal message `fmt.Sprintf("%s is not a worktree of the weft repo at %s", target, own.repoDir)`;
  - the membership predicate itself is `isWarpCheckout`'s body, which is already repo-agnostic (`List(repoDir)` membership with no main-entry exclusion): rename it to the side-free `isAnyWorktreeOf(repoDir, target string) bool`, retarget both arms at it, and retarget `export_test.go`'s `IsWarpCheckoutForTest` seam assignment and `destroy_test.go`'s file-header comment reference at the new name (the seam's own exported name stays `IsWarpCheckoutForTest`, so `destructivegaps_integration_test.go`'s `TestOwnership_WarpCheckoutKind` is untouched) — do not duplicate the loop.

  Extend `mergestate_integration_test.go` (and `export_test.go` with a `resetMergeSides` re-export):
  - on a hubforge pair, create divergent commits, drive a real conflicted `MergeStart` on the warp side (conflict markers present, worktree dirty), then `resetMergeSides` to the captured pre-merge SHAs: both HEADs restored exactly, worktrees clean, `MergeHeadPresent` false on both sides, and the mutation record carries exactly two `KindWorktreeReset` entries in warp-then-weft order.
  - the same call succeeds when the weft side is the dirty one.
  - **the prime pair explicitly**: drive the same reset against the hub's prime warp worktree and the weft primary that `hubforge.NewHub` clones (not an `AddPair` linked worktree), asserting the gate admits both sides.
    An `AddPair`-only fixture can never exercise this — the weft primary is the one weft checkout that is a main worktree, and it is the pair `Merge` targets in the real workflow.
- **Commit:** `feat(fabricengine): gated two-sided merge-abort resets`

## Batch Tests

`verify` runs the untagged fabricengine unit suite (`mergeerrors_test.go`, `mergepaths_test.go` plus the existing package regression), the untagged `cmd/lyx` guard suite (destructive-bypass, uncontained-write, and mutation-record guards see the new files and allowlist entries), and the integration tests matching `Merge` (`mergestate_integration_test.go`).
The full fabricengine integration suite is deferred to the repo-wide `done_gate` per the Shared Decision on verify scoping.
