MILL_REVIEW_BEGIN
# Review: Reed attach dot-fill render artifact on resize and cross-client mouse move

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-31
```

## Findings

### [BLOCKING:consistency] "Array never installed on Windows" is false
**Section:** Scope/Out bullet 3, Technical context "Windows", Constraints bullet 3
**Issue:** `installResizePinsLocked` (windowsize.go:303) has no `runtime.GOOS` gate and issues the clear plus every `resize-pane` pin argv on Windows too; only the *signal entry* is suppressed there, by `resizeSignalHookCommand` returning `""`. `pinGeometryOptionsLocked`'s early return covers the unset half only.
**Fix:** Restate the premise correctly and say explicitly which mechanism excludes the repaint entry on Windows (a `""`-returning builder like `resizeSignalHookCommand`), since there is no "unchanged inheritance" to rely on.

### [BLOCKING:design] Repaint entry's disposition under `watchdog: off` unstated
**Section:** `repaint-mechanism`; Constraints "discovered during discussion"
**Issue:** The discussion only says a zero-pin rebuild "with the watchdog on" installs both entries. With `watchdog: off`, `pinGeometryOptionsLocked` unsets the whole array on every attach while `installResizePinsLocked` still rebuilds it later in the same `AttachArgv` closure (attach.go:87 then :144) — so whether repaint survives the kill-switch is undecided.
**Fix:** State whether the repaint entry is gated on `watchdogOption` like the signal entry, or independent of it, and reconcile that with the unconditional unset.

### [BLOCKING:design] Candidate 1's hook body has no specified composition
**Section:** `repaint-mechanism` candidate 1; Technical context "Hook body quoting"
**Issue:** A `run-shell` fragment that enumerates and refreshes clients must embed the tmux binary path, reed's `-L` socket, and a shell loop; `resizeHookCommand` establishes none of that (it wraps `sh.Touch(path)` only), and `shell.Shell` exposes only `Quote`/`Invoke`/`ReadFile`/`WithEnv`/`Touch`. The Shell Mechanics Seam constraint is absent from the Constraints section entirely.
**Fix:** Name where the binary path and socket come from inside the body, and state that any new fragment primitive is added in `internal/shell` under the Shell Mechanics Seam.

### [BLOCKING:design] Regression test has no negative control or assertion predicate
**Section:** Testing, `internal/reedcli` smoke scenarios
**Issue:** "Reproduce first, then be free of it" is a one-time manual property: once the repaint entry lands, the shipped test only ever asserts absence and passes vacuously on any machine where the timing-dependent artifact never appears. The dot predicate is also unspecified — `pollPaneContains` takes a substring, and harness pane content legitimately contains dots.
**Fix:** Specify the durable negative control (e.g. a scenario that installs the array without the repaint entry and asserts the artifact appears) and the exact dot-run predicate the assertion uses.

### [NIT:consistency] Harness primitives cited to the wrong file
**Section:** `test-vehicle-is-harness-in-harness` rationale
**Issue:** `tmuxBinaryPath`, `harnessShellBinaryPath`, `buildLyxBinary`, `sendKeysLine`, `pollPaneContains`, `reapHarnessServer` all live in `internal/reedcli/smoke_test.go`, not `smoke_attach_test.go`.
**Fix:** Cite `smoke_test.go` for the primitives and `smoke_attach_test.go` for the harness-in-harness pattern.

### [NIT:consistency] `list-clients` capability stated as a hypothetical
**Section:** Technical context "Windows"
**Issue:** "if it turns out not to be in `requiredSubcommands`" — it is not; `probe.go:32-47` lists fifteen verbs and `list-clients` is not among them (nor is `refresh-client`).
**Fix:** State the degrade-to-no-warning rule as a decision, not a conditional.

## Verdict

REQUEST_CHANGES
Four blocking gaps: a false Windows premise, watchdog-off gating, hook-body composition, and test negative control.
MILL_REVIEW_END
