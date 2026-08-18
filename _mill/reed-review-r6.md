# `reed` — independent review, round 6 (`opus-high-r6`)

Scoped round per `_mill/reed-review-prompt.md`:
Part A — probe the generation mechanism (`internal/reedengine/generation.go`) for its own failure modes;
Part B — close the two wiring-level regression-test gaps the orchestrator's independent verification of round 5 found.

Clean-room: findings below were formed without reading any `_mill/reed-review-*` file or `_mill/reed-shuttle-HANDOFF.md`.
`internal/reedengine/generation.go` and `generation_test.go` were read in full as ordinary code, per the prompt's explicit exception.

## What was tested

### Hermetic baseline (clean before any edit)

- `go build ./...` — clean.
- `go vet ./internal/reedengine/... ./internal/reedcli/... ./internal/hubgeom/...` — clean.
- `go test -count=5 ./internal/reedengine/... ./internal/reedcli/... ./internal/hubgeom/... ./cmd/lyx/...` — all `ok`.
- `./deploy-dev` → `.dev-bin/lyx` @ `538380e5`. Live driving below runs that binary directly, foreground.

### Live fixture

`<scratch>/lambdahub-HUB/{wt-north,wt-south}`, each a plain git repo (hub = `lambdahub-HUB`, socket `lyx-lambdahub-HUB-1460bad0`), tmux 3.6 at `/usr/bin/tmux`.
Names never used by rounds 1–5.
`wt-north` booted with one strand `alpha` running `sleep 4000`, plus the header pane.

### P1 — tmux 3.6 substrate probes: what `display-message` actually does for an ABSENT session

The generation probe's whole absent-vs-alive discrimination rests on what tmux does when `-t` names a session that does not exist.
Measured directly:

| command | result |
| --- | --- |
| `display-message -p -t '=wt-north:' '#{session_id}\|#{pid}\|#{session_created}'` | `$0\|2912080\|1787050585`, **exit 0** |
| `display-message -p -t '=nosuch:' '#{session_id}\|#{pid}\|#{session_created}'` | `\|2912080\|`, **exit 0** |
| `display-message -p -t '=nosuch:' '#{pid}'` | `2912080`, **exit 0** |
| `display-message -p -t '=nosuch:' …` with a second session on the socket | `\|\|2912080\|`, **exit 0** — no fallback to a "current" session |
| `display-message -p` (no `-t`) | most-recently-used session's fields, exit 0 |
| `display-message -p -t '=nosuch:' …` against a socket with no server | `error connecting to …`, exit 1 |
| `list-panes -t '=nosuch:'` | `can't find session: nosuch`, exit 1 |
| `has-session -t '=nosuch'` | `can't find session: nosuch`, exit 1 |
| `has-session -t '=nosuch'` against a socket with no server | `error connecting to …`, exit 1 |
| `has-session -t '=a b:c.d'` (unparseable name) | exit 1 |
| `has-session -t '='` (empty name) | exit 1 |
| `has-session -t '=wt-north'` | exit 0 |

**Key result:** `display-message` does NOT error for an absent session — it exits 0 and expands the session-scoped fields to empty, while the server-global `#{pid}` still fills.
`has-session` and `list-panes` DO error (exit 1) and are reliable existence authorities; `has-session` exits 1 for every absent/no-server/unparseable shape measured, so `TmuxCmd.hasSession`'s `(false, nil)` is a trustworthy "not alive" and its `(false, err)` a trustworthy "could not ask".

### P2 — the generation probe fails intermittently on an otherwise-healthy server (Part A item 1)

Wrapper `tmux-flaky-dm.sh` (passthrough tmux that fails **only** the pane-generation `display-message`, exit 1 with stderr), selected via `LYX_REED_TMUX`.

- Healthy: `lyx reed status` → `strands:[{alpha, live:true, paneId:"%0"}]`.
- Flaky probe: `lyx reed status` → byte-identical output.
  Fail-open confirmed as documented for a healthy state: a probe hiccup costs nothing when the bindings are in fact valid.

### P3 — the rename refusal, live (baseline re-confirmation of R5-F5)

`mv wt-north wt-moved` with session `wt-north` still up and the strand running:

- `lyx reed resume` → refusal naming `wt-north`, the socket, this worktree's session, and the exact `kill-session` remedy. exit 1.
- `lyx reed up` → same refusal. exit 1. No session created.
- `list-sessions` → `wt-north` only. R5-F5 holds.

But the non-booting verbs answer something else entirely:

- `lyx reed status` → `no reed session (1 strands persisted); run "lyx reed resume" to rebuild, or "lyx reed up" for a bare substrate`
- `lyx reed add …` → same message
- `lyx reed attach` → same message

