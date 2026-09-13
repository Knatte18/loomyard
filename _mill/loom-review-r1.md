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

## Docs & operability findings

(filled as spotted)

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
