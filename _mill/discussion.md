# Discussion: fabric: no remote/GitHub branch deletion

```yaml
task: 'fabric: no remote/GitHub branch deletion'
slug: fabric-remote-branch-delete
status: discussing
parent: main
```

## Problem

`fabricengine`'s two branch-cleanup call sites — `Topology.Cleanup` (`internal/fabricengine/cleanup.go`) and `removeWeftWorktree`'s `alsoDeleteBranch` path (`internal/fabricengine/weftwiring.go:224`) — delete a weft branch with `git branch -D` in the hub's local weft repo and stop there.
If that branch was ever pushed, its copy on the weft remote survives the teardown, and nothing anywhere in lyx can remove it.
`lyx fabric cleanup`'s own help text already states the gap as settled behaviour: *"Deletion is local to the hub's weft repo: a deleted branch's copy on the weft remote, if it was ever pushed, is left untouched."*

Why now: weft branches are pushed routinely (`PushAnchored`, `lyx fabric push`/`sync`, loom's per-transition push seam), so every completed task leaves a `<slug>-weft` branch on the weft remote forever.
The set only grows — nothing prunes it, and the operator has no lyx-side verb to reach for.
The roadmap names this as part of why task cleanup leaves orphaned branches upstream.

## Scope

**In:**

- A remote-ref deletion primitive on `internal/gitrepo` (`git push <remote> --delete <branch>`), sitting on the `gitexec` side of the gitrepo Client Boundary Invariant.
- A new gated executor in `internal/fabricengine/destroy.go` — the only file permitted a destructive primitive — plus its own request type, so remote deletion runs the same ownership/dirtiness pipeline local deletion already runs.
- A new mutation kind for the remote deletion, appended via `AppendRef`.
- Wiring the executor into exactly the two existing call sites the roadmap names: `Topology.Cleanup` and `removeWeftWorktree`'s `alsoDeleteBranch` path (reached from `Topology.Remove`).
- An opt-in `--remote` flag on `lyx fabric cleanup` and `lyx fabric remove`, defaulting off.
- Result-type fields reporting the remote outcome per branch, so the JSON envelope says what happened on the remote and why it didn't when it didn't.
- `CONSTRAINTS.md` update: the Fabric Destruction Chokepoint Invariant's enumerated primitive list grows from five to six.
- Doc updates in the same commit: the two CLI `Long` texts (the cleanup one currently asserts the opposite behaviour), `destroy.go`'s file header, `cleanup.go`'s file header, and `docs/overview.md` if the fabric verb surface line changes.

**Out:**

- Warp-branch deletion, local or remote.
  `Topology.Remove` does not delete the warp branch locally today; adding warp branch deletion is a new capability, not the missing remote half of an existing one.
  The roadmap item's "the corresponding branch" is the branch the two named sites already delete — a weft branch.
- The two rollback call sites, `rollbackAdd` (`add.go:317`) and `rollbackSwitch` (`checkout.go:205`).
  Both delete a branch the same failed call just created, which was never pushed, so there is no remote copy to chase.
- GitHub-specific API work: no `internal/githubclient` consumer, no owner/repo parsing, no PR-aware branch deletion.
- Remote-name configuration. `origin` is hardcoded, matching every other remote assumption in `fabricengine`.
- Probing the remote during a dry run.
- `Topology.Prune` — it removes weft *worktrees* by directory name and deletes no branch at all.
- Any change to when a branch is *pushed*.

## Decisions

### push-delete-not-github-api

- Decision: the primitive is `git push <remote> --delete <branch>`, run through `gitexec` as a new `gitrepo.Repo` method — not a GitHub REST `DeleteRef` call through `internal/githubclient`.
- Rationale: the warp and weft repos are ordinary git clones whose remotes need not be GitHub-hosted (a weft remote is frequently a separate repo, and nothing in `clone.go` asserts a host).
  `git push --delete` works against any remote and reuses the exact credential path every other fabric push already uses, so it adds no second auth chain, no owner/repo parse, and no host assumption.
  The gitrepo Client Boundary Invariant already assigns anything remote-authenticating to `gitexec`, which is precisely this.
  The roadmap title's "remote/GitHub" names the symptom (branches piling up on GitHub), not a required transport.
- Rejected: `c.Git.DeleteRef` via `githubclient` — GitHub-only, needs `parseownerrepo`, and would make the feature silently unavailable for a self-hosted or local-path remote.

### gitrepo-owns-the-primitive

- Decision: add `func (r *Repo) DeleteRemoteBranch(remote, branch string) (deleted bool, err error)` to `internal/gitrepo/push.go`, using `runChecked` (never the raw `run`), returning `deleted == false, err == nil` when the remote ref was already absent.
- Rationale: `gitrepo` is where every other remote-touching git call lives (`Push`, `PushRebaseFree`, `PushCoalesced`, fetch), and `runChecked` is the Checked-Call Invariant's default entry point — this call has no reason for a `//gitexec:raw` pin.
  Returning `deleted` separately from `err` lets the caller distinguish "removed it" from "there was nothing there", which the mutation record needs: `Mutations` entries are appended only after a primitive observably changed state.
- Rejected: calling `gitexec.Run` directly from `fabricengine` — it would duplicate remote-call handling that `gitrepo` already owns and sit outside the client split the invariant draws.

### absent-remote-ref-is-success

- Decision: `git push --delete` failing because the remote ref does not exist is mapped to `(false, nil)` — idempotent success, no mutation recorded, no error surfaced.
  Detection is by matching git's stderr (`remote ref does not exist`, plus the `error: unable to delete '<branch>': remote ref does not exist` form) on a `*gitexec.GitError`, the same stderr-substring technique `PushRebaseFree`'s `rebaseRetryTriggers` already uses.
- Rationale: every executor in `destroy.go` is idempotent for an already-absent target (`removeContainedPath` reports "nothing removed" rather than failing), and the overwhelmingly common case is a weft branch that was never pushed at all.
  Failing there would make `--remote` useless in exactly the situation it is safest.
- Rejected: pre-probing with `git ls-remote` — a second network round trip per branch, and still racy.
  Rejected: treating it as an error — turns the common case into noise.

### local-first-then-remote

- Decision: within one entry, the local `git branch -D` runs first and the remote deletion runs only if the local deletion succeeded.
- Rationale: the gate's dirtiness check (`dirtyCheckedOutBranch`) asks whether the branch is checked out at a worktree — that is the "is this pair still live" question, and it is answered against local state.
  A branch the gate refuses to delete locally is one fabric has not proven it owns, and it must not be deleted on a shared remote either.
  Local-first also means a `--remote` run that fails partway leaves the ordinary, already-shipped local outcome intact.
- Rejected: remote-first — a gate refusal after the remote ref is gone is unrecoverable and strictly worse.
  Rejected: independent attempts — decouples the two and lets a refused local delete still hit the remote.

### remote-failure-is-non-fatal

- Decision: a remote deletion failure (offline, auth unresolvable, rejected by a protected-branch rule, any other git error) never fails the enclosing verb.
  It is recorded on the per-branch result entry and the verb returns success for the local work it completed.
- Rationale: the same reasoning `PushAnchored`'s doc comment records for push — an offline laptop must not kill an autonomous run — and the same posture `Cleanup` already takes for a failed local delete, which populates `CleanupBranchEntry.Error` and continues to the next branch rather than aborting.
  The local branch really is gone; reporting the whole call as failed would misdescribe the outcome.
- Rejected: hard-failing the verb — makes `--remote` unusable offline and would strand `Remove` mid-teardown.

### opt-in-remote-flag

- Decision: a `--remote` boolean flag, default false, on `lyx fabric cleanup` and `lyx fabric remove`.
  The engine signature carries it as an explicit parameter (`Cleanup(l, apply, force, remote bool)`, `Remove(l, slug, force, remote bool)`), threaded down to `removeWeftWorktree`.
  `--remote` is independent of `--force` and, for cleanup, requires `--apply` to do anything — `--remote` alone is still a dry run.
- Rationale: deleting a ref on a shared remote is irreversible and visible to every other clone, which is a different risk class from deleting a local branch in one hub.
  Nothing in this repo makes a network-visible destructive change without the operator asking for it.
  Keeping it separate from `--force` matters because `--force` answers dirtiness only, by invariant, and must not acquire a second unrelated meaning.
- Rejected: on-by-default — silently destructive on shared state.
  Rejected: folding it into `--force` — overloads a flag whose semantics `destroy.go` pins deliberately.
  Rejected: a config key — an operator-per-invocation decision, not a repo-wide policy.

### separate-remote-branch-request-type

- Decision: a new `remoteBranchRequest` struct in `destroy.go` (fields: `what`, `repoDir`, `remote`, `branch`, `ownership branchOwnership`, `dirtiness branchDirtiness`, `force bool`) with its own `checkRemoteBranchRequest`, reusing `branchOwnership`/`branchDirtiness` and `resolveBranchOwnership`/`checkBranchDirtiness` unchanged.
  Not a `remote string` field bolted onto `branchRequest`.
- Rationale: `branchRequest`'s own doc comment states the principle — it carries no container field at all because containment is structurally N/A for a ref, "expressed by the type rather than by a per-site `""` that could be forgotten".
  A `remote` field that must be empty at the four existing local call sites is exactly the forgettable empty string that comment rejects.
  A distinct type also keeps the two executors' call-site sets disjoint and auditable.
- Rejected: one struct with an optional `remote` — reintroduces the sentinel-empty-string shape the file already argued against.

### gate-runs-unchanged-for-remote

- Decision: `checkRemoteBranchRequest` runs the same two checks `checkBranchRequest` runs — `ownedManagedBranch` ownership and the checked-out-at-a-worktree dirtiness probe — against local state, in the same order, with containment structurally N/A.
  `force` stays reserved and hardcoded false at both new call sites, consistent with every existing `branchRequest` construction.
- Rationale: the ownership question ("is this a branch fabric's own naming scheme constructs, not the primary weft line") is identical for the remote copy, and answering it locally is the only way to answer it at all.
  The dirtiness question ("is this pair still materialised somewhere") is likewise a live-pair question, and a branch checked out locally must not have its remote copy deleted.
  Reusing the predicates verbatim means there is one ownership rule for a weft branch, not two that can drift.
- Rejected: a laxer remote-side gate — would let `--remote` delete a remote ref whose local counterpart the gate just protected.

### new-mutation-kind

- Decision: add `KindRemoteBranchDeleted Kind = "remote_branch_deleted"` to `internal/fabricengine/mutation.go`, appended via `AppendRef` (a remote ref is a ref, carrying no hub-relative path conversion), with the remote name in the entry's `detail` field.
  Appended only when `DeleteRemoteBranch` reports `deleted == true`.
- Rationale: the Mutation Record Invariant requires every mutating verb to accumulate the primitive it performed, and requires the append to happen only after the primitive observably changed state.
  A distinct kind rather than reusing `KindBranchDeleted` keeps a consumer able to tell a local deletion from a remote one, which matters because only one of them is recoverable.
- Rejected: reusing `KindBranchDeleted` with a detail marker — makes the two indistinguishable to anything that switches on kind.

### per-entry-remote-reporting

- Decision: `CleanupBranchEntry` gains `RemoteDeleted bool \`json:"remote_deleted,omitempty"\`` and `RemoteError string \`json:"remote_error,omitempty"\``.
  `RemoveResult` gains `RemoteBranchDeleted bool \`json:"remote_branch_deleted,omitempty"\`` and `RemoteBranchError string \`json:"remote_branch_error,omitempty"\``.
- Rationale: the local and remote outcomes are genuinely independent — local success with remote failure is the expected offline case — so overloading the existing `Error` field would make the envelope ambiguous about which half failed.
  Every other disposition in `CleanupBranchEntry` already gets its own field for the same reason.
- Rejected: overloading `Error` — ambiguous.
  Rejected: reporting only via the mutation record — a failure records no mutation, so the reason would be invisible.

### dry-run-does-not-probe-the-remote

- Decision: a cleanup dry run (`--remote` without `--apply`) performs no network call and adds no remote-specific verdict field.
  The existing contract — "protected: false in a dry run means --apply would delete this" — extends unchanged to "…and, with --remote, would attempt the remote copy too".
  Both CLI `Long` texts say so explicitly.
- Rationale: a dry run is currently a purely local, offline-safe enumeration, and one network round trip per orphan branch would make it slow and failure-prone for no decision the operator can act on differently.
  Whether the remote ref exists does not change whether `--apply --remote` should run.
- Rejected: `git ls-remote` probing per entry — cost and flakiness with no changed decision.

### hardcoded-origin

- Decision: the remote is the literal `"origin"`, declared as an unexported constant in `fabricengine` and passed into the request.
- Rationale: `clone.go` already hardcodes `refs/remotes/origin/` and `origin/<branch>` in its weft adopt-or-create path, so fabric's geometry already assumes `origin` throughout.
  Introducing configurability here alone would be inconsistent and untested against any other value.
- Rejected: reading the remote name from config or from the branch's configured upstream — new surface, no caller needs it.

## Technical context

**Where the local deletion happens today.** One executor, `deleteBranch` (`internal/fabricengine/destroy.go:881`), runs `gitexec.Run([]string{"branch", "-D", req.branch}, req.repoDir)` after `checkBranchRequest` and appends `KindBranchDeleted` via `AppendRef` on success.
It has exactly four call sites: `cleanup.go:261` (`deleteWeftBranch`), `weftwiring.go:225` (`removeWeftWorktree`), `add.go:318` (`rollbackAdd`), `checkout.go:205` (`rollbackSwitch`).
Only the first two are in scope.

**The gate pipeline.** `checkBranchRequest` (`destroy.go:680`) refuses an unset ownership kind, refuses an unset dirtiness kind, resolves ownership via `resolveBranchOwnership`, then calls `checkBranchDirtiness`.
`checkBranchDirtiness` (`destroy.go:703`) calls `listWeftBranches(req.ownership.location)` — note it reaches the `*lyxcwd.Location` through the ownership value, which is why `ownedManagedBranch` is the one ownership constructor taking a Location.
Any new remote check must keep that same access path rather than adding a Location field to the request.
Refusals are `*destructiveRefusal` values carrying a `Check` (`CheckContainment`/`CheckOwnership`/`CheckDirtiness`) and are never silently discarded — `surfaceRefusal` is the helper that propagates them at best-effort sites, and `logger.Warn` is the established fallback where a signature cannot return one (`add.go`, `checkout.go`).

**`Topology.Cleanup` shape.** `cleanup.go:103` builds `liveWarpBranches` from `List(l.WorktreePath())`, resolves `primaryWeftBranch(l)` (refusing the whole verb if unreadable — deletions are irreversible, so an unreadable primary fails closed), enumerates `listWeftBranches(l)`, and for each branch applies, in order: unmanaged (`WeftWarpSlug` rejects it) → protected; live pair → skipped entirely, not even reported; primary weft → protected; checked out → protected; `!apply` → reported only; else `deleteWeftBranch`.
The remote deletion belongs strictly inside the final `apply` arm, after `deleteWeftBranch` returned true.

**`Topology.Remove` shape.** `remove.go:43` validates the slug, refuses the prime worktree, refuses a pair mid-merge in either direction, tears down portal and launchers, runs the no-force dirtiness gates on both sides, sweeps junctions, removes the warp worktree, then calls `removeWeftWorktree(rec, l, slug, weftBranch, force, true, t.cfg.BranchPrefix)`.
Note `Remove` never deletes `warpBranch` — it computes it only to derive `weftBranch = WeftBranchName(warpBranch)` and to check merge-source in-flight.
`removeWeftWorktree` (`weftwiring.go:202`) already tolerates a partial failure via a `firstErr` accumulator; the remote deletion must not feed that accumulator, per remote-failure-is-non-fatal.
That means `removeWeftWorktree`'s signature has to grow a way to report the remote outcome back to `Remove` — either an out-parameter struct or a changed return shape.
`removeWeftWorktree`'s only caller is `Remove`, so this is a contained change.

**`gitrepo` conventions.** `Repo.runChecked` wraps `gitexec.Run` and yields a `*gitexec.GitError` (with `ExitCode` and `Stderr`) recoverable via `errors.As` whenever git ran and rejected the command; `Repo.run` is the raw form and carries a `//gitexec:raw` marker.
`PushRebaseFree` (`push.go:90`) is the closest model for the new method: `runChecked`, then `errors.As` into `*gitexec.GitError`, then a stderr-substring classification, then either a sentinel or a wrapped error.
Error strings in this package are prefixed `gitrepo: `.

**Mutations.** `NewMutations(l.HubPath)` is constructed per verb and snapshotted in a deferred closure so the record survives the error path (`res.Mutations = rec.Snapshot()`).
`Append` converts a path to hub-relative form; `AppendRef` does not, and is what `KindBranchDeleted` already uses.

**Envelope.** `internal/fabriccli/envelope.go` plus the Mutation Record Invariant fix the shape: `mutations` always an array, `partial` always a bool, and a pre-flight failure emits a bare `output.Err` with neither key.
New result fields ride the existing result structs; no envelope change is needed.

**CLI.** `internal/fabriccli/fabric.go:331` builds `cleanupCmd` with `--apply`/`--force` and a long help text whose final paragraph currently asserts the exact behaviour this task removes — it must be rewritten, not appended to.
`fabric.go:189` builds `removeCmd`.
Both use `clihelp.WrapRunCtx` and read flags off the captured `*cobra.Command`.
Every command needs a non-empty `Short`, and the repo has help-tree tests that will see a new flag.

**No module design doc.** There is no `manifest/designs/fabric.md`; `manifest/designs/fabric-windows-verification.md` is about Windows verification and is not this module's doc.
The durable design record for the destructive gate is `destroy.go`'s own file header, which enumerates the primitives and must be updated there.
`docs/overview.md`'s fabric line lists the verb surface; check whether a new flag changes it.

## Constraints

From `CONSTRAINTS.md`, the ones this task is directly governed by:

- **Fabric Destruction Chokepoint Invariant** — `destroy.go` is the only file in `package fabricengine` permitted a destructive primitive; every executor runs containment → ownership → dirtiness → force, stopping at first failure; `--force` answers dirtiness only; a gate refusal is never silently discarded; the `rec *Mutations` recorder is threaded into `destroy.go` only.
  The file header's enumeration of five primitives becomes six, and the invariant's own wording in `CONSTRAINTS.md` must move in the same commit.
- **Fabric Git Invariant (warp + weft)** — every git op lyx performs on either side goes through `fabricengine` in Go, in-process, never raw git and never an agent.
  Read-only verbs are exempt; a remote deletion is not read-only.
- **gitrepo Client Boundary Invariant** — go-git owns local reads; `gitexec` owns anything remote-authenticating or working-tree-mutating.
  A remote branch deletion is remote-authenticating, so it is `gitexec`'s side, which is why the new method sits in `gitrepo`'s `gitexec`-backed half rather than reaching for go-git's push.
- **gitexec Checked-Call Invariant** — `gitexec.Run`/`runChecked` is the default; the raw forms survive only at pinned `//gitexec:raw`-marked sites.
  The new method uses `runChecked`.
- **Mutation Record Invariant** — every mutating verb accumulates a `*Mutations` record; an executor appends its primitive only after it observably changed state; every mutating result type embeds `MutationRecord`.
- **GitHub Auth Invariant** — all GitHub authentication goes through `internal/githubclient`, and no other production package shells out to `gh`.
  This task adds no GitHub API consumer, so it stays clear of the invariant entirely; the decision not to use the REST API is partly to avoid widening that surface.
- **Fabric Vocabulary Invariant** — `weft`/`warp` may appear in identifiers, strings, and comments only inside the owner set, which includes `fabricengine` and `fabriccli`.
  `internal/gitrepo` is NOT in the owner set, so the new `gitrepo` method and its doc comment must be vocabulary-neutral: `remote`, `branch`, `repo` — never `weft`.
  This is enforced by `TestEnforcement_FabricVocabulary`.
- **CLI / Cobra Invariant** — module `Command()`/`RunCLI` seam, non-empty `Short` on every command, errors as JSON via `internal/output`, help-tree tests.
- **Test Tier Purity Invariant** — untagged test files may not call `gitexec.Run`/`RunGit`, `exec.Command`, `gitkit.Copy*`, or `hubforge.NewHub`.
  Anything spawning git belongs in an `integration`-tagged file.
- **Hermetic Git Test Environment Invariant** — any test package whose tests spawn git must call `gitkit.HermeticGitEnv()` in `TestMain` (both `fabricengine` and `gitrepo` already do).
- **hubforge Fabric-Fixture Invariant** — every hub fixture is built by `internal/hubforge` through `fabriccli.CloneAndWire`; no hub is hand-assembled.
- **Live-Substrate Spawn Observability** — a code path reachable from a `lyx` command that starts a real OS process logs its spawn via `internal/logger`.
  Check what the existing `deleteBranch`/push paths do and match them rather than inventing a new logging shape.
- **Never Force-Add Invariant** and the **Documentation Lifecycle** apply as always.

Plus the project rule from `CLAUDE.md`: a task changing observable CLI behaviour updates its docs in the same commit, and `manifest/roadmap.md` moves on completing a planned item.

## Testing

**`internal/gitrepo` — TDD candidate, and the clearest one.**
`DeleteRemoteBranch` is a small pure-ish function over git's behaviour with three outcomes worth pinning, so write its tests first:

- deleting a branch that exists on the remote returns `(true, nil)` and the ref is gone from the remote afterwards;
- deleting a branch absent from the remote returns `(false, nil)` — the idempotence contract, and the one most likely to regress if git's stderr wording is matched too narrowly;
- a genuine failure (unreachable remote) returns a wrapped non-nil error and `deleted == false`.

These spawn git, so they live in an `integration`-tagged file against a local bare repo acting as `origin`.
The absent-ref case is the reason to cover both stderr spellings git uses.

**`internal/fabricengine` gate tests — untagged, TDD candidates.**
`checkRemoteBranchRequest`'s refusals need no git spawn beyond what `checkBranchDirtiness` already does, so mirror whatever the existing `branchRequest` shape tests do (`destroy_test.go`, `destroy_toctou_test.go`, `refusalof_test.go`):

- an unset ownership kind is refused with `CheckOwnership`;
- an unset dirtiness kind is refused with `CheckDirtiness`;
- a branch the ownership predicate rejects is refused before any push is attempted — the important one, since the whole point of the new executor is that the remote side is no laxer than the local.

**`internal/fabricengine` behaviour tests — integration-tagged, over a `hubforge` hub.**
Scenarios that must be covered:

- `Cleanup` with `--apply` and `remote` true deletes both the local orphan weft branch and its remote copy, and the result entry reports `RemoteDeleted`;
- `Cleanup` with `--apply` and `remote` false leaves the remote copy intact — the regression guard on the existing default;
- `Cleanup` dry run with `remote` true performs no deletion on either side;
- an orphan branch that was never pushed: local deletion succeeds, `RemoteDeleted` is false, `RemoteError` is empty, and no `KindRemoteBranchDeleted` mutation is recorded — the idempotence path end to end;
- a protected branch (primary weft, checked out, or unmanaged) has neither its local nor its remote copy touched, with `remote` true;
- a remote deletion failure leaves the verb succeeding, the local branch deleted, and `RemoteError` populated — inject this by pointing the remote at a path that no longer exists, so no network is needed;
- `Remove` with `remote` true deletes the pair's weft branch on the remote and records the mutation; with `remote` false it does not;
- `Remove`'s existing partial-teardown guarantees are unchanged when the remote deletion fails — the weft worktree check and the `firstErr` accumulation must not pick up the remote error.

**Mutation-record assertions.**
The existing `mutation_record_integration_test.go` is the model: assert `KindRemoteBranchDeleted` appears exactly once per actually-deleted remote ref, with the branch as the ref and `origin` as the detail, and does not appear when nothing was deleted.

**CLI tests.**
The help-tree and envelope tests already in `fabriccli` will need the new flag registered; add a case asserting `--remote` alone (without `--apply`) on cleanup performs no deletion, since that is the flag-matrix corner an operator is most likely to get wrong.

**Enforcement tests to expect.**
`TestEnforcement_FabricVocabulary` will fail if the `gitrepo` addition uses weft/warp vocabulary.
Whatever meta-test pins `destroy.go`'s executor or primitive set will need its expectation updated alongside the new executor; find it before writing the executor rather than after.

## Q&A log

- **Q:** Delete the remote branch with `git push origin --delete` or with GitHub's REST `DeleteRef` via `internal/githubclient`? **A:** [auto-pick] `git push --delete` through a new `gitrepo` method. **Why:** works against any remote, not just GitHub; reuses the credential path fabric already uses; the gitrepo Client Boundary Invariant already assigns remote-authenticating ops to `gitexec`; the REST route would add owner/repo parsing and a GitHub-only assumption for no gain.
- **Q:** Does the remote deletion get its own gated executor in `destroy.go`, or can it be a plain call from `cleanup.go`? **A:** [auto-pick] Its own gated executor in `destroy.go`. **Why:** the Fabric Destruction Chokepoint Invariant permits a destructive primitive in no other file, and deleting a ref on a shared remote is the most destructive primitive in the set.
- **Q:** Extend `branchRequest` with a `remote` field, or add a separate `remoteBranchRequest`? **A:** [auto-pick] A separate `remoteBranchRequest`. **Why:** `branchRequest`'s doc comment explicitly rejects per-site empty-string sentinels for structurally-N/A fields; a `remote` that must be `""` at four existing call sites is exactly that shape.
- **Q:** Is `--remote` opt-in or on by default? **A:** [auto-pick] Opt-in, default false, independent of `--force`. **Why:** irreversible and visible to every other clone; `--force` answers dirtiness only by invariant and must not acquire a second meaning.
- **Q:** What happens when the remote ref does not exist? **A:** [auto-pick] Idempotent success — `(false, nil)`, no mutation, no error. **Why:** every `destroy.go` executor is idempotent for an absent target, and a never-pushed weft branch is the common case.
- **Q:** Does a failed remote deletion fail the enclosing verb? **A:** [auto-pick] No — recorded per entry, verb still succeeds. **Why:** matches `PushAnchored`'s offline-laptop reasoning and `Cleanup`'s existing per-entry `Error` posture; the local deletion genuinely succeeded.
- **Q:** Local delete first or remote delete first? **A:** [auto-pick] Local first, remote only if local succeeded. **Why:** the gate's ownership and dirtiness answers come from local state; a branch the gate protects locally must not lose its remote copy, and the reverse order is unrecoverable.
- **Q:** Does this task also delete the warp branch, locally or on the remote? **A:** [auto-pick] No — out of scope. **Why:** `Remove` does not delete the warp branch locally today, so warp deletion is a new capability rather than the missing remote half; the roadmap item names `Cleanup` and `alsoDeleteBranch`, both of which act on weft branches.
- **Q:** Do the two rollback call sites (`rollbackAdd`, `rollbackSwitch`) get remote deletion too? **A:** [auto-pick] No. **Why:** both delete a branch the same failed call just created, which was never pushed.
- **Q:** Is the remote name configurable? **A:** [auto-pick] No — hardcoded `origin`. **Why:** `clone.go` already hardcodes `origin` throughout fabric's weft binding; a lone configurable point here would be inconsistent and untested.
- **Q:** Should a cleanup dry run probe the remote to report which remote copies exist? **A:** [auto-pick] No probe, no new dry-run field. **Why:** one network round trip per orphan for information that changes no operator decision; the dry run is currently offline-safe and should stay so.
- **Q:** New mutation kind, or reuse `KindBranchDeleted`? **A:** [auto-pick] New `KindRemoteBranchDeleted`, appended via `AppendRef`. **Why:** a consumer must be able to tell the recoverable local deletion from the irreversible remote one.
- **Q:** How is the remote outcome reported when local succeeds and remote fails? **A:** [auto-pick] Dedicated `RemoteDeleted`/`RemoteError` fields on `CleanupBranchEntry`, and the `Remove` equivalents on `RemoveResult`. **Why:** the two outcomes are independent, so overloading the existing `Error` would make the envelope ambiguous about which half failed.
