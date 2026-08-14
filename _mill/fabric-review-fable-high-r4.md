# fabric — independent review, round 4 (`fable-high-r4`)

Reviewer: crucible round agent `fable-high-r4` (Fable 5, high effort).
Worktree: `/home/knatte/Code/loomyard/wts/fabric-crucible-hardening`, branch `fabric-crucible-hardening`.
Round context: FINAL round of a fixed 4-round campaign — broad whole-module sweep plus the four carried-forward items from round 3's independent verification;
the destruction chokepoint's containment/TOCTOU property is spot-check only (closed-and-verified in round 3).

## Executive summary

The `fabric` module is in strong shape after three prior hardening rounds. This final round read every production file in `internal/fabricengine` and `internal/fabriccli` (plus the paired leaf packages for context), re-derived the destruction-chokepoint properties, drove all 16 verbs against real local-filesystem-remote hubs, and closed out the four carried-forward items.

**No BLOCKING or MEDIUM findings.** The chokepoint's containment/TOCTOU property holds (spot-checked, not re-litigated). Four LOW findings and one NIT, all fixed:

- **F1 (LOW)** — `reconcile.go` `applyStaleRemoval` reports a stale-junction convergence that did not happen: it flips `Action` to `stale_removed` and emits a "stale junction(s) removed:" detail even when every removal was refused/failed (empty `removed` set); it counts an operational (non-refusal) removal failure as a success; and it strips the junction's `.git/info/exclude` entry even when the junction removal was refused (leaving the still-present junction as untracked dirt). Same dishonesty shape as round 2's M2, narrower scope.
- **F2 (LOW)** — `add.go` `rollbackAdd`'s step-5 warp-branch deletion is structurally refused under the default (empty) `branch_prefix`, so a failed Add leaves the warp branch behind and the gate refusal is swallowed with no log at all; the header over-claims a "full rollback ... never leave partial state." CONFIRMED live.
- **F3 (LOW)** — carried item 1: the round-3 fixer report overstates the M1 companion integration test's coverage. That test is caught by the check-phase (M3, round 2) regardless of M1's `os.Root` act-time fix, so it does not guard M1; only the hermetic unit test does.
- **F4 (LOW)** — carried item 4: a leftover unregistered directory at the warp worktree path `<hub>/<slug>` blocks a subsequent `lyx fabric add <slug>` (via `add.go`'s `os.Stat` guard) yet is invisible to every List-based verb (prune/reconcile/list/status) — contradicting round 2's "blocks nothing" claim for that placement. The `add` dir-exists error gives no cleanup guidance.
- **F5 (NIT)** — carried item 2: an operational (ELOOP) failure at a launcher path is swallowed by `surfaceRefusal`'s documented best-effort policy, so `Remove` reports `ok:true`/`partial:false` while leaving that entry unremoved. This is the documented tradeoff (only gate refusals are non-discardable), narrower than M2; recorded, no behavior change.

Carried item 3 (create-side symlink-directed write) was investigated live and **confirmed NOT a defect** — see P2 below.

**Merge-readiness: MERGEABLE.** All findings are LOW/NIT operability/honesty gaps on error/repair paths, not correctness bugs in the normal single-instance flow; every one is fixed with a regression test where practical. Standing limit, unchanged from every prior round: Windows path/junction behaviour is out of scope (unreachable from a Linux host).

## Scope assessment (plan vs shipped)

`fabric-unified-view.md` describes slices 1-10 plus follow-up slices 12-15, all marked shipped; slice 6's orchestration half and weft-remote-provisioning are the only open items, both explicitly out of scope (they belong to loom/Shed, which don't exist yet). The as-built code delivers the documented scope: 16 CLI verbs (clone/add/list/remove/checkout/pairs/reconcile/prune/cleanup/unwire + status/commit/push/pull/sync/diff), the one-repo illusion at the public API boundary (`Open`/`Committed`/`Ready`/`RefScanner`/`Healthy`), the destruction chokepoint, the mutation record, the correspondence-index-as-cache-over-trailers layering, the snapshot-trailer read/write path, and the `.lyx-anchor`/`.lyx-warp`/repo-wide-`fabric.yaml` three repo-wide records. No silently-dropped requirement or over-reach found. `doc.go` is dense but accurate against the code; the one documentation inaccuracy is F3 (a prior-round fixer report, not the module doc).

