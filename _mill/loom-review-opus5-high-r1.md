# `loom` — independent review, round 1 (`opus5-high-r1`)

> Clean-room round-1 review of the `loom` module per `_mill/loom-review-prompt.md`.
> This file is written incrementally as work proceeds (crash-resilience discipline, see `crucible/README.md`).

## Status

IN PROGRESS — Job A (review) underway.

## Executive summary

_(filled at end of Job A)_

## Scope assessment

_(filled at end of Job A)_

## Findings

_(provisional; appended as spotted — severity and CONFIRMED/PLAUSIBLE per entry)_

### F1 — BLOCKING (CONFIRMED live) — a fast-halting driver makes `lyx loom run` report a false bootstrap failure and skip the terminal handover

`internal/loomcli/run.go:257-262`.

`awaitRunLock` is a deliberate **three-way** result (`awaitRunLockReady` / `awaitRunLockChildDied` / `awaitRunLockDeadline`, `internal/loomcli/bootstrap.go:34-45`),
but `run.go` collapses the last two into one error:

```go
if result != awaitRunLockReady {
    _ = bootstrapLock.Release()
    driverLogPath := loomengine.LoomDriverLog(c.location)
    clihelp.SetExit(ctx, output.Err(out, "loom: driver did not take the run lock; see "+driverLogPath))
    return nil
}
```

`awaitRunLockChildDied` means the driver **ran to completion and exited** before the handshake's first poll — which `run.go`'s own comment at :223-231 calls "the common case".
Every fast-halting run hits it: a blocked `Preflight`, a blocked `Loom-Preflight`, an exhausted bounce budget, a `drive` that errors on the envelope.

Failure scenario, reproduced end to end on a real hub:

- `lyx fabric clone` + `lyx fabric add loom-e2e`, then `lyx loom run` in the pair.
- The detached driver ran, hit `Preflight` `Stuck` (weft dirty), wrote `{"halted_producer":"Preflight","outcome":"blocked",...}` to `.lyx/loom/driver.log`, and exited — **correct behaviour**.
- `lyx loom run` printed `{"error":"loom: driver did not take the run lock; see .../driver.log","ok":false}`.

Two things are wrong: the message asserts something false (the driver did take the lock, did its work, and released it), and step 7 — the tmux attach that is the entire point of the bootstrap — is skipped, so the operator's terminal is never handed to the session that is sitting right there with the status strand in it.
The operator is told the bootstrap broke when in fact their *task* halted, and is given no way to see it.

Fix: branch on the three-way result. `awaitRunLockChildDied` is not an error — it means the driver already finished; fall through to step 7 and attach (the status strand shows the halt), or at minimum report the driver's own recorded outcome rather than a lock claim that is untrue.
Only `awaitRunLockDeadline` deserves the "did not take the run lock" refusal.

### F2 — MEDIUM (CONFIRMED live) — every mechanical gate in the pipeline discards its findings, so a bounced or blocked run leaves no diagnosis anywhere

Four producers, one shape. Each computes a structured list of exactly what failed and then throws it away:

| Producer | File:line | Discarded value |
|---|---|---|
| `Preflight` | `internal/preflightshed/preflight.go:52-58` | `preflight.Report.Failures` (`{Check CheckID, Reason string}`) |
| `Loom-Preflight` | `internal/loomshed/loompreflight.go:67-72` | `loomengine.Report.Failures` (same type, alias) |
| `Discussion-Validate` | `internal/loomshed/discussionvalidate.go:55-57` | `discussionparser.Validate`'s `findings` |
| `Plan-Validate` / `Plan-Revalidate` | `internal/loomshed/planvalidate.go:56-61` | `planparser.Validate`'s `findings` |

`Preflight` and `Loom-Preflight` carry no `on_stuck` at all (by design — "a human is the only thing that can fix any of the five"),
so the operator's *only* signal is the generic Shed text.
`Discussion-Validate` and `Plan-Validate` do bounce, but the bounce target is re-spawned with no knowledge of the complaint,
so the discarded findings are the one thing that would let either the operator or the next agent converge.

Both halves reproduced on the driven run:

**Blocked-with-no-reason.** After `Preflight` blocked:

```
$ lyx loom status
"error": "stuck with no OnStuck target",
"activity": {"now":"Preflight","last":"Preflight → stuck","wait":"stuck with no OnStuck target"}
$ cat .lyx/loom/driver.log
{"halted_producer":"Preflight","history_length":1,"ok":true,"outcome":"blocked","reason":"stuck with no OnStuck target"}
```

Nothing named `worktree-clean`, `fabric-sync`, or which path was dirty.

**Bounced-with-no-reason.** Later in the same run, `Plan-Write` produced a plan, `Plan-Validate` rejected it, and the run bounced back to `Plan-Write`:

