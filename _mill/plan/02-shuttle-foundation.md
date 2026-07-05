# Batch: shuttle-foundation

```yaml
task: 'Build internal/shuttle: one LLM agent via a swappable engine'
batch: shuttle-foundation
number: 2
cards: 4
verify: go test ./internal/shuttleengine/... ./internal/configreg/...
depends-on: []
```

## Batch Scope

Create the `internal/shuttleengine` package skeleton: config module (registered in
configreg), the run `Spec` with validation, run-directory management with `run.json`
state and the age-guarded orphan sweep, and the Windows→POSIX path helper. No mux calls
and no provider knowledge yet — everything here is pure and hermetically testable.
External interface consumed by batches 3–5: `Config`/`LoadConfig`/`ConfigTemplate`,
`Spec` (+ validation), `RunState`, `createRunDir`/`saveRunState`/`loadRunState`/
`findRunByStrand`/`sweepOrphans`, `posixPath`.

## Cards

### Card 5: shuttle config module

- **Context:**
  - `internal/muxengine/config.go`
  - `internal/muxengine/template.go`
  - `internal/muxengine/template.yaml`
  - `internal/muxengine/config_test.go`
- **Edits:** none
- **Creates:**
  - `internal/shuttleengine/doc.go`
  - `internal/shuttleengine/config.go`
  - `internal/shuttleengine/template.go`
  - `internal/shuttleengine/template.yaml`
  - `internal/shuttleengine/config_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Mirror muxengine's config trio exactly. `template.yaml` keys (flat,
  `${env:...}` defaults, trailing comments):
  `run_dir: ${env:LYX_SHUTTLE_RUN_DIR:-}` (empty = `<worktree>/.lyx/shuttle` via
  hubgeometry — say so in the comment without constructing the path here),
  `poll_interval_ms: 500`, `liveness_every_n_polls: 10`, `run_timeout_min: 30`,
  `startup_timeout_s: 90`, `claude: ${env:LYX_SHUTTLE_CLAUDE:-}` (comment: path to the
  claude executable; empty falls back to `claude` on PATH — prefer an explicit path, PATH
  aliases can hit the 0-byte WindowsApps stub), `claude_deny_agent_tool: true`,
  `claude_deny_ask_user_question: true`. `template.go`: `ConfigTemplate() string` via
  `go:embed template.yaml` (muxengine pattern). `config.go`: `type Config struct` with
  fields `RunDir, Claude string`, `PollIntervalMS, LivenessEveryNPolls, RunTimeoutMin,
  StartupTimeoutS int`, `ClaudeDenyAgentTool, ClaudeDenyAskUserQuestion bool` (yaml tags
  matching the template keys) and `LoadConfig(baseDir, module string) (Config, error)`
  calling `configengine.Load` with the same "not initialized here; run \"lyx init\""
  wrapping as `muxengine.LoadConfig`. `doc.go`: package header describing shuttleengine's
  role (one agent run over the file contract; provider-invariant core; engine seam) —
  this header is where the durable design from `docs/modules/shuttle.md` folds in per the
  documentation lifecycle. `config_test.go`: template defaults resolve; env override
  works; module arg threaded through; not-initialized error (mirror the four muxengine
  config tests).
- **Commit:** `feat(shuttle): config module (shuttle.yaml) with engine-prefixed claude keys`

### Card 6: configreg registration

- **Context:**
  - `internal/shuttleengine/template.go`
- **Edits:**
  - `internal/configreg/configreg.go`
  - `internal/configreg/configreg_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add `{"shuttle", shuttleengine.ConfigTemplate}` to `Modules()` in
  alphabetical position (between `mux` and `warp`), with the import. Update the `want`
  list in `configreg_test.go`'s `Names()` test to `[board, mux, shuttle, warp, weft]`.
- **Commit:** `feat(shuttle): register shuttle config module in configreg`

### Card 7: Spec type and validation

- **Context:**
  - `internal/muxengine/render/types.go`
  - `internal/shuttleengine/config.go`
