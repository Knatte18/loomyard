# Discussion: fabric: one ownership-and-dirtiness gate for all destruction (slice 12)

```yaml
task: 'fabric: one ownership-and-dirtiness gate for all destruction (slice 12)'
slug: fabric-destructive-chokepoint
status: discussing
parent: main
```

## Problem

The fabric v2 crucible campaign ran six serial model-rotating review rounds against the slice 1-10 rewrite and produced 81 findings, 9 BLOCKING, **8 of them data-loss**.
Every individual defect is fixed and merged.
What is not fixed is that the eight are **one shape, not eight mistakes**: a destructive operation acting on a path it does not own, or without checking whether there is uncommitted work to lose.
They appeared in six different files across five different rounds.

The structural cause is that fabric has **no common chokepoint for destruction**.
Every call site implements its own ownership check and its own dirtiness check, or forgets to.
R4 enumerated 28 destructive sites and closed the two live defects in that enumeration;
R5 then found two more in regions R4's sweep did not cover — `remove`'s slug door and the shared `.git/info/exclude`.
That is the evidence that enumerate-and-fix does not terminate here.

**Why now:** this is the root-cause slice and it goes first, ahead of the live-state harness (slice 13) and the result envelope (slice 14), both of which now depend on it.
Slice 13's cells assert on *refusal* behaviour — that a verb refuses instead of destroying, and which of the four checks refused — and this slice is precisely what changes that behaviour and those messages, so cells written before the gate would be rewritten after it.
Slice 14 rewrites every verb's result path including the destructive ones, so landing the gate first means that work happens on already-gated code.

The full task specification is `manifest/designs/fabric-crucible-followups.md#slice-12--route-every-destructive-operation-through-one-ownership-and-dirtiness-gate`.
Three design points are pinned there and are not relitigated: one shared pre-flight in front of five distinct primitives (never one delete function);
the gate **executes** rather than approves;
one file in `internal/fabricengine`, not a sub-package.

### The eight data-loss defects

| round | verb | what it destroyed |
|---|---|---|
| R1 | `reconcile` | a tracked symlink in the warp worktree |
| R2 | `pull` | uncommitted tracked warp work, via `ResetHard` on every advance path — and returned `ok:true` |
| R3 | `remove` | the warp worktree directory, via an `os.RemoveAll` fallback past a git refusal |
| R3 | `cleanup` | the primary weft branch |
| R3 (orchestrator, not the round) | `prune` | a path git had just refused to remove |
| R4 | `prune` | any hub child whose name ends in the weft suffix — an ordinary user directory, or an unrelated git clone parked there |
| R4 | `clone --reset` | any directory named `<derived>-HUB`, hub or not, on a name fabric *derives* rather than one the operator types |
| R5 | `remove ..` | the entire hub — warp clone, weft clone, `_board`, every pair, all uncommitted work — then reported `{"error":"failed to check worktree status","ok":false}` |

## Scope

**In:**

- A new `internal/fabricengine/destroy.go`: one `gate(req) error` enforcing containment → ownership → dirtiness → force in a fixed order, plus one executor per destructive primitive that calls the gate and then acts.
- A new `internal/fabricengine/dirtiness.go`: the single `git status --porcelain` implementation, replacing **eight** hand-rolled probes (`add.go`, `checkout.go`, `prune.go`, `pull.go`, `remove.go` ×2, `warpclean.go`, `reconcile.go`).
- Conversion of every destructive call site in `internal/fabricengine` onto the gate's executors, across the five primitives:
  `os.RemoveAll`/`os.Remove`, `git worktree remove [--force]`, `git branch -D`, `ResetHard`, and slug-derived `fslink.Remove` (link removal).
- Closing three gaps found during exploration that the manifest does not name (see Technical context): `removeJunctionRecords` containment, `teardownHub` ownership, and `removeWarpWorktreeDir`'s un-regated `os.RemoveAll` fallback.
- R6's validation-asymmetry class, folded in here rather than fixed twice: the gate validates slug-derived targets via `validateWorktreeSlug`, so `Prune`'s and `Reconcile`'s **derived** slugs are validated for the first time.
- A typed `*destructiveRefusal` error carrying which of the four checks refused, wrapped by each verb into its existing error shape.
- A bypass guard test at `cmd/lyx/destructiveguard_test.go`.
- A short imperative `CONSTRAINTS.md` invariant, in the same commit.
- Docs: `internal/fabricengine/doc.go`, `manifest/designs/fabric-crucible-followups.md`, `manifest/roadmap.md`.

**Out:**

