# Discussion: lyxtest: build real fabric hubs, invert the lyxtest/fabric dependency

```yaml
task: 'lyxtest: build real fabric hubs, invert the lyxtest/fabric dependency'
slug: lyxtest-real-hubs
status: discussing
parent: main
```

## Problem

Every git fixture in this repo comes from `internal/lyxtest`, and none of its hub fixtures were ever produced by fabric.
`buildWarpHub` runs `git init` plus one commit;
`buildWeftPrime` makes a `<name>-weft` sibling holding a placeholder `_lyx/config`.
Neither has `_board`, `_portals`, `_launchers`, a hub-level `.lyx`, junctions, a `.lyx-anchor` marker, the warp-URL binding on `weft:main`, or a repo-wide `fabric.yaml`.
A real hub is roughly 155 files;
the templates are roughly 36.

So every test built on those fixtures asserts against a shape someone wrote down rather than the shape fabric produces, and nothing detects drift between the two.
The tests already admit it in their own comments: `internal/fabricengine/weftgit_exclude_test.go:59-62` describes seeding config "at warpPath's parent directory (this fixture's **stand-in Hub**)", and `internal/loomengine/preflight_integration_test.go` hand-rolls a `seedRepoWideFabricConfig` helper for the same reason.
`internal/reedcli/cli_integration_test.go:24-29` does `CopyPaired` → `SeedConfig(fixture.Hub)` → `t.Chdir(fixture.Hub)`, chdir-ing into a directory that is not a hub at all.

**Why now.**
The fabric chain (slices 12→13→14→15) has landed, which was this task's sequencing precondition.
Slice 13 also produced `internal/fabricengine/fabrictest`, whose `hub.go` already implements the exact "copy the bares, clone the hub" factory this task was proposed to build — so the primitive exists and needs promoting rather than inventing.

The fix is to make every hub fixture in the repo come out of fabric's own clone path, so drift is impossible by construction instead of by discipline.

## Scope

**In:**

- New package `internal/hubforge`: the repo-wide real-hub fixture factory, built from today's `internal/lyxtest` plus `internal/fabricengine/fabrictest/hub.go`, merged.
  It imports `internal/fabriccli` and builds hubs via `fabriccli.CloneAndWire`.
- New package `internal/gitkit`: the below-fabric leaf, holding what is left of `lyxtest` — `MustRun`, `SeedConfig`, `HermeticGitEnv`, `GitStatusPorcelain`, and one primitive repo fixture.
- Delete `internal/lyxtest` and `internal/fabricengine/fabrictest` as package names;
  no package by either name survives.
- Migrate all 132 above-fabric `Copy*` call sites onto `hubforge.NewHub`.
- Retarget or delete the 56 `SeedConfig` call sites — a second migration axis, not a subset of the 132.
- Move `fabrictest`'s live-state machinery (`states.go`, `verbs.go`, `manifest.go`, `mutationoracle.go`, `refusal.go` and their tests, ~4960 lines) into `package fabricengine_test` files inside `internal/fabricengine/`.
- Move `internal/fabricengine`'s 14 in-package `lyxtest` callers off the leaf, and the two stuck in-package files in `treadleengine`/`loomengine` to external test packages with an `export_test.go` shim.
- Delete `CopyPaired`, `CopyPairedLocal`, `CopyWeft`, and `NewPairedForTest`.
- Junction-safe fixture teardown in `hubforge`.
- `CONSTRAINTS.md` rewrite — **already applied in this worktree**, see "Constraints" below.
- Module docs for `hubforge` and `gitkit`;
  `docs/overview.md` module table.
- Delete `manifest/designs/lyxtest-real-hubs.md` per the documentation lifecycle, and move `manifest/roadmap.md`'s Planned item 1 to Done, repairing the inbound link.
- Update the 10 markdown files naming `lyxtest`/`fabrictest` (see Technical context).

**Out:**

- `internal/gitrepo` and `internal/lyxcwd` do not get hub fixtures.
  They stay on `gitkit`'s primitive fixtures.
  This is a load-bearing scoping rule, not an oversight — see the Decision below.
- No Windows benchmark run.
  The code must be Win11-correct by construction, but no measurement gates this task.
- No removal of `t.Chdir` from test files and no `t.Parallel` enablement at call sites.
  Filed separately as wiki task `hubforge-parallel-chdir`, which depends on this one.
- No new production code in `internal/fabricengine` or `internal/fabriccli`.
  In particular, no new hub-teardown verb.
- No change to any CLI signature.
- No transport/auth/server-policy coverage (https/ssh, protected branches, server hooks).
  Local bare repos are the fixture substrate; none of that surface is fabric's code.
- The sandbox Hub (`lyx-test-HUB`) is untouched.

## Decisions

### Three packages, split by role — not by campaign

- **Decision:**
  `internal/gitkit` (leaf: git primitives), `internal/hubforge` (factory: real fabric hubs), and fabric's live-state assertions as `package fabricengine_test` files inside `internal/fabricengine/`.
  Neither `lyxtest` nor `fabrictest` survives as a package name.
- **Rationale:**
  The three have genuinely different jobs.
  `gitkit` hands out git primitives and asserts nothing.
  `hubforge` builds hubs and asserts nothing — it is a factory, not a test suite, which is why its name does not end in `test`.
  Fabric's own live-state assertions (`AssertNoUnpermittedChange`, `AssertRecordMatchesDiff`, the states × verbs cross-product) test fabric, so they belong with fabric.
  `hubforge`'s importability is load-bearing: its consumers live in roughly 15 directories.
  `fabrictest`'s importability is used by nobody — both of its consumers (`internal/fabricengine/dotlyxjunction_integration_test.go`, `internal/fabricengine/weftgit_exclude_test.go`) are already `package fabricengine_test`, so they can hold that code directly.
- **Rejected:**
  Inverting `internal/lyxtest` in place while keeping the name — the name is too generic for a package that is now only a hub factory.
  Keeping `fabrictest` as a package for directory hygiene — it adds a third test-support package for no structural gain.
  Folding `hubforge` into `fabrictest` — `reedcli`'s tests would then import fabric's mutation oracle and refusal manifests to obtain a hub.

### A leaf below fabric must exist, and it is `gitkit`

- **Decision:**
  `internal/gitkit` imports only stdlib plus `lyxcwd`, `weftname`, `configengine`, `lyxdirs`.
  It never imports fabric.
