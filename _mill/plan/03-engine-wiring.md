# Batch: engine-wiring

```yaml
task: 'fabric: no remote/GitHub branch deletion'
batch: 'engine-wiring'
number: 3
cards: 6
verify: go vet ./... && go vet -tags integration ./... && go vet -tags smoke ./internal/loomcli/... ./cmd/lyx/... && go test ./internal/fabricengine/... && go test -tags integration ./internal/fabricengine/...
depends-on: [2]
```

## Prior failure

- Round 1: finalize's verify replay failed with `stuck_type: verify`, reason: `vet: internal/burlerengine/smoke_cluster_test.go:140:122: not enough arguments in call to burlerengine.New (have (*shuttleengine.Runner, burlerengine.Geometry, burlerengine.Config, string), want (burlerengine.Shuttle, burlerengine.Geometry, burlerengine.Config, string, string))`.
  Confirmed pre-existing on `main` (same call site, same error, verified directly against the `main` worktree) and untouched by any commit in this task — a plan defect in this batch's `verify:` scope, not a card defect.
  Card 11's own text already narrows the real call site under the `smoke` tag to `internal/loomcli/smoke_test.go`;
  the whole-module `./...` scope for the `-tags smoke` vet pass was overbroad and tripped on this unrelated, already-broken package.
  Fixed by scoping the `-tags smoke` vet pass to `internal/loomcli/...` and `cmd/lyx/...` — the two packages holding `smoke`-tagged files reachable from this task's surface — instead of `./...`.

## Batch Scope

This batch wires the gated executor into the two engine verbs the roadmap names, widens both verb signatures with an explicit `remote` parameter, adds the result fields that report the remote outcome, adds the once-per-verb no-`origin` pre-check, amends `doc.go`'s per-item-failure carve-out, restores compilation across the module, and covers the whole behaviour surface with integration tests over real hubs.
It is one batch because the two verbs share the executor, the no-`origin` pre-check shape, and the result-field naming, and because widening two exported signatures breaks callers in fifteen files that must all move together or nothing compiles.

External interface batch 4 consumes: `Cleanup(l, apply, force, remote bool)`, `Remove(l, slug string, force, remote bool)`, the four new `CleanupBranchEntry`/`RemoveResult` fields, and the verb-level `RemoteSkippedReason` on both result types.

Batch-local decisions beyond the overview's Shared Decisions:

- `removeWeftWorktree` grows a second return value, `(weftTeardownResult, error)`, rather than an out-parameter.
  The function already returns a value-typed error, a second return is the ordinary Go shape, and an out-parameter would let a caller pass nil and silently drop the outcome.
- The existing `error` return of `removeWeftWorktree` keeps its exact current meaning: the `firstErr` accumulator over worktree removal, local branch deletion, and prune.
  The remote outcome rides the struct and never the error.
- Card 10's edit to `internal/fabriccli/fabric.go` is compile-only, per the overview's every-batch-compiles-and-passes decision.

## Cards

### Card 7: Cleanup — result fields, remote parameter, no-origin pre-check, remote deletion

