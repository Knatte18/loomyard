# Discussion: landing: parent-fabric resolution chain

```yaml
task: 'landing: parent-fabric resolution chain'
slug: landing-parent-fabric-resolution-chain
status: discussing
parent: main
```

## Problem

`internal/landingshed.Deps` carries three closures a caller must fill before `landingshed.NewPublish`/`NewFinalize` can be constructed: `OpenFabric`, `OpenParentFabric`, and `PushBranch`.
Today no production caller fills them.
`internal/loomcli/wiring.go` deliberately leaves `shedrecipe.Env.Landing` entirely unfilled, and says so in a comment pointing at this roadmap item;
`internal/landingshed/deps.go`'s own field doc says the resolution chain "belongs to the layer that legitimately resolves geometry, and the next roadmap item builds it".
This is that item.

Why now: `loom` cannot complete an end-to-end run until it lands.
The `Publish` and `Finalize` rows exist in loom's recipe and are constructible in tests only — the moment a real run reaches them, `shedbuild.Build` fails at construction for want of the closures.
The five `loom: real LLM producers` tasks are explicitly *not* blocked on this, so it is the single remaining gap between loom's current state and a run that can actually finish.

## Scope

**In:**

- A new exported `fabricengine.OpenParent(l *lyxcwd.Location, parentBranch string) (*Fabric, error)` implementing the full four-step chain: list the hub's worktrees, match the entry whose branch equals `parentBranch`, resolve that worktree's path to a `*lyxcwd.Location`, and open its pair.
- An unexported matching helper beside it, re-exported through `internal/fabricengine/export_test.go` so the branch-matching logic is unit-testable without a hub fixture.
- Two new vocabulary-neutral `Fabric` methods, both justified by the same `fabric.go` carve-out: `OriginURL()` wrapping the warp side's existing `gitrepo.Repo.RemoteURL("origin")`, and `PushBranch(opts SyncOptions)` delegating to `PushWarpRebaseFreeAt`.
- A new `loomengine.LoomScratchDir(l *lyxcwd.Location) string` accessor returning loom's ephemeral directory, plus the correction of `LoomStatusLock`'s now-false doc comment.
- Filling `shedrecipe.Env.Landing` — the whole `landingshed.Deps` struct, not only the three closures — in `internal/loomcli/drive.go`, immediately before the `loomrecipe.New` call.
- Comment corrections in `internal/landingshed/deps.go` where it defers the chain to "the next roadmap item".
- Tests: an integration test for `OpenParent` via `hubforge`, and a unit test for the matching helper.
- Docs: `manifest/designs/loom.md`, `manifest/roadmap.md` (move the item to Done), and the affected package docs.

**Out:**

- Any change to `landingshed`'s production logic.
  Only comments change there;
  its import allowlist in `seam_enforcement_test.go` needs no new entry, because every new symbol lives in `fabricengine`, `loomengine`, or `loomcli`.
- Tightening `NewPublish`/`NewFinalize` validation (e.g. rejecting an empty `ParentBranch` or `OriginURL`).
  Real hardening, but scope beyond this roadmap item.
- Adding a worktree-listing helper.
  `fabricengine.List` already exists and is already the single shared `git worktree list --porcelain` parser — see Technical context.
- Materializing a parent pair when none exists.
  `Finalize` never creates a worktree to merge into;
  that stays a separate command's job and a human's decision.
- Filling `Env.StencilsDir`, `Env.RunRoot`, `Env.Burler`, or `Env.Now`.
  Those belong to the `loom: real LLM producers` tasks, and no row in loom's recipe reads them yet.
- Any end-to-end `lyx loom drive` test that reaches a real `Publish` run.
  That needs GitHub credentials and would prove construction, not behaviour.

## Decisions

### chain-lives-in-fabricengine

- Decision: the whole resolution chain lives in `internal/fabricengine` as one exported function, `OpenParent(l *lyxcwd.Location, parentBranch string) (*Fabric, error)`, placed beside `List`/`PrimeName` in `worktreelist.go`.
- Rationale: the Cwd Resolution Invariant assigns this territory to fabric explicitly — "Weft-sibling paths and junction construction belong to `internal/fabricengine`, never `lyxcwd`: … and the `Prime`/sibling-worktree-list lookup they're built from, are `fabricengine`-private."
  Keeping all four steps inside fabric makes the chain reusable verbatim by the Someday `Hardener` product, whose own producer list shares `Publish`/`Finalize` by reference.
  The `loomcli` closure then collapses to one line.
- Rejected: a `hubgeom.LandingGeometry(l)` adapter — `landingshed.Deps` is not a `Geometry` struct (it also carries `Shuttle`, `Registry`, `Config`), so the adapter would have to return a partial `Deps` or a bare closure triple, neither of which matches the existing `ReedGeometry`/`WebsterGeometry`/`BurlerGeometry` shape.
  Also rejected: inlining the chain in `loomcli/wiring.go` — cheapest, but buries a reusable chain where a second product cannot reach it.

### open-returns-a-pair-handle-not-a-path

- Decision: `OpenParent` returns an opened `*fabricengine.Fabric`, not a path.
- Rationale: `Finalize` holds the result behind its `parentMerger` seam and calls `parentHandle.Merge(taskBranch, MergeOptions{Squash})`.
  That merge must execute inside the parent's own checkout, coordinated across both sides — a bare path cannot express that, and `Fabric.Merge` is the only thing that can.
- Rejected: a `ParentWorktreePath` primitive with the caller opening — pushes two of the four steps back out to every caller, defeating the reuse goal;
  no second consumer exists for the path alone today.

### env-landing-filled-in-drive-not-wire

- Decision: `Env.Landing` is assembled in `internal/loomcli/drive.go`, immediately before `loomrecipe.New(c.env, c.shedPaths)`, not in `wire()`.
- Rationale: `NewPublish` and `NewFinalize` both call `deps.OpenFabric()` **eagerly** at construction, and `shedbuild.Build` calls every row's constructor.
  `wire()` is the persistent pre-run for *every* loom verb, including `status` and `pause`.
  Filling it there risks exactly the failure `wire()`'s own `OpenBisector` comment already guards against: "opening stat-checks the paired sibling, and this pre-run must not fail 'status'/'pause' against a healthy-but-unwired location."
  `drive` is the only path reaching `loomrecipe.New`, and it already refuses up front when the status file is absent, so a wired worktree is guaranteed there.
- Rejected: filling in `wire()` alongside the rest of `c.env` — tidier, but only accidentally safe, and it would break the moment another verb constructs the recipe.
  Also rejected: a split fill across both sites, which leaves `Env.Landing` half-populated between them.

### parent-branch-from-origin-record

- Decision: `Deps.ParentBranch` comes from `fabricengine.ReadOrigin(c.location)`'s `Origin.ParentBranch`.
- Rationale: the origin record is written once at pair-creation time and read thereafter, never inferred.
  `run.go:76` already reads it exactly this way, so this is established precedent rather than a new source of truth.
