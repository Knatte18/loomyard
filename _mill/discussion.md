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
- Wiring the executor into the two existing call sites the roadmap names: `Topology.Cleanup` and `removeWeftWorktree`'s `alsoDeleteBranch` path **as reached from `Topology.Remove`**.
  `removeWeftWorktree` has a second caller, `rollbackAdd` (`add.go:264`), which also reaches that same `alsoDeleteBranch` arm; it passes `remote: false` and is unchanged in behaviour, but it does have to compile against the widened signature — so the change touches three call sites even though only two of them ever delete remotely.
- An opt-in `--remote` flag on `lyx fabric cleanup` and `lyx fabric remove`, defaulting off.
- Result-type fields reporting the remote outcome per branch, so the JSON envelope says what happened on the remote and why it didn't when it didn't, plus the CLI-layer verdict change that keeps a remote failure from exiting 0.
- Enforcement-ledger updates in `cmd/lyx/destructiveguard_test.go` and `internal/fabricengine/livestate_mutationoracle_test.go` — see Testing → "Enforcement ledgers" for the exact rows.
- `destroy.go`'s file header: its "five primitives" enumeration (`destroy.go:1-6`) becomes six, and its recording contract's "every one of the eight executors below" (`destroy.go:53`) becomes nine.
  `CONSTRAINTS.md` is NOT edited by this task: the shipped Fabric Destruction Chokepoint Invariant names no primitive count and no individual executor — its bullets are "every executor checks, in order", "`--force` answers dirtiness only", "a gate refusal is never silently discarded", "the `rec *Mutations` recorder is threaded into `destroy.go` only" — and every one of those stays true verbatim with a sixth primitive.
- An amendment to `internal/fabricengine/doc.go`'s per-item-failure carve-out paragraph (`:570-572`), narrowing `cleanup`'s exemption to its designed-refusal fields so `RemoteError` is visibly excluded.
- Doc updates in the same commit, as a checklist rather than a category:
  - `cleanupCmd.Long` (`fabric.go:333`) — its final paragraph currently asserts the exact behaviour this task removes, so it is rewritten, not appended to; its flag matrix gains `--remote` and the dry-run disposition.
  - `removeCmd.Long` (`fabric.go:191`).
  - `cleanupCmd.Use` (`"cleanup [--apply] [--force]"`, `fabric.go:331`) and `removeCmd.Use` (`"remove [--force] <slug>"`, `fabric.go:189`) — both gain `[--remote]`, and the help-tree tests see them.
  - `runRemoveWithFlag`'s usage string (`"usage: lyx fabric remove [--force] <slug>"`, `fabric.go:802`) — the same line, in a third place.
  - `destroy.go`'s file header (the primitive and executor counts, above) and `cleanup.go`'s file header (its flag matrix).
  - `CleanupBranchEntry.Error`'s own field doc (`cleanup.go:77`), now that the field drives an exit code.
  - `docs/overview.md`, if the fabric verb surface line changes.

**Out:**

- Warp-branch deletion, local or remote.
  `Topology.Remove` does not delete the warp branch locally today; adding warp branch deletion is a new capability, not the missing remote half of an existing one.
  The roadmap item's "the corresponding branch" is the branch the two named sites already delete — a weft branch.
- Remote deletion from the rollback paths: `rollbackAdd`'s warp-branch deletion (`add.go:317`), `rollbackAdd`'s weft-branch deletion via `removeWeftWorktree` (`add.go:264`), and `rollbackSwitch`'s forked weft branch (`checkout.go:205`).
  All three pass `remote: false`.
  The reason is NOT "the branch was never pushed" — that premise is false for `rollbackAdd`, which is reached after `Add`'s step (11) pushes the warp branch (`add.go:218`) and after step (12) pushes the weft branch, so a rollback can face an already-pushed branch on either side.
  The real reason is that a rollback undoes a failed call and must not make a **network-visible destructive change** while doing it: it runs unattended on an error path the operator did not choose, it is already best-effort and void-returning at both sites (a refusal there only gets a `logger.Warn`), and `--remote` is opt-in precisely because deleting a shared ref needs an explicit operator decision.
  Recovering an orphan left by a rolled-back `Add` is what `cleanup --apply --remote` is for.
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
  Detection is a single `strings.Contains` for the substring `remote ref does not exist` against a `*gitexec.GitError`'s `Stderr`, the same stderr-substring technique `PushRebaseFree`'s `rebaseRetryTriggers` already uses.
  One substring is enough: git's fuller wording (`error: unable to delete '<branch>': remote ref does not exist`) contains it verbatim, so there is no second spelling to match and no list is needed.
  The test pins that exact substring, so a future git rewording fails loudly rather than silently reclassifying the common case as an error.
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

### remote-failure-is-non-fatal-in-the-engine