- **A sub-package.** `internal/fabricengine/destroy` or a lower leaf beside `internal/fslink` is not built. The predicates the gate needs (`isRegisteredLinkedWorktree`, `looksLikeHub`, `applyStalePairOwnership`, weft path construction) are `fabricengine`-private, so a sub-package importing `fabricengine` for them while `fabricengine` imports the sub-package for the gate is an import cycle Go forbids. Extract to a told-everything leaf later if a non-fabric caller appears.
- **Promoting the dirtiness probe into `internal/gitrepo`.** See Decisions.
- **`gitexclude.go`'s exclude rewrite.** R5 already consolidated every read-modify-write of `.git/info/exclude` into one file behind a repo-wide flock and an atomic same-directory rename. It is already a chokepoint of its own shape; its `os.Remove` calls only unlink a temp file the same function created three lines earlier.
- **The result envelope.** Step 5 (honest reporting) lands in each verb's existing error shape. Generalising it into one accumulate-as-you-mutate envelope is slice 14.
- **The live-state harness, `fabrictest`, and the hub factory.** Slice 13.
- **`docs/overview.md`.** No new module, no change to the module table or the execution stack.
- **A shared framework for the repo's ~15 hand-rolled static-analysis guards** (issue #135). Noted, not built here.
- **Behaviour changes.** This is a consolidating refactor. Every current site keeps its current dirtiness scope. Any existing named test that requires an edit is a behaviour change to be flagged, not silently edited.

## Decisions

### gate-call-shape

- Decision: a typed request struct plus one executor per primitive, all in `internal/fabricengine/destroy.go`.
  `destructiveRequest{what, container, target, slug, ownership, dirtiness, force}`, with executors `removeDir(req)`, `removeGitWorktree(req, repoDir)`, `deleteBranch(req, repoDir, branch)`, `resetHardTo(req, repo, sha)`, and a link-removal executor.
  One `gate(req) error` runs checks 1-4 in fixed order;
  each executor calls it first, then performs the act.
- Rationale: every field is required, so an omitted check is a compile error rather than a forgotten one.
  "Which checks did this site declare" stays greppable at each call site.
  The gate executing rather than approving is what makes a raw `os.RemoveAll` outside one file mechanically bannable, which reduces the bypass guard to a trivial file-scoped scan.
- Rejected: free functions with positional args per primitive (a new check means changing every signature, and declarations stop being greppable);
  a `destroyer` value constructed per verb (hides which checks are in play behind the constructor).

### dirtiness-scope-is-caller-declared

- Decision: dirtiness scope is a closed two-value enum the caller declares, with a documented reason at each site: `dirtyScopeTracked`, `dirtyScopeAll`, or `dirtinessNA(reason)`.
  One implementation, one fixed order;
  the scope is the call site's declared choice.
  Every current site keeps its current scope.
- Rationale: scope cannot be derived from the primitive alone.
  `prune.go:197-205` documents its tracked-only probe as deliberate — refusing on untracked files would protect nothing while making prune useless on exactly the debris it exists to clear.
  Meanwhile `remove.go:61` deliberately probes untracked-inclusive.
  Preserving each site's current scope is what makes this a refactor the campaign's ~29 existing integration files can police.
- Rejected: fixed per primitive from what that primitive discards (breaks `Prune`);
  globally tracked-only per the manifest's stated inheritance (opens the `Remove` hole below).

### remove-keeps-untracked-inclusive-and-the-fallback-is-regated

- Decision: `Remove`'s probe stays untracked-inclusive, **and** the `os.RemoveAll` fallback inside `removeWarpWorktreeDir` (`remove.go:197`) is itself routed through the gate with `dirtyScopeAll`.
- Rationale: this is a live hazard the manifest does not name.
  The fallback currently fires on *any* nonzero exit from `git worktree remove` for a registered linked worktree.
  `git worktree remove` without `--force` refuses on untracked files.
  So if the gate normalised `Remove` to tracked-only, the pre-flight would pass, git would refuse on untracked files, and the fallback would then `os.RemoveAll` them — turning a normalisation into a ninth data-loss instance.
  Re-gating the fallback closes that structurally rather than relying on the pre-flight having run first, which is the discipline this slice exists to end.
- Rejected: keeping the fallback as-is (relies on ordering discipline);
  normalising to tracked-only and accepting git's own refusal message (loses the clearer error and still needs the fallback fix).

### rollback-paths-go-through-the-gate

