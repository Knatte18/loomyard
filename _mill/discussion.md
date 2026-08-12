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
- Migrate all 154 above-fabric `Copy*` call sites onto `hubforge.NewHub`.
- Move `fabrictest`'s live-state machinery (`states.go`, `verbs.go`, `manifest.go`, `mutationoracle.go`, `refusal.go` and their tests, ~4960 lines) into `package fabricengine_test` files inside `internal/fabricengine/`.
- Move `internal/fabricengine`'s 14 in-package `lyxtest` callers off the leaf, and the two stuck in-package files in `treadleengine`/`loomengine` to external test packages with an `export_test.go` shim.
- Delete `CopyPaired`, `CopyPairedLocal`, `CopyWeft`, and `NewPairedForTest`.
- Junction-safe fixture teardown in `hubforge`.
- `CONSTRAINTS.md` rewrite — **already applied in this worktree**, see "Constraints" below.
- Module docs for `hubforge` and `gitkit`;
  `docs/overview.md` module table.
- Delete `manifest/designs/lyxtest-real-hubs.md` per the documentation lifecycle.

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
  154 sites × 24 ms ≈ 3.7 s against today's ≈ 0.35 s, so about +3.4 s on Tier 2's ~132 s — roughly 2.6%.
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
  Set `protocol.file.allow always` in the hermetic git env defensively.
- **Rejected:**
  Any real-remote fixture substrate — it would make 154 fixtures slow, flaky and credential-dependent for no gain in fabric coverage.

### Migrate all 154 above-fabric sites; delete three helpers

- **Decision:**
  Every `Copy*` call site outside `internal/lyxcwd` moves to `hubforge.NewHub`.
  `CopyPaired` (57), `CopyPairedLocal` (34) and `CopyWeft` (51) are deleted outright.
  `CopyWarpHub` (23) survives in `gitkit`, narrowed to `internal/lyxcwd` and `internal/gitrepo`, renamed to something that does not advertise itself as a hub.
- **Rationale:**
  A surviving general-purpose helper restores exactly the discipline-not-construction failure mode this task exists to remove.
  After migration the only remaining demand is `lyxcwd`'s 9 `CopyWarpHub` calls.
  A guard test should pin the allowed caller set so drift back is a test failure.
- **Rejected:**
  Keeping all four as a cheap tier;
  migrating only the hub-shape-sensitive packages.

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

### Junction-safe teardown lives in `hubforge`

- **Decision:**
  `hubforge` registers a `tb.Cleanup` that enumerates the hub's junction sites and calls `fslink.Remove` on each, before `tb.TempDir()`'s own removal runs.
  Cleanup is LIFO, so registering after `TempDir` is what orders it correctly.
- **Rationale:**
  `fslink.Remove` is documented to remove only the link entry, never the target, and `fslink` is the repo's mandated cross-OS link primitive.
  This is the Win11 safety requirement: a hub fixture contains junctions inside a temp dir that Go will `os.RemoveAll`, 154 times per suite run.
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
- `internal/fabricengine/fabrictest/hub.go` (361 lines) — **the model to promote.**
  `buildBareTemplate`, `copyBares`, `Hub` + geometry accessors (`PrimeWorktree`, `PrimeWeft`, `BoardDir`, `PairWarpWorktree`, `PairWeftSibling`, `PairPortalLink`, `PairLauncherDir`), `NewHub(tb, anchor)`, `AddPair`, `GitStatusPorcelain`.
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
| `fabricengine` | 14 | 101× `MustRun`, 43× `CopyWeft`, 5× `CopyWarpHub`, 4× `HermeticGitEnv`, 2× `CopyPairedLocal` | primitives stay on `gitkit`; hub-needing ones move to `package fabricengine_test` |
| `gitrepo/gogit_test.go` | 1 | 18× `MustRun` | `gitkit`, unchanged |
| `lyxcwd/gate_test.go` | 1 | 1 call | `gitkit`, unchanged |
| `boardengine` | 2 | 4× `MustRun`, `HermeticGitEnv` | `gitkit`, unchanged |
| `websterengine`, `perchengine`, `burlerengine` | 1 each | `HermeticGitEnv` only | `gitkit`, unchanged |
| `treadleengine/smoke_judge_test.go` | 1 | `CopyPaired`, `SeedConfig` | external test package + `export_test.go` |
| `loomengine/preflight_integration_test.go` | 1 | `CopyPaired`, 11× `PairedFixture`, 8× `MustRun` | external test package + `export_test.go` |

