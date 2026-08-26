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

### F7 — BLOCKING (CONFIRMED live) — the pipeline cannot pass `Plan-Validate`: `Plan-Write` is required to write `approved: false` and `Plan-Validate` hard-fails on exactly that

This is the reason the 17-row pipeline has never been shown to run end to end. It structurally cannot.

Three shipped contracts contradict each other:

1. **The writer must not approve.** `contracts/stencils/loom/loom-template-plan.md:71,79,127` — the stencil emits `approved: false` and states
   "**Always write `approved: false` — you never self-approve**". `internal/loomengine/plan.go:11-13` says the same in the producer's own package doc,
   adding that "flipping `approved` to `true` after that segment returns APPROVED is the future loom orchestrator's job, **not built here**."
2. **The gate immediately after the writer rejects an unapproved plan.** `internal/loomshed/planvalidate.go:56` calls `planparser.Validate`,
   whose `checkFormatAndApproval` (`internal/planparser/validate.go:88-93`) emits
   `plan-unapproved: plan frontmatter approved: is not true` whenever `!plan.Approved`.
   `contracts/recipes/loom-recipe.yaml` routes `Plan-Write --on_done--> Plan-Validate --on_stuck--> Plan-Write`.
3. **Nothing in the recipe ever flips it.** `Plan-Bouncer`'s approved settle only calls `Commit` (`internal/shedadapters/bouncer.go:337-343`).
   `Plan-Revalidate` re-runs the *same* `planparser.Validate`. `Webster` (`internal/websterengine/runlevel.go:332`) runs it a third time and refuses the run.

So `Plan-Write` → `Plan-Validate` → `Stuck` → `Plan-Write` → … until `Plan-Validate`'s bounce budget is spent, then the run blocks to a human.
The plan is never wrong; only the check is unsatisfiable.

Reproduced live on the driven run. History after the discussion segment approved:

```
('Plan-Write','done'), ('Plan-Validate','stuck'), ('Plan-Write','done'), ('Plan-Validate','stuck'), ...
_lyx/plan/  ->  archive-20260826T093144Z/  archive-20260826T093227Z/   (one per rejected generation)
```

The rejected plan, extracted and run through the shipped standalone gate out of band:

```
$ lyx loom validate-plan
{"error":"loom: plan is not yet valid","findings":["plan-unapproved: plan frontmatter approved: is not true"],"ok":false}
```

and the plan itself:

```
---
format: 4
approved: false
---
# Plan: Add Goodbye helper to package greet
...
```

One finding, and it is the one the writer was ordered to produce.

The Gate Self-Check Parity Invariant did not catch this because it only pins that the verb and the row call the *same* functions — they do, and both are equally unsatisfiable.

`contracts/specs/loom-plan-spec.md:206` frames the check as "`plan-unapproved` — `approved: true`; **else refuse to run**", i.e. a guard for the *consumer* (Webster),
not for the format gate that sits between the writer and the reviewer. `manifest/designs/loom.md` row 8 likewise scopes `Plan-Validate` to
"`loom-plan-spec.md`'s existing hard-fail checks (e.g. `depends-on-order`)" — a format subset, not the run-refusal guard.

Fix (both halves are needed; either alone leaves the pipeline stuck):

- **Approval must be produced.** `Plan-Bouncer`'s APPROVED settle is the only place in the list that knows the plan passed review, so it must set
  `approved: true` in `00-overview.md` before it commits — the "future loom orchestrator's job" the design doc defers, reached by an injected
  seam beside `Commit` (an `Approve func() error` on `BouncerConfig`, an `approve_seam: plan` recipe key, an `Env.ApprovePlan` closure, and a
  `planparser` writer, since the Planparser Sole-Parser Invariant reserves plan-format writes to that package).
- **The pre-review gate must not demand it.** `Plan-Validate` runs *before* review and must skip `plan-unapproved`; `Plan-Revalidate`, which runs
  *after* the segment settles, must keep enforcing it. The two rows share the one `PlanValidate` engine, so that engine needs a recipe-authorable
  `require_approved` key, and `lyx loom validate-plan` must pick the mode that keeps Gate Self-Check Parity honest.

