MILL_REVIEW_BEGIN
# Review: Shed engine adapters: SingleLLMProducer, perch, Webster

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: claude-opus-5 (Anthropic Claude, Opus tier)
reviewed_file: _mill/discussion.md
date: 2026-08-16
```

## Findings

### [BLOCKING:design] Run-id rule misses treadle's second refusal branch
**Section:** "Perch run identity — a run-id that advances only past a terminal block"
**Issue:** The decision cites `state.go:126-128` (terminal refusal) but not the adjacent `:130-132` branch — `loadOrInitState` also refuses a **non-terminal** block whose `existing.ProfileHash != hash` ("use a fresh `--run-id`"), and the adapter reuses a non-terminal `<prefix>-<N>` verbatim, so a Profile edit (a `perch.yaml` round-caps/judge-model change after a `failed` gate, exactly what an operator does mid-bounce-loop) wedges that producer permanently: every later `Call` rescans to the same non-terminal N and re-errors, with no advancement path and no operator-expressible remedy.
**Fix:** State the disposition for a hash-mismatch refusal — advance to `<prefix>-<N+1>` on it as well as on a terminal block, or name the human remedy explicitly and accept the wedge.

### [BLOCKING:design] Exit precedence silently discards completed work
**Section:** "Context cancellation — entry check, exit precedence…" + "Stale output files — archive, then respawn"
**Issue:** With no mid-run bridge for shuttle/Webster, a cancel mid-run lets the engine run to completion; the exit check then converts a genuine `Done` into the ctx error, Shed appends no history entry (`run.go:153-175`) and leaves `current_producer` put, so the next `Call` archives the freshly-written, valid output and respawns the whole session. The stated accepted consequence is only the *bound* ("up to one producer's configured timeout"), never that the work inside that window is then thrown away — and the alternative (return `Done`; Shed's own step-3 boundary check at `run.go:116` pauses cleanly on the next iteration) is not among the rejected options.
**Fix:** Decide and record which one holds — accept the discard and state it as an explicit consequence in the package doc, or pin `Done`-wins-when-the-engine-completed and reconcile it with `producer.go:28-29`'s wording.

### [NIT:design] Webster's outcome vocabulary is unexported
**Section:** "Webster adapter — `Fresh` fixed false…"
**Issue:** `outcomeDone`/`outcomeStuck`/`outcomePaused` are unexported (`internal/websterengine/outcome.go:25-27`), so mapping `RunResult.Outcome` means hardcoding `"done"`/`"stuck"`/`"paused"` literals in `shedadapters` with no compile-time link, while "no new engine surface" forbids exporting them.
**Fix:** Name the literal duplication and pin it with a test row, or state that a `default:` branch on an unrecognised outcome returns an error.

### [NIT:decision] `lyx perch pause` becomes inert for adapter-driven blocks
**Section:** "`PAUSED` with a healthy context returns an error" / "Pause reaches the adapters through `ctx` only"
**Issue:** The flag-file check is `perchcli`'s own closure (`internal/perchcli/run.go:295-298`), not engine behaviour, and the adapter replaces it with the ctx bridge — so `lyx perch pause --run-id <prefix>-N` reports success, writes a flag nothing reads, and treadle clears it at the next `Run` entry (`run.go:132`).
**Fix:** State that the shipped perch pause verb is a silent no-op against adapter-driven run dirs, alongside the "one channel, one meaning" rationale.

## Verdict

REQUEST_CHANGES
Two unhandled failure modes: a perch profile-hash wedge and discarded completed work on cancel.
MILL_REVIEW_END