- Decision: a remote deletion failure (offline, auth unresolvable, rejected by a protected-branch rule, any other git error) never aborts the enclosing **engine** verb.
  `Cleanup` records it on the branch's entry and moves to the next branch; `Remove` records it on `RemoveResult` and completes the teardown.
  Neither returns a non-nil `error` for it, and neither lets it feed `removeWeftWorktree`'s `firstErr` accumulator.
- Rationale: the same reasoning `PushAnchored`'s doc comment records for push — an offline laptop must not kill an autonomous run — and the same posture `Cleanup` already takes for a failed local delete, which populates `CleanupBranchEntry.Error` and continues rather than aborting.
  The local branch really is gone, and the remaining teardown work still has to run.
- Rejected: returning an error from the engine verb — would strand `Remove` mid-teardown and abort a cleanup sweep on its first unreachable remote.
- Note: this decides the engine's control flow only.
  The CLI's exit code is decided separately — see remote-failure-is-a-cli-failure.

### remote-failure-is-a-cli-failure

- Decision: at the `internal/fabriccli` layer, a remote deletion failure IS a failure.
  `runCleanupWithFlags` emits its `entries` array through `errWithRecordFields` (not `okWithRecord`) whenever any entry carries a non-empty `RemoteError`; `runRemoveWithFlag` does the same with its fields map whenever `RemoteBranchError` is non-empty.
  The per-item report survives either way; only the verdict, `partial`, and the exit code change.
- Rationale: `internal/fabricengine/doc.go:560-572` fixes the repo rule and states why — an item-level failure must reach the caller as a failure, not only as a field inside a success envelope, because `runReconcile`'s old unconditional `okWithRecord` produced `"ok":true`, `"partial":false`, exit 0 for a repair that did not happen.
  The same doc carves `prune` and `cleanup` out, but on an explicit test: their per-entry `Error` doubles as the explanation for a **designed refusal** (`Protected`, `Unowned`), so "this verb is telling you what it deliberately did not do".
  A push rejected by the network, by auth, or by a protected-branch rule is not a designed refusal — it is the verb failing at part of its job — so it falls on the reconcile side of that same test, not inside the carve-out.
  `partial` then reads true (error non-nil, record non-empty), which is exactly the truth: the local branch went, the remote copy did not.
- **The synthesised error**, since the engine returns nil and `errWithRecordFields(w, rec, err, fields)` calls `err.Error()`:
  the CLI builds it with `fmt.Errorf`, and it is a **summary, not a transcript** — the per-branch reasons already ride `entries[].remote_error`, so repeating them in the error string would duplicate the envelope's own content.
  - `runCleanupWithFlags`, aggregating across N entries: count the entries with a non-empty `RemoteError` as `failed`, and the entries with `Deleted == true` as `attempted`, then
    `fmt.Errorf("remote branch deletion failed for %d of %d orphan branches (%s); each branch's reason is in entries[].remote_error", failed, attempted, strings.Join(failedBranchNames, ", "))`.
    `Deleted == true` IS the attempted set and needs no new field: local-first-then-remote means a remote deletion is tried exactly when the local one succeeded, and an entry that was protected, skipped, or dry-run never has `Deleted` set.
    Branch names are joined in enumeration order, so the string is deterministic across runs.
  - `runCleanupWithFlags`, local failures only (the new non-zero exit this task introduces, so its wording is newly observable and pinned the same way):
    `fmt.Errorf("branch deletion failed for %d of %d orphan branches (%s); each branch's reason is in entries[].error", localFailed, attemptedLocal, strings.Join(localFailedBranchNames, ", "))`,
    where `attemptedLocal` counts entries that reached `deleteWeftBranch` at all — neither protected nor dry-run.
    It never mentions the remote.
  - `runCleanupWithFlags`, both classes in one run: `errWithRecordFields` takes one `error`, so the two are joined into one string rather than reported by two calls —
    `fmt.Errorf("%v; additionally, %v", localErr, remoteErr)`, where each half is the exact string its own format above produces, local first.
    Both halves' per-branch reasons stay in the entries array; the joined string adds no new content.
  - The three formats compose from the same two counts, so a run with one failing class never emits the other half's wording.
  - `runRemoveWithFlag`, with exactly one branch and therefore no aggregation:
    `fmt.Errorf("weft branch %q was deleted locally, but its copy on %q was not: %s", weftBranch, remoteName, r.RemoteBranchError)`.
    Here the reason IS inlined, because there is one of it and no array to read it from.
- **No `refusal` key on this path.** `errWithRecordFields` attaches a `refusal` object only when `fabricengine.RefusalOf(err)` matches a `*destructiveRefusal`; a synthesised `fmt.Errorf` never will, so these envelopes carry `ok`, `error`, `mutations`, `partial` and the verb's own fields, and no `refusal`.
  That is correct rather than a gap: a push the remote rejected is not a gate refusal, and claiming one would misreport which layer said no.
