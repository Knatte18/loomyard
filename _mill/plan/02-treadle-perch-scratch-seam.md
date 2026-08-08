# Batch: treadle-perch-scratch-seam

```yaml
task: .lyx hygiene -- relocate transients out of _lyx, fix .lyx junction geometry (slice 9)
batch: treadle-perch-scratch-seam
number: 2
cards: 6
verify: go test ./internal/treadleengine/... ./internal/perchengine/... ./internal/perchcli/... && go test -tags integration ./internal/perchcli/...
depends-on: [1]
```

## Batch Scope

This batch gives `internal/treadleengine` an explicit scratch-directory input and threads a `.lyx`-anchored value into it from `internal/perchcli` via a new `perchengine.ScratchDir(l)` accessor, moving perch/treadle's three never-tracked artifacts — `run.lock`, `state.json.lock` and the `pause` flag — out of `_lyx/perch/<block>` and into `.lyx/perch/<block>`, while `state.json` and every round artifact stay in `runDir` under `_lyx`.
It is one batch because treadle's engine, perch's re-export shims, and perchcli's three call sites (the run verb's pause closure, the pause verb, and the engine construction) form one compile unit: changing any one alone leaves the others broken or, worse, silently writing the pause flag to a path the loop never reads.

**External interface batch 5 consumes:** `perchengine.ScratchDir(l *lyxcwd.Location) string`, and `treadleengine.PauseFlagPath(scratchDir string)`.

**Batch-local decision — the scratch dir is an `Options` field, defaulted at `Run`'s entry.**
`treadleengine.Options` gains `ScratchDir string`;
`New` stores it verbatim (nils and empties included, matching how it already stores `PauseRequested`/`RunCommand`), and `Run` resolves `scratchDir := e.scratchDir; if scratchDir == "" { scratchDir = runDir }` as its **first statement**, not inside the later block that defaults the `pause` and `runCommand` seams.
`Run` still owns the fallback, as `engine.go`'s doc says it does, but it has to own it earlier than those two seams do: the deferred terminal `clearPauseFlag`, the `runDir` MkdirAll, the run-lock acquisition and the entry-time `clearPauseFlag` all precede that block and all need the value.
The fallback does not widen `Run`'s own signature, whose second parameter stays the durable `runDir`.

**Batch-local decision — `perchcli` derives the per-block scratch dir the same way it derives the run dir.**
`resolveRunTarget` returns a fourth path, `scratchDir = filepath.Join(c.scratchDirBase, id)`, from the same already-derived `id`, so the two directories can never disagree about which block they belong to.

## Cards

### Card 9: give treadleengine an explicit scratch-directory input

- **Context:**
  - `internal/treadleengine/doc.go`
  - `internal/treadleengine/seam_enforcement_test.go`
