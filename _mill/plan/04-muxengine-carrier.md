# Batch: muxengine-carrier

```yaml
task: 'Build internal/mux: the window to the world (overlay + strands + render)'
batch: 'muxengine-carrier'
number: 4
cards: 6
verify: go test ./internal/muxengine/...
depends-on: [1, 2, 3]
```

## Batch Scope

Creates the `internal/muxengine` domain kernel's **carrier primitives**: the psmux
subprocess overlay, pure pane/size parsing, env hygiene, the per-hub server-name +
hub-path hash, the persisted `MuxState` record model + `.lyx/mux.json` persistence, config
(`mux.yaml` template + `LoadConfig` + `ConfigTemplate`), and strand-name/guid helpers.
These are the "dumb carrier" building blocks; the lifecycle operations (Add/Remove/reconcile/
apply/up/resume) that compose them come in batch 5. External interface batch 5 consumes:
`PsmuxCmd` + its typed helpers, `CleanClaudeEnv`, `ServerName`/`SessionName`/`socketName`,
`MuxState`/`Strand` + `LoadState`/`SaveState`, `Config`/`LoadConfig`/`ConfigTemplate`,
`FormatStrandName`, `newGUID`. All techniques are ported from `internal/muxpoccli` (read as
Context) but rebuilt domain-free (no Claude/`review` specifics, no `type` field).

Batch-local decisions:
- The engine's persisted `Strand` reuses `render.Display` / `render.Anchor` for the display
  fields (single source of the vocabulary); it adds the opaque carrier fields
  (`Name`, `Worktree`, `Parent`, `Cmd`, `ResumeCmd`, `SessionID`, `PaneID`, `GUID`).
- `.lyx/mux.json` and its `.lock` resolve through `hubgeometry` `(*Layout).DotLyxDir()`
  (batch 1) — no hardcoded `.lyx` literal.
- The server-spawning `new-session` is **not** routed through `PsmuxCmd` (built raw so
  `cmd.Env = CleanClaudeEnv(...)` + `proc.Detach` + `cmd.Start()` can be attached) — but
  that spawn call lives in batch 5's lifecycle; this batch provides the `CleanClaudeEnv`
  helper and the `PsmuxCmd` wrapper it will use.

## Cards

### Card 10: muxengine package doc + psmux overlay wrapper

- **Context:**
  - `internal/muxpoccli/cmd.go`
  - `internal/muxpoccli/cli.go`
- **Edits:** none
- **Creates:**
  - `internal/muxengine/doc.go`
  - `internal/muxengine/overlay.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/muxengine/doc.go`, a package doc comment (package
  `muxengine`) stating the module's role: the psmux overlay + strand bookkeeping + render
  consumer, the dumb-carrier contract (stores all strand fields, reads none semantically, no
  domain `type`), and the one-named-server-per-hub firewall. In
  `internal/muxengine/overlay.go`, port the `PsmuxCmd` wrapper from `muxpoccli/cmd.go`:
  `type PsmuxCmd struct { psmuxPath, socket string }` (or a small `Config`-free struct
  carrying the resolved psmux binary path + socket name), `func NewPsmuxCmd(psmuxPath,
  socket string) PsmuxCmd`, and methods `run(args ...string) error` and `output(args
  ...string) (string, error)` that **always prepend** `-L <socket>`. Add the typed helpers
  ported from muxpoc: `hasSession(name string) (bool, error)` (exit 1 -> `(false,nil)`;
  other errors surface), `listPanes(session string) ([]LivePane, error)`,
  `activePaneID(session string) (string, error)` (`display-message -p`), `windowSize(session
  string) (int, int, error)`, `paneIDsTopToBottom(session string) ([]string, error)`. Define
  `type LivePane struct { ID string; Dead bool; Width int; Height int }` with json tags.
  Strip all Claude/`review`-specific behavior.
- **Commit:** `feat(muxengine): add psmux overlay wrapper and package doc`

### Card 11: pure pane/size/order parsers

- **Context:**
  - `internal/muxpoccli/cmd.go`
  - `internal/muxpoccli/cmd_test.go`
  - `internal/muxengine/overlay.go`