- **Edits:** none
- **Creates:**
  - `internal/shuttleengine/spec.go`
  - `internal/shuttleengine/spec_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** `type Spec struct` with fields `Prompt string`, `OutputFiles
  []string`, `Model string`, `Interactive bool` (godoc: `Interactive == !Autonomous`; zero
  value = autonomous, the default — see plan Shared Decisions), `Role, Round string`,
  `Parent string`, `Display render.Display`, `Timeout time.Duration`, `KeepPane bool`.
  Implement `func (s *Spec) validate(worktreeRoot string, cfg Config) error`: `Prompt`
  must be non-empty; `OutputFiles` must have ≥1 entry (error text states the file
  contract: a run's output file IS its return value); each entry is resolved to absolute —
  already-absolute kept verbatim, relative joined onto `worktreeRoot` then
  `filepath.Clean`ed — and written back into `s.OutputFiles`; `Timeout == 0` is replaced
  with `time.Duration(cfg.RunTimeoutMin) * time.Minute`; `Display.Anchor == ""` defaults
  to `render.AnchorBelowParent`. Tests: empty prompt, empty OutputFiles, relative→absolute
  resolution, absolute passthrough, timeout defaulting, anchor defaulting.
- **Commit:** `feat(shuttle): run Spec with mandatory-OutputFiles validation`

### Card 8: run directory, run.json state, orphan sweep, POSIX helper

- **Context:**
  - `internal/state/state.go`
  - `internal/muxengine/state.go`
  - `internal/muxengine/name.go`
  - `internal/hubgeometry/hubgeometry.go`
- **Edits:** none
- **Creates:**
  - `internal/shuttleengine/rundir.go`
  - `internal/shuttleengine/rundir_test.go`
  - `internal/shuttleengine/posix.go`
  - `internal/shuttleengine/posix_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** `rundir.go`: (a) `newRunID() (string, error)` — 128-bit crypto/rand
  hex (same recipe as muxengine's `newGUID` in `name.go`); (b) `runDirRoot(cfg Config,
  layout *hubgeometry.Layout) string` — `cfg.RunDir` when non-empty (relative resolved
  against `layout.WorktreeRoot`), else `filepath.Join(layout.DotLyxDir(), "shuttle")`;
  (c) `type RunState struct` persisted as `run.json` per run dir via
  `state.WriteJSON`/`state.ReadJSON` (lock path `run.json.lock`): fields `RunID,
  StrandGUID, SessionID string`, `Interactive bool`, `OutputFiles []string`, `PromptPath,
  SettingsPath, EventsPath string`, `CreatedAt string` (RFC3339; caller supplies);
  (d) `saveRunState`/`loadRunState` wrappers; (e) `findRunByStrand(root, guid string)
  (RunState, string, error)` — scan `<root>/*/run.json` for a matching `StrandGUID`,
  returning the state and its run dir (used by the CLI interrupt/send verbs to verify a
  guid is a shuttle run); (f) `sweepOrphans(root string, strandGUIDs map[string]bool,
  minAge time.Duration, now time.Time) ([]string, error)` — remove each run dir whose
  `run.json` has a `StrandGUID` not in `strandGUIDs`, SKIPPING any dir whose mtime is
  younger than `minAge` (age guard: a concurrently starting run creates its dir before
  `AddStrand` persists the strand — see discussion "Run directory and cleanup"); dirs with
  unreadable/absent `run.json` are removed only when older than `minAge`. Pure signature
  (guids+clock injected) so tests need no mux. `posix.go`: exported `PosixPath(p string) (string,
  error)` — convert an absolute Windows path (`C:\a b\c`) to git-bash POSIX form
  (`/c/a b/c`); forward-slash input tolerated; error on non-drive-rooted input (UNC,
  relative) with a message naming the path. Exported because engine implementations
  (claudeengine) embed the converted path into hook commands. Tests: sweep age guard (young orphan kept,
  old orphan removed, live-guid dir kept), findRunByStrand hit/miss, posixPath table
  (drive root, spaces, forward slashes, UNC error, relative error).
- **Commit:** `feat(shuttle): run-dir lifecycle, run.json state, age-guarded orphan sweep`

## Batch Tests

`verify: go test ./internal/shuttleengine/... ./internal/configreg/...` — covers the new
config tests (template defaults, env override, not-initialized), configreg registration,
spec validation table, run-dir state round-trips, the orphan-sweep age guard, and the
posixPath table. All hermetic — the sweep takes injected guids and clock; nothing here
touches psmux, claude, or mux state.