```
history: [... ('Plan-Write','done'), ('Plan-Validate','stuck')]
$ wc -l .lyx/loom/driver.log
2 .lyx/loom/driver.log        # the ENTIRE log, for a run that had reached row 8
$ grep -ci plan .lyx/loom/driver.log
0
```

The rejected plan was archived to `_lyx/plan/archive-20260826T093144Z/` and a fresh `Plan-Write` agent was spawned with no idea what was wrong.
Which of `loom-plan-spec.md`'s sixteen check IDs fired is unrecoverable.

`Preflight` and `Loom-Preflight` share this shape:

```go
if !report.OK {
    ...
    return shedengine.Stuck, shedengine.OutputPointer{}, nil
}
```

Fix: log the determined failures (`logger.Warn` with the check IDs and reasons) before returning `Stuck`, in all four producers — the same instrumentation posture every adapter in `internal/shedadapters` already takes on a degraded exit (`Bouncer.degrade`, `BurlerProducer`'s retry warning, `WebsterProducer`'s stuck warning).
The driver log is the operator's only window into a detached run, and today it is empty.

### F3 — MEDIUM (CONFIRMED live) — a freshly-added pair has no module config at all, so the shipped `run.sh` launcher fails on first use

`lyx fabric clone` writes the nine module configs into the weft **prime's working tree** but never commits them:
after a clean clone, `git -C <hub>/<name>-weft status --short` reports `?? _lyx/`, and the only commit on the weft primary branch is `fabric clone: initialise weft primary branch main-weft`.

`lyx fabric add <slug>` then branches the weft from that commit, so the new pair's `_lyx/config/` does not exist.
The first `lyx loom run` in the pair fails on the envelope:

```
{"error":"config file .../loom-e2e/_lyx/config/loom.yaml not found; run \"lyx config reconcile\"","ok":false}
```

This is squarely on loom's own bootstrap path — `manifest/designs/loom.md`'s "The run-launcher" section has `lyx fabric add` drop a `run.sh` into the pair's launcher directory whose whole promise is "a double-click shortcut makes this one click".
On a fresh pair that shortcut cannot work.

The defect itself lives in `fabricengine`'s clone (the configs are written but not staged/committed), not in `loom`.
Recording it here because loom is where it surfaces, and because the remedy the error text names (`lyx config reconcile`) is again the bare dry-run verb rather than `--apply`.

## What was tested

Appended after each command/scenario returns.

### Hermetic gates

- `go build ./...` — **PASS** (rc=0, no output).
- `go vet ./internal/loomengine/... ./internal/loomcli/... ./internal/loomrecipe/... ./internal/loomshed/... ./internal/shedengine/... ./internal/shedadapters/... ./internal/shedrecipe/... ./internal/shedbuild/... ./internal/hubgeom/...` — **PASS** (rc=0, no output).
- `go test -count=5 ./internal/loomengine/... ./internal/loomcli/... ./internal/loomrecipe/... ./internal/loomshed/... ./internal/shedengine/... ./internal/shedadapters/... ./internal/shedrecipe/... ./internal/shedbuild/... ./internal/hubgeom/... ./cmd/lyx/...` — **PASS**, rc=0, all ten packages `ok`.

### F4 — BLOCKING (CONFIRMED by trace) — `Plan-Write`'s rotation archives a live agent's output files, defeating probe-before-archive

`internal/loomshed/planwrite.go:67-70`.

```go
func (p *planWrite) Call(ctx context.Context) (...) {
    if err := p.rotateStalePlanDir(); err != nil { ... }   // <-- unconditional, BEFORE inner
    outcome, pointer, err := p.inner.Call(ctx)
    ...
}
```

`rotateStalePlanDir` moves **every top-level `.md` file** out of `_lyx/plan/` into `_lyx/plan/archive-<stamp>/`.
`p.inner` is the `*shedadapters.SingleLLMProducer` whose entire `probe-before-archive` design decision
(`internal/shedadapters/singlellm.go:96-102`) exists to guarantee that nothing renames a live agent's output files before `Attach` has had a chance to find it:

> "Archiving is a rename of the very files a live agent may be about to write, and Wait polls for bare existence at the spec's paths,
> so archiving before the probe would make an attached run unable to ever classify Done, in exactly the case the probe exists to protect."

The decorator does exactly that, one layer above the producer that was hardened against it.
`PlanSpec` declares `OutputFiles: []string{overviewPath}` (`internal/loomengine/plan.go:82`), i.e. `_lyx/plan/00-overview.md` — a top-level `.md` file, so it is always in the rotation's set.

