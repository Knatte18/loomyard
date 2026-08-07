# Batch: dotlyx-group-reanchor-and-logger-sink

```yaml
task: .lyx hygiene -- relocate transients out of _lyx, fix .lyx junction geometry (slice 9)
batch: dotlyx-group-reanchor-and-logger-sink
number: 4
cards: 8
verify: go test ./internal/logger/... ./internal/shuttleengine/... ./internal/burlerengine/... ./internal/reedengine/... ./internal/scoutengine/... ./internal/scoutcli/... ./cmd/lyx/... && go vet -tags scout ./internal/scoutengine/... && go test -tags integration ./internal/reedengine/...
depends-on: [3]
```

## Batch Scope

This batch re-anchors every pre-existing worktree-level `.lyx` consumer from `l.WorktreePath()` onto `l.AnchorPath()`, so `_lyx` and `.lyx` become true directory siblings and a subpath-anchored repo has exactly one `.lyx` root instead of two.
It also removes `internal/logger`'s persistent durable-sink file handle (open-append-close per record), which is a prerequisite rather than a cosmetic cleanup: without it, `lyx fabric reconcile` holds a trace file open inside the very directory batch 8's content adoption moves, and adoption can never succeed on Windows.

It is one batch because the two halves land in the same files and the same test table: `internal/logger/sink.go` both re-anchors and loses its handle, and `cmd/lyx/constructoranchoring_test.go` is the single place the whole anchoring table is pinned, so splitting would put two batches on the same file.

**External interface batch 5 and 8 consume:** `logger.LogsDir(l)` (renamed from `WorktreeLogsDir`, now `AnchorPath`-anchored), `scoutengine.DaemonStateFile`/`DaemonLock` fed an anchor path, and a durable sink that holds no open handle between records.

**Batch-local decision — `reedengine.HubLogsDir` keeps its `l.HubPath` anchor.**
One shared reed server per hub needs one deterministic hub-anchored location;
moving it into a worktree would destroy that.
Only reed's *worktree-level* state sites (`reed.json`, `reed.lock`) re-anchor.

**Batch-local decision — scout threads a new anchor value rather than re-purposing `Options.WorktreeRoot`.**
`internal/scoutcli` computes the anchor alongside `resolveWorktreeRoot` and passes it in a new `Options.AnchorRoot` field;
`scoutengine` cannot derive it itself, since its leaf allowlist excludes `internal/lyxcwd`.
Two corrections to the framing the discussion used for this decision, both verified against the code and recorded here so a later reader is not misled:
the LSP root is `rootURIFor(opts.TargetDir)`, **not** `WorktreeRoot`, so nothing about the language-server session moves with this change;
and the supervised daemon's identity *is* its state-file path, so relocating that path to the anchor necessarily re-keys the daemon.
That re-keying is the intended consequence of having one `.lyx` root per worktree — the point of the separate field is that the change is explicit and named, not that it can be avoided.
Inside `ensureServer`/`ensureSupervised` the anchor **replaces** the `worktreeRoot` parameter rather than joining it, because those two path constructors are its only in-function uses;
`Options.WorktreeRoot` survives as the field `scoutcli` populates and as the out-of-hub fallback `acquireConnection` substitutes when `AnchorRoot` is empty.

## Cards

### Card 23: rename logger.WorktreeLogsDir to LogsDir and re-anchor it

- **Context:**
  - `internal/lyxdirs/dirs.go`
  - `internal/lyxcwd/lyxcwd.go`
