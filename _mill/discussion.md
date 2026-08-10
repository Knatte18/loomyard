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
- Moving the `var RemoveAll = os.RemoveAll` test seam from `clone.go:32` into `destroy.go` — the one file allowed to destroy should own the function that destroys.
- Moving `Fabric.ResetHard` from `warpforward.go:33-35` into `destroy.go`, where it becomes the gated executor; `pull.go`'s three sites call it instead of reaching for the raw `f.warp` handle.
- A typed `*destructiveRefusal` error carrying which of the four checks refused, wrapped by each verb into its existing error shape, and distinguishable via `errors.As` on best-effort paths.
- A bypass guard test at `cmd/lyx/destructiveguard_test.go`.
- A short imperative `CONSTRAINTS.md` invariant, in the same commit.
- Docs: `internal/fabricengine/doc.go`, `manifest/designs/fabric-crucible-followups.md`, `manifest/roadmap.md`.
- **Correcting a stale sentence in the manifest's own slice-12 section**, in the same commit — its open questions say the dirtiness probe "is deliberately tracked-only today" and that the chokepoint should "inherit it rather than silently widen it". That over-generalised `prune.go`'s comment to the whole package and is contradicted by the verified 4/4 scope split. Left standing, it invites a reviewer of this slice to file the caller-declared enum as a deviation from spec.

**Out:**

- **A sub-package.** `internal/fabricengine/destroy` or a lower leaf beside `internal/fslink` is not built. The predicates the gate needs (`isRegisteredLinkedWorktree`, `looksLikeHub`, `applyStalePairOwnership`, weft path construction) are `fabricengine`-private, so a sub-package importing `fabricengine` for them while `fabricengine` imports the sub-package for the gate is an import cycle Go forbids. Extract to a told-everything leaf later if a non-fabric caller appears.
- **Promoting the dirtiness probe into `internal/gitrepo`.** See Decisions.
- **`gitexclude.go`'s exclude rewrite.** R5 already consolidated every read-modify-write of `.git/info/exclude` into one file behind a repo-wide flock and an atomic same-directory rename. It is already a chokepoint of its own shape; its `os.Remove` calls only unlink a temp file the same function created three lines earlier.
- **The result envelope.** Step 5 (honest reporting) lands in each verb's existing error shape. Generalising it into one accumulate-as-you-mutate envelope is slice 14.
- **The live-state harness, `fabrictest`, and the hub factory.** Slice 13.
- **`docs/overview.md`.** No new module, no change to the module table or the execution stack.
- **A shared framework for the repo's ~15 hand-rolled static-analysis guards** (issue #135). Noted, not built here.
- **Behaviour changes other than the three named gaps.** Closing those three gaps *is* a behaviour change by construction — paths that were destroyed will now be refused — and it is in scope. Everything else is a consolidating refactor: every current site keeps its current dirtiness scope, and any existing named test that requires an edit is a behaviour change to be flagged, not silently edited.

## Decisions

### gate-call-shape

- Decision: typed request structs plus one executor per primitive, all in `internal/fabricengine/destroy.go`.
  **Two request shapes**, because destruction has two target shapes:
  - `pathRequest{what, container, target, slug *slugSpec, ownership, dirtiness, force}` — for `os.RemoveAll`/`os.Remove`, `git worktree remove`, `ResetHard`, link removal, and link re-point.
    Executors: `removePath`, `removeGitWorktree`, `resetHardTo`, `removeLink`, `repointLink`.
    `removePath`, not `removeDir`: `removeLaunchers` deletes the `ide` and `fabric-checkout` **script files** (`launchers.go:165`) as well as their directory, and `os.Remove(` is a banned token, so those calls must route through this executor too.
  - `branchRequest{what, repoDir, branch, ownership, dirtiness, force}` — for `git branch -D`.
    Executor: `deleteBranch`.
    It carries **no `container` or `target` field at all**, which is how containment is declared structurally N/A for a ref rather than by a per-call-site `""`.

  **Each check's inputs travel with the check, not on the request.**
  This is the rule that keeps "every field required ⇒ an omitted check is a compile error" true, and it is what the first draft got wrong by hoisting `l *lyxcwd.Location` to a top-level field.

  - Ownership kinds are parameterised by exactly what they need, and nothing else: `ownedFabricHub` and `ownedFreshlyCreatedPath(reason)` take no repo context at all;
    `ownedRegisteredLinkedWorktree(repoDir)`, `ownedWarpCheckout(repoDir)` and `ownedUnderGeometryRoot(root)` take a path;
    `ownedManagedWeftBranch(l *lyxcwd.Location)` takes the Location, because `primaryWeftBranch(l)` is the one predicate that genuinely needs it.
  - Slug validation travels as `slug *slugSpec{name string, junctionNames []string}` — nil when the target is not slug-derived, and otherwise carrying both halves `validateWorktreeSlug(slug, junctionNames)` (`slug.go:30`) requires. `junctionNames` reaches the verbs today as `t.cfg.Dirs()`.

  **This is what makes clone's two hub-level sites work.** `CloneHub(cwd string, …)` (`clone.go:128`) has **no `*lyxcwd.Location` at all** where they run: `resetHub` is called at `clone.go:159` and `:204`, `teardownHub`'s earliest calls are `:243`/`:245`, the only earlier Location is the synthetic partial `&lyxcwd.Location{HubPath, WorktreeName}` at `:260` — itself after the first teardown sites — and the real `lyxcwd.Resolve` is not reached until `:369`.
  A required top-level `l` would have forced those two sites to pass `nil` or a synthetic, which is the "trust me" hole in another costume.
  With inputs on the kinds, both sites declare kinds that need no Location (`ownedFabricHub`, `ownedFreshlyCreatedPath`), nothing is nil, and the compile-error property holds at all six `RemoveAll(` sites.
  Nothing derives `l` — the Cwd Resolution Invariant owns resolution and the gate is a consumer.

  One shared check pipeline runs the four checks in the same fixed order over both shapes;
  each executor calls it first, then performs the act.
- Rationale: every field is required, so an omitted check is a compile error rather than a forgotten one.
  "Which checks did this site declare" stays greppable at each call site.
  The gate executing rather than approving is what makes a raw `os.RemoveAll` outside one file mechanically bannable, which reduces the bypass guard to a trivial file-scoped scan.
  Two shapes rather than one: a ref is not a path, and a single struct would need a `container` field that every `branch -D` site fills with `""` — an omission indistinguishable from a mistake, at the four sites where one of the eight defects lives.
  Two shapes with one pipeline is still one gate: the manifest defines the gate as the four checks, not as one struct.
- Rejected: free functions with positional args per primitive (a new check means changing every signature, and declarations stop being greppable);
  a `destroyer` value constructed per verb (hides which checks are in play behind the constructor);
  one struct covering both shapes (see above).

### dirtiness-scope-is-caller-declared

- Decision: dirtiness is a closed sum type the caller declares, with a documented reason at each site: `dirtyScopeTracked`, `dirtyScopeAll`, `dirtyCheckedOutBranch`, or `dirtinessNA(reason)`.
  One implementation per shape, one fixed order;
  the choice is the call site's, declared.
  Every current site keeps its current behaviour.
  `dirtyCheckedOutBranch` is the ref-shaped member and is valid only on a `branchRequest` — see `branch-deletion-is-ref-shaped`.
  The three path-shaped members are valid only on a `pathRequest`.
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
  R4's `clone --reset` defect *was* a teardown path.
