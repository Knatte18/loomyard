# Batch: claude-engine

```yaml
task: 'Build internal/shuttle: one LLM agent via a swappable engine'
batch: claude-engine
number: 3
cards: 5
verify: go test ./internal/shuttleengine/...
depends-on: [2]
```

## Batch Scope

Define the provider seam (`shuttleengine.Engine` interface + its value types) and deliver
its only v1 implementation, `internal/shuttleengine/claudeengine`: launch/resume command
composition, `--settings` JSON (Stop hook + PreToolUse guardrails), Stop-event parsing,
and startup/trust-screen classification. Everything in this batch is pure functions over
strings/bytes — no psmux, no processes — which is exactly what makes the seam hermetically
testable. External interface consumed by batch 4: `Engine`, `Launch`, `PaneInput`,
`StopEvent`, `StartupState`, `Outcome`, and `claudeengine.New`.

## Cards

### Card 9: the Engine seam types and import-rule enforcement

- **Context:**
  - `internal/shuttleengine/spec.go`
  - `internal/shuttleengine/config.go`
  - `internal/lyxtest/leaf_enforcement_test.go`
- **Edits:** none
- **Creates:**
  - `internal/shuttleengine/engine.go`
  - `internal/shuttleengine/seam_enforcement_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `engine.go` define, with full godoc: `type Outcome string` with
  constants `OutcomeDone("done")`, `OutcomeAsking("asking")`, `OutcomeDied("died")`,
  `OutcomeTimeout("timeout")`; `type Launch struct { Cmd, ResumeCmd, SessionID string }`;
  `type PaneInput struct { Key string; Text string; Submit bool }` (exactly one of
  Key/Text set — godoc the contract; Key is a psmux named key like `Escape`, Text is
  literal text, Submit appends Enter after Text); `type StopEvent struct {
  LastAssistantMessage string; Raw []byte }`; `type StartupState int` with constants
  `StartupPending`, `StartupReady`, `StartupTrustPrompt`; and
  `type Engine interface { Prepare(runDir string, spec Spec, cfg Config) (Launch, error);
  ParseEvents(data []byte) ([]StopEvent, error); Startup(capture string) StartupState;
  InterruptSequence() []PaneInput; ComposeSend(text string) []PaneInput }` — godoc each
  method's contract (Prepare writes the run dir's prompt/settings artifacts and returns
  the opaque command strings; ParseEvents is lenient over the events JSONL; Startup
  classifies a pane capture during the startup window; the two sequence methods return
  provider-specific key choreography). `seam_enforcement_test.go`: parse every
  non-`_test.go` file in the package with `go/parser` and fail if any imports
  `internal/shuttleengine/claudeengine` (style of `lyxtest/leaf_enforcement_test.go`);
  the failure message cites the Shared Decision "provider-seam import rule".
- **Commit:** `feat(shuttle): provider Engine seam types and import-rule enforcement test`

### Card 10: claudeengine package and launch/resume command composition

- **Context:**
  - `internal/shuttleengine/engine.go`
  - `internal/shuttleengine/config.go`
  - `internal/muxcli/smoke_resume_test.go`
- **Edits:** none
- **Creates:**
  - `internal/shuttleengine/claudeengine/doc.go`
  - `internal/shuttleengine/claudeengine/claudeengine.go`
  - `internal/shuttleengine/claudeengine/command.go`
  - `internal/shuttleengine/claudeengine/command_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** `doc.go`: package header — the Claude adapter behind
  `shuttleengine.Engine`; all Claude-specific knowledge (CLI flags, hook schema, TUI
  markers, key choreography) lives here and nowhere else. `claudeengine.go`:
  `type Claude struct{}` and `func New() *Claude`; compile-time assertion
  `var _ shuttleengine.Engine = (*Claude)(nil)`. `command.go`: unexported helpers used by
  `Prepare` (card 11): `pwshSingleQuote(s string) string` (wrap in `'…'`, double embedded
  `'`); `claudeBinary(cfg shuttleengine.Config) string` — `cfg.Claude` when non-empty,
  else the literal `claude`; `buildLaunchCmd(bin, promptPath, settingsPath, sessionID,
  model string, interactive bool) string` producing exactly
  `& '<bin>' (Get-Content -Raw '<promptPath>') --session-id <id> --settings
  '<settingsPath>'` plus ` --model <model>` when model non-empty plus
  ` --dangerously-skip-permissions` when NOT interactive — one line, no newlines
  (the string is typed into a pwsh pane via send-keys; see muxengine `spawn.go`);
  `buildResumeCmd(bin, settingsPath, sessionID string) string` producing
  `& '<bin>' --resume <id> --settings '<settingsPath>'` (never `--continue` — ambiguous
  under concurrent runs, per discussion "Session identity"). `command_test.go`: table
  tests over quoting (paths with spaces and single quotes), model/flag presence per
  interactive, exact resume shape, and a no-newline assertion on every produced command.
- **Commit:** `feat(shuttle): claudeengine launch/resume command composition`

### Card 11: settings.json composition (Stop hook + guardrails) and Prepare

- **Context:**
  - `internal/shuttleengine/engine.go`
  - `internal/shuttleengine/spec.go`
  - `internal/shuttleengine/rundir.go`
  - `internal/shuttleengine/posix.go`
  - `docs/research/mux-hooks-exploration.md`
- **Edits:**
  - `internal/shuttleengine/claudeengine/claudeengine.go`