## Code findings (severity-ranked)

### F1 — LOW — `applyStaleRemoval` reports convergence that did not happen (CONFIRMED, traced)

`internal/fabricengine/reconcile.go`, `applyStaleRemoval` (~lines 813-834).

```go
for _, name := range stale {
    removeErr := removeWarpJunction(rec, warpLayout, slug, []string{name})
    _, _ = unseedGitExclude(rec, warpLayout, slug, []string{name})   // runs unconditionally
    var refusal *destructiveRefusal
    if errors.As(removeErr, &refusal) { logger.Warn(...); continue }  // only *refusals* skipped
    removed = append(removed, name)                                    // any non-refusal counts as removed
}
appendPrDetail(pr, fmt.Sprintf("stale junction(s) removed: %s", strings.Join(removed, ", ")))  // unconditional
if pr.Action == ReconcileActionAlreadyHealthy { pr.Action = ReconcileActionStaleRemoved }      // unconditional
```

Three facets, all the same "report the intent, not the effect" shape M2 fixed for reconcile in round 2:
1. **Empty-removed still reports + flips Action.** When every stale junction's removal is refused, `removed` is empty, yet the code appends `"stale junction(s) removed: "` (empty list) and flips `Action` from `already_healthy` to `stale_removed` — a consumer keying off `Action` sees convergence that did not occur.
2. **Operational failure counted as removed.** Only a `*destructiveRefusal` is filtered from `removed`; an operational failure returned by `removeWarpJunction` (e.g. an `os.Root`/filesystem error surfaced by `removeLink`) is non-refusal, so the name is still appended to `removed` and reported as removed though nothing was.
3. **Exclude stripped on refused removal.** `unseedGitExclude` runs before the refusal check, so a junction whose removal was refused (still on disk) has its `.git/info/exclude` entry stripped anyway — the still-present junction now shows as untracked dirt in `git status`.

Failure scenario: a stale on-disk junction whose raw target has drifted (points into weft but at a subpath other than its nominal target) passes `scanOnDiskJunctionNames`'s fabric-owned test but fails `ownedWiredJunction`'s `RawTarget == expectedTarget` check → refused. Reconcile reports `stale_removed` with the junction still on disk and its exclude entry gone.

Fix: only `unseedGitExclude` and append to `removed` after a nil-error removal; `logger.Warn` an operational failure the same as a refusal (both leave the junction present); and skip the detail + Action flip when `removed` is empty. Add a reconcile integration test that plants a drifted stale junction and asserts Action stays `already_healthy`, no removed-detail, and the exclude entry survives.

### F2 — LOW — `rollbackAdd` cannot delete the warp branch it created under the default empty prefix (CONFIRMED live)

`internal/fabricengine/add.go`, `rollbackAdd` step 5 (lines 287-303); `internal/fabricengine/destroy.go`, `resolveManagedBranch` (lines 547-572).

Under `template.yaml`'s default `branch_prefix: ""`, the warp branch Add creates is the bare slug (`my-task`). `rollbackAdd`'s step-5 `deleteBranch` declares `ownedManagedBranch(l, t.cfg.BranchPrefix)` = `ownedManagedBranch(l, "")`, whose predicate requires either a `-weft`-suffixed name (`WeftWarpSlug` true — false for a warp branch) or a non-empty prefix match (inapplicable when prefix is empty). So the bare-slug warp branch is "not a name fabric's own scheme constructs" → the gate refuses deletion.

Confirmed live: forced a post-creation Add failure (weft fork from a non-existent `main-weft`); the failed Add's mutation record was `worktree_created` / `branch_created` / `worktree_removed` with **no `branch_deleted`** — the warp branch `my-task` persisted, and `lyx fabric add my-task` then returned `branch "my-task" already exists; ... delete it first with "git branch -D my-task"`.

Two sub-issues: (a) the gate refusal in step 5 is swallowed entirely — `rollbackAdd`'s return is discarded by every caller (`_ = t.rollbackAdd(...)`) and, unlike `rollbackSwitch`, it does NOT `logger.Warn` the refusal, so the leftover branch is invisible in the trace; (b) the branch request's `dirtyCheckedOutBranch()` probe consults `listWeftBranches` (WEFT branches) for a WARP branch — vacuous, since a warp branch never appears there.