- **Rationale:**
  This is a Go compile constraint, not a policy choice.
  23 packages call `HermeticGitEnv` from `TestMain`, and 11 of them sit inside `internal/fabriccli`'s dependency set: `gitexec`, `gitrepo`, `lyxcwd`, `boardengine`, `burlerengine`, `loomengine`, `perchengine`, `treadleengine`, `websterengine`, `fabricengine`, `fabriccli`.
  If the only module offering `HermeticGitEnv` imported fabric, those packages' `TestMain` files would not compile.
- **Rejected:**
  A single fixture module importing fabric — impossible for the reason above.

### `hubforge` builds hubs via `fabriccli.CloneAndWire`, not `fabricengine.CloneHub`

- **Decision:**
  `hubforge.NewHub` calls `fabriccli.CloneAndWire`.
- **Rationale:**
  `CloneHub` alone produces a partial hub — warp clone, weft clone, board, anchor marker, warp binding, but no junctions and no repo-wide `fabric.yaml`.
  `fabrictest/hub.go`'s existing doc records that this leaves three of the destruction gate's eight path-ownership kinds (`ownedWiredJunction`, `ownedDriftedWiredJunction`, `ownedUnderGeometryRoot`) structurally unreachable.
- **Rejected:**
  Importing only `fabricengine` and replicating the CLI wiring by hand — that duplicates `CloneAndWire`, which is the thing a second copy of is explicitly called out as a hazard at `internal/fabriccli/clone.go:26`.
- **Consequence, and it is the load-bearing one:**
  `hubforge`'s dependency set is `fabriccli`'s 36 internal packages, not `fabricengine`'s 16.
  The original design measured only the latter.
  See "Technical context" for the measured blast radius.

### Copy the bares, clone the hub

- **Decision:**
  A `sync.Once` template builds one warp bare and one weft bare per test binary.
  Each fixture copies that pair into its own `tb.TempDir()`, then clones a hub from the copies.
- **Rationale:**
  Bare repos contain zero symlinks, so the existing recursive copy handles them (~2 ms per pair).
  A hub cannot be copied: its junctions carry absolute targets, so a filesystem copy leaves every link aimed at the template.
  Measured 2026-08-10 on Linux/WSL2 (Core Ultra 7 155U, 14 logical CPUs): full fixture 24 ms concurrent, against today's `CopyPaired` at 2.3 ms.
  132 sites × 24 ms ≈ 3.2 s against today's ≈ 0.30 s, so about +2.9 s on Tier 2's ~132 s — roughly 2.2%.
- **Template temp-dir lifetime:**
  `buildBareTemplate` allocates via `os.MkdirTemp` with no cleanup and deliberately keeps it that way — one template pair per test binary, left to the OS temp reaper.
  This matches today's behaviour in both `lyxtest` and `fabrictest/hub.go:73`.
  It cannot be `tb.TempDir()`: the template outlives any single test, and a `TestMain` cleanup would race the `sync.Once` under parallel packages.
  Only the per-fixture copies are owned by `tb`.
- **Two recipe gotchas belong in the factory, not at call sites:**
  `git init --bare` leaves `HEAD` on `master` even when the branch pushed is `main`, fixed with `git -C <bare> symbolic-ref HEAD refs/heads/main`;
  and the weft bare must stay genuinely empty and never be pushed to, or `CloneHub`'s bootstrap guard (`clone.go:172`, `!probe.WeftLooksLikeWeft`) refuses it.
  Both are already encoded correctly in `fabrictest/hub.go:71-112` and must survive the move verbatim.
- **Rejected:**
  Building fresh bares per fixture — a full `git init --bare` plus work repo plus commit plus push per test.

### Local bare repos are the remote substrate

- **Decision:**
  Fixture remotes are local bare repos reached by path.
  Push, pull and sync tests need no GitHub.
- **Rationale:**
  A bare repo reached by path is a first-class remote with identical refs, fast-forward rules, rejections and rebase-retry behaviour, and fabric cannot tell the difference — its push goes `gitrepo.Push` → `gitexec` → the git CLI.
  The repo already relies on this: `pull_integration_test.go:73,78` force-pushes from a second clone to build the diverged upstream `Fabric.Pull` re-anchors from, and `coalesce_integration_test.go:128-138` forces a genuine non-fast-forward through `gitrepo.Push`'s rebase-retry.
  **`protocol.file.allow` is deliberately not set.**
  That restriction targets submodule cloning (CVE-2022-39253), not ordinary clone/fetch/push against a path-reached bare, and `internal/lyxtest/hermetic.go:33-43`'s neutral config sets only user/init/core/maintenance/gc.
  `fabrictest/hub.go` already clones and pushes local bares today without it.
  Adding it would be an unlisted edit to `HermeticGitEnv`, which this task otherwise carries over unchanged.
- **Rejected:**
  Any real-remote fixture substrate — it would make 132 fixtures slow, flaky and credential-dependent for no gain in fabric coverage.

### Migrate all 132 above-fabric sites; delete three helpers, rename the fourth

- **Decision:**
  Every `Copy*` call site outside `internal/lyxcwd` moves to `hubforge.NewHub` — 132 of 141.
  `CopyPaired` (49), `CopyPairedLocal` (29) and `CopyWeft` (42) are deleted outright.
  `CopyWarpHub` (21) is **not** a primitive: it is hub-shaped, and 12 of its 21 sites are above fabric and migrate to `hubforge` like the rest.
  Its surviving 9 `lyxcwd` sites become `gitkit.CopyRepo`, returning `RepoFixture{Repo, Bare}`.
  The caller allowlist is `internal/lyxcwd` **alone** — `internal/gitrepo` has zero `Copy*` sites today (18× `MustRun` only), so listing it would pre-authorise exactly the drift this task forbids.
- **The rename is not cosmetic.**
  Today's `CopyWarpHub` returns `WarpFixture{Hub, Bare}`, and the field named `Hub` holds a directory that is not a hub — the field name is itself part of the invented shape this task removes.
  `CopyRepo`/`RepoFixture{Repo, Bare}` names what it actually is: a git repo with a bare origin.
- **Rationale:**
  A surviving general-purpose helper restores exactly the discipline-not-construction failure mode this task exists to remove.
  A guard test pins `CopyRepo`'s caller set to `internal/lyxcwd` alone so drift back is a test failure.
  Do not confuse that allowlist with `gitkit`'s broader consumer set: `MustRun`, `SeedConfig`, `HermeticGitEnv` and `GitStatusPorcelain` are used by many packages including `internal/gitrepo` (18× `MustRun`), and are not pinned.
  Only `CopyRepo` — the fixture that could reintroduce a hand-assembled hub — carries the one-package allowlist.
- **Rejected:**
  Keeping all four as a cheap tier;
  migrating only the hub-shape-sensitive packages;
  keeping the `CopyWarpHub` name on the narrowed helper.