- Rejected: reading `loomengine.Status.Parent` from the seeded status file — it does reflect a `--parent` flag override, but `resolveParentBranch` already refuses to let the flag and the record disagree, so the divergence the fallback would cover cannot occur.
  Also rejected: reading the record with the status file as fallback — complexity for that same impossible divergence.

### drive-refuses-an-unrecorded-parent

- Decision: `drive.go` disposes of all three `ReadOrigin` returns explicitly, by calling `resolveParentBranch(recorded, found, "")` — the existing pure function — with an empty flag, and surfacing its error on the envelope verbatim.
  The `error` return from `ReadOrigin` itself propagates as an ordinary envelope error before that call is reached.
- Rationale: `ReadOrigin` is `(Origin, bool, error)`, and a `false` bool is the documented legacy-worktree case that is explicitly *not* an error.
  Without a stated disposition, an absent or empty `ParentBranch` would flow into `Deps.ParentBranch`, become the pull request's base branch, and reach `OpenParent`'s matcher as `""` — matching nothing and surfacing much later as a `Finalize` `Stuck`, far from the actual cause.
  Rejecting an empty `ParentBranch` inside `landingshed` is out of scope by the `landingshed-comment-only` decision, so nothing else in the chain catches it;
  `drive` is the last place it can be caught cleanly.
  Passing an empty flag drives `resolveParentBranch`'s table straight to its final row, which already handles absent-record and present-but-empty identically ("A present-but-empty recorded value is treated exactly as an absent record throughout") and already emits the right remedy: *"no recorded parent branch for this worktree pair; pass --parent once to record it"*.
  Reusing it adds no new error text and cannot drift from `run`'s own rule.
  It also matches `drive`'s existing refusal style, which already refuses on the envelope when the status file is absent and names `lyx loom run` as the remedy.
- Rejected: a fresh `drive`-specific refusal message naming `lyx loom run --parent <branch>` — marginally more directive, at the cost of stating the same rule in two places that can drift.
  Also rejected: falling back to the seeded status file, which reverses `parent-branch-from-origin-record`.

### landing-config-loads-in-wire

- Decision: `landingshed.LoadConfig(anchorPath, "landing")` is called in `wire()`, alongside the four module configs it already loads, and stored on the `loomCLI` struct.
  `drive.go` reads it off `c` rather than loading it.
  This is a deliberate carve-out from `env-landing-filled-in-drive-not-wire`, which stays in force for everything else.
- Rationale: `drive` is not normally an operator-facing command.
  `run` spawns it as a **detached child** with stdout and stderr redirected to `LoomDriverLog`, so a `drive` envelope error is written to a log file, not the operator's terminal.
  What the operator sees on the normal `lyx loom run` path is `awaitRunLock` failing and the generic message *"loom: driver did not take the run lock; see &lt;log&gt;"*.
  Every pre-existing `drive` precondition is safe from this because `run` guarantees it first — `run` seeds the status file, and `wire()` loads loom's, reed's, shuttle's and webster's configs before either verb proceeds.
  `landing.yaml` would be the **first** precondition not guaranteed anywhere, so an unreconciled hub would turn a one-line, self-remedying config error into "the driver died, go read a log".
  Loading it in `wire()` puts the message back on the operator's own terminal, for every verb.
- Why the cost is acceptable: `wire()` already performs a strict load that fails identically on a hub missing its config — `loomengine.LoadConfig` — so `status` and `pause` already refuse on an unreconciled hub today.
  Adding `landing.yaml` widens the set of files that must exist, not the set of commands that can fail, and `landing` is already a registered `configreg` module, so `lyx config reconcile` writes it with everything else.
  This carve-out does not weaken the original decision: what that decision protects against is the **eager `OpenFabric()`** at producer construction, which stat-checks a possibly-unwired pair.
  A config load performs no fabric I/O and opens nothing, so moving it earlier is safe in exactly the way moving the closures earlier is not.
- Concrete cost to name in the plan: `internal/loomcli/wiring_test.go` drives `wire` against a hand-built location with a seeded config set, so `landing.yaml` must be added to that seed set or the existing test fails.
- Rejected: leaving the load in `drive.go` and accepting the log-only symptom — cheapest, but it degrades a precise, self-remedying error into a generic driver-death message on the path operators actually use.
  Also rejected: a lenient/degrading load — `landingshed`'s package doc records that strictness is required rather than chosen ("neither producer in this package has a standalone entry point, both are reachable only inside a hub, and there an absent config means the hub is broken").

### assembly-seam-takes-plain-values

- Decision: the `Env.Landing` assembly function takes already-resolved plain values (`taskBranch`, `originURL`, `parentBranch`, the geometry, the registry, the runner, the config, and the location) rather than the opened `*fabricengine.Fabric`.
  It performs no I/O and returns no error.
  `drive.go` does the handle reads and both refusal guards above the call.
- Rationale: this is what makes the drift-guard test tier 1.
  Were the handle a parameter, a non-zero `TaskBranch`/`OriginURL` would be unreachable without a real wired hub — `newPaired` stat-checks both sides, `CurrentBranch()` and `RemoteURL("origin")` read real git state, and `landingshed.LoadConfig` goes through strict `configengine.Load` where an absent config file is an error.
  The test would then need `//go:build integration`, a `hubforge` fixture, and a `TestMain` calling `gitkit.HermeticGitEnv` — dragging a pure struct-assembly assertion into the expensive tier for no gain.
  Splitting the I/O out keeps the assertion where it belongs and preserves `internal/loomcli`'s existing tier-1 posture, which `wiring_test.go`'s own header comment is explicit about protecting.
- Rejected: passing the handle and tagging the drift guard as an integration test with a hubforge fixture — it works, but pays fixture cost for an assertion about field population, and pulls a tier-1 package toward tier 2.

### scalar-read-errors-refuse-or-defer-by-consumer

- Decision: `drive` disposes of the two eager scalar reads differently, by who consumes them.
  A `CurrentBranch()` error refuses on the envelope.
  An `OriginURL()` error does **not** refuse — `drive` passes the empty string through and lets `Publish` be the layer that refuses.
- Rationale: `TaskBranch` is consumed by both producers — it is `Finalize`'s merge source and `Publish`'s pull-request head — so an unreadable branch (detached HEAD, unborn HEAD) makes the whole landing segment meaningless, and refusing early names the cause precisely.
  `OriginURL` is different: only `Publish` reads it, and only on the path where `require_pr_to_base` actually contains the parent branch.
  `gitrepo.RemoteURL` errors outright when no `origin` remote is configured, and a remote-less warp checkout is genuinely reachable — a purely local hub with no upstream.
  Refusing `lyx loom drive` there would block a run that never needed the value, for a `Publish` row that would have returned `Done` immediately.
  Passing the empty string through costs nothing and lands the refusal exactly where the value is used: `githubclient.ParseOwnerRepo("")` fails, and `Publish` already turns that into `stuckOrCancelled("origin URL unusable: …")` — a stuck row with a precise reason, not a crash.