Failure scenario (the exact "kill the driver mid Plan-Write" ladder rung this campaign's prompt names):

1. `Plan-Write` spawns agent A; A writes the card files and then `00-overview.md` (written last, as the run's done-sentinel).
2. The driver is killed before `shedengine` persists the `Done` — the accepted at-least-once window.
3. `lyx loom run` resumes at `current_producer: Plan-Write`.
4. `planWrite.Call` rotates `00-overview.md` and every card into `archive-<stamp>/`.
5. `SingleLLMProducer.Call` probes `Attach`, finds A's `run.json` with `Outcome: "running"` and a live strand, and **attaches**.
6. `Run.Wait` polls `allOutputFilesExist([.../00-overview.md])` — false, because step 4 moved it. A has already ended its turn and nothing ever sends it new input.
7. The run spins to `plan_timeout_min`, classifies `OutcomeTimeout`, and `mapOutcome` turns that into a returned error → `shedengine` persists `state: failed` and aborts the whole run.

A completed plan is converted into a hard run failure, with the work sitting in an archive directory nobody looks at.
The milder variant is worse in a different way: a crash while A is still mid-plan rotates the cards A already wrote, A then writes a `00-overview.md` whose Card Index names files that are no longer at those paths, and `Plan-Validate` fails `path-missing` on a plan that was fine.

Fix: the rotation must run only on the respawn branch — after `Attach` reports nothing to attach to.
The natural seam is a `beforeSpawn func() error` hook on `SingleLLMProducer`, invoked between the failed probe and `archiveStaleOutputs`, with `planWrite` supplying its rotation as that hook and dropping its own unconditional call.

### F5 — MEDIUM (CONFIRMED live) — `lyx loom pause` and `lyx loom status` are disabled by a fault in ANY module config

`internal/loomcli/cli.go:115` → `internal/loomcli/wiring.go:36-78`.

`resolvePersistentPreRun` calls `c.wire()` for every subcommand except the bare `loom` group, and `wire()` eagerly loads
`loom.yaml`, `reed.yaml`, `shuttle.yaml`, `webster.yaml`, `landing.yaml`, `burler.yaml`, `models.yaml` and the active batcher,
then resolves webster roles and the review model — before any verb body runs.
`pause` needs `StatusPath`/`StatusLockPath`. `status` needs the same. Neither needs the other eight loads.

Observed live, on the run driven for this review: the Discussion-Write agent (running with bypassed permissions in its own pane) rewrote `_lyx/config/loom.yaml`.
From that moment on:

```
$ lyx loom pause
{"error":"config file .../loom.yaml: missing keys: discussion_interactive, plan, plan_timeout_min, review, review_timeout_min; run \"lyx config reconcile\"","ok":false}
$ lyx loom status
{"error":"...same...","ok":false}
```

while the driver itself kept running happily (it wired once, at process start) and the already-started status strand kept printing.
So the operator lost both the read-out **and the emergency brake** for a run that was still going, and `lyx loom pause` is the documented graceful-stop mechanism (`manifest/designs/loom.md`, "Graceful pause").

The hazard is already recognised twice in `wiring.go`'s own comments — `OpenBisector` stays lazy because "this pre-run must not fail `status`/`pause` against a healthy-but-unwired location", and `Env.Landing` is deferred to `drive.go` because "wire() runs for every verb including `status`/`pause`".
The config loads were not given the same treatment.

Fix: give `status` and `pause` a minimal pre-run that resolves the location and the two status paths only, leaving `wire()` to the verbs that actually build producers (`run`, `drive`).

### F6 — BLOCKING (CONFIRMED live) — the Bouncer's focus directive is never delivered: writer and reader disagree on filename, format, and fields

Two incompatible focus-file contracts live in one package and never meet.

**Writer** — `internal/shedadapters/round.go:63` + `bouncerfiles.go:205-246`:

```go
func focusPath(runDir string, round int) string {
	return filepath.Join(runDir, fmt.Sprintf("round-%d-focus.md", round))
}
```

`renderFocus` emits `---`-delimited **YAML frontmatter** carrying `round`, `exclude_lenses`, `focus`, plus optional prose.
The shipped stencils `contracts/stencils/bouncer/bouncer-template-seed.md:31-47` and `bouncer-template-judge.md:87-103` instruct the judge agent to write exactly that shape at `{{.focus_path}}`.

**Reader** — `internal/shedadapters/focus.go:35-37,50-70`:

```go
func roundFocusFilename(round int) string {
	return fmt.Sprintf("round-%d-focus.json", round)
}
```

`readRoundFocus` opens that `.json` path and strictly decodes **JSON** into `RoundFocus{ExcludeLenses, Hydrate}` — a different extension, a different serialization, and a different field set (`focus` vs `hydrate`).

The `.json` file is written by nothing, so `readRoundFocus` returns the zero directive on **every** call in production, and `BurlerProducer.Call` (`burler.go:256,270-281`) consequently always sees an empty `focus.Hydrate` and an empty `focus.ExcludeLenses`.
The entire targeting channel between the judge and the fixer round is dead.

Reproduced live on the driven run:

```
$ ls .lyx/loom/reviews/discussion/
round-1-focus.md

$ grep focus .lyx/loom/driver.log
level=WARN msg="shedadapters: focus file absent" producer=Discussion-Burler engine=burler
  path=.../reviews/discussion/round-1-focus.json reason=absent
```

The Bouncer's seed pass had just spawned a real `claude` session and written a substantive, well-formed `round-1-focus.md`
(a specific relocation candidate for the fixer to look at) — and the Burler row discarded it and ran unfocused.

Cost: the seed call spends one real LLM spawn **and permanently consumes one unit of the segment's bounce budget**
(`NewBouncer`'s budget rule, `bouncer.go:75-84`) to produce an artifact its only consumer cannot read.
Every judge round after a BLOCKING verdict pays the same: `ensureFocus(round+1)` and the judge's own third output file are written for nobody.

`internal/shedadapters/doc.go:92` pins the reader's spelling (`"The structured next-round directive is JSON at round-<N>-focus.json"`),
so the package doc is wrong too. Notably, the `8cac77aa` design discussion saw this exact line and dismissed it as a stale doc comment
("Out: the stale `round-<N>-focus.json` spelling in `internal/shedadapters/doc.go:90` (the code writes `round-<N>-focus.md`). Pre-existing, unrelated, left alone.")
— it is not a doc-only wart, it is a live code divergence.

Fix: the writer is the load-bearing side (the stencils, the parser, and the judge's contract all agree on it), so `readRoundFocus` must read
`focusPath(runDir, round)` through `parseFocus`, mapping `exclude_lenses` onto `RoundFocus.ExcludeLenses` and hydrating the focus file itself
so the fixer round actually reads the judge's directives; `doc.go:92` moves with it.

### Live driving — the real hub

A fresh, real fabric hub was built for this review (nothing in the operator's own repos was touched):

```
remotes: <scratch>/remotes/loomdrive.git, <scratch>/remotes/loomdrive-weft.git   (bare)
hub:     <scratch>/hubs/loomdrive-HUB
pair:    <scratch>/hubs/loomdrive-HUB/loom-e2e  +  loom-e2e-weft
```

where `<scratch>` is `/tmp/claude-1000/-home-knatte-Code-loomyard-wts-loomyard/e1daed9b-2504-4766-96de-a1a7ca997c32/scratchpad`.
The toy warp repo is a two-file Go module (`greet/greet.go`, `README.md`); the board task `loom-e2e` asks for a `Goodbye` helper beside `Hello`.
`loom.yaml` in the pair was set to `sonnet[effort=medium]` with 20-minute per-role timeouts so a full pipeline run fits in review wall-clock.

- `lyx fabric clone <weft.git> <warp.git>` — OK, after `git symbolic-ref HEAD refs/heads/main` on both bare remotes
  (a bare `git init --bare` leaves HEAD pointing at `master`; `fabric clone` reports this clearly and refuses, which is correct behaviour).
- `lyx board upsert '{"slug":"loom-e2e",...}'` — OK.
- `lyx fabric add loom-e2e` — OK; wrote `_launchers/loom-e2e/run.sh` as documented.
- `lyx loom run` (1st) — **failed on the envelope**: `loom.yaml not found`. See finding F3.
- `lyx config reconcile --apply` in the pair — OK.
- `lyx loom run` (2nd) — driver spawned, ran, halted at `Preflight`, exited; bootstrap reported
  `{"error":"loom: driver did not take the run lock; ...","ok":false}`. See findings F1 and F2.
- `lyx loom status` — round-tripped `slug`/`parent`/`current_producer`/`state`/`activity`/`history_length` correctly.
- `lyx reed status` — `{"ok":true,"session":"loom-e2e","socket":"lyx-loomdrive-HUB-3225f10b","strands":[{"name":"loom-status","live":true,"paneId":"%0"}]}`.

**Operator watch commands for this session** (both take no flags — they resolve from cwd):

```sh
cd /tmp/claude-1000/-home-knatte-Code-loomyard-wts-loomyard/e1daed9b-2504-4766-96de-a1a7ca997c32/scratchpad/hubs/loomdrive-HUB/loom-e2e
lyx reed status     # prints the JSON session/socket/strands
lyx reed attach     # hands this terminal to the live tmux session
```

## Could NOT verify

_(filled at end of Job A)_