- **Context:**
  - `internal/fabricengine/destroy.go`
  - `internal/fabricengine/mutation.go`
  - `internal/fabricengine/worktreelist.go`
  - `internal/gitrepo/remote.go`
  - `internal/gitrepo/push.go`
  - `internal/gitrepo/gitrepo.go`
  - `internal/fabricengine/doc.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/fabricengine/cleanup.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Five changes to this file.

  First, the result types.
  `CleanupBranchEntry` gains `RemoteDeleted bool` with the tag `json:"remote_deleted,omitempty"` and `RemoteError string` with the tag `json:"remote_error,omitempty"`, each with a doc comment.
  `RemoteDeleted` is true only when the branch's copy on the remote was observably removed;
  `RemoteError` is non-empty when the remote deletion was attempted and did not succeed, and its text always names the layer that said no — see the third change below.
  `CleanupResult` gains a verb-level `RemoteSkippedReason string` with the tag `json:"remote_skipped_reason,omitempty"`, declared beside `Entries` and never on `CleanupBranchEntry`, with a doc comment recording that it carries a once-per-verb reason no remote deletion was attempted at all — today only a weft repo with no `origin` configured — and that it is deliberately not `RemoteError`, because `RemoteError` is the field the CLI switches its exit code on and a missing `origin` must exit 0.

  Second, rewrite `CleanupBranchEntry.Error`'s own field doc.
  It currently reads `Error is non-empty when apply is true and branch deletion failed.`
  It must now also record that the field drives the CLI's exit code: a non-empty `Error` makes `lyx fabric cleanup --apply` exit non-zero, because it names a genuine failure of the local `git branch -D` rather than a designed refusal — `Protected` and unmanaged entries set no `Error` at all.

  Third, the verb.
  `Cleanup`'s signature becomes `func (t *Topology) Cleanup(l *lyxcwd.Location, apply, force, remote bool) (res CleanupResult, err error)`.
  Before the per-branch loop, and only when `remote` is true, resolve the remote once: call `RemoteURL` on `gitrepo.New(weftRepoRoot)` for `originRemoteName`, where `weftRepoRoot` comes from `WeftRepoRoot(l)`.
  On an error, set `result.RemoteSkippedReason` to a message naming that no remote deletion was attempted because the weft repo has no `origin` remote configured, include the underlying error, and attempt no remote deletion for any entry in this call — every entry's `RemoteError` stays empty.
  This pre-check must run under `remote` true in a dry run too, and populate `RemoteSkippedReason` there: `RemoteURL` is a go-git local config read that spawns no process and contacts no network, so the dry run stays offline-safe, and telling the operator up front that `--apply --remote` would do nothing remotely is the whole value of reporting it.
  With `remote` false the pre-check does not run at all and `RemoteSkippedReason` stays empty — without that guard, a plain `cleanup --apply` against a remoteless repo would start reporting a non-empty reason, an observable change to a path this task does not otherwise touch.
  This pre-check gates only the remote deletion;
  it is not a general reachability probe, and a configured-but-unreachable remote still fails per-branch through the ordinary path.

  Fourth, the remote deletion itself, strictly inside the final `apply` arm and only after `deleteWeftBranch` returned true.
  Gate it on `remote` being true and on the no-`origin` pre-check having succeeded.
  Perform it through a new unexported helper in this file — name it `deleteWeftBranchOnRemote` — taking the recorder, the `*lyxcwd.Location`, the branch, the branch prefix, the resolved weft repo root, and the entry pointer, mirroring `deleteWeftBranch`'s existing shape.
  The helper constructs a `remoteBranchRequest` with `what` set to a phrase naming the act (e.g. `delete weft branch on remote`), `repoDir` the weft repo root, `remote` the `originRemoteName` constant, `branch` the branch, `ownership` `ownedManagedBranch(l, branchPrefix)`, `dirtiness` `dirtyCheckedOutBranch()`, and `force` the literal false, then calls `deleteRemoteBranch`.
  On success it sets `entry.RemoteDeleted` from the executor's `deleted` return.
  On an error it sets `entry.RemoteError` and must distinguish the two error classes, because `resolveBranchOwnership` spawns git twice and can therefore fail locally before `git push --delete` ever runs, and reporting that as a remote failure would blame the remote for a local git failure:
  inspect the error with `RefusalOf` and, on a match, format as `gate refused remote deletion of %q: %s` with the branch and the refusal's `Reason`;
  otherwise format as `delete remote branch %q on %q failed: %v` with the branch, the remote name, and the error.
  A remote failure never makes `Cleanup` return a non-nil `error` and never aborts the sweep — record it on the entry and continue to the next branch.

  Fifth, the file header.
  Its flag matrix currently documents `apply` and `force` only;
  add `remote` to it, stating that `remote` is independent of `--force`, that it requires `apply` to delete anything (`remote` alone is still a dry run), and that the no-`origin` reason is reported once per verb on the result rather than per branch.

  Do not change `deleteWeftBranch`, and do not reorder the existing per-branch disposition chain: unmanaged, live pair, primary weft, checked out, `!apply`, delete.
- **Commit:** `feat(fabric): delete a cleaned-up orphan weft branch on the remote under --remote`

### Card 8: removeWeftWorktree's teardown result, Remove's remote parameter, rollbackAdd

- **Context:**
  - `internal/fabricengine/destroy.go`
  - `internal/fabricengine/cleanup.go`
  - `internal/fabricengine/mutation.go`
  - `internal/fabricengine/worktreelist.go`
  - `internal/gitrepo/remote.go`
  - `internal/gitrepo/gitrepo.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/fabricengine/weftwiring.go`
  - `internal/fabricengine/remove.go`
  - `internal/fabricengine/add.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `internal/fabricengine/weftwiring.go`, declare an unexported struct `weftTeardownResult` carrying exactly three fields: `remoteBranchDeleted bool`, `remoteBranchError string`, and `remoteSkippedReason string`.
  Its doc comment records that it exists so the remote outcome can reach `Remove` without passing through the `error` return, whose meaning must not change.

  Widen `removeWeftWorktree` to `func removeWeftWorktree(rec *Mutations, l *lyxcwd.Location, slug, branch string, force, alsoDeleteBranch, remote bool, branchPrefix string) (weftTeardownResult, error)`.
  The existing `firstErr` accumulator keeps its exact current membership — worktree removal, local branch deletion, and `worktree prune` — and the remote deletion must not feed it.
  Inside the `alsoDeleteBranch` arm, after the existing `deleteBranch` call returned a nil error and only when `remote` is true, run the same once-per-call no-`origin` pre-check card 7 describes (`RemoteURL` on `gitrepo.New(weftRoot)` for `originRemoteName`), recording its failure in the result's `remoteSkippedReason` and attempting no remote deletion;
  otherwise construct a `remoteBranchRequest` exactly as card 7's helper does, call `deleteRemoteBranch`, and record `remoteBranchDeleted` or `remoteBranchError` on the result.
  `remoteBranchError`'s text uses the identical two-class `RefusalOf` formatting card 7 specifies, so the two call sites never disagree about which layer said no.
  Extend the function's doc comment to state that the returned struct carries the remote outcome, that the `error` return's meaning is unchanged, and that a remote failure is never accumulated into it.

  In `internal/fabricengine/remove.go`, add to `RemoveResult` the fields `RemoteBranchDeleted bool` with tag `json:"remote_branch_deleted,omitempty"`, `RemoteBranchError string` with tag `json:"remote_branch_error,omitempty"`, and `RemoteSkippedReason string` with tag `json:"remote_skipped_reason,omitempty"`, each documented in the same terms as their `Cleanup` counterparts.
  Widen `Remove` to `func (t *Topology) Remove(l *lyxcwd.Location, slug string, force, remote bool) (res RemoveResult, err error)` and pass `remote` through to `removeWeftWorktree` at the existing single call site, which today passes `true` for `alsoDeleteBranch`.
  Capture the new first return value and copy its three fields onto the returned `RemoveResult`.
  The existing partial-teardown guarantee is unchanged: a weft-teardown failure is still tolerated only when the weft worktree is actually gone, and that check still reads the `error` return alone, never the struct.
  `Remove` still never deletes `warpBranch` — it computes it only to derive `weftBranch` and to check merge-source in-flight — and this card adds no warp-branch deletion of either kind.
  Extend `Remove`'s doc comment to name the new parameter and to record that a remote deletion failure never makes the verb return a non-nil error.

  In `internal/fabricengine/add.go`, update `rollbackAdd`'s single `removeWeftWorktree` call to pass the literal `false` for the new `remote` parameter and to discard the new first return value with `_`.
  Add a short comment at that call site recording why it is `false`: not because the branch was never pushed — `Add` pushes the warp branch at step (11) and the weft branch at step (12), so a rollback can genuinely face an already-pushed branch on either side — but because an unattended best-effort rollback on an error path the operator did not choose must not make a network-visible destructive change, and `--remote` is opt-in precisely because deleting a shared ref needs an explicit operator decision.
  Change nothing else about `rollbackAdd`'s behaviour, and do not touch its step (5) warp-branch deletion.
