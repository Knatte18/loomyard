# shuttle — round 4 fixer report (`opus-high-r4`)

Three findings recorded in `_mill/shuttle-review-r4.md`, **all three fixed**, one commit each, nothing deferred.
No BLOCKING finding. R2-F11 is closed rather than re-deferred.

| Finding | Severity | Commit | Regression test | Sabotage-proved | Live re-driven |
| --- | --- | --- | --- | --- | --- |
| R4-F1 (closes R2-F11) | MEDIUM | `2fd63e29` | `TestRun_Send_BaselineOccurrenceEvictedAsDeliveredOneArrives_NoReplay`, `TestRun_Send_ViewportScrollsWithoutDelivery_StillReportsFailure`, `TestScanPaneForNeedle` | yes | yes — A/B against a real Claude TUI |
| R4-F2 | MEDIUM | `6fc6655f` | `TestRun_Wait_ClearedPaneBinding_IsMechanismFailureNotDied`, `TestRun_Wait_ClearedPaneBinding_OutputFilesStillWin` | yes | yes |
| R4-F3 | LOW | `88b8b52f` | `TestRun_InterruptAndSend_RefuseDeadOrUntrackedStrand/cleared_pane_binding` | yes | yes |

## R4-F1 — `sendVerified` now judges delivery by position as well as by count (commit `2fd63e29`)

**What changed.** `internal/shuttleengine/run.go`. `sendVerified` still counts occurrences of the needle over the
same normalized capture, and its `count > baseline` success and `count < baseline` re-baseline branches are
untouched — that count is the live-proven swallowed-send detector and its failure path did not move.
What is new is one additional acceptance in the branch that could not decide before: `scanPaneForNeedle` now
returns, alongside the count, how many content lines sit BELOW the last occurrence, and `deliveredBelowBaseline`
accepts delivery when the current last occurrence sits closer to the bottom (by a 2-line margin) than every
occurrence the baseline counted.
Position is recovered by carrying a line index alongside every byte of the normalized string, so a needle
straddling a wrap boundary still matches exactly as before — nothing about what counts as a match changed.

**Why it is sound in the dangerous direction.** A pane only ever appends at its bottom, so an occurrence already
on screen can move up or scroll off but never down; a copy below all the baseline's copies is therefore new.
When nothing is delivered, no copy appears anywhere, so the check cannot manufacture a success.

**Evidence.**

- Measurement: 1195 recorded frames of a live Claude TUI, two needles, four sends, idle/streaming/tool-calling
  panes. `linesBelow` never decreased at equal count except in the three frames where a copy was genuinely
  delivered (46→8, 46→9, 47→8).
- Regression test built from the two REAL recorded frames, committed as
  `internal/shuttleengine/testdata/pane-scroll-{baseline,delivered}.txt` rather than hand-written.
- **Live A/B on one pane, same text, same priming (the strongest evidence in this round):**
  a pre-fix binary built from the same tree with only this fix reverted failed **2/2** —
  `ok:false, "the send was NOT delivered"` after 11.61 s and 11.54 s, each having replayed the choreography into
  a pane that already had the message. The fixed binary succeeded **3/3** — `ok:true` in 0.43 s each, exactly one
  copy of the instruction in the pane.
  The recorded frames for all three fixed-binary sends show the count NEVER rose above the baseline of 1
  (47 → 8/9 `linesBelow` is the only thing that moved), so it was demonstrably the new position rule that
  verified them.

**Sabotage proof.** Removing the `deliveredBelowBaseline` case failed exactly
`TestRun_Send_BaselineOccurrenceEvictedAsDeliveredOneArrives_NoReplay`, with the pre-fix
"never appeared in the pane" error, and nothing else. Restored; diff against the saved fixed copy empty.

**Docs.** `sendVerified`'s doc comment now states both signals, the reproduction they close, and the residual
(a copy evicted between two 250 ms polls is invisible to any viewport-only check).
`tools/sandbox/SANDBOX-SHUTTLE-SUITE.md` gained **S6**, which drives this by hand.

## R4-F2 — a cleared pane binding is a mechanism failure, not `died` (commit `6fc6655f`)

**What changed.** `internal/shuttleengine/wait.go`. Inside `checkLivenessTick`'s `!strand.Live` branch — after the
satisfied-file-contract short-circuit, which still wins — a strand reed tracks with an EMPTY `PaneID` now returns
the new `errStrandPaneBindingCleared` sentinel instead of `OutcomeDied`, so `Wait` takes its identity-preserving
exit through the existing two-strike `statusFailures` counter. `Wait`'s failure message names the case in its own
words rather than reusing the not-tracked one.

