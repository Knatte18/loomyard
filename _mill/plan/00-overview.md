# Plan: fabric: merge-conflict primitive

```yaml
task: 'fabric: merge-conflict primitive'
slug: fabric-merge-conflict-primitive
approved: true
started: '20260819-084655'
parent: standalone-producers
root: ""
verify: go vet ./...
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches.
Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: gitrepo merge primitives
    file: 01-gitrepo-merge-primitives.md
    depends-on: []
    verify: go test ./cmd/lyx/ ./internal/lyxcwd/ && go test -tags integration ./internal/gitrepo/
  - number: 2
    name: merge state, errors, and path mapping
    file: 02-merge-state-errors-mapping.md
    depends-on: [1]
    verify: go test ./internal/fabricengine/ ./cmd/lyx/ ./internal/lyxcwd/ && go test -tags integration -run Merge ./internal/fabricengine/
  - number: 3
    name: MergeIn and the lifecycle quartet
    file: 03-mergein-and-lifecycle.md
    depends-on: [2]
    verify: go test ./internal/fabricengine/ ./cmd/lyx/ ./internal/lyxcwd/ && go test -tags integration -run Merge ./internal/fabricengine/
  - number: 4
    name: Merge target-pair verb
    file: 04-merge-target-verb.md
    depends-on: [3]
    verify: go test ./internal/fabricengine/ ./cmd/lyx/ ./internal/lyxcwd/ && go test -tags integration -run Merge ./internal/fabricengine/
  - number: 5
    name: sibling-verb guards and vocabulary assertions
    file: 05-sibling-guards-vocabulary.md
    depends-on: [4]
    verify: go test ./internal/fabricengine/ ./cmd/lyx/ ./internal/lyxcwd/ && go test -tags integration -run 'Merge|Commit|Pull|Checkout|Remove|Cleanup' ./internal/fabricengine/
  - number: 6
    name: CLI verbs and docs
    file: 06-cli-and-docs.md
    depends-on: [5]
    verify: go test ./cmd/lyx/ ./internal/fabriccli/ ./internal/lyxcwd/ && go test -tags integration -run Merge ./internal/fabriccli/
```

## Shared Decisions

### Decision: public surface is pinned by discussion.md — copy, never invent

- **Decision:** every exported type, method signature, error type, JSON tag, and the closed guard-reason string set come verbatim from `_mill/discussion.md`'s `public-surface-shapes` and `safety-guards-are-aggregated-and-side-free` decisions.
  Cards restate the signatures they build; where a card and the discussion ever disagree, the discussion wins — **except** where a later Shared Decision in this file explicitly names the discussion clause it supersedes and why.
  Four such supersessions exist today: the Decision on the post-fetch remote-only weft counterpart (which widens `discussion.md`'s `weftBranchExists` pin), the Decision on foreign-state disposition (which picks between two contradictory discussion clauses), the Decision that `Cleanup` needs no merge guard (which supersedes the lock decision's own disposition table), and the Decision on `MergeStart`'s conflict probe (which substitutes one already-built probe for the godoc's spelling).
  An implementer applying this Decision's tie-breaker must check that list first;
  reverting either supersession fails a test this plan requires.
- **Rationale:** the discussion pinned the surface precisely to keep it out of plan-time and implementation-time invention.
  The carve-out exists because a bare "the discussion wins" would silently revert the two places where the discussion is either self-contradictory or contradicted by its own test matrix.
- **Applies to:** all batches

### Decision: MergeHeadPresent resolves via runChecked, not go-git, and joins the pinned list

- **Decision:** `MergeHeadPresent` runs `runChecked("rev-parse", "--verify", "--quiet", "MERGE_HEAD")` and classifies exit code 1 as `false` via the `ancestry.go` `errors.As` idiom, joining `gitrepoPinnedRunBoundMethods`.
- **Rationale:** the discussion's "may resolve via go-git (a ref/file read) and stay off the list" is permissive, not mandatory.
  `MERGE_HEAD` lives in the per-worktree gitdir, and go-git's `storer.Filesystem()` routing for per-worktree files in linked worktrees (which every weft checkout is) is unverified territory, whereas the exact `rev-parse --verify --quiet MERGE_HEAD` probe is already used by `internal/gitrepo/gitrepo_test.go`'s mid-merge fixture and is worktree-correct by construction.