- **Edits:** none
- **Creates:**
  - `internal/muxengine/parse.go`
  - `internal/muxengine/parse_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Port the pure parse functions from `muxpoccli/cmd.go` into
  `internal/muxengine/parse.go`: `func parsePaneList(out string) ([]LivePane, error)`
  (parses `#{pane_id} #{pane_dead} #{pane_width} #{pane_height}` lines; `dead := parts[1]
  == "1"`; empty -> `nil, nil`), `func parseWindowSize(out string) (int, int, error)`
  (splits `WxH` on `x`), `func parsePaneOrder(out string) ([]string, error)` (parses
  `#{pane_top} #{pane_id}`, sorts by top ascending, returns ids top-first). The overlay
  methods in card 10 call these. In `parse_test.go`, table-test each parser incl. the
  `pane_dead=1` row (which `remain-on-exit on` produces), empty input, and malformed lines.
- **Commit:** `feat(muxengine): pure pane/size/order parsers ported from muxpoc`

### Card 12: env hygiene — CleanClaudeEnv

- **Context:**
  - `internal/muxpoccli/state.go`
- **Edits:** none
- **Creates:**
  - `internal/muxengine/env.go`
  - `internal/muxengine/env_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/muxengine/env.go`, add the exported `func
  CleanClaudeEnv(environ []string) (clean []string, strippedKeys []string)` combining
  muxpoc's `sanitizeEnv` + `strippedEnvKeys` (`state.go:106`/`:120`) into one call: it
  returns `environ` minus every entry whose key (before `=`) is exactly `CLAUDECODE` **or**
  has prefix `CLAUDE_CODE_`, plus the list of stripped keys in environ order. This is the
  single documented chokepoint (the discussion's env-hygiene decision) and is exported so
  shuttle can reuse it. In `env_test.go`, assert exactly `CLAUDECODE` and every
  `CLAUDE_CODE_*` key (incl. `CLAUDE_CODE_CHILD_SESSION`, `CLAUDE_CODE_SESSION_ID`,
  `CLAUDE_CODE_ENTRYPOINT`, `CLAUDE_CODE_SSE_PORT`) are stripped, unrelated keys are kept
  untouched, and `strippedKeys` lists exactly the removed keys.
- **Commit:** `feat(muxengine): export CleanClaudeEnv env-hygiene helper`

### Card 13: server naming + hub-path hash + session name

- **Context:**
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/muxpoccli/state.go`
- **Edits:** none
- **Creates:**
  - `internal/muxengine/naming.go`
  - `internal/muxengine/naming_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/muxengine/naming.go`: `func ServerName(hubPath string)
  string` returns `lyx-<hub-basename>-<short-hash>` where `<hub-basename> =
  filepath.Base(hubPath)` and `<short-hash>` = first 8 hex chars of
  `sha256(abs-hub-path)` (use the cleaned/abs hub path). `func SessionName(worktreeRoot
  string) string` returns `filepath.Base(worktreeRoot)` (the worktree slug). `func
  socketName(hubPath string) string` returns the socket-safe server name (same as
  `ServerName`, guaranteed no `:`/`\`/space — sanitize like muxpoc's `socketName` regex if
  needed, but the sha8 form is already safe). Server-name construction lives here (psmux
  domain), computed from `Layout.Hub` obtained via hubgeometry. In `naming_test.go`: assert
  determinism, socket-safety (no `:`/`\`/space), that two hubs sharing a basename on
  different absolute paths produce **distinct** names, and that the hash is stable/case-path
  normalized as intended.
- **Commit:** `feat(muxengine): per-hub server name with sha256 hub-path hash`

### Card 14: MuxState record model + persistence

- **Context:**
  - `internal/muxpoccli/state.go`
  - `internal/state/state.go`
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/muxengine/render/types.go`
- **Edits:** none
- **Creates:**
  - `internal/muxengine/state.go`
  - `internal/muxengine/state_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/muxengine/state.go`, define the persisted record: `type
  Strand struct { GUID string; Name string; Worktree string; Parent string; Cmd string;
  ResumeCmd string; SessionID string; PaneID string; Display render.Display }` (json tags,
  omitempty on optional `Parent`/`ResumeCmd`/`SessionID`). Define `type MuxState struct {
  Server string; Socket string; Session string; StrippedEnv []string; Strands []Strand }`
  (a flat, GUID-keyed list — the v2 union seam; every strand self-describes its `Worktree`).
  Add persistence wrappers delegating to `internal/state`: `func LoadState(dotLyxDir string)
  (*MuxState, error)` -> `state.ReadJSON[MuxState](path, path+".lock")` with `path =
  filepath.Join(dotLyxDir, "mux.json")` (caller passes `layout.DotLyxDir()`); `!found` ->
  `(nil, nil)`; corruption surfaced. `func SaveState(dotLyxDir string, s *MuxState) error`
  -> `state.WriteJSON`. Add a mapper `func toRenderStrands(strands []Strand, liveIDs
  map[string]bool) []render.Strand` producing the render view (`GUID`, `Parent`, `Display`,
  `PaneID`, `Live = liveIDs[PaneID]`). In `state_test.go`: round-trip a `MuxState` through
  Save/Load (absent file -> `(nil,nil)`; corruption -> error); assert `toRenderStrands`
  copies display fields and sets `Live` from the live set.
- **Commit:** `feat(muxengine): MuxState record model and .lyx/mux.json persistence`

### Card 15: config (mux.yaml template + LoadConfig) + strand-name/guid helpers

- **Context:**
  - `internal/warpengine/config.go`
  - `internal/warpengine/template.go`
  - `internal/warpengine/template.yaml`
  - `internal/configengine/config.go`
  - `internal/muxpoccli/state.go`
  - `internal/lyxtest/lyxtest.go`
- **Edits:** none
- **Creates:**
  - `internal/muxengine/config.go`
  - `internal/muxengine/config_test.go`
  - `internal/muxengine/template.go`
  - `internal/muxengine/template.yaml`
  - `internal/muxengine/name.go`
  - `internal/muxengine/name_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Mirror the warpengine config pattern. In
  `internal/muxengine/template.go`: `//go:embed template.yaml` into `var configTemplate
  string` and `func ConfigTemplate() string { return configTemplate }`. In
  `internal/muxengine/template.yaml`: keys `psmux`, `pwsh`, `claude` (machine tool paths via
  `${env:VAR:-default}`; defaults `C:\Code\tools\bin\psmux.exe`,
  `C:\Code\tools\powershell7\pwsh.exe`, empty claude), `width` (220), `height` (50),
  `collapsed_strip_rows` (default 3), `top_band_rows` (default 1), `min_full_rows` (default
  3), and `strand_name` (default `<ROLE>:<ROUND>:<SHORT_GUID>`). In
  `internal/muxengine/config.go`: `type Config struct { Psmux, Pwsh, Claude string; Width,
  Height, CollapsedStripRows, TopBandRows, MinFullRows int; StrandName string }` (yaml tags
  matching the template) and `func LoadConfig(baseDir, module string) (Config, error)` via
  `configengine.Load(baseDir, module, []byte(ConfigTemplate()))` + `yaml.Unmarshal` (thread
  the `module` arg through — do not hardcode `"mux"`, mirroring warpengine), mapping
  the "not initialized" error to a `run "lyx init"` hint (copy warpengine's shape). In
  `internal/muxengine/name.go`: `func newGUID() (string, error)` (128-bit `crypto/rand`, hex)
  and `func FormatStrandName(template string, parts map[string]string) string` — a pure
  substitution over tokens `<WORKTREE> <ROLE> <ROUND> <SHORT_GUID>` (unfilled tokens -> ""),
  with a caller convention that when neither name nor role is given the name falls back to
  `<SHORT_GUID>` alone. In `config_test.go`: assert the template parses and defaults resolve
  (use `lyxtest.SeedConfig` per the lyxtest Leaf Invariant if a real config dir is needed).
  In `name_test.go`: table-test `FormatStrandName` over templates/tokens (reorder, override,
  `<SHORT_GUID>` fallback) and assert `newGUID` is unique/hex across calls.
- **Commit:** `feat(muxengine): mux.yaml config, ConfigTemplate, FormatStrandName, guid`

## Batch Tests

`verify: go test ./internal/muxengine/...` runs the carrier unit tests: parsers, env
hygiene, server naming + hub hash, `MuxState` round-trip + `toRenderStrands`, config
template resolution, and `FormatStrandName`/`newGUID`. The overlay's live psmux I/O
(`run`/`output`/`new-session`) is **not** exercised here — those are driven only under the
`smoke` build tag in batch 6, so this batch stays hermetic. `config_test.go` uses
`lyxtest.SeedConfig` for any real-config need (Leaf Invariant: the configreg->map
conversion happens at the test site).
