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

(filled at the end of Job 1)

## Scope assessment — plan-vs-shipped

(filled at the end of Job 1)

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
