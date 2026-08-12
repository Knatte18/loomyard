# Batch: gate-auto-recording

```yaml
task: 'fabric: accumulate the result envelope from mutations, not control flow (slice 14)'
batch: 'gate-auto-recording'
number: 4
cards: 5
verify: go test ./internal/fabricengine/ ./internal/fabriccli/ && go vet -tags integration ./...
depends-on: [2, 3]
```

## Batch Scope

This batch makes destructive-mutation coverage provably total by construction: all eight executors in `internal/fabricengine/destroy.go` take an explicit leading `rec *Mutations` parameter and record their own primitive, and every intermediate helper on the path from a verb entry to a gate call is threaded with the same parameter.
It is one batch because a parameter added to an executor breaks every call site at once — this cannot be split without leaving the tree uncompilable.

The threading mechanism is an explicit parameter on all eight sites, never a request-type field: a missing struct field is a silent zero value the compiler accepts, and a missing parameter does not compile.
This slice exists because a record was silently dropped, so the mechanism that turns dropping it into a build failure is the right one.

Batch-local decision: `repointLink` records nothing of its own beyond the `link_removed` its inner `removeLink` call already produces.
There is deliberately no `link_repointed` kind — a repoint physically *is* a removal here plus a creation at the caller's own `fslink.CreateDirLink`, which batch 5 records.

Batch-local decision: **the compiler-enforced-parameter rule applies to the eight gate executors, not to every function on the path to one.** The exported `WireJunctions(l, slug, names) error` keeps its current signature and gains a recording sibling instead — see card 14 for the exact shape and the reason.
The gate executors are where a silently dropped record produced this slice's defects, and they are what the batch-8 guard pins;
extending the same hard rule to an exported helper with roughly fifty existing test call sites would buy nothing and churn fifteen files.

## Cards

### Card 12: the eight gate executors take and use a recorder

- **Context:**
  - `internal/fabricengine/mutation.go`
  - `internal/fabricengine/pull.go`
  - `_mill/discussion.md`
