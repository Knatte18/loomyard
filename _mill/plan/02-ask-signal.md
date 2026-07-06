# Batch: ask-signal

```yaml
task: Add Effort to shuttle's run Spec
batch: ask-signal
number: 2
cards: 6
verify: go test ./internal/shuttleengine/...
depends-on: [1]
```

## Batch Scope

This batch delivers the folded-in live-`AskUserQuestion` signal: the
provider-neutral `Event` seam type (renamed from `StopEvent`, gaining a `Kind`
discriminator), `claudeengine`'s parsing of both turn-end and live-ask events, the
non-denying interactive marker hook that emits the ask line, `pollEventsTick`'s
real-time `OutcomeAsking` classification, and the test-fake + tests that exercise
it. It is one batch because the `StopEvent → Event` rename is a single compile
unit spanning `engine.go`, `events.go`, `wait.go`, and `fakes_test.go` — the
`internal/shuttleengine` package (and its `claudeengine` subpackage) does NOT
compile until every reference is migrated, so the cards are implemented in order
and the batch is verified only once at the end (do not expect intermediate
compiles). It `depends-on: [1]` solely to serialize its `docs/overview.md` edit
after batch 1's. Batch-local decisions: all in the overview's Shared Decisions
(`provider-seam containment`, the live-ask decision).

## Cards

### Card 6: Rename `StopEvent` → `Event` with a `Kind` discriminator

- **Context:**
  - `internal/shuttleengine/claudeengine/events.go`
  - `internal/shuttleengine/wait.go`
  - `internal/shuttleengine/fakes_test.go`