- **Creates:**
  - `internal/shuttleengine/claudeengine/settings.go`
  - `internal/shuttleengine/claudeengine/settings_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** `settings.go`: `buildSettings(eventsPathPosix string, interactive
  bool, cfg shuttleengine.Config) ([]byte, error)` — marshal (encoding/json, stable
  field order via structs) a Claude Code settings document with: a `Stop` hook entry whose
  command is `cat >> '<eventsPathPosix>' && printf '\n' >> '<eventsPathPosix>'` (hook
  commands run under git-bash on Windows — POSIX path only, single quotes; the printf
  guarantees line separation for JSONL parsing); a `PreToolUse` entry with matcher
  `Agent` present when `cfg.ClaudeDenyAgentTool`, whose command `echo`s the deny JSON
  `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny",
  "permissionDecisionReason":"<steer>"}}` with steer: do the work in this session; nested
  agents are not available here — all work must stay visible in this pane; and a
  `PreToolUse` entry with matcher `AskUserQuestion` present only when
  `cfg.ClaudeDenyAskUserQuestion` AND NOT `interactive`, steer: you cannot open an
  interactive dialog here. If you are blocked or need operator input, state the question
  as your final message and end your turn WITHOUT writing the result file. Steer texts
  must contain no single quotes (they ride inside single-quoted `echo` under git-bash) —
  add a unit assertion for that. Implement `func (c *Claude) Prepare(runDir string, spec
  shuttleengine.Spec, cfg shuttleengine.Config) (shuttleengine.Launch, error)`: generate
  the session UUID (crypto/rand, RFC-4122-v4 formatting), write `<runDir>/prompt.md`
  (spec.Prompt verbatim, 0o644), convert `<runDir>/events.jsonl` to POSIX via
  `shuttleengine.PosixPath` (exported by card 8), write
  `<runDir>/settings.json`, and return `Launch` from card 10's builders. `settings_test.go`:
  golden-ish assertions on the marshaled JSON for all four toggle combinations
  (agent-deny on/off × askuser-deny on/off) and interactive-vs-autonomous, POSIX path
  embedding, and the no-single-quote steer rule; a `Prepare` test against `t.TempDir()`
  asserting the three artifacts land and `Launch` fields are consistent.
- **Commit:** `feat(shuttle): claudeengine settings composition and Prepare`

### Card 12: Stop-event parsing

- **Context:**
  - `internal/shuttleengine/engine.go`
  - `docs/research/mux-hooks-exploration.md`
- **Edits:** none
- **Creates:**
  - `internal/shuttleengine/claudeengine/events.go`
  - `internal/shuttleengine/claudeengine/events_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** `func (c *Claude) ParseEvents(data []byte) ([]shuttleengine.StopEvent,
  error)` — split on newlines, skip blank lines, `json.Unmarshal` each into
  `map[string]any`; LENIENT: a malformed line is skipped, never fatal (partial appends
  happen; unknown fields are expected across claude versions — discussion "Claude CLI
  version note"); a line whose `hook_event_name` is present and ≠ `Stop` is skipped;
  lines without `hook_event_name` are skipped; matched lines yield
  `StopEvent{LastAssistantMessage: <string field last_assistant_message, "" when absent>,
  Raw: <line bytes>}`. Tests: fixture JSONL with two Stop events, an interleaved garbage
  line, a non-Stop event, a blank line — assert two events with correct messages and Raw
  round-trip.
- **Commit:** `feat(shuttle): claudeengine lenient Stop-event parsing`

### Card 13: startup and trust-screen classification

- **Context:**
  - `internal/shuttleengine/engine.go`
  - `internal/muxcli/smoke_resume_test.go`
  - `docs/research/mux-hooks-exploration.md`
- **Edits:** none
- **Creates:**
  - `internal/shuttleengine/claudeengine/startup.go`
  - `internal/shuttleengine/claudeengine/startup_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** `func (c *Claude) Startup(capture string) shuttleengine.StartupState`:
  return `StartupTrustPrompt` when the capture contains both `trust` and `folder`
  (case-insensitive — the exact heuristic proven in muxcli's `dismissTrust`);
  `StartupReady` when it contains the Claude TUI input marker `❯` or the ASCII status
  token `shortcuts` (the idle-key marker from the mux research — robust across the
  non-ASCII-space quirk); else `StartupPending`. Also implement
  `func (c *Claude) InterruptSequence() []shuttleengine.PaneInput` returning
  `[{Key: "Escape"}]` and `func (c *Claude) ComposeSend(text string)
  []shuttleengine.PaneInput` returning `[{Key: "Escape"}, {Text: text, Submit: true}]`
  (Esc first clears leaked auto-suggest — empirical rule from the mux research; reuse
  turns are single-line). Tests: classification table (trust screen fixture, ready
  fixture with `❯`, ready fixture with `shortcuts`, cold/pending fixture), sequence
  shapes.
- **Commit:** `feat(shuttle): claudeengine startup/trust classification and key sequences`

## Batch Tests

`verify: go test ./internal/shuttleengine/...` — runs the seam-enforcement import scan,
the command-composition tables, the settings-JSON toggle matrix (incl. the POSIX-path and
no-single-quote invariants), the lenient event-parsing fixtures, and the startup
classification table. All pure; no process is spawned. The unprobed deny-and-steer
behaviour is deliberately NOT claimed here — batch 6's smoke test is its live proof.
