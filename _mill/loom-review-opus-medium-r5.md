# `loom` — independent review, ROUND 5 (`opus-medium-r5`) — SAFETY PASS

Reviewer tag: `opus-medium-r5` (Opus 5, medium effort).
Worktree: `/home/hanf/Code/loomyard/wts/crucible-loom-glyph-hardening` (confirmed with `git rev-parse --show-toplevel`).
Branch: `crucible-loom-glyph-hardening` (confirmed with `git branch --show-current`).
HEAD at review start: `3cf37eea8`.

Clean-room: this report's findings were formed WITHOUT reading any `_mill/loom-review-*` file
(no prior round's review, no fixer report, no `loom-review-HANDOFF.md`). The design docs,
`CONSTRAINTS.md`, `CLAUDE.md` and the code were read; prior-round material was consulted only
after the findings list below was complete and committed.

## Executive summary

**This round is NOT a clean safety pass.** Round 5 found TWO BLOCKING defects that all four prior
rounds missed — the second reachable only after the first was fixed — and both were missed for the
same structural reason: it is invisible on any fixture the
`claude` CLI has already been accepted in, and every prior round reused or inherited such a
fixture. This round's own item 2 — "a FRESH fixture, not `r4-crash-hub`/`r4-drift-hub`" — is
exactly what exposed it, on the very first `lyx webster run` against a hub cloned minutes earlier.

Counts: **7 findings — 2 BLOCKING, 1 MEDIUM, 2 LOW, 2 NIT.** All CONFIRMED; none PLAUSIBLE-only.

- **R5-2 (BLOCKING)** — `claudeengine.TrustDismissSequence` sends a bare `Enter` at claude's
  trust-this-folder gate. On the shipped claude (2.1.263) that gate's caret **defaults to
  `No, exit`**, so lyx's own "dismissal" confirms the refusal and claude quits. Every agent lyx
  spawns in a directory claude has not previously been accepted in dies at startup, reported only
  as an opaque `master pane died`. Every freshly-created fabric worktree pair is such a directory.
- **R5-7 (BLOCKING)** — claude's Bypass-Permissions acceptance modal, raised on every
  `--dangerously-skip-permissions` spawn in a fresh environment, is not a gate `Startup` knows; its
  own selection caret is the `❯` ready marker, so the pane is classified READY and the startup
  deadline stops applying. The run then burns the whole `master_timeout_min` (480 minutes as
  configured) parked on a dialog and reports a timeout rather than a fast death. Found only by
  driving PAST R5-2 on a second fresh hub.
- **R5-1 (MEDIUM)** — `webstercli`'s standalone integration test boots a real tmux server through
  the `reedUp` seam and never tears it down: one orphan server per suite run, forever.
- **R5-6 (LOW)** — `planglyph.DoneChecks` silently PASSES a blocking done-check whose target the
  batched resolve answer did not cover, contradicting its own documented "passing it would be a
  false success" and diverging from `CanonicalizeHandles`' guard on the same class of boundary.
- **R5-3 (LOW)** — `websterengine.Run` drops a plan-fingerprint re-baseline persist failure when a
  validation error coincides with it; both bracket CLI verbs report the pair loudly.
- **R5-4, R5-5 (NIT)** — a self-contradicting doc comment and an unreachable error return, each
  duplicated across `webstercli` and `burlercli`'s wiring.

### High-yield-focus items — what was attempted and how far each got

1. **General adversarial sweep.** Done over the glyph surface (`planparser`, `planglyph`,
   `websterengine`'s changed files), the standalone-webster material (`shuttleengine/run.go`,
   `standalonegeom`, `standalonestate`, `webstercli`, `burlercli`, `logger/sink.go`) and loom's own
   wiring. Produced R5-1, R5-3, R5-4, R5-5, R5-6. The four prior rounds' territory held up: the
   fingerprint re-baseline sites, the drift gates, the `PendingPlan`/`ValidateDispatch` scoping, the
   handle-collapse rewrite rule, the standalone path-resolution chain and the sink redirect all read
   correct on their own terms, and the only thing found in them is R5-3's error-reporting asymmetry.
2. **Independent second hub-mode crash-kill reproduction on a FRESH fixture.** Fixture built fresh
   and confirmed real (clone + pair + Go module + glyph/handle-bearing two-card plan + `reed up`).
   The crash-kill itself was initially blocked by R5-2 and then by R5-7 — the run could never reach
   a batch to kill. With both fixed it was DELIVERED in full: begun-batch confirmed from
   state.json, both target processes confirmed alive by `ps`, a real `kill -9`, both confirmed
   dead, `outcome.yaml` confirmed absent, the orphan strand confirmed still live, and a clean
   `lyx webster run` resume to `outcome: done`. See "Live crash-kill scenario" below.
3. **What a fifth pass at a lighter tier turns up.** R5-2 — a defect on the single hottest live path
   in the module, sitting one layer below everything four prior rounds drove.

## What was tested

Appended as each command/scenario returned.

### Hermetic gates (all green, at HEAD `3cf37eea8`, before any edit)

| Command | Result |
|---|---|
| `CGO_ENABLED=1 go build ./...` | clean, no output |
| `CGO_ENABLED=1 go vet` over the 17 packages the prompt names | exit 0, no diagnostics |
| `CGO_ENABLED=1 go test -count=5` over those 17 packages + `./cmd/lyx/...` | all `ok`, exit 0 |
| `CGO_ENABLED=1 go test -tags integration ./internal/planglyph/... ./internal/planparser/... ./internal/websterengine/... ./internal/loomcli/... ./internal/loomshed/... ./internal/webstercli/... ./internal/burlercli/...` | all `ok`, exit 0 |

### Binary/PATH agreement

`which lyx` found NOTHING on this host at review start — no stale binary, no binary at all.
Deployed with `CGO_ENABLED=1 go run ./tools/deploy` → `Deployed lyx @ 3cf37eea8 (25541 KB) /home/hanf/go/bin/lyx`,
which is on `PATH`. (`lyx --version` is not a flag this CLI carries; the deploy tool's own
`@ <sha>` line is the agreement evidence.)

### Substrate-leak probe (this is where finding R5-1 came from)

Baseline `ps aux | grep -E 'tmux|claude'` after the hermetic gates showed two orphan
`tmux -L lyx-<hash8> new-session -d -s 001-<hash8> -c /tmp/TestRunCLIIn_StandalonePreRun_ReachesRunsOwnValidationGate<N>/001 ... bash`
servers, whose `-c` cwd is a `t.TempDir()` that no longer exists.

Isolated reproduction:

```
pgrep -af "tmux -L lyx-" | wc -l                       # -> 3 (2 orphans + the probe's own shell)
CGO_ENABLED=1 go test -tags integration -count=1 \
  -run 'TestRunCLIIn_StandalonePreRun_ReachesRunsOwnValidationGate' ./internal/webstercli/
# ok  github.com/Knatte18/loomyard/internal/webstercli  0.177s
pgrep -af "tmux -L lyx-"                               # -> a THIRD live server, socket lyx-dc9e259e
```

Every socket under `/tmp/tmux-1000/` was then classified live-vs-dead with `tmux -L <n> ls`:
27 sockets, of which exactly the three from this one test were LIVE. Every other lyx test that
boots a tmux server (`lyx-001-*`, `lyx-contract-*`, `lyx-TestDeadHeaderPane*`,
`lyx-TestRemoveStrand_*`) had torn its server down and left only a dead socket file behind.

`internal/burlercli`'s same-named integration test does NOT leak: `burlercli`'s `run` verb
refuses on the missing `--profile` flag *before* `c.reedUp()` is reached
(`internal/burlercli/run.go:120-127` runs the flag check ahead of everything), so no server boots.

### Live hub-mode fixture (fresh, per the round's own item 2)

A FRESH fixture was built rather than reusing `r4-crash-hub`/`r4-drift-hub`
(`gh api repos/Knatte18/lyx-test/branches` confirmed both still exist upstream; neither was
reused, and no local clone of either was trusted):

```
mkdir -p /home/hanf/Code/r5sandbox && cd /home/hanf/Code/r5sandbox
lyx fabric clone https://github.com/Knatte18/lyx-test-weft
#   -> hub /home/hanf/Code/r5sandbox/lyx-test-HUB, warp https://github.com/Knatte18/lyx-test
cd /home/hanf/Code/r5sandbox/lyx-test-HUB/lyx-test && lyx fabric add r5-crash
#   -> pair r5-crash / r5-crash-weft, both branches pushed
```

The fixture worktree was seeded with a real Go module (`go.mod`, `internal/greet/greet.go`
declaring `Hello`) and a real two-card, glyph-bearing, `plan:`-handle-carrying plan under
`_lyx/plan/` (card 1 `Create` `plan:internal/greet#Farewell`, card 2 `Edit`
`internal/greet#Hello` with the handle in `Uses:`), so the identity batchifier yields two
sequential batches.

`lyx loom validate-plan` against it: `{"ok":true,...}` — the glyph/handle surface accepted the
plan, canonicalized it, and reported clean.

Reed bring-up and the exact commands used every time:

```
cd /home/hanf/Code/r5sandbox/lyx-test-HUB/r5-crash
lyx reed up
#   -> {"ok":true,"session":"r5-crash","socket":"lyx-lyx-test-HUB-12fd835a","strands":0}
lyx reed status
#   -> {"ok":true,"session":"r5-crash","socket":"lyx-lyx-test-HUB-12fd835a","strands":[...]}
lyx reed attach          # (attach form, from the same worktree)
lyx webster run
```

The first `lyx webster run` did NOT reach a batch. It ended:

```
level=WARN msg="websterengine: master run died" outcome=died
  sessionID=2f694bbf-91f0-468f-9074-83b2abc37e26
  runDir=.../.lyx/shuttle/18de23e14563849c2761777fbea29143
{"error":"webster: master pane died (...)","ok":false}
```

Driving the pane directly while the second attempt started
(`tmux -L lyx-lyx-test-HUB-12fd835a capture-pane -p -t %3`, every 3 s) showed exactly why —
see finding R5-2, which is what this round's item 2 actually turned up.

## Findings

Severity-ranked. `CONFIRMED` = reproduced or proven by reading a definite code path;
`PLAUSIBLE` = reasoned but not driven.

### R5-2 — BLOCKING — CONFIRMED — a bare `Enter` on claude's trust gate confirms "No, exit", so every agent spawned in a not-yet-trusted directory dies at startup

`internal/shuttleengine/claudeengine/startup.go:55-59` (`TrustDismissSequence`), consumed by
`internal/shuttleengine/wait.go:360-363`.

`TrustDismissSequence` returns a single `Enter`, documented as "the key choreography that
dismisses the trust gate". Against the claude CLI shipped on this host (`claude --version`
→ `2.1.263 (Claude Code)`) the trust dialog renders as a two-item select whose caret
**defaults to the refusing option**:

```
 Quick safety check: Is this a project you created or one you trust? ...
 ❯ No, exit
   Yes, I trust this folder
 Enter to confirm · Esc to cancel
```

So the "dismissal" confirms `No, exit`. Claude quits, the pane falls back to a bare shell, the
startup window expires, and shuttle classifies the run `died` — surfaced to the operator as
`webster: master pane died`, which names neither the trust gate nor any recourse.

Failure scenario, live-observed end to end, twice, on the fresh fixture above:

1. `lyx fabric clone` + `lyx fabric add r5-crash` create a brand-new directory
   `<hub>/r5-crash`. Claude has never been run there, so it is not in `~/.claude.json`'s
   trusted-project set. **Every new fabric worktree pair is such a directory** — this is the
   module's own primary entry path, not an exotic case.
2. `lyx reed up` && `lyx webster run` → Master's pane comes up on the trust dialog.
3. `Startup` correctly classifies it `StartupTrustPrompt` (the `trustthisfolder` needle matches).
4. `wait.go` plays `TrustDismissSequence()` → one `Enter` → the caret is on `No, exit` → claude
   exits.
5. `webster: master pane died`. The batch loop never starts. Repeated on the second attempt.

Proof that the caret position, not anything else, is the cause — a manual pane on the same
directory:

```
tmux -L r5probe new-session -d -s p -c <fixture> bash
tmux -L r5probe send-keys -t p 'claude' Enter      # dialog appears, caret on "No, exit"
tmux -L r5probe send-keys -t p Down                # caret moves: "❯ Yes, I trust this folder"
tmux -L r5probe send-keys -t p Enter
# -> "Claude Code v2.1.263 / Sonnet 5 with high effort / ~/Code/r5sandbox/lyx-test-HUB/r5-crash"
#    i.e. a fully booted session
```

Blast radius: this is not webster-specific. It is on the one path every lyx-spawned agent takes
— Master, every in-session fork's own pane is inside Master so it is spared, but every `burler`
round, every loom `Discussion-Write`/`Plan-Write` producer, every recovery strand, and every
standalone `lyx webster run`/`lyx burler run` spawn goes through the same
`Startup` → `TrustDismissSequence` path. Any of them, run in a directory claude has not
previously been accepted in, dies at startup.

Why four prior rounds missed it: every prior round drove hubs whose directories claude had
already been accepted in (a prior interactive run, or a host where the dialog's option order
put "Yes" first). It is only observable on a genuinely fresh fixture — exactly what this
round's item 2 mandated.

Suggested fix: make the dismissal capture-aware instead of positional-by-luck. Widen the
`Engine` seam's method to `TrustDismissSequence(capture string) []PaneInput`, and have
`claudeengine` locate the caret line (`❯`) and the trusting option's line (`trust this folder`)
in the capture it was already given, emitting exactly the `Down`/`Up` presses that move the
caret onto the trusting option before `Enter`. When either line cannot be located, emit
NOTHING rather than a bare `Enter`: doing nothing lets the startup window expire into the same
`died` classification, whereas a blind `Enter` actively presses whatever the provider happens to
have selected — which is how this bug produced a self-inflicted exit. Provider specifics stay
inside `claudeengine`, per the Shuttle Provider-Seam Invariant.

