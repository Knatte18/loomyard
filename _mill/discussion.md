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
- A new `Fabric.OriginURL()` method wrapping the warp side's existing `gitrepo.Repo.RemoteURL("origin")`.
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

### push-verb-named-by-the-caller

- Decision: the closure body naming `PushWarpRebaseFreeAt` lives in `loomcli`, never in `landingshed`.
- Rationale: `landingshed` is not in the Fabric Vocabulary Invariant's owner set, so none of its identifiers, string literals, or comments may name either side.
  `PushWarpRebaseFreeAt` carries `Warp`.
  This is the entire reason `Deps.PushBranch` is an injected closure rather than a direct call, as its field doc states.
  The ban is machine-enforced by `internal/lyxcwd`'s `TestEnforcement_FabricVocabulary`.
- Rejected: nothing — this is a hard constraint, recorded here so the plan does not "simplify" the closure away.

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
| `TaskBranch` | `handle.CurrentBranch()` |
| `ParentBranch` | `fabricengine.ReadOrigin(c.location)` → `Origin.ParentBranch` |
| `WebsterDir` | `c.runDeps.Geom.WebsterDir` |
| `StencilsDir` | `c.runDeps.Geom.StencilsDir` |
| `ScratchDir` | new `loomengine.LoomScratchDir(c.location)` |
| `OriginURL` | new `handle.OriginURL()` |
| `PushSkipped` | `fabricengine.EnvSyncOptions().SkipPush` |
| `PushBranch` | closure over `fabricengine.PushWarpRebaseFreeAt` |
| `OpenFabric` | closure over `fabricengine.Open(c.location)` |
| `OpenParentFabric` | closure over new `fabricengine.OpenParent(c.location, parentBranch)` |
| `Shuttle` | the `*shuttleengine.Runner` already built in `wire()` — a compile-time assertion that it satisfies `mergeresolve.Shuttle` already exists at `mergeresolve/deps.go:46` |
| `Registry` | the `modelspec.Registry` already loaded in `wire()` |
| `Config` | `landingshed.LoadConfig(anchorPath, "landing")` — `"landing"` is already a registered config module in `internal/configreg/configreg.go:47` |

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
- `internal/fabricengine/fabric.go` — add `OriginURL()`.
- `internal/loomengine/config.go` — add `LoomScratchDir`, correct `LoomStatusLock`'s doc comment.
- `internal/loomcli/wiring.go` — carry `registry` and `runner` onto the struct;
  replace the "Landing is left unfilled" comment block.
- `internal/loomcli/cli.go` — the two new struct fields.
- `internal/loomcli/drive.go` — assemble `Env.Landing` before `loomrecipe.New`.
- `internal/landingshed/deps.go` — correct the two comments deferring to "the next roadmap item", and reconcile the `OpenFabric` laziness wording per the `two-opens-in-drive-rather-than-a-shared-handle` decision's implementer note.

Docs, required by the Documentation Lifecycle in the same commit:

- `manifest/roadmap.md` — move the item to Done, and correct its stale "no worktree-listing helper exists yet" claim rather than carrying it into the Done entry.
- `manifest/designs/loom.md` — the landing rows are now constructible in a real run, which is the observable behaviour change.
- Package docs for each module whose surface grows: `internal/fabricengine` (`OpenParent`, `Fabric.OriginURL`), `internal/loomengine` (`LoomScratchDir`), and `internal/loomcli` (`Env.Landing` now filled).
- `docs/overview.md` — only if the module table or execution stack changes;
  this task adds no module, so most likely untouched.
- `CONSTRAINTS.md` — no new cross-cutting invariant is introduced, so no change expected.
  Recorded here so the plan states it deliberately rather than by omission.

## Constraints

From `CONSTRAINTS.md`, in order of how easily this task could trip them:

- **Fabric Vocabulary Invariant.**
  `internal/landingshed` is *not* in the owner set, so no identifier, string literal, or comment there may name either side.
  This is why `PushBranch` is a closure.
  Machine-enforced by `internal/lyxcwd/enforcement_test.go`'s `TestEnforcement_FabricVocabulary` over production `.go` files under `internal/` and `cmd/`, plus an `internal/**/*.md` walk.
  `internal/fabricengine` and `internal/fabriccli` *are* in the owner set, so `OpenParent`'s implementation may name warp and weft freely.
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
- The parent branch equals the acting worktree's own branch → matches the acting worktree itself.
  Decide and pin the behaviour here rather than leaving it implicit;
  it is reachable when a task branch is somehow its own parent, and the resulting `Fabric` would merge a branch into itself.
- A path with forward slashes → normalized via `filepath.FromSlash`.
- The main worktree matching (`Main == true`) is not treated specially — matching is on branch alone.

### `internal/fabricengine` — `OpenParent` end to end (integration)

`//go:build integration`, external `fabricengine_test` package, mirroring `worktreelist_test.go`'s existing shape: `hubforge.NewHub` plus `hubforge.AddPair` to create a task pair alongside the prime worktree.
Scenarios:

- Parent found: `OpenParent(taskLocation, "main")` returns a usable `*Fabric` over the prime pair, verified by an operation that proves it is the *parent's* pair and not the task's.
- No live pair for the branch: a branch that exists but has no worktree → error, and the error text names the branch.
- The parent's weft sibling is missing → `Open` fails with `*ErrMissingPath`, and `OpenParent` surfaces it rather than masking it as not-found.
  These two failure modes must stay distinguishable, since only the first is the operator-fixable "materialize the pair" case.

