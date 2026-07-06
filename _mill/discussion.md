# Discussion: Add Effort to shuttle's run Spec

```yaml
task: Add Effort to shuttle's run Spec
slug: shuttle-spec-effort
status: discussing
parent: main
```

## Problem

Orchestration wants to pick *model* and *reasoning effort* independently, per
run — a cheap judge at low effort, a strong handler at high effort, on the same
axis. `shuttleengine.Spec` ships with `Model` but has no effort knob, so every
run today gets the provider's default effort with no way to override it.

Separately, a real gap surfaced while manually driving shuttle's
operator-assisted-attach scenario during a review loop: an **interactive** run
that blocks inside a live `AskUserQuestion` tool call is today indistinguishable
from a run that is "still silently working." Claude Code fires no `Stop` hook
while a tool call is open, so shuttle's poll loop sees *no event at all* until
the whole run eventually finishes or times out. The autonomous path denies
`AskUserQuestion` (steering the model to end its turn with the question, which
produces a normal `Stop`), but the interactive path has no observer on it. This
task folds in a fix: give the run loop a genuine third real-time signal for
"the agent is asking a question right now."

**Why now:** both are prerequisites for the orchestration layer (loom/review)
that drives shuttle — it needs to choose effort per run, and it needs to detect
an interactive run that has stalled on a question so it can route to a human
instead of waiting out the timeout.

## Scope

**In:**

- `shuttleengine.Spec.Effort string` — a new per-run field; empty = provider
  default. `Spec.validate` does **not** validate it (effort *values* are
  provider vocabulary; the engine owns validation).
- `claudeengine` realizes `Effort` via the claude CLI **`--effort` flag**,
  appended in `buildLaunchCmd` next to `--model`. Hard-error in `Prepare` on an
  unrealizable (out-of-vocabulary) value.
- `--effort` flag on `lyx shuttle run`, threaded into `Spec.Effort`.
- A **non-denying** `PreToolUse(AskUserQuestion)` hook for **interactive** runs
  that appends the hook payload to `events.jsonl` (the same `cat >>` append the
  `Stop` hook already uses), making a live question observable in real time.
- Rename the provider-seam event type `StopEvent` → `Event` with a
  provider-neutral `Kind` discriminator, so `ParseEvents` can surface both
  turn-end and live-asking events. `pollEventsTick` classifies a live-ask event
  as `OutcomeAsking` in real time (pane kept for attach).
- Tests: spec field pass-through, engine effort mapping + validation, CLI flag
  composition, settings hook shape (interactive vs autonomous), event parsing,
  and real-time asking classification.
- Docs: the shuttle entry in `docs/overview.md` and the `shuttleengine` package
  doc comments.

**Out:**

- **No `shuttle.yaml` config default for effort** (e.g. `claude_effort`). Effort
  is per-run; a config default is deferred until a concrete need appears (YAGNI,
  per the proposal).
- **No per-model effort compatibility policing.** See the decision below — the
  CLI gives shuttle no signal for it, so shuttle validates the *vocabulary* only
  and transports the value verbatim.
- **No new config key for the AskUserQuestion marker.** It is always on for
  interactive runs (pure observability).
- **No effort on `buildResumeCmd`.** Launch-only, mirroring `--model`.
- **No new `Outcome` value.** A live ask maps to the existing `OutcomeAsking`.
- **Non-Claude engines.** Nothing here touches a hypothetical second engine.

## Decisions

### effort-carrier — `--effort` CLI flag, not the `effortLevel` settings key

- Decision: `claudeengine` realizes `Spec.Effort` by appending
  `--effort <value>` to the launch command in `buildLaunchCmd`, single-quoted
  exactly like `--model`.
- Rationale: Both carriers were verified live against `claude` v2.1.200. The
  CLI flag `--effort <level>` accepts `low, medium, high, xhigh, max`. The
  settings-file key `effortLevel` accepts only `low, medium, high, xhigh` (no
  `max`). The flag (a) supports the full documented range including `max`, (b)
  is symmetric with the existing `--model` handling (same function, same
  quoting, same conditional-when-non-empty append, parallel tests), and (c)
  keeps a run's provider selectors together on the launch line.