### R5-7 — BLOCKING — CONFIRMED — claude's Bypass-Permissions acceptance gate is not a recognized startup gate, and its own caret glyph makes `Startup` report the pane READY

`internal/shuttleengine/claudeengine/startup.go:18-35` (`trustDialogNeedles` / `Startup`),
consumed by `internal/shuttleengine/wait.go:355-366`.

Found by driving past R5-2 on a second fresh hub. With the folder-trust gate accepted, claude
2.1.263 immediately raises a SECOND one-time modal, because every lyx spawn launches with
`--dangerously-skip-permissions`:

```
  WARNING: Claude Code running in Bypass Permissions mode
  ...
  By proceeding, you accept all responsibility for actions taken while running in Bypass
  Permissions mode.
  https://code.claude.com/docs/en/security

  ❯ No, exit
    Yes, I accept

  Enter to confirm · Esc to cancel
```

`Startup` does not know this gate: neither `trustthisfolder` nor `filesinthisfolder` matches. It
falls through to the ready check — and the modal's own selection caret IS the `❯` ready marker
`Startup` looks for. So the gate is classified **`StartupReady`**, `*started` is set to true, and
the startup deadline stops applying.

Live evidence, hub `/home/hanf/Code/r5sandbox2/lyx-test-HUB/r5-kill`, socket
`lyx-lyx-test-HUB-b432e999`:

```
lyx reed up            # {"ok":true,"session":"r5-kill","socket":"lyx-lyx-test-HUB-b432e999",...}
lyx reed status
lyx webster run        # backgrounded, watched via
                       # tmux -L lyx-lyx-test-HUB-b432e999 capture-pane -p -t %2
```

The pane sat on that modal, byte-identical, for 165 s of continuous polling — well past
`startup_timeout_s: 90` — and `run.json` still read `"outcome": "running"` throughout. That is
the proof of misclassification: had `Startup` reported anything other than `StartupReady`,
`classifyStartupWindow` would have returned `OutcomeDied` at 90 s.

Consequence: the run does not fail fast. It burns `master_timeout_min` — 480 minutes in the
shipped fixture config — parked on a dialog, and then reports `master run timed out`, i.e. "the
agent was working", which is the exact misdiagnosis `classifyStartupWindow`'s own doc comment says
the startup window exists to prevent. In hub mode that is eight hours of a wedged loom `Webster`
row per fresh machine or fresh checkout.

Suggested fix: give the gate its own needle so `Startup` classifies it before the ready check —
`yes,iaccept`, the accepting option's own label, which is precise to this modal and never appears
in a running claude TUI (whose bypass footer reads "bypass permissions on") — and add the same
string to `TrustDismissSequence`'s accepting-option needles, so the caret walk R5-2 introduced
carries this gate too with no second mechanism.

### R5-1 — MEDIUM — CONFIRMED — `webstercli`'s standalone integration test leaks one live tmux server per run, forever