- **Edits:**
  - `internal/fabricengine/destroy.go`
  - `internal/fabricengine/warpforward_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `internal/fabricengine/destroy.go`, add `rec *Mutations` as the **leading** parameter of all eight executors, and record after the primitive observably changed state:

  - `removePath(rec *Mutations, req pathRequest) error` — on success, `rec.Append(KindPathRemoved, req.target, detail)` where `detail` is the literal `"recursive"` on the `RemoveAll` branch and `"single"` on the `os.Remove` branch. The already-absent early return (`os.IsNotExist(statErr)`) records **nothing** — that is a successful no-op, not a removal.
  - `removeGitWorktree(rec *Mutations, req pathRequest, repoDir string) (exitCode int, stderr string, err error)` — record `KindWorktreeRemoved` with `req.target` and an empty detail, **only** when `err == nil` **and** `exitCode == 0`. A nonzero exit with a nil error is reachable here and must not be recorded.
  - `removeLink(rec *Mutations, req pathRequest) error` — record `KindLinkRemoved` with `req.target` **only when the link was actually there**. A nil error from `fslink.Remove` is not sufficient: `fslink.Remove` is documented as idempotent and returns nil for an absent link, and `checkPathRequest` deliberately passes an absent target through as a no-op success before any check runs (it names `removePortal`, `removeJunctionRecords` and `removeLaunchers` as the idempotent callers that depend on it). So probe first — `os.Lstat(req.target)` before calling `fslink.Remove`, exactly as `removePath` already probes — and record only when the probe found something and the removal then succeeded. `os.Lstat`, not `os.Stat`, for the same reason `checkPathRequest` uses it: a dangling link is present as a link even though its target is not.
  - `repointLink(rec *Mutations, what, container, target string, own pathOwnership) error` — passes `rec` straight through to `removeLink` and appends nothing itself.
  - `deleteBranch(rec *Mutations, req branchRequest) (exitCode int, stderr string, err error)` — record via `rec.AppendRef(KindBranchDeleted, req.branch, "")`, **only** when `err == nil` **and** `exitCode == 0`. A branch name is a ref, not a path, so this uses `AppendRef` and never `Append`.
  - `createExclusiveDir(rec *Mutations, path string) (createdToken, error)` — record `KindDirCreated` with `path` after `os.Mkdir` succeeds, never before attempting it.
  - `createGitWorktree(rec *Mutations, repoDir string, addArgs []string, target string) (tok createdToken, exitCode int, stderr string, err error)` — record `KindWorktreeCreated` with `target` only on the success path that mints the token.
  - `resetHardTo(rec *Mutations, req pathRequest, repo *gitrepo.Repo, sha string) error` — record `KindWorktreeReset` with `req.target` and `sha` as the detail, on a nil error from `repo.ResetHard(sha)`. This is the primitive behind defect 1.

  Also change the exported wrapper `(*Fabric).ResetHard(sha string) error` to `(*Fabric).ResetHard(rec *Mutations, sha string) error`, passing `rec` into `resetHardTo`. It has five callers, not three: the three production sites in `internal/fabricengine/pull.go` (updated by card 15) and two test sites in `internal/fabricengine/warpforward_integration_test.go`, which this card updates to pass a throwaway `NewMutations("")` recorder so the assertions they carry are unchanged.
  `internal/gitrepo`'s own `(*Repo).ResetHard` is a different method on a different type and is not touched.

  Every executor records **after** the pipeline passed and the primitive succeeded, never before — a refusal records nothing, since nothing happened.
  Add a paragraph to `destroy.go`'s file header stating the recording contract and the record-only-on-observed-effect rule, and why it is what makes the truthfulness cross-check's commission direction sound.

  Do not convert `Target` here: the gate passes the absolute paths it already has, and `Mutations.Append` does the hub-relative conversion.
  `destroy.go` performs no path arithmetic for the record.
- **Commit:** `feat(fabricengine): auto-record every destructive primitive at the gate`

### Card 13: thread the recorder through the removal helpers

- **Context:**
  - `internal/fabricengine/destroy.go`
  - `internal/fabricengine/mutation.go`
  - `internal/fabricengine/export_test.go`
- **Edits:**
  - `internal/fabricengine/portals.go`
  - `internal/fabricengine/launchers.go`
  - `internal/fabricengine/add.go`
  - `internal/fabricengine/remove.go`
  - `internal/fabricengine/prune.go`
  - `internal/fabricengine/cleanup.go`
  - `internal/fabricengine/weftwiring.go`
  - `internal/fabricengine/reconcile.go`
  - `internal/fabricengine/portallauncher_test.go`
  - `internal/fabricengine/weftwiring_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add a leading `rec *Mutations` parameter to each helper that reaches a gate executor, and pass the verb's own recorder down from the entry point card 9 already installed.
  The functions, each named with the file it lives in:

  - `removePortal` — `internal/fabricengine/portals.go`; callers at `internal/fabricengine/add.go`, `internal/fabricengine/remove.go`, `internal/fabricengine/prune.go`
  - `removeLaunchers` — `internal/fabricengine/launchers.go`; same three callers. Note this function calls `checkPathRequest` and then `os.Remove` directly rather than `removePath` (it is on the destructive guard's allowlist for exactly that reason), so it records `KindPathRemoved` with detail `"single"` itself, at its own success site, using the same record-only-on-observed-effect rule.
  - `removeWarpWorktreeDir` — `internal/fabricengine/remove.go`
  - `removeStalePair` — `internal/fabricengine/prune.go`; callers at `internal/fabricengine/prune.go`
  - `deleteWeftBranch` — `internal/fabricengine/cleanup.go`; caller at `internal/fabricengine/cleanup.go`
  - `removeJunctionRecords` and `removeWeftWorktree` — `internal/fabricengine/weftwiring.go`. `removeWeftWorktree` is called from `internal/fabricengine/add.go` and `internal/fabricengine/remove.go`; `removeJunctionRecords` is reached only through the intermediates `removeWarpJunction` (`internal/fabricengine/weftwiring.go`) and `applyStaleRemoval` (`internal/fabricengine/reconcile.go`), both of which must be threaded too.

  **The caller lists in this card are illustrative, not authoritative.** The compiler is the authority for every one of these — a missing argument does not build — and the grep step below is what finds a caller this list missed. Do not treat a name absent from the list as a site that needs no threading.

  Where a helper's caller is itself a helper, thread the parameter through rather than constructing a second recorder — there is exactly one `*Mutations` per verb invocation, and it is the one card 9 or card 10 built.

  **Four of these helpers are reached from inside `(*Topology).rollbackAdd`**, which card 15 gives its own leading `rec *Mutations` parameter: `removeWeftWorktree`, `removeWarpJunction`, `removePortal` and `removeLaunchers`.
  Pass `rollbackAdd`'s `rec` at all four — passing `nil` there compiles and silently drops the entire rollback record, which **no** later assertion catches, because `Add`'s mint-then-rollback nets to zero in the manifest diff and batch 7's commission direction exempts exactly that shape.
  Card 15 owns the parameter's introduction;
  this card owns four of the six calls that record through it, and card 15 owns the other two (`removeGitWorktree`, `deleteBranch`). Six calls, one recorder, two cards — check all six are threaded before either card is considered done.

  Two of these helpers have in-package test callers, which this card repoints by passing a throwaway `NewMutations("")` recorder so each assertion's meaning is unchanged: `removeLaunchers` in `internal/fabricengine/portallauncher_test.go` and `removeJunctionRecords` in `internal/fabricengine/weftwiring_test.go`.
  `teardownHub`'s own seam in `internal/fabricengine/export_test.go` is **not** repointed here — `teardownHub` does not gain its parameter until card 14, so card 14 owns that repoint and lists the file.
  Grep each renamed helper across `internal/fabricengine/*_test.go` before finishing the card rather than trusting this list — an unbuilt test file is invisible to `go build ./...` and only the widened `go vet -tags integration ./...` in this batch's verify catches it.

  No behaviour, ordering, or error text changes anywhere in this card.
- **Commit:** `refactor(fabricengine): thread the recorder through the removal helpers`

### Card 14: thread the recorder through the junction, unwire and clone helpers

- **Context:**
  - `internal/fabricengine/destroy.go`
  - `internal/fabricengine/mutation.go`
  - `internal/configcli/configcli_integration_test.go`
  - `internal/loomengine/preflight_integration_test.go`
- **Edits:**
  - `internal/fabricengine/junction.go`
  - `internal/fabricengine/unwire.go`
  - `internal/fabricengine/clone.go`
  - `internal/fabricengine/reconcile.go`
  - `internal/fabricengine/add.go`
  - `internal/fabricengine/checkout.go`
  - `internal/fabriccli/clone.go`
  - `internal/fabricengine/remove.go`
  - `internal/fabricengine/clone_reset_guard_test.go`
  - `internal/fabricengine/junction_test.go`
  - `internal/fabricengine/export_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add a leading `rec *Mutations` parameter to:

  - `seedLyxJunction`, `unseedJunctionRecords` and `wireBoardLink` — `internal/fabricengine/junction.go`. `unseedJunctionRecords` is reachable only through the intermediate `unseedLyxJunction` (also `internal/fabricengine/junction.go`), which must be threaded too — passing `nil` through it compiles and would silently drop every `link_removed` the unwire verb produces.
  - `unwireBoardLink` — `internal/fabricengine/unwire.go`; it has **two** callers, `internal/fabricengine/unwire.go` and `internal/fabricengine/remove.go`, both of which this card updates
  - `resetHub` and `teardownHub` — `internal/fabricengine/clone.go`. Both have in-package test callers this card repoints with a throwaway `NewMutations("")` recorder: `resetHub` in `internal/fabricengine/clone_reset_guard_test.go`, and `teardownHub` through the seam in `internal/fabricengine/export_test.go`.
  - `(*Topology).repairPairWiring` — `internal/fabricengine/reconcile.go`, since it reaches the junction wiring

  **`WireJunctions` is the one exception to the leading-parameter rule, and the shape is exact.** It keeps its current signature and gains a recording sibling:

  ```go
  func WireJunctionsWith(rec *Mutations, l *lyxcwd.Location, slug string, names []string) error
  func WireJunctions(l *lyxcwd.Location, slug string, names []string) error {
  	return WireJunctionsWith(nil, l, slug, names)
  }
  ```

  `WireJunctionsWith` carries the whole body;
  `WireJunctions` is a thin nil-recorder delegation and its doc comment says so, naming `WireJunctionsWith` as the form production code calls.
  The four production callers — `internal/fabricengine/add.go`, `internal/fabricengine/checkout.go`, `internal/fabricengine/reconcile.go` and `internal/fabriccli/clone.go` — switch to `WireJunctionsWith`; card 15 owns the `add.go` and `checkout.go` half.

  This split exists because the bare `WireJunctions` has roughly fifty existing call sites across fifteen test files (`internal/fabricengine`'s own junction, unwire, reconcile, checkout, dotlyxjunction and healthreason suites, plus `internal/configcli/configcli_integration_test.go` and `internal/loomengine/preflight_integration_test.go`), none of which have a recorder to pass and none of which assert anything about the record.
  Changing the exported signature would churn all fifteen files for no coverage gain, and the two outside `internal/fabricengine` sit outside every batch's own verify scope — which is exactly why this card's verify is widened to `go vet -tags integration ./...` rather than the package-scoped form batch 3 uses.
  Verify the wrapper is complete by grepping the tree for `WireJunctions(` after the edit: every remaining hit must be either the wrapper's own declaration or a `_test.go` file.

  At `internal/fabriccli/clone.go`, `CloneAndWire` has no recorder of its own yet — batch 6 gives it one.
  For this batch, pass the record already carried by the `fabricengine.CloneResult` the preceding `CloneHub` call returned: build a local `rec := fabricengine.NewMutations(res.HubPath)`, seed it with `rec.Extend(res.Mutated())`, pass it into `WireJunctionsWith`, and leave it otherwise unused.
  Batch 6 is what folds that recorder into the returned result and the envelope;
  doing it here would mean editing `CloneAndWire`'s return shape twice.

  `teardownHub`'s many call sites in `internal/fabricengine/clone.go` all sit inside `CloneHub`, whose recorder card 10 installed — pass it at each.
  `resetHub`'s two call sites are also inside `CloneHub`, and the recorder is **already** non-nil at both: card 10 places `rec = NewMutations(hubPath)` immediately after each `hubPath = HubPath(cwd, name)` line, ahead of that branch's `if opts.Reset` block.
  This card relies on that placement and must not move it;
  if the recorder is found nil at a `resetHub` call site, card 10 was implemented wrongly and that is the thing to fix.
  The placement is load-bearing, not tidiness: the teardown's `path_removed` entry — whose hub-relative target is `"."` — is the only entry that can cover the `CloneHubReset`/`RealHub` cell's removals in batch 7's omission direction (see card 29's split treatment of a `"."` target).

  No behaviour, ordering, or error text changes anywhere in this card.
- **Commit:** `refactor(fabricengine): thread the recorder through the junction, unwire and clone helpers`

### Card 15: thread the recorder through add, checkout and pull

- **Context:**
  - `internal/fabricengine/destroy.go`
  - `internal/fabricengine/mutation.go`
- **Edits:**
  - `internal/fabricengine/add.go`
  - `internal/fabricengine/checkout.go`
  - `internal/fabricengine/pull.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - `internal/fabricengine/add.go`: pass the verb's recorder into the `createGitWorktree` call, and add a leading `rec *Mutations` parameter to `(*Topology).rollbackAdd`, threading it from each of its eleven call sites inside `Add`. `rollbackAdd` reaches **six** gate-bound calls through that parameter: its own `removeGitWorktree` and `deleteBranch`, plus the four helpers card 13 threads (`removeWeftWorktree`, `removeWarpJunction`, `removePortal`, `removeLaunchers`). All six must receive `rec`, not `nil` — that is the point: `Add`'s record must carry both the creations and the rollback's own destructions, in execution order, and a dropped rollback record is invisible to every later assertion because the mint-then-rollback pair nets to zero in the manifest diff.
  - `internal/fabricengine/checkout.go`: add a leading `rec *Mutations` parameter to `(*Topology).rollbackSwitch`, threading it from its three call sites inside `Checkout`, and switch the `WireJunctions` call to `WireJunctionsWith`, passing the recorder.
  - `internal/fabricengine/pull.go`: update the three `f.ResetHard(upstreamSHA)` call sites to `f.ResetHard(rec, upstreamSHA)`, using the recorder card 10 installed in `Pull`.

  No behaviour, ordering, or error text changes.
  After this card the tree compiles with every destructive primitive recorded through the gate, and `go test ./internal/fabricengine/ ./internal/fabriccli/` passes unchanged — nothing yet *reads* the record, so no existing assertion can move.
- **Commit:** `refactor(fabricengine): thread the recorder through add, checkout and pull`

### Card 16: gate recording tests

- **Context:**
  - `internal/fabricengine/destroy.go`
  - `internal/fabricengine/mutation.go`
  - `internal/fabricengine/destroy_test.go`
  - `internal/fabricengine/export_test.go`
- **Edits:**
  - `internal/fabricengine/destroy_test.go`
  - `internal/fabricengine/export_test.go`
  - `internal/fabricengine/destructivegaps_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Repoint every executor call in `internal/fabricengine/destroy_test.go` and `internal/fabricengine/export_test.go` to the new leading-`rec` signatures, preserving each assertion's current meaning.

  `internal/fabricengine/destructivegaps_integration_test.go` calls `RemoveWarpWorktreeDirForTest` directly (a re-exported `var` alias of `removeWarpWorktreeDir`, whose own leading-`rec` parameter card 13 already added) — its one call site is repointed with a throwaway `NewMutations("")` recorder, same as every other in-package test caller in this batch, so the assertion's meaning is unchanged.

  Add untagged table tests to `internal/fabricengine/destroy_test.go` covering the record-only-on-observed-effect rule, using only filesystem primitives already reachable from an untagged test in this package (`removePath`, `removeLink`, `createExclusiveDir` — no git spawn, per the Test Tier Purity Invariant):

  - `removePath` on an already-absent target returns nil **and** records nothing.
  - `removePath` on a directory records one `path_removed` with detail `recursive`;
    on a plain file, detail `single`.
  - `removePath` on a refused request (a containment failure, reusing the file's existing refusal fixtures) records nothing.
  - `removeLink` on a refused request records nothing.
  - `removeLink` on an **already-absent** target returns nil and records nothing — the absent-link case `fslink.Remove`'s own idempotence would otherwise turn into a recorded removal that never happened.
  - `removeLink` on a present link records exactly one `link_removed`.
  - `createExclusiveDir` records one `dir_created` on success and nothing on the already-exists EEXIST path.
  - A `nil` recorder passed to any of the above does not panic.

  The git-spawning executors (`removeGitWorktree`, `deleteBranch`) are **not** covered here: an untagged test in this package may not spawn git, and their nonzero-exit-with-nil-error rule is asserted through `internal/fabricengine/fabrictest`'s tagged matrix in batch 7 instead.
  Say so in a comment on the new test group, so the omission reads as a decision rather than a gap.
- **Commit:** `test(fabricengine): cover the gate's record-only-on-observed-effect rule`

## Batch Tests

`verify: go test ./internal/fabricengine/ ./internal/fabriccli/ && go vet -tags integration ./...` covers the two packages whose production code this batch rewrites, plus a module-wide tagged type-check.
The chained vet is `go vet -tags integration ./...` — module-wide, not the package-scoped form batch 3 uses — and the width is load-bearing: `UnwireJunctions` is called from the `integration`-tagged `internal/fabricengine/fabrictest/verbs.go`, invisible to both the untagged test run and `go build ./...`, and the helper repoints in this batch reach tagged files in `internal/configcli` and `internal/loomengine` that a `./internal/fabricengine/...` scope would never compile.
The new assertions live in `internal/fabricengine/destroy_test.go`;
`internal/fabricengine/export_test.go` is repointed but adds no new case.