- Rejected: exit 0 with the reason buried in a field — reproduces verbatim the `runReconcile` defect `doc.go` records, for a scripted caller that has no reason to parse `remote_error`.
- Rejected: joining every per-branch reason into the error string — duplicates `entries[].remote_error` and grows without bound on a wide sweep.
- Rejected: widening `prune`/`cleanup`'s existing carve-out to cover this — the carve-out's stated test is "designed refusal", and stretching it to cover a genuine failure would erase the distinction the doc draws.
- **`cleanup` leaves `doc.go`'s carve-out entirely; `prune` stays in it alone.** The carve-out's premise does not hold for `cleanup` and never did.
  `doc.go:570-572` justifies exempting both verbs on their per-entry `Error` doubling as a designed-refusal explanation, and quotes two strings to prove it — but both belong to **`prune`** (`prune.go:205` "fabric will not remove it", `prune.go:236` "commit them or re-run with --force").
  `Cleanup` never sets `Error` on a protected or unmanaged entry: those arms set `Protected: true` and nothing else (`cleanup.go:137-166`), and `CleanupBranchEntry.Error` is documented at `cleanup.go:77` as "non-empty when apply is true and branch deletion failed" — a genuine failure, populated only at `cleanup.go:258` and `:270`.
  So `cleanup` has no designed-refusal `Error` at all, and by `doc.go`'s own test its existing local-delete failure belongs on the reconcile side exactly as `RemoteError` does.
- **Consequently `runCleanupWithFlags` keys `errWithRecordFields` on BOTH `Error` and `RemoteError`**, not on `RemoteError` alone.
  This is a deliberate, in-scope behaviour change to an existing path: a `cleanup --apply` whose local `git branch -D` failed exits non-zero from now on, where today it exits 0.
  Taking it is the only way to avoid shipping the asymmetry — same verb, same entry struct, two failures of the same class, two verdicts — and the fix is one condition at a call site this task already rewrites.
  `Protected`/`Unowned` entries still exit 0, because they set no `Error` at all.
  `prune` is untouched: its `Error` genuinely is a designed refusal, and it is a different verb with a different result type.
- Consequence for the plan: `doc.go`'s carve-out paragraph is amended in the same commit to name `prune` alone, and to record that `cleanup` was removed from it because its `Error` is a failure rather than a refusal — with a pointer to `cleanup.go:77`'s own field doc as the evidence.

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
- Rationale: one ownership rule for a weft branch, not two that can drift, and an executor that cannot be called without declaring both predicates.
- **What actually protects the call sites, stated plainly.** At both real call sites the remote executor runs only after `git branch -D` already succeeded (local-first-then-remote), so the branch is gone from the local repo by then.
  `checkBranchDirtiness` probes `listWeftBranches` for a checked-out worktree path and therefore **cannot refuse at either real call site** — the branch it would look for no longer exists.
  The "a branch checked out locally must not have its remote copy deleted" guarantee is delivered entirely by the local-first ordering: the gate refused the local deletion, so the remote deletion is never attempted.
  The reused dirtiness check is defence-in-depth and request-shape enforcement at the executor — it keeps a future call site that reaches the executor *without* a preceding local deletion from bypassing the probe, and it keeps the request type unconstructable without a declared dirtiness kind.
  It is not an end-to-end guarantee, and the plan must not describe it as one.
- **Ownership is only partly vacuous, unlike dirtiness.** `ownedManagedBranch`'s naming predicate (accepted by `WeftWarpSlug`, or carrying `branchPrefix`) and its primary-weft-branch carve-out both read values that survive the local deletion, so those still evaluate meaningfully at the real call sites; only its checked-out component shares dirtiness's fate.
- **A gate error is not a push failure, and `RemoteError` must say which.** `resolveBranchOwnership` spawns git twice (`primaryWeftBranch`, `listWeftBranches`), so the executor can return a `*destructiveRefusal` — or a refusal whose own `Reason` is a spawn failure — before `git push --delete` ever runs.
  Reporting that as "remote branch deletion failed" would blame the remote for a local git failure.
  Rule: the call site inspects the executor's error with `fabricengine.RefusalOf` and formats accordingly — `fmt.Sprintf("gate refused remote deletion of %q: %s", branch, refusal.Reason)` on a match, `fmt.Sprintf("delete remote branch %q on %q failed: %v", branch, remoteName, err)` otherwise — so `RemoteError`'s text always names the layer that said no.
  The CLI's synthesised summary stays the neutral "remote branch deletion failed for N of M", which is true of both.
- Rejected: a laxer remote-side gate — would let a future direct caller delete a remote ref against no predicate at all.
- Rejected: reordering to run the remote deletion first so the dirtiness probe bites — see local-first-then-remote; an unrecoverable remote deletion ahead of a gate refusal is strictly worse than a check that is redundant with the ordering.

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

