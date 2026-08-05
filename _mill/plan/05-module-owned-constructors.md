# Batch: module-owned-constructors

```yaml
task: 'fabric: shrink hubgeometry to the minimal illusion primitive (slice 7)'
batch: module-owned-constructors
number: 5
cards: 10
verify: go vet -tags "integration smoke scout" ./... && go test ./internal/lyxcwd/... ./internal/loomengine/... ./internal/planparser/... ./internal/builderengine/... ./internal/buildercli/... ./internal/websterengine/... ./internal/webstercli/... ./internal/perchengine/... ./internal/perchcli/... ./internal/scoutengine/... ./internal/pattern/... ./internal/logger/... ./internal/reedengine/... ./internal/reedcli/... ./internal/burlerengine/... ./internal/shuttleengine/... ./cmd/lyx/... && go test -tags integration ./internal/lyxcwd/... ./internal/loomengine/... ./internal/planparser/... ./internal/builderengine/... ./internal/buildercli/... ./internal/websterengine/... ./internal/webstercli/... ./internal/perchengine/... ./internal/perchcli/... ./internal/scoutengine/... ./internal/pattern/... ./internal/logger/... ./internal/reedengine/... ./internal/reedcli/... ./internal/burlerengine/... ./internal/shuttleengine/... ./cmd/lyx/...
depends-on: [4]
```

## Rename mechanic

For each `Moves:` pair the implementer MUST:

1. Run `git mv <old> <new>` FIRST, before making any other change to the moved file.
2. Make ONLY surgical edits — touch only the lines that must change after the move (package declaration, import lines, identifier retargeting, seam splits).
3. Use a full-file `Creates:` entry only for genuinely new files that have no predecessor.
4. Never write the relocated file from scratch and delete the original — that breaks git rename history and inflates review diffs.

## Batch Scope

This batch delivers the bottleneck fix GitHub issue #127 named: each of the ~20 per-module path constructors moves out of the shrunk module into the module that owns the directory, as a local constructor over a private relative-path constant. After it, adding a module subdirectory under `_lyx` is a change to that module, never to a shared package. Every card here is pure relocation — batches 1-4 already rewrote every signature and every field read, so no call site changes shape, only its package qualifier.

The batch is one unit because the guard invariant it must not break is global: `configengine.LyxDirName` is the single declarer of `_lyx`, and each relocated constructor joins **per segment** (`filepath.Join(l.AnchorPath(), configengine.LyxDirName, planDirName)`), never a fused `"_lyx/plan"` literal. A fused literal is invisible to the whole-token guard and would make the `_lyx` ownership row police a token nobody declares. Card 29 is the zero-diff gate that proves no card in this batch introduced one.

Batch-local decision — `PlanDirRel` goes to `internal/planparser`, not `internal/loomengine`. The discussion's `per-module-constructors` decision lists it with the loom group, but its sole production consumer is `planparser/parse.go:236`, which stamps the token into `Card.SourcePath`; sending it to `loomengine` would make a pure parser library import a feature engine. `planparser` declares `PlanDirName = "plan"` and `PlanDirRel()`, and `loomengine.PlanDir` joins onto `planparser.PlanDirName` so the segment still has one declarer. `loomengine` → `planparser` is a new edge and is acyclic: `planparser` imports only `configengine` after this card.

Batch-local decision — the relocated constructors keep the base each has today, per the anchoring table in the overview's Shared Decisions. There is no single base, and a blanket "join onto `AnchorPath()`" would silently relocate four of them: `HubLogsDir` is hub-anchored so one reed server per hub resolves to one place, and `WorktreeLogsDir`/`ScoutDaemonStateFile`/`ScoutDaemonLock` live under the ephemeral `.lyx`, never the git-tracked `_lyx`, because they are PIDs, sockets and rotating logs.

External interface batch 6 consumes: nothing new — batch 6 depends on this batch only so the two do not edit `internal/lyxcwd/lyxcwd.go` concurrently.

## Cards

### Card 20: loom's plan, discussion and status paths

- **Context:**
  - `internal/configengine/config.go`
  - `internal/loomengine/config.go`