### `gitrepo` and `lyxcwd` keep primitive fixtures

- **Decision:**
  Neither package gets a hub fixture, ever.
- **Rationale:**
  Both sit below fabric and are inside its dependency set, so importing `hubforge` is a compile error regardless.
  Independently of that: keeping the low-level packages on primitive fixtures preserves a layer that still fails on its own when a fabric clone bug lands, instead of every package in the repo failing at once.
- **Rejected:**
  Routing every fixture through clone.

### The two stuck in-package files move to external test packages

- **Decision:**
  `internal/treadleengine/smoke_judge_test.go` becomes `package treadleengine_test`, and `internal/loomengine/preflight_integration_test.go` becomes `package loomengine_test`, each with an `export_test.go` shim in the original package.
  Both then import `hubforge` directly.
- **Rationale:**
  Both packages are inside `fabriccli`'s dependency set, so an in-package test cannot import `hubforge`.
  Both need unexported access — `runCircling` and `judgeInputs` in treadleengine, `checkResolved` (26 uses) in loomengine — which the shim provides.
  `internal/fabricengine/export_test.go` is the existing precedent.
  `preflight_integration_test.go` is the higher-value migration: it hand-rolls `seedRepoWideFabricConfig`, another stand-in-hub hack.
- **Rejected:**
  A `loomtest`/`treadletest` fixture subpackage — it would import `hubforge` → `fabriccli` → the parent package, so an in-package test still could not reach it;
  the subpackage does not solve the cycle, moving the test file does.
  Leaving both on primitive fixtures.

### Build tags on the merged packages

- **The problem.**
  `hubforge` merges an untagged source (`internal/lyxtest/lyxtest.go`) with an integration-tagged one (`internal/fabricengine/fabrictest/hub.go:1` is `//go:build integration`).
  Tagging every file `integration` would leave `internal/hubforge` with zero files in the untagged build, which the stated `go vet ./...` gate runs.
- **Decision:**
  `hubforge` and `gitkit` production code is **untagged**, exactly as `internal/lyxtest/lyxtest.go` is today.
  Each package's own git-spawning tests carry `//go:build integration` per the Test Tier Purity Invariant.
  Each keeps an untagged `doc.go`, which is `fabrictest`'s own existing pattern — every file there is integration-tagged except `doc.go`.
- **Why untagged production is safe:**
  the Test Tier Purity Invariant bans untagged *tests* from calling `gitkit.Copy*` and `hubforge.NewHub` by token, not by build tag, and CONSTRAINTS already carries those tokens.
  Verified: all 132 current `Copy*` call sites already sit in tagged files, so nothing regresses.

### Config seeding on a real hub — 56 sites, and most of them shrink

- **The problem.**
  `lyxtest.SeedConfig(tb, repoDir, …)` writes `<repoDir>/_lyx/config/<module>.yaml` then runs `git add .` + `git commit` in `repoDir` (`internal/lyxtest/lyxtest.go:38-58`).
  On a real hub neither of its two current arguments works.
  32 of the 56 call sites pass `fixture.Hub`, which on a real hub is the `<name>-HUB` container — **not a git repo at all**, so the commit fails outright.
  The other 21 pass `fixture.WeftPrime`, and 3 pass an ad-hoc path (`warpSubdir`, `sibling`, `nested`).
  Seeding into the *warp* worktree would also fail: `<worktree>/_lyx` is a weft junction excluded from the warp's index via `.git/info/exclude`, so `git add .` stages nothing and the commit errors.
- **Decision — three-way split:**
  1. **Most sites stop seeding entirely.**
     `fabriccli.CloneAndWire` already runs `configsync.ReconcileAll(res.WeftBase, true)` and `configsync.ReconcileFabricAt(res.BoardDir, true)`, so a real hub arrives with materialized default config for every registered module.
     The fake fixture carried only a placeholder, which is the sole reason these sites seed at all.
     A site that seeds a module's plain `ConfigTemplate()` can simply delete the call.
  2. **Sites that override a value** call a new `hubforge.SeedConfig(tb, h *Hub, map[string]string)`, which writes into **`h.WeftBase`** — a new field populated verbatim from `res.WeftBase` — and commits in the weft worktree.
     `WeftBase` is **anchor-joined**: `internal/fabricengine/clone.go:406` computes it as `filepath.Join(WeftWorktree(l), l.AnchorRel)`.
     It is **not** `PrimeWeft()`, which returns the un-anchored weft sibling (`fabricengine.WeftWorktree(h.Location)`).
     At the `"backend"` anchor the two differ, and seeding into the un-anchored path would write `<weft>/_lyx/config` while every module loader reads `<weft>/backend/_lyx/config` — the override would silently not take effect, with no error.
     Since `NewHub` supports both anchors, this distinction is load-bearing, not theoretical.
  3. **Repo-wide fabric config** goes to `res.BoardDir` via a separate `hubforge.SeedFabricConfig`, matching `repoWideFabricBase(l) = BoardDir(l.HubPath)`.
     **It commits**, through `fabricengine.NewBolt(BoardDir).Commit(...)` — the same path `CloneAndWire` itself uses at `internal/fabriccli/clone.go:57-58` to leave the board clean after `ReconcileFabricAt`.
     Leaving the board dirty is not safe: `BoardDir` is the `weft:main` checkout the destruction gate's dirtiness check observes, so an uncommitted seed would silently change verb outcomes in fabric's own live-state cells.
- **`gitkit.SeedConfig` keeps its current body unchanged**, restricted to primitive repos alongside `CopyRepo`.
- **Rationale:**
  The 32 `fixture.Hub` sites are the same stand-in-hub lie as the fixtures themselves — config seeded at a container path that no production code would ever read from.
  Retargeting them onto the weft is not a mechanical rename;
  each one needs its intent read, which is why this is called out as its own migration axis rather than folded into the `Copy*` count.
- **Rejected:**
  Keeping a single `SeedConfig` that guesses its base from the path shape — it would silently pick wrong on the ad-hoc sites.
  Seeding through the warp-side `_lyx` junction — excluded from the warp index by design.

### Teardown discovers junctions by walking, not by slug

- **The problem.**
  The junction inventory (`<hub>/_portals/<slug>`, `<hub>/_launchers/<slug>`, `<worktree>/_lyx`, `<worktree>/.lyx`, `<worktree>/_board`) is slug-parameterised, but `hubforge` does not know the slug set at cleanup time: for fabric's own live-state tests the pairs are created by the verb under test, and some are destroyed by it.
  `fslink.RemoveLinksIn` covers only the immediate children of one directory.