- **Edits:**
  - `internal/logger/sink.go`
  - `internal/logger/logger.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** rename `WorktreeLogsDir` to `LogsDir` and change its body to `filepath.Join(l.AnchorPath(), lyxdirs.DotLyxDirName, "logs")`.
  Rewrite its godoc: it is `AnchorPath`-anchored so it is a directory sibling of the durable `_lyx` tree, and the old name is gone because it would assert an anchor the function no longer uses.
  Update the one internal call site in `ensureDurableSink` (`dir = WorktreeLogsDir(layout)`).
  Leave `header.WorktreeRoot = layout.WorktreePath()` exactly as it is on the line below — that field records the worktree root as trace metadata and is a separate concern from where the file lands;
  add a one-line comment saying so, since the two lines now disagree by design.
  In `internal/logger/logger.go`, update the two doc-comment references to `WorktreeLogsDir(l)` (near the package's sink description and near its durable-half note) to `LogsDir(l)`.
- **Commit:** `refactor(logger): rename WorktreeLogsDir to LogsDir and anchor it on AnchorPath`

### Card 24: drop the durable sink's persistent file handle

- **Context:**
  - `internal/logger/logger.go`
  - `internal/logger/retention.go`
- **Edits:**
  - `internal/logger/sink.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** stop holding an `*os.File` for the process lifetime.
  Replace the `sinkWriter io.Writer` package global with a `sinkPath string` package global (the resolved trace-file path), because today the filename is a local inside `ensureDurableSink`'s `sync.Once` closure and is reachable only through the writer.
  `ensureDurableSink`'s return contract changes from `(io.Writer, bool)` to `(bool)` — readiness only;
  it still resolves the directory, arms the header, `MkdirAll`s, calls `Sweep(dir)`, composes the same `trace-<ts>-<traceID>-<pid>.log` filename, and writes `headerLine()` exactly once, but it opens the file with `os.O_CREATE|os.O_WRONLY|os.O_APPEND`, writes the header, and **closes it again** before returning, recording `sinkPath` and `sinkBytesWritten` as it does today.
  `writeDurable` opens `sinkPath` with the same three flags, appends `p`, and closes, all inside the `sinkMu` critical section it already takes — so the extra open/close pair sits under a lock that is held anyway.
  Keep every other behaviour byte-identical: the `sinkTruncated` early return, the `sinkBytesWritten + len(p) > sinkMaxBytes` check, the single `"trace sink truncated: size cap reached\n"` marker, and the accounting.
  The truncation marker must be written through the same open-append-close path, not to a stale handle.
  Add `sinkPath = ""` to `SetDurableSinkDir`'s reset block alongside the existing globals — that function already resets `sinkOnce` and every sink global, so re-entrant sink state is an established pattern here.
  In `logger.go`, `durableWriter.Write` currently does `if _, ok := ensureDurableSink(); !ok` — change it to the single-value form.
  `NotifyExit` keeps calling `ensureDurableSink()` for its side effect.
  Do not change `sinkMaxBytes`, `retention.go`'s `traceFilePattern`, or the truncate-and-go-silent policy;
  rollover to numbered parts is explicitly a separate task.
- **Commit:** `refactor(logger): open, append and close the durable sink per record`

### Card 25: cover the handle-free sink and the renamed accessor

- **Context:**
  - `internal/logger/sink.go`
  - `internal/logger/retention.go`
  - `internal/lyxcwd/lyxcwd.go`
- **Edits:**
  - `internal/logger/sink_test.go`
- **Creates:**
  - `internal/logger/logsdir_test.go`
- **Deletes:**
  - `internal/logger/worktreelogs_test.go`
- **Moves:** none
- **Requirements:** create `logsdir_test.go` as the replacement for `worktreelogs_test.go`, testing `logger.LogsDir` instead of `logger.WorktreeLogsDir`: for an unanchored synthetic `*lyxcwd.Location` it equals `filepath.Join(l.WorktreePath(), ".lyx", "logs")`, and for a subpath-anchored one it equals `filepath.Join(l.AnchorPath(), ".lyx", "logs")` and therefore **differs** from the `WorktreePath`-based path.
  The old file's `TestWorktreeLogsDir_IgnoresAnchorRel` asserted the inverse and is deleted with it, not carried over — a full-file replacement is correct here because every assertion inverts.
  In `sink_test.go`: update the `ensureDurableSink()` call sites to the single-return form (the existing test asserting the returned writer is non-nil must instead assert readiness is true and that `sinkPath` names an existing file);
  keep the existing header-written-once, byte-accounting and cap-marker assertions passing unchanged;
  and add the load-bearing new test — **rename the logs directory between two log records and assert the second record still lands**, which fails today with an open handle and must succeed after.
  Assert it by writing one record, renaming the sink directory out from under the process, writing a second record, and confirming no error and that the process holds no handle inside the original directory (on Linux, the rename succeeding plus the second write succeeding is the observable; do not attempt to enumerate file descriptors).
  Both files stay untagged — `t.TempDir()` plus `SetDurableSinkDir` only, no process spawn (Test Tier Purity Invariant).