- Rejected: `effortLevel` in the `settings.json` that `buildSettings` already
  writes — it would auto-carry to resume via `--settings`, but it drops `max`
  and breaks the "per-run provider selector is a flag" symmetry `Model` set.

### effort-validation — engine hard-errors on out-of-vocabulary values, exact lowercase

- Decision: `claudeengine.Prepare` validates `spec.Effort` **before writing any
  artifact** (alongside the existing `maxLaunchPromptBytes` guard). An empty
  value is accepted (no flag emitted). A non-empty value is accepted only if it
  is an **exact-lowercase** member of `{low, medium, high, xhigh, max}`; anything
  else is a hard error returned from `Prepare`.
- Rationale: The live probe showed claude **silently ignores** a bad `--effort`
  value — it prints only a stderr `Warning: Unknown --effort value ... ignoring
  it` and runs at default effort (exit 0). Shuttle never reads that stderr as
  structured data, so without its own validation an unrealizable value becomes
  exactly the silent ignore the task forbids. Exact-lowercase (rejecting `High`,
  `HIGH`) keeps with the task's "hard error, never silent" stance — a casing
  mistake is a caller bug worth surfacing, not something to quietly normalize.
- Rejected: case-insensitive normalization (hides caller mistakes); relying on
  claude's own validation (it warns-and-ignores, never errors).

### per-model-realizability — not policed; vocabulary-only

- Decision: Shuttle validates the effort **vocabulary** only. It does **not**
  know or check whether a given model actually supports a given effort level.