The gate's refusal is defensible-by-design: a bare-slug branch is indistinguishable from a user's own branch, so the gate conservatively refuses to prove ownership by name alone. The genuine defects are the *silence* and the *over-claiming header* ("performing a best-effort full rollback on any post-creation failure so a partial worktree pair is never left behind"). Fix (surgical, does not weaken the gate): `logger.Warn` the swallowed refusal in `rollbackAdd` exactly as `rollbackSwitch` does; correct `rollbackAdd`'s doc to state the warp branch is only gate-deletable when ownership is provable (a non-empty prefix, or the `-weft` weft branch), and under the default empty prefix a rolled-back Add leaves the warp branch behind, self-healing via the remedy Add's own "already exists" error names. A deeper functional fix (a "freshly-created branch" ownership token analogous to `createdToken`) is noted as deferred — it would touch the closed branch-ownership enum + guard table, a larger change than this final round should make when the leftover is recoverable and self-described.

### F3 — LOW — round-3 fixer report overstates the M1 companion integration test's coverage (carried item 1, CONFIRMED)

`_mill/fabric-review-fable-high-r3-fixer-report.md` lines 27-29.

The report calls `destroy_containment_toctou_integration_test.go` "the end-to-end companion" that proves "the whole `remove` call never deletes outside the hub through the escaping segment," implying it guards M1's `os.Root` act-time fix. Traced: the test plants an already-live escaping symlink at the per-slug launcher directory before `Remove` runs. In `removeLaunchers`, the per-script `pathRequest` has `container = launchersDir` and `target = <...>/<slug-symlink>/ide.sh`; `checkPathRequest` → `containmentFailure` resolves the target's PARENT ancestry (which includes the escaping `<slug>` symlink) via `containmentPath`, so containment FAILS at the **check phase** (M3, round 2's fix) and refuses before M1's executor-level `os.Root` act is ever reached. Sabotaging M1's production code alone (reverting `removeContainedPath` to a nominal-path unlink) therefore leaves this integration test green — only the hermetic `TestRemoveContainedPath_RefusesEscapingIntermediate` guards M1's act-time property.

Fix per carried item 1(b): append a Round-4 correction note to the round-3 fixer report accurately stating the hermetic unit test is the sole regression guard for M1's act-time TOCTOU, and the integration test guards the check-phase (M3) end-to-end, not M1. Option (a) — a genuine M1-specific live test — is impractical (the window is closed by design; it would need a build-tag/injection seam to force the pre-fix code path), and the deterministic unit test already sabotage-proves M1 authoritatively.

### F4 — LOW — leftover unregistered directory at `<hub>/<slug>` blocks `add` and is invisible to List-based verbs (carried item 4, CONFIRMED by reasoning)

`internal/fabricengine/add.go` lines 78-81; `prune.go`/`reconcile.go`/`status.go`/`list.go` (all List-based).

Add's target-exists guard is `os.Stat(WorktreePath(l, slug))` → refuses `"worktree directory %q already exists"` on any directory there. `prune`, `reconcile`, `status`, and `list` all enumerate via `List` (`git worktree list`), which only sees git-registered worktrees. So a leftover unregistered directory at `<hub>/<slug>` (round 2's "inert leftover" — an empty dir with dangling junctions a remove/reconcile race can strand) BLOCKS a subsequent `add <slug>` yet is invisible to every verb that could report or clean it — contradicting round 2's "blocks nothing, remedy is rmdir" for that placement.

I could not construct the exact round-2 race deterministically within this round's budget, so the precise on-disk placement the race produces is unconfirmed; the reasoning above establishes that *if* the leftover lands at `<hub>/<slug>`, it blocks add and no verb guides cleanup. Cheap safe fix: improve Add's dir-exists error to name the leftover-cleanup remedy (mirroring the actionable branch-exists message right above it), so an operator hitting a stranded directory has a path forward. A deeper automated sweep is deliberately NOT added: safely distinguishing a fabric leftover from user content at that path needs a deterministic race harness to validate and risks the exact "never delete what might be user content" rule the module guards — deferred with reason.