- **Edits:**
  - `internal/buildercli/cli.go`
  - `internal/buildercli/poll_test.go`
  - `internal/buildercli/run_test.go`
  - `internal/buildercli/smoke_test.go`
  - `internal/buildercli/spawnbatch_test.go`
  - `internal/buildercli/testdata_test.go`
  - `internal/loomengine/discussion.go`
  - `internal/loomengine/plan.go`
  - `internal/loomengine/plan_test.go`
  - `internal/loomengine/preflight.go`
  - `internal/loomengine/preflight_integration_test.go`
  - `internal/lyxcwd/lyxcwd.go`
  - `internal/planparser/parse.go`
  - `internal/webstercli/cli.go`
  - `internal/webstercli/cli_test.go`
  - `internal/webstercli/verbs_test.go`
  - `internal/websterengine/template_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:**
  - `internal/lyxcwd/planpath_test.go` -> `internal/loomengine/planpath_test.go`
  - `internal/lyxcwd/discussionpath_test.go` -> `internal/loomengine/discussionpath_test.go`
  - `internal/lyxcwd/loomstatus_test.go` -> `internal/loomengine/loomstatus_test.go`
- **Requirements:** Delete `PlanDir`, `PlanDirRel`, `(*Location).PlanDir`, `(*Location).PlanOverview`, `(*Location).DiscussionDir`, `(*Location).DiscussionDecisionRecord`, `(*Location).DiscussionSupportLog`, `(*Location).LoomStatusFile` and `(*Location).LoomStatusLock` from `internal/lyxcwd/lyxcwd.go`. Declare in `internal/planparser/parse.go`: `const PlanDirName = "plan"` and `func PlanDirRel() string` returning `path.Join(configengine.LyxDirName, PlanDirName)` using the stdlib `path` package so the token stays forward-slash, never OS-dependent; `parse.go:236` calls its own `PlanDirRel()`. Declare in `internal/loomengine`: `func PlanDir(l *lyxcwd.Location) string` = `filepath.Join(l.AnchorPath(), configengine.LyxDirName, planparser.PlanDirName)`, `func PlanOverview(l *lyxcwd.Location) string` = `filepath.Join(PlanDir(l), "00-overview.md")`, `func DiscussionDir(l *lyxcwd.Location) string` = `filepath.Join(l.AnchorPath(), configengine.LyxDirName, discussionDirName)` with `discussionDirName = "discussion"`, `DiscussionDecisionRecord` and `DiscussionSupportLog` joining `"decision-record.md"`/`"support-log.md"` onto it, and `LoomStatusFile`/`LoomStatusLock` joining `"status.json"`/`"status.json.lock"` onto `filepath.Join(l.AnchorPath(), configengine.LyxDirName)`. Retarget every listed caller. `buildercli/cli.go` and `webstercli/cli.go` already import `loomengine`; verify rather than assume.
- **Commit:** `refactor(loomengine): own the plan, discussion and loom-status paths`

### Card 21: builder's run-state paths

- **Context:**
  - `internal/configengine/config.go`
  - `internal/builderengine/config.go`
- **Edits:**
  - `internal/buildercli/cli.go`
  - `internal/buildercli/pause_test.go`
  - `internal/buildercli/poll_test.go`
  - `internal/buildercli/run_test.go`
  - `internal/buildercli/smoke_test.go`
  - `internal/buildercli/spawnbatch_test.go`
  - `internal/buildercli/status_test.go`
  - `internal/lyxcwd/lyxcwd.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Delete `BuilderDir` and `BuilderReportsDir` from `internal/lyxcwd/lyxcwd.go`. Declare in `internal/builderengine`: `const builderDirName = "builder"`, `func Dir(l *lyxcwd.Location) string` = `filepath.Join(l.AnchorPath(), configengine.LyxDirName, builderDirName)` and `func ReportsDir(l *lyxcwd.Location) string` = `filepath.Join(Dir(l), "reports")`. Retarget `buildercli`'s production and test callers. Name them `Dir`/`ReportsDir` rather than `BuilderDir`/`BuilderReportsDir`: inside `builderengine` the prefix is redundant, and callers read `builderengine.Dir(l)`.
- **Commit:** `refactor(builderengine): own the builder run-state and reports paths`

### Card 22: webster's run-state, reports and prompts paths

- **Context:**
  - `internal/configengine/config.go`
  - `internal/websterengine/config.go`
- **Edits:**
  - `internal/lyxcwd/lyxcwd.go`
  - `internal/webstercli/cli.go`
  - `internal/webstercli/cli_test.go`
  - `internal/webstercli/verbs_test.go`
  - `internal/websterengine/report.go`
- **Creates:** none
- **Deletes:** none
- **Moves:**
  - `internal/lyxcwd/webstergeom_test.go` -> `internal/websterengine/webstergeom_test.go`