- Rejected: refusing on both — symmetric and simpler to state, but it makes a local-only hub unable to run loom at all over a value its own config says is unnecessary.
  Also rejected: passing both through empty — an empty `TaskBranch` would reach `Finalize`'s merge and `Publish`'s pull-request head with no layer positioned to catch it.
- `fabricengine.Open(c.location)` failing refuses on `drive`'s envelope: the pair is not wired, and no producer in the list can run without it.
- The landing config load is different, and it is the one part of `Env.Landing`'s assembly that **does** belong in `wire()` — see `landing-config-loads-in-wire`.

### self-parent-is-loom-policy-not-fabric-policy

- Decision: `fabricengine.OpenParent` matches on branch alone and special-cases nothing;
  if the parent branch names the acting worktree's own branch, it returns that worktree's own pair, correctly and without complaint.
  `drive.go` refuses when `parentBranch == taskBranch`, as a second clause of the same guard that implements `drive-refuses-an-unrecorded-parent`.
- Rationale: the case is reachable — `resolveParentBranch` does not reject `--parent <own-branch>`, so a mis-repaired legacy worktree can record itself as its own parent — and if it reached `Finalize`, `parentHandle.Merge` would merge a branch into itself.
  But "a task may not be its own parent" is loom's policy, not a truth about opening a pair by branch name.
  `OpenParent` is meant to be reusable verbatim by the Someday `Hardener` (the whole reason for `chain-lives-in-fabricengine`), and baking one consumer's policy into a generic helper is what would break that reuse.
  Putting the refusal in `drive` beside the unrecorded-parent refusal means both provenance-record defects are caught in one place, with one shape, at the point the record is read.
- Rejected: refusing inside `OpenParent` — it cannot be forgotten by a future second caller, but it makes a generic helper enforce a consumer's rule.
  Also rejected: matching self and proceeding — git would report already-up-to-date and nothing would crash, but a corrupt provenance record would be silently accepted rather than named.

### no-match-is-a-plain-error-becoming-stuck

- Decision: when no worktree's branch matches `parentBranch`, `OpenParent` returns an ordinary error.
  The producers turn it into `Stuck`.
- Rationale: `Finalize.Call` already does exactly this — its `parentOpener()` error path formats `"no live pair for parent branch %q: %v"` and routes through `stuckOrCancelled`.
  A missing parent pair is an operator-fixable condition, not a crash, and the handling already exists.
- Rejected: a typed sentinel such as `ErrNoParentWorktree` — more precision than any current call site consumes.
  Also rejected: failing at construction, which would turn a legitimately transient condition into a build-time refusal.

### push-uses-the-rebase-free-warp-primitive

- Decision: `PushBranch` is `func() error` wrapping `fabricengine.PushWarpRebaseFreeAt(c.location.WorktreePath(), fabricengine.EnvSyncOptions())`, discarding the `PushResult`.
- Rationale: `Publish` already has an explicit `errors.Is(err, gitrepo.ErrPushRejected)` branch that reports `Stuck "push rejected by the remote"`.
  Only `gitrepo.PushRebaseFree` ever produces that sentinel;
  `PushWarpAt` routes through `PushCoalesced` → `pushWithRebaseRetry`, which never returns it, so choosing `PushWarpAt` would leave that branch permanently dead code.
  `PushWarpRebaseFreeAt`'s own doc states the contract this needs: "A rejected push surfaces as `gitrepo.ErrPushRejected`, which the caller is expected to treat as a human-decidable condition rather than retrying."
  It additionally discharges two hazards rather than mitigating them — it never rewrites this side's SHAs while the paired side is not rebased (which would invalidate the correspondence index), and it takes no push lock, so it leaves no untracked `.gitrepo-push.lock` residue in the operator's warp repo.
  The sentinel is returned bare by `PushRebaseFree` and passed through unwrapped by `PushWarpRebaseFreeAt`, so `errors.Is` still matches across the closure boundary.
- Rejected: `PushWarpAt` — pairs neatly with `PushSkipped` from the same `EnvSyncOptions()`, but leaves `publish.go`'s rejection handling unreachable and reintroduces the rebase hazards.
  Also rejected: `CoalescePushBothAt`, which pushes both sides and contradicts `publish.go`'s stated rule that only the externally visible branch is pushed here.

### push-verb-gets-a-neutral-fabric-method

- Decision: add a vocabulary-neutral method `Fabric.PushBranch(opts SyncOptions) (PushResult, error)`, delegating internally to `PushWarpRebaseFreeAt(f.warpPath, opts)`.
  `drive.go`'s closure body calls `handle.PushBranch(...)` and never names the underlying verb.
- Rationale: the bare warp/weft ban is **not** `landingshed`-specific, and an earlier version of this decision was wrong to assume it was.
  `fabricVocabularyOwners` (`internal/lyxcwd/enforcement_test.go:597`) is exactly `{fabricengine, fabriccli, weftname, gitkit, boardengine, configsync, hubforge}`;
  `internal/loomcli` is not in it.
  `bareVocabularyToken` (`:654`) is a case-insensitive **substring** test, and `fabricVocabularyHits` (`:699`) walks every `*ast.Ident` — which includes a selector expression's `Sel`.
  So `fabricengine.PushWarpRebaseFreeAt(...)` written anywhere in `internal/loomcli` fails `TestEnforcement_FabricVocabulary`, whose walk covers all production `.go` under `internal` and `cmd` (`:907`).
  There is therefore no valid caller for the closure as originally framed: `landingshed` may not name the verb, and neither may `loomcli`.
  The neutral method resolves it inside the one package that *is* an owner.
  It also reuses the identical carve-out already invoked for `Fabric.OriginURL()` — `fabric.go`'s rule that a single-sided uncoordinated op earns a named `Fabric` method precisely when it must be callable from outside the package — so the two additions land as one consistent pair, and `drive.go` already holds the handle both need.
  Naming it `PushBranch` matches `landingshed.Deps.PushBranch` exactly, so the closure reads as a straight pass-through.
- Note: this changes only the *spelling* and call site.
  The rebase-free semantics chosen in `push-uses-the-rebase-free-warp-primitive` are unaffected — `Fabric.PushBranch` delegates to that same primitive, and `gitrepo.ErrPushRejected` still propagates unwrapped through both the method and the closure, so `Publish.Call`'s `errors.Is` check keeps matching.
- Also note: `Deps.PushBranch` remains an injected closure rather than a direct call.
  That is still correct and still required — `landingshed` may not import a verb it cannot name, and the field doc's reasoning stands even though its phrase "the layer that names it is the caller" now resolves to `fabricengine` rather than to `loomcli`.