- **Commit:** `test(logger): assert the sink survives a logs-dir rename mid-process`

### Card 26: re-anchor shuttle's run-dir root and reed-state lookup

- **Context:**
  - `internal/lyxdirs/dirs.go`
  - `internal/shuttleengine/config.go`
  - `internal/shuttleengine/template.yaml`
- **Edits:**
  - `internal/shuttleengine/rundir.go`
  - `internal/shuttleengine/run.go`
  - `internal/shuttleengine/rundir_test.go`
  - `internal/shuttleengine/run_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** in `runDirRoot`, change **both** bases from `layout.WorktreePath()` to `layout.AnchorPath()`: the `.lyx/shuttle` default branch and the relative-`cfg.RunDir` branch.
  An absolute `cfg.RunDir` stays verbatim, unchanged.
  Rewrite the godoc to say so explicitly and to state why both moved together: one function must never resolve against two different bases when `AnchorRel != "."`.
  In `run.go`'s `sweepOrphansOpportunistic`, change `reedengine.LoadState(filepath.Join(r.layout.WorktreePath(), lyxdirs.DotLyxDirName))` to use `r.layout.AnchorPath()` so it reads the same `reed.json` reed itself now writes (card 28).
  Do not change `template.yaml`'s `run_dir` default or its worktree-local-only documentation — the constraint that a configured run dir stays worktree-local is unaffected by which base a relative value resolves against.
  Update the tests that pin the old base: `rundir_test.go`'s `TestRunDirRoot_DefaultUsesDotLyxShuttle` and `TestRunDirRoot_RelativeResolvesAgainstWorktreeRoot` (rename the latter, since it now resolves against the anchor), leaving `TestRunDirRoot_AbsoluteUsedVerbatim` unchanged;
  and `run_test.go`'s four reed-state seeding joins in the orphan-sweep tests.
  At least one of the re-pointed cases must use a fixture whose `AnchorRel` is a real subpath, or the change is not observable.
- **Commit:** `refactor(shuttleengine): anchor the run-dir root and reed-state lookup on AnchorPath`

### Card 27: re-anchor burler's per-round instruction dir

- **Context:**
  - `internal/lyxdirs/dirs.go`
- **Edits:**
  - `internal/burlerengine/engine.go`
  - `internal/burlerengine/doc.go`
  - `internal/burlerengine/engine_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** change `Engine.Run`'s `burlerDir := filepath.Join(e.layout.WorktreePath(), lyxdirs.DotLyxDirName, "burler")` to use `e.layout.AnchorPath()`, and update the doc comment above it plus the matching sentence in `doc.go` that describes materializing instruction files "to a fresh per-round directory under .lyx".
  Note that `p.validate(e.layout.WorktreePath(), e.cfg)` on the line above is a **different** concern — it validates the review target's own file paths against the worktree root — and must NOT be re-anchored.
  In `engine_test.go`, update the two existing assertions that expect the instruction dir under a `WorktreePath`-based `.lyx` to expect the `AnchorPath`-based one, and make at least one of them use a fixture whose `AnchorRel` is a real subpath so the change is actually observable.
- **Commit:** `refactor(burlerengine): anchor the per-round instruction dir on AnchorPath`

### Card 28: re-anchor reed's worktree-level state sites behind one helper

- **Context:**
  - `internal/lyxdirs/dirs.go`
  - `internal/reedengine/state.go`