- Rationale: Effort support is model-dependent (e.g. Haiku does not support
  reasoning effort), but this is **invisible from the CLI**. Live probe:
  `claude --model haiku --effort high` and `--effort max` both returned a bare
  `ok`, exit 0, with **no warning or error at all** — claude silently accepts
  and ignores effort on a model that can't use it, emitting no signal shuttle
  could key off. The only ways to "catch" it would be a hardcoded model→effort
  matrix (which rots as models/levels change, and the task explicitly warns
  against rot-prone copied logic) or a post-hoc `Stop`-payload effort readback
  (post-hoc, run-wasting, and resting on the *unverified* assumption that the
  payload's `effort` field reflects realized vs echoed effort). Both were
  rejected in favor of the honest scope: validate what the fixed CLI grammar
  makes knowable, transport the rest verbatim, and document the limitation.
- Rejected: hardcoded per-model matrix (rots, CLI can't confirm it);
  `Stop`-hook effort readback (unverified, post-hoc, complicates outcomes).

### ask-signal-mechanism — non-denying marker hook, interactive only, reusing stopCmd

- Decision: For interactive runs (`!spec.Interactive == false`, i.e.
  `Interactive == true`), `buildSettings` adds a `PreToolUse` hook on matcher
  `AskUserQuestion` whose command is the **same** append command the `Stop` hook
  uses (`cat >> <events> && printf '\n' >> <events>`). It emits no
  `permissionDecision`, so the tool proceeds normally; it only records the
  payload. Autonomous runs are unchanged: they keep the existing deny hook
  (gated by `cfg.ClaudeDenyAskUserQuestion`). The two are mutually exclusive:

  ```
  if cfg.ClaudeDenyAgentTool:            append Agent deny            (unchanged)
  if interactive:                        append AskUserQuestion marker (new)
  else if cfg.ClaudeDenyAskUserQuestion: append AskUserQuestion deny   (unchanged)
  ```

- Rationale: The append trick is already proven for `Stop`. The payload
  self-describes (`hook_event_name`, `tool_name`), so the *same* command works
  for both hooks — no new command shape. Interactive runs already allowed
  `AskUserQuestion` (there was simply no hook on it), so adding a non-denying
  observer changes nothing for the agent; it only gains the signal. Always-on
  for interactive because it is pure observability with no downside — no config
  surface needed (YAGNI).
- Rejected: a config key to toggle the marker (speculative, no need); denying or
  altering the tool in interactive runs (would regress the interactive UX).

### ask-signal-outcome — real-time `OutcomeAsking`, no new outcome value

- Decision: `pollEventsTick` keeps done-first semantics (if every output file
  exists → `OutcomeDone`). Otherwise, when the most recent parsed event is a
  live-ask event, it classifies `OutcomeAsking` immediately (mid-turn, in real
  time) with the question text as the message. A `Stop` event with no output
  files still classifies `OutcomeAsking` as today, with `last_assistant_message`
  as the message. Both use the existing `OutcomeAsking`; `finalize` keeps the
  pane and run dir (asking ≠ done) for operator-assisted attach.
- Rationale: shuttle *always* drives runs via `Wait` (even interactive ones —
  `Interactive` only means "the agent may ask"). A live ask means "blocked on
  operator input right now," which is precisely the `asking` outcome — just
  detected the instant the tool call opens instead of at the timeout. Keeping
  the pane lets the operator/orchestrator attach and answer (the S2 scenario
  that surfaced this). A distinct new `Outcome` value has no consumer that
  branches on *how* the ask was detected, so it would be dead surface (YAGNI).
- Rejected: a new `Outcome` (e.g. `asking-live`) — no consumer; non-terminal
  observability-only — `Wait` only returns a terminal `Result`, so its caller
  still couldn't see the signal live.

### seam-shape — rename `StopEvent` → `Event{Kind, Message, Raw}`, provider-neutral Kind

- Decision: Rename the provider-seam type `shuttleengine.StopEvent` to
  `Event`, adding a `Kind` discriminator and generalizing the message field.
  Proposed shape (mill-plan may finalize identifiers):

  ```go
  type EventKind int
  const (
      EventStop EventKind = iota // provider turn-end (existing StopEvent)
      EventAsk                    // agent is asking via a live tool call
  )
  type Event struct {
      Kind    EventKind
      Message string  // Stop: last assistant message. Ask: the question text.
      Raw     []byte
  }
  ```

  `Engine.ParseEvents(data []byte) ([]Event, error)` returns both kinds.
  `claudeengine.ParseEvents` maps a `hook_event_name == "Stop"` line to
  `EventStop` (message = `last_assistant_message`), and a
  `hook_event_name == "PreToolUse"` line with `tool_name == "AskUserQuestion"`
  to `EventAsk` (message = `tool_input.questions[].question`, the entries
  newline-joined; "" if the shape is unexpected — leniency preserved). All
  other lines are skipped, as today.
- Rationale: The marker adds a non-`Stop` line to the same `events.jsonl` that
  `ParseEvents` currently skips, so the seam type must carry a discriminator.
  Renaming (vs. keeping the name `StopEvent` with a `Kind` field) keeps the type
  honest — it is no longer only about `Stop`. **Provider-seam invariant:** the
  `Kind` constant names in `shuttleengine` are provider-neutral (`EventStop` /
  `EventAsk`); the Claude tool name `"AskUserQuestion"` and the
  `hook_event_name`/`tool_input` payload shapes appear **only** inside
  `claudeengine` (matcher + parser). This satisfies the semantic half of the
  Shuttle Provider-Seam Invariant.
- Rejected: keep `StopEvent`, add a `Kind` field (name would lie); a second seam
  method `ParseAskEvents` or a separate marker file (two channels for one
  append stream — needless).

## Technical context

Files that change (all Claude specifics stay under `claudeengine`):

- `internal/shuttleengine/spec.go` — add `Effort string` to `Spec`
  (documented: empty = provider default, engine-validated). `validate` is
  **not** touched.
- `internal/shuttleengine/engine.go` — rename `StopEvent` → `Event`, add
  `EventKind` + constants, generalize the message field, update the
  `Engine.ParseEvents` doc/signature to `[]Event`.
- `internal/shuttleengine/wait.go` — `pollEventsTick` branches on `Event.Kind`;
  a live-ask event → `OutcomeAsking` in real time (done-first preserved). The
  message wired into `Result.LastAssistantMessage` comes from `Event.Message`.
- `internal/shuttleengine/claudeengine/claudeengine.go` — `Prepare` validates
  `spec.Effort` (before writing artifacts) and threads it into
  `buildLaunchCmd`.
- `internal/shuttleengine/claudeengine/command.go` — `buildLaunchCmd` gains an
  `effort` parameter; appends `--effort <val>` (single-quoted) when non-empty,
  next to the `--model` append. `buildResumeCmd` unchanged. Add the effort
  vocabulary as a package-level set/validator here (this is where the Claude
  vocabulary belongs).
- `internal/shuttleengine/claudeengine/settings.go` — `buildSettings` adds the
  interactive marker hook per the mutually-exclusive logic above (reusing
  `stopCmd`); autonomous deny logic unchanged.
- `internal/shuttleengine/claudeengine/events.go` — `ParseEvents` emits
  `EventStop` and `EventAsk`, extracting the question text for the latter.
- `internal/shuttlecli/run.go` — add the `--effort` flag (StringVar, default
  "") and set `Spec.Effort`. Consider an effort mention in the command `Long`.
- `docs/overview.md` — the shuttle module entry: note the per-run effort knob
  and the real-time live-ask asking signal.

Reference details verified live (claude v2.1.200):

- `--effort <level>`: values `low, medium, high, xhigh, max`.
- `effortLevel` settings key: values `low, medium, high, xhigh` (no `max`) —
  not used, see decision.
- Bad `--effort` value → stderr `Warning: Unknown --effort value '<x>' —
  ignoring it and using the default effort` + exit 0 (silent ignore to shuttle).
- `--effort` on Haiku → bare `ok`, exit 0, no warning (per-model support
  invisible).
- Claude Code hook payloads carry a common set `session_id, transcript_path,
  cwd, hook_event_name`; `PreToolUse` payloads add `tool_name` and `tool_input`.
  The `AskUserQuestion` tool_input has a `questions` array with a `question`
  field per entry (this session's tool schema). `events.jsonl` only ever
  contains `Stop` lines and the new `AskUserQuestion` marker lines — the
  `Agent`/`AskUserQuestion` **deny** hooks `echo` to stdout and never append to
  the events file, so there is no collision.

## Constraints

From `CONSTRAINTS.md`:

- **Shuttle Provider-Seam Invariant** (§ "Shuttle Provider-Seam Invariant").
  All Claude specifics — the `--effort`/`--model` flags, the effort vocabulary,
  the `settings.json` hook schema, `hook_event_name`/`tool_name`/`tool_input`
  payload shapes, and the `AskUserQuestion` tool name — stay inside
  `internal/shuttleengine/claudeengine`. `internal/shuttleengine` stays
  provider-invariant: the renamed `Event` type and its `Kind` constants are
  provider-neutral. The import-graph half is machine-checked by
  `seam_enforcement_test.go`; the semantic half (no Claude marker strings
  leaking) is a review obligation — the neutral `Kind` naming is exactly this.
- **CLI / Cobra Invariant** (§ "CLI / Cobra Invariant"). `--effort` is a new
  flag on the existing `run` subcommand — no new command, so the help-tree /
  registration / longlist pinned sets do not change. The `run` command already
  has `Short` + `Long`; if observable help text changes, re-read it for
  accuracy (help accuracy is a review obligation). Errors stay on the
  `output.Ok`/`output.Err` JSON envelope (a `Prepare` effort error surfaces via
  `Run` → `output.Err`, as spec-validation errors already do).
- **Documentation Lifecycle** (§ "Documentation Lifecycle" / CLAUDE.md "Task
  completion"). The overview module entry and `shuttleengine` package doc
  comments update in the same commit as the behaviour. No new cross-cutting
  invariant is introduced, so `CONSTRAINTS.md` itself does not change. This is
  not a planned roadmap milestone, so `docs/roadmap.md` is not touched.

## Testing

TDD-friendly, all hermetic (the engine is a pure function of its args; no psmux
or real claude needed):

- **`claudeengine` effort mapping** (`command_test.go`, extend
  `TestBuildLaunchCmd`): table rows for no-effort (no `--effort`), each valid
  value incl. `max`, effort with model (both flags present, correct order and
  quoting), and an effort value containing a space/quote (must stay one
  single-quoted argument — mirror the existing model-quoting row).
- **`claudeengine` effort validation** (`claudeengine.go` / a `Prepare`-level
  test): empty passes; each of `{low,medium,high,xhigh,max}` passes; an
  out-of-vocab value (`bogus`) and a wrong-case value (`High`) are hard-errored
  by `Prepare` **before** any artifact is written (assert no `prompt.md` /
  `settings.json` created).
- **`Spec` field pass-through** (`spec_test.go`): `validate` leaves `Effort`
  untouched (it neither defaults nor rejects it) — a row proving `validate`
  never validates effort.
- **CLI flag composition** (`shuttlecli` `run_test.go` / `cli_test.go`):
  `--effort high` lands in the built `Spec.Effort`; omitted flag → empty.
- **Settings hook shape** (`settings_test.go`): an interactive run emits a
  non-denying `PreToolUse(AskUserQuestion)` hook whose command equals the `Stop`
  append command (and emits **no** deny JSON); an autonomous run emits the deny
  (unchanged) and **no** marker; the mutual-exclusion holds across the
  `ClaudeDenyAskUserQuestion` combinations.
- **Event parsing** (`events_test.go`): a `Stop` line → `EventStop` with the
  message; a `PreToolUse`+`AskUserQuestion` line → `EventAsk` with the
  newline-joined question text; a malformed / unexpected-shape ask line → still
  lenient (skipped or empty message, per the existing leniency contract); other
  `PreToolUse` tool lines are skipped.
- **Real-time asking classification** (`wait_test.go`): a batch whose last event
  is an `EventAsk` (no output files present) → `OutcomeAsking` with the question
  as the message, pane kept; done-first still wins when output files exist.
- **Seam enforcement** (`seam_enforcement_test.go`): unchanged rule still passes
  after the `Event` rename (no `claudeengine` import leaks into
  `shuttleengine`).

## Q&A log

- **Q:** Scope — both features (Effort + the folded-in live AskUserQuestion
  signal) in this task, or split the fold-in out? **A:** Both in this task —
  honor the fold-in; one discussion/plan/review cycle.
- **Q:** Effort carrier — `--effort` CLI flag or the `effortLevel` settings
  key? **A:** `--effort` flag (matches `--model`, supports `max`; the settings
  key lacks `max`).
- **Q:** Should effort be on `buildResumeCmd` too, or launch-only? **A:**
  Launch-only, mirroring `--model`.
- **Q:** What should `Wait` do on a live `AskUserQuestion` marker in an
  interactive run? **A:** Classify `OutcomeAsking` in real time (keep pane for
  attach); no new outcome value.
- **Q:** Effort validation strictness/location — and how to handle the fact that
  valid efforts are **per-model** (Haiku doesn't support it)? **A:** Validate
  in `Prepare`, exact-lowercase vocabulary match, hard-error otherwise. Per-model
  support is **not** policed — the live probe proved it is invisible from the CLI
  (claude silently ignores unsupported model+effort with no signal), so shuttle
  validates the vocabulary only and transports the value verbatim.
- **Q:** Seam shape for carrying the new event kind? **A:** Rename
  `StopEvent` → `Event` with a provider-neutral `Kind` field; the Claude tool
  name and payload shapes stay inside `claudeengine`.
- **Q:** Capture the question text from the `AskUserQuestion` payload, or a bare
  marker? **A:** Capture the question text as the asking message (symmetric with
  idle-asking carrying `last_assistant_message`).
- **Q:** Gate the interactive marker hook behind config? **A:** No — always on
  for interactive runs; no new config key (pure observability).