- **Requirements:** Delete `WebsterDir`, `WebsterReportsDir` and `WebsterPromptsDir` from `internal/lyxcwd/lyxcwd.go`. Declare in `internal/websterengine`: `const websterDirName = "webster"`, `func Dir(l *lyxcwd.Location) string` = `filepath.Join(l.AnchorPath(), configengine.LyxDirName, websterDirName)`, `func ReportsDir(l *lyxcwd.Location) string` = `filepath.Join(Dir(l), "reports")` and `func PromptsDir(l *lyxcwd.Location) string` = `filepath.Join(Dir(l), "prompts")`. Prompts stay under `webster/` and stay machine-local, re-renderable artifacts excluded from weft commits — the move must not change that. Retarget `webstercli`'s callers and `websterengine/report.go`'s two godoc references at lines 3 and 82.
- **Commit:** `refactor(websterengine): own the webster run-state, reports and prompts paths`

### Card 23: perch's run-artifact path

- **Context:**
  - `internal/configengine/config.go`
  - `internal/perchengine/engine.go`
- **Edits:**
  - `internal/lyxcwd/lyxcwd.go`
  - `internal/perchcli/cli.go`
  - `internal/perchcli/cli_integration_test.go`
  - `internal/perchcli/run_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Delete `PerchRunsDir` from `internal/lyxcwd/lyxcwd.go`. Declare `const perchDirName = "perch"` and `func RunsDir(l *lyxcwd.Location) string` = `filepath.Join(l.AnchorPath(), configengine.LyxDirName, perchDirName)` in `internal/perchengine`. Retarget `perchcli/cli.go` and the two integration tests. `perchcli` already imports `perchengine`.
- **Commit:** `refactor(perchengine): own the perch run-artifact path`

### Card 24: scout's daemon state and lock paths

- **Context:**
  - `internal/lyxcwd/lyxcwd.go`
  - `internal/scoutengine/daemonstate.go`
- **Edits:**
  - `internal/lyxcwd/lyxcwd.go`
  - `internal/scoutengine/daemonstate.go`
  - `internal/scoutengine/ensureserver.go`
  - `internal/scoutengine/ensureserver_integration_test.go`
  - `internal/scoutengine/ensureserver_test.go`
  - `internal/scoutengine/refs_integration_test.go`
  - `internal/scoutengine/supervised_integration_test.go`
  - `internal/scoutengine/supervised_scout_test.go`
  - `internal/scoutengine/supervised_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:**
  - `internal/lyxcwd/scoutdaemon_test.go` -> `internal/scoutengine/scoutdaemon_test.go`
- **Requirements:** Delete `(*Location).ScoutDaemonStateFile` and `(*Location).ScoutDaemonLock` from `internal/lyxcwd/lyxcwd.go`, along with the `dotLyxDirName` const if no other `lyxcwd` symbol still uses it. Declare in `internal/scoutengine`: `const dotLyxDirName = ".lyx"` and `scoutDirName = "scout"`, then `func DaemonStateFile(worktreePath, lang string) string` = `filepath.Join(worktreePath, dotLyxDirName, scoutDirName, lang, "daemon.json")` and `func DaemonLock(worktreePath, lang string) string` for `daemon.lock`. Take a plain worktree path, not a `*Location`: these are the only two constructors whose caller (`ensureserver.go:300`) built a synthetic `Location` purely to reach them, and a plain-path signature deletes that literal outright rather than re-expressing it. Retarget every listed caller accordingly. `.lyx` stays unpoliced by the geometry guard this slice; slice 9 is where it gets an owner.
- **Commit:** `refactor(scoutengine): own the scout daemon state and lock paths`

### Card 25: pattern's own directory and file paths

- **Context:**
  - `internal/pattern/leaf_enforcement_test.go`
  - `internal/pattern/pattern_test.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/lyxcwd/enforcement_test.go`
  - `internal/lyxcwd/lyxcwd.go`
  - `internal/pattern/doc.go`
  - `internal/pattern/pattern.go`
- **Creates:** none
- **Deletes:** none
- **Moves:**
  - `internal/lyxcwd/pattern_test.go` -> `internal/pattern/patternpath_test.go`