- Rejected: a package-level `fabricengine.PushTaskBranchAt(path, opts)` — same delegation and no handle required, but it widens the package surface and re-passes a path the handle already carries.
  Also rejected: adding `internal/loomcli` to `fabricVocabularyOwners` with the `CONSTRAINTS.md` amendment it implies — it would grant loom permission to know weft exists, which is exactly what the Fabric Vocabulary and Hub Containment rules exist to prevent, and it would make this discussion's "no `CONSTRAINTS.md` change expected" false.

### loom-owns-its-scratch-dir

- Decision: add `loomengine.LoomScratchDir(l *lyxcwd.Location) string` returning `filepath.Join(l.AnchorPath(), lyxdirs.DotLyxDirName, loomDirName)`, and use it for `Deps.ScratchDir`.
- Rationale: the Durable-vs-Ephemeral State Invariant requires that "No engine derives its own `.lyx` path — each module exposes a scratch accessor beside its durable one."
  This is the directory `LoomRunLock`, `LoomDriverLog`, and `LoomBootstrapLock` already live in, so the accessor names an existing directory rather than introducing one.
  `LoomStatusLock`'s doc comment currently asserts "loomengine has no `Dir(l)` accessor for a `ScratchDir(l)` to mirror" — that becomes false and must be corrected in the same commit.
- Rejected: reusing `websterGeom.ScratchDir` — zero new code, but it files landing's stuck-reason artifacts and the resolver's conflict reports under webster's directory, a lie about ownership.
  Also rejected: `fabricengine.HubScratchDir(hub)`, which is hub-wide where these artifacts are per-task.

### origin-url-through-a-fabric-method

- Decision: add `Fabric.OriginURL() (string, error)` delegating to the warp side's `RemoteURL("origin")`.
- Rationale: `gitrepo.Repo.RemoteURL` already exists with **zero** production callers — it was built for this field and nothing else.
  `fabric.go`'s own doc records the carve-out that applies: "A single-sided, uncoordinated op also earns a named `Fabric` method — rather than staying direct field access — precisely when it must be callable from OUTSIDE this package, so the one-repo illusion holds at the public API boundary."
- Rejected: `gitrepo.New(warpPath).RemoteURL("origin")` at the call site — permitted, since the Fabric Git Invariant exempts read-only verbs, but it makes `loomcli` name a warp path and reach past `Fabric` for something `Fabric` should answer.
  Also rejected: a `landing.yaml` config key, since geometry is structural and never config-overridable, and the remote is already in git config.

### task-branch-is-read-not-derived

- Decision: `Deps.TaskBranch` comes from `Fabric.CurrentBranch()`, not from `c.location.WorktreeName`.
- Rationale: fabric's `Add` supports a `branch_prefix` config key, so the warp branch is not guaranteed to equal the worktree name.
  Only the default empty prefix makes them coincide.
  Reading the branch is correct under every prefix setting;
  deriving it from the worktree name is correct only by accident.
- Rejected: deriving from `WorktreeName` — silently wrong for any hub configuring a branch prefix.

### two-opens-in-drive-rather-than-a-shared-handle

- Decision: `drive.go` opens one `*Fabric` of its own for the two scalar reads (`CurrentBranch()`, `OriginURL()`), and both `OpenFabric` and `OpenParentFabric` stay genuine closures rather than returning an already-open handle.
- Rationale: laziness is *not* what distinguishes the two options for `OpenFabric` in loom's own path — `NewPublish` and `NewFinalize` both call `deps.OpenFabric()` eagerly at construction, and `drive.go` opens its handle at the same site it would have cached, so in this call path both options open eagerly and the laziness argument discriminates nothing.
  Two things do still bind.
  First, `OpenParentFabric` is genuinely lazy: `Finalize` calls it per-`Call` through its `parentOpener`, never at construction, so caching a parent handle would change real behaviour by pinning a pair opened before the run's own merge-in.
  Second, a cached `OpenFabric` would make the field a misnomer — it would no longer open anything — and `landingshed` is shared by reference with the Someday `Hardener`, whose preflight ordering may differ from loom's.
  A second `Open` is cheap regardless: two stat checks plus two `gitrepo.New` calls, no git subprocess.
- Note for the implementer: `deps.go`'s own laziness comment ("opening eagerly would fail before the run's own preflight has confirmed anything is wired") reads as absolute, while `publish.go`'s constructor doc already carves out why construction-time opening is nonetheless correct there.
  If that tension is worth resolving, it is a third comment correction in the same commit — but the correction is to `deps.go`'s wording, never to the closure's shape.
- Exactly two `Open` calls exist in this path, and no third: `drive.go`'s own handle (used for `CurrentBranch()`, `OriginURL()`, and the prebuilt push closure), and whatever `OpenFabric`/`OpenParentFabric` open when the producers call them.
  The push closure specifically must **not** open a third time — it closes over `drive.go`'s handle, which is why `pushBranch` is passed into the assembly seam prebuilt rather than constructed from `l` inside it.
- Rejected: caching the handle into the closure — saves negligible work, breaks `OpenParentFabric`'s real per-`Call` semantics, and makes `OpenFabric`'s name false.

## Technical context

### The roadmap's premise is partly stale

`manifest/roadmap.md` states that "no worktree-listing helper exists yet in `internal/gitrepo`/`internal/fabricengine`, and one must be added to do the matching."
This is no longer true.
`internal/fabricengine/worktreelist.go` already provides `List(sourceDir string) ([]WorktreeEntry, error)`, described in its own header as "the single porcelain parser shared across the codebase", returning `{Path, Head, Branch, Main}` per entry with `refs/heads/` already trimmed from `Branch` and a detached entry marked `"(detached)"`.
The listing half of the task is done;
what remains is matching, resolving, and opening on top of it.
The roadmap entry should be corrected when the item moves to Done.

### The four steps, concretely

Given the task worktree's `l` and `parentBranch = "main"`:

1. `List(l.AnchorPath())` runs `git worktree list --porcelain`.
   All worktrees of a repo share one git dir, so running it from the task's own warp checkout lists every warp worktree in the hub, including the prime one sitting on the parent branch.
2. Match `entry.Branch == parentBranch`.
   Normalize the porcelain path with `filepath.FromSlash` before use — git may emit forward slashes, and `PrimeName` in the same file already does exactly this.
3. `lyxcwd.ResolveWorktree(matchedPath)` yields the parent worktree's own `*lyxcwd.Location`.
   `ResolveWorktree`, not `Resolve`: we hold a worktree root rather than an acting cwd, and the strict cwd gate would reject it.
   Its doc names this case outright — "It exists for callers holding a worktree root (not an acting cwd) where the gate would spuriously fire."
4. `fabricengine.Open(parentLocation)` calls `newPaired(parentLoc.WorktreePath(), WeftWorktree(parentLoc))`, which stat-checks both sides exist and returns the `*Fabric`.

Git forbids the same branch being checked out in two worktrees, so at most one entry can match — no ambiguity handling is needed.
Detached entries carry `Branch == "(detached)"` and cannot collide with a real branch name.

### Where each `Deps` field comes from in `drive.go`

