# shuttle — fixer report, round 3 (`opus-medium-r3`)

Companion to `_mill/shuttle-review-r3.md`. Every finding from that report, what was changed, how it was
verified, and what is deliberately unchanged.

## Summary

| finding | severity | size | status | commit |
| --- | --- | --- | --- | --- |
| R3-F1 — `Wait` classifies a live agent as `died` when reed's strand table loses the strand | MEDIUM | small | FIXED | `a2fa460a` |
| R3-F2 — the orphan sweep deletes a live run's directory when `reed.json` is absent | MEDIUM | small | FIXED | `79b08a36` |
| Sandbox suite S5 (covers both, live) | — | — | ADDED | `1c2be9bd` |
| OUT-OF-CAMPAIGN — reed re-creates a vanished told anchor path | — | — | NOT FIXED (reed is a closed campaign) | — |

No BLOCKING findings. **No NOT-FIXED-THIS-ROUND large finding** — both fixes are scoped bugfixes inside
`internal/shuttleengine`, neither reaches outside the module, and neither warrants its own design step.
R2-F11 remains named-and-unfixed from round 2 and was left untouched, per the brief.

## R3-F1 — `Wait` no longer manufactures `died` from reed's bookkeeping going away

**Changed** — `internal/shuttleengine/wait.go`:

- `checkLivenessTick` now resolves the strand through a new `strandStatusByGUID(strands, guid) (StrandStatus, bool)`
  helper instead of folding the whole table into one `live` boolean. The two negative answers now separate:
  a strand reed TRACKS whose pane is not alive still classifies `OutcomeDied` (unchanged); a strand reed does
  not track at all returns the new `errStrandNotTracked` sentinel.
- A satisfied file contract still outranks both — `allOutputFilesExist` → `OutcomeDone` on either negative
  answer, so an agent that finished and was then untracked is not reported as a failure.
- `Wait`'s give-up branch distinguishes the two error shapes: `reed status failed N times consecutively` for a
  Status that could not be RUN, and `reed did not track strand %q on N consecutive liveness checks` for the new
  case. Both take the existing `identity()` exit, so R1-F2's guid/sessionId/runDir contract is unchanged, and
  both ride the existing `statusFailures` counter, so a one-tick blip does not trip either.

**Docs changed in the same commit** — `engine.go`'s `OutcomeDied` doc now states the narrowed meaning;
`wait.go`'s file header and `Wait`'s doc comment say why the untracked case is a mechanism failure;
`docs/overview.md`'s shuttle bullet documents the observable CLI consequence.

**Regression tests** — `wait_test.go`:

- `TestRun_Wait_UntrackedStrand_IsMechanismFailureNotDied` (table, two cases): the absent-from-table case must
  error (`errors.Is(err, errStrandNotTracked)`), must name the guid, must keep the identity triple, and must
  reach no outcome; the tracked-but-not-live case must still be `OutcomeDied`. Both assert no `RemoveStrand`
  and that the run dir survives.
- `TestRun_Wait_UntrackedStrand_OutputFilesStillWin`: an untracked strand whose output files exist is `OutcomeDone`.

**Sabotage proof** — reverted `checkLivenessTick` to the single-boolean form; the new test failed exactly at the
intended assertion (`Wait() = ({Outcome:died …}, nil); want the untracked-strand mechanism error`); restored,
green again.

**Live re-verification against the real substrate** (fixed build deployed first): fresh hub, `lyx reed up`,
a run held inside one turn by three blocking `python3` waits, confirmed `live:true` and visibly mid-tool-call,
then `rm .lyx/reed.json` at 16:57:11.458. The run exited at 16:57:20.753 with

```
{"ok":false,"guid":"84c9c327…","sessionId":"5bcecdf1-…","runDir":"…",
 "error":"shuttle: reed did not track strand \"84c9c327…\" on 2 consecutive liveness checks: reed's strand table no longer holds this run's strand — … its process may still be working in its pane. Check \"lyx reed status\""}
```

and the pane still showed `⎿ $ python3 -c 'import time; time.sleep(100); print(1)' (39s)` — the agent working,
now reported honestly instead of as `ok:true, outcome:"died"`.

## R3-F2 — the orphan sweep no longer sweeps against a strand table it never read

**Changed** — `internal/shuttleengine/run.go`, `sweepOrphansOpportunistic`: an absent `reed.json`
(`LoadState` → `(nil, nil)`) now skips the sweep and logs, exactly as an unreadable one already did. The
remaining path operates on a state file that was genuinely read, so the `if st != nil` guard around the guid
collection is gone as dead weight rather than left as a silent fall-through.

