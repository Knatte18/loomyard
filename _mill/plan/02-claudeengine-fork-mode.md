# Batch: claudeengine-fork-mode

```yaml
task: "Fork-based cluster review in burler"
batch: "claudeengine-fork-mode"
number: 2
cards: 3
verify: go test ./internal/shuttleengine/...
depends-on: [1]
```

## Batch Scope

claudeengine realizes fork mode end-to-end: the launch line carries
`CLAUDE_CODE_FORK_SUBAGENT=1` (card 3), the settings.json Agent hook becomes conditional
— allow unnamed fork calls, deny everything else — when the Spec requests forks
(card 4), and the provider seam gains `AuditForks`, implemented by reading the Claude
transcript layout, with the run loop attaching the audit to a done Result (card 5).
External interface for batch 4: a burler round that sets `Spec.ForkSubagents` gets a
fork-capable session and a populated `Result.ForkAudit` on done. All Claude-specific
knowledge (env flag, hook JSON, transcript paths) stays in this package per the
Provider-Seam Invariant; batch-local decision: the hook's payload match is the
compact-JSON substring `"subagent_type":"fork"` — deliberately a steering guard, not a
security boundary (the audit is the backstop), as pinned in `_mill/discussion.md`.

## Cards

### Card 3: fork env flag on the launch line

- **Context:**
  - `internal/shell/shell.go`
  - `internal/shuttleengine/spec.go`
  - `internal/shuttleengine/claudeengine/settings.go`
