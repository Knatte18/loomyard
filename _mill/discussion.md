# Discussion: fabric: clone doesn't commit written module configs

```yaml
task: 'fabric: clone doesn''t commit written module configs'
slug: fabric-clone-commit-module-configs
status: discussing
parent: main
```

## Problem

`lyx fabric clone` materialises the nine per-worktree module configs (`batcher`, `board`, `burler`, `landing`, `loom`, `models`, `reed`, `shuttle`, `webster`) into the weft prime's working tree and then stops.
The files are written but never staged and never committed.
After a clean clone, `git -C <hub>/<name>-weft status --short` reports `?? _lyx/`, and the only commit on the weft primary branch (`main-weft`) is the initialising empty commit `fabric clone: initialise weft primary branch main-weft`.

`lyx fabric add <slug>` forks the new pair's weft branch from the parent's weft branch (`add.go`'s `createWeftWorktree(rec, l, slug, weftBranch, parentWeftBranch)`), so the fresh pair inherits a weft branch with no `_lyx/config/` at all.
The `run.sh` launcher that `add` drops into the pair's launcher directory — whose whole promise is "a double-click shortcut makes this one click" — therefore fails on its very first use:

```
{"error":"config file .../loom-e2e/_lyx/config/loom.yaml not found; run \"lyx config reconcile\"","ok":false}
```

**Why now:** found live during the loom crucible campaign (wiki task #115), recorded as finding F3 in `_mill/loom-review-opus5-high-r1.md` on branch `loom-crucible-hardening` (commit `8be3fcea`, now merged into `main` via `0793059b`).
F3 was recorded against loom because that is where it surfaced, but the defect is squarely in fabric's clone path — the configs are written and left untracked there.
Every hub minted since the config-materialisation step landed carries the defect, and every pair created off such a hub is born unusable until the operator runs `lyx config reconcile --apply` by hand.

## Scope

**In:**

- `internal/fabriccli/clone.go` — `CloneAndWire` gains a weft commit of the module configs it just materialised, immediately after `configsync.ReconcileAll(res.WeftBase, true)`.
- `internal/configengine/config.go` — a new `ConfigFileRel(module string) string` accessor returning the anchor-relative config path (`_lyx/config/<module>.yaml`), so `configDirName` stays unexported and the path segments are joined in exactly one place.
- `internal/hubforge/seed.go` — `SeedConfig`'s commit gains `--allow-empty`, because this change removes the nine untracked files that until now guaranteed its `git add .` always staged something. See the `seedconfig-tolerates-empty-stage` Decision.
- `internal/preflight/preflight_integration_test.go:51-52` and `internal/preflightshed/preflight_integration_test.go:52-53` — the same `--allow-empty` change, for the same reason. These are the other two members of the class; see the `empty-stage-class-enumeration` Decision for how the class was enumerated and why `internal/fabriccli/pushbypass_integration_test.go` is not in it.
- Doc comments on `CloneAndWire` and on `internal/hubforge` (`hub.go` / `doc.go`), which describe the post-clone fixture state that this change makes clean rather than dirty.
- New integration coverage proving both halves: the weft prime is clean after clone, and a pair created by `Topology.Add` off that clone has a populated `_lyx/config/`.

**Out:**

- **No push of the weft primary branch.** `CloneAndWire` pushes the board (`weft:main`) via `Bolt.Push` today and does not push `main-weft`; that stays true. See the `no-weft-primary-push` Decision.
- **`internal/fabricengine/clone.go` (`CloneHub`) is not touched.** It stays git/geometry-only so `fabricengine` never imports `configsync` — the documented `fabricengine → configsync → configreg → fabricengine` cycle. The brief's phrase "the defect is in fabricengine's clone path" means fabric's clone as a verb, not the `CloneHub` function; the config-materialisation half has always lived in the CLI layer.
- **`lyx config reconcile` is not changed.** An operator running it by hand in a live pair still leaves the rewritten files uncommitted. That is a separate defect on a separate verb.
- **The remedy text in `configengine`'s not-found error is not changed.** F3 notes in passing that it names the bare dry-run `lyx config reconcile` rather than `--apply`; that is its own finding on its own surface. Once this task lands, the error should not be reachable on a fresh pair at all.
- **`lyx fabric add` gains no defensive config materialisation.** The root cause is fixed at clone; a second materialisation site would be a divergent copy of a sequence that already exists once.
- **The repo-wide `fabric.yaml` path is unchanged.** It is already committed on `weft:main` through `ReconcileFabricAt` + `Bolt.Commit`; only the per-worktree nine are affected.
- **Hubs already minted before this lands are not backfilled.** No migration verb, no repair step in `reconcile`, no clone-time detection of an old hub. See the `no-backfill-for-existing-hubs` Decision for the operator remedy — a plan writer must not invent a backfill.

## Decisions

### commit-site-in-cloneandwire

- Decision: the commit lands in `internal/fabriccli/clone.go`'s `CloneAndWire`, as the final step after the `configsync.ReconcileAll(res.WeftBase, true)` loop, using `fabricengine.CommitAnchoredPaths(rec, l, relPaths, msg, fabricengine.SyncOptions{})`.
- Rationale: `CloneAndWire` is where the configs are written, and it is the single wiring sequence shared by the cobra handler and `internal/hubforge`'s hub factory — fixing it there fixes production and every fixture at once, with no second copy of the sequence to drift.
  `l` (`lyxcwd.Resolve(res.PrimeCwd)`) is already in scope from the junction-wiring step, and `CommitAnchoredPaths` resolves the weft worktree from `l` itself, so the CLI layer never names a weft path.
  `CommitWeftPaths` (which `CommitAnchoredPaths` wraps) is the narrow, positive-pathspec, no-push, lock-taking shape the Fabric Git Invariant's cross-module-exclusions rule demands for durable-`_lyx` commits, and it records `KindCommitCreated` at its own success site.
- Rejected: pushing the commit into `fabricengine.CloneHub` — reintroduces the `configsync` import cycle `CloneHub`'s doc comment exists to prevent.
  A stage-all commit at the weft prime via `NewBolt(WeftWorktree(l)).Commit(...)` — `commitWeftAt` is a stage-all, which the Fabric Git Invariant forbids for the `_lyx` tree shared by every round-loop module, and `Bolt` is documented as the handle over the *unpaired* `weft:main` area, which the weft prime is not.

### commit-only-applied-modules

- Decision: build `relPaths` from exactly the `configsync.Result` entries whose `Applied` field is `true` — the same predicate the existing `KindFileWritten` recording loop already uses.
- Rationale: a commit pathspec should name what this call wrote and nothing else.
  On the re-clone path, `suffixWeftPrimaryBranch` adopts an existing `origin/<branch>-weft` that may already carry committed configs; `ReconcileAll` then reports `Applied: false` for every module *whose committed config already matches its template*, so when that holds for all of them `relPaths` comes out empty and `CommitWeftPaths`'s own `len(relPaths) == 0` guard returns `("", false, nil)` with no lock taken and nothing recorded.
  A config that has drifted from its template still reports `Applied: true` (the `hasChanges` branch in `configsync.ReconcileAll`) and is committed, which is also correct — the empty-`relPaths` no-op is a possible outcome on this path, not a guaranteed one, and both outcomes are right.
  Seed-only modules (`burler`, `models`) with a present file are likewise `Applied: false` and correctly left alone.
- Rejected: naming all nine registered non-fabric modules unconditionally — would stage files this call did not write, and on the adopt path would take the weft write lock to produce a guaranteed no-op commit.

### configfilerel-accessor

- Decision: add `configengine.ConfigFileRel(module string) string` returning `filepath.Join(lyxdirs.LyxDirName, configDirName, module+".yaml")`, and build `relPaths` from it in `CloneAndWire`.
- Rationale: `configDirName` is unexported and `internal/configengine` is its sole declarer; an anchor-relative accessor mirrors the existing `ConfigDir`/`ConfigFile` pair and the `fabricengine.OriginRecordRel()` idiom, where the segments are joined in exactly one place so path accessors and commit pathspecs can never drift.
- Rejected: `filepath.Rel(res.WeftBase, configengine.ConfigFile(res.WeftBase, m))` in the CLI — correct but obscure, and it derives a relative path by cancelling an absolute one.
  Relying on `configengine.ConfigFile("", module)` happening to return the relative form — an undocumented accident of `filepath.Join`.
  Declaring the `_lyx/config` literal in `fabricengine` — would violate the Lyxdirs Single-Declarer Invariant's spirit and duplicate `configDirName`.

### no-weft-primary-push

- Decision: commit only. `CloneAndWire` does not gain a push of the weft primary branch.
- Rationale: it is already the status quo that `main-weft` carries local-only commits after clone — `bornWeftPrimaryBranch` lands `fabric clone: initialise weft primary branch <b>` and nothing pushes it.
  Adding one more local commit changes nothing about the divergence story, whereas adding a push introduces a brand-new network failure mode into `clone`'s critical path and a new exported push surface (`pushWeftBranch` is unexported and slug-shaped; `Bolt` is the wrong handle for the paired prime).
  The commit is carried to the remote by the first `lyx fabric add`, whose `pushWeftBranch` pushes the new pair's branch together with its inherited history.
- Rejected: pushing via `fabricengine.NewBolt(WeftWorktree(l)).Push(...)` — `gitrepo.PushCoalesced` would handle the no-upstream case via `push.autoSetupRemote=true`, so it is mechanically feasible, but it widens the verb's blast radius well past the brief for a divergence problem that predates this task.

### commit-message-wording

- Decision: the commit message is `fabric clone: record module configs`.
- Rationale: matches the sibling board commit two steps earlier in the same function, `fabric clone: record anchor + repo-wide config` — same `fabric clone:` prefix, same `record` verb, and the "module configs" half names the per-worktree nine against the board commit's "repo-wide config".
- Rejected: `fabric clone: materialize module configs` (describes the write, not the commit); reusing the board commit's message verbatim (two different branches, two different contents).

### error-path-returns-zero-result

- Decision: a commit failure returns `fabricengine.CloneResult{}, err`, exactly like every other post-recorder failure site in `CloneAndWire`.
- Rationale: the named-result plus `defer func() { res.Mutations = rec.Snapshot() }()` idiom this function documents is what carries the accumulated record through a zero return; the CLI handler then routes it through `errWithRecord`.
  The clone is left intact on error, per `CloneAndWire`'s own contract, and the operator completes wiring with `reconcile`.
- Verification: **no new test.** The path is covered by the shared named-result + `defer` idiom, which every other post-recorder failure site in `CloneAndWire` already exercises under test; reaching this specific site would need a seam for making `CommitWeftPaths` fail, and adding one to production code to test a two-line error return is a worse trade than the coverage is worth. A plan writer should not invent an injection point here.
- Rejected: swallowing the error and logging a warning — a hub whose pairs cannot boot is not a warning-level outcome.
  Rolling the hub back — the function's documented contract is explicitly "on error, the clone is left intact".

### fixture-state-change-is-the-point

- Decision: accept that `hubforge.NewHub` fixtures change from "weft prime carries nine untracked files under `_lyx/`" to "weft prime is clean", and treat any test that breaks as a test asserting the bug.
- Rationale: the hubforge Fabric-Fixture Invariant states every hub fixture is built through `fabriccli.CloneAndWire`, so this is not a side effect to be worked around — it is the invariant working.
  A clean post-clone weft prime is also strictly more correct for the destruction gate's dirtiness checks, which observe real worktree state.
  Scope note: this Decision covers *assertions* about the old dirty state. It does not cover a fixture **helper** acquiring a new way to fail — that is `seedconfig-tolerates-empty-stage`'s subject, and it is fixed in the helper rather than absorbed by its callers.
- Rejected: gating the new commit behind an option so fixtures keep the old shape — that would make the fixture stop matching a real clone, which is the one thing the Fabric-Fixture Invariant exists to prevent.

### seedconfig-tolerates-empty-stage

- Decision: `internal/hubforge/seed.go`'s `SeedConfig` commits with `git commit --allow-empty` instead of a bare `git commit`.
- Rationale: `SeedConfig` runs `gitkit.MustRun(tb, h.PrimeWeft(), "git", "add", ".")` followed by `gitkit.MustRun(tb, h.PrimeWeft(), "git", "commit", "-m", "hubforge: seed config")`, and `gitkit.MustRun` calls `tb.Fatalf` on any non-zero exit (`internal/gitkit/gitkit.go:41-43`).
  Today the nine untracked config files guarantee `git add .` always stages something, so the commit can never exit non-zero.
  After this change the base clone leaves the weft prime clean, so a seeded override that is byte-identical to the just-committed reconciled file stages nothing and `git commit` exits 1 — a fixture-level `tb.Fatalf` in a test that did nothing wrong, and a failure mode unreachable before this change.
  `--allow-empty` removes the new failure mode outright, at the cost of an occasional empty fixture commit that nothing observes.
- Rejected: leaving `SeedConfig` alone and letting it break — this is not a test asserting the bug, it is a fixture helper acquiring a new way to fail, so the `fixture-state-change-is-the-point` Decision does not cover it.
  Probing `git diff --cached --quiet` and skipping the commit conditionally — more moving parts in a fixture helper for the same outcome, and it makes "did SeedConfig commit?" ambiguous to a reader.
  Both live callers (`internal/webstercli/verbs_test.go:773`, `internal/loomcli/smoke_test.go:155`) seed real overrides today, so this is a latent trap rather than an immediate breakage — which is exactly why it must be closed now rather than discovered later.

### empty-stage-class-enumeration

- Decision: the "fixture stages nothing and the bare `git commit` exits 1" class is enumerated mechanically, not by hand: every `gitkit.MustRun(..., "git", "add", ...)` followed by a bare `gitkit.MustRun(..., "git", "commit", ...)` run at `h.PrimeWeft()`, minus the sites where the fixture itself writes a file before staging.
  The full class is three sites, and all three take `--allow-empty`.
- Rationale: `gitkit.MustRun` `tb.Fatalf`s on any non-zero exit, so every member of this class becomes a hard fixture failure the moment the weft prime starts out clean.
  Enumerating by grep rather than by memory is what makes the disposition complete instead of "the ones we happened to notice".
  The members:
  - `internal/hubforge/seed.go:44-45` — writes overrides, but a seed byte-identical to the reconciled file stages nothing.
  - `internal/preflight/preflight_integration_test.go:51-52` — writes nothing of its own. Its only stageable content today is the nine untracked configs: `.lyx` is excluded via `.git/info/exclude`, and the `_extra` junction target materialises as an empty directory, which git does not track.
  - `internal/preflightshed/preflight_integration_test.go:52-53` — same shape, same reason, and its own comment says it mirrors `internal/preflight`'s fixture.
  - **Not** in the class: `internal/fabriccli/pushbypass_integration_test.go:68-73`, which writes `placeholderFile` under `_lyx` immediately before its `git add .`, so it always has something to stage.
- Rationale for `--allow-empty` at the two preflight sites specifically: both exist to leave the fixture clean on the weft side, and after this change the clone already leaves it clean, so the add-and-commit becomes a no-op that must be allowed to succeed rather than deleted.
  Deleting it would silently drop the guarantee if a future fixture change reintroduces untracked weft content.
- Rejected: leaving the two preflight sites to be discovered by the Testing §8 regression sweep — the sweep would catch them, but as an unexplained red suite rather than a planned edit, and a plan writer with no disposition would be free to "fix" them by deleting the commits.

### no-backfill-for-existing-hubs

- Decision: hubs minted before this change are out of scope. Nothing detects them, nothing repairs them, and `lyx config reconcile` is not taught to commit.
- Rationale: the defect is a missing step in `clone`, and `clone` is where it is fixed.
  A backfill would need either a new verb or a repair branch in `reconcile` that commits on the operator's behalf — both are larger surfaces than the bug, and both would outlive the one-off condition they exist for.
  The operator remedy is a one-time manual fix-up per affected hub: run `lyx config reconcile --apply` in the prime pair, then commit `_lyx/config/` on the weft primary branch by hand.
  After that fix-up, a pair added **from the fixed prime** inherits the configs exactly as it would from a hub minted post-fix.
  The reach stops there: `Topology.Add` forks the new weft branch from `WeftBranchName(<the invoking worktree's warp HEAD branch>)` (`add.go`'s `parentBranch` / `parentWeftBranch`, used at the `createWeftWorktree` call), so a pair added *from an already-broken pre-fix pair* still inherits a config-less weft branch and still needs `lyx config reconcile --apply`.
  Fixing an existing pair the same way — `reconcile --apply` then commit `_lyx/config/` on that pair's own weft branch by hand — makes it a sound parent in turn.
  Committing by hand in the weft repo is a human running ordinary git, which the Fabric Git Invariant explicitly permits — that invariant binds LYX's own code, not the operator.
  Without any fix-up, the fallback stays what it is today: `lyx config reconcile --apply` in each new pair, forever.
- Rejected: a `--backfill-configs` flag on `reconcile` — a migration surface for a condition that stops occurring the day this lands.
  Telling operators to `lyx fabric clone --reset` — destroys and re-mints the whole hub to fix nine files.

### docs-in-the-same-commit

- Decision: update, all in the same commit —
  - `CloneAndWire`'s doc comment, whose enumerated wiring sequence gains the commit step;
  - `internal/fabriccli/clone.go`'s file-header comment (lines 1-6), which enumerates that same sequence and would otherwise go stale;
  - `internal/hubforge`'s `hub.go` / `doc.go` post-clone-state description;
  - `internal/hubforge/seed.go`'s `SeedConfig` doc comment, which already explains where its commit runs and why, and must now also say why the commit is `--allow-empty`;
  - `docs/shared-libs/configengine.md`'s `## Exported functions` section, which gains a `### ConfigFileRel(module string) string` entry beside the existing `ConfigDir` / `ConfigFile` entries.

  No `CONSTRAINTS.md` change, no `docs/overview.md` change, no `manifest/roadmap.md` move.
- Rationale: the Documentation Lifecycle rule requires the touched module's doc to land in the same commit.
  There is no `manifest/designs/fabric.md`; fabric's module doc is `internal/fabricengine/doc.go` plus the CLI-layer package comments, and the changed behaviour is in `fabriccli`.
  `docs/shared-libs/configengine.md` is in because it enumerates `configengine`'s exported surface function by function, and `ConfigFileRel` is a new exported function on exactly that surface — omitting it would leave the doc silently incomplete the day it lands.
  No new cross-cutting invariant is introduced — this change *satisfies* existing invariants rather than adding one.
  `roadmap.md` moves only on completing or adding a planned item; this is a bugfix.
- Rejected: adding a "clone commits what it materialises" invariant to `CONSTRAINTS.md` — the Fabric Git Invariant and the Fabric-Fixture Invariant already cover the commit shape and the fixture consequence.

## Technical context

**Where the defect is.**
`internal/fabriccli/clone.go:98-104` — `CloneAndWire`'s final block:

```go
results, err := configsync.ReconcileAll(res.WeftBase, true)
if err != nil {
    return fabricengine.CloneResult{}, err
}
for _, r := range results {
    if r.Applied {
        rec.Append(fabricengine.KindFileWritten, configengine.ConfigFile(res.WeftBase, r.Module), "")
    }
}

return res, nil
```

The files are written and recorded; nothing commits them.

**Geometry.**
`fabricengine.CloneResult` (`internal/fabricengine/clone.go:68-77`) carries `HubPath`, `Anchor`, `BoardDir` (the `weft:main` checkout at `<hub>/_board`), `WeftBase` (the anchor-joined weft directory paired with the prime warp worktree), and `PrimeCwd`.
`WeftBase` is computed as `filepath.Join(WeftWorktree(l), l.AnchorRel)`, so at a non-`"."` anchor it is a *subdirectory* of the weft worktree root — which is exactly why the commit must be anchor-scoped rather than run at `WeftBase`.

`BoardDir` and `WeftBase` are different worktrees on different branches: `BoardDir` is on the unsuffixed default branch (`main`) of the weft repo, `WeftBase`'s worktree is on the `-weft`-suffixed pairing (`main-weft`).
The existing `NewBolt(res.BoardDir).Commit(...)` in `CloneAndWire` therefore does *not* reach the module configs, and never could.

**The helper to reuse.**
`internal/fabricengine/commitweftpaths.go`:

- `CommitWeftPaths(rec *Mutations, weftPath, anchorRel string, relPaths []string, msg string, opts SyncOptions) (sha string, committed bool, err error)` — takes the weft write lock, stages exactly `ScopedPathspec(anchorRel, relPaths)` via `gitrepo.New(weftPath).StageAndCommit`, never pushes, and appends `KindCommitCreated` only when the commit observably landed.
- `CommitAnchoredPaths(rec *Mutations, l *lyxcwd.Location, relPaths []string, msg string, opts SyncOptions)` — the vocabulary-neutral wrapper that resolves `WeftWorktree(l)` and `l.AnchorRel` from `l` itself. **This is the one to call**: the CLI never names a weft path.
- `relPaths` are anchor-relative, the same shape `OriginRecordRel()` returns.
- Empty `relPaths` and `opts.SkipGit` both short-circuit to `("", false, nil)` with no lock taken and nothing recorded.

`l` is already bound in `CloneAndWire` at line ~72 (`l, err := lyxcwd.Resolve(res.PrimeCwd)`), used for `WireJunctionsWith`.
`rec` is the function's own `*fabricengine.Mutations`.

**The nine modules.**
`internal/configreg/configreg.go`'s `Modules()` returns ten entries in alphabetical order — `batcher`, `board`, `burler` (SeedOnly), `fabric`, `landing`, `loom`, `models` (SeedOnly), `reed`, `shuttle`, `webster`.
`configsync.ReconcileAll` skips `fabric` explicitly (repo-wide, handled by `ReconcileFabricAt` against `BoardDir`), leaving the nine per-worktree ones.

**Config paths.**
`internal/configengine/config.go`: `configDirName = "config"` (unexported), `ConfigDir(baseDir) = <baseDir>/_lyx/config`, `ConfigFile(baseDir, module) = <baseDir>/_lyx/config/<module>.yaml`.
`_lyx` comes from `lyxdirs.LyxDirName`, whose sole declarer is `internal/lyxdirs`.
The new `ConfigFileRel` belongs beside `ConfigFile`.

**Consumers of the fix.**
`internal/hubforge/hub.go`'s `NewHub` drives `fabriccli.CloneAndWire` (line ~226) and populates `Hub.WeftBase` verbatim from `res.WeftBase`.
Every hub fixture in the repo is therefore affected — this is the hubforge Fabric-Fixture Invariant working as designed.
`hubforge.SeedConfig` (`internal/hubforge/seed.go:44-45`) writes overrides into `h.WeftBase` and commits them at `h.PrimeWeft()` with `git add .` + `git commit`, both through `gitkit.MustRun`, which `tb.Fatalf`s on a non-zero exit.
After this change that `git add .` will find nothing new to stage from the base clone, only the seeded overrides — the intended shape, but also the reason the commit needs `--allow-empty` (see the `seedconfig-tolerates-empty-stage` Decision): a seed byte-identical to the reconciled file now stages nothing and the bare commit would exit 1.

**Gotchas.**

- `seedWeftArtifactExcludes` (called from `CloneHub`) seeds the weft repo's `.git/info/exclude` with `.lyx/`, `.weft/`, push-lock and module lock files. `_lyx/config/*.yaml` is not excluded — nothing blocks the stage.
- Never force-add: `gitrepo.StageAndCommit` has no `-f` path, and none is needed.
- Ordering matters for the mutation record: array order is the only thing carrying ordering in that vocabulary, so the `KindFileWritten` loop must stay *before* the commit call, which appends `KindCommitCreated` last.
- `ReconcileAll` is also called by `internal/configcli` (`lyx config reconcile`). That call site is out of scope and must not be changed.

## Constraints

From `CONSTRAINTS.md`:

- **Fabric Git Invariant (warp + weft).** Every weft-commit caller passes a **positive-only** file list — no `:(exclude)` pathspec magic — built via `fabricengine.ScopedPathspec`. `CommitWeftPaths` is the shape that satisfies this; a stage-all (`commitWeftAt` / `Bolt.Commit`) does not, because the `_lyx` tree is shared by every round-loop module. All git operations go through `internal/fabricengine`, in-process, never raw git.
- **Durable-vs-Ephemeral State Invariant.** `_lyx` holds tracked content only; `.lyx` is never tracked. Committing `_lyx/config/*.yaml` is exactly what `_lyx` is for. Nothing under `.lyx` may reach the pathspec.
- **Mutation Record Invariant.** A recording site fires only *after* the primitive observably changed state, never on a no-op. `CommitWeftPaths` already honours this internally; the call site adds no recording of its own.
- **hubforge Fabric-Fixture Invariant.** Every hub fixture is built by `internal/hubforge` through `fabriccli.CloneAndWire`; no hub is hand-assembled. No package inside `internal/fabriccli`'s dependency set may import `hubforge` — such tests use an external `*_test` package.
- **Test Tier Purity Invariant.** An untagged test file must not call `hubforge.NewHub`, `gitexec.Run`, `exec.Command`, or `gitkit.Copy*`. Any test using a real hub carries `//go:build integration` as its first non-empty line.
- **Hermetic Git Test Environment Invariant.** A git-spawning test package needs a `TestMain` calling `gitkit.HermeticGitEnv()`. `internal/fabriccli`'s `testmain_test.go` already provides one for `package fabriccli_test`.
- **Never Force-Add Invariant.** No `git add -f`, ever.
- **Config Strictness Invariant.** Governs `Load` vs `LoadOrTemplate` policy for new config callers. Not triggered here — `ConfigFileRel` is a pure path accessor with no loading policy.
- **Documentation Lifecycle.** Docs for a touched module land in the same commit.

Discovered during discussion:

- `CloneAndWire`'s named-result + `defer` idiom means every post-recorder failure site returns `fabricengine.CloneResult{}, err` and still carries the accumulated mutation record. A new failure site must follow it.
- The verify command for this repo is `go test ./... && go test -tags integration ./...` (`mill-config.yaml`'s `done_gate`). Both halves must pass; the integration half is where the fixture-state change lands.

## Testing

**TDD candidates** (write the failing test first — both fail today for exactly the F3 reason):

1. **Weft prime is clean after clone.** `//go:build integration`, `package fabriccli_test`, new file in `internal/fabriccli/`.
   Build a hub with `hubforge.NewHub(t, ".")`, then assert the weft prime worktree reports no untracked or modified paths under the durable lyx directory, and that `git ls-files` there lists a `_lyx/config/<module>.yaml` entry for each of the nine non-`fabric` modules from `configreg.Modules()`.
   Deriving the expected set from `configreg.Modules()` rather than a hard-coded list of nine keeps the test honest when a tenth module is registered.
2. **A pair created off the clone has its configs.** Same file. From the same hub, run `Topology.Add` for a slug, then assert the new pair's weft worktree carries `_lyx/config/loom.yaml` on disk at the anchored path.
   This is the direct end-to-end proof of F3's reported symptom (`lyx loom run` failing on `loom.yaml not found` in a fresh pair), stopping short of spawning loom itself.

**Additional scenarios:**

3. **Anchor scoping.** Repeat scenario 1 with `hubforge.NewHub(t, "backend")` and assert the committed paths are `backend/_lyx/config/<module>.yaml` — proves `ScopedPathspec(l.AnchorRel, …)` is doing its job and that the commit was not run at `WeftBase` (a subdirectory of the worktree root at a non-`"."` anchor).
4. **Idempotence / no spurious commit.** Assert that the commit exists exactly once on the weft primary branch after a clone — i.e. the module-config commit is a single commit, not one per module, and a second `configsync.ReconcileAll` over the same tree would report nothing applied.
5. **Mutation record.** Assert `res.Mutated()` from a direct `fabriccli.CloneAndWire` call contains one `file_written` entry per materialised module followed by a `commit_created` entry naming the weft worktree — order is part of the vocabulary.
   Derive the expected count from the same source scenario 1 uses (`configreg.Modules()` minus `fabric`), never a hard-coded nine, so a tenth registered module does not silently make this test wrong.
6. **`SeedConfig` survives a redundant seed.** Build a hub, then call `hubforge.SeedConfig` with a module's config set to exactly the content the clone already committed, and assert the helper does not `tb.Fatalf`.
   This is the direct regression test for the `seedconfig-tolerates-empty-stage` Decision, and it fails today only *after* the clone-commit change lands — so it is written alongside that change, not before it.
   The two preflight fixtures in `empty-stage-class-enumeration` need no test of their own: their existing integration tests exercise the fixture on every run, so the `--allow-empty` edit is covered by the §8 sweep passing.

**Unit coverage:**

7. `configengine.ConfigFileRel` — untagged, table-free unit test asserting the returned path for a couple of module names and that it is relative (`!filepath.IsAbs`). Pair it with an assertion tying it to `ConfigFile`: `ConfigFile(base, m)` equals `filepath.Join(base, ConfigFileRel(m))`, so the two accessors can never drift.

**Regression sweep:**

8. Run the full gate — `go test ./...` then `go test -tags integration ./...`. The fixture-state change (weft prime clean rather than carrying nine untracked files) will surface in `internal/fabricengine`'s live-state integration harness if any cell encodes the old dirty shape.
   Any such failure is a test asserting the bug and gets updated to the corrected state, not worked around; note each one in the batch's report so the change in fixture state is auditable rather than silent.

## Q&A log

- **Q:** Where does the commit go — `CloneAndWire`, `CloneHub`, or a stage-all Bolt commit at the weft prime? **A:** [auto-pick] `CloneAndWire`, right after `configsync.ReconcileAll`, via `fabricengine.CommitAnchoredPaths`. **Why:** it is where the configs are written and the single sequence shared by the CLI handler and `hubforge`; `CloneHub` cannot import `configsync` (cycle), and a stage-all violates the Fabric Git Invariant's positive-only rule for the shared `_lyx` tree.
- **Q:** Commit only the modules whose `Result.Applied` is true, or all nine unconditionally? **A:** [auto-pick] only the `Applied` ones. **Why:** matches the existing `KindFileWritten` predicate, and makes the re-clone/adopt path a clean no-op through `CommitWeftPaths`'s own empty-pathspec guard rather than a lock-taking empty commit.
- **Q:** How is the anchor-relative config path derived — a new `configengine.ConfigFileRel`, `filepath.Rel` at the call site, or a literal in `fabricengine`? **A:** [auto-pick] add `configengine.ConfigFileRel(module)`. **Why:** `configDirName` is unexported and `configengine` is its sole declarer; mirrors the `OriginRecordRel()` idiom of joining segments in exactly one place.
- **Q:** Should `CloneAndWire` also push the weft primary branch so the config commit reaches the remote? **A:** [auto-pick] no — commit only. **Why:** `main-weft` already carries unpushed local commits after clone (`bornWeftPrimaryBranch`), so nothing regresses; a push adds a network failure mode to `clone` and a new exported push surface, for a divergence problem that predates this task. The first `lyx fabric add` carries the commit remote-ward anyway.
- **Q:** Fix the `run "lyx config reconcile"` remedy text F3 flags as naming the dry-run verb instead of `--apply`? **A:** [auto-pick] no, out of scope. **Why:** it is a separate finding on a separate surface (`configengine`'s not-found error), the error text is asserted by existing tests, and after this fix the message should be unreachable on a fresh pair.
- **Q:** Where do the tests live? **A:** [auto-pick] a new `//go:build integration` file in `internal/fabriccli/`, `package fabriccli_test`, using `hubforge.NewHub`, plus an untagged unit test for `ConfigFileRel`. **Why:** the hubforge Fabric-Fixture Invariant explicitly sanctions an external `*_test` package for tests inside `fabriccli`'s dependency set, and `testmain_test.go` already wires `gitkit.HermeticGitEnv()`; the Test Tier Purity Invariant forbids `hubforge.NewHub` from an untagged file.
- **Q:** What commit message? **A:** [auto-pick] `fabric clone: record module configs`. **Why:** mirrors the sibling board commit `fabric clone: record anchor + repo-wide config` in the same function, with "module configs" distinguishing the per-worktree nine from the repo-wide one.
- **Q:** Which docs move in the same commit? **A:** [auto-pick] `CloneAndWire`'s doc comment plus `internal/hubforge`'s `hub.go`/`doc.go` post-clone-state description; no `CONSTRAINTS.md`, no `docs/overview.md`, no `manifest/roadmap.md`. **Why:** there is no `manifest/designs/fabric.md` — fabric's module doc is its package comments; no new cross-cutting invariant is introduced; and `roadmap.md` moves only for planned items, not bugfixes.
- **Q:** [review r1 gap] `hubforge.SeedConfig`'s `git add .` + `git commit` runs through `gitkit.MustRun`, which `tb.Fatalf`s on non-zero exit; once the base clone leaves the weft prime clean, a seed byte-identical to the reconciled file stages nothing and the commit exits 1. What is the disposition? **A:** [auto-pick] `SeedConfig`'s commit gains `--allow-empty`, and `internal/hubforge/seed.go` joins the in-scope file list. **Why:** the new failure mode is a fixture helper acquiring a new way to fail, not a test asserting the bug, so the `fixture-state-change-is-the-point` Decision does not cover it; `--allow-empty` closes it outright with one flag, versus a conditional `git diff --cached --quiet` probe that adds branching to a fixture helper for the same outcome. Both live callers seed real overrides today, so this is a latent trap that would surface later rather than at merge.
- **Q:** How are hubforge-fixture breakages handled when the weft prime becomes clean? **A:** [auto-pick] update them to the corrected state and note each in the batch report. **Why:** the Fabric-Fixture Invariant makes fixture state track a real clone by design, so a cell encoding the old dirty shape is asserting the bug.
