# `loom` (loom-step + self-report Tier 1 + Tier 2) — independent review, round 1

> Clean-room review per `_mill/loom-review-prompt.md`. Findings were formed before any fix was made and before any prior-round material was consulted (round 1 — none exists).
> Written incrementally as the review progressed; the "What was tested" section is appended to immediately after each command/scenario returns.

## Environment check (done FIRST, per the prompt's "legitimate cannot-verify" rule)

| Substrate | Result |
| --- | --- |
| `tmux` | `/usr/bin/tmux`, tmux 3.6 — available |
| `claude` | `/home/knatte/.local/bin/claude`, 2.1.236 — available |
| `gh` / GitHub token | authed as `Knatte18`, scopes `gist, read:org, repo, workflow` — live-fire is possible |
| `go` | go1.26.0 linux/amd64 |
| stray tmux servers at start | none |
| stray loom drivers at start | none |

No environment gap blocks any scenario in the prompt's high-yield list.

Anomaly check per the clean-room constraint: `_mill/` contained `loom-review-prompt.md`, `loom-review-HANDOFF.md`, `loom-crucible-orchestrator-kickoff.md`, and `status.md` at the start of this round — exactly the set the prompt anticipated. No prior review or review-dialogue file existed, so nothing was avoided or read out of order.

## Executive summary

The trio's *pure* logic is genuinely solid and the live drive confirmed it rather than contradicting it: eighteen real `lyx loom step` invocations walked a dummy task through `Preflight → Loom-Preflight → Discussion-Write → Discussion-Validate → Discussion-Review → Plan-Write → Plan-Validate → Plan-Review → Plan-Revalidate → Batchifier → Webster` with **zero envelope-fidelity mismatches**, the `busy` refusal held against a genuinely live producer, the `reinvoke` contract held on a `BurlerProducer` row with no double-spawn, and both self-report tiers fired off one real halt and filed two real GitHub issues end to end.

The defects are all at the **seams the live path added and the unit tests cannot see**: a helper that is written, documented, and tested but never called; a verb that reads a lock whose parent directory nothing has created; a `Bouncer` branch that is the one spawning mode without a live-agent probe; and — the blocking one — a `run` handshake whose central assumption Tier 2 quietly invalidated.

**Top risks, in order:**

1. **F-0 (BLOCKING)** — Tier 2's post-`shed.Run` reflection phase keeps the driver alive with the run lock released, which `run`'s handshake is documented to interpret as "a wedged spawn and nothing else". Every fast halt with a friction note now reports a false bootstrap failure *and* skips the terminal handover — defeating the exact purpose the handshake's own comment defends. Reproduced live.
2. **F-4 (MEDIUM)** — the `Bouncer`'s re-bounce branch has no live-agent probe, so a crash-resume there abandons a paid-for, still-running seed agent that holds the very file the next row reads. Reproduced live; it also falsifies a sentence in the shipped supervisor skill.
3. **F-3 (MEDIUM)** — `lyx loom status` and `lyx loom pause` leak an internal lock-path error on any never-bootstrapped task, making both verbs' own "no status file" messages unreachable. This is the `ly-supervise` skill's literal first instruction, and the skill asserts that read "always exists".
4. **F-1 (MEDIUM)** — `ensureFrictionDirAfterSeed` is never called; the clear-on-first-seed it documents never ships, and its four tests are false-green. Proved live: notes placed before a genuine first seed reached the new task's reflection agent.
5. **F-2 (MEDIUM)** — `step` fires neither tier and never ensures the friction directory. Tier 2's exemption is documented; Tier 1's is not, anywhere a `step` user would look.

**Counts:** 1 BLOCKING, 4 MEDIUM, 2 LOW, 1 NIT, plus 2 documentation/operability items. Nothing found is a LARGE finding requiring a separate mill-wiki task.

**Merge-readiness opinion (pre-fix): NOT READY.** F-0 alone is a regression on a common path, introduced by this round's own subject, that turns a healthy run into a reported failure and strands the operator outside the session. Everything found is small and local, though — none of it questions the design, and all of it is fixable within this round.

## Scope assessment — plan-vs-shipped

### `loom-step.md` — shipped as designed, one contract with no consumer

| Promised | Shipped | Verdict |
| --- | --- | --- |
| Thin wrapper over existing dispatch, no new phase logic | `step.go` calls `shed.Step` once; all routing stays in `shedengine` | met |
| Ten-key envelope, closed set | exactly ten keys on all 18 live envelopes; `stepKinds` test pins the set | met |
| Five-value `kind` vocabulary on error envelopes | `busy` observed live; the other four are constants with an exact-set test | met |
| `continue` derived so the caller carries no state vocabulary | derived at `step.go:96`; held on every envelope | met |
| `next_interrupt_policy` as the supervisor's re-invocation signal | computed, but no shipped consumer reads it — see D-1 | **drift** |
| `/ly:ly-supervise` skill with step-loop, anomaly-watching, `selfreport create`, `reed attach` | all present in `SKILL.md` | met |
| Skill never advances past a stuck/blocked gate | skill says so; `step` returns `continue:false` on every non-running state, verified live twice | met |