- **Edits:**
  - `internal/shuttleengine/claudeengine/command.go`
  - `internal/shuttleengine/claudeengine/claudeengine.go`
  - `internal/shuttleengine/claudeengine/command_test.go`
  - `internal/shuttleengine/claudeengine/prepare_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add a `forkSubagents bool` parameter to `buildLaunchCmd` in
  `command.go`. When true, wrap the fully composed launch line via
  `sh.WithEnv(forkSubagentEnvKey, "1", cmd)` where
  `const forkSubagentEnvKey = "CLAUDE_CODE_FORK_SUBAGENT"` is a new package constant
  with a doc comment: staged-rollout flag (Claude Code v2.1.117+) enabling the built-in
  fork subagent type; set inline on the launch line ONLY — the mux server env is
  deliberately scrubbed of `CLAUDE_CODE_*` at boot by `muxengine.CleanClaudeEnv`, so
  the flag must ride the pane command, and only fork-mode runs get it. `buildResumeCmd`
  gets the same treatment (a resumed fork-mode session must keep the capability), so
  give it the same parameter. Thread `spec.ForkSubagents` through both call sites in
  `Prepare` (`claudeengine.go`). Extend `command_test.go`: fork mode on → line starts
  with the shell's env prefix (assert exact composed prefix for both `shell.Pwsh()` and
  `shell.Posix()`); fork mode off → line unchanged from today's shape. Extend
  `prepare_test.go` with one case asserting a `Spec{ForkSubagents: true}` Prepare
  result's `Launch.Cmd` contains `CLAUDE_CODE_FORK_SUBAGENT` and a false one does not.
- **Commit:** `claudeengine: set CLAUDE_CODE_FORK_SUBAGENT=1 on fork-mode launch lines`

### Card 4: conditional Agent hook for fork-mode runs

- **Context:**
  - `internal/shuttleengine/spec.go`
  - `internal/shuttleengine/config.go`
- **Edits:**
  - `internal/shuttleengine/claudeengine/settings.go`
  - `internal/shuttleengine/claudeengine/settings_test.go`
  - `internal/shuttleengine/claudeengine/claudeengine.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add a `forkSubagents bool` parameter to `buildSettings` in
  `settings.go` and thread `spec.ForkSubagents` from `Prepare`. Behavior when
  `cfg.ClaudeDenyAgentTool` is true: `forkSubagents == false` keeps today's blanket
  Agent deny unchanged; `forkSubagents == true` replaces it with a conditional hook
  entry (same `Matcher: "Agent"`) whose command is
  `grep -q '"subagent_type":"fork"' || echo '<denyJSON(steerAgentNonForkDeny)>'` —
  grep reads the hook's stdin payload; a match (an unnamed-fork call's compact-JSON
  tool_input) exits 0 printing nothing, which allows the call; no match falls through
  to echoing the deny JSON. Add the new steer constant
  `steerAgentNonForkDeny = "only fork subagents may be spawned here; other agents are unavailable — do the work in this session or in your forks"`
  (must contain none of `steerTextForbiddenChars`; add it to the `init` guard's slice).
  When `cfg.ClaudeDenyAgentTool` is false, fork mode emits no Agent hook at all
  (unchanged from today). Document on the conditional entry: this is a steering guard,
  not a security boundary — the `name` parameter is NOT hook-checked (a `"name"`
  substring is indistinguishable from prompt-string content by grep); unnamed-ness is
  verified post-hoc by `AuditForks` from the parent transcript. The hook applies
  session-wide, so it also polices Agent calls attempted inside forks. Extend
  `settings_test.go`: fork mode on → the Agent entry's command contains
  `"subagent_type":"fork"` and the new steer text, and does NOT contain
  `steerAgentDeny`; fork mode off → today's blanket deny asserted unchanged; both
  denies off + fork mode on → no Agent entry.
- **Commit:** `claudeengine: conditional Agent hook allows unnamed forks in fork-mode runs`

### Card 5: AuditForks on the provider seam, transcript-based

- **Context:**
  - `internal/shuttleengine/forkaudit.go`
  - `internal/shuttleengine/spec.go`
  - `internal/muxcli/smoke_test.go`
- **Edits:**
  - `internal/shuttleengine/engine.go`
  - `internal/shuttleengine/wait.go`
  - `internal/shuttleengine/run.go`
  - `internal/shuttleengine/fakes_test.go`
  - `internal/shuttleengine/wait_test.go`
- **Creates:**
  - `internal/shuttleengine/claudeengine/audit.go`
  - `internal/shuttleengine/claudeengine/audit_test.go`
  - `internal/shuttleengine/claudeengine/testdata/fork-clean.jsonl`
  - `internal/shuttleengine/claudeengine/testdata/fork-nested-agent.jsonl`
  - `internal/shuttleengine/claudeengine/testdata/fork-mutating.jsonl`
  - `internal/shuttleengine/claudeengine/testdata/parent-spawns.jsonl`
- **Deletes:** none
- **Moves:** none
- **Requirements:** (1) Add to the `Engine` interface in `engine.go`:
  `AuditForks(sessionID, workdir string) (ForkAudit, error)` with a doc comment — reads
  the provider's on-disk record of the session's fork subagents and returns mechanical
  facts; called by the run loop only for done-classified fork-mode runs; an engine whose
  provider has no fork concept returns an error. (2) Implement it in
  `claudeengine/audit.go`: derive the project directory as
  `<user home>/.claude/projects/<encoded>` where `encoded` is `workdir` with every
  non-alphanumeric byte replaced by `-` (same encoding as `claudeProjectDir` in
  `internal/muxcli/smoke_test.go` — cite it in the doc comment; production code must
  not import test code, so re-implement the ~6-line loop). Fork transcripts are
  `<projectDir>/<sessionID>/subagents/*.jsonl`; the parent transcript is
  `<projectDir>/<sessionID>.jsonl`. Parse each fork transcript leniently (skip
  malformed lines, mirroring `ParseEvents`' posture): count `tool_use` blocks per tool
  name into `ToolCalls`; `AgentCalls` = count of tool name `Agent`; `WriteCalls` =
  count of `Write` + `Edit` + `NotebookEdit`; `BashCommands` = each Bash `tool_use`
  input's `command` string verbatim; `ReportReturned` = a final assistant message with
  non-empty text exists. Parse the parent transcript for Agent `tool_use` entries:
  `SpawnCalls` = their count, `NamedSpawns` = those whose input carries a non-empty
  `name` field. A missing `subagents/` directory yields `ForkAudit{Forks: []ForkReport{}}`
  with no error (zero forks is a legitimate finding for the caller's policy); an
  unreadable file or missing parent transcript is an error. (3) In `wait.go`, at the
  point where a done classification builds the terminal `Result`, when
  `spec.ForkSubagents` call `AuditForks(state.SessionID, layout-worktree-root)` (use
  whatever the run loop already holds for the worktree root; follow the existing field
  access pattern in the file) and attach the result to `Result.ForkAudit`; an audit
  error fails `Wait` with a wrapped error (fail-loud — a fork-mode run whose audit
  cannot be read must not classify as a clean done). (4) Update the fake engine in
  `fakes_test.go` with a configurable `AuditForks` (returning a canned `ForkAudit`),
  and add one `wait_test.go` case: fork-mode spec + done classification → Result
  carries the fake audit; non-fork spec → `Result.ForkAudit` nil and the fake's
  audit method not called. (5) Fixtures: hand-write minimal-but-realistic JSONL —
  `fork-clean.jsonl` (a few Read/Grep tool_use lines + final assistant message),
  `fork-nested-agent.jsonl` (contains one Agent tool_use), `fork-mutating.jsonl`
  (one Write tool_use and one Bash tool_use with a `git commit` command string),
  `parent-spawns.jsonl` (two Agent tool_use entries, one with `"name"` set).
  `audit_test.go` builds a fake project dir under `t.TempDir()` by copying fixtures
  into the derived layout and asserts every `ForkReport`/`ForkAudit` field, the
  missing-subagents-dir case, and the path-encoding derivation.
- **Commit:** `claudeengine: implement AuditForks over the session transcript layout`

## Batch Tests

`go test ./internal/shuttleengine/...` — command/settings/prepare tables extended for
fork mode in claudeengine, `audit_test.go` fixture suite for the transcript parsing,
and the shuttleengine `wait_test.go` attach case through the fake engine. All Tier-1:
fixtures are static files, no processes spawned.