- **Edits:**
  - `internal/reedengine/lifecycle.go`
  - `internal/reedengine/lock.go`
  - `internal/reedengine/spawn.go`
  - `internal/reedengine/strand.go`
  - `internal/reedengine/spawn_test.go`
  - `internal/reedengine/strand_test.go`
  - `internal/reedengine/lock_test.go`
  - `internal/reedengine/contract_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** reed repeats `filepath.Join(e.layout.WorktreePath(), lyxdirs.DotLyxDirName)` at ten sites.
  Add one private method on `*Engine` — `func (e *Engine) stateDir() string { return filepath.Join(e.layout.AnchorPath(), lyxdirs.DotLyxDirName) }` — in `lifecycle.go`, documented as the worktree-level ephemeral tree holding `reed.json` and `reed.lock`, `AnchorPath`-anchored so it is a sibling of `_lyx`, and distinct from `HubLogsDir`'s hub anchor.
  Replace all ten joins with `e.stateDir()`: `lifecycle.go`'s two `SaveState` calls, its `reedStateFileName` path join and its `LoadState` call;
  `lock.go`'s `withOpLock` (keep the existing `MkdirAll`-before-lock and its comment);
  `spawn.go`'s `LoadState` and `SaveState`;
  `strand.go`'s three `SaveState` calls.
  Leave `HubLogsDir` and reed's own idempotent `MkdirAll(HubLogsDir(e.layout))` in the boot path untouched — hub-anchored by design.
  Retarget the four test files that build the same join by hand: `spawn_test.go`, `strand_test.go`, `lock_test.go` and `contract_integration_test.go` (`//go:build integration`, tag line stays first).
  Since `stateDir` is a method on `*Engine` and these tests already hold an `e`, prefer `e.stateDir()` in them over re-deriving the join, so a future base change needs no test edit;
  make at least one case use a subpath-anchored fixture so the move is observable.
- **Commit:** `refactor(reedengine): anchor worktree state on AnchorPath behind one helper`

### Card 29: thread an anchor root into scout's daemon-state paths

- **Context:**
  - `internal/lyxdirs/dirs.go`
  - `internal/scoutengine/leaf_enforcement_test.go`
  - `internal/scoutengine/ensureserver.go`
- **Edits:**
  - `internal/scoutengine/daemonstate.go`
  - `internal/scoutengine/refs.go`
  - `internal/scoutengine/ensureserver.go`
  - `internal/scoutengine/ensureserver_test.go`
  - `internal/scoutengine/supervised_test.go`
  - `internal/scoutengine/ensureserver_integration_test.go`
  - `internal/scoutengine/supervised_integration_test.go`
  - `internal/scoutengine/supervised_scout_test.go`
  - `internal/scoutcli/cli.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** add `AnchorRoot string` to `scoutengine.Options` (`refs.go`), documented as the anchor-relative root the daemon's ephemeral state tree hangs off, supplied by `internal/scoutcli` because `scoutengine`'s leaf allowlist excludes `internal/lyxcwd`.
  It is a **new field, not a re-purposing** of `WorktreeRoot`: `WorktreeRoot` keeps its own name, value and meaning, and continues to be populated by `scoutcli`.
  Document at the field that this deliberately does change the daemon's identity — the supervised daemon is keyed by its state-file path, so relocating that path to the anchor re-keys it — and that this is the intended, explicit consequence of having exactly one `.lyx` root per worktree, not a side effect of overloading an existing field.
  Note also, in the field's godoc, that `WorktreeRoot` does **not** determine the LSP root: `rootURIFor(opts.TargetDir)` does, so nothing about the language-server session moves with this change.
  Rename `DaemonStateFile`/`DaemonLock`'s first parameter from `worktreePath` to `anchorPath` and update their godocs to say the daemon is still a per-worktree, per-language singleton but its state tree is now a sibling of `_lyx` at the anchor.
  Thread the value by **replacing** the `worktreeRoot` parameter of `ensureServer` and `ensureSupervised` with `anchorRoot` rather than adding a second one: inside those two functions, `worktreeRoot`'s only uses are `DaemonStateFile(worktreeRoot, lang)` and `DaemonLock(worktreeRoot, lang)` in `ensureSupervised` (plus one comment naming the `(worktreeRoot, lang)` key) and `ensureServer`'s forwarding of it, so once path construction moves to the anchor **no in-function use of `worktreeRoot` remains** and keeping it would leave a dead parameter.
  Update that `(worktreeRoot, lang)`-key comment to `(anchorRoot, lang)`.
  Resolve the out-of-hub fallback **once**, in `acquireConnection`: when `opts.AnchorRoot` is empty (a caller outside a lyx hub, where `resolveAnchorRoot` returns `""` while `resolveWorktreeRoot` still yields an absolute target dir), pass `opts.WorktreeRoot` in its place, so the out-of-hub path behaves exactly as it does today.
  That fallback is the live use `Options.WorktreeRoot` retains inside `scoutengine` — document it at both fields.
  Update the five test files that call `ensureServer`/`ensureSupervised` positionally to the new parameter list, or they fail to compile: `ensureserver_test.go` and `supervised_test.go` (untagged, so they break batch 4's own untagged `verify:` run), and `ensureserver_integration_test.go`, `supervised_integration_test.go` and `supervised_scout_test.go` (all three `//go:build scout`, so neither a plain `go test` nor `go vet -tags integration` compiles them — the batch's `verify:` carries a `-tags scout` vet specifically for these).
  In the untagged pair, the existing calls pass the same value for both `targetDir`/`worktreeRoot` or a `t.TempDir()`;
  keep each call's intent, just drop to the single anchor-root argument.
  Give at least one untagged case an anchor root that differs from the worktree root, so the re-keying is observable rather than accidentally identical.
  In `internal/scoutcli/cli.go`, add `resolveAnchorRoot(cwd, targetDir string) string` beside `resolveWorktreeRoot`, returning `layout.AnchorPath()` on a successful `lyxcwd.Resolve` and `""` otherwise, and add an `anchorRoot` parameter to `buildOptions` so **every** `buildOptions` call site threads it — six in total, two per verb (the single-arg path and the batch-mode path) across the `references`, `definition` and `symbol` verbs — plus the one hand-built `scoutengine.Options` literal.
  The three `buildOptions`-driven verbs each need `anchorRoot := resolveAnchorRoot(cwd, dir)` alongside their existing `worktreeRoot := resolveWorktreeRoot(cwd, dir)` line.
  `assertNoCallersCommand` is the fourth, and it does **not** follow that shape: it resolves its root inline — `var worktreeRoot string; if layout, resolveErr := lyxcwd.Resolve(cwd); resolveErr == nil { registry = loaded; worktreeRoot = layout.WorktreePath() }` — never calling the shared `resolveWorktreeRoot` helper, and leaving `worktreeRoot` empty outside a hub rather than falling back to `filepath.Abs(dir)`.
  Thread the anchor there by adding `var anchorRoot string` beside it and setting `anchorRoot = layout.AnchorPath()` inside that same `if` block, reusing the `layout` already in scope rather than calling `resolveAnchorRoot` a second time, then pass `AnchorRoot: anchorRoot` in the hand-built `baseOpts` literal.
  Do not "fix" this verb's divergent inline resolution or its missing out-of-hub fallback — that is pre-existing behaviour outside this task's scope;
  just make sure the anchor is threaded, or `assert-no-callers` would keep resolving a pre-slice-9 daemon identity in a subpath-anchored repo while the other three verbs resolve the new one.