No over-reach: `step` adds nothing beyond the documented envelope. The one gap is that it also adds nothing where `drive` does — see F-2.

### `self-report-tier1.md` — shipped as designed

All five kinds are implemented and exhaustively table-tested; the live drive fired `escalation-to-human` for real and filed #240 with the exact documented title shape. The four-step filing pass (collapse → marker filter → file → record-per-success) is implemented exactly as described, and the marker was written with the single expected title. The `selfreport` knob gates the whole step before any read. Degrade-to-warning is honoured throughout.

One scope statement in the doc is narrower than a reader will take it: "`lyx loom drive` now detects and files…" is literally true and is the only place the `step` exemption is even implied. See F-2.

### `self-report-tier2.md` — shipped as designed, and the guard earns its keep

All seven prompt composers inject `{{.friction_directive}}` through `friction.MarkerName` (never a literal), all seven host stencils carry the marker in `contracts/stencils/`, and the four role variants plus the reflection template are all present. Verified by exhaustive call-site audit and confirmed live: the real `Discussion-Write` prompt carried the full interview-variant directive with the composed note path.

Aggregation → one reflection agent → `reflection-report.md` → archive-on-clean-return was verified end to end on a real halt, including the agent filing #241 itself. The "one reflection agent, never several" promise held: exactly one spawned.

`WarnIfMarkerAbsent` is not defensive tidying — it was the only thing that made a real, total Tier-2 outage visible during this round (D-2). Keep it.

Nothing shipped beyond scope. The doc's stated limitation (a Tier-2 note can only describe friction in its own scope) is honoured by construction.

## Code findings

(severity-ranked; provisional entries are jotted here as they are spotted and firmed up later)

### F-0 — Tier 2 broke `lyx loom run`'s driver handshake: a fast-halting run with any friction note reports a false "driver did not take the run lock" and skips the terminal handover — BLOCKING — CONFIRMED (reproduced live)

`internal/loomcli/bootstrap.go:99-106` (`dispositionForHandshake`), `internal/loomcli/run.go:163-198`, against `internal/loomcli/drive.go:188-191`.

`shedengine.Run` releases the run lock on return (`defer runLock.Release()`, run.go:319 in shedengine). Tier 2 added a phase to `drive` that runs **after** that return: `shouldReflectFriction` → `reflectFriction` → a real LLM agent bounded by `friction_timeout_min`, whose shipped default is **30 minutes**.

`run`'s handshake polls for at most `bootstrapHandshakeAttempts * bootstrapHandshakePollInterval` = 300 × 100ms = **30 seconds** for the spawned driver to take the run lock, and `awaitRunLock` returns `awaitRunLockDeadline` when the child is still alive and the lock was never seen held. `dispositionForHandshake` maps that to `handshakeRefuse`, on the strength of its own stated reasoning:

> Only awaitRunLockDeadline is a genuine refusal: the child is still alive after the whole attempt budget and has never taken the lock, **which is a wedged spawn and nothing else.**

That is no longer true. A driver that halts faster than the first poll and then spends longer than 30s in the friction reflection is alive, has released the lock, and is doing exactly the right thing.

Reproduced live on `dummy-r2`, whose only arrangement was a dirty worktree (so `Preflight` halts) and two friction notes:

```
$ lyx loom run
{"error":"loom: driver did not take the run lock; see .../dummy-r2/.lyx/loom/driver.log","ok":false}
```

The driver it had just declared wedged went on, in that same minute, to file issue #240, spawn the reflection agent, file issue #241, write `reflection-report.md`, archive the friction directory, and exit cleanly:

```
(driver.log tail) {"friction":"reflected","halted_producer":"Preflight","history_length":1,
                   "ok":true,"outcome":"blocked","reason":"stuck with no OnStuck target"}
```

Two harms, and the second is the worse one:

1. A healthy run is reported as a bootstrap failure on the envelope, pointing the operator at a driver log that shows success.
2. The `handshakeRefuse` arm returns **before run.go's step 7**, so the tmux handover never happens. `dispositionForHandshake`'s own comment explains why that matters — "the one place the halt is legible — the status strand sitting in the session — is the one place they are not put" — and that is precisely what now happens on the case the comment was written to protect.

This fires on the *common* path, not an exotic one: the comment itself names "a blocked Preflight or Loom-Preflight, an exhausted bounce budget" as the fast-halt cases, friction is default-on in the shipped `template.yaml`, and any halt with at least one note left behind triggers a reflection.

Fix: the handshake must be able to tell "wedged spawn" from "machine finished, post-run bookkeeping still running". The run lock alone can no longer carry that distinction, but the status file can: give `awaitRunLock` a third injected predicate reporting whether the persisted state has left `running`, and treat that as a proceed rather than a deadline. That keeps the genuine wedged-spawn refusal (child alive, lock never taken, machine never left `running`) while restoring the handover on every fast halt.

### F-1 — `ensureFrictionDirAfterSeed` is dead code; the once-per-task friction clear never runs — MEDIUM — CONFIRMED

`internal/loomcli/run.go:257`.
The function's own doc comment says it "performs the once-per-task clear-and-create split immediately after `loomshed.Seed`" and was "factored out of runCmd's RunE".
It is called from nowhere in production:

```
$ grep -rn "ensureFrictionDirAfterSeed" --include=*.go .
internal/loomcli/friction_test.go:36,61,79,80,99   (tests only)
internal/loomcli/run.go:248,257                    (doc comment + definition)
```

`runCmd`'s RunE calls `c.seedAndCommitBootstrap(...)` (run.go:76), and `seedAndCommitBootstrap` (sharedbootstrap.go:58-113) never calls it either. `drive.go:142` calls bare `friction.EnsureDir` with no clear.

Failure scenario: a worktree whose `.lyx/loom/friction/` still holds notes from an earlier task or an earlier run that never reached a reflection trigger (a paused run, a `step`-driven run — see F-2 — or a reflection whose agent died, which deliberately leaves the directory unarchived) is seeded as a **fresh** task. Every one of those stale notes is then scanned by `frictionengine.scanNotes` and folded into the new task's reflection dossier, which the reflection agent reads as this task's own friction and may file a GitHub issue about.

The four tests in `friction_test.go` are false-green: they call the orphaned function directly and prove its logic, not that the behaviour ships.

Fix: call it from `seedAndCommitBootstrap` at the one place that knows whether the seed was genuine (it already distinguishes `loomshed.ErrSeedExists` from a first seed at sharedbootstrap.go:82), and add a test that asserts the wiring rather than only the helper.

### F-2 — `lyx loom step` fires neither Tier 1 nor Tier 2, and never ensures the friction directory — MEDIUM — CONFIRMED (reproduced live)

`internal/loomcli/step.go:104-217` contains no `detectAndFileAnomalies` call, no `observeEntry` call, no `friction.EnsureDir` call, and no `reflectFriction` call. `drive.go` has all four (drive.go:77, 142, 160, 189).

Reproduced live against the real fixture: a `lyx loom step` invocation drove `Preflight` to `outcome: stuck` with `state: blocked` and `reason: "stuck with no OnStuck target"` — byte-for-byte the condition `loomengine.detectHaltAnomaly` maps to `AnomalyEscalation` (anomaly.go:158, matching `errorTextEscalation`) — and:

- no `.lyx/loom/selfreport-filed.json` marker was written;
- no GitHub issue was filed;
- `.lyx/loom/friction/` did not exist at all after the step, so a producer's composed note path pointed into a directory nothing had created.