- **Commit:** `feat(fabric): thread remote branch deletion through Remove's weft teardown`

### Card 9: doc.go's per-item-failure carve-out amendment

- **Context:**
  - `internal/fabricengine/cleanup.go`
  - `internal/fabricengine/prune.go`
  - `internal/fabriccli/envelope.go`
- **Edits:**
  - `internal/fabricengine/doc.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  The package doc's per-item-failure paragraph currently carves both `prune` and `cleanup` out of the rule that an item-level failure must reach the caller as a failure, on the stated test that their per-entry `Error` doubles as the explanation for a designed refusal.
  That premise never held for `cleanup`.
  The paragraph quotes two strings to prove it, and both belong to `prune` (`prune.go`'s "fabric will not remove it" and "commit them or re-run with --force").
  `Cleanup` sets no `Error` on a protected or unmanaged entry — those arms set `Protected: true` and nothing else — and `CleanupBranchEntry.Error`'s own field doc describes it as a genuine deletion failure.

  Rewrite the carve-out to name `prune` alone.
  Record that `cleanup` was removed from it because its per-entry `Error` is a failure rather than a refusal, with a pointer to `CleanupBranchEntry.Error`'s own field doc in `internal/fabricengine/cleanup.go` as the evidence, and that the new `RemoteError` falls on the same side for the same reason.
  Narrow the carve-out's scope explicitly so a reader cannot conclude that `cleanup`'s designed-refusal dispositions are covered by it: `Protected` and unmanaged entries still exit 0, because they set no `Error` at all, and that is a property of the field being empty rather than of a carve-out.
  Keep the paragraph's stated test — whether the field means "this verb failed at its job" or "this verb is telling you what it deliberately did not do" — verbatim in substance;
  it is the test that decides the question, and it is what places `cleanup` on the reconcile side.

  Add one sentence recording that a missing `origin` remote sits on the deliberately-did-not-do side and exits 0, carried by the verb-level `RemoteSkippedReason` rather than by either per-branch error field, so the identical configuration state produces the identical verdict from `cleanup` and `remove` alike.

  Do not alter the paragraph's `reconcile` history or the `ReconcileActionVanishedMidWalk` paragraph that follows it.
- **Commit:** `docs(fabric): remove cleanup from doc.go's designed-refusal carve-out`