- **Commit:** `refactor(scout): resolve daemon state from an explicit anchor root`

### Card 30: collapse the anchoring table and correct the anchoring docs

- **Context:**
  - `internal/logger/sink.go`
  - `internal/scoutengine/daemonstate.go`
  - `internal/reedengine/lifecycle.go`
  - `internal/websterengine/state.go`
  - `internal/lyxdirs/dirs.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `cmd/lyx/constructoranchoring_test.go`
  - `docs/shared-libs/README.md`
  - `manifest/designs/fabric-unified-view.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** in `cmd/lyx/constructoranchoring_test.go`, collapse batch 3's two `.lyx` bases back into one: delete `dotLyxBase`'s `worktree`-based definition and the temporary `dotLyxAnchorBase` local, keeping a single `dotLyxBase := filepath.Join(anchor, ".lyx")`.
  Retarget `logger.WorktreeLogsDir` to `logger.LogsDir` and move `scoutengine.DaemonStateFile`/`DaemonLock` onto an anchor-path argument, so in `TestConstructorAnchoring_SubpathAnchored` every worktree-level `.lyx` entry now moves down by `AnchorRel` exactly like the `_lyx` group, while `reedengine.HubLogsDir` alone stays byte-identical.
  Rewrite the file-header comment accordingly — its current text says the `.lyx` group "stays byte-identical", which this batch inverts — and rename the sub-test comment "`.lyx` ephemeral group: stays WorktreePath-anchored, ignoring AnchorRel entirely" to state the new rule.
  Add an explicit assertion that there is exactly **one** `.lyx` root in the subpath-anchored fixture: every worktree-level `.lyx` constructor's result must have `filepath.Join(anchor, ".lyx")` as a prefix, and none may have `filepath.Join(worktree, ".lyx")` as its prefix.
  This is the regression guard for the two-roots bug the whole re-anchoring exists to remove.
  In `docs/shared-libs/README.md`, rewrite the `internal/logger` bullet's clause "worktree-anchored (`.lyx/logs`, lazily opened, retained by age and count, capped at 8 MiB per file)": both halves change — the anchor is now `AnchorPath()`, and the sink is no longer a lazily-opened persistent handle but is opened, appended to and closed per record.
  In `manifest/designs/fabric-unified-view.md`, correct the "Shipped correction (slice 7)" paragraph's as-built anchoring table: the `.lyx` group (`WorktreeLogsDir`, `ScoutDaemonStateFile`, `ScoutDaemonLock`) no longer joins onto `Location.WorktreePath()` — it joins onto `Location.AnchorPath()` as of slice 9, with `HubLogsDir` alone still on `Location.HubPath`, and `WorktreeLogsDir` is now named `logger.LogsDir`.
  Drop that paragraph's claim that the table is "mirrored verbatim in `CONSTRAINTS.md`'s Cwd Resolution Invariant" — no such per-symbol table exists there;
  CONSTRAINTS carries the per-segment join rule only, so the cross-reference is stale and is removed rather than chased.
  Keep the repo's semantic-line-break markdown rule in both docs.
