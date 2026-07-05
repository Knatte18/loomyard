# Batch: smoke-tests

```yaml
task: 'Build internal/shuttle: one LLM agent via a swappable engine'
batch: smoke-tests
number: 6
cards: 3
verify: go vet -tags smoke ./internal/shuttlecli/... && go test ./internal/shuttleengine/... ./internal/shuttlecli/...
depends-on: [5]
```

## Batch Scope

The live-integration proof layer: opt-in `-tags smoke` tests that run a REAL claude in a
REAL psmux pane (subscription-consuming, skipped when `claude` is absent), following the
`internal/muxcli/smoke_*.go` conventions — `//go:build smoke` tag, fixture via
`lyxtest.CopyPaired` + `lyxtest.SeedConfig` (seeding BOTH `shuttle` and `mux` templates),
`deferHubRelease`-style teardown for the orphaned-conhost hazard, and generous
saturation-tolerant deadlines. This batch closes the discussion's open item: the
deny-and-steer guardrail path is unprobed until card 23 proves it.

## Cards

### Card 22: smoke — full Run end-to-end

- **Context:**
  - `internal/muxcli/smoke_test.go`
  - `internal/muxcli/smoke_resume_test.go`
  - `internal/muxcli/smoke_lifecycle_test.go`
  - `internal/shuttleengine/run.go`
  - `internal/shuttleengine/wait.go`
  - `internal/shuttlecli/cli.go`
- **Edits:** none
- **Creates:**
  - `internal/shuttlecli/smoke_run_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** `//go:build smoke`. `TestSmokeShuttleRunWritesOutputAndCleans`:
  paired fixture, seed `shuttle` + `mux` configs, `lyx mux up` via `muxcli.RunCLI` (the
  session must exist before AddStrand), then drive ONE full run through
  `shuttlecli.RunCLI` (`shuttle run --prompt <one-liner instructing: write exactly DONE
  to <abs output path> then stop> --output-file <path> --timeout 5m`). Assert: exit 0;
  envelope `outcome == "done"`; the output file exists with the expected content; the
  strand is gone from `lyx mux status` output; the run dir under `.lyx/shuttle/` is
  removed. Copy the small private helpers the muxcli smoke files use where needed
  (pane capture, hub release) rather than exporting them — smoke files are
  self-contained by convention. Skip via `testing.Short()`-style guard when no `claude`
  binary resolves (same `claudeBinaryPath` pattern as muxcli).
- **Commit:** `test(shuttle): smoke — full run end-to-end (done + cleanup)`

### Card 23: smoke — guardrail deny-and-steer proof

- **Context:**
  - `internal/shuttlecli/smoke_run_test.go`
  - `internal/shuttleengine/claudeengine/settings.go`
  - `docs/research/mux-hooks-exploration.md`
- **Edits:** none
- **Creates:**
  - `internal/shuttlecli/smoke_guardrail_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** `//go:build smoke`. `TestSmokeGuardrailDeniesAgentTool`: autonomous
  run whose prompt explicitly instructs the agent to dispatch a subagent via its Agent
  tool to write the output file, and only write the file itself if the Agent tool is
  unavailable. Assert the run still ends `outcome == "done"` with the file written —
  proof the PreToolUse deny fired AND the steer redirected the work in-session (this
  closes the "deny-and-steer unprobed" open item from the hooks research; state that in
  the test's doc comment). `TestSmokeGuardrailAskingSurfacesQuestion`: autonomous run
  whose prompt instructs the agent to ask the operator a question before writing
  anything. Assert `outcome == "asking"` and `lastAssistantMessage` is non-empty, the
  strand is still live in `lyx mux status`, and the run dir persists.
- **Commit:** `test(shuttle): smoke — guardrail deny-and-steer and asking outcome`

### Card 24: smoke — interrupt and send

- **Context:**
  - `internal/shuttlecli/smoke_run_test.go`
  - `internal/shuttleengine/run.go`
- **Edits:** none
- **Creates:**
  - `internal/shuttlecli/smoke_interrupt_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** `//go:build smoke`. `TestSmokeInterruptSendContinues`: start a run
  IN-PROCESS via `shuttleengine.NewRunner(...).Start(spec)` (not the CLI — the test
  needs the handle while Wait blocks in a goroutine) with a prompt that instructs a slow
  multi-step task writing the output file at the end. After the startup window, call
  `run.Interrupt()`, then `run.Send("<one-line replacement instruction: write exactly
  REDIRECTED to the output file and stop>")`. Assert Wait returns `done` and the output
  file contains the redirected content — proof the ESC-hold left the session warm and
  the follow-up line drove it (the discussion's core interrupt use case:
  stop-update-continue).
- **Commit:** `test(shuttle): smoke — interrupt + one-line redirect continues the run`

## Batch Tests

`verify: go vet -tags smoke ./internal/shuttlecli/... && go test ./internal/shuttleengine/...
./internal/shuttlecli/...` — vet type-checks the smoke files (they must always compile)
without running them; the hermetic suites re-run to catch regressions from any exported
helper added for smoke. The smoke tests themselves are opt-in (`go test -tags smoke
./internal/shuttlecli/`) because each consumes a real subscription session (~1–5 min
each) — same posture as `TestSmokeClaudeResumeRecallsCodeword`.