### Card 10: Restore fabriccli compilation

- **Context:**
  - `internal/fabricengine/cleanup.go`
  - `internal/fabricengine/remove.go`
- **Edits:**
  - `internal/fabriccli/fabric.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Pass the literal `false` for the new fourth argument at `runCleanupWithFlags`'s `Cleanup` call and for the new fourth argument at `runRemoveWithFlag`'s `Remove` call, so the package compiles against the widened signatures.
  Change nothing else in this file: no flag is registered, no help text is rewritten, no fields map gains a key, and neither handler changes which envelope helper it exits through.
  All of that is batch 4's work on the same two functions.
  Add no comment claiming the `false` is a permanent decision — it is an interim value one batch long.
- **Commit:** `refactor(fabric): pass remote=false at the two engine call sites`

### Card 11: Update existing call sites for the widened signatures

- **Context:**
  - `internal/fabricengine/cleanup.go`
  - `internal/fabricengine/remove.go`
- **Edits:**
  - `internal/fabricengine/reconcile_stale_registration_test.go`
  - `internal/fabricengine/reconcile_stale_removal_test.go`
  - `internal/fabricengine/mergesiblings_integration_test.go`
  - `internal/fabricengine/mergecrucible_integration_test.go`
  - `internal/fabricengine/cleanup_primary_integration_test.go`
  - `internal/fabricengine/livestate_verbs_test.go`
  - `internal/fabricengine/livestate_refusal_selftest_test.go`
  - `internal/fabricengine/destroy_containment_toctou_integration_test.go`
  - `internal/fabricengine/remove_reserved_integration_test.go`
  - `internal/fabricengine/remove_junctions_integration_test.go`
  - `internal/fabricengine/remove_guard_integration_test.go`
  - `internal/fabricengine/remove_refusal_remedy_integration_test.go`
  - `internal/fabricengine/mutation_record_integration_test.go`
  - `internal/fabricengine/origin_integration_test.go`
  - `internal/loomcli/smoke_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Every existing `Cleanup` and `Remove` call site in these files gains the literal `false` as its new trailing `remote` argument, and nothing else changes — no assertion, no fixture, no test name, no comment.
  These are pure signature updates that preserve each test's existing meaning: every one of them predates `--remote` and asserts a purely local outcome, so `false` is the value that keeps it asserting what it always asserted.

  The call sites, by file, are the ones `Cleanup(` and `Remove(` resolve to on a `*Topology` receiver.
  `internal/fabricengine/reconcile_stale_registration_test.go` holds nine `Cleanup` calls and two `Remove` calls;
  `internal/fabricengine/mergesiblings_integration_test.go` holds one `Cleanup` and two `Remove`;
  `internal/fabricengine/cleanup_primary_integration_test.go` holds two `Cleanup`;
  `internal/fabricengine/livestate_verbs_test.go` holds one `Cleanup` and two `Remove`;
  `internal/fabricengine/mergecrucible_integration_test.go` holds six `Remove`;
  `internal/fabricengine/remove_junctions_integration_test.go` holds three `Remove`;
  `internal/fabricengine/remove_refusal_remedy_integration_test.go` holds four `Remove`;
  `internal/fabricengine/remove_guard_integration_test.go` holds two `Remove`;
  `internal/fabricengine/livestate_refusal_selftest_test.go` holds two `Remove`;
  and `internal/fabricengine/reconcile_stale_removal_test.go`, `internal/fabricengine/destroy_containment_toctou_integration_test.go`, `internal/fabricengine/remove_reserved_integration_test.go`, `internal/fabricengine/mutation_record_integration_test.go`, `internal/fabricengine/origin_integration_test.go`, and `internal/loomcli/smoke_test.go` each hold one `Remove`.
  Treat those counts as the expected shape rather than as a substitute for the compiler: the three `go vet` passes in this batch's `verify:` are what enumerate the real set, and any additional site they report inside these files is fixed here, in this same card.
  A site the compiler reports in a file NOT listed in this card's `Edits:` is a plan defect — report it rather than editing an unlisted file.

  Note the tag spread, which is why this batch vets three configurations: `internal/fabricengine/destroy_containment_toctou_integration_test.go` and most of the rest are `//go:build integration`, and `internal/loomcli/smoke_test.go` is `//go:build smoke`, so neither an untagged build nor an untagged test run would ever type-check them.