- **Commit:** `test+docs: collapse the .lyx group onto AnchorPath`

## Batch Tests

`verify: go test ./internal/logger/... ./internal/shuttleengine/... ./internal/burlerengine/... ./internal/reedengine/... ./internal/scoutengine/... ./internal/scoutcli/... ./cmd/lyx/... && go vet -tags scout ./internal/scoutengine/... && go test -tags integration ./internal/reedengine/...` — the six edited packages plus `cmd/lyx` for the anchoring table, a `-tags scout` **vet** because card 29 re-signatures `ensureServer`/`ensureSupervised` and three of their callers live behind `//go:build scout` (a tag neither the plain `go test` run nor the overview's module-wide `go vet -tags integration ./...` compiles), and a tagged reed run because reed's state-path change is the one whose real-substrate behaviour is covered by `internal/reedengine/contract_integration_test.go`.
The scout gate is a `vet` (compile + analysis) rather than a `test` run on purpose: those three files spawn a real `gopls` daemon, which is a substrate dependency a per-batch gate must not require.
Their behavioural run stays with the operator-driven `scout`-tagged suite, and this batch's stake in them is only that they still compile against the new signature.

Covered files: `internal/logger/logsdir_test.go` (new), `internal/logger/sink_test.go`, `internal/logger/retention_test.go`, `internal/shuttleengine/rundir_test.go`, `internal/burlerengine/engine_test.go`, `internal/reedengine/contract_integration_test.go`, `cmd/lyx/constructoranchoring_test.go`.

Two assertions carry this batch.
The first is card 25's rename-the-logs-directory-between-two-records test: it is the only observable proof that no handle survives a `writeDurable` call, it fails against today's code, and batch 8's content adoption is unsafe without it.
The second is card 30's one-`.lyx`-root assertion on a subpath-anchored fixture: asserting each constructor's new value individually would pass an implementation that re-anchored some sites and missed others, whereas the prefix-exclusion form fails the moment any worktree-level consumer is left behind.

Scout's anchoring is asserted through `cmd/lyx/constructoranchoring_test.go` for the path arithmetic, plus card 29's requirement that at least one untagged `ensureSupervised` case use an anchor root differing from the worktree root — without that, every existing call passes the same value twice and the re-keying would be untestable by construction.
The end-to-end claim that a lookup in a subpath-anchored worktree resolves `daemon.json`/`daemon.lock` under the anchor is asserted in `internal/scoutcli`'s own tests if a suitable harness already exists there, and is otherwise left to the `scout`-tagged suite — state which applies in the implementer's commit message rather than silently skipping it.
Note that the daemon's identity **does** change with this batch (it is keyed by its state-file path), which is intended;
there is therefore no "singleton key unchanged" assertion to write, and card 29 documents that consequence at the `AnchorRoot` field instead.
