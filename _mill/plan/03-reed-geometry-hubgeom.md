# Batch: reed-geometry-hubgeom

```yaml
task: "shuttleengine + reedengine + tokenvocab told-geometry"
batch: "reed-geometry-hubgeom"
number: 3
cards: 5
verify: go test ./internal/hubgeom/... ./internal/reedengine/... ./internal/reedcli/... ./internal/shuttlecli/... ./internal/burlercli/... ./internal/perchcli/... ./internal/webstercli/... ./cmd/lyx/... && go test -tags integration ./internal/reedengine/... && go vet -tags smoke ./internal/reedcli/... ./internal/shuttlecli/... ./internal/treadleengine/... ./internal/burlerengine/...
depends-on: [1, 2]
```

## Batch Scope

This batch introduces `reedengine.Geometry` (seven told fields), the new `internal/hubgeom` package that builds one from a `*lyxcwd.Location`, and converts `reedengine.Engine` to hold a `Geometry` instead of a `*lyxcwd.Location`.
It is one batch because `reedengine.New`'s signature change is not divisible: the no-additive-twins rule means every one of its nine out-of-package call sites plus every in-package test fixture changes in the same commit as the constructor.
It depends on batch 1 (`fabricengine.HubLogsDir`, which `hubgeom` calls to populate `Geometry.LogsDir`) and batch 2 (`tokenvocab.Ctx`'s two plain fields, which `header.go` fills from `e.geom`).

The external interfaces batch 4 consumes are `hubgeom.ReedGeometry(l *lyxcwd.Location) reedengine.Geometry` and the `Geometry` value each CLI construction site now holds, whose `AnchorPath` and `WorktreeRoot` fields feed `shuttleengine.NewRunner`'s two new string parameters.

Batch-local decisions beyond `## Shared Decisions`:

- `internal/reedengine`'s own tests build `Geometry` struct literals directly rather than calling `hubgeom.ReedGeometry`. `hubgeom` imports `reedengine`, so an in-package reed test importing `hubgeom` would close an import cycle.
  The one-teller rule binds every hub-mode call site outside `internal/reedengine`; in-package reed tests are its only exemption.
- Reed test fixtures give each `Geometry` field a distinct value rather than the same temp dir three times, so a field mix-up inside the engine surfaces instead of passing silently.

## Cards

### Card 6: Define `reedengine.Geometry`

- **Context:**
  - `internal/reedengine/lock.go`
  - `internal/reedengine/server.go`
  - `internal/reedengine/lifecycle.go`
  - `internal/reedengine/strand.go`
  - `internal/lyxcwd/lyxcwd.go`
- **Edits:** none
- **Creates:**
  - `internal/reedengine/geometry.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/reedengine/geometry.go` declaring an exported `Geometry` struct with exactly seven string fields, in this order: `SocketKey`, `SessionName`, `AnchorPath`, `WorktreeRoot`, `LogsDir`, `RepoName`, `HubPath`.
  Give each field a doc comment naming its single consumer inside the package: `SocketKey` is the tmux `-L` socket name and what `Engine.Socket` returns; `SessionName` is the tmux session name and what `Engine.SessionName` returns; `AnchorPath` is the base `stateDir` joins onto for `reed.json`/`reed.lock` and the cwd every pane is spawned with; `WorktreeRoot` is what `Strand.Worktree` is stamped with and what `resolveStrandName` substitutes for the `<WORKTREE>` token; `LogsDir` is the shared per-hub server's runtime log directory; `RepoName` and `HubPath` are the header pane's `repo` and `hub` tokens, passed through `internal/tokenvocab`.
  Give the type itself a doc comment stating that reed is *told* its geometry and derives none of it, that no field is validated by `New` or by any method, and that populating every field with a usable absolute path — or, for `SocketKey`, a socket-safe key — is the caller's obligation.
  That comment must name `hubgeom.ReedGeometry` as the hub-mode answer and `reedengine.ServerName(hubPath)` as the `SocketKey` derivation, without importing either.
  The file declares the type only; do not move `New` or any method into it, and do not add constructors, validators or defaults.
  Note that `SessionName` is both a field name on this struct and an existing package-level function in `server.go`; that is legal Go and both keep their present names.
- **Commit:** `feat(reedengine): add the told-geometry Geometry struct`

### Card 7: Add `internal/hubgeom`, the single hub-mode teller

- **Context:**
  - `internal/reedengine/geometry.go`
  - `internal/reedengine/server.go`
  - `internal/fabricengine/junctionnames.go`
  - `internal/lyxcwd/lyxcwd.go`
  - `internal/shedadapters/doc.go`
- **Edits:** none
- **Creates:**
  - `internal/hubgeom/hubgeom.go`
  - `internal/hubgeom/doc.go`
  - `internal/hubgeom/hubgeom_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create package `hubgeom` at `internal/hubgeom`, exporting exactly one function today: `ReedGeometry(l *lyxcwd.Location) reedengine.Geometry`.
  It populates the seven fields as `SocketKey: reedengine.ServerName(l.HubPath)`, `SessionName: reedengine.SessionName(l.WorktreePath())`, `AnchorPath: l.AnchorPath()`, `WorktreeRoot: l.WorktreePath()`, `LogsDir: fabricengine.HubLogsDir(l.HubPath)`, `RepoName: l.RepoName`, `HubPath: l.HubPath`.
  It performs no `os.Getwd`, no git discovery and no path resolution of its own — it reads accessors off a `Location` its caller already resolved, which is what keeps `internal/lyxcwd` the sole owner of cwd resolution under the Cwd Resolution Invariant.
  Write `internal/hubgeom/doc.go` as the package godoc: state that `hubgeom` is the hub-mode adapter that tells engines their geometry, that the engines it serves never import it so the told direction is preserved, that its whole contract today is `ReedGeometry`, and that later waves (T6 for burler and perch, T7 for webster) add their own `*Geometry` siblings here rather than spawning per-engine packages or re-deriving the construction inline.
  Also state that standalone CLIs do not call it, because they have no `Location`.
  Write `internal/hubgeom/hubgeom_test.go` as a table test over a subpath-anchored fixture where hub, worktree root and anchor path are three distinct directories and `RepoName` differs from every basename — a fixture where anchor equals worktree would pass while the two were swapped and is worthless here.
  Assert each of the seven fields lands on the right source, with `SocketKey` compared against `reedengine.ServerName(hub)` and `SessionName` against `reedengine.SessionName(worktreeRoot)` called for real rather than against hardcoded strings, so the test tracks those derivations instead of freezing them.
  Write this card's test before card 8 changes any signature — it is the only guard against the anchor/worktree swap, which is this refactor's one silent failure mode.
- **Commit:** `feat(hubgeom): add the hub-mode told-geometry teller`

### Card 8: Convert `reedengine.Engine` to hold a `Geometry`, and every call site with it

- **Context:**
  - `internal/reedengine/geometry.go`
  - `internal/reedengine/server.go`
  - `internal/hubgeom/hubgeom.go`
  - `internal/fabricengine/junctionnames.go`
  - `internal/lyxcwd/lyxcwd.go`
  - `internal/lyxdirs/dirs.go`
  - `internal/tokenvocab/tokenvocab.go`
  - `internal/hubforge/hub.go`
  - `internal/reedengine/state.go`
  - `internal/reedengine/name.go`
- **Edits:**
  - `internal/reedengine/lock.go`
  - `internal/reedengine/lifecycle.go`
  - `internal/reedengine/strand.go`
  - `internal/reedengine/header.go`
  - `internal/reedengine/doc.go`
  - `internal/reedengine/lock_test.go`
  - `internal/reedengine/header_test.go`
  - `internal/reedengine/contract_integration_test.go`
  - `internal/reedengine/mouse_boot_integration_test.go`
  - `internal/burlercli/cli.go`
  - `internal/perchcli/cli.go`
  - `internal/webstercli/cli.go`
  - `internal/shuttlecli/cli.go`
  - `internal/reedcli/cli.go`
  - `internal/shuttlecli/smoke_interrupt_test.go`
  - `internal/treadleengine/smoke_judge_test.go`
  - `internal/burlerengine/smoke_cluster_test.go`
  - `internal/burlerengine/smoke_round_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Replace `Engine`'s `layout *lyxcwd.Location` field in `internal/reedengine/lock.go` with `geom Geometry`, change the constructor to `New(cfg Config, geom Geometry) *Engine`, and build its `tmux` field as `NewTmuxCmd(cfg.Tmux, geom.SocketKey)` instead of `NewTmuxCmd(cfg.Tmux, socketName(layout.HubPath))`.
  `Engine.Socket` returns `e.geom.SocketKey` and `Engine.SessionName` returns `e.geom.SessionName`, both without re-deriving.
  Update `New`'s doc comment to state that the caller owns populating every `Geometry` field and that `New` validates none of them, naming `hubgeom.ReedGeometry` as the hub-mode answer.
  Update `lock.go`'s file header comment, which describes the Engine as holding "the worktree's lyxcwd.Location", and drop the `github.com/Knatte18/loomyard/internal/lyxcwd` import.
  In `internal/reedengine/lifecycle.go`, change `stateDir` to join onto `e.geom.AnchorPath`, replace the `logsDir := fabricengine.HubLogsDir(e.layout.HubPath)` assignment with `logsDir := e.geom.LogsDir`, replace both pane-spawn `-c` arguments with `e.geom.AnchorPath`, and drop the now-unused `github.com/Knatte18/loomyard/internal/fabricengine` import.
  Every `logger.Info` and `logger.Warn` call in that file survives unchanged in intent — same event, same level, same key/value fields.
  In `internal/reedengine/strand.go`, replace the `resolveStrandName` argument and the `Strand.Worktree` assignment, both currently `e.layout.WorktreePath()`, with `e.geom.WorktreeRoot`.
  In `internal/reedengine/header.go`, change the `tokenvocab.Ctx` construction to read `e.geom.RepoName` and `e.geom.HubPath`.
  In `internal/reedengine/doc.go`, add a short paragraph stating that reed is told its geometry as a `Geometry` value and derives none of it, that `internal/lyxcwd` and `internal/fabricengine` are consequently absent from the package's production imports, and that `hubgeom.ReedGeometry` is the hub-mode teller.
  In `internal/reedengine/lock_test.go`, convert `newTestEngine` to build a `Geometry` whose fields are distinct values derived from one `t.TempDir()` — a synthetic hub, a worktree root under it, and an anchor path under that — so a field mix-up inside the engine surfaces.
  Convert `TestWithOpLock_PathIsUnderDotLyx`'s own fixture the same way, keeping its property that the anchor path is a real subpath of the worktree root so `stateDir`'s anchoring stays observable, and rewrite its two `e.layout.…` reads as the corresponding `e.geom` fields.
  Rewrite `TestEngine_SocketAndSessionName` so it builds its own engine from locally-known `hub` and `worktreeRoot` values and asserts `Socket()` equals `ServerName(hub)` and `SessionName()` equals `filepath.Base(worktreeRoot)` — do not assert `Socket()` equals `e.geom.SocketKey`, which would be a tautology.
  In `internal/reedengine/header_test.go`, build the fixture engine from a `Geometry` carrying `RepoName` and `HubPath`, and change the two `e.layout.…` assertion expressions to `e.geom.RepoName` / `e.geom.HubPath`; update the file header comment, which names `*lyxcwd.Location` struct literals.
  In `internal/reedengine/contract_integration_test.go` and `internal/reedengine/mouse_boot_integration_test.go`, replace each `New(cfg, layout)` with a `Geometry`-literal construction preserving the same underlying directories, and change `New(cfg2, e1.layout)` to `New(cfg2, e1.geom)`.
  Build these `Geometry` literals directly rather than importing `hubgeom`; `hubgeom` imports `reedengine`, so an in-package reed test importing it would close an import cycle.
  Drop the `lyxcwd` import from every reed test file that no longer needs it.
  At the five CLI construction sites — `internal/burlercli/cli.go`, `internal/perchcli/cli.go`, `internal/webstercli/cli.go`, `internal/shuttlecli/cli.go`, `internal/reedcli/cli.go` — call `hubgeom.ReedGeometry(layout)` once into a local, and pass that local to `reedengine.New(reedCfg, ...)`.
  Bind the local even in `internal/reedcli/cli.go`, which constructs no runner, so all five sites read the same way.
  Leave every `LoadConfig(layout.AnchorPath(), …)` call, every `burlerengine.New(…, layout, …)` argument, and every `c.layout = layout` assignment exactly as they are — those belong to T2 and T6.
  Leave `shuttleengine.NewRunner`'s `layout` argument as it is at these sites; batch 4 changes it.
  In the four smoke tests — `internal/shuttlecli/smoke_interrupt_test.go`, `internal/treadleengine/smoke_judge_test.go`, `internal/burlerengine/smoke_cluster_test.go`, `internal/burlerengine/smoke_round_test.go` — replace the `reedengine.New` argument with `hubgeom.ReedGeometry(h.Location)` or `hubgeom.ReedGeometry(layout)` as the local fixture names it, and add the `hubgeom` import.
  Do not change `internal/reedengine/server.go`; `ServerName`, `SessionName` and `socketName` stay exactly as they are, and `socketName` stays unexported and in use by nothing but `ServerName`'s own alias contract.
- **Commit:** `refactor(reedengine): take a told Geometry instead of a lyxcwd.Location`

### Card 9: Reword the Treadle Runner-Seam Invariant

- **Context:**
  - `internal/reedengine/lifecycle.go`
  - `internal/fabricengine/junctionnames.go`
  - `internal/treadleengine/seam_enforcement_test.go`
- **Edits:**
  - `CONSTRAINTS.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Under the `## Treadle Runner-Seam Invariant` heading, the import-allowlist bullet's final clause asserts that `internal/treadleengine` to `internal/shuttleengine` to `internal/reedengine` to `internal/fabricengine` "is a real transitive path (`reedengine.HubLogsDir` calls `fabricengine.HubScratchDir`)".
  Reword that clause: the path no longer exists, because `HubLogsDir` moved to `internal/fabricengine` and reed is now told its logs directory as `Geometry.LogsDir`.
  Keep the surrounding sentence's actual point intact — that what the `internal/lyxcwd` exclusion buys is treadle being told its geometry rather than a transitive exclusion of `internal/fabricengine`.
  Do not change the allowlist itself, and do not change the **Enforced by** bullet.
  Follow the repo's semantic-line-break rule.
  Add no new named invariant for told geometry and no new allowlist-enforcement test for `reedengine` or `shuttleengine` — T10 owns naming the cross-cutting rule.
- **Commit:** `docs(constraints): reword the Treadle Runner-Seam transitive-path clause`

### Card 10: Document `hubgeom` in the package tree and point T6/T7 at it

- **Context:**
  - `internal/hubgeom/hubgeom.go`
  - `internal/hubgeom/doc.go`
  - `docs/shared-libs/README.md`
- **Edits:**
  - `docs/overview.md`
  - `manifest/designs/producers-standalone.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `docs/overview.md`, add one `internal/hubgeom/` line to the package tree, describing it as the hub-mode told-geometry teller that converts a resolved `lyxcwd.Location` into each engine's geometry struct.
  Place it near the `internal/lyxcwd/` entry so a reader finds the two together, and match the tree's existing alignment and box-drawing characters exactly, including whichever entry currently carries the closing `└──` connector.
  Do not add `internal/hubgeom` to the shared-infrastructure sentence further down that file, and do not add it to `docs/shared-libs/README.md`: that document's admission line is "one mechanical thing … no domain logic", every entry there sits below its consumers, and `hubgeom` fails both — it encodes reed-specific derivation and it imports `reedengine` and `fabricengine`.
  In `manifest/designs/producers-standalone.md`, add one line to the T6 entry and one line to the T7 entry, each naming `internal/hubgeom` and stating that it holds the hub-mode `Location`-to-geometry conversion, exports `ReedGeometry` today, and is where that task adds its own `BurlerGeometry`/`PerchGeometry`/`WebsterGeometry` rather than re-deriving the construction inline at each CLI site.
  Those two lines plus card 3's row correction are the complete permitted edit set for that file — do not rewrite T3's Files or Verify lines, and do not touch `manifest/roadmap.md`.
  Follow the repo's semantic-line-break rule in both files.
- **Commit:** `docs(overview): document internal/hubgeom and point T6/T7 at it`

## Batch Tests

`verify:` runs the untagged suites of `internal/hubgeom`, `internal/reedengine`, the five CLI packages and `cmd/lyx`; then the reed integration tier for real; then a `go vet -tags smoke` type-check of the four smoke packages this batch edits.

- `internal/hubgeom/hubgeom_test.go` — the new table test from card 7 is the load-bearing one.
  Its subpath-anchored fixture with three distinct directories and a distinct `RepoName` is the only assertion that catches an anchor/worktree swap, which compiles cleanly and fails silently everywhere else.
- `internal/reedengine/lock_test.go` — `stateDir` still resolves under the anchor path and not the worktree root, the op lock still serializes and still re-acquires cleanly, and `Socket()`/`SessionName()` still equal `ServerName(hub)` / `filepath.Base(worktreeRoot)`.
- `internal/reedengine/header_test.go` — `repo` and `hub` still render real values, now sourced from `Geometry` through the two-field `tokenvocab.Ctx` batch 2 introduced.
- `internal/reedengine/server_test.go` and `strand_test.go` are not edited by this batch and must stay green unchanged: `server.go` is untouched, so `ServerName` determinism and per-hub uniqueness still hold, and `resolveStrandName` still receives the worktree root it received before.
- `internal/reedengine/contract_integration_test.go` and `mouse_boot_integration_test.go` are `//go:build integration` over real tmux and are run for real by the second verify clause, per the discussion's own verify list.
- `cmd/lyx/constructoranchoring_test.go` is not edited by this batch but is run: it is the named enforcer of the Durable-vs-Ephemeral State Invariant, and every path this batch re-plumbs must still anchor exactly where it anchored before.
- The four smoke packages are compile-checked rather than run, per the shared decision on tagged tiers; the only breakage card 8 can introduce there is a `reedengine.New` call-site mismatch, which `go vet -tags smoke` catches.
