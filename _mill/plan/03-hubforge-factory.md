# Batch: hubforge factory

```yaml
task: 'lyxtest: build real fabric hubs, invert the lyxtest/fabric dependency'
batch: 'hubforge factory'
number: 3
cards: 8
verify: go vet -tags integration ./... && go test -tags integration ./internal/hubforge/... ./internal/fabricengine/... ./cmd/lyx/...
depends-on: [2]
```

## Batch Scope

This batch turns the relocated hub factory into the repo-wide fixture API the remaining eight batches consume: the anchored `WeftBase` field, junction-safe teardown, the two seeding entry points, and the tests that make each of those claims falsifiable — including the two that license the migration's biggest simplification (a real hub arrives with materialized config, so most `SeedConfig` sites can simply drop the call).
It also re-establishes the fixture benchmarks on the real hub and adds `hubforge.NewHub` to `cmd/lyx`'s two guard-token lists.

The external interface batches 4 through 10 consume is: `hubforge.NewHub(tb, anchor)`, `hubforge.AddPair`, `hubforge.SeedConfig(tb, h, map[string]string)`, `hubforge.SeedFabricConfig(tb, h, yaml)`, and the `Hub` fields/accessors `Path`, `Anchor`, `Location`, `Topology`, `WarpBare`, `WeftBare`, `WeftBase`, `Container`, `PrimeWorktree()`, `PrimeWeft()`, `BoardDir()`, `PairWarpWorktree(slug)`, `PairWeftSibling(slug)`, `PairPortalLink(slug)`, `PairLauncherDir(slug)`.

Batch-local decision: teardown is a method on nothing — it is registered by `NewHub` itself via `tb.Cleanup`, never called by a test.
No call site ever opts in or out, which is what makes the Win11 junction-removal guarantee structural rather than a discipline every one of 132 sites has to remember.

## Cards

### Card 13: Carry WeftBase on the Hub

- **Context:**
  - `internal/fabricengine/clone.go`
  - `internal/fabricengine/fabric.go`
  - `internal/fabriccli/clone.go`
- **Edits:**
  - `internal/hubforge/hub.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add a `WeftBase string` field to `Hub` and populate it in `NewHub` verbatim from `res.WeftBase`, never re-derived.
  Document on the field that `WeftBase` is the **anchor-joined** weft directory — `fabricengine`'s `CloneHub` computes it as `filepath.Join(WeftWorktree(l), l.AnchorRel)` — and that it is deliberately not the same thing as `PrimeWeft()`, which returns the un-anchored weft worktree root.
  State the consequence explicitly: the two coincide at the `"."` anchor and diverge at `"backend"`, where writing config to the un-anchored path produces a file no module loader ever reads, with no error at all.
  Add the same warning to `PrimeWeft`'s own doc comment so a caller reaching for it to seed config is told where to go instead.
- **Commit:** `feat(hubforge): carry the anchor-joined WeftBase on Hub`

### Card 14: Junction-safe teardown

- **Context:**
  - `internal/fslink/fslink.go`
  - `internal/fslink/fslink_linux.go`
  - `internal/fslink/fslink_windows.go`
  - `internal/fabricengine/junctionnames.go`
- **Edits:**
  - `internal/hubforge/hub.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add an unexported `registerTeardown(tb testing.TB, hubPath string)` called by `NewHub` **after** both `copyBares` and the `container := tb.TempDir()` call, so LIFO cleanup ordering runs it before Go removes the temp dir.
  It registers a `tb.Cleanup` that does one `filepath.WalkDir` from the hub root, calls `fslink.IsLink` on every entry, and calls `fslink.Remove` on each link found.
  The callback returns `nil` after removing a link and **must not** return `filepath.SkipDir`: `WalkDir` reports a link as a non-directory entry and never follows it, so non-descent is already free, whereas `SkipDir` from a non-directory callback skips the containing directory's remaining entries — which would leave every sibling junction wired, abandoning `<hub>/_portals/<slug2>` onward after removing `<slug1>`, and leaving `_lyx` and `_board` behind after removing `<worktree>/.lyx`.
  Put that reasoning in the code comment;
  it is the single most reversible mistake in this file.
  Discovery is slug-free by design — the walk never consults a slug list, because for the live-state matrix the pairs are created by the verb under test and some are destroyed by it, and because enumerating worktrees through fabric requires fabric to still work against a hub those tests deliberately corrupt.
  Errors from `WalkDir`, `IsLink` and `Remove` are reported with `tb.Logf` and never `tb.Fatalf` or `tb.Errorf`: teardown must not fail a test that already passed.
  A missing hub directory (a worktree removed by hand mid-test) simply yields no entries, and `fslink.Remove` is documented idempotent, so neither case needs a special branch.