- **Which ownership kind `teardownHub` declares, and why neither obvious answer is right.**
  This must be stated rather than left to the implementer, because both available answers fail:
  - **`ownedFabricHub` is wrong on the merits, not merely inconvenient.** `looksLikeHub` (`clone.go:579`) requires a `_board` entry or at least one weft sibling. `teardownHub` has **13 call sites** in `clone.go`, the earliest at `:243`/`:245` — immediately after the warp clone attempt, when neither exists yet. The rollback would be *refused* at nearly every early failure, and `clone.go:606` would leave "residual hub left at %s; remove it manually". A gate that blocks cleanup of a half-built hub is worse than the gap it closes.
  - **A bare `ownedInTransaction` trust-me closes the gap in name only** — "teardown is special" is precisely the reasoning that produced the `clone --reset` defect.

  The resolution is **containment plus a transaction identity the gate itself mints**, declared as `ownedFreshlyCreatedPath(tok createdToken)`.

  **The gate creates the path, so it can verify the claim instead of believing it.**
  An earlier draft had this kind take a free-text `reason` and no repo context, which left the gate executing nothing for it — the `ownedInTransaction` trust-me this section rejects, wearing a better name.
  Instead `destroy.go` exposes the *creation* side too: `createExclusiveDir(path) (createdToken, error)` creates the directory **exclusively** — `os.Mkdir` on the final component, which fails with `EEXIST` rather than succeeding on an existing directory the way `os.MkdirAll` does — and returns an unexported `createdToken`.
  `ownedFreshlyCreatedPath` accepts only that token.
  Since the type is unexported and the gate is its sole minter, **a site cannot declare this kind for a path the gate did not create** — the claim becomes structural rather than asserted, and a new declaration is impossible to write rather than merely discouraged.
  This is the same "the gate executes rather than approves" principle applied one step earlier: the gate now owns the creation whose destruction it will later authorise.

  **Exactly one call, at `clone.go:220`, replacing `os.MkdirAll(hubPath, 0o755)` — and the two existing offline stat guards stay where they are.**
  This placement has to be stated, because the three statements are *not* adjacent and both alternative readings change behaviour.
  `CloneHub`'s stat guards sit at `clone.go:163` (two-argument) and `clone.go:212` (one-argument), and `probeWeftBinding` — the network call — runs between the first of them and the creation, an ordering `clone.go:208-211` documents as deliberate.
  So:
  - Creating **early**, at either stat guard, would leak a residual hub whenever the probe then fails, since that path returns without teardown.
  - Creating **late but folding the stat guards in** would defer the offline "hub already exists" refusal until after a network call, breaking the documented offline-before-network ordering.
  - Creating late and **leaving the guards alone** — the choice here — preserves both existing error messages verbatim, keeps the offline refusal offline, and mints the token at the single point where the directory actually comes into being.

  The guards are UX and ordering;
  `createExclusiveDir`'s own `EEXIST` is the safety property, and being later it is also strictly more current.
  That closes a real TOCTOU window present today between the stat at `:163` and the `MkdirAll` at `:220`, in which a concurrent process can create the hub and have `os.MkdirAll` silently accept it.

  With that in place the two halves are:
  - *Containment*: `hubPath` resolves strictly below the `cwd` the operator named. `HubPath(parent, name)` is `filepath.Join(parent, name+HubSuffix)` (`junctionnames.go:106-108`) and `DeriveWarpName` splits on `/`, `\` and `:` so no separator survives — a derived `..` becomes the harmless `..-HUB`. Containment therefore holds today; asserting it costs nothing, which is `refuseUncontainedPath`'s own stated rationale (`ancestors.go:17-19`).
  - *Transaction identity*: `teardownHub` provably removes only a directory this invocation created, and after the change the **gate** is what proves it. Today the guarantee is spread across `CloneHub`: both forms `os.Stat(hubPath)` and fail with "hub already exists at %s" — two-argument at `clone.go:163`, one-argument at `clone.go:212` — and `os.MkdirAll(hubPath, 0o755)` follows at `clone.go:220`. `createExclusiveDir` takes over the creation only, so the property moves from "a structural guarantee spread across the surrounding function" to "a token the gate minted", and it strengthens on the way (see the placement note above).

  **Why that is stronger than `looksLikeHub`, not weaker.** `looksLikeHub` answers "does this *look like* a hub" — a pattern match, and pattern-matching a name fabric *derives* rather than one the operator types is exactly what R4's `clone --reset` defect exploited. The transaction check answers "did this invocation create this exact path", which is strictly stronger. `--reset` genuinely cannot use it, because it acts on a directory that pre-existed the invocation — which is why `looksLikeHub` is correct there and wrong here. The two siblings *should* differ; what was missing was the reason, and containment, which `teardownHub` lacks entirely today.
- Rejected: bypassing the gate with a guard allowlist (re-opens the "teardown is special" reasoning that produced the `clone --reset` defect);
  passing `force: true` (makes rollback indistinguishable from an operator's `--force`, which step 4 of the gate explicitly forbids conflating);
  `ownedFabricHub` for `teardownHub` (refuses at 13 early-failure sites, see above).

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

- Decision: ownership is a closed enum of kinds, each resolved by the gate itself.

  Directory-shaped, valid on a `pathRequest`:
  `ownedRegisteredLinkedWorktree(repoDir)`, `ownedWarpCheckout(repoDir)`, `ownedFabricHub`, `ownedUnderGeometryRoot(root)`, `ownedFreshlyCreatedPath(tok createdToken)`, `ownedFreshlyCreatedWorktree(tok createdToken)`.

  The two token-carrying kinds differ only in what the gate minted — a bare directory (`createExclusiveDir`) versus a git worktree the gate added — and neither token is constructible outside `destroy.go`.

  Link-shaped, valid on a `pathRequest`:
  `ownedWiredJunction(wiredLinks []string, expectedTarget string)`, `ownedDriftedWiredJunction(wiredLinks []string)` — see below.

- **Every kind names the predicate the gate runs.** No kind is a label only;
  if a kind cannot state what it verifies, it is a trust-me and does not belong in the enum.

  | kind | what the gate verifies |
  |---|---|
  | `ownedRegisteredLinkedWorktree(repoDir)` | `isRegisteredLinkedWorktreeIn(repoDir, target)` — `List` membership **excluding** the main entry |
  | `ownedWarpCheckout(repoDir)` | `List(repoDir)` membership **including** the main entry |
  | `ownedFabricHub` | `looksLikeHub(target)` — a `_board` entry or at least one weft sibling |
  | `ownedUnderGeometryRoot(root)` | `root` is a member of the closed set `{portalsDir(l), launchersDir(l)}`, and `target` resolves at or below it |
  | `ownedFreshlyCreatedPath(tok)` / `ownedFreshlyCreatedWorktree(tok)` | the token was minted by the gate for exactly this `target` |
  | `ownedWiredJunction(wiredLinks, expectedTarget)` | `target ∈ wiredLinks`, `target` is a link, and it resolves to `expectedTarget` |
  | `ownedDriftedWiredJunction(wiredLinks)` | `target ∈ wiredLinks` and `target` is a link; the resolved target is deliberately not compared |
  | `ownedManagedWeftBranch(l)` | `WeftWarpSlug` accepts the branch name, and it is not `primaryWeftBranch(l)` (fail-closed if unreadable) |
  | `ownedTransactionCreatedBranch` | the branch was created earlier in this same invocation; the checked-out floor check still applies |

- **`ownedUnderGeometryRoot` replaces the earlier `ownedHubGeometryChild`, and the rename is not cosmetic.**
  "Child" was literally false at its main site: `launchersDir(l)` is `<hub>/_launchers` (`launchers.go:25-27`) while `LauncherDir(l, slug)` is `<hub>/_launchers/<AnchorRel>/<slug>` (`launchers.go:33-35`), so on a subpath-anchored hub the script files `removeLaunchers` deletes sit three or more levels down.
  The kind admits **deep descendants and non-directory targets**, both deliberately.
  What it actually adds over containment is the half containment cannot supply: **containment proves the target is below the container, but proves nothing if the caller chose the container.**
  `ownedUnderGeometryRoot` closes that by requiring `root` to come from a fixed set of fabric's own geometry roots, so a call site cannot satisfy containment by naming a convenient parent.

- **The two link kinds take the wired link-path set explicitly**, because "its location is one fabric wires" is half the rule and a kind cannot evaluate an input it never receives.
  Without it `ownedDriftedWiredJunction` would degenerate to bare link-ness — the exact R1 rule this decision rejects.
  Where `wiredLinks` comes from at each site:
  `WarpJunctions(l, slug, names)`'s `.Link` values for `weftwiring.go:156`, `junction.go:161` and `junction.go:461`;
  `PortalLink(l, slug)` for `portals.go:57`;
  `filepath.Join(WorktreePath(l, slug), l.AnchorRel, BoardDirName)` for `unwire.go:143` and `junction.go:311`.

  Ref-shaped, valid on a `branchRequest`:
  `ownedManagedWeftBranch(l *lyxcwd.Location)`, `ownedTransactionCreatedBranch(reason)` — see `branch-deletion-is-ref-shaped`.

  Nothing else.

- **The two link-shaped kinds, because "a path is a link" is not ownership.**
  The six gated `fslink.Remove` sites need "this is a fabric junction", and the earlier draft deferred that to each site's existing `fslink.IsLink` / target-comparison refusal — which names no enum member and re-admits the per-site checks the enum exists to abolish.
  Link-ness alone cannot be the rule: **R1's defect destroyed a *tracked symlink* in the warp worktree**, and a user's tracked symlink is a link. What separates fabric's junction from the operator's own is *where it sits* — a path fabric wires a junction at, from the configured name-set.
  - `ownedWiredJunction(expectedTarget)` — teardown (`weftwiring.go:156`, `portals.go:57`, `unwire.go:143`, `junction.go:461`): the path is a link, its location is one fabric wires, **and** it resolves to `expectedTarget`.
  - `ownedDriftedWiredJunction` — re-point (`junction.go:161`, `:311`): the path is a link and its location is one fabric wires, and the target is deliberately **not** compared, because a drifted or dangling target is the precondition for repairing it.

  Two kinds rather than one with a flag: the target comparison is not merely optional between them, it is inverted, and a single kind would let a teardown site silently opt out of the check that protects it.
  The gate calls `isRegisteredLinkedWorktreeIn`, `looksLikeHub`, `WeftWarpSlug` and `primaryWeftBranch` internally.

- **`ownedWarpCheckout` exists because `ResetHard` is the one primitive that mutates a worktree in place rather than removing something, and the prime worktree is its normal target.**
  `isRegisteredLinkedWorktreeIn` skips `entry.Main` (`remove.go:229-231`) — deliberately, since a main working tree is exactly what must never be deleted after a git refusal.
  Reusing it for `resetHardTo` would therefore refuse `lyx fabric pull` run in the hub's prime warp worktree, which is the ordinary case, not an edge one.
  `ownedWarpCheckout(repoDir)` is `List(repoDir)` membership **including** the main entry: the target must be a worktree of this warp repo, prime or linked.
  The two predicates are deliberately distinct and the difference is exactly the `Main` skip;
  neither may be substituted for the other.
- **`--force` satisfies dirtiness only. It never satisfies containment and never satisfies ownership.**
  The manifest's step 4 states the ownership half;
  the containment half needs saying too, and did not have it.
  `remove ..` — the defect that destroyed an entire hub — is a *containment* failure, so if `--force` were ever read as answering containment, that defect returns behind a flag.
  This is a single rule over all four checks, tested directly.
- Rationale: a closed enum is what makes the guard meaningful — there is no escape hatch through which a call site can supply "yes, trust me".
  Every one of the 28 sites already had the freedom to define its own check, and that freedom is what produced the class.
- Rejected: a caller-supplied predicate `func() (bool, string)` (exactly the hole the slice exists to close);
  an interface with per-kind implementations — note this would **not** be equivalently closed, since Go interfaces are open sets unless sealed with an unexported method, so the closure property would have to be re-established rather than assumed.

### resethard-declares-a-full-check-set

- Decision: `resetHardTo` takes a `pathRequest` with every field named, not a partial one:
  - **container** — the hub (`l.HubPath`), since the warp checkout being reset always sits inside it.
  - **target** — the warp worktree path (`f.warpPath`), the working tree `git reset --hard` actually rewrites.
  - **ownership** — `ownedWarpCheckout(warpRepoDir)`, *not* `ownedRegisteredLinkedWorktree`. See the `Main`-skip note under `ownership-is-a-closed-enum`. **`lyx fabric pull` in the hub's prime warp worktree must still pass**, and does, because this kind counts the main entry.
  - **dirtiness** — `dirtyScopeTracked`, matching `warpWorktreeDirty` (`pull.go:142-152`) exactly: `reset --hard` leaves untracked files alone, so they are no reason to refuse.
  - **force** — always `false`. `Pull` exposes no force flag, and R2's defect was `ResetHard` discarding uncommitted tracked work on every advance path;
    a force parameter here would be a hole with no caller asking for it.
- **`Fabric.ResetHard(sha string) error` keeps its exported signature.**
  A one-argument exported method cannot let its callers declare ownership or dirtiness — and it should not. There is exactly one correct declaration for "reset this Fabric's warp checkout", so the wrapper hardcodes the five fields above and its callers get the gate for free.
  What changes is the body and the file, not the API: the method moves from `warpforward.go:33-35` into `destroy.go` and builds the request itself.
  `warpforward.go`'s package doc describes these delegations as public API for out-of-package callers preserving the one-repo illusion, so the signature is preserved deliberately — verified: no production caller outside `internal/fabricengine` exists today (the only tree-wide matches are in test files and two guard tests), but the surface is kept regardless.
  `CheckoutDetached`, `RestoreBranch` and `CurrentBranch` stay in `warpforward.go` untouched — none is one of the manifest's five destructive primitives.
- Rationale: without this, `ResetHard` is assigned to `pathRequest` with no stated container, target or ownership kind, and the only pre-existing worktree-ownership predicate silently refuses the primitive's most common caller.
- Rejected: giving `resetHardTo` a reduced request shape (a fourth shape for one primitive, and it would need containment anyway);
  reusing `ownedRegisteredLinkedWorktree` (refuses pull in the prime worktree);
  replacing the exported `Fabric.ResetHard` with a request-taking method (breaks the documented public delegation surface for no gain, since the declaration is fixed anyway).

### branch-deletion-is-ref-shaped

- Decision: `git branch -D` goes through the gate as a `branchRequest`, with the four checks reinterpreted for a ref target rather than skipped:
  1. **Containment — structurally N/A.** `branchRequest` carries no `container` and no `target`, so this is expressed by the type rather than by a per-site declaration that could be forgotten. (It does carry `repoDir` — which repo the branch lives in, not a path being destroyed.)
  2. **Ownership** — `ownedManagedWeftBranch(l *lyxcwd.Location)`, resolved inside the gate: the name must be fabric-managed (`WeftWarpSlug` accepts its suffix) **and** must not be `primaryWeftBranch(l)`. The gate resolves `primaryWeftBranch` itself and **inherits its fail-closed direction** — an unreadable primary refuses rather than proceeds (`cleanup.go:200-211`: "Cleanup's deletions are irreversible, so an unreadable primary is the one direction that must fail closed"). The Location is the kind's parameter because `primaryWeftBranch(l)` needs it and `WeftRepoRoot(l)` derives the repo root from it — a bare `weftRepoRoot` could not resolve the primary. The alternative kind, `ownedTransactionCreatedBranch`, covers only the two rollback sites that delete a branch the same invocation created.
  3. **Dirtiness — `dirtyCheckedOutBranch`.** For a ref, "is there work here to lose" is "is this branch checked out at some worktree". `git branch -D` cannot delete a checked-out branch anyway, so this converts git's own refusal into a named gate refusal, the same move as re-gating `removeWarpWorktreeDir`'s fallback.
  4. **Force** — `--force` may answer the `raddleFoldedBack` gate, which is what `Cleanup`'s `force` means today. It may **not** answer the primary-weft carve-out and may not answer the checked-out check. `TestCleanup_PrimaryBranchSurvivesForceWhenNotCheckedOut` pins exactly this and must stay green unchanged.

  Checks 2 and 3 are a **floor applied to every `deleteBranch` call**, including the two rollback sites. Ownership kind selects which branches are candidates at all; it does not license skipping the floor.
- Rationale: `branch -D` is one of the five gated primitives with four sites (`cleanup.go:273`, `checkout.go:193`, `add.go:277`, `weftwiring.go:192`), and its target is a ref, not a path.
  Without a ref-shaped model the only kind that would compile is the transaction escape hatch — which would cover `branch -D` nominally while delegating the real check back to the call site, reproducing the exact class this slice exists to close, for one of its five primitives.
  This is not hypothetical: R3's defect was `cleanup` destroying **the primary weft branch**, and what protects it today is branch-space logic no path-shaped kind can reach — `primaryWeftBranch` (`cleanup.go:~190`), the `branch == primaryWeft` carve-out (`cleanup.go:154`), and the `weftBranch.WorktreePath != ""` checked-out protection just below it.
  Pulling that logic **into** the gate is what makes it apply to the other three sites too, rather than to `Cleanup` alone.
- Note on the two rollback sites: `add.go:277` deletes a **warp** branch this `Add` created, and `checkout.go:193` a weft branch this `Checkout` forked (non-empty only in that case). Both are `ownedTransactionCreatedBranch`, and both still pass the checked-out floor check.
  `weftwiring.go:192` is reached from `Remove` (the removed pair's weft branch) and from `rollbackAdd` with `deleteBranch = !weftBranchAdopted`, which already preserves a pre-existing adopted weft branch — that existing carve-out is preserved, not replaced.
- Rejected: declaring containment N/A per call site via an empty `container` field (an omission indistinguishable from a mistake, at the four sites where one of the eight defects lives);
  routing `branch -D` through `ownedInTransaction` at all four sites (the escape hatch, and the cheap pick under time pressure);
  leaving `branch -D` outside the gate (it is one of the manifest's five pinned primitives).

### link-repoint-is-gated-too

- Decision: the two junction **re-point** sites — `junction.go:161` (`seedLyxJunction`) and `junction.go:311` (`wireBoardLink`) — go through the gate via a distinct `repointLink` executor, not through `removeLink`.
  `repointLink` enforces containment and ownership, declares dirtiness N/A, and accepts no `force` at all.
- Rationale: both remove a link and immediately `fslink.CreateDirLink` it back, so they are repair, not teardown — but what they remove is decided by "the path is a link and it dangles or resolves elsewhere", and **R1's defect was `reconcile` destroying a tracked symlink in the warp worktree**, which is exactly this family of judgement.
  Both sites already guard informally (`fslink.IsLink`, `fslink.PointsTo` target comparison);
  routing them through the gate makes that structural instead of per-site, which is the whole slice.
  A separate executor rather than `removeLink` because the checks differ: a re-point has no `force` semantics and no dirtiness question, and folding it into the teardown executor would mean a teardown call could accidentally reach a repair-shaped check set.
- Rejected: allowlisting both as "repair, not destruction" (that judgement is precisely what R1's defect got wrong);
  routing them through `removeLink` (gives repair a `force` parameter it must never have).

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

### a-refusal-is-never-best-effort

- Decision: one policy, stated once and applied at every best-effort call site.
  An executor returns a value the caller **may** discard for an *operational* failure (git exited nonzero, the filesystem said no) on a path documented as best-effort.
  A **refusal** — a `*destructiveRefusal` from the gate — is never discardable: it must be surfaced, even on a best-effort path.
- Rationale: several sites deliberately discard errors today.
  `rollbackSwitch` (`checkout.go:193`) does `_, _, _, _ = gitexec.RunGit([]string{"branch", "-D", forkedWeftBranch}, ...)`;
  `removeWarpWorktreeDir`'s post-fallback `worktree prune` (`remove.go:200-202`) is commented "Bookkeeping only … must not turn a completed removal into an error";
  `Remove` itself does `_ = removePortal(...)` and `_ = removeLaunchers(...)`.
  Once those become executors that run the gate, each site faces a choice the document must make for it: keep discarding, and a *refusal* is silently swallowed — a new way to lose exactly the signal this slice adds — or start propagating, which changes rollback behaviour.
  Splitting on refusal-vs-operational-failure keeps both properties: best-effort paths stay best-effort, and no gate refusal is ever invisible.
  Mechanically this means executors return a typed error the caller can test with `errors.As` for `*destructiveRefusal`, so "discard operational failures only" is expressible rather than aspirational.
- Rejected: keep discarding everywhere (loses refusals silently, at four of the sites the gate exists to protect);
  propagate everywhere (turns best-effort rollback and bookkeeping into hard failures, changing behaviour this slice declared out of scope).

### bypass-guard-shape-and-home

- Decision: a new `cmd/lyx/destructiveguard_test.go`, cloning `cmd/lyx/rawgitmutation_test.go`'s machinery, scanning `internal/fabricengine` only, with a per-file allowlist carrying reasons and a vacuous-scan floor.
- **The banned token must be `RemoveAll(`, not `os.RemoveAll(`.**
  `clone.go:32` declares `var RemoveAll = os.RemoveAll`, and both of its call sites are **bare**: `clone.go:569` and `clone.go:605` read `RemoveAll(hubPath)`.
  `os.RemoveAll(` is not a substring of `RemoveAll(hubPath)`, so that token misses both — and `clone.go:605` is `teardownHub`, gap #2 in this document's own list.
  The two sites the slice most wants policed would be the two the guard is blind to.
  `RemoveAll(` is a superset that also catches `os.RemoveAll(`, making the latter redundant.
  The seam declaration itself carries no trailing paren and so is not self-flagged.
  **Move the seam into `destroy.go`** in the same commit: it is now a test seam for the gate's own removal primitive, it has no user outside `internal/fabricengine` (verified — `clone.go:32` is the only reference in the tree), and leaving it in `clone.go` means the one file allowed to destroy does not own the function that destroys.
- **The `ResetHard` token must be `warp.ResetHard(` / `weft.ResetHard(`, not `.ResetHard(`.**
  Re-running the enumeration exposed the mirror image of the seam problem: `.ResetHard(` is too *broad*, not too narrow.
  Once `Fabric.ResetHard` becomes the gated executor and `pull.go`'s three sites call `f.ResetHard(...)`, `.ResetHard(` flags the **correctly migrated** callers.
  This exact trap is already documented in the file being cloned — `rawgitmutation_test.go:8-11` explains that it bans construction/call tokens rather than per-verb method names precisely because "a verb-name ban would both flag the correctly-migrated consumer code (which legitimately calls `.CheckoutDetached(`/`.ResetHard(` …) and miss the raw `gitexec.RunGit(` bypass".
  Banning the raw handles instead — `warp.ResetHard(`, `weft.ResetHard(` — targets the thing that is actually forbidden: reaching past the gate to the `gitrepo.Repo` field. It needs no leading dot, so it matches under any receiver name.
- **State the guard's blind spot in the `CONSTRAINTS.md` entry**, in one sentence, per repo convention.
  `"worktree", "remove"` is a raw substring with specific spacing: `[]string{"worktree","remove"}` evades it, as does a dynamically built arg slice.
  The gitrepo Client Boundary Invariant carries a "Known guard blind spot" paragraph and the Fabric Vocabulary Invariant a "what the machine check does and does not reach" section written "stated honestly, not implying full coverage".
  This entry gets the same register — one sentence, imperative, no narrative.
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

**Enumeration method, stated so it can be re-run and audited.**
Per-token `grep -rn '<token>' --include='*.go' internal/fabricengine | grep -v _test`, one pass per token, over the **final** token set below — not over the manifest's prose list.
This matters: the first pass used `os.RemoveAll(` and `.ResetHard(` and produced a wrong inventory both ways (it missed the bare-seam calls, and it mislabelled `junction.go:259`).
The list below is the re-run result.
**Every site has a disposition. There are no unlisted sites for these tokens**, which is the property the In-scope promise ("every destructive call site in `internal/fabricengine`") depends on.

Every gated row names its ownership kind, so no site is left for the implementer to guess.

`RemoveAll(` — 6 sites:

| site | disposition | ownership kind | dirtiness |
|---|---|---|---|
| `remove.go:197` | **gate** — `removePath`, the re-gated fallback | `ownedRegisteredLinkedWorktree(warpRepoDir)` | `dirtyScopeAll` |
| `prune.go:276` | **gate** — `removePath` | `ownedRegisteredLinkedWorktree(weftRepoRoot)` | `dirtyScopeTracked` |
| `clone.go:569` (`resetHub`) | **gate** — `removePath` | `ownedFabricHub` | `dirtinessNA("--reset is the operator explicitly asking for this hub to be replaced; ownership is the check that matters here")` |
| `clone.go:605` (`teardownHub`) | **gate** — `removePath` | `ownedFreshlyCreatedPath(tok)` | `dirtinessNA("gate-created within this invocation; nothing pre-existing to lose")` |
| `warpprobe.go:57` | **allowlist** — removes a probe directory the same function created | — | — |
| `destroy.go` (after the seam moves) | **allowlist** — the gate itself | — | — |

`os.Remove(` — 10 sites:

| site | disposition | ownership kind | dirtiness |
|---|---|---|---|
| `launchers.go:165,170` (`removeLaunchers`) | **gate** — `removePath` | `ownedUnderGeometryRoot(launchersDir(l))` | `dirtinessNA("launcher scripts are generated artifacts, never edited content")` |
| `gitexclude.go:108,112,116,120` | **allowlist** — temp-file cleanup inside `writeFileAtomically`, under the flock | — | — |
| `ancestors.go:52` | **allowlist** — `pruneEmptyAncestors`; `os.Remove` on a directory is refused by the OS when non-empty, and the loop halts on the first refusal | — | — |
| `junction.go:259` | **allowlist** — same class: removes the directory the loop immediately above just emptied by `os.Rename`, and cannot remove it if anything remains. **Not a link removal** — the first pass mislabelled it as one | — | — |
| `index.go:315` | **allowlist** — fabric's own derived correspondence-index cache, deliberately deleted-then-rebuilt so a failed refresh misses honestly rather than answering cross-branch | — | — |
| `hook.go:160` | **allowlist** — removes the user-hook backup this same function wrote ten lines earlier, on its own rollback path after restoring the original | — | — |

`"worktree", "remove"` — 4 sites, all gated via `removeGitWorktree`:

| site | caller | ownership kind | dirtiness |
|---|---|---|---|
| `remove.go:177-185` | `Remove` | `ownedRegisteredLinkedWorktree(warpRepoDir)` | `dirtyScopeAll` |
| `prune.go:258-261` | `removeStalePair` | `ownedRegisteredLinkedWorktree(weftRepoRoot)` | `dirtyScopeTracked` |
| `add.go:265` | `rollbackAdd` | `ownedFreshlyCreatedWorktree(tok)` — the warp worktree this `Add` created; minted by the gate the same way `createExclusiveDir` mints a directory token | `dirtinessNA("rollback of the worktree this Add created")` |
| `weftwiring.go:175-180` | `removeWeftWorktree`, from `Remove` and `rollbackAdd` | `ownedRegisteredLinkedWorktree(weftRepoRoot)` | `dirtyScopeAll`, matching `refuseDirtyWeftWorktree`'s current scope; `rollbackAdd` passes `force` |

`"branch", "-D"` — 4 sites, all gated via `deleteBranch`:

| site | caller | ownership kind | dirtiness |
|---|---|---|---|
| `cleanup.go:273` | `deleteWeftBranch` | `ownedManagedWeftBranch(l)` | `dirtyCheckedOutBranch` |
| `checkout.go:193` | `rollbackSwitch` | `ownedTransactionCreatedBranch` — the weft branch this `Checkout` forked, non-empty only in that case | `dirtyCheckedOutBranch` |
| `add.go:277` | `rollbackAdd` | `ownedTransactionCreatedBranch` — the warp branch this `Add` created | `dirtyCheckedOutBranch` |
| `weftwiring.go:192` | `removeWeftWorktree(deleteBranch=true)` | `ownedManagedWeftBranch(l)`; `rollbackAdd`'s existing `!weftBranchAdopted` carve-out is preserved ahead of the call | `dirtyCheckedOutBranch` |

`warp.ResetHard(` / `weft.ResetHard(` — 4 sites: `pull.go:233,267,285` and `warpforward.go:34`.
`warpforward.go:34` is `Fabric.ResetHard`, the exported thin delegation.
**That method moves into `destroy.go` and becomes the gated executor**, and `pull.go`'s three sites call it (`f.ResetHard(...)`) instead of reaching for the raw handle.
One gated entry, three callers, no allowlist entry needed.

`fslink.Remove(` — 6 sites:

| site | disposition | ownership kind | dirtiness |
|---|---|---|---|
| `weftwiring.go:156` (`removeJunctionRecords`) | **gate** — `removeLink`; the live containment gap | `ownedWiredJunction(WarpJunctions links, j.Target)` | `dirtinessNA("a junction holds no content; the weft target it points at is untouched")` |
| `portals.go:57` (`removePortal`) | **gate** — `removeLink` | `ownedWiredJunction([PortalLink(l, slug)], portal target)` | `dirtinessNA("a junction holds no content; the weft target it points at is untouched")` |
| `unwire.go:143` (board-junction unwire) | **gate** — `removeLink` | `ownedWiredJunction([board link path], BoardDir(l.HubPath))`; subsumes the site's existing is-it-a-link refusal | `dirtinessNA("a junction holds no content; the weft target it points at is untouched")` |
| `junction.go:461` (unwire sweep) | **gate** — `removeLink` | `ownedWiredJunction(WarpJunctions links, targetResolved)`; subsumes the site's existing `linkResolved != targetResolved` refusal | `dirtinessNA("a junction holds no content; the weft target it points at is untouched")` |
| `junction.go:161` (`seedLyxJunction` re-point) | **gate** — `repointLink`, see `link-repoint-is-gated-too` | `ownedDriftedWiredJunction(WarpJunctions links)` | `dirtinessNA("a junction holds no content; the weft target it points at is untouched")` |
| `junction.go:311` (`wireBoardLink` re-point) | **gate** — `repointLink` | `ownedDriftedWiredJunction([board link path])` | `dirtinessNA("a junction holds no content; the weft target it points at is untouched")` |

The first pass listed only two of these six and named a non-`fslink` site among them.
`unwire.go` was missed entirely as a file.

### Ingredients that already exist — consolidate, do not invent

- `refuseUncontainedPath(container, target, what)` — `ancestors.go:20`. R5's containment assertion. Already used by `removePortal` and `removeLaunchers`, **not** by `removeJunctionRecords`.
- `isRegisteredLinkedWorktree(l, target)` / `isRegisteredLinkedWorktreeIn(repoDir, target)` — `remove.go:210,223`. "Is this git's, and this hub's?" A failure to enumerate answers false, the conservative direction.
- `applyStalePairOwnership(l, weftPath, pe)` — `prune.go:175`. R4's ownership gate, deliberately not bypassed by `--force`.
- `looksLikeHub(hubPath)` — `clone.go:579`. R4's hub predicate: a `_board` entry, or at least one weft sibling directory. An unreadable directory answers false.
- `refuseDirtyWeftWorktree(weftTarget)` — `remove.go:127`. Untracked-inclusive. An absent weft worktree is not a refusal; an unreadable one is.
- `applyStalePairProtection(weftPath, force, pe)` — `prune.go:206`. Tracked-only.
- `validateWorktreeSlug(slug, junctionNames)` — `slug.go:30`. Rejects empty, separator-containing, `.`/`..`/non-`Clean`, weft-suffixed, and reserved hub names.
- `var RemoveAll = os.RemoveAll` — `clone.go:32`. An existing test seam demonstrating the routing idea already works. It has **no reference anywhere in the tree except its own declaration and the two bare call sites** (`clone.go:569,605`), so moving it into `destroy.go` breaks nothing.
- `primaryWeftBranch(l)` — `cleanup.go:~190`. The hub's durable primary weft line, read from the branch `<Hub>/_board` is checked out on. Fails closed on an unreadable board branch. The gate resolves this itself for `ownedManagedWeftBranch`.
- `WeftWarpSlug(name)` — inverts the weft suffix; the test for "is this branch/directory fabric-managed at all".
- `refusePrimeSlug(l, slug)` — `remove.go:153`. Refuses the hub's prime worktree by name.

### Geometry helpers the gate will need

`WorktreePath` (`junction.go:24`), `WeftWorktreePath` (`weftwiring.go:35`), `WeftRepoRoot` (`worktreelist.go:108`), `LauncherDir` / `launchersDir` (`launchers.go:33,25`), `PortalLink` / `PortalsDir` (`portals.go`), `BoardDir` (`junctionnames.go:100`), `WeftWorktree` (`fabric.go:115`), `PrimeName` (`worktreelist.go:85`).

### Three gaps found during exploration that the manifest does not name

1. **`removeJunctionRecords` has no containment check** (`weftwiring.go:153-161`). Its siblings `removePortal` (`portals.go:54`) and `removeLaunchers` (`launchers.go:159`) both call `refuseUncontainedPath`; this one calls `fslink.Remove` straight on a slug-derived `WarpJunction.Link`. Reached from `Remove` and `rollbackAdd`.
2. **`teardownHub` has no containment check and no ownership check** (`clone.go:604`). It calls `RemoveAll(hubPath)` unconditionally on any clone-or-worktree-add failure, from **13 call sites** in `clone.go` (`:243,245,268,279,288,308,325,335,346,349,361,371,379`). The `--reset` path 40 lines above (`clone.go:563`) calls `looksLikeHub` first — the same directory, two different rules. The resolution is *not* to give `teardownHub` the same rule: see the `rollback-paths-go-through-the-gate` decision for why `looksLikeHub` would refuse at nearly every early failure, and what replaces it.
3. **`removeWarpWorktreeDir`'s fallback is not re-gated** (`remove.go:191-199`). It fires on *any* nonzero exit from `git worktree remove` for a registered linked worktree, and `git worktree remove` without `--force` refuses on untracked files. See the `remove-keeps-untracked-inclusive-and-the-fallback-is-regated` decision.

### The eight probe sites to collapse

`add.go:43` (tracked-only), `checkout.go:41` (tracked-only), `prune.go:215` (tracked-only), `pull.go:143` (tracked-only), `remove.go:61` (all), `remove.go:132` (all, inside `refuseDirtyWeftWorktree`), `warpclean.go:54` (all, inside `dirtyReason`), `reconcile.go:299` (all).
Four tracked-only, four untracked-inclusive — which is why scope is a declared parameter rather than a constant.

`worktreelist.go:28`'s `git worktree list --porcelain` is a different command and out of scope.
`internal/websterengine/gitwrap.go:31` is outside `fabricengine` and outside this slice.

### Guard test template

`cmd/lyx/rawgitmutation_test.go` is the model to clone.
Its shape: `rawGitMutationScanPackages` (module-relative subtrees), `rawGitMutationBannedTokens` (raw substrings), `rawGitMutationAllowlist` (module-relative slash-separated path → reason), `rawGitMutationMinScannedFiles` (vacuous-scan floor), `exec.LookPath("go")` skip, `go env GOMOD` module-root resolution, `filepath.WalkDir` skipping `_test.go`, `filepath.ToSlash` before comparison (Windows is the primary dev OS).

Banned token set: `RemoveAll(`, `os.Remove(`, `"worktree", "remove"`, `"branch", "-D"`, `warp.ResetHard(`, `weft.ResetHard(`, `fslink.Remove(`.

Two tokens were corrected after re-running the enumeration, in opposite directions:
`RemoveAll(` rather than `os.RemoveAll(`, because the latter is too narrow and misses the bare-seam calls at `clone.go:569,605`, one of them `teardownHub`;
`warp.ResetHard(`/`weft.ResetHard(` rather than `.ResetHard(`, because the latter is too broad and would flag `pull.go`'s correctly migrated `f.ResetHard(...)` calls.

Allowlist, complete — every file the final token set matches that is not converted to a gate call:
`destroy.go` (the gate itself), `gitexclude.go`, `warpprobe.go`, `ancestors.go`, `index.go`, `junction.go` (line 259 only — the emptied-directory removal; the file's five other matches all convert to gate calls), `hook.go` (line 160 only), and `doc.go`.

`doc.go` is on the list because this slice adds the destruction rationale to it and the scan is a raw substring match over every non-test `.go` file in the tree — a prose sentence mentioning `os.RemoveAll(` or `fslink.Remove(` would trip the guard from inside the very document explaining it.
The reason recorded for the entry is that `doc.go` is documentation-only: its sole non-comment line is `package fabricengine` (line 513), so it can never contain a real call.
Each entry carries its reason, per the table in "The five primitives and their current sites".

Note the per-file granularity limitation this creates: `junction.go` and `hook.go` are allowlisted as whole files, so a *new* raw `os.Remove(` added to either would not be caught.
`rawgitmutation_test.go`'s allowlist has the same shape and the same limitation.
State it in the invariant's blind-spot sentence rather than build per-line allowlisting, which is issue #135 territory.

Known blind spot, to be stated in the invariant: raw substring matching means `[]string{"worktree","remove"}` (no space) and any dynamically built arg slice evade the check.

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

- **Cwd Resolution Invariant** — `internal/lyxcwd` owns cwd resolution alone. Each check's inputs travel with the check; only `ownedManagedWeftBranch` takes a `*lyxcwd.Location`, and it is passed in, never derived. The gate reaches paths through the existing geometry helpers, never resolves cwd itself, and never constructs a weft or junction path by string literal.
- **Fabric Git Invariant (warp + weft)** — every git operation LYX's own code performs goes through `internal/fabricengine`. Read-only verbs (`git status --porcelain`, current SHA) are exempt, which is why `dirtiness.go` is not a violation. Its `mutateGitExclude` bullet is why `gitexclude.go` is out of scope.
- **gitrepo Client Boundary Invariant** — `gitexec` is the only path to the git CLI inside `internal/gitrepo`, with a pinned method list any new call must update in the same commit. Not triggered: the `dirtiness-probe-stays-fabric-local` decision means nothing is added to `internal/gitrepo`.
- **Test Tier Purity Invariant** — untagged test files must not call `gitexec.RunGit`, `exec.Command*`, or `lyxtest.Copy*`, and a raw substring match trips on a comment or string literal too. The new guard test at `cmd/lyx/destructiveguard_test.go` carries banned tokens as test data and uses `exec.Command` for `go env GOMOD`, so **it needs an allowlist entry in `cmd/lyx/tierpurity_test.go`**, exactly as `rawgitmutation_test.go` and `tierpurity_test.go` itself do. Hermetic unit tests for the gate must not spawn git.
- **Hermetic Git Test Environment Invariant** — every git-spawning test package needs a `TestMain` calling `lyxtest.HermeticGitEnv()`. `internal/fabricengine` already has `testmain_test.go`; new integration tests land in that package and inherit it.
- **CLI / Cobra Invariant** — not triggered: no CLI surface changes, no new commands.
- **Never Force-Add Invariant** — not triggered, but it is the closest precedent in the file for a narrow, machine-checked, imperative entry.
- **Markdown Link Integrity** — `manifest/designs/fabric-crucible-followups.md` and `manifest/roadmap.md` are both scan sources for `TestEnforcement_MarkdownLinks`. Every link this slice adds or edits must resolve, including its `#anchor` for a `.md` target and any `../../internal/fabricengine/doc.go`-style target — the root restriction is source-side only, so an out-of-root target still gets resolved.
- **Documentation Lifecycle** — `manifest/designs/fabric-crucible-followups.md` is deleted once all four slices land, with its durable rationale folded into `internal/fabricengine`'s package doc. This slice folds slice 12's share of that rationale into `doc.go` now, and marks slice 12 landed in the manifest file rather than deleting it.

New invariant this slice adds (short, imperative, rules only):

- **Fabric Destruction Chokepoint Invariant** — pinning: the one file that may destroy; the banned token set; the four checks and their fixed order; that `--force` answers dirtiness and **never containment or ownership**; that a gate refusal is never discarded on a best-effort path; that every allowlist entry carries a reason; a one-sentence known-blind-spot note (raw substring matching, so alternative arg-slice spacing or a dynamically built slice evades it, and the allowlist is per-file, so a new raw call added to an allowlisted file is not caught); and the enforcing test name.

Discovered during discussion:

- `shed-model-contradiction-sweep` also edits `CONSTRAINTS.md` (its pointer-rule invariant). Different section — rebase rather than assume.
- Worktree isolation: this agent operates only within `wts/fabric-destructive-chokepoint`, and never pushes to `main`.

## Testing

### Hermetic tier (untagged, `package fabricengine`)

**What stays hermetic once ownership kinds carry their own inputs (a `*lyxcwd.Location` among them, for `ownedManagedWeftBranch`).**
The Test Tier Purity Invariant bans `gitexec.RunGit`, `exec.Command*` and `lyxtest.Copy*` — it does not ban filesystem access.
So the split is by predicate, not by struct field:

- **Hermetic** — `refuseUncontainedPath` (pure), `validateWorktreeSlug` (pure: string + `[]string`), `looksLikeHub` (`os.Stat`/`os.ReadDir` only), force semantics, `dirtinessNA` reason enforcement, and refusal typing.
  Check *ordering* is provable hermetically too: to show containment refuses before ownership, submit a request failing both and assert the reported `Check`;
  to show ownership refuses before dirtiness, use `ownedFabricHub` against a temp directory, which needs no git.
- **Integration only** — `isRegisteredLinkedWorktreeIn` (calls `List` → `gitexec.RunGit`), `primaryWeftBranch` (`readBranch`), and every dirtiness probe including `dirtyCheckedOutBranch`.

Passing `l` therefore costs no hermetic coverage;
a `*lyxcwd.Location` can be constructed in a temp directory without spawning anything.

TDD candidates — these are the gate's own logic and should be written first:

- **Check ordering.** The gate runs containment → ownership → dirtiness → force. A request failing two checks reports the *earlier* one. This is pure logic over the request struct and needs no git.
- **Containment.** `refuseUncontainedPath` semantics through the gate: `..`, `../x`, `.`, an absolute path outside the container, a path equal to the container, and the platform-separator cases (`/` and `\` on both platforms).
- **Slug validation reaches the gate.** A request carrying a derived slug of `..`, `_board`, `<name>-weft`, `""`, or a separator-containing string is refused before anything is touched.
- **Force semantics, all four checks.** `force: true` satisfies dirtiness and satisfies **nothing else** — not containment, not ownership. The containment half matters as much as the ownership half: `remove ..`, the defect that destroyed an entire hub, is a containment failure, so a force-satisfies-containment reading brings it back behind a flag. One test per check, not one test for ownership alone.
- **`dirtinessNA` requires a reason.** An empty reason is a refusal, not a pass.
- **Refusal typing.** `*destructiveRefusal` carries the correct `Check` value for each of the four.
- **Shape separation.** A `dirtyCheckedOutBranch` on a `pathRequest`, or a path-shaped scope on a `branchRequest`, does not compile — asserted by construction rather than by test where the type system reaches, and by a refusal test where it does not.
- **Token kinds are unforgeable.** `createdToken` is unexported with no exported constructor, so `ownedFreshlyCreatedPath` / `ownedFreshlyCreatedWorktree` cannot be declared for a path the gate did not create. Assert the round trip: `createExclusiveDir` refuses a path that already exists, and the token it returns authorises removal of that path and no other.
- **Link kinds.** `ownedWiredJunction` refuses a real directory, refuses a link at a path outside the wired name-set, and refuses a link resolving somewhere other than `expectedTarget` — the last being the R1 case, an operator's own tracked symlink sitting where fabric did not wire one. `ownedDriftedWiredJunction` accepts a dangling or mis-pointed link at a wired path and still refuses a real directory.
- **Best-effort policy.** An operational failure from an executor is discardable; a `*destructiveRefusal` is distinguishable via `errors.As` so a best-effort caller can discard the former without swallowing the latter.

### Integration tier (`//go:build integration`, `package fabricengine_test`)

Ownership and dirtiness need real git:

- **Ownership.** `ownedRegisteredLinkedWorktree` against a real registered linked worktree, against the main worktree, against an unrelated git clone parked at a fabric-named path, and against a plain directory. An unenumerable repo answers false.
- **`ownedFabricHub`.** A real hub, a directory with `_board` only, a directory with a weft sibling only, a directory with neither, and an unreadable directory.
- **Dirtiness, both scopes.** Tracked modification, staged change, untracked file only, and clean — asserted separately under `dirtyScopeTracked` and `dirtyScopeAll`, so the two scopes are proven to differ on the untracked-only case.
- **Branch ownership, the R3 defect's own ground.** `ownedManagedWeftBranch` refuses the primary weft branch;
  refuses when `primaryWeftBranch` cannot be read (fail-closed);
  refuses a branch checked out at some worktree;
  and refuses a non-suffixed, non-fabric-managed branch.
  Critically: **each of these refuses under `force: true` as well** — `--force` reaches only the `raddleFoldedBack` gate.
  These must hold at all four `branch -D` sites, not only in `Cleanup`, which is the point of moving the logic into the gate.
- **The three newly-closed gaps**, one test each:
  - `removeJunctionRecords` refuses a junction link outside the worktree it belongs to.
  - `teardownHub` refuses a `hubPath` outside the operator-named parent, and **succeeds** on a half-built hub with no `_board` and no weft sibling — the case `ownedFabricHub` would have wrongly refused at 13 call sites.
  - `removeWarpWorktreeDir`'s fallback does not delete a registered linked worktree carrying untracked files when git refused for that reason.

### Guard test

`cmd/lyx/destructiveguard_test.go` — a new raw destructive token in a non-allowlisted `internal/fabricengine` file fails the build.
Sabotage-prove it twice, because one of the two cases is the one the original token set missed:
add a raw `os.RemoveAll(` to a scanned file, confirm failure, revert;
then add a **bare** `RemoveAll(` call to a scanned file, confirm failure, revert.
The second case is the `clone.go:569,605` seam shape and is the whole reason the token is `RemoveAll(`.
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

- **Q:** What is the gate's call shape? **A:** Typed request structs plus one executor per primitive, all in `internal/fabricengine/destroy.go`, with one check pipeline running the four checks in fixed order. Required fields make an omitted check a compile error. *(First answered as a single `destructiveRequest` struct — **superseded**: a ref is not a path, so there are two shapes, `pathRequest` and `branchRequest`. See `gate-call-shape`.)*
- **Q:** How is dirtiness scope decided — fixed per primitive, or declared by the caller? **A:** Caller-declared from a closed sum type, with every current site keeping its current behaviour. *(First answered with three members — **superseded**: `dirtyCheckedOutBranch` was added for the ref shape, since "is there work here to lose" means something different for a branch. See `branch-deletion-is-ref-shaped`.)* Deriving it from the primitive breaks `Prune`, whose tracked-only probe is deliberate.
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

Resolved against the orchestrator's review of this document (`_mill/discussion-review.md`, three blocking, four non-blocking — all verified against the tree before fixing):

- **Q:** `git branch -D` is a gated primitive, but the ownership enum was four path-shaped kinds and a ref is not a path — so the only kind that would compile is the trust-me escape hatch. **A:** A second request shape (`branchRequest`, carrying no path fields, so containment is N/A by type) plus a ref-shaped ownership kind and a `dirtyCheckedOutBranch` member. `primaryWeftBranch` and the checked-out check move **into** the gate, so they apply at all four `branch -D` sites rather than in `Cleanup` alone.
- **Q:** Does the proposed token set see the `RemoveAll` seam? **A:** No — `clone.go:569,605` call `RemoveAll(hubPath)` bare, and `os.RemoveAll(` is not a substring of that. The two sites the slice most wants policed were the two the guard was blind to, one of them `teardownHub`. Token is now `RemoveAll(`, and the seam moves into `destroy.go` (verified to have no other reference in the tree).
- **Q:** Which ownership kind does `teardownHub` declare? **A:** Neither obvious answer works. `ownedFabricHub` would refuse at 13 early-failure sites, since `looksLikeHub` needs a `_board` or weft sibling that does not exist yet — leaving "residual hub, remove it manually" where teardown works today. A bare trust-me closes the gap in name only. Resolved as containment plus a *structurally guaranteed* transaction identity: both clone forms prove non-existence (`clone.go:163`, `:211`) before `os.MkdirAll` (`:219`). That is strictly stronger than `looksLikeHub`, which pattern-matches a derived name — exactly what R4's `clone --reset` defect exploited.
- **Q:** Does the Out-section's "no behaviour changes" contradict the In-section's three gap closures? **A:** Yes, as written. Reworded to "other than the three named gaps"; the load-bearing part — every site keeps its dirtiness scope, and a test needing an edit is surfaced not edited — is kept.
- **Q:** Does `--force` satisfy containment? **A:** No, and it needed saying. `remove ..` is a containment failure, so a force-satisfies-containment reading returns the hub-destroying defect behind a flag. The rule is now: force answers dirtiness only, with a test per check.
- **Q:** Should the guard's substring brittleness be stated? **A:** Yes — repo convention. The gitrepo Client Boundary Invariant and the Fabric Vocabulary Invariant both carry explicit blind-spot notes. One imperative sentence in the new entry.
- **Q:** `checkout.go:193` and `remove.go`'s `worktree prune` deliberately discard errors — what happens when they become gate executors? **A:** One policy: an operational failure stays discardable on a documented best-effort path; a `*destructiveRefusal` never is. Distinguishable via `errors.As`, so it is expressible rather than aspirational.
- **Q:** The manifest's slice-12 open questions say the probe "is deliberately tracked-only" and the chokepoint should inherit that. **A:** That sentence over-generalised `prune.go`'s comment and is contradicted by the verified 4/4 split. Corrected in the same commit rather than merely diverged from, since a reviewer of this slice reads that section.

Resolved in discussion review round 1 (2 blocking, 2 nits — all verified against the tree before fixing):

- **Q:** The request structs list no `Location` and no junction-name set, yet the gate must resolve `primaryWeftBranch(l)` and `validateWorktreeSlug(slug, junctionNames)` itself. **A:** The inputs must reach the gate. *(First answered by putting `l` and `junctionNames` on both request structs — **superseded in round 2**, which found that clone's two hub-level sites have no Location to pass. Inputs now travel with the check that needs them; see the round-2 entry below.)* Either way this costs no hermetic coverage — the purity split is by predicate, not by field, and `looksLikeHub`/`refuseUncontainedPath`/`validateWorktreeSlug` are all git-free.
- **Q:** Is the link-removal site list reliable? **A:** No — it was wrong in both directions. `junction.go:259` is `os.Remove(link)`, not `fslink.Remove`, and four real `fslink.Remove(` sites were missing, including the whole of `unwire.go`. Root cause: the first pass enumerated from the manifest's prose rather than by grepping the actual token set. The enumeration method is now stated in Technical context, was re-run over the final token set, and every site carries a disposition.
- **Q:** Do any banned-token matches lack a disposition? **A:** Yes — `warpforward.go:34`, `hook.go:160` and `junction.go:259` would have landed the one-commit slice with a red guard. All three now have one: `Fabric.ResetHard` moves into `destroy.go` as the gated executor; the other two are allowlisted with reasons.
- **Q:** Is `.ResetHard(` the right token? **A:** No, and this is the mirror of the seam problem — it is too broad, not too narrow. Once `pull.go` calls the gated `f.ResetHard(...)`, `.ResetHard(` flags the correctly migrated code. `rawgitmutation_test.go:8-11` documents this exact trap in the file being cloned. Token is now `warp.ResetHard(` / `weft.ResetHard(`, which bans reaching past the gate to the raw handle.
- **Q:** Are the two junction re-point sites (`junction.go:161`, `:311`) destruction? **A:** They remove a link and recreate it in the same breath, so they are repair — but the judgement "this link is fabric's and it is broken" is exactly what R1's defect got wrong when `reconcile` destroyed a tracked symlink. Gated via a distinct `repointLink` executor with no `force` parameter.
- **Q:** `teardownHub` call-site count? **A:** 13, not 14. `grep -c "teardownHub(hubPath"` returns 14 because one match is the function declaration at `clone.go:604`. Corrected in all five places.

Resolved in discussion review round 2 (2 blocking, 3 nits — all verified against the tree before fixing):

- **Q:** Clone's two hub-level gate sites run where no `*lyxcwd.Location` exists — `CloneHub` takes only `cwd string`, `resetHub` fires at `clone.go:159`/`:204` and `teardownHub` from `:243`, while `lyxcwd.Resolve` is not reached until `:369`. What do they pass? **A:** Nothing, because `l` comes off the request entirely. Each check's inputs now travel with the check: ownership kinds are parameterised by exactly what they need, and the two kinds clone uses (`ownedFabricHub`, `ownedFreshlyCreatedPath`) need no repo context. A required top-level `l` would have forced `nil` or a synthetic partial at those sites — the trust-me hole in another costume. Slug validation travels the same way, as `slug *slugSpec{name, junctionNames}`.
- **Q:** What are `resetHardTo`'s container, target and ownership kind? **A:** Hub, warp worktree path, and a **new** `ownedWarpCheckout(repoDir)`. This one matters: `isRegisteredLinkedWorktreeIn` skips `entry.Main` (`remove.go:229-231`) on purpose, so reusing it would refuse `lyx fabric pull` in the hub's prime warp worktree — the ordinary case. `ownedWarpCheckout` is the same membership test *including* main. Dirtiness is `dirtyScopeTracked`, force always false.
- **Q:** A one-argument `Fabric.ResetHard(sha)` cannot let its callers declare ownership or dirtiness. **A:** It should not. There is exactly one correct declaration for "reset this Fabric's warp checkout", so the exported signature is kept and the wrapper hardcodes all five fields. Only the body and the file move. `warpforward.go`'s other three delegations are untouched — none is a destructive primitive.
- **Q:** Is `internal/fabricengine/doc.go` safe from its own guard? **A:** No — this slice writes destruction rationale into it, and the scan is a raw substring match over every non-test `.go` file, so a prose `os.RemoveAll(` would trip it from inside the document explaining the rule. Allowlisted, with the reason that its only non-comment line is `package fabricengine`.
- **Q:** Does Markdown Link Integrity apply? **A:** Yes — both manifest files this slice edits are scan sources. Added to Constraints.

Resolved in discussion review round 3 (2 blocking, 5 nits — all verified against the tree before fixing):

- **Q:** The closed enum has no link-shaped kind, yet six gated link sites need one — the tables deferred to each site's existing `fslink.IsLink` / target-comparison refusal, which is no enum member and re-admits per-site checks. **A:** Two link kinds added. Link-ness alone cannot be ownership: **R1's defect destroyed a tracked symlink**, and a user's tracked symlink is a link. What separates fabric's junction is *where it sits* — a path fabric wires. `ownedWiredJunction(expectedTarget)` for teardown (link + wired location + resolves to target); `ownedDriftedWiredJunction` for re-point, which deliberately does *not* compare the target, since drift is the precondition for repair. Two kinds because the comparison is inverted, not optional.
- **Q:** `ownedFreshlyCreatedPath` takes no repo context, so the gate executes nothing for it — what stops a future site declaring it? **A:** Nothing did, and that made it the `ownedInTransaction` trust-me under a better name. Fixed by having the gate mint the claim: `destroy.go` gains `createExclusiveDir(path) (createdToken, error)`, absorbing `CloneHub`'s existing stat-then-`MkdirAll` sequence, and the kind accepts only that unexported token. A site cannot declare it for a path the gate did not create — impossible to write rather than discouraged. The gate now owns the creation whose destruction it later authorises.
- **Q:** Do all gated sites name an ownership kind? **A:** No — `rollbackAdd`, `rollbackSwitch`, `add.go:265` and `launchers.go:165,170` had none, and `ownedHubGeometryChild` was defined but mapped to no site. Every disposition table now carries an ownership-kind column. This also surfaced a needed `ownedFreshlyCreatedWorktree(tok)` for `add.go:265`'s rollback of the worktree that same `Add` created.
- **Q:** `ownedManagedWeftBranch(l)` or `(weftRepoRoot)`? **A:** `(l *lyxcwd.Location)`. A bare repo root cannot resolve `primaryWeftBranch`, which is the kind's whole point; `WeftRepoRoot(l)` derives the root from the Location anyway.
- **Q:** Three statements still described superseded models — the Constraints bullet ("the gate takes a `*lyxcwd.Location`"), and the Q&A log's first two entries (single struct, three-member dirtiness enum). **A:** All three corrected, the Q&A ones marked as superseded rather than rewritten, matching how the round-1 entry was handled.
- **Q:** Does `branchRequest` have "no path fields at all"? **A:** No — it carries `repoDir`. Narrowed to the accurate claim: no `container` and no `target`.

Resolved in discussion review round 4 (3 blocking, 3 nits — all verified against the tree before fixing):

- **Q:** The link kinds state the rule as "a link **and** at a location fabric wires", but neither kind receives the wired-location half. **A:** Correct, and it made `ownedDriftedWiredJunction` degenerate to bare link-ness — the R1 rule the decision explicitly rejects. Both kinds now take `wiredLinks []string`, and the document names the helper producing it at each of the six sites.
- **Q:** Where exactly does `createExclusiveDir` land? The stat guards and `os.MkdirAll` are not adjacent — `probeWeftBinding` runs between them, and the offline-before-network ordering is documented as deliberate at `clone.go:208-211`. **A:** One call at `clone.go:220`, replacing `os.MkdirAll`; the two offline stat guards stay untouched. Creating early leaks a residual hub when the probe fails; folding the guards in defers the offline refusal past a network call. This placement preserves both existing messages verbatim and, using `os.Mkdir`'s `EEXIST` rather than `MkdirAll`, closes a real TOCTOU window that exists today between `:163` and `:220`.
- **Q:** What does `ownedHubGeometryChild` verify? **A:** It was a label with no predicate — the one kind that never named what the gate runs. Renamed `ownedUnderGeometryRoot(root)` and defined: `root` must be a member of a closed set of fabric geometry roots, and the target at or below it. "Child" was also literally false — `removeLaunchers` deletes script files three or more levels below `launchersDir(l)` on a subpath-anchored hub. The kind supplies what containment cannot: **containment proves the target is below the container, but proves nothing if the caller chose the container.** Every kind now has a stated predicate, in a table.
- **Q:** Do the disposition tables declare dirtiness? **A:** Only four sites did. Dirtiness is a required field, so the same gap round 3 closed for ownership was still open here. Every gated row now carries a dirtiness column, with the reason string spelled out for each `dirtinessNA`.
- **Q:** `removeDir` for file removals? **A:** Renamed `removePath`. `removeLaunchers` deletes the `ide` and `fabric-checkout` script files, not only directories.
- **Q:** Line citations. **A:** The one-argument stat guard is `clone.go:212` and `os.MkdirAll` is `:220`; both were cited one line low. Corrected everywhere.