- **Decision:**
  Teardown does a `filepath.WalkDir` from the hub root, calling `fslink.IsLink` on every entry and `fslink.Remove` on each link found.
  **It never descends into a link, and it must not return `filepath.SkipDir` for one.**
  `WalkDir` reports a link as a *non-directory* entry and never follows it, so non-descent is already guaranteed by construction — nothing has to be done to get it.
  Returning `SkipDir` from a non-directory callback skips **the remaining entries of the containing directory**, which would abandon every sibling junction: removing `<hub>/_portals/<slug1>` would leave `<slug2>` onward wired, and removing `<worktree>/.lyx` would leave `_lyx` and `_board` behind.
  The callback removes the link and returns `nil`.
  Errors are logged, never fatal: teardown must not fail a test that already passed.
- **Behaviour on a hand-removed worktree:**
  nothing special — a missing directory simply yields no entries, and `fslink.Remove` is documented idempotent (returns nil when the link is absent).
- **Rationale:**
  Slug-free discovery is the only mechanism that survives the deliberately-corrupt hubs fabric's live-state matrix plants.
  Enumerating worktrees from fabric and applying `RepoWiredNames` per worktree requires fabric to still be functional against that hub, which is exactly what those tests break.
  Walking ~155 entries per fixture is negligible beside the 24 ms clone.
- **Rejected:**
  Enumerate-worktrees-then-`RepoWiredNames`;
  `RemoveLinksIn` on a hardcoded site list.

### The fixture benchmarks are retargeted, not deleted

- **Decision:**
  `internal/lyxtest/bench_test.go`'s four benchmarks move to `internal/hubforge` and are retargeted:
  `BenchmarkNewHub` and `BenchmarkNewHubParallel` measure the full fixture (bare copy plus clone plus wire), and `BenchmarkCopyBares` measures the bare-copy step alone.
  `gitkit` keeps one benchmark for the surviving `CopyRepo`.
- **Rationale:**
  The benchmarks are what produced the numbers this whole design rests on, and the clone-versus-copy comparison stays live precisely because the runtime cost is the standing objection.
  Deleting them would leave the +2.9 s claim unfalsifiable from inside the repo.
- **`docs/benchmarks/fixture-copy.md` is updated in the same commit:**
  its Reproducing section names the new benchmark identifiers, and its recorded measurements are kept as historical rows with their date and hardware intact rather than rewritten.

### Junction-safe teardown lives in `hubforge`

- **Decision:**
  `hubforge` registers a `tb.Cleanup` that enumerates the hub's junction sites and calls `fslink.Remove` on each, before `tb.TempDir()`'s own removal runs.
  Cleanup is LIFO, so registering after `TempDir` is what orders it correctly.
- **Rationale:**
  `fslink.Remove` is documented to remove only the link entry, never the target, and `fslink` is the repo's mandated cross-OS link primitive.
  This is the Win11 safety requirement: a hub fixture contains junctions inside a temp dir that Go will `os.RemoveAll`, 132 times per suite run.
- **Rejected:**
  `fabricengine.Unwire` — it is per-warp-worktree rather than per-hub, it deliberately never touches repo-wide `weft:main` records or weft-side content, and it carries refusal semantics that would fail teardown on exactly the deliberately-corrupt fixtures the live-state matrix plants.
  Adding a hub-teardown verb to `fabricengine` — new production code, out of scope.
  Relying on `os.RemoveAll` alone — leaves the Win11 question open.

### Parallel safety is `hubforge`'s guarantee; `t.Chdir` is not this task's problem

- **Decision:**
  `hubforge.NewHub` is safe under concurrent use and carries a test proving it (N concurrent `NewHub` calls).
  No call site is migrated off `t.Chdir` in this task.
- **Rationale:**
  Parallel safety is structural: a `sync.Once` template built once, then per-test `tb.TempDir()` for the bare copies and the hub.
  Nothing is shared but a read-only template.
  Roughly 20 fixture-using files call `t.Chdir`, which Go makes incompatible with `t.Parallel`, and unblocking them means touching CLI signatures.
- **Follow-up:**
  Filed as wiki task `hubforge-parallel-chdir`, `depends_on: lyxtest-real-hubs`.

### Windows: correct by construction, unmeasured

- **Decision:**
  No Windows benchmark, no measurement gate.
  Every path must be Win11-correct by construction.
- **Concretely:**
  all links go through `internal/fslink` (`CreateDirLink`, directory-only);
  remote URLs pass through `filepath.ToSlash` before reaching git, as `fabrictest/hub.go:212-213` already does;
  teardown removes junctions before `RemoveAll`;
  no reliance on Windows file symlinks.
- **Rationale:**
  Even a 5× worse clone-versus-copy ratio is roughly +15 s on a 132 s Tier 2 run, so it is not a design blocker.
- **Related:**
  `manifest/designs/fabric-windows-verification.md` carries the same platform gap for correctness rather than speed.

## Technical context

### Where the code is today

- `internal/lyxtest/lyxtest.go` (583 lines) — the four `Copy*` helpers, three `sync.Once` template builders (`buildWarpHub`, `buildWeftPrime`, `buildWeftOnly`), `MustRun`, `SeedConfig`, `copyDirRecursive`, `rewriteOriginURLInConfig`, `initRepo`, `initBareRemote`, `mustGit`, `stripHookSamples`.
- `internal/lyxtest/hermetic.go` (70) — `HermeticGitEnv`;
  `reexecguard.go` (50) — `refuseCLIReexec`;
  `leaf_enforcement_test.go` (94) — `TestLeafInvariant_AllowlistOnly`, an AST-based import allowlist walk that becomes `gitkit`'s.
- `internal/lyxtest/bench_test.go` (48) — `BenchmarkCopyPaired`, `BenchmarkCopyPairedLocal`, `BenchmarkCopyPairedParallel`, `BenchmarkCopyPairedLocalParallel`.
  All four call helpers this task deletes, and they are the permanent probes `docs/benchmarks/fixture-copy.md` documents a Reproducing section for.
  Disposition below.