- **Applies to:** gitrepo merge primitives

### Decision: two support primitives beyond the pinned four — MergeFFOnly and ResolveSHA

- **Decision:** `internal/gitrepo` additionally gains `MergeFFOnly(ref string) error` (runChecked `merge --ff-only <ref>`; joins the pinned method list) and `ResolveSHA(ref string) (string, error)` (go-git `ResolveRevision`, stays off the list).
- **Rationale:** the guards decision requires the pre-merge sync to advance via `merge --ff-only`, never `reset --hard`, and the freshness rule requires resolving arbitrary local and remote-tracking refs to SHAs before merging (`merges-name-a-sha-never-a-branch`).
  Both are git-level single-repo operations, which the `git-primitive-in-gitrepo-coordination-in-fabricengine` decision places in `gitrepo`;
  neither exists today (`CurrentSHA` resolves HEAD only).
- **Applies to:** gitrepo merge primitives

### Decision: scenario-symmetric mutation recording rule

- **Decision:** during the merge-attempt phase, record `KindMergeStaged` once per side whose `MergeStart` outcome is not `MergeAlreadyUpToDate`, in fixed warp-then-weft order, with `Target` = that side's checkout path and `Detail` = the resolved source SHA merged into that side — never the outcome name.
  During the conclude phase, record `KindMergeCommitted` per landed conclude-commit, warp first, `Detail` = the new commit SHA.
  Abort resets record through the existing `KindWorktreeReset` inside `resetHardTo`, unconditionally on both sides.
- **Rationale:** the Mutation Record Invariant forbids recording no-ops, and the `mutation-recording-stays-scenario-symmetric` decision requires that two scenarios differing only in which side conflicted produce the same kinds against the same target set in the same order, differing only in SHAs.
  Recording per-side on any observable change (staged, conflicted, fast-forwarded) satisfies both: a conflicted side and a staged side both changed observably, and the outcome never appears in the record.
- **Applies to:** MergeIn and the lifecycle quartet, Merge target-pair verb

### Decision: Cleanup needs no merge guard — its own structure already protects a mid-merge pair

- **Decision:** `Topology.Cleanup` gains no merge-in-progress refusal and `cleanup.go` is not edited.
  Card 14 asserts the existing behaviour (a mid-merge pair survives `Cleanup` in every flag combination) instead.
- **Rationale:** this supersedes `discussion.md`'s `combined-lock-around-mutating-steps-only` disposition table and its Testing line, both of which list `Remove`/`Cleanup` together as verbs that must refuse.
  `Remove` genuinely tears a pair down and does get the refusal;
  `Cleanup` does not — it deletes only *orphaned weft branches*, and a mid-merge pair is by construction live, so `cleanup.go`'s `liveWarpBranches[warpBranch]` arm `continue`s before the branch ever becomes a `CleanupBranchEntry`, with checked-out weft branches unconditionally protected further down.
  `CleanupBranchEntry` also has no field a refusal reason could travel in (`Branch`/`Deleted`/`Protected`/`Error`, the last documented as deletion-failure-only), so the disposition table's own remedy is unimplementable there.
  Adding a guard would be dead code justified only by the discussion's grouping of two verbs that behave differently.
- **Applies to:** sibling-verb guards and vocabulary assertions

### Decision: MergeStart's post-error conflict probe reuses ConflictedFiles

