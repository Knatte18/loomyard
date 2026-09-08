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

**This round is NOT a clean safety pass.** Round 5 found one BLOCKING defect that all four prior
rounds missed, and it was missed for a structural reason: it is invisible on any fixture the
`claude` CLI has already been accepted in, and every prior round reused or inherited such a
fixture. This round's own item 2 — "a FRESH fixture, not `r4-crash-hub`/`r4-drift-hub`" — is
exactly what exposed it, on the very first `lyx webster run` against a hub cloned minutes earlier.

Counts: **6 findings — 1 BLOCKING, 1 MEDIUM, 2 LOW, 2 NIT.** All CONFIRMED; none PLAUSIBLE-only.

- **R5-2 (BLOCKING)** — `claudeengine.TrustDismissSequence` sends a bare `Enter` at claude's
  trust-this-folder gate. On the shipped claude (2.1.263) that gate's caret **defaults to
  `No, exit`**, so lyx's own "dismissal" confirms the refusal and claude quits. Every agent lyx
  spawns in a directory claude has not previously been accepted in dies at startup, reported only
  as an opaque `master pane died`. Every freshly-created fabric worktree pair is such a directory.
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
   **The crash-kill itself was blocked by R5-2** — the run could never reach a batch to kill,
   because Master's own pane died at the trust gate. See "Live crash-kill scenario" below for the
   post-fix outcome.
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

_(recorded after R5-2's fix landed and `lyx` was redeployed — see below)_

## Verdict

_(recorded at the end of Job 2)_