### `internal/loomengine`

`LoomScratchDir` is a pure path join.
A table test asserting it equals the directory `LoomRunLock`/`LoomDriverLog`/`LoomBootstrapLock` are parented by is enough, and it is the assertion that keeps the four from drifting apart.

### `internal/loomcli`

`wiring_test.go` already drives `wire` against a hand-built `*lyxcwd.Location` and stays tier 1 — its header comment records this explicitly.
Carrying `registry` and `runner` onto the struct must not break that: the test's tier-1 status depends on `wire` resolving no cwd and spawning no process, and both values are already built there today.
The `Env.Landing` assembly itself moves to `drive.go`, so it should be extracted into its own testable function (mirroring how `wire` was extracted from the pre-run for exactly this reason) rather than written inline in the `RunE` closure.
That function takes the location, the opened handle, the registry, the runner, and the config, and returns a `landingshed.Deps` — testable without spawning a driver.

The drift guard on that function is the point of the test: a field added to `Deps` later must fail loudly here rather than nil-panicking mid-run.
State it as "every field except `PushSkipped` is non-zero", not "all fourteen".
`PushSkipped` is a `bool` sourced from `EnvSyncOptions().SkipPush`, which reads `WEFT_SKIP_PUSH` and is therefore `false` in every ordinary invocation — an all-fields-non-zero assertion could never pass.
Cover `PushSkipped` separately with a two-case assertion driving the env var set and unset, which tests the wiring rather than the value.
A reflection-based field walk is the shape that actually catches a newly added field;
an enumerated list of fourteen assertions silently keeps passing when a fifteenth is added.

### `internal/landingshed`

No new tests.
The package's own `publish_test.go`/`finalize_test.go` already fill the closures directly, and this task changes no logic there.

## Q&A log

- **Q:** Where does the parent-fabric resolution chain live — `fabricengine`, a `hubgeom` adapter, or inline in `loomcli`? **A:** `fabricengine`, as one exported helper. Fabric already owns the worktree-listing and pairing vocabulary, and the Cwd Resolution Invariant states sibling-worktree lookup is fabric-private territory. Makes it reusable verbatim by a future Hardener, and the `loomcli` closure becomes one line.
- **Q:** What does `OpenParent` actually *open*? **A:** It returns a `*Fabric` handle over the parent worktree's own warp+weft pair — the same sense `fabricengine.Open` already carries. It creates and mutates nothing. A handle rather than a path because `Finalize` must call `Merge` inside the parent's own checkout, which a path cannot express.
- **Q:** Is `Env.Landing` filled in `wire()` or in `drive.go`? **A:** `drive.go`. `wire()` runs for every verb including `status`/`pause`, and `OpenFabric()` is opened eagerly at construction — filling in `wire()` risks exactly what the existing `OpenBisector` comment warns against. `drive` is the only path reaching `loomrecipe.New` and already checks the status file exists.
- **Q:** Where does `ParentBranch` come from? **A:** `fabricengine.ReadOrigin`. Established precedent at `run.go:76`, and named explicitly by the roadmap. A status-file fallback would cover a divergence `resolveParentBranch` already refuses to permit.
- **Q:** What happens when no worktree matches the parent branch? **A:** A plain error from the closure, which the producers convert to `Stuck`. `finalize.go:120` already implements exactly this. No new error type; a missing parent pair is operator-fixable, not a crash.
- **Q:** Which push primitive backs `PushBranch` — `PushWarpAt` or `PushWarpRebaseFreeAt`? **A:** `PushWarpRebaseFreeAt`, overriding the initial recommendation of `PushWarpAt`. Only `PushRebaseFree` produces `gitrepo.ErrPushRejected`, and `publish.go:120` already has an explicit branch handling that sentinel — `PushWarpAt` would leave it dead code. It also avoids the SHA-rewriting hazard that invalidates the warp/weft correspondence index, which the method's own doc names. The code is already built for non-rebase behaviour.
- **Q:** Where does `Deps.ScratchDir` come from? **A:** A new `loomengine.LoomScratchDir(l)`. Matches the Durable-vs-Ephemeral invariant's rule that each module owns its own scratch accessor. The stale `LoomStatusLock` comment saying "loomengine has no `Dir(l)` accessor" must be corrected in the same commit regardless.
- **Q:** Where does `Deps.OriginURL` come from? **A:** A new `Fabric.OriginURL()` method. `gitrepo.Repo.RemoteURL` already exists with zero callers — built precisely for this. Matches `fabric.go`'s documented carve-out for single-sided operations that must be callable from outside the package.
- **Q:** What test coverage? **A:** An integration test via `hubforge` mirroring `worktreelist_test.go`'s existing pattern, plus a dedicated unit test for the matching logic. An end-to-end test would need GitHub credentials and would prove only construction, not behaviour.
- **Q:** Does `internal/landingshed` itself change? **A:** Comments only. `deps.go`'s "the next roadmap item builds it" claims become false when this lands and must be corrected in the same commit. Tightening `NewPublish`/`NewFinalize` validation would be real scope creep beyond the roadmap item.