`landingshed.Deps` has fourteen fields and **all** of them must be filled — the task is not only the three closures, because `Env.Landing` is currently the zero value in its entirety.

| Field | Source |
| --- | --- |
| `WorktreeRoot` | `c.location.WorktreePath()` |
| `TaskBranch` | `handle.CurrentBranch()`, refusing on the envelope on error |
| `ParentBranch` | `fabricengine.ReadOrigin(c.location)` → `resolveParentBranch(recorded, found, "")`, refusing on the envelope when unrecorded or empty |
| `WebsterDir` | `c.runDeps.Geom.WebsterDir` |
| `StencilsDir` | `c.runDeps.Geom.StencilsDir` |
| `ScratchDir` | new `loomengine.LoomScratchDir(c.location)` |
| `OriginURL` | new `handle.OriginURL()`, passing `""` through on error so `Publish` refuses instead of `drive` |
| `PushSkipped` | `fabricengine.EnvSyncOptions().SkipPush`, from the single `EnvSyncOptions()` call `drive.go` also builds the push closure's `opts` from |
| `PushBranch` | prebuilt in `drive.go` as a closure over the already-open `handle.PushBranch(opts)`, discarding the `PushResult`; passed into the assembly seam, never constructed inside it |
| `OpenFabric` | closure over `fabricengine.Open(c.location)` |
| `OpenParentFabric` | closure over new `fabricengine.OpenParent(c.location, parentBranch)` |
| `Shuttle` | the `*shuttleengine.Runner` already built in `wire()` — a compile-time assertion that it satisfies `mergeresolve.Shuttle` already exists at `mergeresolve/deps.go:46` |
| `Registry` | the `modelspec.Registry` already loaded in `wire()` |
| `Config` | `c.landingCfg`, loaded by `wire()` via `landingshed.LoadConfig(anchorPath, "landing")` — `"landing"` is already a registered config module in `internal/configreg/configreg.go:47` |

Three of these values are `wire()` locals today, and they are reachable from `drive.go` by two different routes — do not conflate them.

`websterGeom` is a `wire()` local too, but it needs no new field: `wire()` already stores it as `runDeps.Geom`, and `c.runDeps` is on the struct.
So both `WebsterDir` and `StencilsDir` come off `c.runDeps.Geom` with no change to `wire()` at all.
`hubgeom.WebsterGeometry` builds `StencilsDir` as `fabricengine.StencilsDir(l.HubPath)`, so reading it from `c.runDeps.Geom.StencilsDir` yields exactly that value while keeping one source rather than two spellings of it.

`registry` and `runner` are the genuine gap: `registry` is consumed only to build `roles`, and `runner` only to build `runDeps`, so neither survives on `c` in any form.
Both must be carried onto the `loomCLI` struct (as `c.registry` and `c.runner`, or equivalent).
Those two fields are the only structural change to `wire()`/`cli.go` that filling in `drive.go` implies.

### Eager construction is confined to `drive`

`shedbuild.Build` calls every registry row's constructor, and its own doc notes it "is not filesystem-free: it is a pass-through for the construction-time filesystem effects some registry constructors have of their own accord".
`publishEntry`/`finalizeEntry` are two such rows.
`loomrecipe.New` — the only caller of `shedbuild.Build` — is invoked from `drive.go:45` alone.
Nothing in `status`, `pause`, or `run` reaches it, which is what makes the eager `OpenFabric()` acceptable once `Env.Landing` is filled at that site.

### Files that will be touched

- `internal/fabricengine/worktreelist.go` — add `OpenParent` and the unexported matcher.
- `internal/fabricengine/export_test.go` — re-export the matcher for unit testing.
- `internal/fabricengine/fabric.go` — add `OriginURL()` and `PushBranch(opts)`, the two neutral-named methods.
- `internal/loomengine/config.go` — add `LoomScratchDir`, correct `LoomStatusLock`'s doc comment.
- `internal/loomcli/wiring.go` — carry `registry` and `runner` onto the struct, add the `landingshed.LoadConfig` call, and replace the "Landing is left unfilled" comment block.
- `internal/loomcli/wiring_test.go` — add `landing.yaml` to the seeded config set, or the existing tier-1 test fails on the new strict load.
- `internal/loomcli/cli.go` — the three new struct fields (`registry`, `runner`, `landingCfg`).
- `internal/loomcli/drive.go` — the handle reads, `resolveLandingParent`, and the `landingDeps` call before `loomrecipe.New`.
- `internal/loomcli/seedinput.go` (or a sibling) — the new `resolveLandingParent` pure helper, beside the existing `resolveParentBranch` it wraps.
- `internal/landingshed/deps.go` — correct the `OpenFabric`/`OpenParentFabric` field doc's deferral to "the next roadmap item", plus its laziness wording per the `two-opens-in-drive-rather-than-a-shared-handle` decision's implementer note.
  That field doc is the only deferral in this file;
  the second one lives in `internal/loomcli/wiring.go`, already listed separately above.

Docs, required by the Documentation Lifecycle in the same commit:

- `manifest/roadmap.md` — move the item to Done, and correct its stale "no worktree-listing helper exists yet" claim rather than carrying it into the Done entry.
  Also clear the forward reference in the `loom: convert to a Shed recipe` Done entry, which states "`Env.Landing` is deliberately left unfilled by `internal/loomcli`, preserving the pre-existing gap the new `landing: parent-fabric resolution chain` Planned item above closes" — false the moment this lands, and it names this very item.
- `manifest/designs/loom.md` — the landing rows are now constructible in a real run, which is the observable behaviour change.
  Its "`loom: phase-machine scaffolding` stubs both and swaps in the real, shared-by-reference producers once `landing: Publish + Finalize producers` lands" sentence is the same shape of stale forward reference and is cleared in the same commit.
- Package docs for each module whose surface grows: `internal/fabricengine` (`OpenParent`, `Fabric.OriginURL`), `internal/loomengine` (`LoomScratchDir`), and `internal/loomcli` (`Env.Landing` now filled).
- `docs/overview.md` — only if the module table or execution stack changes;
  this task adds no module, so most likely untouched.
- `CONSTRAINTS.md` — no new cross-cutting invariant is introduced, so no change expected.
  Recorded here so the plan states it deliberately rather than by omission.

## Constraints

From `CONSTRAINTS.md`, in order of how easily this task could trip them:

- **Fabric Vocabulary Invariant.**
  This is the sharpest constraint on this task and the one it already tripped once — read it before writing any call site.
  The owner set is exactly `{internal/fabricengine, internal/fabriccli, internal/weftname, internal/gitkit, internal/boardengine, internal/configsync, internal/hubforge}` (`enforcement_test.go:597`).
  **Neither `internal/landingshed` nor `internal/loomcli` is in it**, so neither may name warp or weft in any identifier, string literal, or comment.
  The check is unforgiving in three specific ways the plan must respect: `bareVocabularyToken` (`:654`) is a case-insensitive **substring** test, not a word match;
  `fabricVocabularyHits` (`:699`) walks every `*ast.Ident`, which includes the `Sel` of a selector expression, so `pkg.SomeWarpThing(...)` is a hit at the call site;
  and the walk covers all production `.go` under `internal/` and `cmd/` (`:907`), plus an `internal/**/*.md` and `contracts/stencils/**/*.md` walk.
  Test files are excluded.
  This is why `Deps.PushBranch` is a closure *and* why the closure body cannot name the underlying verb either — see `push-verb-gets-a-neutral-fabric-method`.
  `internal/fabricengine` *is* an owner, so `OpenParent`'s implementation and the two new neutral methods may name warp and weft freely inside that package.