- `internal/fabricengine/fabrictest/hub.go` (361 lines) — **the model to promote.**
  `buildBareTemplate`, `copyBares`, `Hub` + geometry accessors (`PrimeWorktree`, `PrimeWeft`, `BoardDir`, `PairWarpWorktree`, `PairWeftSibling`, `PairPortalLink`, `PairLauncherDir`), `NewHub(tb, anchor)`, `AddPair`, `GitStatusPorcelain`.
  **Anchored vs un-anchored, on both sides.**
  `PrimeWorktree()` and `PrimeWeft()` both return worktree *roots*, not anchor paths.
  At a `"backend"` anchor the anchored warp path is `Location.AnchorPath()` and the anchored weft path is `res.WeftBase`.
  `hubforge.Hub` therefore carries `WeftBase` as its own field, populated verbatim from `CloneResult`, and anything reading or writing `_lyx/config` uses the anchored form.
  Getting this wrong fails silently rather than loudly, which is why it is called out here.
  Its private `mustGit`, `copyDirRecursive`, `initBareRepo`, `initScratchRepo`, `commitAll`, `stripHookSamples` are, by its own comments, copies of `lyxtest`'s — the merge deletes one side of each.
- `internal/fabricengine/fabrictest/` remainder (~4960 lines) — `states.go` (422), `verbs.go` (1409), `manifest.go` (461), `mutationoracle.go` (371), `refusal.go` (56), `doc.go` (317), plus 1394 lines of tests of the harness itself.
  `doc.go` is a substantial design document and must survive the move, retargeted;
  its "crucible campaign" framing should be dropped, since `crucible/` is three markdown prompt files, a review process with no code.

### Measured blast radius of the `fabriccli` dependency set

`internal/fabriccli` transitively pulls 36 internal packages (it reaches `configreg`, hence `boardengine`, `burlerengine`, `loomengine`, `perchengine`, `shuttleengine`, `treadleengine`, `websterengine`).
`internal/fabricengine` pulls 16.
The original design measured against the 16.
In-package test files calling `lyxtest.*` inside `fabriccli`'s set, measured 2026-08-12:

| package | files | what they use | disposition |
|---|---|---|---|
| `fabricengine` | 14 | 100× `MustRun`, 38× `CopyWeft`, 5× `CopyWarpHub`, 4× `HermeticGitEnv`, 2× `CopyPairedLocal`, 0× `SeedConfig` | only `MustRun`/`HermeticGitEnv` stay on `gitkit`; all 45 in-package `Copy*` sites move to `package fabricengine_test` + `hubforge` |
| `gitrepo/gogit_test.go` | 1 | 18× `MustRun` | `gitkit`, unchanged |
| `lyxcwd/gate_test.go` | 1 | 1 call | `gitkit`, unchanged |
| `boardengine` | 2 | 4× `MustRun`, `HermeticGitEnv` | `gitkit`, unchanged |
| `websterengine`, `perchengine`, `burlerengine` | 1 each | `HermeticGitEnv` only | `gitkit`, unchanged |
| `treadleengine/smoke_judge_test.go` | 1 | `CopyPaired`, `SeedConfig` | external test package + `export_test.go` |
| `loomengine/preflight_integration_test.go` | 1 | `CopyPaired`, 11× `PairedFixture`, 8× `MustRun` | external test package + `export_test.go` |

`burlerengine`'s two `CopyPaired` sites are already in `package burlerengine_test` and are not stuck.

**This row is re-derived with the same call-expression method as the `Copy*` table, and reconciles with it.**
`internal/fabricengine`'s 82 `Copy*` sites split in-package/external as: `CopyWeft` 38/2, `CopyPairedLocal` 2/23, `CopyWarpHub` 5/2, `CopyPaired` 0/10 — 45 in-package, 37 already external.
`SeedConfig` has **zero** in-package sites in `fabricengine`; all 19 are already in external test packages, so that package's share of the seeding migration needs no file moves.

**`CopyWarpHub` is hub-shaped, not primitive.**
Its 21 sites split 5 in-package `fabricengine` (`hook_test.go` 4, `warplayout_test.go` 1), 2 external `fabricengine_test` (`unwire_test.go`, `worktreelist_test.go`), 2 `fabriccli`, 2 `idecli`, 1 `webstercli`, and 9 `lyxcwd`.
The first 12 migrate to `hubforge`;
the 5 in-package ones move to `package fabricengine_test` on the way.
Only `lyxcwd`'s 9 stay behind, as `gitkit.CopyRepo`.
Reading the row above as "all `fabricengine` primitives stay on `gitkit`" would leave those 5 sites calling a helper the `gitkit` caller-set guard test forbids.

### `Copy*` call sites, measured 2026-08-12

**Counting method — reproduce before trusting these numbers.**
Count *call expressions*, not matching lines:
`grep -rho "lyxtest\.<Helper>(" --include=*.go internal cmd | wc -l` per helper, and the same with `find <pkg> -maxdepth 1 -name '*.go'` per package.
The trailing `(` is what excludes doc-comment mentions.
A line-based count over-reports by 22: `internal/fabricengine` alone carries 11 comment mentions, and `cmd/lyx` carries 6 that are entirely comment text plus two guard-token string literals.

| package | CopyPaired | CopyPairedLocal | CopyWarpHub | CopyWeft | total |
|---|---|---|---|---|---|
| `internal/fabricengine` | 10 | 25 | 7 | 40 | 82 |
| `internal/reedcli` | 20 | — | — | — | 20 |
| `internal/fabriccli` | 8 | — | 2 | 1 | 11 |
| `internal/lyxcwd` | — | — | 9 | — | 9 |
| `internal/perchcli` | 2 | 4 | — | — | 6 |
| `internal/shuttlecli` | 4 | — | — | — | 4 |
| `internal/burlerengine` | 2 | — | — | — | 2 |
| `internal/idecli` | — | — | 2 | — | 2 |
| `internal/configcli` | 1 | — | — | — | 1 |
| `internal/loomengine` | 1 | — | — | — | 1 |
| `internal/treadleengine` | 1 | — | — | — | 1 |
| `internal/boardengine/boardtest` | — | — | — | 1 | 1 |
| `internal/webstercli` | — | — | 1 | — | 1 |
| **total** | **49** | **29** | **21** | **42** | **141** |

Row totals and column totals both sum to 141.

**`cmd/lyx` has zero call sites** and is absent from this table deliberately — its only occurrences are comment text and the two guard-token string literals in `tierpurity_test.go:57` and `hermeticenv_test.go:51`, which are still updated under "Files naming the old packages by path" below.

**132 sites migrate** — 141 minus `internal/lyxcwd`'s 9, which stay on `gitkit` per the scoping rule.
The count is high because it is one fixture per test function for isolation: 279 test functions and 116 local setup helpers live in these files.

### `SeedConfig` call sites, measured 2026-08-12

Same counting method as the `Copy*` table: 56 call expressions of `lyxtest.SeedConfig(`.