### no-origin-is-resolved-once-per-verb

- Decision: a weft repo with no `origin` remote configured is detected **once per verb**, before the loop, via `gitrepo.New(weftRepoRoot).RemoteURL("origin")` — and **only when `remote` is true**.
  With `remote` false the pre-check does not run at all, `RemoteSkippedReason` stays empty, and the JSON key is present-but-empty exactly as `remote_branch_error` is.
  This matters because the fields maps emit their keys unconditionally: without the `remote` guard, a plain `cleanup --apply` or `remove` against a remoteless repo would start reporting a non-empty `remote_skipped_reason`, an observable change to a path this task does not otherwise touch.
  On failure, `Cleanup` performs no remote deletion for any entry and records the reason once — on the verb's result, not per branch — and every entry's `RemoteError` stays empty.
  `Remove`, having one branch, does the same check inline and records the reason the same way.
  This pre-check is the ONLY thing it gates; it is not a general reachability probe, and a configured-but-unreachable remote still fails per-branch through the ordinary path.
- Decision on the dry run: the pre-check **does** run under `--remote` without `--apply`, and populates `RemoteSkippedReason` there.
  This does not contradict dry-run-does-not-probe-the-remote, which bans a *network* probe; `RemoteURL` is a local config read that spawns no process and contacts nothing, so the dry run stays offline-safe.
  Reporting it in the dry run is the point: an operator checking what `--apply --remote` would do should be told up front that it would do nothing remotely, rather than discovering it after the local deletions have happened.
- Decision on the carrier, so the plan has no fork: a dedicated `RemoteSkippedReason string \`json:"remote_skipped_reason,omitempty"\`` field on **both** `CleanupResult` (verb-level, beside `Entries` — not on `CleanupBranchEntry`) and `RemoveResult`.
  It is deliberately NOT `RemoteError`/`RemoteBranchError`: those two are the keys remote-failure-is-a-cli-failure switches the exit code on, and reusing them would make a missing `origin` fail the verb.
- Decision on the exit code, uniformly across both verbs: a missing `origin` exits **0**.
  `runCleanupWithFlags` and `runRemoveWithFlag` both key `errWithRecordFields` on their per-branch `RemoteError`/`RemoteBranchError` field alone and never on `RemoteSkippedReason`, so the identical configuration state produces the identical verdict from either verb.
- Rationale for exit 0: an unconfigured remote is not the verb failing at its job — there was no remote to delete from, the operator asked for something the repo has no counterpart for, and the local work completed exactly as specified.
  It sits on the "telling you what it deliberately did not do" side of `doc.go`'s own test, which is precisely the side the `prune`/`cleanup` carve-out already occupies.
- Rationale: `RemoteURL` is a go-git local config read that spawns no process and touches no network (see `internal/gitrepo/remote.go`'s own doc comment, which records exactly that as why it sits outside the Client Boundary Invariant's pinned list), so the check is free.
  Without it, `cleanup --apply --remote` against a remoteless weft repo emits one identical `'origin' does not appear to be a git repository` per orphan branch — N copies of a single fact — and, under remote-failure-is-a-cli-failure, turns the whole verb into a failure for a condition that is a configuration state rather than a per-branch outcome.
  Detecting it once says it once.
- Rejected: folding it into the generic per-branch failure path — N duplicated errors for one cause.
- Rejected: treating a missing `origin` as a second idempotent-success case alongside absent-remote-ref-is-success — a repo with no remote is not a repo whose remote ref is already gone, and silently succeeding would hide a real misconfiguration.
- Guardrails the carrier above already satisfies, restated because they are what makes it the right one: it must not make the verb return a non-nil engine error (remote-failure-is-non-fatal-in-the-engine), and it must not by itself trip remote-failure-is-a-cli-failure's `errWithRecordFields` branch.

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
`removeWeftWorktree` (`weftwiring.go:202`) already tolerates a partial failure via a `firstErr` accumulator; the remote deletion must not feed that accumulator, per remote-failure-is-non-fatal-in-the-engine.

**`removeWeftWorktree` has TWO callers, and both must compile against the new shape:**

- `remove.go:132` — `Remove`, passing `alsoDeleteBranch: true`;
- `add.go:264` — `rollbackAdd`, passing `alsoDeleteBranch: !weftBranchAdopted`, so the rollback path genuinely reaches the branch-deletion arm this task wires into.
  It passes `remote: false` (see the Scope → Out bullet on the rollback sites) and discards the new return value.

Decided reporting shape, so the plan writer has no fork: `removeWeftWorktree` returns `(weftTeardownResult, error)`, where `weftTeardownResult` is a small unexported struct carrying `remoteBranchDeleted bool`, `remoteBranchError string`, and `remoteSkippedReason string`.
The existing `error` return keeps its exact current meaning — the `firstErr` accumulator over worktree removal, local branch deletion, and prune — and the remote outcome rides the struct, never the error.
Chosen over an out-parameter (`*weftTeardownResult` argument) because the function already returns a value-typed error, a second return is the ordinary Go shape here, and an out-parameter would let a caller pass nil and silently drop the outcome.
`Remove` copies the struct's three fields onto `RemoveResult`; `rollbackAdd` discards it with `_`.