Size note: this is a genuine feature addition across `planparser`, `shedadapters`, `shedrecipe`, `loomshed`, `loomcli`, the recipe, and the docs —
larger than a commit-per-fix hardening change. See the fixer report for the disposition taken.

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

### F8 — MEDIUM (CONFIRMED live) — the Discussion-Write agent can rewrite loom's own `_lyx/config/`, and did, flipping the run to interactive

`contracts/stencils/loom/loom-template-discussion.md` — the whole file.

The Discussion producer is a **design** agent: its contract is to read the board task, read the codebase, interview, and write exactly two files
(`_lyx/discussion/decision-record.md` and `_lyx/discussion/support-log.md`).
In autonomous mode it launches with `--dangerously-skip-permissions` (the pane header showed `⏵⏵ bypass permissions on`), and the stencil places
**no write fence of any kind** on it. Grepping the stencil for prohibitions turns up only content rules — do not invent scope, do not propose an MVP,
never call `AskUserQuestion` — and nothing at all about which paths it may write.

Contrast the sibling contracts, which do fence: the `Webster-Burler` recipe row records that "fork reviewers are read-only and may never run any git command",
and `fix-scope: overlay` on both other Burler rows "runs no git at all and restricts writes to Target.Paths".
The one producer with an unrestricted, permission-bypassed shell is the one with no fence.

Observed live: during the driven run the Discussion-Write agent rewrote `_lyx/config/loom.yaml`, the driver's own configuration.
The file it left behind carries keys and comments that came from neither the shipped template nor the operator's edit:

```yaml
discussion_timeout_min: 20 # minutes the discussion agent's shuttle run is allowed to run (interactive interviews run long)
discussion_interactive: true # whether the discussion phase runs as an interactive interview
plan_timeout_min: 30 # minutes the plan agent's shuttle run is allowed to run
review: sonnet[effort=medium] # model-spec for the discussion-review gate agent (see docs/reference/model-spec.md)
```

against the shipped `internal/loomengine/template.yaml`, which has `discussion_interactive: false` and entirely different comment text.

The consequential line is `discussion_interactive: true`. `wire()` reads that key fresh on every call
(`internal/loomcli/wiring.go:141-143`, "read fresh on every `wire()` call"), so **the next `lyx loom run` in this worktree would spawn an interview
and wait for an operator who is not there** — on a task deliberately configured to run unattended.
An agent silently converting its own successor into a blocking human gate is a self-sabotage path with no guard on it.