- Decision: in-transaction rollback and teardown paths — `rollbackAdd` (`add.go:224`), `rollbackSwitch` (`checkout.go:187`), `teardownHub` (`clone.go:604`), `add.go:265`'s `worktree remove --force` — go **through** the gate with containment and ownership enforced and dirtiness declared `dirtinessNA(reason)` per site, reason being "fabric created this path within this transaction".
- Rationale: ownership still matters on a teardown path.
  R4's `clone --reset` defect *was* a teardown path, and `teardownHub` today removes `hubPath` with no `looksLikeHub` check at all, unlike the `--reset` path 40 lines above it at `clone.go:563`.
- Rejected: bypassing the gate with a guard allowlist (re-opens the "teardown is special" reasoning that produced the `clone --reset` defect);
  passing `force: true` (makes rollback indistinguishable from an operator's `--force`, which step 4 of the gate explicitly forbids conflating).

### dirtiness-probe-stays-fabric-local

- Decision: the tracked-only dirtiness probe is **not** promoted into `internal/gitrepo`.
  It lives in `internal/fabricengine/dirtiness.go`.
- Rationale: this is the manifest's one deferred call, and the manifest states a criterion rather than a preference — promote if the gate needs it against both warp and weft through a `Repo` handle it already holds;
  keep it fabric-local if the gate only ever probes paths it resolves itself.
  Applying it: six of the eight probe sites are `Topology` verbs, and `Topology` holds only a `Config` — no `Repo` at all (`topology.go:18-20` states this explicitly, since a pair does not exist yet when `Add` runs).
  Only `pull.go`'s `warpWorktreeDirty` has a `Repo` handle.
  A `gitrepo` method would mean constructing a `Repo` per probe solely to answer one question, at six sites that have none.
  The eight hand-rolled probes still collapse to one implementation either way — that half is not optional.
- Rejected: a tracked-only variant of `gitrepo.WorktreeChangedFiles` (trivial in go-git — skip `git.Untracked` entries, no `gitexec` call, so no update to the gitrepo Client Boundary Invariant's pinned list — but it serves one consumer and forces `Repo` construction at six sites);
  both, as a fabric-local wrapper over a promoted primitive (two layers for one caller).

### dirtiness-lives-beside-the-gate-not-inside-it

- Decision: the probe lives in its own `dirtiness.go`, not in `destroy.go`.
  The gate calls it;
  read-only callers (`warpclean.go`'s exported `Clean`, used by `loomengine.Preflight`;
  `reconcile.go`'s board-status check;
  `checkout.go`'s pre-switch check) call it directly, bypassing the gate.
- Rationale: keeps the guard's allowlist honest — `destroy.go` is "the only file that destroys", not "the only file that also happens to run `git status`".
  Routing read-only callers through a destruction gate would be nonsense.
- Rejected: inside `destroy.go` and exported (muddies the one file whose whole value is that its contents are precisely the destructive surface);
  two implementations, one for the gate and one for read-only callers (that is the disease, one layer down).

### ownership-is-a-closed-enum

- Decision: ownership is a closed enum of kinds, each resolved by the gate itself:
  `ownedRegisteredLinkedWorktree(repoDir)`, `ownedFabricHub`, `ownedHubGeometryChild(container)`, `ownedInTransaction(reason)`.
  Nothing else.
  The gate calls `isRegisteredLinkedWorktreeIn` and `looksLikeHub` internally.
- Rationale: a closed enum is what makes the guard meaningful — there is no escape hatch through which a call site can supply "yes, trust me".
  Every one of the 28 sites already had the freedom to define its own check, and that freedom is what produced the class.
- Rejected: a caller-supplied predicate `func() (bool, string)` (exactly the hole the slice exists to close);
  an interface with per-kind implementations (same closure, more indirection, no added property).

### link-removal-is-gated-exclude-rewrite-is-not

- Decision: slug-derived `fslink.Remove` calls go through the gate (containment + ownership).
  `gitexclude.go` stays as-is and is allowlisted in the guard.
- Rationale: `removeWarpJunction` → `removeJunctionRecords` (`weftwiring.go:145-161`) has **no containment check at all**, unlike its siblings `removePortal` (`portals.go:54`) and `removeLaunchers` (`launchers.go:159`), and it takes a slug-derived path from both `Remove` and `rollbackAdd`.
  That is a live gap, not a hypothetical one.
  The exclude rewrite is already a chokepoint under a flock;
  the four checks say nothing useful about a lock-held atomic rewrite, so gating it would mean declaring three N/As for uniformity's own sake.
- Rejected: gating both;
  gating neither (leaves the `removeJunctionRecords` gap open).

### r6-validation-asymmetry-folds-into-the-gate

- Decision: the gate calls `validateWorktreeSlug` whenever the request carries a `slug` field.
  `Add` and `Remove` keep their existing entry-point calls.
- Rationale: the real asymmetry was never "six of the eight exported `Topology` verbs take an unvalidated slug argument" — six of them take no slug at all (`Prune`, `Cleanup`, `Reconcile`, `Status`, `List` take none; `Checkout` takes a branch).
  It is that `Prune` and `Reconcile` **derive** slugs from directory names (`filepath.Base(warpPath)` at `prune.go:92`, `WeftWarpSlug(name)` at `prune.go:131`) and feed them straight to `removePortal`, `removeLaunchers` and `WeftWorktreePath` unvalidated.
  Gate-side validation catches derived slugs;
  entry-point validation keeps an early, clear error before any work happens.
  Belt and braces, and cheap.
  Note there is no branch-name validator in the tree (`branchname.go` holds only `WeftBranchName`), so `Checkout`'s branch argument is out of scope here.
- Rejected: entry points only (fixes nothing, since `Prune`/`Reconcile` derive rather than accept);
  gate only (loses the early error, and surfaces `Add`'s "invalid slug" failure only once something is about to be destroyed).

### refusal-is-a-typed-error

- Decision: a refusal returns `*destructiveRefusal{Check, What, Target, Reason}` with `Check` an enum over containment / ownership / dirtiness / force.
  Each verb wraps it into its existing error shape (`fmt.Errorf`, `PruneEntry.Error`, `CleanupBranchEntry.Error`, …).
- Rationale: satisfies the manifest's step 5 (which may land in each verb's existing error shape) while giving slice 13's cells something to assert on — "which of the four checks refused" — and slice 14 something to generalise, without this slice building the envelope.
- Rejected: plain `fmt.Errorf` strings (slice 13 would have to string-match refusal messages, the brittlest possible coupling between two slices landing back to back);
  a typed error plus a struct field on every result type (that is slice 14's envelope, a slice early).

### bypass-guard-shape-and-home

- Decision: a new `cmd/lyx/destructiveguard_test.go`, cloning `cmd/lyx/rawgitmutation_test.go`'s machinery, scanning `internal/fabricengine` only, with a per-file allowlist carrying reasons and a vacuous-scan floor.
- Rationale: `rawgitmutation_test.go` is the repo's existing template for exactly this — token scan, `go env GOMOD` module-root resolution, `filepath.ToSlash` normalisation for Windows, per-file allowlist map with reasons, minimum-scanned-files floor, and a clean skip when the go toolchain is absent.
  Every other tree-walking guard in this repo lives in `cmd/lyx/`.
  Scope stays `fabricengine` because the class was fabric's and `os.Remove` is legitimate in a dozen other packages (`internal/logger`, `internal/configengine`, `internal/shuttleengine`, …).
  No `golangci-lint` config exists in this repo, so a forbidigo rule would mean introducing new tooling.
  Issue #135 (the repo's ~15 hand-rolled static-analysis guards) is noted in the invariant text rather than allowed to block a sixteenth walk: a shared framework for fifteen guards is its own slice, and inventing it here would make this one inconsistent with the fourteen it does not yet cover.
- Rejected: an in-package `internal/fabricengine/destroy_guard_test.go` mirroring `lspclient_guard_test.go` (sits in the package it polices);
  extending `rawgitmutation_test.go` with a second scan config (conflates two unrelated invariants in one test).

### constraints-entry-is-a-short-imperative-rule

- Decision: a new top-level `CONSTRAINTS.md` section, kept **short and imperative — rules only**.
  No rationale, no incident narrative, no historical justification;
  those go to `internal/fabricengine/doc.go`.
- Rationale: the objection "do we need a constraint for something this internal — all git operations go through fabric anyway" was raised and does not hold, for two reasons.
  First, the Fabric Git Invariant binds *which module* may run git and says nothing about what `fabricengine` does internally;
  it explicitly exempts read-only verbs, naming `git status --porcelain` — the exact probe this slice consolidates eight copies of.
  Second, **three of the five primitives are not git at all**: `os.RemoveAll`, `os.Remove`, and `fslink.Remove`.
  The `remove ..` defect that destroyed an entire hub was `os.RemoveAll`;
  R4's `clone --reset` defect was `os.RemoveAll`.
  Routing every git operation through fabric would have prevented neither, and they are two of the eight.
  A top-level section rather than a sub-bullet because filing a rule about `os.RemoveAll` and `fslink.Remove` under a *git* invariant would be the wrong parent for three of the five primitives, and because `Never Force-Add Invariant` is narrower than this and already has its own section.
  The file's own preamble says it "states rules only", which is what keeps the entry short.
- Rejected: a sub-bullet under the Fabric Git Invariant, mirroring the lspclient precedent (wrong parent for the non-git primitives);
  no `CONSTRAINTS.md` entry at all, `doc.go` plus the guard test only (loses the two parts nothing enforces — the fixed check order and force-never-answers-ownership — which today live only in a `prune.go` comment).

### one-commit-for-the-slice

- Decision: one commit covering gate, call-site conversions, probe consolidation, guard test, and all four docs.
  Rebase onto `main` and re-read `CONSTRAINTS.md` before pushing.
- Rationale: CLAUDE.md requires the docs in the same commit as cross-cutting infrastructure, and the call-site conversions are meaningless without the gate.
  `shed-model-contradiction-sweep` also edits `CONSTRAINTS.md` (its pointer-rule invariant) — a different section, so a rebase resolves cleanly, but rebase rather than assume.
- Rejected: gate + guard + docs first, then conversions (leaves a commit where the guard passes only because the allowlist is wide open);
  split by primitive (CI red for four commits, since the guard cannot pass until the last).

## Technical context

### Dependency ordering — the wiki body's trailing line is stale

The wiki task body ends with `depends_on: fabric-live-state-harness`.
**That line is stale prose from a pre-reversal draft.**
The wiki's actual `depends_on` field for this task is `[]`, and `fabric-live-state-harness` carries `depends_on: ['fabric-destructive-chokepoint']`.
This matches `manifest/roadmap.md:21` and commit `faa0fe2b` ("manifest: make the live-state harness depend on the chokepoint, not run beside it").
This task is unblocked and goes first.

### The five primitives and their current sites

| primitive | sites in `internal/fabricengine` |
|---|---|
| `os.RemoveAll` / `os.Remove` | `remove.go:197`, `prune.go:276`, `clone.go:569,605` (via the `RemoveAll` seam), `launchers.go:165,170`, `junction.go:259`, `index.go:315`, `hook.go:160`, `ancestors.go:52`, `warpprobe.go:57`, `gitexclude.go:108,112,116,120` |
| `git worktree remove [--force]` | `remove.go:177-185`, `prune.go:258-261`, `add.go:265`, `weftwiring.go:175-180` |
| `git branch -D` | `cleanup.go:273`, `checkout.go:193`, `add.go:277`, `weftwiring.go:192` |
| `ResetHard` | `pull.go:233,267,285` (all via `f.warp.ResetHard`, thin delegation through `warpforward.go:33`) |
| link removal | `weftwiring.go:156` (`fslink.Remove` via `removeJunctionRecords`), `portals.go:57`, `junction.go:259` |

Not all of these are gate candidates.
`gitexclude.go`'s four are temp-file cleanup inside `writeFileAtomically` under a flock;
`warpprobe.go:57` removes a probe directory the same function created;
`ancestors.go:52` is `pruneEmptyAncestors`, which removes only directories that are already empty and halts on the first non-empty one;
`index.go:315` removes fabric's own derived correspondence-index cache file.
These are allowlist candidates, and each allowlist entry carries its reason.

### Ingredients that already exist — consolidate, do not invent

- `refuseUncontainedPath(container, target, what)` — `ancestors.go:20`. R5's containment assertion. Already used by `removePortal` and `removeLaunchers`, **not** by `removeJunctionRecords`.
- `isRegisteredLinkedWorktree(l, target)` / `isRegisteredLinkedWorktreeIn(repoDir, target)` — `remove.go:210,223`. "Is this git's, and this hub's?" A failure to enumerate answers false, the conservative direction.
- `applyStalePairOwnership(l, weftPath, pe)` — `prune.go:175`. R4's ownership gate, deliberately not bypassed by `--force`.
- `looksLikeHub(hubPath)` — `clone.go:579`. R4's hub predicate: a `_board` entry, or at least one weft sibling directory. An unreadable directory answers false.
- `refuseDirtyWeftWorktree(weftTarget)` — `remove.go:127`. Untracked-inclusive. An absent weft worktree is not a refusal; an unreadable one is.
- `applyStalePairProtection(weftPath, force, pe)` — `prune.go:206`. Tracked-only.
- `validateWorktreeSlug(slug, junctionNames)` — `slug.go:30`. Rejects empty, separator-containing, `.`/`..`/non-`Clean`, weft-suffixed, and reserved hub names.
- `var RemoveAll = os.RemoveAll` — `clone.go:32`. An existing test seam demonstrating the routing idea already works.
- `refusePrimeSlug(l, slug)` — `remove.go:153`. Refuses the hub's prime worktree by name.

### Geometry helpers the gate will need

`WorktreePath` (`junction.go:24`), `WeftWorktreePath` (`weftwiring.go:35`), `WeftRepoRoot` (`worktreelist.go:108`), `LauncherDir` / `launchersDir` (`launchers.go:33,25`), `PortalLink` / `PortalsDir` (`portals.go`), `BoardDir` (`junctionnames.go:100`), `WeftWorktree` (`fabric.go:115`), `PrimeName` (`worktreelist.go:85`).

### Three gaps found during exploration that the manifest does not name

1. **`removeJunctionRecords` has no containment check** (`weftwiring.go:153-161`). Its siblings `removePortal` (`portals.go:54`) and `removeLaunchers` (`launchers.go:159`) both call `refuseUncontainedPath`; this one calls `fslink.Remove` straight on a slug-derived `WarpJunction.Link`. Reached from `Remove` and `rollbackAdd`.
2. **`teardownHub` has no ownership check** (`clone.go:604`). It calls `RemoveAll(hubPath)` unconditionally on any clone-or-worktree-add failure. The `--reset` path 40 lines above (`clone.go:563`) calls `looksLikeHub` first — the same directory, two different rules.
3. **`removeWarpWorktreeDir`'s fallback is not re-gated** (`remove.go:191-199`). It fires on *any* nonzero exit from `git worktree remove` for a registered linked worktree, and `git worktree remove` without `--force` refuses on untracked files. See the `remove-keeps-untracked-inclusive-and-the-fallback-is-regated` decision.

### The eight probe sites to collapse

`add.go:43` (tracked-only), `checkout.go:41` (tracked-only), `prune.go:215` (tracked-only), `pull.go:143` (tracked-only), `remove.go:61` (all), `remove.go:132` (all, inside `refuseDirtyWeftWorktree`), `warpclean.go:54` (all, inside `dirtyReason`), `reconcile.go:299` (all).
Four tracked-only, four untracked-inclusive — which is why scope is a declared parameter rather than a constant.

`worktreelist.go:28`'s `git worktree list --porcelain` is a different command and out of scope.
`internal/websterengine/gitwrap.go:31` is outside `fabricengine` and outside this slice.

### Guard test template

`cmd/lyx/rawgitmutation_test.go` is the model to clone.
Its shape: `rawGitMutationScanPackages` (module-relative subtrees), `rawGitMutationBannedTokens` (raw substrings), `rawGitMutationAllowlist` (module-relative slash-separated path → reason), `rawGitMutationMinScannedFiles` (vacuous-scan floor), `exec.LookPath("go")` skip, `go env GOMOD` module-root resolution, `filepath.WalkDir` skipping `_test.go`, `filepath.ToSlash` before comparison (Windows is the primary dev OS).

Proposed banned token set: `os.RemoveAll(`, `os.Remove(`, `"worktree", "remove"`, `"branch", "-D"`, `.ResetHard(`, `fslink.Remove(`.
Proposed allowlist: `destroy.go` (the gate itself), `gitexclude.go`, `warpprobe.go`, `ancestors.go`, `index.go` — each with its reason.

`internal/scoutengine/lspclient_guard_test.go` is the repo's precedent that a **single-file** scope is exactly as machine-checkable as a package scope, which is what makes "one file in `fabricengine`, not a sub-package" cost nothing in enforcement.

### Existing regression cover

The campaign fixed each of the eight defects with a named, sabotage-proved test, inside roughly 29 integration test files covering the destructive verbs:
`TestPull_DirtyWarpRefusesBeforeMovingWarp`, `TestPrune_RefusesHubDirectoryItDoesNotOwn`, `TestPrune_RefusesUnrelatedGitCloneInHub`, `TestPrune_ProtectsDirtyWeftWorktreeUntilForced`, `TestCleanup_ProtectsPrimaryWeftBranchAfterCheckout`, `TestCleanup_PrimaryBranchSurvivesForceWhenNotCheckedOut`, `TestAdd_RejectsReservedHubNameSlug`, plus `remove_guard_integration_test.go` and `remove_reserved_integration_test.go`.
A consolidating refactor is exactly the change those tests are good at policing.
`clone_reset_guard_test.go` and `prune_unowned_integration_test.go` / `prune_dirty_integration_test.go` cover the ownership and dirtiness halves directly.

### Package layout note

`internal/fabricengine` runs 38 external (`package fabricengine_test`) test files against 45 in-package ones.
`clone_test.go` is in-package.
No `fabrictest` package exists yet — slice 13 creates it.
Integration tests for this slice therefore go in existing or new `//go:build integration` files in `package fabricengine_test`.

## Constraints

From `CONSTRAINTS.md`:

- **Cwd Resolution Invariant** — `internal/lyxcwd` owns cwd resolution alone. The gate takes a `*lyxcwd.Location` and derives paths through the existing geometry helpers; it never resolves cwd itself and never constructs a weft or junction path by string literal.
- **Fabric Git Invariant (warp + weft)** — every git operation LYX's own code performs goes through `internal/fabricengine`. Read-only verbs (`git status --porcelain`, current SHA) are exempt, which is why `dirtiness.go` is not a violation. Its `mutateGitExclude` bullet is why `gitexclude.go` is out of scope.
- **gitrepo Client Boundary Invariant** — `gitexec` is the only path to the git CLI inside `internal/gitrepo`, with a pinned method list any new call must update in the same commit. Not triggered: the `dirtiness-probe-stays-fabric-local` decision means nothing is added to `internal/gitrepo`.
- **Test Tier Purity Invariant** — untagged test files must not call `gitexec.RunGit`, `exec.Command*`, or `lyxtest.Copy*`, and a raw substring match trips on a comment or string literal too. The new guard test at `cmd/lyx/destructiveguard_test.go` carries banned tokens as test data and uses `exec.Command` for `go env GOMOD`, so **it needs an allowlist entry in `cmd/lyx/tierpurity_test.go`**, exactly as `rawgitmutation_test.go` and `tierpurity_test.go` itself do. Hermetic unit tests for the gate must not spawn git.
- **Hermetic Git Test Environment Invariant** — every git-spawning test package needs a `TestMain` calling `lyxtest.HermeticGitEnv()`. `internal/fabricengine` already has `testmain_test.go`; new integration tests land in that package and inherit it.
- **CLI / Cobra Invariant** — not triggered: no CLI surface changes, no new commands.
- **Never Force-Add Invariant** — not triggered, but it is the closest precedent in the file for a narrow, machine-checked, imperative entry.
- **Documentation Lifecycle** — `manifest/designs/fabric-crucible-followups.md` is deleted once all four slices land, with its durable rationale folded into `internal/fabricengine`'s package doc. This slice folds slice 12's share of that rationale into `doc.go` now, and marks slice 12 landed in the manifest file rather than deleting it.

New invariant this slice adds (short, imperative, rules only):

- **Fabric Destruction Chokepoint Invariant** — pinning: the one file that may destroy; the banned token set; the four checks and their fixed order; that `--force` answers dirtiness and never ownership; that every allowlist entry carries a reason; and the enforcing test name.

Discovered during discussion:

- `shed-model-contradiction-sweep` also edits `CONSTRAINTS.md` (its pointer-rule invariant). Different section — rebase rather than assume.
- Worktree isolation: this agent operates only within `wts/fabric-destructive-chokepoint`, and never pushes to `main`.

## Testing

### Hermetic tier (untagged, `package fabricengine`)

TDD candidates — these are the gate's own logic and should be written first:

- **Check ordering.** The gate runs containment → ownership → dirtiness → force. A request failing two checks reports the *earlier* one. This is pure logic over the request struct and needs no git.
- **Containment.** `refuseUncontainedPath` semantics through the gate: `..`, `../x`, `.`, an absolute path outside the container, a path equal to the container, and the platform-separator cases (`/` and `\` on both platforms).
- **Slug validation reaches the gate.** A request carrying a derived slug of `..`, `_board`, `<name>-weft`, `""`, or a separator-containing string is refused before anything is touched.
- **Force semantics.** `force: true` satisfies a dirtiness check and never satisfies an ownership check. This is the rule that is pure review discipline today.
- **`dirtinessNA` requires a reason.** An empty reason is a refusal, not a pass.
- **Refusal typing.** `*destructiveRefusal` carries the correct `Check` value for each of the four.

### Integration tier (`//go:build integration`, `package fabricengine_test`)

Ownership and dirtiness need real git:

- **Ownership.** `ownedRegisteredLinkedWorktree` against a real registered linked worktree, against the main worktree, against an unrelated git clone parked at a fabric-named path, and against a plain directory. An unenumerable repo answers false.
- **`ownedFabricHub`.** A real hub, a directory with `_board` only, a directory with a weft sibling only, a directory with neither, and an unreadable directory.
- **Dirtiness, both scopes.** Tracked modification, staged change, untracked file only, and clean — asserted separately under `dirtyScopeTracked` and `dirtyScopeAll`, so the two scopes are proven to differ on the untracked-only case.
- **The three newly-closed gaps**, one test each:
  - `removeJunctionRecords` refuses a junction link outside the worktree it belongs to.
  - `teardownHub` refuses a `hubPath` that is not a fabric hub.
  - `removeWarpWorktreeDir`'s fallback does not delete a registered linked worktree carrying untracked files when git refused for that reason.

### Guard test

`cmd/lyx/destructiveguard_test.go` — a new raw destructive token in a non-allowlisted `internal/fabricengine` file fails the build.
Sabotage-prove it: add a raw `os.RemoveAll(` to a scanned file, confirm the test fails, revert.
Confirm the vacuous-scan floor fires when the scan path is wrong.
Add the file to `cmd/lyx/tierpurity_test.go`'s allowlist and confirm tier purity still passes.

### Regression posture

Every existing named test listed under Technical context must pass **unchanged**.
Any that requires an edit is a behaviour change: stop, name it, and surface it rather than editing the test.
The full `//go:build integration` fabricengine suite is the gate on this refactor.

### Explicitly not tested here

The full per-primitive × per-state × per-verb cross product.
That is slice 13's harness, and building it here without the hub factory would mean hand-rolled fixtures slice 13 would delete.

## Q&A log

- **Q:** What is the gate's call shape? **A:** Typed `destructiveRequest` struct plus one executor per primitive, all in `internal/fabricengine/destroy.go`, with a single `gate(req)` running the four checks in fixed order. Required fields make an omitted check a compile error.
- **Q:** How is dirtiness scope decided — fixed per primitive, or declared by the caller? **A:** Caller-declared from a closed enum (`dirtyScopeTracked` / `dirtyScopeAll` / `dirtinessNA(reason)`), with every current site keeping its current scope. Deriving it from the primitive breaks `Prune`, whose tracked-only probe is deliberate.
- **Q:** Does `Remove` normalise to tracked-only? **A:** No. It stays untracked-inclusive, and `removeWarpWorktreeDir`'s `os.RemoveAll` fallback is itself re-gated with `dirtyScopeAll` — otherwise git's untracked refusal would route straight into the fallback and delete the untracked files.
- **Q:** Do in-transaction rollback and teardown paths go through the gate? **A:** Yes, with containment and ownership enforced and dirtiness declared N/A per site. Ownership matters there: R4's `clone --reset` defect was a teardown path, and `teardownHub` has no ownership check today.
- **Q:** Is the tracked-only dirtiness probe promoted into `internal/gitrepo`? **A:** No — fabric-local. Applying the manifest's stated criterion: six of the eight probe sites are `Topology` verbs, and `Topology` holds no `Repo` at all. Only `pull.go` has one.
- **Q:** Does the probe live inside `destroy.go`? **A:** No, in its own `dirtiness.go`. Three of the eight callers are read-only and would otherwise be routed through a destruction gate; and it keeps the guard's allowlist honest.
- **Q:** Is ownership a closed enum or a caller-supplied predicate? **A:** Closed enum resolved by the gate. A caller-supplied predicate is exactly the escape hatch that produced the class.
- **Q:** Is link/exclude removal gated? **A:** Link removal yes (it closes the live `removeJunctionRecords` containment gap); exclude rewrite no (R5 already consolidated it behind a flock and an atomic rename).
- **Q:** Where does R6's validation asymmetry get fixed? **A:** In the gate, on a `slug` field, plus keeping the existing entry-point calls. The real asymmetry is `Prune`/`Reconcile` deriving slugs from directory names, not the six verbs that take no slug.
- **Q:** What does a refusal return? **A:** A typed `*destructiveRefusal{Check, What, Target, Reason}`, wrapped by each verb into its existing error shape — so slice 13 can assert which check refused without string-matching, and slice 14 has something to generalise.
- **Q:** Where does the bypass guard live? **A:** A new `cmd/lyx/destructiveguard_test.go` cloning `rawgitmutation_test.go`'s machinery, scanning `internal/fabricengine` only, with a reasoned per-file allowlist and a vacuous-scan floor. Issue #135 is noted, not resolved here.
- **Q:** Do we need a `CONSTRAINTS.md` entry at all for something this internal, when all git operations go through fabric anyway? **A:** Yes, but short. The Fabric Git Invariant binds which *module* runs git and explicitly exempts `git status --porcelain`; and three of the five primitives are not git at all — the hub-destroying `remove ..` defect was `os.RemoveAll`. A top-level section, because filing an `os.RemoveAll` rule under a git invariant is the wrong parent.
- **Q:** How short? **A:** Imperative rule only. No rationale, no narrative, no history — those go to `doc.go`. The file's own preamble says it states rules only.
- **Q:** Commit shape, given the `CONSTRAINTS.md` co-edit? **A:** One commit for the whole slice. Rebase onto `main` and re-read before pushing rather than assuming `shed-model-contradiction-sweep`'s section is untouched.