**`gitrepo` conventions.** `Repo.runChecked` wraps `gitexec.Run` and yields a `*gitexec.GitError` (with `ExitCode` and `Stderr`) recoverable via `errors.As` whenever git ran and rejected the command; `Repo.run` is the raw form and carries a `//gitexec:raw` marker.
`PushRebaseFree` (`push.go:90`) is the closest model for the new method: `runChecked`, then `errors.As` into `*gitexec.GitError`, then a stderr-substring classification, then either a sentinel or a wrapped error.
Error strings in this package are prefixed `gitrepo: `.

**Mutations.** `NewMutations(l.HubPath)` is constructed per verb and snapshotted in a deferred closure so the record survives the error path (`res.Mutations = rec.Snapshot()`).
`Append` converts a path to hub-relative form; `AppendRef` does not, and is what `KindBranchDeleted` already uses.

**Envelope.** `internal/fabriccli/envelope.go` plus the Mutation Record Invariant fix the shape: `mutations` always an array, `partial` always a bool, and a pre-flight failure emits a bare `output.Err` with neither key.
The three helpers are `okWithRecord`, `errWithRecord`, and `errWithRecordFields` (the last is the one that carries a per-item report on the failure path, and it also attaches a `refusal` object when `fabricengine.RefusalOf` matches).

The two call sites are NOT symmetric, and getting this wrong would make half the feature invisible:

- `runCleanupWithFlags` (`internal/fabriccli/fabric.go:762`) passes `map[string]any{"entries": r.Entries}` — the entries marshal from the struct, so new **`CleanupBranchEntry`** fields appear in the JSON automatically.
  This does NOT extend to a verb-level field sitting beside `Entries`: `CleanupResult.RemoteSkippedReason` is never marshalled, because `CleanupResult` itself is not.
  The map gains `"remote_skipped_reason": r.RemoteSkippedReason` explicitly.
- `runRemoveWithFlag` (`internal/fabriccli/fabric.go:786`, envelope at `:810`) hand-builds `map[string]any{"slug", "path", "links_removed"}` and never marshals `RemoveResult` at all.
  Every new field is invisible unless the map gains it explicitly: add `"remote_branch_deleted": r.RemoteBranchDeleted`, `"remote_branch_error": r.RemoteBranchError`, and `"remote_skipped_reason": r.RemoteSkippedReason`, matching the struct tags.
  The map always emits its three existing keys unconditionally, so emit these three the same way rather than conditionally, whatever the struct's `omitempty` says.

The general rule to carry into the plan: `okWithRecord` and `errWithRecordFields` emit the caller's hand-built map plus their own reserved keys (`envelope.go:29-33, 54-68`) — a field is in the envelope because the map names it, never because a result struct declares it.
The only reason cleanup's per-entry fields come free is that `Entries` is itself a map value.

Both sites also change which helper they exit through, per remote-failure-is-a-cli-failure, and both keep `"remote_skipped_reason"` in the map on either exit — the skip reason is equally worth reporting alongside a per-branch failure elsewhere in the same sweep.

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
  The shipped invariant text names no primitive count and no executor, so `CONSTRAINTS.md` itself needs no edit for this task — the counts that move live in `destroy.go`'s own header, per the Scope bullet above.
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
- **Live-Substrate Spawn Observability** — satisfied with no new work, and the reason is already recorded rather than needing rediscovery.
  `cmd/lyx/spawnobservability_test.go:77` allowlists `internal/gitexec/gitexec.go` as an exemption INSIDE the rule, with its written reason: `internal/logger` imports `internal/lyxcwd`, which imports `internal/gitexec`, so importing `logger` there would close an import cycle.
  Every git spawn in this task flows through `gitexec` exactly as `deleteBranch`'s own `git branch -D` and every `gitrepo` push already do, and `internal/gitrepo` spawns nothing directly.
  So: no `logger` call is added, and no allowlist entry is added either.
- **Never Force-Add Invariant** and the **Documentation Lifecycle** apply as always.

Plus the project rule from `CLAUDE.md`: a task changing observable CLI behaviour updates its docs in the same commit, and `manifest/roadmap.md` moves on completing a planned item.

## Testing

**`internal/gitrepo` — TDD candidate, and the clearest one.**
`DeleteRemoteBranch` is a small pure-ish function over git's behaviour with three outcomes worth pinning, so write its tests first:

- deleting a branch that exists on the remote returns `(true, nil)` and the ref is gone from the remote afterwards;
- deleting a branch absent from the remote returns `(false, nil)` — the idempotence contract, and the one most likely to regress if git's stderr wording is matched too narrowly;
- a genuine failure (unreachable remote) returns a wrapped non-nil error and `deleted == false`.

These spawn git, so they live in an `integration`-tagged file against a local bare repo acting as `origin`.
The absent-ref case asserts against a real `git push --delete` of a ref that is not there, so the test observes git's actual stderr rather than a fixture — that is what makes it a tripwire on the single pinned substring `remote ref does not exist` (per absent-remote-ref-is-success) if a future git reworded it.
One case, one substring; there is no second spelling to cover.

**`internal/fabricengine` gate tests — untagged, TDD candidates, and DIRECT-CALL only.**
These construct a `remoteBranchRequest` and call `checkRemoteBranchRequest`/`deleteRemoteBranch` directly.
They are not end-to-end guarantees and must not be written or described as if they were: per gate-runs-unchanged-for-remote, the dirtiness probe cannot refuse at either real call site, because the local deletion has already removed the branch by the time the remote executor runs.
What these tests pin is the executor's own shape — that it cannot be reached with an undeclared predicate, and that a future call site arriving without a preceding local deletion is still gated.
Mirror the existing `branchRequest` shape tests (`destroy_test.go`, `destroy_toctou_test.go`, `refusalof_test.go`).

**Which bullet goes in which tier is not a judgment call — it follows from whether the assertion reaches git.**
`destroy_test.go:540-563` already covers exactly the two zero-declaration refusals and no more, and its `BranchZeroDirtiness` case carries the comment recording why: a hand-built `Location` is safe there only because the zero-value declaration refuses before `resolveManagedBranch` runs, so `primaryWeftBranch` (which spawns git) is never reached.
`resolveManagedBranch` (`destroy.go:566-583`) short-circuits on the naming predicate, then spawns git for `primaryWeftBranch` and again for `listWeftBranches`.

**Untagged** (`destroy_test.go`), reaching no git:

- an unset ownership kind is refused with `CheckOwnership` — refused before ownership resolves;
- an unset dirtiness kind is refused with `CheckDirtiness` — refused before ownership resolves;
- a branch whose NAME fabric's scheme does not construct is refused with `CheckOwnership` — `resolveManagedBranch` returns on the name check, ahead of its first spawn.

**`integration`-tagged**, because each spawns git and Test Tier Purity bans that in an untagged file:

- the primary weft branch is refused with `CheckOwnership` — reaches `primaryWeftBranch`;
- a branch still checked out at a worktree is refused with `CheckDirtiness` — reaches `listWeftBranches`.
  Both are direct-call tests, and each should carry a comment saying so: neither is reachable from `Cleanup`/`Remove`, for the reason above.

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
The help-tree and envelope tests already in `fabriccli` will need the new flag registered.
Scenarios:

- `--remote` alone (without `--apply`) on cleanup performs no deletion — the flag-matrix corner an operator is most likely to get wrong;
- `lyx fabric remove --remote`'s success envelope carries `remote_branch_deleted` — the regression guard for the hand-built fields map, which would otherwise drop the field silently;
- a cleanup run where one entry carries `RemoteError` exits non-zero, emits `"ok":false` and `"partial":true`, and still carries the full `entries` array — the remote-failure-is-a-cli-failure contract;
- a cleanup run whose entries are all `Protected` (primary weft, checked out, or unmanaged) and carry no `Error` at all still exits 0 — the guard that protection is a report, not a failure.
  Note this is `Protected` with an EMPTY `Error`: `Cleanup` never sets `Error` on a protected or unmanaged entry, so a test asserting "protected entry with a non-empty `Error`" would be asserting a state the verb cannot produce.
- a cleanup run where one entry carries a local `Error` (the `git branch -D` itself failed) exits non-zero — the new behaviour from remote-failure-is-a-cli-failure's second bullet, and the one existing-path change this task makes deliberately;
- a `prune` run with a `Protected` or `Unowned` entry still exits 0 — the regression guard that `prune`'s carve-out survived untouched while `cleanup` left it.
  `internal/fabriccli/envelopecontract_integration_test.go` is the existing model for all of these, since it covers exactly the `runReconcile` defect this decision follows.

**The no-origin path.**
A weft repo with no `origin` configured, run under `cleanup --apply --remote`: every orphan branch is deleted locally, no remote attempt is made, the reason appears exactly once in `remote_skipped_reason` rather than N times across `entries[].remote_error`, and the verb exits 0.
The same condition under `remove --remote` must exit 0 too, with the reason in the same key — cover both verbs, since an asymmetric exit code for one configuration state is the defect this decision exists to prevent.
Cheap to fixture — a hub whose weft repo has its remote removed — and the case most likely to regress into N-duplicated errors.