- **Requirements:** Delete `PatternDir`, `PatternFile`, `(*Location).PatternFileHere` and the `PatternDirName` const from `internal/lyxcwd/lyxcwd.go`. Declare in `internal/pattern`: `const DirName = "_pattern"`, `func Dir(baseDir string) string` = `filepath.Join(baseDir, DirName)`, `func File(baseDir string) string` = `filepath.Join(Dir(baseDir), "PATTERN.md")` and `func FileHere(l *lyxcwd.Location) string` = `File(l.AnchorPath())`. `internal/pattern` becomes a declarer of `_pattern`, so **this card moves that token's ownership row in the same breath** — `const DirName = "_pattern"` is a production const declaration of a policed token, and this batch's `verify` runs `go test ./internal/lyxcwd/...`, which executes the guard. In `internal/lyxcwd/enforcement_test.go`, change `_pattern`'s owner from `internal/lyxcwd` to `internal/pattern`. `fabricengine` becomes the row's second owner in batch 6, for the bare name as a git pathspec (`pull.go:299`); it is not added here, because nothing in this card puts a `_pattern` literal in `fabricengine`. The Pattern Leaf Invariant needs no widening: `internal/pattern` imports stdlib plus `internal/lyxcwd` only — which is exactly what `leaf_enforcement_test.go:23-24` already allowlists — never `configengine`, never `fabricengine` and never `weftname`. None of the four declared bodies needs a config path. The moved test file drops the sub-tests covering `WeftPatternDir`/`WeftPatternDirFor`/`HostPatternLink`/`HostPatternLinkHere` (`pattern_test.go:88-118`) — those four accessors are deleted in batch 6 and the coverage they stood in for is re-asserted there against the generic junction path — and keeps everything else. Rename the file to `patternpath_test.go` so it does not collide with the existing `internal/pattern/pattern_test.go`.
- **Commit:** `refactor(pattern): own the _pattern directory and PATTERN.md paths`

### Card 26: logger's trace directory and reed's hub log directory

- **Context:**
  - `internal/lyxcwd/lyxcwd.go`
  - `internal/reedengine/config.go`
- **Edits:**
  - `internal/logger/logger.go`
  - `internal/logger/sink.go`
  - `internal/lyxcwd/lyxcwd.go`
  - `internal/reedengine/lifecycle.go`
- **Creates:** none
- **Deletes:** none
- **Moves:**
  - `internal/lyxcwd/worktreelogs_test.go` -> `internal/logger/worktreelogs_test.go`
- **Requirements:** Delete `(*Location).WorktreeLogsDir` and `(*Location).HubLogsDir` from `internal/lyxcwd/lyxcwd.go`. Declare `func WorktreeLogsDir(l *lyxcwd.Location) string` = `filepath.Join(l.WorktreePath(), dotLyxDirName, "logs")` in `internal/logger` with its own `dotLyxDirName = ".lyx"` const, and `func HubLogsDir(l *lyxcwd.Location) string` = `filepath.Join(l.HubPath, dotLyxDirName, "logs")` in `internal/reedengine` with its own. Both keep today's base exactly: the logger sink is a worktree-wide singleton, the reed server log is hub-anchored so one server per hub resolves to one deterministic place. Neither moves under `_lyx` — these are rotating logs and must never be git-tracked. Correct the two godoc references in `logger/logger.go` at lines 48 and 409 that name `hubgeometry.Layout.WorktreeLogsDir()`.
- **Commit:** `refactor(logger,reedengine): own the worktree and hub log directories`

### Card 27: dissolve `LyxDir` and `DotLyxDir` into their callers

- **Context:**
  - `internal/lyxcwd/lyxcwd.go`
  - `internal/configengine/config.go`