- **Commit:** `feat(hubforge): remove junctions via fslink before TempDir cleanup`

### Card 15: SeedConfig on the weft side

- **Context:**
  - `internal/configengine/config.go`
  - `internal/gitkit/gitkit.go`
  - `internal/hubforge/hub.go`
- **Edits:** none
- **Creates:**
  - `internal/hubforge/seed.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add `func SeedConfig(tb testing.TB, h *Hub, configByModule map[string]string)` in a new untagged `internal/hubforge/seed.go`.
  It creates `configengine.ConfigDir(h.WeftBase)`, writes each module's YAML to `configengine.ConfigFile(h.WeftBase, module)`, then stages and commits in the weft **worktree root** `h.PrimeWeft()` — the commit must run at the worktree root, not at `h.WeftBase`, because at a non-`"."` anchor `h.WeftBase` is a subdirectory of it — using `gitkit.MustRun(tb, h.PrimeWeft(), "git", "add", ".")` and `gitkit.MustRun(tb, h.PrimeWeft(), "git", "commit", "-m", "hubforge: seed config")`.
  Document why the base is `h.WeftBase` and not `h.PrimeWeft()`, and why seeding the warp side is impossible: `<worktree>/_lyx` is a weft junction excluded from the warp's index via `.git/info/exclude`, so `git add .` there stages nothing and the commit errors.
  Do not guess the base from the path shape and do not accept a caller-supplied directory — a single seeding entry point that infers its base is exactly what makes the three ad-hoc call sites silently wrong.
- **Commit:** `feat(hubforge): add weft-side SeedConfig`

### Card 16: SeedFabricConfig on the board

- **Context:**
  - `internal/configengine/config.go`
  - `internal/configsync/configsync.go`
  - `internal/fabricengine/bolt.go`
  - `internal/fabriccli/clone.go`
  - `internal/hubforge/hub.go`
- **Edits:**
  - `internal/hubforge/seed.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add `func SeedFabricConfig(tb testing.TB, h *Hub, yaml string)` writing to `configengine.ConfigFile(h.BoardDir(), "fabric")` — the repo-wide fabric config base, matching `configsync.ReconcileFabricAt`'s own `boardDir` argument — after creating `configengine.ConfigDir(h.BoardDir())`.
  It **commits**, via `fabricengine.NewBolt(h.BoardDir()).Commit("hubforge: seed repo-wide fabric config", fabricengine.SyncOptions{})`, calling `tb.Fatalf` on error.
  Document why leaving it uncommitted is unsafe rather than merely untidy: `BoardDir` is the `weft:main` checkout the destruction gate's dirtiness check observes, so an uncommitted seed would silently change verb outcomes in fabric's own live-state cells.
  This mirrors what `fabriccli.CloneAndWire` itself does after `ReconcileFabricAt` — the same `NewBolt(res.BoardDir).Commit(...)` call — so the fixture leaves the board in the same state a real clone does.
- **Commit:** `feat(hubforge): add board-side SeedFabricConfig`

### Card 17: Prove NewHub builds a real hub, at both anchors