| package | sites |
|---|---|
| `internal/reedcli` | 21 |
| `internal/fabricengine` | 19 |
| `internal/perchcli` | 6 |
| `internal/shuttlecli` | 4 |
| `internal/burlerengine` | 2 |
| `internal/configcli`, `internal/loomengine`, `internal/treadleengine`, `internal/webstercli` | 1 each |
| **total** | **56** |

By base argument: 32 pass `fixture.Hub`, 21 pass `fixture.WeftPrime` (or `f.WeftPrime`), 3 pass an ad-hoc path (`warpSubdir`, `sibling`, `nested`).
The 32 `fixture.Hub` sites are the ones that cannot survive unchanged — see the Decision above.

### Assertion migration is the real work

A real hub is ~155 files against the templates' ~36, and carries `_board`, `_portals`, `_launchers`, junctions, an anchor marker, hub-level `.lyx` and repo-wide `fabric.yaml` that today's fixtures lack.
Directory listings, file counts and "this path should not exist" assertions will break.
**Every break is worth reading rather than silencing** — it marks a place currently asserting against an invented shape.

Two specific shapes to expect:

- The 40 `CopyWeft` sites in `fabricengine` pair an unrelated weft with a `newWarpFixture` warp via `NewPairedForTest`, a shim in `internal/fabricengine/export_test.go` with 22 call sites across 4 files (`warpforward_integration_test.go` 10, `weftgit_exclude_test.go` 4, `fabric_test.go` 4, `checkout_index_refresh_test.go` 2).
  A real hub's `PrimeWorktree`/`PrimeWeft` is a genuine pair, so the shim is deleted.
- Sites that seed config into a fixture's parent directory as a stand-in hub (`weftgit_exclude_test.go`, `loomengine/preflight_integration_test.go`) drop that scaffolding entirely.

### Files naming the old packages by path

These reference `internal/lyxtest` or `internal/fabricengine/fabrictest` as strings and must be updated:

- `internal/lyxcwd/enforcement_test.go:604-619` — two allowlist maps keyed on `internal/fabricengine/fabrictest`.
- `cmd/lyx/destructiveguard_test.go:124` — subtree exclusion for `internal/fabricengine/fabrictest`.
  Its walk already skips `*_test.go` (`destructiveguard_test.go:231`), so once the live-state builders are `package fabricengine_test` files the exclusion is dead and is deleted.
- `cmd/lyx/tierpurity_test.go:50,57` and `cmd/lyx/hermeticenv_test.go:51` — banned-token string data (`"lyxtest.Copy"`).
- `internal/fabriccli/clone.go:26` and `internal/fabricengine/mutation.go:214` — comments naming `fabrictest`.

### Markdown naming the old packages — 10 files, one of them a build break

The Go sweep above is not the whole surface.
Reproduce with `grep -rln "lyxtest\|fabrictest" --include=*.md .` (excluding `_mill/`):

- `manifest/roadmap.md` — **the build break.**
  Line 22 sits under `## Planned` and links `[designs/lyxtest-real-hubs.md](designs/lyxtest-real-hubs.md)`.
  Deleting that design doc without moving the entry breaks the machine-checked Markdown Link Integrity invariant (`CONSTRAINTS.md:280`).
  Line 21 also names `fabrictest` as the landing zone slice 13 created.
  This resolves the roadmap question outright: `lyxtest-real-hubs` **is** Planned item 1, so moving it Planned → Done with the link repaired is in scope, not optional.
- `docs/benchmarks/fixture-copy.md` — 13 references including a Reproducing section;
  updated with the retargeted benchmarks per the Decision above.
- `docs/benchmarks/running-tests.md`, `docs/benchmarks/test-suite-timing.md`, `docs/benchmarks/scout-vs-grep.md` — tier and timing prose naming `lyxtest.Copy*`.
- `docs/shared-libs/lyxcwd.md` — names lyxtest's synthetic hubs as the `ResolveWithAnchor` bypass caller;
  must match the `CONSTRAINTS.md` wording already changed to `gitkit`'s primitive repo fixtures.
- `docs/overview.md` — the module table, already in scope.
- `manifest/designs/fabric-unified-view.md` — prose reference.
- `crucible/review-prompt-template.md` — names the retired "lyxtest Leaf" invariant;
  must name the `gitkit` Leaf and `hubforge` Fabric-Fixture invariants instead, or reviewers will keep checking against a rule that no longer exists.
- `CLAUDE.md` — prose reference.

`manifest/designs/lyxtest-real-hubs.md` is deleted, so it needs no rewrite — only the inbound link from `roadmap.md`.

### Reusable pieces

- `internal/fslink` — `Remove` (link-only, idempotent), `RemoveLinksIn` (immediate children of one dir only), `IsLink`, `PointsTo`, `RawTarget`, `CreateDirLink`.
- `internal/fabricengine` — `WiredNames(baseDir)`, `RepoWiredNames(l)`, `HubReservedNames()`, `BoardDir(hub)`, `WorktreePath`, `WeftWorktreePath`, `PortalLink`, `LauncherDir`.
  A teardown walk needs more than `RemoveLinksIn` on one directory: junctions sit at `<hub>/_portals/<slug>`, `<hub>/_launchers/<slug>`, `<worktree>/_lyx`, `<worktree>/.lyx`, `<worktree>/_board`.
- `fabriccli.CloneAndWire(cwd string, opts fabricengine.CloneOptions) (fabricengine.CloneResult, error)` — note the first parameter is `cwd`, not a container.
  `CloneResult` carries an embedded `MutationRecord` plus `HubPath`, `Anchor`, `BoardDir`, `WeftBase`, `PrimeCwd`, `WarpURL`, `WarpBindingRecorded`.
  `hubforge`'s accessors should take `BoardDir` and `WeftBase` from the result rather than re-deriving them.
- `internal/fabricengine/export_test.go` — the shim precedent, and itself in scope.
  It is **extended as needed** for the 14 in-package files becoming `package fabricengine_test`: each unexported identifier they reach gets an exported alias there.
  In the same pass `NewPairedForTest` is deleted from it.
  The shim's growth is planned work, not something the implementer discovers mid-migration.
- `internal/boardengine/boardtest` — precedent for a test-only sibling package.

## Constraints

`CONSTRAINTS.md` has **already been rewritten in this worktree** to describe the post-task state, so that reviewers do not flag `discussion.md` against a stale invariant.
It is therefore ahead of the code until this task lands — the tree still contains `internal/lyxtest` and `internal/fabricengine/fabrictest`, and the enforcement tests it names do not exist yet.
Making it true is part of this task's definition of done.

Changes already applied:

- **`lyxtest` Leaf Invariant → `gitkit` Leaf Invariant.**
  Same allowlist (stdlib + `lyxcwd`, `weftname`, `configengine`, `lyxdirs`);
  adds that `gitkit` owns `MustRun`/`SeedConfig`/`HermeticGitEnv`/`GitStatusPorcelain`, and that `gitkit.CopyRepo` alone is pinned — callable from `internal/lyxcwd` only.
  The other helpers are unpinned;
  `internal/gitrepo` is a `MustRun` consumer with no fixture call.
  Enforced by `internal/gitkit/leaf_enforcement_test.go` (`TestLeafInvariant_AllowlistOnly`) — **must be created**, ported from `internal/lyxtest/leaf_enforcement_test.go`.
- **New: `hubforge` Fabric-Fixture Invariant.**
  Every hub fixture is built by `hubforge` through `fabriccli.CloneAndWire`;
  `hubforge` asserts nothing about fabric;
  no package inside `fabriccli`'s dependency set may import it;
  `NewHub` is concurrency-safe;
  teardown removes junctions via `fslink` before `tb.TempDir()` cleanup.
  Self-enforcing — an in-package test importing `hubforge` from inside fabric's dependency set is a compile error.
- **Cwd Resolution Invariant** — the `ResolveWithAnchor` bypass note now reads "fabric's clone, `gitkit`'s primitive repo fixtures".
- **Fabric Vocabulary owner set** — `internal/lyxtest` replaced by `internal/gitkit` and `internal/hubforge`;
  `internal/fabricengine/fabrictest` removed from both the owner set and the narrower `weftname`-import subset.
- **Fabric Destruction Chokepoint Invariant** — the `fabrictest` subdirectory-exclusion bullet is replaced by a note that the guard's walk skips `*_test.go`.
- **Test Tier Purity Invariant** — banned tokens for untagged tests are now `gitkit.Copy*` and `hubforge.NewHub`.
- **Hermetic Git Test Environment Invariant** — fixture-helper tokens are `gitkit.Copy*`, `gitkit.MustRun`, `gitkit.SeedConfig`, `hubforge.NewHub`;
  `TestMain` calls `gitkit.HermeticGitEnv()`.

Unchanged invariants this task must still respect:

- **Cwd Resolution Invariant** — `hubforge` obtains its `*lyxcwd.Location` from `lyxcwd.Resolve`, never by hand.
- **CLI/Cobra Invariant** — no CLI signature changes in this task.
- **Documentation Lifecycle** — `manifest/designs/lyxtest-real-hubs.md` is deleted on landing;
  its durable half is the two CONSTRAINTS invariants above plus the new packages' own doc comments.
- **Markdown** — semantic line breaks, no fixed-column hard wrap.
- **Task completion** — module docs and `docs/overview.md` land in the same commit.
  `manifest/roadmap.md` moves only if this is a planned roadmap item.

## Testing

`hubforge` and `gitkit` are test infrastructure, so "tests" here means both tests *of* the new packages and the migration of the 132 sites that consume them.

**`internal/hubforge` — TDD candidates, write these first:**

- `NewHub` produces a real hub: `_board` exists, the anchor marker exists, the hub-level `.lyx` is a real directory and not a link, junctions are wired, repo-wide `fabric.yaml` is present, `weft:main` carries the warp-URL binding.
  This is the test that makes drift impossible;
  it should assert against `fabricengine`'s own name accessors rather than hardcoded strings.
- `NewHub` at both anchors, `"."` and a subpath.
- **Concurrency**: N concurrent `NewHub` calls each produce an independent, correct hub, and no two share a path.
- **Teardown**: after a fixture completes, every junction is gone and — the load-bearing assertion — each junction's *target* content still exists.
  Must run on Windows semantics in mind;
  guard with `runtime.GOOS` only if a platform genuinely diverges.
- Teardown succeeds against a deliberately corrupted hub (a junction repointed, a worktree directory removed by hand).
- The bare-template gotchas: the warp bare's `HEAD` is on `refs/heads/main` after push, and the weft bare is genuinely empty.
  Port `fabrictest/hub_test.go` (149 lines), which already covers `buildBareTemplate` and `NewHub`.

**`internal/gitkit`:**

- `TestLeafInvariant_AllowlistOnly` ported from `internal/lyxtest/leaf_enforcement_test.go` — the AST import-allowlist walk.
  This is the machine half of the leaf invariant and must exist before the migration starts, or nothing stops `gitkit` from growing a fabric import.
- A guard test pinning `gitkit.CopyRepo`'s caller set to `internal/lyxcwd` alone.
  This is what catches a migration that leaves one of `fabricengine`'s 5 in-package `CopyWarpHub` sites behind.
- Existing `lyxtest_test.go` (389 lines) and `reexecguard_test.go` coverage carries over for the retained helpers.

**`SeedConfig` migration — verify per site, do not sweep:**

- For each of the 56 sites, decide which of the three outcomes applies (drop / `hubforge.SeedConfig` into the weft / `hubforge.SeedFabricConfig` into the board).
  A site whose seeded YAML equals the module's plain `ConfigTemplate()` is a drop candidate, since `CloneAndWire` already materialises it.
- `hubforge` needs a test proving a real hub arrives with materialised config for a registered module *without* any seeding, since that is what licenses the drops.
- `hubforge.SeedConfig` needs a test proving the value it writes is what the module's config loader reads back through the warp-side `_lyx` junction — the seed goes in on the weft side and must be visible from the warp side.
- **The same test must run at the `"backend"` anchor, not only at `"."`.**
  This is the regression test for the anchored-base error: seeding into the un-anchored weft sibling writes a file no loader ever reads, and a `"."`-only test passes anyway because the two paths coincide there.

**Migration of the 132 sites:**

- Migrate package by package, smallest first, so the pattern is settled before `fabricengine`'s 82 sites.
  Suggested order, summing to 132: `webstercli`/`loomengine`/`treadleengine`/`configcli`/`boardtest` (1 each) plus `idecli`/`burlerengine` (2 each) = 9 → `shuttlecli` (4) + `perchcli` (6) = 10 → `reedcli` (20) → `fabriccli` (11) → `fabricengine` (82).
- Each broken assertion is read and re-expressed against the real shape, never silenced.
  A migration that deletes an assertion rather than re-pointing it needs an explicit note saying why.
- `internal/fabricengine`'s live-state machinery moves to `package fabricengine_test` in the same batch as its `fabrictest` deletion, so the tree never has both.
- After migration: `grep` proves zero remaining references to `lyxtest`, `fabrictest`, `CopyPaired`, `CopyPairedLocal`, `CopyWeft`, `NewPairedForTest` anywhere in `internal/` and `cmd/`.

**Regression gates:**