**The one exclusion.** `--anchor hidden` runs are never given a pane, so their empty `PaneID` is normal rather than
cleared; the branch is gated on `run.spec.Display.Anchor != render.AnchorHidden` and hidden runs keep today's
classification. Pinned by the second row of the new test.

**Fixture correction this surfaced.** Three existing `died` fixtures built a `StrandStatus` with no `PaneID`, which
reed never does for a strand whose pane it still holds — they were testing the cleared-binding shape while claiming
to test a dead pane. They now carry `PaneID: "%0"`, and `liveStrandStatus`'s doc says why the pane id is
load-bearing rather than incidental.

**Live re-drive against the fixed, redeployed binary** (same construction as the review's scenario 2, fresh run):
trigger at 21:37:59.9, run exited 4.0 s later with
`ok:false … "reed held no pane binding for strand … on 2 consecutive liveness checks"`, identity triple intact,
agent still alive — where the pre-fix binary had answered `ok:true, outcome:"died"`.
Restoring the stamp made the same strand report `live:true` on the same pane again, proving the agent was alive
throughout both drives.

**Sabotage proof.** Disabling the new branch (kept compiling so the failure is the assertion, not the build) failed
exactly `…/cleared_binding_under_an_ordinary_run_is_a_mechanism_failure` showing `Outcome:died`, while the hidden
row kept passing. Restored; diff empty.

**Docs.** `wait.go`'s file header, `maxStatusRetries`, `Wait`'s doc comment and `checkLivenessTick`'s doc comment all
now state the three-way split. `SANDBOX-SHUTTLE-SUITE.md` gained **S7**.

## R4-F3 — the no-live-pane refusal no longer asserts a dead pane it cannot see (commit `88b8b52f`)

**What changed.** `internal/shuttleengine/run.go`'s `requireLiveStrand`, the guard behind `Interrupt`, `Send` and
`Inject`. A not-live strand with an empty `PaneID` now gets its own message naming the cleared binding and the
`anchor:hidden` reading — `requireLiveStrand` has no `Spec` and so cannot tell those apart, and the message owns
both rather than guessing — plus the fact that the agent may still be working in a pane reed can no longer address.
A not-live strand WITH a pane id keeps the existing message verbatim.

**Live re-drive:** `lyx shuttle interrupt` against the cleared-binding strand printed the new message.

**Sabotage proof.** Disabling the branch failed exactly the new `cleared_pane_binding` row with the old text.

## Verification summary

- `go build ./...`, `go vet` (plain + `-tags smoke`) — green after every fix.
- `go test -count=5` over `shuttleengine`, `claudeengine`, `shuttlecli`, `cmd/lyx` — green.
- Whole-repo `go test ./...` — green.
- All four live smoke tests, one at a time, by exact name: `TestSmokeShuttleRunWritesOutputAndCleans` (12.6 s),
  `TestSmokeInterruptSendContinues` (14.9 s), `TestSmokeGuardrailDeniesAgentTool` (15.0 s),
  `TestSmokeGuardrailAskingSurfacesQuestion` (11.1 s) — all `ok`.
- `./deploy-dev` re-run after every source change; every live re-drive used the redeployed binary.

## Live-substrate accounting

Every real `claude` was `--model haiku`, one at a time, never concurrent.
Eight real processes in total across review and fixing: three for focus item 1 (one discarded fixture attempt, one
for the reproduction, one for the post-fix A/B), four for focus item 2 (one incidental five-second `asking` run that
established the subpath fixture's output-file and transcript facts, plus one per scenario), and one for the post-fix
re-drive of scenario 2. That is at the top of the declared 4-5 + 3 budget and is stated rather than rounded down.
The four smoke tests spawn their own agents under the suite's existing `--model haiku` pin.

**Teardown:** `ps -eo comm | grep -cx 'tmux: server'` = **0** and zero `claude` processes of mine (tracked by argv —
`--model haiku` plus a `--settings <scratch>/…` path) after every scenario, after every smoke test, and at the end.
The frame recorder (a read-only `tmux capture-pane` loop) was stopped and confirmed gone.
`git status --porcelain` clean at the end; nothing pushed.