`burlerengine`'s two `CopyPaired` sites are already in `package burlerengine_test` and are not stuck.

### `Copy*` call sites, measured 2026-08-12

163 real call sites (an earlier count of 167/170 included guard-token string literals in `cmd/lyx/tierpurity_test.go` and `cmd/lyx/hermeticenv_test.go`, plus comment mentions).

| package | CopyPaired | CopyPairedLocal | CopyWarpHub | CopyWeft | total |
|---|---|---|---|---|---|
| `internal/fabricengine` | 14 | 28 | 7 | 46 | 95 |
| `internal/reedcli` | 21 | — | — | — | 21 |
| `internal/fabriccli` | 8 | — | 3 | 1 | 12 |
| `internal/lyxcwd` | — | — | 9 | — | 9 |
| `internal/perchcli` | 3 | 5 | — | — | 8 |
| `internal/shuttlecli` | 4 | — | — | — | 4 |
| `internal/configcli` | 2 | — | — | — | 2 |
| `internal/burlerengine` | 2 | — | — | — | 2 |
| `internal/idecli` | — | — | 2 | — | 2 |
| `internal/boardengine/boardtest` | — | — | — | 2 | 2 |
| `cmd/lyx` | — | 1 | — | 1 | 2 |
| `internal/webstercli` | — | — | 1 | — | 1 |
| `internal/loomengine` | 1 | — | — | — | 1 |
| `internal/treadleengine` | 1 | — | — | — | 1 |
| **total** | **57** | **34** | **23** | **51** | **163** |

154 of these are above fabric and migrate;
`lyxcwd`'s 9 stay.
The count is high because it is one fixture per test function for isolation — 279 test functions and 116 local setup helpers live in these files.

### Assertion migration is the real work

A real hub is ~155 files against the templates' ~36, and carries `_board`, `_portals`, `_launchers`, junctions, an anchor marker, hub-level `.lyx` and repo-wide `fabric.yaml` that today's fixtures lack.
Directory listings, file counts and "this path should not exist" assertions will break.
**Every break is worth reading rather than silencing** — it marks a place currently asserting against an invented shape.

Two specific shapes to expect:

- The 46 `CopyWeft` sites in `fabricengine` pair an unrelated weft with a `newWarpFixture` warp via `NewPairedForTest`, a shim in `internal/fabricengine/export_test.go` with 22 call sites across 4 files (`warpforward_integration_test.go` 10, `weftgit_exclude_test.go` 4, `fabric_test.go` 4, `checkout_index_refresh_test.go` 2).
  A real hub's `PrimeWorktree`/`PrimeWeft` is a genuine pair, so the shim is deleted.
- Sites that seed config into a fixture's parent directory as a stand-in hub (`weftgit_exclude_test.go`, `loomengine/preflight_integration_test.go`) drop that scaffolding entirely.

### Files naming the old packages by path

These reference `internal/lyxtest` or `internal/fabricengine/fabrictest` as strings and must be updated:

- `internal/lyxcwd/enforcement_test.go:604-619` — two allowlist maps keyed on `internal/fabricengine/fabrictest`.
- `cmd/lyx/destructiveguard_test.go:124` — subtree exclusion for `internal/fabricengine/fabrictest`.
  Its walk already skips `*_test.go` (`destructiveguard_test.go:231`), so once the live-state builders are `package fabricengine_test` files the exclusion is dead and is deleted.
- `cmd/lyx/tierpurity_test.go:50,57` and `cmd/lyx/hermeticenv_test.go:51` — banned-token string data (`"lyxtest.Copy"`).
- `internal/fabriccli/clone.go:26` and `internal/fabricengine/mutation.go:214` — comments naming `fabrictest`.

### Reusable pieces