Tier 2 being step-exempt is defensible and *is* documented (`self-report-tier2.md`'s "Scope: this is specifically for the unsupervised path"). Tier 1 being step-exempt is **not** documented anywhere a step user would look: `manifest/designs/loom-step.md` never mentions self-report, and `self-report-tier1.md` says only "`lyx loom drive` now detects and files…", which a reader can equally take as naming the implementation site rather than excluding `step`. The `ly-supervise` skill's self-report section describes an operator-approved manual `lyx selfreport create` and says nothing about the automatic tier being off.

Fix (documentation-first, since making `step` file per-step would mint an issue per invocation): state the exemption explicitly in `loom-step.md`, in `self-report-tier1.md`, and in the `ly-supervise` skill, and have `step` ensure the friction directory so a step-driven run's notes land where a later `run`/`drive` reflection can still aggregate them.

### F-3 — `lyx loom status` and `lyx loom pause` leak an internal lock-path error on a never-bootstrapped task; both verbs' own "no status file" messages are unreachable — MEDIUM — CONFIRMED (reproduced live)

`internal/loomcli/status.go:105-113` and `internal/loomcli/pause.go:33-43`.

The status file is durable (`_lyx/loom/status.json`) but its advisory lock is **ephemeral** (`.lyx/loom/status.json.lock`) — a different directory tree. `internal/lock` opens with `O_CREATE` but never creates a parent (stated in `shedengine.preflight`, run.go:64-70, which is why `Shed` MkdirAlls both lock parents on every `Step`). Nothing creates `.lyx/loom/` until `run`/`step`/`drive` bootstraps, so on a freshly-added task pair both verbs fail inside lock acquisition, before the `found` value they branch on is ever produced.

Reproduced verbatim on the live fixture, immediately after `lyx fabric add` and before any bootstrap:

```
$ lyx loom status
{"error":"loom: decode status file .../_lyx/loom/status.json: acquire read lock: acquire read lock: open .../.lyx/loom/status.json.lock: no such file or directory","ok":false}

$ lyx loom pause
{"error":"acquire write lock: acquire write lock: open .../.lyx/loom/status.json.lock: no such file or directory","ok":false}

$ lyx loom drive
{"error":"loom: no status file at .../_lyx/loom/status.json; run \"lyx loom run\" first to bootstrap this task","ok":false}
```

`drive` gets it right only because it `os.Stat`s the status file first (drive.go:60). The two carefully-worded messages at `status.go:111` and `pause.go:35` are dead on the one path they exist for; the unit tests never catch it because they build both paths under a `t.TempDir()` that already exists.

Why this belongs to *this* round rather than to `status`/`pause` generally: it is the `ly-supervise` skill's very first instruction. "The pre-loop baseline" tells the supervisor to take one `lyx loom status` read before the first step and asserts **"This baseline always exists"**. On a fresh task it does not exist, the read returns an error envelope, and the skill has no branch for a *status* error — its error handling is written entirely around the `step` envelope's five-value `kind` vocabulary, which a status envelope does not carry. The skill's interrupted-invocation branch also re-reads `lyx loom status` and reads `interrupt_policy` off it, so the same hole is on the recovery path.

Fix: `MkdirAll(filepath.Dir(StatusLockPath))` before the read in both verbs (the same one-liner `step.go:139` already performs for the run lock, for the same stated reason), so the `!found` branch becomes reachable and each verb emits its own documented remedy.

### F-4 — the Bouncer's re-bounce branch abandons a still-live seed agent; it is the one spawning-adapter mode with no live-agent probe — MEDIUM — CONFIRMED (reproduced live)

`internal/shedadapters/bouncer.go:253-262`.

`internal/shedadapters/doc.go`'s "Every spawning adapter probes for a live agent first" section says all four adapters answer "is an agent for this exact step still alive?" before acting, and enumerates the Bouncer's probe sites as "on its seed pass, on its judge pass, and once more at Call entry". That entry-time probe (bouncer.go:200-229) is guarded by `n > 0 && b.judged(n)`, so it covers only the judge-mode case. The **re-bounce** branch — `n == 0` with `round1FocusSeeded()` true — is a fourth mode that spawns nothing, and it returns `Stuck` with no probe of any kind:

```go
if n == 0 {
    if b.round1FocusSeeded() {
        logger.Warn("shedadapters: bouncer segment already seeded; round producer returned no report", ...)
        if cerr := cancelErr(...); cerr != nil { ... }
        return shedengine.Stuck, shedengine.OutputPointer{}, nil
    }
    return b.seedCall(ctx)
}
```

`seedCall` → `runSeedSpawn` (bouncer.go:463) *does* probe-before-archive, and its own doc comment explains why in detail. The re-bounce branch bypasses that path entirely.

Reproduced live, exactly the campaign's reinvoke scenario:

1. `lyx loom step` launched against `Discussion-Bouncer` (seed mode), reaching one live seed agent (pid 1059702, reed strand `bouncer-seed:1:40ebedb2`). Ground truth pre-count: exactly 1 hub-scoped `claude` process, exactly 1 shuttle run dir.
2. The `lyx loom step` process SIGKILLed 12s in — a driver crash. Agent survived; status still `Discussion-Bouncer`/`running`, history 7; no envelope written (the skill's "interrupted invocation" case); `lyx loom status` reported `interrupt_policy: reinvoke`.
3. `lyx loom step` re-invoked, as `reinvoke` instructs.

Observed: **no double-spawn** — still exactly 1 agent, same pid, still 1 shuttle run dir. That half of the contract holds and is the more important half.
But the reinvoked step took the re-bounce branch, returned `outcome: stuck`, routed to `Discussion-Burler`, and left pid 1059702 **running** (age 75s and climbing) holding `round-1-focus.md` as its declared output — the file `Discussion-Burler` then reads as its round directive. Two writers, one file, no ordering.

This also falsifies a documented promise: `plugins/ly/skills/ly-supervise/SKILL.md`'s interrupted-invocation section ends "Outside the handback branch, print no orphaned-agent warning, **because there is no orphan**: the next step attaches to the agent rather than abandoning it." On this branch the next step does abandon it.

Fix: give the re-bounce branch the same probe the seed pass already has — attach to a live seed run on `focusPath(RunDir, 1)` and wait on it before concluding the segment is seeded — so the branch either harvests the live seed or acts on genuinely settled state. Then the skill's "there is no orphan" sentence becomes true rather than aspirational.

### F-5 — killing a live Webster Master on a handback reinvoke is logged at Info, below the driver log's default level — LOW — CONFIRMED (reproduced live)

`internal/websterengine/strand.go:108`, against the precedent at `internal/shedadapters/bouncer.go:245`.

`internal/loomshed/interruptpolicy.go`'s own comment is accurate and was confirmed exactly as written: re-invoking an interrupted `Webster` step "kills the in-flight Master and restarts the batch run from state.json", Webster "being the most expensive row in the list, and that cost is the operator's to accept rather than the supervisor skill's."

Reproduced live. `lyx loom step` was killed mid-`Webster` with one live Master (pid 1068947, reed strand `master::52175471`); `lyx loom status` reported `interrupt_policy: handback`. A re-invocation then:

- **did not refuse** — no `busy`, no error envelope, no refusal of any kind;
- killed pid 1068947 within seconds and started a fresh Master, pid 1070022, strand `master::99f554a0`;
- printed **nothing** about it at default verbosity. The only `WARN` lines the reinvocation produced were the two unrelated friction-marker ones.

The cost the operator is supposed to be "accepting" is therefore never shown to them. `removeStrandIfLive` does log the teardown, but at `Info`, and `internal/logger`'s default console level is `Warn` (`logger.go:326`, `logger.go:385`); `Info` reaches the durable trace file only. The detached driver's own log — the one `run`'s failure envelope points an operator at — carries `Warn` and above.

The in-repo precedent is unambiguous and argues this exact case: `bouncer.go:245` logs its own expensive reclaim at `Warn`, with the reasoning "An operator whose run suddenly costs a second generation would otherwise find nothing about it in the driver log, the status file, or the run directory." Killing a live Webster Master is strictly the more expensive of the two.

Fix: raise that one line to `Warn` and name the cost in its message.

Note what is **not** a finding here: the absence of a code-level handback guard is correct and correctly documented. `loomshed.InterruptPolicies` is read in exactly two production places — `status.go:154` and `step.go:208` — and both only copy the value onto a JSON envelope. Nothing compares against `InterruptPolicyHandback` anywhere. The `ly-supervise` skill states this honestly ("a later re-invocation would restart it rather than attach to it"), so the skill is not overselling enforcement. See D-1 for the one place the wording does drift.

### F-6 — the friction reflection runs outside the run lock, so a second `lyx loom run` in that window spawns a second driver that races the archive — LOW — PLAUSIBLE (traced, not reproduced)

`internal/loomcli/drive.go:188-191` against `internal/loomcli/run.go:110-161` and `internal/frictionengine/reflect.go:157-169`.

`shedengine.Run` releases the run lock on return. `drive`'s reflection step runs after that return, for up to `friction_timeout_min` (shipped default 30 minutes). Throughout that window the run lock reads as free, so `run`'s `mustSpawnDriver(runLockHeld)` answers true and a second `lyx loom run` spawns a second detached driver.

That second driver calls `friction.EnsureDir` and, at its own halt, `Reflect` again over the same directory. Two reflection agents then hold the same `reflection-report.md` as a declared output, and `archiveFrictionDir`'s `os.Rename` races: whichever loses gets a "no such file or directory" and returns `StatusFailed` after having already paid for its agent.

Nothing is corrupted and nothing is lost — the archive is atomic and `Reflect` degrades to a `Report` rather than an error — so this is cost and confusion, not damage, which is why it is LOW. It shares a root cause with F-0 (the run lock no longer spans the driver's whole useful life) but F-0's fix does not close it: teaching the *handshake* to read the status file leaves `mustSpawnDriver` still looking at the lock.

Not reproduced: doing so means holding a real reflection agent open while racing a second `run` against it, and the campaign's cost declaration rules out running two dummy-task drives concurrently. Recorded as PLAUSIBLE on a code trace.

### F-7 — `collapseAnomaliesByTitle`'s trailing comment describes a guard that is not there — NIT — CONFIRMED

`internal/loomcli/selfreport.go:252-257`.

The loop's final branch ends with a comment and no code:

```go
if a.Kind == loomengine.AnomalyRecurringFinding { ...; continue }
// Unreachable by construction: ... Kept as a defensive guard, following the in-repo convention
// selfreportengine.CreateIssue's own targetRepo guard already sets, so a future change to the
// detector's output fails predictably rather than silently.
```

There is no guard. The comment names a specific in-repo precedent (`CreateIssue`'s `targetRepo` check, which really does return an error) and claims this code follows it so a future change "fails predictably rather than silently" — but the actual behaviour is the opposite: a non-recurring duplicate title is silently collapsed onto the first occurrence with nothing reported anywhere.

Harmless today, since the claim that duplicates are unreachable is correct. It matters because this package's comments are load-bearing and this one would mislead the next reader into believing a safety net exists. Fix: either say what the code really does (first-occurrence-wins fall-through) or add the guard the comment advertises.

## Docs & operability findings

### D-1 — `next_interrupt_policy` is computed, documented as the supervisor's signal, and read by nobody — LOW

`internal/loomcli/step.go:208` computes it and `manifest/designs/loom-step.md`'s settled contract describes it as "telling a caller whether re-invoking after an interruption on the next row is safe". The only shipped caller, `plugins/ly/skills/ly-supervise/SKILL.md`, never mentions the key: its interrupted-invocation branch instead spends a second process on `lyx loom status` and reads that verb's `interrupt_policy`.

Both values are the same table (`loomshed.InterruptPolicyFor`), and on the interrupted-step path they agree: step N-1's `next` is step N's `current_producer`. So this is not a correctness bug — it is a contract with no consumer, which is how a key rots. Either the skill should carry the previous envelope's `next_interrupt_policy` forward (one fewer process per interruption, and it works even when the `status` read itself fails — see F-3), or the design doc should say why the status read is preferred. Right now neither document acknowledges the other's existence.

### D-2 — environment observation, NOT a loom defect: a second `lyx` build on the agents' PATH silently downgrades the hub's shared stencils mid-run

Recorded because it cost real debugging time during this round and because the *detection* is a credit to Tier 2's design, not because loom should change.

Loom's own producer prompts instruct the spawned agents to run `lyx` themselves (`lyx board get`, `lyx loom validate-discussion`, and — for the Tier-2 reflection agent — `lyx selfreport create`). Those resolve through PATH inside the reed pane, not through the driver's own `os.Executable()`. On this host PATH resolved `/home/knatte/go/bin/lyx`, a Sep-8 pre-Tier-2 build, while the driver was the freshly-deployed `.dev-bin/lyx`. The agent's first `lyx board get` ran its startup stencil reconcile and rewrote `<hub>/_board/_lyx/stencils/loom/loom-template-{discussion,plan}.md` (mtime 15:29:28, mid-run) to its own older embedded copies — copies with no `{{.friction_directive}}` marker.

What caught it was Tier 2's own guard, working exactly as designed:

```
level=WARN msg="friction: stencil is missing the friction directive marker; a computed directive
will render as nothing -- see \"lyx stencil diff\" and \"lyx stencil sync\""
stencil=loom-template-discussion marker={{.friction_directive}}
```

Without `friction.WarnIfMarkerAbsent` (friction.go:120) this would have been a silent, total Tier-2 outage: every producer prompt composed after that moment would have carried no friction directive and no agent would ever have been asked for a note, with nothing anywhere reporting it. The guard is worth keeping and is doing real work.

The fix for the *fixture* was to pin PATH to the dev binary and run `lyx stencil sync` (which restored the marker in all seven host stencils). The general hazard — two lyx builds sharing one hub, the agent-side one silently rewriting shared board state — belongs to `stencilstore`'s reconcile policy, is outside this trio, and is recorded here rather than fixed.

**One detail that makes this sharper than "the operator forgot to set PATH", and that cost this round real time:** the fix did not hold. All seven host stencils were stripped again at 15:33:17, after the 15:31:49 `lyx stencil sync`. The reason is that the reed **tmux server** captures its environment when it starts, and this hub's server had started at 15:26 — before PATH was pinned. Every agent spawned into that server therefore kept resolving the old binary no matter what the driver's own shell had. Correcting PATH for the agents requires restarting the tmux server, not just exporting it.

The practical consequence for this round is worth stating plainly: from 15:33 onward every friction directive in the `dummy-r1` run rendered as nothing, so no producer in that run was ever asked to write a note — which is why `dummy-r1`'s friction directory stayed empty throughout. `WarnIfMarkerAbsent` fired on every single affected composition, which is the only reason this is a paragraph in a report rather than an invisible hole in the coverage.

## What was tested

### Hermetic baseline (before any edit)

```
go build ./...                                                  -> BUILD OK
go vet ./internal/loomcli/... ./internal/loomengine/... \
       ./internal/loomshed/... ./internal/loomrecipe/... \
       ./internal/friction/... ./internal/frictionengine/... \
       ./internal/selfreportengine/... ./internal/selfreportcli/...  -> VET OK
go test -count=5 <the nine packages above + ./cmd/lyx/...>       -> all ok, exit 0
```

All nine packages green at `-count=5`. This is the baseline every fix must preserve.

### Live smoke (real substrate, `smoke` tag)

```
go test -tags smoke ./internal/loomcli/... -run Smoke -v -count=1   -> ok, 16.3s, 13/13 PASS
```

Confirmed against the cost declaration: both `//go:build smoke` files spawn zero real LLM subprocesses (`smoke_test.go`'s `providerlessShuttleConfig`, `smoke_attachprobe_test.go`'s `shellLaunchEngine`). Safe to re-run as written.

### Live fixture

Built with the real CLI, not an in-process `RunCLI` call, because `lyx loom run`'s driver spawn resolves `os.Executable()`:

- `deploy-dev` → `.dev-bin/lyx` @ 891e54739.
- Warp/weft bare remotes built by hand, mirroring `internal/hubforge`'s `buildBareTemplate` (warp pushed and HEAD re-pointed to main; weft left genuinely empty and never pushed to).
- `lyx fabric clone <weft-bare> <warp-bare>` → a real wired hub.
- `lyx fabric add dummy-r1`, `lyx fabric add dummy-r2` → two real task pairs.
- Cost override committed into each pair's `_lyx/config/loom.yaml`: `discussion`/`plan`/`review` = `sonnet[effort=low]`, `friction` = `haiku`, timeouts cut to 25/25/25/10 minutes. `discussion_interactive: false` kept (shipped default). **`selfreport: true` and a non-empty `friction` left as shipped**, per the campaign's live-fire requirement.
- `PATH` pinned to `.dev-bin` — see D-2 for why that turned out to be load-bearing.
- A board task per slug, `dummy-r1`'s deliberately scoped to one trivial single-file change so `Webster` fans out to exactly one batch.

### Scenario 1 — envelope fidelity across a real multi-step run

`lyx loom step` invoked repeatedly against `dummy-r1`, each invocation in the foreground and waited to completion, never two at once. After every call the envelope was diffed against `_lyx/loom/status.json`'s own fields on five predicates: `next == current_producer`, `state == state`, `history_length == len(history)`, `reason == error` when blocked, and `continue == (state == "running")`.

| step | producer | outcome | next | state | hist | fidelity |
| --- | --- | --- | --- | --- | --- | --- |
| 1 | Preflight | stuck | Preflight | blocked | 1 | OK |
| 2 | Preflight | done | Loom-Preflight | running | 2 | OK |
| 3 | Loom-Preflight | done | Discussion-Write | running | 3 | OK |
| 4-5 | Discussion-Write | done | Discussion-Validate | running | 6 | OK |
| 6 | Discussion-Validate | done | Discussion-Bouncer | running | 7 | OK |
| 8 | Discussion-Bouncer | stuck | Discussion-Burler | running | 8 | OK |
| 10 | Discussion-Burler | stuck | Discussion-Bouncer | running | 9 | OK |
| 11 | Discussion-Bouncer | done | Plan-Write | running | 10 | OK |
| 12+ | Plan-Write onward | — | — | — | — | OK |

**No fidelity mismatch was observed at any step**, including the two blocked halts and the interrupted/resumed steps. `next_interrupt_policy` tracked `loomshed.InterruptPolicies` correctly throughout, and `status`'s own `interrupt_policy` agreed with the previous envelope's `next_interrupt_policy` on every comparison. Step 1's envelope also confirmed the escalation shape end to end: `outcome: stuck`, `reason: "stuck with no OnStuck target"`, `state: blocked`, `continue: false`.

One observation that is *not* a defect but is worth an operator note: on a `Preflight` halt the envelope's `reason` is the generic `"stuck with no OnStuck target"`, while the actual cause (`worktree-clean: uncommitted state changes under _lyx: M _lyx/config/loom.yaml`) goes only to a `logger.Warn` in the trace log. A supervisor driving via `step` is told a task is blocked but never why. `preflightshed.Call`'s own comment acknowledges this trade-off deliberately, so it is recorded as a known limit rather than filed.

### Scenario 2 — interrupted-and-resumed reinvoke rows (two repros)

**2a, `Discussion-Bouncer` seed pass → surfaced F-4.** `lyx loom step` launched detached, polled until exactly one hub-scoped `claude` process existed (pid 1059702, reed strand `bouncer-seed:1:40ebedb2`), SIGKILLed 12s in. Pre-count ground truth: 1 agent, 1 shuttle run dir. Agent survived the kill; status still `Discussion-Bouncer`/`running`; no envelope written; `lyx loom status` reported `interrupt_policy: reinvoke`. On re-invoke: **no double-spawn** (still 1 agent, same pid, 1 run dir) but the re-bounce branch abandoned the live seed agent — full detail in F-4.

**2b, `Discussion-Burler` round → clean.** Same protocol: killed 13s into the round with exactly one agent alive (pid 1061100, strand `burler:1:d56ff8d8`). On re-invoke, the agent count never exceeded 1 and the pid never changed across 32 seconds of polling; the step attached to the live round, waited on it, harvested `round-1-review.md`, and routed `stuck` → `Discussion-Bouncer`. Shuttle run dirs went 2 → 1 as the stale one was archived. **No second agent, no second commit, no duplicated round artifact.** The `reinvoke` contract holds on this adapter exactly as `internal/shedadapters/doc.go` describes.

### Scenario 3 — `step`'s busy refusal against a genuinely live driver

With `lyx loom step` (pid 1062224) mid-`Discussion-Bouncer` and one live agent:

```
$ lyx loom step
{"error":"loom: a driver already holds the run lock; run \"lyx loom pause\" to request a pause at
 the next producer boundary","kind":"busy","ok":false}     exit=1

$ lyx loom run < /dev/null
(no second driver spawned; pgrep -af "loom drive" empty; agent count still exactly 1)
```

Both refusals correct: `step` reports `kind: busy` and refuses rather than racing, and `run` observes the held lock and declines to spawn a second driver. Agent count stayed at 1 across both.

### Scenario 4 — the live-fire: Tier 1 and Tier 2 off one real halt

`dummy-r2` arranged so `Preflight` halts (one untracked stray file) — a genuine `escalation-to-human` condition costing **zero** LLM rows to reach — with two friction notes placed under `.lyx/loom/friction/` before a genuine first seed. Then one `lyx loom run`.

Ground truth pre-count: highest existing issue on `Knatte18/loomyard` was **#228**.

Observed, all in one driver pass:

- Status halted at `Preflight` / `blocked` / `error: "stuck with no OnStuck target"`, history 1.
- **Tier 1 fired.** Marker at `.lyx/loom/selfreport-filed.json`:
  `{"titles":["loom anomaly: escalation-to-human — dummy-r2 — Preflight#0"]}` — the title shape `anomaly.go:renderHaltTitle` specifies, with `countDoneEntries` correctly rendering `#0`.
- **A real GitHub issue was filed: [#240](https://github.com/Knatte18/loomyard/issues/240)** — "loom anomaly: escalation-to-human — dummy-r2 — Preflight#0", label `bug`, created 2026-09-13T13:40:43Z. This is the campaign's deliberate live-fire.
- **Tier 2 fired off the same halt.** Exactly **one** reflection agent spawned over the aggregated dossier; it read both notes, judged them individually, filed one issue and declined the other, wrote `reflection-report.md`, and the directory was archived to `.lyx/loom/friction-20260913-134119/` with the live directory recreated empty. `drive`'s envelope reported `"friction":"reflected"`.
- **A second real GitHub issue was filed, by the reflection agent's own judgment: [#241](https://github.com/Knatte18/loomyard/issues/241)** — "Plan spec should document card format spec file path", label `documentation`. The agent invoked `lyx selfreport create` itself, exactly as `friction-template-reflection.md` Step 3 instructs. Its report records both decisions:

  > **#241** … **Reason**: The plan spec directive references a card format authority file but doesn't provide its path …
  > **Discussion-Write.md** — Not filed … already resolved during that same session … not worth carrying forward.

**Tier 1 / Tier 2 interaction off one halt: safe.** No double-filing (distinct titles, distinct subjects), no race on the status file (Tier 1 re-reads it after `shed.Run` returns and writes only its own machine-local marker; Tier 2 touches only `.lyx/loom/friction/`), and the ordering in `drive.go` — anomalies before the early return, reflection after it — held.

**Both issues are deliberate crucible test-fires and both must be closed at campaign wrap-up.** Only #240 was a deliberately-triggered live-fire; #241 is the reflection agent exercising its own documented judgment, which is itself the end-to-end proof that the Tier-2 filing path works. Recorded here so the orchestrator closes both rather than only the one it was expecting.

**F-1 proved live in the same run.** The two friction notes were placed *before* a genuine first seed and survived it, reaching the reflection agent as this task's own friction. That is exactly the outcome `ensureFrictionDirAfterSeed`'s clear-on-first-seed branch was written to prevent, and it is what a never-called helper buys.

**F-0 surfaced in the same run** — `lyx loom run` reported `"driver did not take the run lock"` while that driver was mid-reflection and about to finish cleanly.

### Scenario 5 — the `handback` row, `NameWebster`, driven for real

`dummy-r1` was walked serially from `Plan-Write` to `Batchifier` — one `lyx loom step` at a time, each waited to completion, never two at once — reaching `Webster` at step 18 with `next_interrupt_policy: "handback"`, the only row in the table that carries it.

Step 19 was then launched and SIGKILLed 21 seconds in, with exactly one live Master (pid 1068947, reed strand `master::52175471`) and `_lyx/webster/state.json` already written. Post-kill: Master alive, status `Webster`/`running`, `lyx loom status` → `interrupt_policy: "handback"`.

Step 20 re-invoked `lyx loom step` anyway — the thing the policy exists to advise against. Observed:

- **no refusal of any kind.** No `busy`, no error envelope, no gate.
- pid 1068947 gone within 3-6 seconds; a fresh Master at pid 1070022, strand `master::99f554a0`.
- **no warning at default verbosity** about the kill — see F-5.
- the restarted run then completed correctly: `{"producer":"Webster","outcome":"done","next":"Webster-Bouncer","state":"running","history_length":18,"next_interrupt_policy":"reinvoke"}`, exactly one batch, landing a real commit `1: greeting-constant` touching `src/main.go` (+3/-1).

So `interruptpolicy.go`'s claim is confirmed in both halves: the machine silently restarts rather than resumes, **and** "correctness survives that restart". The handback policy is advisory metadata with zero code-level enforcement, and `plugins/ly/skills/ly-supervise/SKILL.md` is the only thing between an automated caller and that restart. The skill states this accurately.

### Scenario 6 — envelope key-set and vocabulary replay

All 19 captured envelopes were replayed offline against four invariants: exactly the ten documented keys and no eleventh; `continue == (state == "running")`; `reason` non-empty **iff** `state == "blocked"`; `next_interrupt_policy` within the two-value vocabulary.

```
envelopes with a mismatch: 0
```

The two deliberately-interrupted invocations wrote no envelope at all, which is the distinct case `ly-supervise`'s "Interrupted invocations" section is written for — confirmed to behave as that section describes.

## What could NOT be verified, and why

- **An agent spontaneously choosing to write a Tier-2 friction note.** The *injection* leg was verified live (the real `Discussion-Write` prompt carried the full interview-variant directive with its composed note path), and the *aggregation → reflection → filing → archive* leg was verified live on `dummy-r2` with hand-placed notes. What was not observed is a producer deciding, of its own judgment, that something went wrong. That is a model-judgment event, not a code behaviour, and it cannot be forced without doctoring the prompt. Compounding it, D-2 meant every directive after 15:33 in the `dummy-r1` run rendered as nothing, so that run could not have produced one.
- **Windows behaviour** — out of scope per the round prompt; nothing here was verified on Windows and nothing is claimed about it.
- **F-6** — recorded as PLAUSIBLE on a code trace. Reproducing it needs two concurrent dummy-task drives, which the cost declaration forbids.
- **Publish and Finalize rows** — `dummy-r1` was deliberately stopped at `Webster-Bouncer`. Both are landing rows outside this trio, both adapters in the remaining review segment had already been exercised twice, and driving to `Finalize` would have spent several more real sessions for no additional coverage of `step`, Tier 1, or Tier 2. A `RunDone`-triggered reflection was therefore not observed; the `RunBlocked` trigger was, and `shouldReflectFriction` treats the two identically (drive.go:213).
- **`selfreportengine.CreateIssue`'s error-classification branches** (token unresolvable, `*github.ErrorResponse`, network) — the live path succeeded, so only the success branch was exercised live. All three are covered by the package's hermetic tests.