- **Edits:**
  - `internal/treadleengine/engine.go`
  - `internal/treadleengine/run.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** add `ScratchDir string` to `treadleengine.Options` with a godoc stating it is the directory this block's never-tracked artifacts (`run.lock`, `state.json.lock`, the `pause` flag) are written to, that an empty value defaults to `runDir` for back-compat, and that the engine never derives it — the caller is told, never the engine (Cwd Resolution Invariant).
  Add a matching `scratchDir string` field to `Engine` and store `opts.ScratchDir` verbatim in `New`.
  In `Run`, resolve `scratchDir` at the **very top of the function body**, as its first statement — before the deferred terminal-clear closure, before `p.validate`, before `os.MkdirAll(runDir, ...)`, and before the run-lock acquisition: `scratchDir := e.scratchDir; if scratchDir == "" { scratchDir = runDir }`.
  This placement is load-bearing and must not be folded into the existing "Seam defaulting happens here, once, at Run's entry" block that defaults `pause` and `runCommand`: that block sits *after* the deferred closure, the `runDir` MkdirAll, the run-lock acquisition and the entry-time `clearPauseFlag` call, all four of which this card retargets onto `scratchDir`, so resolving it there would either forward-reference an undeclared local from the deferred closure or silently demand hoisting that whole block.
  Leave `pause`/`runCommand` defaulting exactly where it is;
  only `scratchDir` moves to the top.
  Add `os.MkdirAll(scratchDir, 0o755)` right after the existing `os.MkdirAll(runDir, ...)` — the run lock is taken inside the scratch dir, so it must exist first — and note in a comment that the `runDir` MkdirAll stays because `state.json` and round artifacts still live there.
  When `e.scratchDir` is empty the two MkdirAll calls name the same directory, which is a harmless no-op on the second.
  Retarget `Run`'s three transient sites onto `scratchDir`: the `lock.TryAcquireWriteLock(filepath.Join(runDir, runLockName))` call, and both `clearPauseFlag(e.name, runDir)` calls (the deferred terminal clear and the entry-time clear).
  Pass `scratchDir` through to `loadOrInitState` and to every `saveState` call in `Run` and `runRound`'s callers (card 10 changes those signatures).
  Update `Run`'s and `runLockName`'s godoc to say the lock lives in the scratch dir, still held for the whole call, and note that `state.json` itself stays in `runDir`.
  Do not change `Engine.Run`'s own parameter list, and do not add any import beyond what is already there — `treadleengine` must stay off `internal/lyxdirs` and `internal/lyxcwd` so its seam allowlist is unchanged.
- **Commit:** `feat(treadleengine): add an explicit scratch-directory input`

### Card 10: move treadle's state lock and pause flag onto the scratch dir

- **Context:**
  - `internal/treadleengine/run.go`
  - `internal/state/state.go`
- **Edits:**
  - `internal/treadleengine/state.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** thread a `scratchDir` parameter through the four functions in this file that resolve a lock or the pause flag, keeping `runDir` as the first path parameter wherever both are needed:
  `loadOrInitState(name, runDir, scratchDir, hash string/[]int...)`, `saveState(runDir, scratchDir string, s runState)`, `TerminalOutcome(runDir, scratchDir string)`, and `clearPauseFlag(name, scratchDir string)`.
  In `loadOrInitState`, `saveState` and `TerminalOutcome` the JSON path stays `filepath.Join(runDir, stateFileName)` while the lock path becomes `filepath.Join(scratchDir, stateFileName+".lock")` — do NOT keep the `path + ".lock"` idiom, which would follow the file into `runDir`.
  `saveState` must `os.MkdirAll(scratchDir, 0o755)` before writing, since `internal/state`'s `WriteJSON` creates the parent of the file it writes but not the parent of a lock in a sibling tree.
  Change `PauseFlagPath` to take `scratchDir` and update its godoc to say the flag lives in the block's scratch dir, not its run dir, and that a caller's pause verb must resolve it from the same scratch base the run verb passes to the engine.
  `moveStaleArtifacts`/`moveStaleIfExists`/`artifactPaths` are unchanged — round artifacts stay in `runDir`.
  Update the file-header comment to record the split: `state.json` and round artifacts in `runDir`, `state.json.lock`/`run.lock`/`pause` in `scratchDir`.
- **Commit:** `refactor(treadleengine): resolve state lock and pause flag from the scratch dir`

### Card 11: cover the treadle scratch seam in tests

- **Context:**
  - `internal/treadleengine/state.go`
  - `internal/treadleengine/run.go`
  - `internal/treadleengine/engine.go`
  - `internal/treadleengine/runner.go`
  - `internal/treadleengine/profile.go`