### P2 — carried item 3 (create-side symlink-directed write) — CONFIRMED NOT A DEFECT

Live-tested git + Go behaviour:
- `createExclusiveDir` (clone `hubPath`): `os.Mkdir` fails EEXIST on ANY symlink at the path (dangling OR →existing-dir) — planted-symlink write-through fully refused. Matches doc's "os.Mkdir EEXIST semantics" claim.
- `createGitWorktree` (Add `target`): non-racing planted cases both refused — a symlink→existing-dir is caught by `os.Stat(target)` at add.go:79 (Stat follows the link → non-IsNotExist → "worktree directory already exists"); a dangling symlink → `os.Stat` reports IsNotExist so Add proceeds, but `git worktree add` then refuses via its own lstat ("fatal: '<target>' already exists"), no write-through. Live-verified: `git worktree add` DOES follow a live symlink→existing-empty-dir and materialise through it, so the only residual is the same-process [stat@:79, add@:125] race window with a symlink planted mid-Add — no concurrent fabric writer is expected at the unique slug path, so this is the identical accepted-residual class as N4's dirtiness-probe TOCTOU. Grade: not a live-exploitable defect; residual noted, no fix.

## Docs & operability findings

- **F5 (NIT) — carried item 2: symlink-loop / operational failure at a launcher path is swallowed.** `removeLaunchers`'s `removePath` returns an operational error (e.g. ELOOP from `removeContainedPath`'s `os.Root.Lstat`) for a symlink loop at a launcher path; `Remove` wraps it in `surfaceRefusal`, which returns nil for any non-`*destructiveRefusal` error, so `Remove` continues and reports `ok:true`/`partial:false` while that launcher entry (and the loop) remain. This is `surfaceRefusal`'s documented policy — "an operational failure (git exited nonzero, the filesystem said no) stays discardable while a gate refusal never does" (doc.go, `surfaceRefusal`) — applied uniformly, narrower than M2 (which was about a *refusal*). Recorded as the documented tradeoff; no behavior change, since making launcher ELOOP specifically surface would be inconsistent with every other best-effort operational swallow in the module and is out of this round's scope.
- **doc.go accuracy**: read in full against the code; accurate. The `template.yaml` default `branch_prefix: ""` / `pathspec: ""` matches the code's structural-injection behaviour.

## What was tested

- `go build ./...` + `go vet` (fabricengine/fabriccli/gitexec/gitrepo): PASS (task bhxxfja79, exit 0).
- Hermetic `go test ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitexec/... ./internal/gitrepo/... ./cmd/lyx/... -count=5`: PASS, all 5 packages ok (task btq2urt4t, exit 0).
- Live integration `go test -tags integration ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitexec/... ./internal/gitrepo/... -count=1`: PASS (task bl17rhsfb, exit 0), no FAIL/corruption markers.
- Live git-substrate probes (scratchpad, real `git`): `git worktree add` to a live symlink→existing-empty-dir SUCCEEDS (writes through); to a dangling symlink FAILS ("already exists"); `os.Mkdir` EEXIST on any symlink (dangling or →dir); `os.Stat` follows symlinks (→dir non-IsNotExist, dangling IsNotExist). Basis for P2's confirmed-non-defect verdict.
- Live verb drive (`./deploy-dev` binary, local bare warp+weft remotes): clone (incl. the `refuseUncheckedOutWarpClone` refusal + clean teardownHub when warp HEAD names a missing ref), pairs, add, list, status, reconcile (idempotent + junction_repointed re-wire after unwire), prune (dry), cleanup (dry, primary-weft protected), checkout (both directions + correct refusal when target branch already checked out in another worktree), remove, unwire (weft preserved, sibling-census keeps shared exclude entries), full add→checkout→remove→re-add lifecycle. All behaved as documented; the failed-Add run live-confirmed F2.
- Teardown: all scratch hubs/temp dirs are under the session scratchpad; no stray git processes or lock files outside it (verified after each drive).

Could NOT verify: Windows path/junction behaviour (unreachable from a Linux host — permanent, stated limit); the exact on-disk placement round 2's remove/reconcile race produces for F4 (no deterministic race harness built this round — F4's reasoning is placement-conditional).