- **Commit:** `test(fabric): pass remote=false at every existing Cleanup and Remove call site`

### Card 12: Engine behaviour tests over real hubs

- **Context:**
  - `internal/fabricengine/cleanup.go`
  - `internal/fabricengine/remove.go`
  - `internal/fabricengine/weftwiring.go`
  - `internal/fabricengine/destroy.go`
  - `internal/fabricengine/mutation.go`
  - `internal/fabricengine/branchname.go`
  - `internal/fabricengine/worktreelist.go`
  - `internal/fabricengine/mutation_record_integration_test.go`
  - `internal/fabricengine/cleanup_primary_integration_test.go`
  - `internal/fabricengine/testmain_test.go`
  - `internal/hubforge/hub.go`
  - `CONSTRAINTS.md`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/cleanupremote_integration_test.go`
  - `internal/fabricengine/removeremote_integration_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Two new files, each with a leading `//go:build integration` line, a file-header comment naming what it covers, and `package fabricengine_test` — the external test package, matching `internal/fabricengine/cleanup_primary_integration_test.go` and `internal/fabricengine/mutation_record_integration_test.go`, since every assertion here goes through exported API.
  Build every hub through `hubforge.NewHub` per the hubforge Fabric-Fixture Invariant, and use the hub's own `WeftBare` field as the weft remote to assert against — it is this hub's private copy of the weft bare remote, so a test can push an orphan branch to it and then assert the ref is gone.
  Add no `TestMain`: the package already has one.

  `internal/fabricengine/cleanupremote_integration_test.go` covers `Cleanup`:

  1. `apply` and `remote` both true deletes both the local orphan weft branch and its copy on the remote, and the result entry reports `RemoteDeleted`.
  2. `apply` true and `remote` false leaves the remote copy intact — the regression guard on the existing default, and the assertion that proves the feature is opt-in.
  3. A dry run with `remote` true performs no deletion on either side.
  4. An orphan branch that was never pushed: the local deletion succeeds, `RemoteDeleted` is false, `RemoteError` is empty, and no `KindRemoteBranchDeleted` entry appears in the mutation record — the idempotence path end to end.
  5. A protected branch — primary weft, checked out, or unmanaged — has neither its local nor its remote copy touched with `remote` true.
  6. A remote deletion failure leaves the verb returning a nil error, the local branch deleted, and `RemoteError` populated.
     Induce it by pointing the weft repo's `origin` at a filesystem path that no longer exists, so no network is needed.
     Assert the verb's returned `error` is nil, which is what makes the engine's non-fatal posture observable rather than assumed.
  7. A weft repo with no `origin` configured, under `apply` and `remote` both true: every orphan branch is deleted locally, `RemoteSkippedReason` is non-empty exactly once on the result, every entry's `RemoteError` is empty, and the verb returns a nil error.
     Assert the reason appears once rather than once per entry — N duplicated copies of one fact is the specific defect this pre-check exists to prevent.
  8. The same remoteless weft repo under `remote` true without `apply`: `RemoteSkippedReason` is still populated, and nothing is deleted on either side.
     This is the dry-run half of the pre-check decision, and it is offline-safe because `RemoteURL` reads local config and spawns nothing.
  9. The same remoteless weft repo with `remote` false: `RemoteSkippedReason` is empty.
     This guards the `remote` guard itself — without it, a plain `cleanup --apply` against a remoteless repo would start reporting a reason, an observable change to a path this task does not otherwise touch.

  `internal/fabricengine/removeremote_integration_test.go` covers `Remove`:

  1. `remote` true deletes the pair's weft branch on the remote, sets `RemoteBranchDeleted`, and records exactly one `KindRemoteBranchDeleted` entry;
     `remote` false does neither.
  2. `Remove`'s existing partial-teardown guarantees are unchanged when the remote deletion fails: the weft worktree is gone, the verb returns a nil error, `RemoteBranchError` is populated, and the failure has not been picked up by the teardown's own error accumulation.
     Induce the failure the same way — a weft `origin` pointing at an absent path.
  3. A weft repo with no `origin` configured under `remote` true: the teardown completes, `RemoteSkippedReason` is populated, `RemoteBranchError` is empty, and the verb returns a nil error.
     Covering this on `remove` as well as on `cleanup` is the point — an asymmetric outcome for one configuration state across the two verbs is the defect the shared pre-check exists to prevent.

  Mutation-record assertions follow `internal/fabricengine/mutation_record_integration_test.go`'s existing model: `KindRemoteBranchDeleted` appears exactly once per ref actually deleted on the remote, with the branch name as the entry's target and the remote name as its detail, and does not appear when nothing was deleted.
  Derive the expected branch names through `WeftBranchName` rather than hand-spelling a suffix.