- **Edits:**
  - `internal/treadleengine/state_test.go`
  - `internal/treadleengine/engine_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** update every existing call of the four re-signatured functions to the new parameter lists, and add coverage for the seam itself:
  (a) with `Options.ScratchDir` unset, `run.lock`, `state.json.lock` and `pause` all land in `runDir` — the back-compat default;
  (b) with `Options.ScratchDir` set to a second temp dir, `run.lock`, `state.json.lock` and `pause` land there while `state.json` and the round artifacts stay in `runDir`, and `runDir` contains no `.lock` file and no `pause` entry at all;
  (c) `ErrBlockBusy` still fires for a second concurrent `Run` against the same pair of directories — assert via `errors.Is`;
  (d) `PauseFlagPath(scratchDir)` and a subsequent `Run` agree: writing the flag at `PauseFlagPath(scratchDir)` makes the run return `OutcomePaused`, and the flag is gone afterwards, when scratch and run dirs differ.
  Reuse whatever fake `RoundRunner`/`Shuttle` the existing tests in these two files already define rather than adding new fakes.
  Both files are untagged and must stay untagged: use `t.TempDir()`, spawn no process, and copy no fixture tree (Test Tier Purity Invariant).
- **Commit:** `test(treadleengine): cover the scratch-dir seam and its back-compat default`

### Card 12: add perchengine.ScratchDir and re-key its transient re-exports

- **Context:**
  - `internal/lyxdirs/dirs.go`
  - `internal/treadleengine/state.go`
  - `internal/perchengine/doc.go`
- **Edits:**
  - `internal/perchengine/identity.go`
  - `internal/perchengine/engine.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** in `identity.go`, add `func ScratchDir(l *lyxcwd.Location) string` returning `filepath.Join(l.AnchorPath(), lyxdirs.DotLyxDirName, perchDirName)` — the mirrored sibling of `RunsDir`, documented as the never-tracked half and as the base a caller joins a block's run-id onto, exactly as it joins onto `RunsDir`.
  Update `perchDirName`'s godoc, which today names only `LyxDirName`, to say `perchengine` joins the segment onto both `lyxdirs.LyxDirName` and `lyxdirs.DotLyxDirName`.
  Re-key the two transient re-exports: `PauseFlagPath(scratchDir string)` forwards to `treadleengine.PauseFlagPath(scratchDir)`, and `TerminalOutcome(runDir, scratchDir string)` forwards to `treadleengine.TerminalOutcome(runDir, scratchDir)` — keep the existing `logger.Warn` on error and add `scratchDir` to its key/value pairs.
  In `engine.go`, widen `Engine.Run` to `Run(p Profile, runDir, scratchDir string)` and pass `ScratchDir: scratchDir` in the `treadleengine.Options` literal it builds.
  Update `Engine.Run`'s godoc: it stays weft-blind and geometry-blind and constructs neither path — both are caller-supplied absolutes — and `treadleengine.Engine.Run` owns creating both directories.
  Add `scratchDir` to the existing `logger.Warn("perch: round loop failed", ...)` key/value pairs.
- **Commit:** `feat(perchengine): add ScratchDir and re-key the transient accessors`

### Card 13: thread the .lyx scratch dir through perchcli

- **Context:**
  - `internal/perchengine/identity.go`
  - `internal/perchengine/engine.go`