- **Context:**
  - `internal/fabricengine/junctionnames.go`
  - `internal/fabricengine/junction.go`
  - `internal/fabricengine/weftwiring.go`
  - `internal/fabricengine/portals.go`
  - `internal/fabricengine/launchers.go`
  - `internal/fabricengine/clone.go`
  - `internal/fslink/fslink.go`
  - `internal/configengine/config.go`
  - `internal/configreg/configreg.go`
  - `internal/hubforge/hub.go`
- **Edits:**
  - `internal/hubforge/hub_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add `TestNewHub_IsARealHub` asserting, for a hub built at the `"."` anchor: `_board` exists as a real directory, the `.lyx-anchor` marker exists, the hub-level `.lyx` exists and is **not** a link (assert via `fslink.IsLink` returning false), the wired junctions exist and are links, the repo-wide `fabric.yaml` is present under `configengine.ConfigDir(h.BoardDir())`, and `weft:main` carries the warp-URL binding.
  Every path must come from `fabricengine`'s own name accessors — `BoardDir`, `HubReservedNames`, `WiredNames`, `RepoWiredNames`, `WorktreePath`, `WeftWorktreePath`, `PortalLink`, `LauncherDir` — never from a hardcoded string, because a hardcoded string is precisely the invented shape this whole task removes.
  Add `TestNewHub_BackendAnchor` running the same assertions at the `"backend"` anchor, and additionally asserting `h.WeftBase != h.PrimeWeft()` there while `h.WeftBase == h.PrimeWeft()` at `"."`.
  Add `TestNewHub_ConfigMaterializedWithoutSeeding` asserting that a freshly built hub already carries a materialized config file for at least one registered module without any seeding call — read it back through the warp-side anchored path `configengine.ConfigFile(h.Location.AnchorPath(), <module>)` and require non-empty content matching the module's registered template.
  That test is what licenses batches 4 through 10 to delete a `SeedConfig` call rather than retarget it, so it is not optional colour.
- **Commit:** `test(hubforge): assert NewHub produces a real hub at both anchors`

### Card 18: Prove seeding, concurrency and teardown

- **Context:**
  - `internal/configengine/config.go`
  - `internal/fslink/fslink.go`
  - `internal/hubforge/hub.go`
  - `internal/hubforge/seed.go`
- **Edits:**
  - `internal/hubforge/hub_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add `TestSeedConfig_VisibleFromWarpSide` as a table over both anchors, `"."` and `"backend"`: seed an overriding value with `SeedConfig`, then read it back through the **warp-side** `_lyx` junction at `configengine.ConfigFile(h.Location.AnchorPath(), <module>)` and assert the override is what comes back.
  Running it at `"backend"` is the whole point — a `"."`-only test passes even when the base is wrong, because there the anchored and un-anchored weft paths coincide.
  Add `TestSeedFabricConfig_CommitsAndLeavesBoardClean` asserting the seeded value is present at `configengine.ConfigFile(h.BoardDir(), "fabric")` and that `gitkit.GitStatusPorcelain(t, h.BoardDir())` returns empty afterwards.
  Add `TestNewHub_Concurrent` launching N concurrent `NewHub` calls (N = 8) from one test, asserting every returned hub is independently correct on the `TestNewHub_IsARealHub` assertions and that no two share a `Path`, `Container`, `WarpBare` or `WeftBare`.
  Add `TestNewHub_TeardownRemovesJunctionsKeepsTargets`: build a hub in a nested `t.Run` subtest, capture each junction path and its `fslink.PointsTo` target before the subtest returns, then after it returns assert every junction path is gone and — the load-bearing half — every captured target directory still exists with its content intact.
  Add `TestNewHub_TeardownSurvivesCorruptHub` covering the two hostile shapes the live-state matrix plants: a junction repointed at an unrelated directory, and a warp worktree directory removed by hand.
  Teardown must complete without failing the test in both.
- **Commit:** `test(hubforge): cover seeding, concurrency and junction-safe teardown`