- **Decision:** `MergeStart`'s post-error probe is `ConflictedFiles()` (`git diff --name-only --diff-filter=U`), not a second `git ls-files -u` call.
- **Rationale:** this supersedes the spelling in `discussion.md`'s `public-surface-shapes` godoc ("probes: unmerged index entries (`git ls-files -u`)"), which names the *concept* — unmerged index entries — and happens to spell one of the two commands that answer it.
  Both report exactly the unmerged-stage entries; `ConflictedFiles` is a method this same card ships, so reusing it means one probe, one pinned-list entry, and one behaviour the tests already cover, rather than a second raw spelling that could drift from it.
- **Applies to:** gitrepo merge primitives

### Decision: the weft abort reset needs its own any-worktree ownership kind

- **Decision:** `resetMergeSides`' weft-side `pathRequest` declares a new ownership kind `ownedWeftCheckout(repoDir string)` — "target is ANY worktree of the weft repo at `repoDir`, prime included" — mirroring `ownedWarpCheckout`'s membership test, not `ownedRegisteredLinkedWorktree`.
  It is built in card 6 as a new member of the closed `pathOwnershipKind` set with its own constructor, its own `resolvePathOwnership` arm, its own refusal message, and godoc.
- **Rationale:** `ownedRegisteredLinkedWorktree`'s own godoc pins it to worktrees *other than the main one*, and `clone.go` creates the weft primary with `cloneRepo(opts.WeftURL, weftPath)` — a full clone, i.e. the weft repo's main worktree.
  The prime pair is exactly the pair `Merge` targets when merging a task branch into the trunk, so declaring `ownedRegisteredLinkedWorktree` would make every abort on the prime pair fail the gate.
  A `hubforge.NewHub` + `AddPair` fixture only ever produces *linked* weft worktrees, so a test built on it would pass while the real case stayed broken — which is why this is decided here rather than discovered by test.
  `ownedWarpCheckout` is not reused with a weft path: its refusal message and godoc name the warp repo, and a side-accurate refusal message is what the destruction gate exists to produce.
- **Applies to:** merge state, errors, and path mapping; MergeIn and the lifecycle quartet; Merge target-pair verb

### Decision: a genuine MergeStart error mid-attempt self-aborts, symmetrically

- **Decision:** during the merge-attempt phase of both `MergeIn` and `Merge` — after the state record was written and possibly after the first side already mutated — a `MergeStart` call that returns a genuine error (any error that is not the `MergeConflicted` classification) on either side aborts the whole attempt: `resetMergeSides(rec, warpStart, weftStart)` to the captured pre-merge SHAs, `deleteMergeState()`, and return the wrapped `MergeStart` error.
  The disposition is identical whichever side failed, and the returned error value differs between the two sides only in the wrapped git cause, never in shape or in which side it names — the cause goes to the internal log via `logger.Warn`, not into a user-facing message.
  If the reset itself fails, the record is deliberately retained and that reset error is returned instead: the pair is then in a state only `MergeAbort` can clear, and deleting the record would strand it.
- **Rationale:** card 1's `MergeStart` classification admits a genuine non-conflict error (a corrupt index, a missing object, a killed git), and neither the conflict path nor the clean path covers it.
  Leaving it unspecified would let an implementer retain a half-mutated pair with a live record, or delete the record over a mutated pair — both unrecoverable through the public surface.
  Self-abort matches the `unmappable` path's disposition (`Decision: unified conflict-path mapping`, card 8 step 9), so the two "something went wrong mid-attempt" paths behave alike.
- **Applies to:** MergeIn and the lifecycle quartet, Merge target-pair verb

### Decision: conflict envelope never goes through errWithRecordFields

- **Decision:** the CLI maps a non-empty `Conflicts` result to a failure envelope via a new dedicated helper `errConflictsWithRecord(w, rec, conflicts)` in `internal/fabriccli/envelope.go`, which sets `mutations` from the record, `partial` to the literal `false`, a `conflicts` array field, and a fixed error message — never via `errWithRecordFields`, whose `partial = rec.Len() > 0` computation would wrongly report `true`.
- **Rationale:** the Mutation Record Invariant derives `partial` from exactly one rule — `error ≠ nil ∧ record non-empty` — and a reported conflict returns a nil engine error, so `partial` stays `false` even on the failure envelope.
  The discussion states this is exactly the place an implementer would guess the other way.