- **Edits:**
  - `internal/burlerengine/doc.go`
  - `internal/burlerengine/engine.go`
  - `internal/burlerengine/engine_test.go`
  - `internal/lyxcwd/lyxcwd.go`
  - `internal/reedcli/cli_integration_test.go`
  - `internal/reedcli/smoke_lifecycle_test.go`
  - `internal/reedengine/contract_integration_test.go`
  - `internal/reedengine/lifecycle.go`
  - `internal/reedengine/lock.go`
  - `internal/reedengine/lock_test.go`
  - `internal/reedengine/spawn.go`
  - `internal/reedengine/spawn_test.go`
  - `internal/reedengine/strand.go`
  - `internal/reedengine/strand_test.go`
  - `internal/shuttleengine/run.go`
  - `internal/shuttleengine/run_test.go`
  - `internal/shuttleengine/rundir.go`
  - `internal/shuttleengine/rundir_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Delete `(*Location).LyxDir` and `(*Location).DotLyxDir` from `internal/lyxcwd/lyxcwd.go` and the `dotLyxDirName` const with them, leaving the module with no directory-name constant at all beyond `HubSuffix` and `BoardDirName` (which batch 6 handles). Each caller joins the segment itself: `burlerengine`, `reedengine` and `shuttleengine` each declare their own private `dotLyxDirName = ".lyx"` and build `filepath.Join(l.WorktreePath(), dotLyxDirName, …)` at the sites that called `DotLyxDir()`. `reedcli`'s two tagged tests join the same way against the fixture's location. `.lyx` is deliberately duplicated across these packages: it stays unpoliced this slice, and slice 9 — which registers it as a pathspec junction and removes `crossModuleMachineLocalExcludes` — is where it gets a single owner. Adding it to the ownership map now would have to be undone one slice later. Do not change any resulting path: every one of these is byte-identical before and after for `AnchorRel == "."`, and the `.lyx` group is byte-identical for a subpath-anchored repo too, because it was already `WorktreeRoot`-anchored.
- **Commit:** `refactor: join .lyx in each owning module instead of via lyxcwd`

### Card 28: anchoring-table equivalence test

- **Context:**
  - `internal/loomengine/plan.go`
  - `internal/builderengine/spawn.go`
  - `internal/websterengine/report.go`
  - `internal/perchengine/engine.go`
  - `internal/scoutengine/ensureserver.go`
  - `internal/pattern/pattern.go`
  - `internal/logger/sink.go`
  - `internal/reedengine/lifecycle.go`
  - `internal/planparser/parse.go`
  - `internal/configengine/config.go`
- **Edits:** none
- **Creates:**
  - `cmd/lyx/constructoranchoring_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add an untagged table test asserting every relocated constructor resolves to the path the anchoring table says it should, run over **two** synthetic `lyxcwd.Location` values: one unanchored (`AnchorRel == "."`) and one subpath-anchored (`AnchorRel == "backend"`). The check is **anchor-aware, not byte-identical**: for the unanchored fixture every constructor in all three groups is byte-identical to what `internal/hubgeometry` produced before this task; for the subpath-anchored fixture the `_lyx`-durable group intentionally moves down by `AnchorRel` while the `.lyx` group and `HubLogsDir` stay byte-identical. Assert the intended move rather than assuming it. The test lives in `cmd/lyx` because it is the only package that may import every owning module at once; it is pure `filepath.Join` arithmetic and spawns nothing, so it stays untagged per the Test Tier Purity Invariant.
- **Commit:** `test(cmd/lyx): pin the relocated constructors to the anchoring table`

### Card 29: verify the per-segment `_lyx` join

- **Context:**
  - `internal/lyxcwd/enforcement_test.go`
  - `internal/configengine/config.go`
  - `internal/loomengine/plan.go`
  - `internal/builderengine/spawn.go`
  - `internal/websterengine/report.go`
  - `internal/perchengine/engine.go`
  - `internal/planparser/parse.go`
- **Edits:** none
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Zero-diff gate. Confirm by grep that no card in this batch introduced a fused geometry literal — `grep -rn '"_lyx/' --include='*.go' internal cmd` must return nothing, and every relocated constructor must reach `_lyx` through `configengine.LyxDirName` rather than a local string. Confirm that `TestEnforcement_GeometryLiterals` passes, and that the only ownership row this batch touched is `_pattern`'s, moved to `internal/pattern` by card 25 because that card declares the token. Specifically, this batch registers **no** new `_lyx` owner: under the per-segment form no relocated constructor declares that token. An earlier draft of the staging plan said this batch would register `_lyx` owners; that is wrong under the pinned constant form, and if this grep finds a fused literal the fix is to split the join, never to add an ownership row. If either check fails, fix the offending constructor in this batch before proceeding — do not defer it to batch 8.
- **Commit:** none

## Batch Tests

`verify` runs the repo-wide tagged type-check followed by the untagged suites of every package that gains or loses a constructor. The scope is per-batch rather than repo-wide because batches 1-4 already landed the rename that touched everything; what moves here is confined to the listed packages and their callers.

The load-bearing new coverage is `cmd/lyx/constructoranchoring_test.go` (card 28), which is what makes this batch safe to review: it pins all three anchoring groups over both an unanchored and a subpath-anchored fixture, so a constructor silently re-based onto the wrong root fails immediately rather than at runtime in a subpath-anchored repo. The relocated test files (`planpath_test.go`, `discussionpath_test.go`, `loomstatus_test.go`, `webstergeom_test.go`, `scoutdaemon_test.go`, `worktreelogs_test.go`, `patternpath_test.go`) move with their symbols rather than being deleted — losing that coverage would be the silent cost of this refactor. Card 29 is a zero-diff grep gate, not a test.