The doc comment now states all three `LoadState` answers and why two of them skip, including what the sweep
destroys in the dangerous case (`events.jsonl` under an appending Stop hook; the `run.json` that is the only
map from guid back to run) and why skipping costs no cleanup.

**Regression test** — `run_test.go`'s `TestRunner_Start_SweepSkipsEntirelyOnAbsentReedState`, the sibling of
the existing `…OnReedStateReadError`: `.lyx` present, no `reed.json`, one aged run dir whose strand is not in
any table; `Start` must leave it alone.

**Sabotage proof** — restored the `if st != nil` fall-through; the new test failed at its own assertion
(`run dir was swept with no reed.json to prove it orphaned`); restored, green.

**Live re-verification** (fixed build): the same three-shape probe as the review's sweep table, at zero
live-substrate cost (the sweep precedes `AddStrand`, which fails with the session down, so no `claude` spawns).
Aged run dir + absent `reed.json` → **PRESENT** (was GONE before the fix).

**Live proof the cleanup path is NOT lost** — the specific risk round 2 declined this finding over. With the
fix in place: `lyx reed up` (writes a fresh `reed.json`, zero strands) → `lyx shuttle run` → the aged orphan
dir was **GONE**. Post-`down` collection therefore still happens, on the next run after the next `up`, which is
the next run that can succeed at all.

## Disagreement with round 2, stated plainly

Round 2 examined R3-F2's exact code path and recorded it under "Assessed and deliberately NOT recorded as
findings", reasoning that "blocking the sweep on absence would break the ordinary post-`down` cleanup path" and
that "the age guard is the correct and sufficient protection here".

I fixed it anyway. Both halves of that reasoning are answered above with evidence rather than argument: the
post-`down` path demonstrably still sweeps (measured, previous paragraph), and the age guard cannot be the
protection here because it protects a run that is STARTING — the run this sweep destroys is one that has been
running LONGER than the guard, which is the opposite end of the same axis. Flagged here and in the review
report so the orchestrator adjudicates rather than discovers.

## Gates

Run after each fix, and again at the end:

- `go build ./...` — green.
- `go vet ./internal/shuttleengine/... ./internal/shuttleengine/claudeengine/... ./internal/shuttlecli/...` — green.
- `go vet -tags smoke ./internal/shuttlecli/...` — green.
- `go test -count=5 ./internal/shuttleengine/... ./internal/shuttleengine/claudeengine/... ./internal/shuttlecli/... ./cmd/lyx/...` — all `ok`.
- `go test ./cmd/lyx/ -run TestSandboxCoverage` — green after adding S5.

Live smoke (`-tags smoke`, one at a time, by exact name, on the deployed fixed build):

| test | result |
| --- | --- |
| `TestSmokeShuttleRunWritesOutputAndCleans` | PASS (14.4 s) |
| `TestSmokeInterruptSendContinues` | FAIL on the first attempt, PASS on the re-run (19.8 s) — see below |
| `TestSmokeGuardrailDeniesAgentTool` | PASS (21.8 s) |
| `TestSmokeGuardrailAskingSurfacesQuestion` | PASS (12.0 s) |

**On the one smoke failure, honestly:** `TestSmokeInterruptSendContinues` failed its file-content poll
(`output file content = "" after 3m; want "REDIRECTED"`) on the first attempt and passed cleanly on the second
(`run.Wait outcome=done`). It is a live-LLM flake, not a regression: `run.Send` itself returned nil, meaning
shuttle delivered and verified the text in the pane — the agent simply did not act on the redirect inside the
three-minute window on that attempt. Neither fix touches `Interrupt`, `Send`, `sendVerified`, or the startup
probe. Recorded rather than quietly re-run, because a reviewer should see it.

## Teardown

Baseline recorded before any driving: `tmux: server` = 0, `pgrep -xc claude` = 4.
After every scenario and after the final smoke run: `tmux: server` = **0**, `pgrep -xc claude` = **3** — the
three remaining are pre-existing unrelated agent sessions; one of the original four ended on its own during the
round. **Zero new stray tmux servers, zero new stray `claude` processes.**

## Live-substrate spend

`--model haiku` on every real `claude`, no exceptions. Real processes spawned: 8 across the joint scenarios
(scenario 4 accounting for 2 of them, simultaneously — the round's one authorized exception, never exceeded),
plus 1 for the fixed-build re-verification, 1 short `--timeout 1s` run proving the sweep's cleanup path, and 5
smoke-test invocations (4 tests + 1 re-run), which the brief excludes from the scenario budget. Never more than
2 real `claude` processes at once, and only during scenario 4.