All three point the operator at `resume`, which then refuses. See finding **R6-F1**.

### P4 — the pre-boot refusal's no-residue guarantee under a probe hiccup (Part A item 2)

The pre-boot check (`refuseRecordedForeignSessionBeforeBootLocked`) and the post-boot one (`adoptPaneGenerationLocked` → `refuseLiveForeignSessionLocked`) read the SAME recorded value under the SAME op lock, so they can only disagree if the probe answers differently between them.
A genuine two-worktree race cannot make them disagree in the damaging direction: a session (re)created inside the boot window is a different incarnation, so `SameIncarnation` is false and the post-boot check does not refuse either.
The one interleaving that DOES make them disagree is a probe that fails at the pre-boot call and succeeds at the post-boot one — reachable because `refuseLiveForeignSessionLocked` maps EVERY probe error to "the recorded session is gone".

Reproduced deterministically with `tmux-race-window.sh` (passthrough tmux answering the FIRST `'=wt-north:'` generation probe with tmux's own exit-0 empty-session-field shape, and passing every later probe through), against the live renamed-worktree state from P3:

```
sessions before:  wt-north
lyx reed up    →  {"error":"… tmux session \"wt-north\" … STILL RUNNING …","ok":false}   exit 1
sessions after:   wt-moved (1 window, pane %3)   wt-north
probe count:      2
```

`up` refused — and left a bare `wt-moved` session squatting on the shared per-hub socket, which is exactly the residue `refuseRecordedForeignSessionBeforeBootLocked`'s own doc comment exists to guarantee against ("never leave a bare session squatting on the SHARED per-hub server as the residue of a refusal").
See finding **R6-F2**.

### P5 — can the refusal wedge an operator who cannot reach tmux? (Part A item 3)

The refusal's only named remedy is raw `tmux … kill-session`.
An lyx-only escape does exist and is not named anywhere: `lyx reed down` does not load state and therefore never reaches the refusal.
Driven live in the renamed worktree (`wt-moved`, orphan `wt-north` up with `sleep 4000` running):

```
lyx reed down     →  {"ok":true,"session":"wt-moved"}          exit 0
list-sessions     →  wt-north                                   (orphan survives)
.lyx/reed.json    →  deleted
pgrep sleep 4000  →  2912341 sleep 4000                         (still running)
```

So the operator is not hard-wedged — but `down` reports unqualified success while abandoning a live session and its strand processes on the shared socket, permanently unreachable by any reed verb (no worktree of that name exists for any engine to derive it from), and deletes the only record naming it.
See finding **R6-F3**.

## Findings

### R6-F1 — every non-booting verb sends the operator to `resume`, which then refuses (LOW, CONFIRMED)

`internal/reedengine/lifecycle.go:1056` (`requireSessionLocked`), message built at `lifecycle.go:1044` (`noSessionMessage`).

**Scenario (reproduced live, P3):** a worktree is renamed while its session is up (or a `.lyx` is copied between worktrees of one hub — the two ordinary routes `refuseLiveForeignSessionLocked`'s own doc names).
`requireSessionLocked` runs first in `Status`, `AddStrand`, `RemoveStrand`, `UpdateStrand`, `SendText`, `SendKey`, `CapturePane` and `Attach`, sees this worktree's session is absent, and returns `no reed session (1 strands persisted); run "lyx reed resume" to rebuild, or "lyx reed up" for a bare substrate`.
Both commands it names then refuse with the foreign-session error.
The operator's whole diagnostic surface — `status` above all — reports a plain "no session" and never names the orphaned session, even though `requireSessionLocked` already has the state file in hand and the diagnosis is one function call away.

**Why it matters beyond wording:** this is the R5-F8 defect class (a no-session message that says a wrong thing) with a closed loop attached — the message does not merely omit, it actively routes the operator into a command that cannot succeed, and the only verb that does explain the situation is one of the two the message tells them to run.

**Suggested fix:** in `requireSessionLocked`, once the state is loaded, run `refuseLiveForeignSessionLocked(st.PaneGeneration)` and return that error when it fires, falling through to `noSessionMessage` otherwise.
Costs no tmux round trip in the healthy case (the check returns immediately when the recorded session name is empty or is this worktree's own).

### R6-F2 — the foreign-session refusal fails OPEN on a probe failure, defeating the pre-boot check's no-residue guarantee (MEDIUM, CONFIRMED)

`internal/reedengine/generation.go:174-179` (`refuseLiveForeignSessionLocked`), with `paneGenerationLocked` at `generation.go:60`.

```go
orphan, err := e.paneGenerationLocked(recorded.SessionName)
if err != nil {
    // The recorded session is gone, so there is nothing to collide with; the caller clears the
    // stale bindings and carries on.
    return nil
}
```

**Scenario (reproduced live and deterministically, P4):** the comment states a fact the error does not support.
Per P1, tmux does not error for an absent session at all — it exits 0 with empty session fields, and the absence is detected downstream by `parsePaneGeneration`'s blank-field rejection.
What DOES reach this `err` is the transport class: a failed fork/exec of the tmux binary, a non-exit-1 tmux failure, a socket-connect failure.
Every one of them is silently reclassified as "gone", which is fail-open on the ONE check in this package whose entire purpose is to refuse.

`adoptPaneGenerationLocked`'s own fail-open IS a deliberate, documented trade (clearing on "I could not tell" would discard a healthy table). This one is not the same trade and is not documented as one: the outcome of guessing wrong here is a second copy of every strand, or a cross-worktree pane kill — the two damages R5-F5 and R5-F4 were opened for.

Because the pre-boot and post-boot call sites run the SAME check against the SAME recorded value, a single transient failure at the pre-boot call is sufficient to split their verdicts: the boot proceeds, and the post-boot check then refuses over a session that is now created.
Live result: `up` reports the refusal AND leaves a bare `wt-moved` session on the shared per-hub socket — the precise residue `refuseRecordedForeignSessionBeforeBootLocked` was written to prevent, and which its doc comment claims as its reason for existing.

**Suggested fix:** make the check distinguish "the probe answered and the session is not there" from "the probe could not be run", using the authority reed already has and already trusts — `TmuxCmd.hasSession`, which exits 1 (and so answers `(false, nil)`) for absent-session, no-server, unparseable-name and empty-name alike (P1), and returns a real error only when it genuinely could not ask.
Gate the generation probe behind it, and return an actionable uncertainty error rather than `nil` when either step fails for a transport reason.

### R6-F3 — `Down` reports `ok:true` while abandoning a live orphan session and deleting the only record of it (MEDIUM, CONFIRMED)

`internal/reedengine/lifecycle.go:770` (`Down`), state deletion at `lifecycle.go:850-853`.

**Scenario (reproduced live, P5):** a renamed worktree, orphan session `wt-north` up with the strand's `sleep 4000` running.
`lyx reed down` in `wt-moved` kills the (absent) `wt-moved` session, sees a non-empty `list-sessions` so it correctly leaves the shared server alone, deletes `.lyx/reed.json`, and returns `{"ok":true,"session":"wt-moved"}`.
The orphan session and its process are still running, are now unreachable by every reed verb, and the file that named them is gone.

This is a false-healthy report in the class rounds 4–5 established as highest-weight: an unqualified success while a live session with running agent work is stranded on the shared socket forever, with no trace left behind.
`Down` is also, per P5, the ONLY lyx-only escape from the refusal — so this is the exact command a tmux-less operator is steered toward, and it is the one that destroys the evidence.

Note what this finding does NOT propose: `Down` must not kill the recorded session.
The recorded name is this worktree's own former session in the rename case but a SIBLING's live session in the hand-copied-`.lyx` case (R5-F4), and reed cannot tell them apart — killing it would resurrect R5-F4's damage under a different verb.

**Suggested fix:** before deleting the state file, detect a recorded foreign session that is still alive at the recorded incarnation, report it in `DownResult` (so the JSON envelope carries it — agents read the envelope, not logs) and log a `logger.Warn`, naming the session, the socket and the exact `kill-session` command. Keep deleting the file, so `down` stays the idempotent escape it is.

### R6-F4 — `paneGenerationLocked` and `serverPIDLocked` both document a tmux behaviour that tmux 3.6 does not have (LOW, CONFIRMED)

`internal/reedengine/generation.go:57-59` and `internal/reedengine/lifecycle.go:882-883`.

`paneGenerationLocked`: *"It errors when the session does not exist, which is how the orphan check below distinguishes 'the session this state was recorded against is gone' from 'it is still running'."*
Measured false (P1): `display-message` exits 0 and expands the session fields to empty.
The absent case is in fact caught by `parsePaneGeneration`'s blank-field rejection — whose own doc comment justifies that rejection on an entirely different ground ("a stamp missing a field would compare unequal to every future probe and clear this worktree's bindings on every op") and never mentions that the orphan check's correctness rests on it.

`serverPIDLocked`: *"returns the tmux server's OS pid, or 0 if unknown"* — for a session that does not exist on a live shared server it returns the shared server's pid, not 0, because `#{pid}` is server-global.
Traced through `Down`, that value is harmless today (it is only spent when `list-sessions` came back empty), but the comment is what a future reader would reason from.

**Why it matters:** two guards' correctness depends on a substrate property whose real load-bearing check is documented elsewhere for an unrelated reason. Relaxing `parsePaneGeneration`'s blank-field rule — a change its own doc comment makes look purely cosmetic — silently converts the orphan check into "never refuses" and `adoptPaneGenerationLocked` into "clears on every op", with no test naming the connection.

**Suggested fix:** state the measured behaviour in both comments, and make the absent-session case an explicit, named branch rather than an emergent consequence of a field-count check.

### R6-B1 — `clearConflictingPaneBindings`'s call site has no regression coverage (Part B item 1, wiring gap)

`internal/reedengine/spawn.go:214`, inside `loadOrInitStateLocked`.

Provenance: this is the orchestrator's independent-verification finding, not one I formed — round 5's own reports did not know about it.
Confirmed by reading: `reconcile_test.go:291` exercises the helper directly and `render/rules_test.go:407` exercises the independent second layer (`removeDuplicatePaneCells`), but nothing asserts that `loadOrInitStateLocked` actually calls the helper.
Deleting the call site leaves both those tests passing.

The observable that ONLY the reconcile-side layer produces is `status`'s `live` flag: a strand whose `PaneID` equals `HeaderPaneID` is, with the call site present, cleared to `""` and reported `live:false`;
with it removed, the header pane IS alive, so `aliveIDs[PaneID]` is true and status reports `live:true` against the header pane — the exact false-healthy symptom R5-F3 reproduced live.
`removeDuplicatePaneCells` sits in the render path and does not touch that report, so this assertion isolates the layer under test.

**Test to add:** drive the whole `Status` op through `TmuxCmd`'s `execHook` seam with a saved state carrying each of R5-F3's two observed conflict shapes (a strand sharing the header's pane id; two strands sharing one pane id), asserting the conflicting strand is reported not-live.

### R6-B2 — `noSessionMessage`'s readable/unreadable split has no call-site coverage (Part B item 2, wiring gap)

`internal/reedengine/lifecycle.go:1076`, inside `requireSessionLocked`.

Provenance: also the orchestrator's independent-verification finding.
`lifecycle_test.go:131` pins the pure function across both branches, but nothing asserts `requireSessionLocked` passes `loadErr == nil` rather than a constant.
Hard-coding `true` at the call site keeps the whole hermetic and smoke suites green.

The branch cannot be reached hermetically through `execHook`: it needs `TmuxCmd.hasSession` to answer `(false, nil)`, which requires a real `*exec.ExitError` with code 1, and `*os.ProcessState` has no public constructor.
It is reachable trivially against a real tmux, since `has-session` against a socket with no server exits 1 (P1) — no session boot required.

**Test to add:** a smoke-tagged CLI-seam test asserting both branches of the split — a corrupt `reed.json` with no session yields the "could not be read" text, and a worktree with no `reed.json` at all yields the plain `run "lyx reed up"` text.

### Considered and rejected as findings

- **Generation collision via PID reuse.** A reborn tmux server that recycles the previous server's pid, whose first session is `$0` again, created in the same wall-clock second, would compare `SameIncarnation` against a stale stamp. Three independent coincidences including second-granularity timing; the three fields are jointly strong enough. Not a finding.
- **A malformed multi-field probe answer from a real tmux.** All three expanded fields (`$N`, a pid, an epoch second) are separator-free by construction, so a real tmux cannot produce a 4-field answer. `parsePaneGeneration` rejects it anyway.
- **Fail-open in `adoptPaneGenerationLocked` itself.** Verified live (P2) and it is the documented, correct trade: clearing a healthy worktree's whole binding table over a tmux hiccup is strictly worse than the staleness it guards. Left as is — R6-F2 is scoped to the OTHER call site, which does not share that trade.
- **A bare session left behind when `up` boots and then fails on a corrupt `reed.json`.** Real, but it is not a refusal, and R5-F1's error already names both remedies; the next `up` adopts the session rather than being wedged by it.
- **A genuine two-worktree boot race defeating the refusal.** Probed in P4 and it cannot: a session created inside the race window is a different incarnation, so both the refusal and its `SameIncarnation` guard fall through to the safe clear.
- **Windows tmux/path behaviour.** Not driven — this host is Linux, and the prompt names a Windows verification gap as expected rather than actionable. Every measurement above is tmux 3.6 on Linux and is labelled as such.

## Counts

| severity | count | findings |
| --- | --- | --- |
| BLOCKING | 0 | — |
| MEDIUM | 2 | R6-F2, R6-F3 |
| LOW | 2 | R6-F1, R6-F4 |
| NIT | 0 | — |
| wiring-test gaps (Part B) | 2 | R6-B1, R6-B2 |

All four code findings are CONFIRMED (reproduced or measured against real tmux 3.6), none PLAUSIBLE-only.