- **Commit:** `test(fabric): cover remote branch deletion across Cleanup and Remove`

## Batch Tests

`verify:` runs five commands in sequence.
The three `go vet` passes — untagged, `-tags integration`, and `-tags smoke` — are what prove card 11 found every call site of the two widened signatures across the whole module, including test files no scoped `go test` would ever compile;
`go vet` type-checks test files, which `go build` does not, and it completes in about two seconds here.
They come first so a missed call site fails fast, before any test runs.

`go test ./internal/fabricengine/...` then covers the untagged tier, which for this batch is a compile check on cards 7 through 9 plus the existing untagged gate tests.
`go test -tags integration ./internal/fabricengine/...` is where card 12's two new files and most of card 11's edits actually execute.

Neither `internal/fabriccli` nor `cmd/lyx` is in this batch's `go test` scope: card 10's change to `internal/fabriccli/fabric.go` is a two-argument compile fix with no behavioural surface, and the `go vet` passes already prove it compiles.
Batch 4 is what tests that file.
`cmd/lyx` is included in the `-tags smoke` vet pass only because it holds a `smoke`-tagged file (`tierpurity_test.go`, unrelated to this batch's verbs) alongside the module's other smoke-tagged packages — see `## Prior failure` for why the `-tags smoke` pass is scoped rather than whole-module.