- **Edits:**
  - `internal/shuttleengine/engine.go`
  - `internal/shuttlecli/cli_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `engine.go`, rename the `StopEvent` type to `Event` and add:
  an `EventKind` type with provider-neutral constants `EventStop` (the existing
  turn-end signal) and `EventAsk` (the agent is asking via a live tool call); a
  `Kind EventKind` field; and a generalized message field renamed from
  `LastAssistantMessage` to `Message` (doc: for `EventStop` it is the last assistant
  message, for `EventAsk` it is the question text). Keep the `Raw []byte` field.
  Update the `Engine.ParseEvents` method signature and doc comment to return
  `[]Event` and to describe that it surfaces both turn-end and live-ask events. Use
  ONLY provider-neutral names here — no `Stop`/`AskUserQuestion` Claude marker
  strings leak into this provider-invariant file beyond the established `EventStop`
  turn-end concept. Note: this file alone will not compile until cards 7, 9, and 10
  migrate the other references — that is expected; the batch verifies at the end.
  Correction discovered mid-implementation: `internal/shuttlecli/cli_test.go`'s
  `specCapturingEngine` (a hermetic `shuttleengine.Engine` double added by batch 1)
  also implements `ParseEvents(data []byte) ([]shuttleengine.StopEvent, error)` —
  contrary to the Shared Decision's grep claim that only the two shuttleengine
  packages reference `StopEvent`. Update its signature to
  `([]shuttleengine.Event, error)` in this same card so `internal/shuttlecli` keeps
  compiling; this is a mechanical signature migration only, no behavior change.
- **Commit:** `refactor(shuttleengine): rename StopEvent to Event with a Kind field`

### Card 7: Parse both turn-end and live-ask events

- **Context:**
  - `internal/shuttleengine/engine.go`
- **Edits:**
  - `internal/shuttleengine/claudeengine/events.go`
  - `internal/shuttleengine/claudeengine/events_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `events.go`, change `ParseEvents` to return `[]shuttleengine.Event`.
  For a line whose `hook_event_name == "Stop"`, emit
  `Event{Kind: shuttleengine.EventStop, Message: <last_assistant_message>, Raw: ...}`
  (preserving today's leniency: `""` when the field is absent/not a string). For a
  line whose `hook_event_name == "PreToolUse"` AND `tool_name == "AskUserQuestion"`,
  emit `Event{Kind: shuttleengine.EventAsk, Message: <question text>, Raw: ...}`,
  where the question text is every `tool_input.questions[].question` string
  newline-joined (`""` if the shape is unexpected — stay lenient, do not error). All
  other lines are skipped exactly as today (blank, malformed JSON, missing/other
  `hook_event_name`, other `PreToolUse` tools). Keep the Claude payload-shape
  knowledge (`hook_event_name`, `tool_name`, `tool_input`, `AskUserQuestion`) entirely
  inside this file. In `events_test.go`, update existing cases to the new type/field
  names and add: a `Stop` line → `EventStop` with the message; a
  `PreToolUse`+`AskUserQuestion` line → `EventAsk` with the newline-joined question
  text; a malformed/unexpected-shape ask line → skipped or empty-message per the
  leniency contract; a `PreToolUse` line for a different tool → skipped.
- **Commit:** `feat(claudeengine): parse live AskUserQuestion events`

### Card 8: Non-denying interactive marker hook

- **Context:**
  - `internal/shuttleengine/claudeengine/claudeengine.go`
- **Edits:**
  - `internal/shuttleengine/claudeengine/settings.go`
  - `internal/shuttleengine/claudeengine/settings_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `buildSettings` (`settings.go`), restructure the `PreToolUse`
  assembly to this mutually-exclusive logic: keep the `Agent` deny when
  `cfg.ClaudeDenyAgentTool`; when `interactive`, append a `PreToolUse` hook on matcher
  `AskUserQuestion` whose command is the SAME append command the `Stop` hook uses
  (reuse the existing `stopCmd` value — `cat >> <events> && printf '\n' >> <events>`)
  and which emits NO deny JSON, so the tool proceeds; else (autonomous) keep the
  existing `AskUserQuestion` deny gated on `cfg.ClaudeDenyAskUserQuestion`. The
  interactive marker is always on (no new config key). In `settings_test.go`: an
  interactive run emits a non-denying `AskUserQuestion` `PreToolUse` hook whose command
  equals the `Stop` append command and contains no deny payload, and emits no deny
  hook; an autonomous run (with `ClaudeDenyAskUserQuestion` set) emits the deny and no
  marker; the two are mutually exclusive across the `ClaudeDenyAskUserQuestion` /
  `interactive` combinations.
- **Commit:** `feat(claudeengine): record live AskUserQuestion in interactive runs`

### Card 9: Classify a live ask as real-time `OutcomeAsking`

- **Context:**
  - `internal/shuttleengine/engine.go`
  - `internal/shuttleengine/claudeengine/events.go`
- **Edits:**
  - `internal/shuttleengine/wait.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `pollEventsTick` (`wait.go`), update it for `[]Event`,
  preserving done-first semantics with the SAME two-way branch as today — NOT a
  `Kind` switch (both non-done kinds classify identically, so a `Kind` branch here
  would be dead): if `allOutputFilesExist` → `OutcomeDone`; otherwise →
  `OutcomeAsking` with `last.Message` as the message. Because `ParseEvents` now also
  emits `EventAsk`, this unchanged branch now additionally covers the real-time
  live-ask (an `EventAsk` with no output files, classified the instant the tool call
  opens) in addition to today's `EventStop`-with-no-files case — `Kind` stays a
  parse-time discriminator (it selects the message source in `events.go`), not a
  classification input here. Wire `last.Message` (renamed from
  `LastAssistantMessage`) into the returned message so it flows to
  `Result.LastAssistantMessage`. Do not add a new `Outcome` value.
- **Commit:** `feat(shuttleengine): classify live asks as real-time asking`

### Card 10: Test fake emits both kinds; real-time asking test

- **Context:**
  - `internal/shuttleengine/engine.go`
  - `internal/shuttleengine/wait.go`
  - `internal/shuttleengine/claudeengine/events.go`
- **Edits:**
  - `internal/shuttleengine/fakes_test.go`
  - `internal/shuttleengine/wait_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `fakes_test.go`, change `fakeEngine.ParseEvents` to return
  `[]Event` and extend its scripted line format so a fixture can synthesize an
  `EventAsk` (question text) as well as an `EventStop` (turn-end message) — e.g. a
  per-line kind marker the fake recognizes — updating the doc comment to match. In
  `wait_test.go`, migrate any renamed type/field references and add a real-time
  asking case: a poll whose newest event is an `EventAsk` with no output files present
  classifies `OutcomeAsking` carrying the question as the message, and the pane/run
  dir are kept (asking ≠ done); keep a case proving done-first still wins when output
  files exist.
- **Commit:** `test(shuttleengine): cover real-time live-ask classification`

### Card 11: Document the real-time asking signal in the overview

- **Context:**
  - `internal/shuttleengine/wait.go`
  - `internal/shuttleengine/claudeengine/settings.go`
- **Edits:**
  - `docs/overview.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In the **shuttle** module entry of `docs/overview.md`, add a short
  phrase noting that an interactive run now also detects a live `AskUserQuestion`
  tool call in real time (a non-denying marker hook), classified as the same `asking`
  outcome instead of waiting for the timeout — extending the existing `asking`
  escalation-channel sentence. One line; do not restate the whole entry.
- **Commit:** `docs(overview): note the real-time live-ask asking signal`

## Batch Tests

`verify: go test ./internal/shuttleengine/...` covers both packages this batch
touches — `shuttleengine` (`wait_test.go`, plus the `engine.go`/`wait.go`/
`fakes_test.go` compile) and its `claudeengine` subpackage (`events_test.go`,
`settings_test.go`). Native Go runner, no `PYTHONPATH=` prefix (Go repo).
Correction discovered mid-implementation: the `StopEvent → Event` rename actually
has one reference outside these two packages —
`internal/shuttlecli/cli_test.go`'s `specCapturingEngine` double (added by batch
1) — migrated in card 6 alongside `engine.go` so `internal/shuttlecli` keeps
compiling even though this batch's `verify:` does not itself re-run its suite.
The package compiles (and thus the suite runs) only after every card in the
batch is implemented.