- **Told-Geometry Invariant.**
  `internal/landingshed` is machine-enforced via its `seam_enforcement_test.go`'s `TestToldGeometryInvariant_AllowlistOnly`: no direct production import of `internal/lyxcwd`, every absolute path told by the caller.
  This task adds no import there, so the allowlist is untouched.
  `internal/hubgeom` is an adapter, not a told package, and legitimately imports `lyxcwd` — relevant only if the rejected Q1 alternative is revisited.
- **Cwd Resolution Invariant.**
  `internal/lyxcwd` owns cwd resolution and nothing else;
  sibling-worktree-list lookup is `fabricengine`-private.
  `ResolveWorktree` is ungated by design and is the correct resolver for a held worktree root — it is not the documented bypass (`ResolveWithAnchor` is).
  Raw `os.Getwd` and `git rev-parse --show-toplevel` stay banned outside `lyxcwd` and `cmd/lyx/main.go`.
- **Durable-vs-Ephemeral State Invariant.**
  `LoomScratchDir` must sit at the mirrored `.lyx` subpath of loom's `_lyx` content, which `<anchor>/.lyx/loom/` already is.
  Enforced in part by `cmd/lyx/notransients_test.go` and `cmd/lyx/constructoranchoring_test.go`, which walk constructors rather than call sites — so the accessor must exist as a named function, not an inline join.
- **Lyxdirs Single-Declarer Invariant.**
  `LoomScratchDir` must use `lyxdirs.DotLyxDirName`, never the `.lyx` literal.
  Enforced by `TestEnforcement_GeometryLiterals`.
- **Test Tier Purity Invariant.**
  An untagged test file must not call `gitexec.Run`, `exec.Command`/`exec.CommandContext`, `gitkit.Copy*`, or `hubforge.NewHub` — matched as a **raw substring**, so even a comment or string-literal mention trips it.
  This is what forces `assembly-seam-takes-plain-values`: the `loomcli` drift guard stays untagged only because it never touches a fixture.
  Enforced by `cmd/lyx/tierpurity_test.go`'s `TestTierPurity_UntaggedTestsSpawnNothing`.
- **Hermetic Git Test Environment Invariant.**
  Any test package that spawns git, directly or through a `gitkit`/`hubforge` fixture helper, must have a `TestMain` calling `gitkit.HermeticGitEnv()` before `m.Run()`.
  `internal/fabricengine` already has `testmain_test.go`, so the new `OpenParent` integration test inherits it and adds nothing.
  `internal/loomcli` also already has one — but under `assembly-seam-takes-plain-values` the new test spawns no git, so this invariant does not newly bind it.
  Enforced by `cmd/lyx/hermeticenv_test.go` (presence only;
  correct ordering is a review obligation).
- **hubforge Fabric-Fixture Invariant.**
  Every hub fixture goes through `hubforge`, and no package inside `internal/fabriccli`'s dependency set may import it — so the `OpenParent` integration test must live in the external `fabricengine_test` package, exactly as `worktreelist_test.go` already does.
- **Fabric Git Invariant.**
  Every mutating git operation LYX performs goes through `internal/fabricengine`, in-process.
  Read-only verbs are exempt, which is what permits `RemoteURL` — but the decision above routes it through `Fabric.OriginURL()` anyway, for the API-boundary reason rather than the invariant.
- **Documentation Lifecycle** (from `CLAUDE.md`).
  This task adds cross-cutting infrastructure and changes module surfaces, so `manifest/designs/loom.md` and the affected package docs update in the same commit, and `manifest/roadmap.md` moves the item to Done — including correcting its stale "no worktree-listing helper exists" claim.
- **Markdown formatting.**
  Semantic line breaks, one sentence per line, no fixed-column hard wrap, in every `.md` file touched.

## Testing

### `internal/fabricengine` — the matcher (TDD candidate)

The branch-matching step is a pure function over `[]WorktreeEntry`, so it is the natural TDD candidate and needs no fixture.
Re-export it through `export_test.go` and drive it from `fabricengine_test` with hand-built entries.
Scenarios that must be covered:

- Exactly one entry matches the parent branch → that entry's path is returned.
- No entry matches → the not-found signal, which `OpenParent` turns into its error.
- The only candidate is detached (`Branch == "(detached)"`) → no match, and no panic on the parenthesised sentinel.
- The parent branch equals the acting worktree's own branch → matches, and returns that worktree's own pair.
  This is the decided behaviour, not an accident: per `self-parent-is-loom-policy-not-fabric-policy`, `OpenParent` special-cases nothing and the refusal lives in `drive`.
  Assert the match explicitly so a future implementer does not "fix" it into an error.
- A path with forward slashes → normalized via `filepath.FromSlash`.
- The main worktree matching (`Main == true`) is not treated specially — matching is on branch alone.

### `internal/fabricengine` — `OpenParent` end to end (integration)

`//go:build integration`, external `fabricengine_test` package, mirroring `worktreelist_test.go`'s existing shape: `hubforge.NewHub` plus `hubforge.AddPair` to create a task pair alongside the prime worktree.
Scenarios:

- Parent found: `OpenParent(taskLocation, "main")` returns a usable `*Fabric` over the prime pair, verified by an operation that proves it is the *parent's* pair and not the task's.
- No live pair for the branch: a branch that exists but has no worktree → error, and the error text names the branch.
- The parent's weft sibling is missing → `Open` fails with `*ErrMissingPath`, and `OpenParent` surfaces it rather than masking it as not-found.
  These two failure modes must stay distinguishable, since only the first is the operator-fixable "materialize the pair" case.

### `internal/fabricengine` — the two new `Fabric` methods

Both are thin single-sided delegations, and their coverage is stated here so the plan does not over- or under-build it.

`OriginURL()` gets one assertion folded into the existing `OpenParent` integration test rather than a test of its own — a `hubforge` hub has an `origin` remote, so asserting the returned URL is non-empty and matches the fixture's proves the delegation and the warp-side targeting in one line.
Its error path (no `origin` remote configured) needs no new test: `internal/gitrepo`'s own `RemoteURL` tests already cover it, and `scalar-read-errors-refuse-or-defer-by-consumer` deliberately routes that case to `Publish` rather than to a refusal here.