- `internal/fslink` — `Remove` (link-only, idempotent), `RemoveLinksIn` (immediate children of one dir only), `IsLink`, `PointsTo`, `RawTarget`, `CreateDirLink`.
- `internal/fabricengine` — `WiredNames(baseDir)`, `RepoWiredNames(l)`, `HubReservedNames()`, `BoardDir(hub)`, `WorktreePath`, `WeftWorktreePath`, `PortalLink`, `LauncherDir`.
  A teardown walk needs more than `RemoveLinksIn` on one directory: junctions sit at `<hub>/_portals/<slug>`, `<hub>/_launchers/<slug>`, `<worktree>/_lyx`, `<worktree>/.lyx`, `<worktree>/_board`.
- `fabriccli.CloneAndWire(container, fabricengine.CloneOptions{WeftURL, WarpURL, Subpath})` returns `HubPath`, `Anchor`, `PrimeCwd`.
- `internal/fabricengine/export_test.go` — the `export_test.go` shim precedent.
- `internal/boardengine/boardtest` — precedent for a test-only sibling package.

## Constraints

`CONSTRAINTS.md` has **already been rewritten in this worktree** to describe the post-task state, so that reviewers do not flag `discussion.md` against a stale invariant.
It is therefore ahead of the code until this task lands — the tree still contains `internal/lyxtest` and `internal/fabricengine/fabrictest`, and the enforcement tests it names do not exist yet.
Making it true is part of this task's definition of done.

Changes already applied:

- **`lyxtest` Leaf Invariant → `gitkit` Leaf Invariant.**
  Same allowlist (stdlib + `lyxcwd`, `weftname`, `configengine`, `lyxdirs`);
  adds that `gitkit` owns `MustRun`/`SeedConfig`/`HermeticGitEnv`/`GitStatusPorcelain` and that its primitive repo fixtures serve `gitrepo` and `lyxcwd` only.
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

`hubforge` and `gitkit` are test infrastructure, so "tests" here means both tests *of* the new packages and the migration of the 154 tests that consume them.

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
- A guard test pinning the surviving primitive fixture's caller set to `internal/gitrepo` and `internal/lyxcwd`.
- Existing `lyxtest_test.go` (389 lines) and `reexecguard_test.go` coverage carries over for the retained helpers.

**Migration of the 154 sites:**

- Migrate package by package, smallest first, so the pattern is settled before `fabricengine`'s 95 sites.
  Suggested order: `webstercli`/`loomengine`/`treadleengine`/`configcli`/`idecli`/`boardtest`/`cmd/lyx`/`burlerengine` (13 sites) → `shuttlecli`/`perchcli` (12) → `reedcli` (21) → `fabriccli` (12) → `fabricengine` (95).
- Each broken assertion is read and re-expressed against the real shape, never silenced.
  A migration that deletes an assertion rather than re-pointing it needs an explicit note saying why.
- `internal/fabricengine`'s live-state machinery moves to `package fabricengine_test` in the same batch as its `fabrictest` deletion, so the tree never has both.
- After migration: `grep` proves zero remaining references to `lyxtest`, `fabrictest`, `CopyPaired`, `CopyPairedLocal`, `CopyWeft`, `NewPairedForTest` anywhere in `internal/` and `cmd/`.

**Regression gates:**

- Full Tier 2 run green (`go test -tags integration ./...`), and its wall-clock recorded against the ~132 s baseline.
  A delta materially worse than the predicted +3.4 s is a finding worth investigating, not a timing assertion to encode.
- `cmd/lyx/tierpurity_test.go` and `cmd/lyx/hermeticenv_test.go` pass with their updated token data.
- `cmd/lyx/destructiveguard_test.go` passes with the `fabrictest` exclusion removed.
- `internal/lyxcwd/enforcement_test.go` passes with its two allowlist maps updated.
- `go vet ./...` clean, and no import cycle anywhere — the compile is the primary proof of the `hubforge` invariant.

## Q&A log

- **Q:** Where do repo-wide hub fixtures live, given `fabrictest.NewHub` already exists and needs `fabriccli`? **A:** A dedicated factory module separate from both `lyxtest` and the fabric module — inverting `lyxtest` in place buys nothing because the stuck-file set is identical either way.
- **Q:** Is it really 163 `Copy*` calls, and why so many? **A:** Yes;
  one fixture per test function for isolation, across 279 test functions.
  Not duplication.
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