`internal/webstercli/cli_integration_test.go:39-63`
(`TestRunCLIIn_StandalonePreRun_ReachesRunsOwnValidationGate`).

The test drives `RunCLIIn(target, out, []string{"run"})` against a fresh `t.TempDir()`.
Standalone wiring assigns `c.reedUp` (`internal/webstercli/wiring.go:265-268`), and `run` calls
it (`internal/webstercli/run.go:101-107`) BEFORE `websterengine.Run`'s plan-validation gate
refuses. So the test boots a real `tmux -L lyx-<hash8>` server, then asserts the refusal and
returns. Nothing ever tears the server down: the test has no `t.Cleanup`, and the temp target it
was booted for is deleted out from under it.

Reproduced in isolation (see "What was tested" above): one additional live server per
execution, with a dangling `-c` cwd. Because each execution derives a fresh `hash8` from a fresh
`t.TempDir()`, the leak does not coalesce — it is one orphan server per run of the integration
suite, on every developer machine and every CI worker that runs the tagged suite.
Three were live on this host from three suite runs; all 24 other lyx test sockets under
`/tmp/tmux-1000/` were dead, i.e. every other tmux-booting test in this repo does tear down.

This also violates the campaign's own teardown discipline from the inside: a reviewer running
the mandated hermetic gates leaves substrate behind by doing so.