`PushBranch(opts)` gets **no** new integration test, deliberately.
Exercising a real push needs a real remote, which the fixture does not provide, and the delegation target's behaviour — including the `ErrPushRejected` sentinel this whole decision rests on — is already covered by `internal/gitrepo`'s `PushRebaseFree` tests.
What *is* worth asserting cheaply is the `opts.SkipPush`/`SkipGit` short-circuit, which returns before touching git at all and so needs no remote.
The load-bearing guard on this method is not a unit test but `TestEnforcement_FabricVocabulary`, which is what stops the neutral spelling being "simplified" away at the call site.

### `internal/loomengine`

`LoomScratchDir` is a pure path join.
A table test asserting it equals the directory `LoomRunLock`/`LoomDriverLog`/`LoomBootstrapLock` are parented by is enough, and it is the assertion that keeps the four from drifting apart.

### `internal/loomcli`

`wiring_test.go` already drives `wire` against a hand-built `*lyxcwd.Location` and stays tier 1 — its header comment records this explicitly.
Carrying `registry` and `runner` onto the struct must not break that: the test's tier-1 status depends on `wire` resolving no cwd and spawning no process, and both values are already built there today.
The `Env.Landing` assembly itself moves to `drive.go`, so it should be extracted into its own testable function (mirroring how `wire` was extracted from the pre-run for exactly this reason) rather than written inline in the `RunE` closure.
**The seam takes plain values, never the opened handle.**
This is what keeps the drift guard tier 1, and it is a decision, not a detail — see `assembly-seam-takes-plain-values`.
The stated signature is:

```go
func landingDeps(
    l *lyxcwd.Location,
    geom websterengine.Geometry,
    taskBranch, originURL, parentBranch string,
    pushSkipped bool,
    pushBranch func() error,
    registry modelspec.Registry,
    runner *shuttleengine.Runner,
    cfg landingshed.Config,
) landingshed.Deps
```

Every value arrives already resolved, so the function performs no I/O of any kind — not filesystem, not git, not process environment — and returns no error.
`geom` supplies `WebsterDir` and `StencilsDir`;
`l` supplies `WorktreeRoot` and backs `OpenFabric`/`OpenParentFabric`, the two closures that are *meant* to open lazily.

`pushBranch` is passed in prebuilt rather than constructed here, and that is deliberate: it closes over the handle `drive.go` already opened, so there is no third `fabricengine.Open` anywhere in this path.
`drive.go` builds it as `func() error { _, err := handle.PushBranch(opts); return err }`, discarding the `PushResult`.
`pushSkipped` likewise arrives as a plain `bool`.
`drive.go` calls `fabricengine.EnvSyncOptions()` exactly once and uses that single value for both `pushSkipped` and the closure's `opts`, so the two cannot disagree.

`drive.go` therefore owns, above this call: the handle open, both handle reads, `EnvSyncOptions()`, and the `resolveLandingParent` call whose error it surfaces.
The landing config is *not* loaded here — `wire()` owns it, per `landing-config-loads-in-wire`, and `drive.go` reads it off `c`.
`landingDeps` owns only the struct population.

Because the seam takes plain values, this test is **untagged (tier 1)**: no `git init`, no `hubforge.NewHub`, no fixture tree, so it needs no `//go:build integration` tag and no `TestMain` calling `gitkit.HermeticGitEnv`.
That is the whole reason the signature is shaped this way.
`internal/loomcli`'s existing untagged tests stay untagged, and `wiring_test.go`'s tier-1 property is preserved.

The drift guard on that function is the point of the test: a field added to `Deps` later must fail loudly here rather than nil-panicking mid-run.
Because `pushSkipped` is now a parameter rather than an env read inside the function, the guard can simply pass `true` — so a plain "every field is non-zero" assertion is achievable, with no carve-out for the bool and no `WEFT_SKIP_PUSH` manipulation.
(An earlier draft of this section carved `PushSkipped` out as unassertable;
that carve-out was an artefact of the older seam shape and no longer applies.)
The env-to-bool wiring itself now lives in `drive.go` beside the other reads, where the single `EnvSyncOptions()` call feeding both `pushSkipped` and the push closure's `opts` is the thing worth asserting — that the two agree.
A reflection-based field walk is the shape that actually catches a newly added field;
an enumerated list of fourteen assertions silently keeps passing when a fifteenth is added.

**Both refusal clauses live in a second extracted pure helper, not inline in `RunE`.**
Inline in the `RunE` closure they would be reachable only through the CLI with a resolved `*lyxcwd.Location`, and the "pure-function tests needing no fixture" claim would be false.
The extraction rationale is the one `wire` and `landingDeps` already carry in this package.
The stated signature is:

```go
func resolveLandingParent(
    recorded fabricengine.Origin,
    found bool,
    taskBranch string,
) (parentBranch string, err error)
```

It calls `resolveParentBranch(recorded, found, "")`, then applies the self-parent clause to the result.
Both inputs are plain values, so it does no I/O and is tier 1.
`drive.go` calls it between the handle reads and `landingDeps`, and surfaces its error on the envelope.

Scenarios:

- Unrecorded parent (`found == false`) and present-but-empty `ParentBranch` both refuse, with the message `resolveParentBranch` already emits.
  That function has its own tests;
  what is new — and what this helper's tests cover — is that the empty flag is passed and the error propagated rather than swallowed.
- `parentBranch == taskBranch` refuses with its own distinct message.
- The negative: an ordinary differing parent returns cleanly and trips neither clause.

`TestEnforcement_FabricVocabulary` is the guard that catches a regression on `push-verb-gets-a-neutral-fabric-method`: it already exists, needs no change, and will fail if an implementer "simplifies" `handle.PushBranch(...)` back into a direct `fabricengine.PushWarpRebaseFreeAt(...)` call in `drive.go`.
No new enforcement test is needed for that;
it is named here so the plan does not add a redundant one.

### `internal/landingshed`

No new tests.
The package's own `publish_test.go`/`finalize_test.go` already fill the closures directly, and this task changes no logic there.

## Q&A log