- **Applies to:** CLI verbs and docs

### Decision: fabric-managed guard accepts a post-fetch remote-only weft counterpart

- **Decision:** the "source branch is not fabric-managed" guard passes when `<source>-weft` exists as a local weft branch (the existing `weftBranchExists` probe) **or**, after the best-effort fetch, resolves as `origin/<source>-weft` in the weft repo (via `ResolveSHA`).
  The freshness rule then independently picks which ref each side actually merges.
- **Rationale:** the discussion pins `weftBranchExists` for the local probe but its own test matrix requires "source existing only remotely → merged";
  a local-only probe would fail that scenario.
  Accepting the post-fetch remote-tracking ref reconciles the two clauses without weakening the refusal for genuinely foreign branches, which exist on neither.
- **Applies to:** MergeIn and the lifecycle quartet, Merge target-pair verb

### Decision: foreign-state disposition — the four mutating verbs refuse, MergeInProgress reports false

- **Decision:** when git-level merge state exists that fabric did not record (`foreignMergeStatePresent` true, no record on disk), all four *mutating* merge verbs — `MergeIn`, `Merge`, `MergeContinue`, `MergeAbort` — refuse with `*ErrForeignMergeState`, mutating nothing and leaving the foreign state untouched.
  `MergeInProgress` is not one of the four: it reports `false` and never errors on foreign state, since it answers "does fabric have a merge in progress", which is exactly false here.
  `*ErrNoMergeInProgress` is reserved for the case where there is neither a record nor foreign git merge state.
- **Rationale:** `discussion.md`'s `lifecycle-quartet-on-both-verbs` decision says `MergeContinue`/`MergeAbort` refuse with `*ErrNoMergeInProgress` when no record exists, while its Testing section says "`MergeInProgress` false, and all four verbs refuse with `*ErrForeignMergeState`" — a direct contradiction the plan may not leave to the implementer.
  This pinning satisfies both readings: "all four verbs" is exactly the four mutating verbs (the fifth, `MergeInProgress`, is given its own disposition in that same sentence), `*ErrNoMergeInProgress` keeps its stated meaning for the genuinely-nothing-in-progress case, and "never touch foreign git merge state" is honoured — refusing is not touching.
  A distinct typed error is also the more useful one: it tells the operator that plain-git state exists and must be concluded or aborted with plain git, which `*ErrNoMergeInProgress` would actively mislead about.
  This supersedes the `lifecycle-quartet-on-both-verbs` clause read in isolation, per the carve-out in the first Shared Decision.
- **Applies to:** MergeIn and the lifecycle quartet, Merge target-pair verb

### Decision: new-file layout in fabricengine

- **Decision:** the merge surface splits across six production files:
  `mergeerrors.go` (error types + the closed reason set), `mergestate.go` (the on-disk record + foreign-state probes), `mergepaths.go` (unified conflict-path mapping), `mergeguards.go` (guard evaluation, source resolution, freshness/sync helpers), `merge.go` (`MergeOptions`/`MergeResult`/`MergeIn`/`Merge`), `mergelifecycle.go` (`MergeContinue`/`MergeAbort`/`MergeInProgress` + the shared conclude phase).
  The abort/self-abort resets live in `destroy.go` per the Destruction Chokepoint Invariant.
