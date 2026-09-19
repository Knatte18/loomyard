MILL_REVIEW_BEGIN
# Review: reed: born-as-strand for loom start's operator attach

```yaml
duration_s: 163.0
verdict: REQUEST_CHANGES
reviewer_model: opus
reviewer_self_id: claude-opus-4.x class (self-assessed; brief dictates "opusmedium")
reviewed_file: _mill/discussion.md
date: 2026-09-19
```

## Findings

### [BLOCKING:design] "matches Selvage exactly" is a false premise
**Section:** `### shell-not-agent`
**Issue:** Selvage gets `e.cfg.Shell` as `split-window`'s trailing *pane command* (`lifecycle.go:521`, pinned by `lifecycle_test.go:545`), but a Strand's `Cmd` is typed into an already-running pane shell via `send-keys -l` + `Enter` (`spawn.go:191-200`), so `Cmd: Shell` yields a *nested* shell, not Selvage's shape — two `exit`s to close, and a different process tree than the rationale claims.
**Fix:** State the disposition explicitly — either accept the nested shell and say so, or use an empty `Cmd` (send-keys of "" + Enter is a no-op and leaves the pane's own default shell), and drop the "behaviour matches Selvage exactly" rationale either way.

### [BLOCKING:design] Focus:true is persisted and steals focus all run
**Section:** `### display-below-parent-focused-no-shrink`
**Issue:** `Display.Focus` is stored on the strand, and `focusTarget` (`render/focus.go:15`) re-runs on every `applyLayoutLocked`, reached from `reconcileApplyPersistLocked` on every `AddStrand` — agent strands carry `Focus:false` (`shuttleengine/run.go:252`), so the operator pane wins focus again on *every* agent-pane spawn for the whole run, not just at attach.
**Fix:** Decide and record whether that ongoing focus capture is intended, or whether focus should be a one-shot at bootstrap (e.g. `Focus:false` persisted plus an explicit select at attach time).

### [NIT:scope] "beside the substrate step" is ambiguous for `loom step`
**Demoted-from:** BLOCKING
**Section:** `### watchdog-parity` (gating) / Technical context
**Issue:** The named substrate step is `ensureStatusStrand`, which lives in `internal/loomcli/sharedbootstrap.go` and is called by **both** `start` and `step`; Scope declares `lyx loom step` out, so "beside the substrate step" leaves two incompatible readings (watchdog for `step` too, or `start` only) — and the stated rationale ("a daemon is for a session that exists") argues for `step` as well.
**Fix:** Name the call site literally — `start.go`'s RunE only, or `sharedbootstrap.go` covering both — and reconcile it with the Scope "Out" line.

### [NIT:consistency] `--no-attach` watchdog smoke assertion is unobservable
**Demoted-from:** BLOCKING
**Section:** Testing, smoke bullet 4
**Issue:** `suppressWatchdogSpawn` is initialised from `testing.Testing()` per the `watchdog-suppression-is-passed-in-not-inferred` decision, and Live-Substrate Spawn Observability bars re-exec'ing `os.Executable()` under `go test`, so no Go test can observe "`--no-attach` still spawns the watchdog"; the nearest precedent (`reedcli/watchdog_integration_test.go:620`) only asserts a *failing* spawn against an unusable hub.
**Fix:** Restate that assertion as what is actually checkable in-process (the seam is reached with the expected hub/tmux arguments, or the suppressed no-op), or move it to the sandbox suite driving the deployed binary.

### [NIT:consistency] Hub-path derivation sentence is muddled
**Section:** Technical context, "Watchdog extraction"
**Issue:** "`fabricengine.HubScratchDir` is what the function derives its scratch dir from, so the hub path is reachable" does not name a source; the actual value is `c.location.HubPath`, the same field `reedcli` stores (`reedcli/cli.go:115`).
**Fix:** Say `c.location.HubPath` outright.

## Verdict

REQUEST_CHANGES
Two rationales rest on unverified engine behaviour; watchdog placement and one test assertion unresolved.
_Note: 2 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 2._
MILL_REVIEW_END