### Card 19: Re-establish the fixture benchmarks on the real hub

- **Context:**
  - `internal/hubforge/hub.go`
  - `internal/gitkit/bench_test.go`
- **Edits:** none
- **Creates:**
  - `internal/hubforge/bench_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add `//go:build integration`-tagged `BenchmarkNewHub` and `BenchmarkNewHubParallel` measuring the full fixture — bare copy plus clone plus wire — and `BenchmarkCopyBares` measuring the bare-copy step alone, so the clone-versus-copy comparison the whole design rests on stays reproducible from inside the repo.
  Carry over the file-header note that `b.TempDir()` cleanup accumulates to the end of the benchmark, and give the run line as `go test -tags integration -bench 'BenchmarkNewHub|BenchmarkCopyBares' -run '^$' ./internal/hubforge`.
  `BenchmarkCopyBares` needs `copyBares`, which is unexported and in the same package — no shim is required.
- **Commit:** `test(hubforge): add the real-hub fixture benchmarks`

### Card 20: Teach cmd/lyx's guards about hubforge.NewHub

- **Context:**
  - `CONSTRAINTS.md`
  - `internal/hubforge/hub.go`
- **Edits:**
  - `cmd/lyx/tierpurity_test.go`
  - `cmd/lyx/hermeticenv_test.go`
  - `internal/gitkit/callerset_enforcement_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add the token `"hubforge.NewHub"` to `cmd/lyx/tierpurity_test.go`'s banned-token list alongside the existing `"gitkit.Copy"`, so an untagged test calling the real-hub fixture is a build-gate failure, and update the file-header comment to name both tokens.
  Add the same token to `cmd/lyx/hermeticenv_test.go`'s git-spawning token list alongside `"gitkit.Copy"`, `"gitkit.MustRun"` and `"gitkit.SeedConfig"`, and update that file's header comment to say the fixture helpers now live in `gitkit` and `hubforge`.
  Do not add `hubforge.SeedConfig`/`hubforge.SeedFabricConfig` as separate tokens: both take a `*Hub` that only `NewHub` can produce, so the `hubforge.NewHub` token already covers every package that can reach them.
  Run both guards and confirm no package trips them — `internal/hubforge`'s own `hub_test.go` and `bench_test.go` are integration-tagged and its `testmain_test.go` calls `gitkit.HermeticGitEnv()`, so it satisfies both.
  The new `"hubforge.NewHub"` token, run against the untagged suite, also trips `internal/gitkit/callerset_enforcement_test.go`: batch 2 wrote that file's doc comment and its failure message both naming `hubforge.NewHub` in prose (never as a real call), and the guard matches raw substrings, comments included.
  Reword those two prose mentions (the file-header comment and `TestCopyRepoCallerSet_LyxcwdOnly`'s failure-message string) to break the literal substring — e.g. "hubforge's real-hub factory" — without losing the point being made, rather than adding an `allowedSpawners` entry: the mention is incidental prose, not scan data the guard needs to see, so rewording is more honest than allowlisting.
- **Commit:** `test(lyx): add hubforge.NewHub to the tier-purity and hermetic guards`

## Batch Tests

`verify:` compile-checks the repo under `-tags integration`, then runs `internal/hubforge`'s own suite (`hub_test.go`'s eight tests plus the moved `buildBareTemplate` coverage) and `cmd/lyx`'s guard suite, which card 20 changes.
The `smoke` tag is not compile-checked here because no file this batch touches is smoke-tagged.

`internal/fabricengine`'s integration suite is re-run despite batch 2 having already proved it green, and the reason is card 14 alone: `registerTeardown` is additive as an API but it now runs for every existing `NewHub` caller, and the live-state matrix is the one suite that plants deliberately-corrupt hubs for it to walk.
Everything else in this batch (`WeftBase`, `SeedConfig`, `SeedFabricConfig`, the benchmarks) is purely additive and would not on its own justify the cost.