- **Edits:**
  - `internal/perchcli/cli.go`
  - `internal/perchcli/run.go`
  - `internal/perchcli/pause.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** in `cli.go`, add a `scratchDirBase string` field to `perchCLI` beside `runDirBase` and populate it in `PersistentPreRunE` with `perchengine.ScratchDir(layout)`, immediately after the existing `c.runDirBase = perchengine.RunsDir(layout)` line, with a comment noting it is the same `layout.AnchorPath()` anchor and the never-tracked half of the pair.
  In `run.go`, widen `resolveRunTarget` to also return `scratchDir string` computed as `filepath.Join(c.scratchDirBase, id)` from the same `id`, extend its godoc to say the two directories are derived from one identity so they can never disagree about the block, pass `scratchDir` into `engine.Run(profile, runDir, scratchDir)`, and change the `PauseRequested` closure to stat `perchengine.PauseFlagPath(scratchDir)`.
  Add `"scratchDir": scratchDir` to the success envelope's `output.Ok` map beside the existing `"runDir"` key, so an operator can see both halves.
  Leave the fabric-sync block's `fabricengine.ScopedPathspec(c.layout.AnchorRel, []string{lyxdirs.LyxDirName})` naming `_lyx` only — `.lyx` must never appear in a pathspec — and rewrite the long comment above it that today explains perch's locks "must live beside state.json inside the run dir (the engine is geometry-blind), so they are excluded solely by the fabric repo's .git/info/exclude (deepened to reach perch's two-deep locks)" to state the new reality: the locks and the pause flag now live in the block's `.lyx` scratch dir, so no exclusion layer is involved at all.
  In `pause.go`, derive `scratchDir := filepath.Join(c.scratchDirBase, runID)` next to the existing `runDir`, call `perchengine.TerminalOutcome(runDir, scratchDir)`, and write the flag at `perchengine.PauseFlagPath(scratchDir)` — keeping the existing stat-the-run-dir precondition (the block must have started at least once) against `runDir`, and adding an `os.MkdirAll(scratchDir, 0o755)` before the write, since a block whose run dir exists may have no scratch dir yet if it was created by a pre-fix binary.
- **Commit:** `refactor(perchcli): pass the .lyx-anchored scratch dir to the perch engine`

### Card 14: cover perch's scratch-dir wiring in tests

- **Context:**
  - `internal/perchengine/identity.go`
  - `internal/perchcli/run.go`
  - `internal/perchcli/pause.go`
  - `internal/perchcli/cli.go`
  - `internal/lyxdirs/dirs.go`
- **Edits:**
  - `internal/perchengine/identity_test.go`
  - `internal/perchengine/run_test.go`
  - `internal/perchcli/run_test.go`
  - `internal/perchcli/cli_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** in `internal/perchengine/identity_test.go`, add a `ScratchDir` case asserting it equals `filepath.Join(l.AnchorPath(), ".lyx", "perch")` for both an unanchored and a subpath-anchored synthetic `*lyxcwd.Location`, and assert it differs from `RunsDir(l)` only in that one segment.
  In `internal/perchengine/run_test.go` and `internal/perchcli/run_test.go`, update every `Engine.Run` / `resolveRunTarget` call to the widened signatures and assert `resolveRunTarget` returns a `scratchDir` whose last segment equals `runDir`'s last segment (same block id, different base).
  In `internal/perchcli/cli_integration_test.go`, update the `perchengine.PauseFlagPath` call sites to the scratch dir and add an assertion that after `lyx perch pause`, the flag exists under the `.lyx` perch tree and **no** `pause` file exists under the `_lyx` perch tree.
  Keep `//go:build integration` first in that file.
- **Commit:** `test(perch): assert the scratch dir is .lyx-anchored end to end`

## Batch Tests

`verify: go test ./internal/treadleengine/... ./internal/perchengine/... ./internal/perchcli/... && go test -tags integration ./internal/perchcli/...` — the three packages this batch edits, plus the tagged run needed because card 14 edits `internal/perchcli/cli_integration_test.go` (`//go:build integration`).
The overview's module-wide `go build ./... && go vet -tags integration ./...` catches any other package that still calls the re-signatured `perchengine.TerminalOutcome`/`PauseFlagPath` or `Engine.Run`.

Covered files: `internal/treadleengine/state_test.go`, `internal/treadleengine/engine_test.go`, `internal/perchengine/identity_test.go`, `internal/perchengine/run_test.go`, `internal/perchcli/run_test.go`, `internal/perchcli/cli_integration_test.go`, plus the already-passing `internal/perchcli/run_integration_test.go` and `internal/treadleengine/gate_test.go`/`judge_test.go`/`handoff_test.go`/`roundfiles_test.go`, which must keep passing untouched.

The load-bearing assertions are the pair in card 11: the unset-`ScratchDir` default must keep every artifact in `runDir` (so no existing treadle caller silently changes behaviour), and the set-`ScratchDir` case must show `runDir` holding **no** `.lock` and **no** `pause` entry — asserting only "the lock is in scratch" would pass an implementation that writes it to both.
Mutual exclusion (`ErrBlockBusy`) and pause-clearing are asserted specifically with the two directories different, because that is the configuration where a half-threaded seam still looks correct in a same-directory test.