Honest attribution: what *triggered* the rewrite in my run was an environment artifact — the agent's pane inherited `lyx` from `PATH`
(the operator's older global install, not the dev binary under test), so `lyx board get` failed on a config-schema mismatch and the agent
"repaired" the config to unblock itself. That trigger is mine.
The **exposure** is not: nothing in the shipped stencil, spec, or shuttle layer prevents this agent from writing any path in the worktree,
and `_lyx/config/` is precisely the state that decides whether the next run is unattended.

Fix: give the Discussion stencil an explicit write fence — it writes the two named output files and nothing else, and specifically never
`_lyx/config/`, `_lyx/plan/`, `_lyx/loom/`, or git history — in the same shape the other producers' scopes already state.

### F9 — MEDIUM (CONFIRMED live, operator-observed) — scrolling an attached loom session injects arrow keys into the live agent's pane

Reported by the operator while attached to the session this review drove:

> "if I scroll the mouse inside the window, lots of `^[[B` and `^[[A` show up in there."

Confirmed against the live server: reed ships `mouse: off` by default
(`internal/reedengine`'s config template, materialized here as
`mouse: ${env:LYX_REED_MOUSE:-off} # ... off preserves native terminal text selection/copy ...`),
and the running server agrees:

```
$ tmux -L lyx-loomdrive-HUB-3225f10b show-options -g mouse
mouse off
```

With `mouse off` tmux never claims the wheel, so the terminal emulator's own alternate-screen wheel→arrow translation fires and the
`^[[A`/`^[[B` sequences are delivered **to the pane's foreground process** — the live `claude` agent — rather than scrolling anything.
Two consequences, and the second is the serious one:

1. The operator cannot scroll back through what an agent said, which is the main reason to attach at all.
   `manifest/designs/loom.md`'s "Entry point" section sells exactly this view ("the screen is free for the reed view — the status line on top,
   agents below as they spawn") and it is not navigable.
2. Under `discussion_interactive: true` an operator is at the pane **by design** (that mode's entire premise), and every scroll gesture
   types arrow keys into the interview's input. Trying to re-read the question edits the answer.

The config comment states the trade it made ("off preserves native terminal text selection/copy") but nothing states the cost,
and nothing tells an operator who attaches a loom session that the wheel is live input.

Fix disposition: the default itself lives in `internal/reedengine`, outside this campaign's module.
What is loom's to fix here is the operator-facing documentation of its own attach step, plus naming the escape
(`LYX_REED_MOUSE=on`, or `mouse: on` in `reed.yaml`, either of which needs a fresh reed server boot to take effect, and costs
native terminal selection unless the terminal's Shift-bypass is used).
Changing reed's shipped default — `mouse: on` plus a `WheelUpPane` copy-mode binding, the conventional configuration that keeps both —
is a reed change and is recorded here for the orchestrator rather than made in a loom hardening round.

### F10 — MEDIUM (CONFIRMED live, operator-observed) — `lyx loom status --watch` reprints an unchanged line every second, flooding the strand pane and evicting its scrollback

`internal/loomcli/status.go:80-89`.

```go
for {
    polled, polledFound, pollErr := state.ReadJSONStrict[...](...)
    switch {
    case pollErr != nil || !polledFound:
        fmt.Fprintln(out, "loom status unavailable (status file transiently unreadable)")
    default:
        fmt.Fprintln(out, renderStatusLine(polled))
    }
    time.Sleep(interval)
}
```

The line is printed unconditionally on every poll — default `interval` is `1s` — regardless of whether anything changed.
A producer call lasts minutes, so the overwhelming majority of those lines are byte-identical to the one above them.

Reported by the operator while attached:

> "WHY does it print 'loom pause' ALL THE TIME. Once is enough."

Captured from the live pane during the driven run:

```
loom running | now Discussion-Write | last Loom-Preflight → done
loom running | now Discussion-Write | last Loom-Preflight → done
loom running | now Discussion-Write | last Loom-Preflight → done
... (identical, once per second, for the whole producer call)
```

Three harms, in increasing order of seriousness:

1. `manifest/designs/loom.md` specifies this strand as "**a 1-line pane at the top**". A pane emitting a fresh line every second is a scrolling
   ticker, not a status line — and reed collapses it to `collapsed_strip_rows: 3` once an agent pane exists, so the operator watches three rows of
   the same sentence scroll past forever.
2. It fills tmux's scrollback. Measured on the live pane after ~15 minutes: `%0 [history 434/2000, 279476 bytes]`.
   With `history-limit` at tmux's 2000 default, a run fills and starts evicting the buffer roughly every 33 minutes, so anything else that pane
   ever printed is gone. This compounds F9: the wheel does not scroll, and there is nothing worth scrolling to anyway.
3. It hides real transitions. The one line an operator actually needs — the moment `now` changes — is visually indistinguishable from the
   hundreds of identical lines around it.

Fix: print only when the composed line differs from the last one printed (always printing the first). That keeps the keepalive's real purpose —
a change is surfaced within one poll — while making the pane match its own design contract.
The `"status file transiently unreadable"` fallback needs the same dedupe, or a transient fault becomes its own flood.

### F11 — LOW (CONFIRMED live) — `lyx loom drive` advertises itself as the no-tmux escape hatch but cannot run any LLM producer without a live reed session

`internal/loomcli/drive.go:20-28`.

```
Short: "run loom's phase machine in the foreground, with no tmux and no strand",
Long:  `drive runs loom's phase machine in the foreground: no tmux, no strand,
and no terminal handover. It is the no-tmux escape hatch for debugging and CI.
```

"No tmux" is true only of `drive` itself. Every LLM row underneath it — `Discussion-Write`, `Plan-Write`, and all three review segments — spawns
through `shuttleengine` → `reed`, which requires a live tmux session. `run` calls `c.reed.Up()` as its step 4; `drive` calls nothing equivalent
and performs no precondition check.

Reproduced live: after tearing down the reed server, `lyx loom drive` ran happily through `Discussion-Bouncer` and only failed two producers later:

```
{"error":"shedadapters: Discussion-Burler (burler): round 1 attempt 1: run: burler: shuttle run: shuttle: add strand:
 no reed session (1 strands persisted); run \"lyx reed resume\" to rebuild, or \"lyx reed up\" for a bare substrate","ok":false}
```

The cost of failing late rather than up front is not zero: `Discussion-Bouncer`'s seed call had already attempted its spawn, and because
`runSeedSpawn` degrades every failure to a `logger.Warn` and returns (`internal/shedadapters/bouncer.go:387-432`), the failure was swallowed,
`ensureFocus(1)` wrote a **synthetic empty focus file** over the real one, and the row consumed a unit of bounce budget — all before anything
told the operator the substrate was missing. The `Discussion-Burler` row then hard-errors on the identical condition, so the two rows disagree
about whether a dead substrate is recoverable.

Fix: `drive` should ensure the substrate (`c.reed.Up()`, as `run` does) or refuse on the envelope naming `lyx reed up`, before the first producer
call. And the help text must stop claiming "no tmux" for a verb that cannot function without it.

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

Then, on the clean pair, the full chain ran for real — the first time any of it has been shown to chain end to end:

```
Preflight → done
Loom-Preflight → done
Discussion-Write → done          (real sonnet session, wrote decision-record.md + support-log.md)
Discussion-Validate → done
Discussion-Bouncer → stuck       (seed call: real spawn, wrote round-1-focus.md)
Discussion-Burler → stuck        (real burler round: round-1-review.md + round-1-fixer-report.md)
Discussion-Bouncer → done        (judge: APPROVED, commit seam fired)
Plan-Write → done
Plan-Validate → stuck            ← and here it bounces forever. See F7.
Plan-Write → done
Plan-Validate → stuck
... (x4 observed before the run was paused)
```

**`commit_seam: discussion` proven live.** Two real weft commits landed, both named `loom: discussion artifacts for loom-e2e`:
`8651cf4` from `discussionWrite`'s own post-Done commit, and `fe3ce97` from the **Bouncer's approved settle** — carrying exactly the
fixer round's relocation edit:

```
$ git -C loom-e2e-weft show --stat fe3ce97
 _lyx/discussion/decision-record.md | 1 -
-- Fixed the `_lyx/config/board.yaml` config ... via `lyx config reconcile --apply` ...
```

This is the Fabric Git Invariant's whole reason for the row, and it holds.

**Graceful pause proven live.** With a real `Plan-Write` agent mid-run, `lyx loom pause` returned `{"ok":true}`, and the driver stopped at the
next producer boundary rather than mid-call:

```
pause_requested False | Plan-Validate paused | activity {'now':'Plan-Validate','last':'Plan-Write → done','wait':''}
```

The flag was consumed in the same persist that recorded `paused`, exactly as `shedengine.Run` step 3 documents, and the agent pane was
cleaned up — `tmux list-panes` showed only the header and the status strand left.

**Bouncer clear-and-re-seed (the `8cac77aa` fix) proven live.** With `Discussion-Review` sitting APPROVED from the run above, `current_producer`
was set back to `Discussion-Bouncer` to force segment re-entry. The Bouncer archived the whole settled generation aside and re-seeded from empty:

```
.lyx/loom/reviews/discussion-20260826T093809Z/   round-1-{review,fixer-report,bouncer-verdict,bouncer-ledger,focus}.md, round-2-focus.md
.lyx/loom/reviews/discussion/                    round-1-focus.md   (fresh)
```

Both rows' artifacts moved together, as `internal/shedadapters/doc.go` requires, and the stale APPROVED verdict was **not** replayed —
history recorded `Discussion-Bouncer → stuck` (a seed), never a second `done`.
(The fresh `round-1-focus.md` is the *synthetic* empty one, because the seed spawn itself failed against a torn-down reed server — see F11.)

**Operator watch commands for this session** (both take no flags — they resolve from cwd):

```sh
cd /tmp/claude-1000/-home-knatte-Code-loomyard-wts-loomyard/e1daed9b-2504-4766-96de-a1a7ca997c32/scratchpad/hubs/loomdrive-HUB/loom-e2e
lyx reed status     # prints the JSON session/socket/strands
lyx reed attach     # hands this terminal to the live tmux session
```

## Could NOT verify

_(filled at end of Job A)_