- **Q:** Where does the parent-fabric resolution chain live — `fabricengine`, a `hubgeom` adapter, or inline in `loomcli`? **A:** `fabricengine`, as one exported helper. Fabric already owns the worktree-listing and pairing vocabulary, and the Cwd Resolution Invariant states sibling-worktree lookup is fabric-private territory. Makes it reusable verbatim by a future Hardener, and the `loomcli` closure becomes one line.
- **Q:** What does `OpenParent` actually *open*? **A:** It returns a `*Fabric` handle over the parent worktree's own warp+weft pair — the same sense `fabricengine.Open` already carries. It creates and mutates nothing. A handle rather than a path because `Finalize` must call `Merge` inside the parent's own checkout, which a path cannot express.
- **Q:** Is `Env.Landing` filled in `wire()` or in `drive.go`? **A:** `drive.go`. `wire()` runs for every verb including `status`/`pause`, and `OpenFabric()` is opened eagerly at construction — filling in `wire()` risks exactly what the existing `OpenBisector` comment warns against. `drive` is the only path reaching `loomrecipe.New` and already checks the status file exists.
- **Q:** Where does `ParentBranch` come from? **A:** `fabricengine.ReadOrigin`. Established precedent at `run.go:76`, and named explicitly by the roadmap. A status-file fallback would cover a divergence `resolveParentBranch` already refuses to permit.
- **Q:** What happens when no worktree matches the parent branch? **A:** A plain error from the closure, which the producers convert to `Stuck`. `Finalize.Call`'s `parentOpener()` error path already implements exactly this. No new error type; a missing parent pair is operator-fixable, not a crash.
- **Q:** Which push primitive backs `PushBranch` — `PushWarpAt` or `PushWarpRebaseFreeAt`? **A:** `PushWarpRebaseFreeAt`, overriding the initial recommendation of `PushWarpAt`. Only `PushRebaseFree` produces `gitrepo.ErrPushRejected`, and `Publish.Call` already has an explicit branch handling that sentinel — `PushWarpAt` would leave it dead code. It also avoids the SHA-rewriting hazard that invalidates the warp/weft correspondence index, which the method's own doc names. The code is already built for non-rebase behaviour.
- **Q:** Where does `Deps.ScratchDir` come from? **A:** A new `loomengine.LoomScratchDir(l)`. Matches the Durable-vs-Ephemeral invariant's rule that each module owns its own scratch accessor. The stale `LoomStatusLock` comment saying "loomengine has no `Dir(l)` accessor" must be corrected in the same commit regardless.
- **Q:** Where does `Deps.OriginURL` come from? **A:** A new `Fabric.OriginURL()` method. `gitrepo.Repo.RemoteURL` already exists with zero callers — built precisely for this. Matches `fabric.go`'s documented carve-out for single-sided operations that must be callable from outside the package.
- **Q:** What test coverage? **A:** An integration test via `hubforge` mirroring `worktreelist_test.go`'s existing pattern, plus a dedicated unit test for the matching logic. An end-to-end test would need GitHub credentials and would prove only construction, not behaviour.
- **Q:** Can `loomcli` name `PushWarpRebaseFreeAt` in the push closure? **A:** No — and the earlier decision assuming it could was wrong. The bare warp/weft ban is not `landingshed`-specific: `fabricVocabularyOwners` excludes `internal/loomcli`, `bareVocabularyToken` is a substring test, and the AST walk visits a selector's `Sel`, so the call fails `TestEnforcement_FabricVocabulary`. Resolved by adding a neutral `Fabric.PushBranch(opts)` method inside the owner package, reusing the same carve-out as `Fabric.OriginURL()`. The rebase-free semantics are unchanged.
- **Q:** What does `drive` do when `ReadOrigin` reports no record, an empty `ParentBranch`, or an error? **A:** Refuse on the envelope, by calling the existing `resolveParentBranch(recorded, found, "")` with an empty flag and surfacing its message verbatim. Absent and present-but-empty are already treated identically by that function's own table, and reusing it means the rule cannot drift from `run`'s.
- **Q:** What happens when the parent branch equals the acting worktree's own branch? **A:** `OpenParent` matches it and returns that pair — it special-cases nothing, staying policy-free so a future Hardener can reuse it. `drive` refuses the configuration, as a second clause of the same guard handling the unrecorded-parent case. A corrupt provenance record gets named rather than silently accepted.
- **Q:** Does the `Deps` drift-guard test need a hub fixture? **A:** No, and the seam is shaped specifically so it doesn't. The assembly function takes plain resolved values rather than the opened handle, so it does no I/O and the test stays untagged (tier 1). Passing the handle would have made a non-zero `TaskBranch`/`OriginURL`/`Config` unreachable without a real wired hub, dragging a struct-population assertion into the integration tier and pulling `loomcli` off its documented tier-1 posture.
- **Q:** What does `drive` do when `CurrentBranch()` or `OriginURL()` errors? **A:** Different things, by consumer. `CurrentBranch()` refuses on the envelope — both producers need it, so an unreadable branch makes the segment meaningless. `OriginURL()` passes `""` through instead, because only `Publish` reads it and only when a pull request is actually required; a remote-less local hub is reachable, and `Publish` already turns an unusable origin URL into a stuck row with a precise reason.
- **Q:** Does the push closure open a third `Fabric`? **A:** No. It is built in `drive.go` over the handle already opened there and passed into the assembly seam prebuilt, so exactly two `Open` sites exist in the path: `drive.go`'s own, and whatever `OpenFabric`/`OpenParentFabric` open when the producers call them. `EnvSyncOptions()` is likewise called once in `drive.go` and feeds both `pushSkipped` and the closure's `opts`, so they cannot disagree.
- **Q:** What do `fabricengine.Open` and `landingshed.LoadConfig` errors do in `drive`? **A:** Both refuse on the envelope. An unwired pair blocks every producer, and `LoadConfig` is strict by design — `landingshed`'s package doc records that an absent config inside a hub means the hub is broken, and the error already names `lyx config reconcile` as its remedy. This is a genuinely new hard-failure mode for `lyx loom drive` on an unreconciled hub, recorded as a decision rather than left as a surprise.
- **Q:** Do the two new `Fabric` methods get their own tests? **A:** `OriginURL()` gets one assertion folded into the `OpenParent` integration test. `PushBranch(opts)` gets no new integration test — a real push needs a real remote the fixture lacks, and `internal/gitrepo`'s `PushRebaseFree` tests already cover the delegation target including the `ErrPushRejected` sentinel; only the `SkipPush`/`SkipGit` short-circuit is worth asserting cheaply. `TestEnforcement_FabricVocabulary` is the load-bearing guard on that method, not a unit test.
- **Q:** Who actually sees the new refusals, given `drive` is normally detached? **A:** Nobody, on the `run` path — which is why the landing config load moves to `wire()`. `run` spawns `drive` with stdio redirected to the driver log, so the operator sees only "driver did not take the run lock; see &lt;log&gt;". `landing.yaml` would have been the first `drive` precondition not guaranteed beforehand; loading it in `wire()` puts a precise, self-remedying error back on the operator's own terminal. The cost is that `wiring_test.go`'s seed set must gain `landing.yaml`. This does not weaken `env-landing-filled-in-drive-not-wire`: that decision protects against the eager `OpenFabric()` at producer construction, and a config load opens no fabric.
- **Q:** Where do the two refusal clauses live, if their tests are to be tier 1? **A:** In a second extracted pure helper, `resolveLandingParent(recorded, found, taskBranch)`, not inline in `RunE`. Inline they would be reachable only through the CLI and the "pure-function tests needing no fixture" claim would have been false. The extraction rationale is the one `wire` and `landingDeps` already carry.
- **Q:** Does `internal/landingshed` itself change? **A:** Comments only. `deps.go`'s "the next roadmap item builds it" claims become false when this lands and must be corrected in the same commit. Tightening `NewPublish`/`NewFinalize` validation would be real scope creep beyond the roadmap item.