- Full Tier 2 run green (`go test -tags integration ./...`), and its wall-clock recorded against the ~132 s baseline.
  A delta materially worse than the predicted +2.9 s is a finding worth investigating, not a timing assertion to encode.
- `cmd/lyx/tierpurity_test.go` and `cmd/lyx/hermeticenv_test.go` pass with their updated token data.
- `cmd/lyx/destructiveguard_test.go` passes with the `fabrictest` exclusion removed.
- `internal/lyxcwd/enforcement_test.go` passes with its two allowlist maps updated.
- `go vet ./...` clean, and no import cycle anywhere — the compile is the primary proof of the `hubforge` invariant.

## Q&A log

- **Q:** Where do repo-wide hub fixtures live, given `fabrictest.NewHub` already exists and needs `fabriccli`? **A:** A dedicated factory module separate from both `lyxtest` and the fabric module — inverting `lyxtest` in place buys nothing because the stuck-file set is identical either way.
- **Q:** Is it really 163 `Copy*` calls, and why so many? **A:** No — 163 was a line-based count that included doc-comment mentions.
  The correct figure is 141 call expressions, of which 132 migrate.
  The count is high because it is one fixture per test function for isolation, across 279 test functions — not duplication.
- **Q:** Do we keep all four `Copy*` helpers if something better exists? **A:** No.
  Delete `CopyPaired`, `CopyPairedLocal`, `CopyWeft`;
  keep only the primitive one, narrowed to `gitrepo`/`lyxcwd`.
  A surviving general helper restores the discipline problem.
- **Q:** Windows measurement? **A:** Do not measure.
  Build every path so it is correct on Win11 — `fslink` for links, `ToSlash` for URLs, junction removal before `RemoveAll`.
- **Q:** Do we need our own teardown, or does fabric already have one? **A:** Fabric has `Unwire`, but it is per-worktree, deliberately partial, and refuses on hostile state.
  `hubforge` owns a small `fslink`-based teardown instead.
- **Q:** Why are `fabrictest` and the hub factory separate at all — isn't `hubforge` also testing fabric? **A:** No.
  `hubforge` contains zero assertions;
  it is a factory consumed by ~15 packages' tests.
  `fabrictest`'s machinery asserts about fabric, so it belongs with fabric as `package fabricengine_test`.
- **Q:** What is the difference between `fabrictest` and `fabricengine_test`? **A:** Importability, and nothing else.
  An external test package can import a fixture module that imports fabric — proven live: `dotlyxjunction_integration_test.go` (`package fabricengine_test`) → `fabrictest` → `fabriccli` → `fabricengine` compiles today.
  40 of fabricengine's 88 test files are already external.
  The cycle forces the 14 *in-package* files to move;
  it does not force the destination to be an importable package.
- **Q:** Should we have a test module only for the crucible campaign? **A:** No.
  `crucible/` is three markdown prompt files, a review process with no code.
  Test modules are named for what they do.
- **Q:** Is `lyxtest` the right name for the hub factory? **A:** No — too generic, and a factory whose name ends in `test` reads as if it runs tests.
  Hence `hubforge` (factory) and `gitkit` (leaf).
- **Q:** Should `CONSTRAINTS.md` be rewritten now or when the code lands? **A:** Now, before reviewers run, since they anchor on it and would otherwise flag `discussion.md` as contradicting a stale invariant.
  Keep it to short bullets — rules only, no rationale.
- **Q:** The `Copy*` table did not reconcile — how is the count re-derived? (discussion review r1, BLOCKING) **A:** Count call expressions (`lyxtest.<Helper>(`), never matching lines;
  the trailing paren excludes doc-comment mentions.
  141 call expressions, 132 migrating, `cmd/lyx` dropping out with zero real sites.
  Method stated in Technical context so it is reproducible.
- **Q:** Should all rounds' recommended resolutions be applied without asking? **A:** Yes — operator granted blanket approval for the recommended option in every review round.
- **Q:** What does `SeedConfig` mean on a real hub, where its 32 `fixture.Hub` sites point at a non-repo container? (discussion review r2, BLOCKING) **A:** Three-way split — drop the call where `CloneAndWire` already materialises the module's default config, else `hubforge.SeedConfig` into `PrimeWeft`/`WeftBase`, or `hubforge.SeedFabricConfig` into `BoardDir` for repo-wide fabric config.
  `gitkit.SeedConfig` keeps its body, restricted to primitive repos.
- **Q:** How does teardown discover junction sites when the slug set is created by the verb under test? (discussion review r2, BLOCKING) **A:** A slug-free `filepath.WalkDir` from the hub root using `fslink.IsLink`, never descending into a link (`SkipDir` on encountering one).
  Slug-free is the only mechanism that survives the deliberately-corrupt hubs fabric's live-state matrix plants.
- **Q:** Which build tags do the merged `hubforge`/`gitkit` files carry, given they merge an untagged and an integration-tagged source? (discussion review r3) **A:** Production untagged, git-spawning tests `//go:build integration`, one untagged `doc.go` each — `fabrictest`'s own existing pattern, and what keeps `go vet ./...` from seeing a package with zero untagged files.
- **Q:** Does `SeedFabricConfig` commit in `BoardDir`? (discussion review r3) **A:** Yes, via `fabricengine.NewBolt(BoardDir).Commit(...)`, matching `CloneAndWire`.
  An uncommitted seed would leave the `weft:main` checkout dirty, which the destruction gate's dirtiness check observes and which would silently change verb outcomes in fabric's live-state cells.
- **Q:** Is `hubforge.SeedConfig`'s base `PrimeWeft()`? (discussion review r4, BLOCKING) **A:** No — it is `res.WeftBase`, which is anchor-joined (`clone.go:406`), whereas `PrimeWeft()` is the un-anchored weft sibling.
  They coincide at `"."` and diverge at `"backend"`, where seeding the un-anchored path writes a file no module loader reads, silently.
  `Hub` carries `WeftBase` as its own field, and the seeding test runs at both anchors.
- **Q:** Does the hermetic git env need `protocol.file.allow always`? (discussion review r4) **A:** No.
  That restriction targets submodule cloning, not path-reached bares, and `fabrictest` already clones and pushes local bares without it.
  Dropped explicitly rather than left as an unlisted edit to `HermeticGitEnv`.
- **Q:** Should teardown return `filepath.SkipDir` after removing a link? (discussion review r5, BLOCKING) **A:** No — it returns `nil`.
  `WalkDir` reports a link as a non-directory and never follows it, so non-descent is free;
  `SkipDir` from a non-directory callback skips the containing directory's remaining entries, which would leave every sibling junction wired.