**Enforcement ledgers.**
These are closed, named tables that fail loud on an unregistered addition.
Each needs a specific row, and none of them is a search-and-discover:

- `cmd/lyx/destructiveguard_test.go:79` — `destructiveGuardBannedTokens`, a raw-substring ban list applied to every non-test `.go` file under `destructiveGuardScanPackages` (`:71`, currently `internal/fabricengine` alone) except those on `destructiveGuardAllowlist` (`:94`).
  Today it bans `"branch", "-D"` among seven other tokens.
  The sixth primitive gets **no chokepoint enforcement at all** unless a token for the remote deletion is added — add the call form the executor uses (e.g. `.DeleteRemoteBranch(`), so any `fabricengine` file other than `destroy.go` naming it fails the guard.
  `internal/fabricengine/destroy.go` is already on the allowlist with the invariant-required reason, so no allowlist row changes.
  Note the scan covers `internal/fabricengine` only, so the new `gitrepo` method itself is out of the guard's reach by design — the ban exists to stop a second caller appearing inside `fabricengine`.
- `cmd/lyx/destructiveguard_test.go:131` — `destructiveGuardRecordingExecutors`, one row per `destroy.go` executor that must take a leading `rec *Mutations`, each paired with the exact declaration-line prefix the test greps for.
  Add `{"deleteRemoteBranch", "func deleteRemoteBranch(rec *Mutations, "}` (matching the executor's real name), bringing the table from 8 rows to 9.
  `destructiveGuardRecordingExecutorsMin` (`:149`) is a vacuous-scan floor of 5 and does not need to move.
- `internal/fabricengine/livestate_mutationoracle_test.go:32` — `manifestObservableKind`, a closed `map[fabricengine.Kind]bool` whose own comment states that declaring every kind, rather than testing list membership, is what makes a newly added `Kind` fail loud via `participatesInCommission`.
  Add `fabricengine.KindRemoteBranchDeleted: false`, grouped with the other git-state kinds (`KindBranchDeleted`, `KindBranchPushed`, …) — a remote ref is not something the filesystem `CaptureManifest` records, exactly as local branch existence already is not.

**Other enforcement tests to expect.**
`TestEnforcement_FabricVocabulary` fails if the `gitrepo` addition uses weft/warp vocabulary in identifiers, strings, or comments.
`TestMutationRecord_FabricengineProductionSource` is the test that consumes `destructiveGuardRecordingExecutors`.
The `fabriccli` help-tree tests see the two new `--remote` flags.

## Q&A log

- **Q:** Delete the remote branch with `git push origin --delete` or with GitHub's REST `DeleteRef` via `internal/githubclient`? **A:** [auto-pick] `git push --delete` through a new `gitrepo` method. **Why:** works against any remote, not just GitHub; reuses the credential path fabric already uses; the gitrepo Client Boundary Invariant already assigns remote-authenticating ops to `gitexec`; the REST route would add owner/repo parsing and a GitHub-only assumption for no gain.
- **Q:** Does the remote deletion get its own gated executor in `destroy.go`, or can it be a plain call from `cleanup.go`? **A:** [auto-pick] Its own gated executor in `destroy.go`. **Why:** the Fabric Destruction Chokepoint Invariant permits a destructive primitive in no other file, and deleting a ref on a shared remote is the most destructive primitive in the set.
- **Q:** Extend `branchRequest` with a `remote` field, or add a separate `remoteBranchRequest`? **A:** [auto-pick] A separate `remoteBranchRequest`. **Why:** `branchRequest`'s doc comment explicitly rejects per-site empty-string sentinels for structurally-N/A fields; a `remote` that must be `""` at four existing call sites is exactly that shape.
- **Q:** Is `--remote` opt-in or on by default? **A:** [auto-pick] Opt-in, default false, independent of `--force`. **Why:** irreversible and visible to every other clone; `--force` answers dirtiness only by invariant and must not acquire a second meaning.
- **Q:** What happens when the remote ref does not exist? **A:** [auto-pick] Idempotent success — `(false, nil)`, no mutation, no error. **Why:** every `destroy.go` executor is idempotent for an absent target, and a never-pushed weft branch is the common case.
- **Q:** Does a failed remote deletion fail the enclosing verb? **A:** [auto-pick] No — recorded per entry, verb still succeeds. **Why:** matches `PushAnchored`'s offline-laptop reasoning and `Cleanup`'s existing per-entry `Error` posture; the local deletion genuinely succeeded.
- **Q:** Local delete first or remote delete first? **A:** [auto-pick] Local first, remote only if local succeeded. **Why:** the gate's ownership and dirtiness answers come from local state; a branch the gate protects locally must not lose its remote copy, and the reverse order is unrecoverable.
- **Q:** Does this task also delete the warp branch, locally or on the remote? **A:** [auto-pick] No — out of scope. **Why:** `Remove` does not delete the warp branch locally today, so warp deletion is a new capability rather than the missing remote half; the roadmap item names `Cleanup` and `alsoDeleteBranch`, both of which act on weft branches.
- **Q:** Do the rollback call sites (`rollbackAdd`, `rollbackSwitch`) get remote deletion too? **A:** [auto-pick, reason corrected r5] No — all pass `remote: false`. **Why:** not because the branch was never pushed (false for `rollbackAdd`, which runs after `Add` has pushed both sides), but because an unattended best-effort rollback on an error path must not make a network-visible destructive change the operator never asked for.
- **Q:** Is the remote name configurable? **A:** [auto-pick] No — hardcoded `origin`. **Why:** `clone.go` already hardcodes `origin` throughout fabric's weft binding; a lone configurable point here would be inconsistent and untested.
- **Q:** Should a cleanup dry run probe the remote to report which remote copies exist? **A:** [auto-pick] No probe, no new dry-run field. **Why:** one network round trip per orphan for information that changes no operator decision; the dry run is currently offline-safe and should stay so.
- **Q:** New mutation kind, or reuse `KindBranchDeleted`? **A:** [auto-pick] New `KindRemoteBranchDeleted`, appended via `AppendRef`. **Why:** a consumer must be able to tell the recoverable local deletion from the irreversible remote one.
- **Q:** How is the remote outcome reported when local succeeds and remote fails? **A:** [auto-pick] Dedicated `RemoteDeleted`/`RemoteError` fields on `CleanupBranchEntry`, and the `Remove` equivalents on `RemoveResult`. **Why:** the two outcomes are independent, so overloading the existing `Error` would make the envelope ambiguous about which half failed.
- **Q:** Does a remote-deletion failure exit 0 with the reason in a field, or exit non-zero? **A:** [r2 review] Non-zero, via `errWithRecordFields`, while the engine verb still returns nil. **Why:** `fabricengine/doc.go:560-572` requires a per-item failure to reach the caller as a failure, and carves `prune`/`cleanup` out only for *designed refusals*; a rejected push is not one. Exit 0 here would reproduce the `runReconcile` defect that doc records.
- **Q:** What happens on a weft repo with no `origin` remote — N identical errors, or something else? **A:** [r2 review] Resolved once per verb with `gitrepo.RemoteURL("origin")` (a free local config read), reported once, no per-entry `RemoteError`. **Why:** it is a configuration state, not a per-branch outcome; folding it into the per-branch path would emit N copies of one fact and, under the exit-code decision above, fail the whole verb for it.
- **Q:** Which field carries the once-per-verb no-origin reason, and does it change the exit code? **A:** [r3 review] A dedicated verb-level `RemoteSkippedReason` on both result types; exit 0 from both verbs. **Why:** reusing `RemoteError`/`RemoteBranchError` would trip the CLI's failure branch, which would have made the identical configuration state exit 0 from `cleanup` and non-zero from `remove`.
- **Q:** `cleanup`'s existing local-delete `Error` is a genuine failure too — does it also exit non-zero, or stay at today's exit 0? **A:** [r6 review] It exits non-zero as well; `errWithRecordFields` keys on both `Error` and `RemoteError`. **Why:** `doc.go`'s carve-out rests on the entry `Error` being a designed refusal, which is true of `prune` but never was of `cleanup` (it sets only `Protected` on those arms); shipping the asymmetry would mean two verdicts for the same class of failure in the same struct. `cleanup` leaves the carve-out; `prune` stays in it.
- **Q:** How is a gate refusal escaping the remote executor told apart from a push failure? **A:** [r6 review] The call site inspects the error with `RefusalOf` and prefixes `RemoteError` accordingly. **Why:** ownership resolution spawns git twice, so a local spawn failure could otherwise be reported as "remote branch deletion failed".
- **Q:** What error does the CLI synthesise for the failure exit, given the engine returns nil? **A:** [r4 review] A `fmt.Errorf` summary — cleanup names the failed/attempted counts and the branch names and points at `entries[].remote_error`; remove, having one branch, inlines its reason. No `refusal` key, since `RefusalOf` cannot match a synthesised error. **Why:** `errWithRecordFields` dereferences `err.Error()`, so leaving it unnamed would have the plan writer inventing observable CLI output.
- **Q:** Does the no-origin pre-check run on a dry run? **A:** [r4 review] Yes, and it populates `remote_skipped_reason` there. **Why:** `RemoteURL` is a local config read, so the dry run stays offline-safe, and telling the operator up front is the whole value of a dry run.
- **Q:** Does the reused dirtiness check actually protect the two real call sites? **A:** [r3 review] No — it is vacuous as sequenced, and the discussion now says so. **Why:** the remote executor runs only after the local `git branch -D` succeeded, so the branch is already gone from the local list the probe reads; the protection comes from local-first ordering, and the reused check is defence-in-depth plus request-shape enforcement for any future direct caller.