- **Rationale:** keeps each review unit small and gives the guard tests stable file names to pin (`destructiveGuardMutatingResultTypes` needs `MergeResult`'s file).
- **Applies to:** all batches

### Decision: docs land inside the task, split across the batches that motivate them

- **Decision:** `internal/gitrepo/doc.go` and the two `cmd/lyx` pinned-list tests change in the same card/commit as the code that makes them stale (batch 1);
  `internal/fabricengine/doc.go`, `manifest/designs/finalize.md`, `manifest/roadmap.md`, and `docs/overview.md` land in batch 6, before the task completes.
  New `Kind` members, guard-test entries, and allowlist entries always land in the same commit as their code, per each invariant's same-commit rule.
- **Rationale:** the task branch squash-merges onto `standalone-producers`, so the Documentation Lifecycle's same-commit discipline is satisfied at the parent level;
  within the task, invariant-enforced same-commit rules (pinned lists, Kind members) are honoured per card.
- **Applies to:** all batches

### Decision: one typed sibling-refusal error

- **Decision:** `ErrMergeInProgress` (fixed, side-free message) is the single typed error every sibling mutating verb returns while a merge record exists — `Commit` additionally returns it for foreign git merge state with no record.
  The merge verbs themselves refuse foreign state with `*ErrForeignMergeState`, and a recorded in-progress merge with the guard reason `"merge already in progress"`.
- **Rationale:** the lock decision's consequence table demands one typed, side-free error for every sibling refusal;
  the merge verbs' own refusals are pinned separately by `a-recorded-merge-not-a-derived-one` and the guards decision.
- **Applies to:** merge state, errors, and path mapping; sibling-verb guards and vocabulary assertions

### Decision: verify scoping leans on -run Merge plus the repo-wide done gate

- **Decision:** fabricengine batches verify with the untagged package suites (fast), the `cmd/lyx` guard suite, and `-tags integration -run Merge` (or the widened pattern for batch 5).
  Full-suite regressions are caught by the configured `pipeline.done_gate` (`go test ./... && go test -tags integration ./...`) before the task is marked done.
- **Rationale:** the full fabricengine integration suite is minutes-long and runs many times per batch through fixer rounds;
  the merge-scoped pattern covers every test this plan writes (all test names contain `Merge`), and the done gate closes the cross-package gap.
- **Applies to:** all batches

## All Files Touched

- `CONSTRAINTS.md`
- `cmd/lyx/destructiveguard_test.go`
- `cmd/lyx/gitrepoboundary_test.go`
- `cmd/lyx/helptree_test.go`
- `docs/overview.md`
- `internal/fabriccli/argsarity_test.go`
- `internal/fabriccli/cli_test.go`
- `internal/fabriccli/envelope.go`
- `internal/fabriccli/envelope_test.go`
- `internal/fabriccli/envelopecontract_integration_test.go`
- `internal/fabriccli/merge_cli_integration_test.go`
- `internal/fabriccli/merge_verbs.go`
- `internal/fabriccli/weft_verbs.go`
- `internal/fabricengine/checkout.go`
- `internal/fabricengine/commit.go`
- `internal/fabricengine/destroy.go`
- `internal/fabricengine/destroy_test.go`
- `internal/fabricengine/doc.go`
- `internal/fabricengine/export_test.go`
- `internal/fabricengine/livestate_mutationoracle_selftest_test.go`
- `internal/fabricengine/livestate_mutationoracle_test.go`
- `internal/fabricengine/merge.go`
- `internal/fabricengine/merge_target_integration_test.go`
- `internal/fabricengine/mergeerrors.go`
- `internal/fabricengine/mergeerrors_test.go`
- `internal/fabricengine/mergeguards.go`
- `internal/fabricengine/mergein_integration_test.go`
- `internal/fabricengine/mergein_recovery_integration_test.go`
- `internal/fabricengine/mergelifecycle.go`
- `internal/fabricengine/mergepaths.go`
- `internal/fabricengine/mergepaths_test.go`
- `internal/fabricengine/mergesiblings_integration_test.go`
- `internal/fabricengine/mergestate.go`
- `internal/fabricengine/mergestate_integration_test.go`
- `internal/fabricengine/mergevocab_test.go`
- `internal/fabricengine/mutation.go`
- `internal/fabricengine/mutation_test.go`
- `internal/fabricengine/pull.go`
- `internal/fabricengine/remove.go`
- `internal/gitrepo/doc.go`
- `internal/gitrepo/merge.go`
- `internal/gitrepo/merge_integration_test.go`
- `manifest/designs/finalize.md`
- `manifest/roadmap.md`