Suggested fix: give the test a `t.Cleanup` that tears the derived reed session down
(`reedengine`'s own down/kill path over `standalonegeom.ReedGeometry(target, stateDir, hash8)`),
so the assertion keeps its value and the substrate does not survive the test.

### R5-6 — LOW — CONFIRMED — `DoneChecks` silently PASSES a blocking done-check whose target the resolve answer did not cover

`internal/planglyph/donecheck.go:139-142` — `r, ok := index[e.key]; if !ok { continue }`.

Every `e.key` reaching that loop was itself put into `targets` and handed to
`resolveTargets`, so an absent entry means quarry's positional answer did not echo the target it
was asked about. The loop's response is to skip the entry — i.e. to let a `create-not-done` /
`delete-not-done` / `rename-not-done` check PASS.

That is the exact disposition this function's own doc comment rejects two paragraphs earlier:
"a Create done-check that could not resolve is indistinguishable from a Create that never
happened, and passing it would be a false success." It is also the opposite of what the sibling
batched boundary does: `CanonicalizeHandles`
(`internal/planglyph/handle.go:236-243`) explicitly guards `quarry.Name`'s positional contract
with a length check and reports a mismatch as `ErrQuarryUnavailable`, on the stated reasoning
that "a batched boundary this file cannot see inside is exactly where a length guard belongs".

`createFindings`' own identical-looking `if !ok { continue }` (`create.go:118-121`) is NOT this
bug: its index is a documented superset lookup where a miss legitimately means "a path, a bare
symbol, or any ref `collectGlyphTargets` excluded". `DoneChecks` has no such excuse — it built
the target list itself.

Suggested fix: treat an uncovered key as an infrastructure failure, not a pass — return a
`%w: ErrQuarryUnavailable`-wrapped error naming the target the resolve answer did not cover,
matching `handle.go`'s own guard.

### R5-3 — LOW — CONFIRMED — `Run` silently drops a fingerprint re-baseline persist failure when a validation error is also in flight

`internal/websterengine/runlevel.go:469-472`:

```go
if rebaseErr := restampAndSaveFingerprint(deps.Geom, st); rebaseErr != nil && err == nil {
    return RunResult{}, rebaseErr
}
```

When `ValidateDispatch` returned an error AND the re-baseline's `SaveState` failed, `rebaseErr`
is discarded entirely. The resolve pass has by then already rewritten `_lyx/plan/` on disk
(handle canonicalization), so state.json still records the PRE-rewrite fingerprint — and the
next `lyx webster run`/`begin-batch` refuses this run's own edit as a foreign one with
`ErrFingerprintMismatch`, whose advised recourse (`--fresh`) restarts into the same wall. That
is precisely the wedge the re-baseline exists to prevent, and the operator gets no hint it
happened.

The two CLI bracket verbs handle the same coincidence correctly and loudly:
`internal/webstercli/beginbatch.go:118-121` and `internal/webstercli/recordbatch.go:143-147`
both emit `"<primary error>; additionally, persisting the plan-fingerprint re-baseline this call
had already earned failed: <err>"`. `runlevel.go` is the odd one out.

Suggested fix: mirror the CLI verbs — when both are non-nil, return an error that names both,
rather than dropping one.

### R5-4 — NIT — CONFIRMED — `repositoryRootOf`'s doc comment says "topmost", the code returns the nearest

`internal/webstercli/wiring.go:390-392` and `internal/burlercli/wiring.go:311-313`, identically:

> `repositoryRootOf` returns the **topmost-known** repository root at or above dir -- the
> **nearest** ancestor (dir itself included) carrying a ".git" entry -- ...

The two halves of that one sentence contradict each other. The loop returns on the FIRST
(nearest) ancestor carrying `.git` and never continues upward, which is the correct behaviour
for a submodule or a nested repository. "topmost-known" is simply wrong and would mislead an
editor into "fixing" the loop to keep walking, which would silently re-target a submodule's
standalone run at its superproject.

Suggested fix: drop "topmost-known" in both copies.

### R5-5 — NIT — CONFIRMED — `resolveStandaloneTarget` declares an `error` return it can never populate

`internal/webstercli/wiring.go:382-388` and `internal/burlercli/wiring.go:303-309`: both bodies
end `return repositoryRootOf(standalonestate.Normalize(told)), nil` and have no other return.
Neither `resolveToldDir`, `standalonestate.Normalize`, nor `repositoryRootOf` can fail. Both
call sites therefore carry a permanently-dead `if err != nil { return err }` branch, which no
test can cover and which reads as if the resolve were fallible.

Suggested fix: keep the signature (the error is a plausible future need at exactly this
boundary) but say so in the doc comment, so the dead branch is documented as deliberate rather
than looking like unreached error handling.


## What was read

- Glyph surface: `internal/planparser/{parse,validate,plan,rewrite,normalize}.go`,
  `internal/planglyph/{planglyph,handle,donecheck,drift,create,containment,repo}.go`.
- webster: `internal/websterengine/{runlevel,fingerprint,recordbatch,state}.go`,
  `internal/webstercli/{wiring,cli,run,beginbatch,recordbatch,recoverbatch}.go`.
- Standalone material: `internal/shuttleengine/run.go` (`NewRunner`/`NewDetachedRunner`/
  `validateDetachedToldPaths`/`playInputs`), `internal/shuttleengine/wait.go`'s startup window,
  `internal/shuttleengine/claudeengine/startup.go`, `internal/burlercli/{wiring,run}.go`,
  `internal/logger/sink.go`.
- Docs: `CONSTRAINTS.md` (full), root `CLAUDE.md`, `manifest/designs/loom.md`,
  `contracts/specs/loom-plan-spec.md`.
- Prior-round material and `loom-review-HANDOFF.md`: read only AFTER the findings above were
  written and committed, per the clean-room constraint.

## Live crash-kill scenario (item 2) — outcome

**Delivered, after both BLOCKING fixes landed and `lyx` was redeployed** (`Deployed lyx @ 1640a59bb`).
Two live hub-mode runs on genuinely fresh fixtures, then the crash-kill itself.

### Run A — a clean end-to-end hub-mode run on a fresh, previously-untrusted hub

Fixture `/home/hanf/Code/r5sandbox2/lyx-test-HUB/r5-kill`, socket `lyx-lyx-test-HUB-b432e999`:

```
cd /home/hanf/Code/r5sandbox2/lyx-test-HUB/r5-kill
lyx reed status     # {"ok":true,"session":"r5-kill","socket":"lyx-lyx-test-HUB-b432e999",...}
lyx reed attach     # (attach form)
lyx webster run
# {"batches_done":2,"cycles":[],"fabricCommitted":true,"ok":true,"outcome":"done",
#  "stuck_reason":"","summary_title":"Farewell greeting helper added alongside Hello","warnings":null}
```

Both gates were passed by lyx itself with no human at the keyboard — the pane went from the
Bypass-Permissions modal to a working TUI, forked `01-add-farewell`, then `02-hello-mentions-farewell`,
and exited. Warp history afterwards: `f3233bb 1: add-farewell`, `c6bc402 2: hello-mentions-farewell`.

Independent proof R5-2's fix is what did it: `~/.claude.json` gained
`"/home/hanf/Code/r5sandbox2/lyx-test-HUB/lyx-test": hasTrustDialogAccepted: true`, written by
lyx's own spawn on a hub that did not exist before the fix — the exact step that killed the
pre-fix run.

This also exercised the whole glyph/handle surface end to end. Card 1 declared
`` `plan:internal/greet#Farewell` -> `func Farewell() string` ``; after the run its card file
reads a plain `` `internal/greet#Farewell` `` ref — i.e. canonicalization computed the glyph,
`BindHandles` bound it from the record-batch delta, and `rewriteBulletLine`'s Create-declaration
collapse rule fired exactly as documented. `DoneChecks` passed both cards.

### Run B — the kill -9, mid-Webster-batch, in hub mode

Fixture `/home/hanf/Code/r5sandbox2/lyx-test-HUB/r5-kill2` (its own fresh pair), driven by one
single-invocation script so every PID killed was one the script itself had started. No fixed
sleeps: the script polls `state.json` against a 240 s deadline.

```
lyx reed up      # {"ok":true,"session":"r5-kill2","socket":"lyx-lyx-test-HUB-b432e999","strands":0}
lyx reed status
lyx webster run  # detached, then polled
```

1. **Mid-batch confirmed, not guessed.** After 15 s `state.json` carried
   `"currentBatch": 1` and `batches["1"] = {slug: add-farewell, startSha: 714fdb28…,
   kind: fork, spawnedAt: 2026-09-08T10:17:35Z, terminal: false, status: ""}` — begun, not
   terminal.
2. **ALIVE before the kill.** `ps -o pid=,stat=,etime=,comm=`:
   `72549 Sl+ 00:15 claude` (Master's agent) and `72509 Sl 00:15 lyx` (the run process).
   `tmux -L lyx-lyx-test-HUB-b432e999 list-panes -a` showed `%6 72526 claude`.
3. **Real `kill -9`**, run process first, then Master's agent — never a graceful stop, never
   `lyx webster pause`.
4. **DEAD after the kill.** Both `ps` lookups returned nothing ("run pid gone", "master pid
   gone"), and a `/proc/<pid>/cmdline` sweep for any surviving `claude` scoped to `r5-kill2`
   printed no `STILL ALIVE` line.
5. **The death was unclean, proven by the terminal artifact's absence.**
   `ls _lyx/webster/outcome.yaml` → `No such file or directory`; the directory held only
   `reports/` and `state.json`. No `summary.md` either.
6. **The orphan survived, exactly as the design says it must.** `tmux list-panes` still showed
   pane `%6`, now fallen back to `bash`, and `lyx reed status` still reported
   `{"guid":"25aa084d044dbccb444094471fa3373e","live":true,"name":"master::25aa084d","paneId":"%6"}`
   — the live strand a dead run process leaves behind, which `reclaimEntryTimeStrands` exists to
   stop.

### Run C — the resume

```
cd /home/hanf/Code/r5sandbox2/lyx-test-HUB/r5-kill2
lyx reed status   # strand 25aa084d… still live (the orphan)
lyx webster run
# {"batches_done":2,"cycles":[],"fabricCommitted":true,"ok":true,"outcome":"done",
#  "stuck_reason":"",
#  "summary_title":"Add Farewell greeting helper and cross-reference it from Hello","warnings":null}
lyx reed status   # {"ok":true,"session":"r5-kill2","socket":"...","strands":[]}
```

The resume reclaimed the orphaned Master strand, re-drove the begun-but-unreported batch 1,
completed batch 2, and finished `done`. Warp history: `ecd3b2d 1: add-farewell`,
`b8e0a46 2: hello-mentions-farewell`. `_lyx/webster/` afterwards holds `outcome.yaml`,
`summary.md`, `reports/`, `state.json`. Reed reports **zero** strands — the reclaim tore the
orphan down rather than leaving a second one beside a fresh Master.

**Verdict on item 2: crash resilience in hub mode is independently reproduced and holds.** The
process-level mechanics round 4 could only report — alive-before, dead-after, absent terminal
artifact, orphan pane survival, clean resume — are now re-observed on a second, independent,
fresh fixture, on a different host.

## Substrate teardown

Fixture reed sessions, all three brought down explicitly:

```
cd /home/hanf/Code/r5sandbox/lyx-test-HUB/r5-crash  && lyx reed down  # {"ok":true,"session":"r5-crash"}
cd /home/hanf/Code/r5sandbox2/lyx-test-HUB/r5-kill  && lyx reed down  # {"ok":true,"session":"r5-kill"}
cd /home/hanf/Code/r5sandbox2/lyx-test-HUB/r5-kill2 && lyx reed down  # {"ok":true,"session":"r5-kill2"}
```

Final sweep, scoped to what this round started:

```
pgrep -af "^tmux"    # NONE
pgrep -af "^lyx"     # NONE
pgrep -af "^claude"  # 2 processes, both the operator's own pre-existing interactive sessions
                     # (started 11:03 and 11:09, before this round began); none of this round's
```

The three orphaned `tmux -L lyx-<hash8>` servers R5-1 describes — leaked by the hermetic
integration suite BEFORE R5-1's fix landed, including by this round's own mandated gate runs —
were also killed, so the host is left with zero lyx substrate.

**What is deliberately left behind, and why:**

- Two fixture hubs on disk: `/home/hanf/Code/r5sandbox/lyx-test-HUB` and
  `/home/hanf/Code/r5sandbox2/lyx-test-HUB`. They are the artifacts backing every live claim above
  (state.json, `_lyx/webster/` outcomes, warp commit history), and this campaign's own verification
  practice is for the orchestrator to re-inspect a round's live evidence rather than trust its
  narrative. Delete them once round 5 is verified.
- Three branches pushed to `github.com/Knatte18/lyx-test` by `lyx fabric add`: `r5-crash`,
  `r5-kill`, `r5-kill2` (plus their `-weft` siblings on `lyx-test-weft`). Same reason, and the same
  disposition rounds 2–4 left `glyph-demo-greet`/`r4-crash-hub`/`r4-drift-hub` in.

**Honestly not verified:**

- **Windows.** Unreachable from this Linux host, as in all four prior rounds. Every Windows-shaped
  path claim in the code this round touched (`LOCALAPPDATA`, junction-vs-symlink behaviour) is read,
  never driven.
- **The bypass gate's own long-term stability.** R5-7's needle is keyed on a string claude 2.1.263
  renders today. That is a provider-owned label, and R5-2 exists precisely because a provider-owned
  detail changed under lyx once already. The fix fails SAFE when it changes again (a gate goes
  unrecognized, the run dies fast rather than pressing the wrong button), but "fails safe" is not
  "keeps working" — this seam is a standing watch item, not a solved problem.
- **`burlercli`'s standalone reed bring-up.** Still not live-driven, by this round or any prior one.
  It was explicitly optional in this round's brief and the budget went to items 1 and 2 instead.
- **Whether anything remains.** Two rounds in a row have now found BLOCKING material, and R5-7 was
  reachable only after R5-2 was fixed. That specific pattern — a defect hidden behind another
  defect on the same path — is not evidence that the path is now clear.

## Verdict

**Merge-readiness: NOT READY as a "clean pass", but the branch itself is in better shape than at
any prior point.** Every gate is green, all 7 findings are fixed and committed, and the module now
has something no prior round produced: a complete, independently reproduced hub-mode run — plan
authored, both gates cleared autonomously, two batches forked and recorded, a real `kill -9`
mid-batch, and a clean resume to `outcome: done` — on a fixture built from nothing minutes earlier.

**Convergence: NOT CONVERGED. Round 5 does not qualify as the campaign's safety pass.**
The README's bar is a round that finds nothing severe. This round found two BLOCKING defects on
the module's hottest live path, and the second was invisible until the first was fixed. That is
the opposite of the convergence signal, and it is the fifth consecutive round to find real
material.

There is also a specific reason to distrust "it works now" here: R5-2 and R5-7 were both invisible
to four prior rounds not because those rounds were careless, but because a fixture that has once
been driven by hand is permanently immunized against them. Any future round that reuses
`r5sandbox`/`r5sandbox2` — or any hub whose repo path has ever hosted an interactive claude — will
be blind to this whole class again. **A round that wants to test live bring-up must build its
fixture from a repository path claude has never seen.** That is a durable lesson about this
campaign's own method, not a fact about these two bugs.

Recommendation: one more round, on a genuinely fresh fixture, before declaring convergence.
