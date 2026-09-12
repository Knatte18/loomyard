# Discussion: lyx loom step + external supervisor skill

```yaml
task: lyx loom step + external supervisor skill
slug: loom-step
status: discussing
parent: main
```

## Problem

`loom`'s design bet (`docs/overview.md` Principle 7) is that phase sequencing is deterministic Go, never LLM judgment.
That bet holds, but it buys a blind spot: Go's periodic gates only catch what they were specifically built to check, so a defect that falls between two gates passes through untouched.
The `crucible-loom-glyph-hardening` campaign's rounds 5-7 are the evidence — a continuously-present LLM operator caught seams no rubric anticipated, while a single pass of that same presence was not reliably sufficient either.

Today the only way to drive a task is `lyx loom run`, which spawns a **detached** driver that runs the whole producer list to a terminal state with nobody reading what each row actually produced.
An operator can attach and watch the status pane, but cannot interpose between producers.
What is missing is an atomic "advance exactly one phase and return" primitive, so a live agent can read each row's real artifact — not just its verdict — before the next row starts.

Why now: the design was settled with the operator on 2026-09-12 (`manifest/designs/loom-step.md`), it supersedes the rejected `llm-driven-loom-alternative` approach, and it has no code dependency on either self-report item, so it can land in parallel with them.

## Scope

**In:**

- `internal/shedengine`: extract `Run`'s loop body into a new exported `(*Shed).Step(ctx) (StepResult, error)` plus a new `StepResult` type; rewrite `Run` as a loop over the same body so the two can never diverge.
- `internal/loomcli`: a new `step` verb (`internal/loomcli/step.go`) that bootstraps idempotently, drives exactly one producer, and emits a JSON envelope.
- `internal/loomcli`: extract `run`'s seed/commit/provenance block and its status-strand block into shared helpers both `run` and `step` call — behaviour-preserving for `run`.
- A new `plugins/ly` Claude Code plugin holding one skill, `ly-supervise`, registered in `.claude-plugin/marketplace.json`.
- Doc updates in the same commits: `manifest/designs/loom-step.md` (status + pinned return contract), `manifest/designs/loom.md` (verb list and the skill-layer table row), `docs/overview.md` (loom's verb enumeration on the module line), `manifest/roadmap.md` (Planned item removed on completion).
- Test updates: `cmd/lyx/helptree_test.go`'s loom `wantSubs`, plus new tests in `internal/shedengine` and `internal/loomcli`.

**Out:**

- Any change to the producer list, the phase table, routing, bounce budgets, or gating semantics. `step` calls the same producers `run` does, in the same order, with the same routing.
- Any phase knowledge inside the skill. The skill never names a producer, never predicts what comes next, and never decides what runs.
- Self-report Tier 1 and Tier 2 machinery (`manifest/designs/self-report-tier1.md`, `self-report-tier2.md`) — no aggregation pass, no Go-side anomaly detection, no changes to `internal/selfreportcli` or `internal/selfreportengine`. The skill's **call** to the already-shipped `lyx selfreport create` verb is in scope and is governed by the `self-report-fires-once-after-the-loop-stops-and-only-with-operator-confirmation` decision below.
- A daemon, a watcher process, or any new long-lived process. `step` is one-shot like every other `lyx` verb.
- Changes to `lyx loom run`'s observable behaviour. Its seed/commit/strand code moves into shared helpers, but its output, ordering, and exit codes are unchanged.
- Killing, cleaning, or repairing reed panes/strands, git state, or `_lyx` artifacts from the skill. The skill reports debris; the operator resolves it.
- `lyx reed attach` changes. The skill only instructs the operator to run the already-shipped verb in a side terminal.

## Decisions

### step-primitive-extracted-from-run-not-duplicated

- Decision: factor `(*Shed).Run`'s `for` body into one unexported iteration function that assumes the run lock is held, then build both `Run` (loop it under one lock) and the new exported `(*Shed).Step` (acquire the lock, call it once, release) on top of that single body.
- Rationale: `manifest/designs/loom-step.md` explicitly asks for "a thin CLI wrapper over the same internal step-dispatch … not new phase-machine logic". A shared body makes that structural rather than aspirational — routing, bounce budgeting, the pause/cancel check, the resume write, history append, and the single-persist-per-iteration crash-safety property are physically the same code for both callers.
- Rejected: a separate `Step` carrying its own copy of the six steps (guarantees drift, and `Run`'s body carries a dozen load-bearing comments about exactly-once persistence that would have to be duplicated verbatim); a loom-local step loop in `loomcli` re-reading the status file and calling producers itself (re-implements the engine outside the engine and bypasses `validate`, the run lock, and `persist`).

### step-lives-on-shedengine-not-loomshed

- Decision: `Step` is a public method on `shedengine.Shed`, available to every shed in the repo, not a loom-only affordance.
- Rationale: `Shed` is the generic engine (`landingshed`, `preflightshed`, and `loomshed` all build on it); a single-iteration primitive is a property of the engine, not of loom. The Shed Producer-Seam Invariant is unaffected — the change adds no import beyond the stdlib/`state`/`lock` set the package already has.
- Rejected: keeping `Step` unexported and exposing it only through a loom-specific wrapper (hides a generally useful primitive behind one consumer for no benefit).

### run-lock-per-step-not-held-across-steps

- Decision: `Run` keeps taking the run lock once for the whole call, exactly as today. `Step` acquires the run lock non-blockingly for the duration of its single iteration and releases it on return.
- Rationale: `Run`'s lock semantics are load-bearing and covered by existing tests — preserving them byte-for-byte keeps this refactor behaviour-preserving. For `Step`, per-call locking is the correct granularity: between steps there is by definition no driver running, and holding a lock across separate process invocations is impossible anyway. The practical payoff is mutual exclusion with a live detached driver: `lyx loom step` against a running `lyx loom run` hits `ErrShedBusy` and refuses instead of double-driving the same status file.
- Rejected: a lock-free `Step` (two steppers, or a stepper and a driver, would both read the same `current_producer` and both spawn its producer); a lock held across steps via a durable marker (a crashed stepper would brick the task, which the OS-advisory-lock design deliberately avoids).

### step-return-contract

- Decision: pin `StepResult` as `{Producer, Outcome, Output, Next, State, Reason, History}` and the CLI envelope as `{producer, outcome, output, next, state, reason, continue, history_length, status_file}`.
  - `producer` — the producer this step called; empty when none was called (already-done short-circuit, or a pause consumed before the call).
  - `outcome` — the producer's verdict, `done` or `stuck`; empty when no producer was called or none reached a verdict.
  - `output` — the producer's `OutputPointer.Path`, empty for a gate or terminal row. This is the artifact the supervisor reads.
  - `next` — `current_producer` as persisted after this step: what the next `lyx loom step` would call.
  - `state` — the persisted state after this step, one of `running`/`paused`/`done`/`blocked`/`failed`.
  - `reason` — populated only alongside `blocked`.
  - `continue` — derived, `state == "running"`: the single field the skill's loop branches on, so the skill never has to know the state vocabulary.
  - `history_length` — cheap progress signal; the supervisor uses it to confirm a step actually recorded a verdict.
  - `status_file` — the absolute status path, so the operator can look for themselves.
- Rationale: this closes `loom-step.md`'s first open question. The set is exactly "which phase ran, pass/fail, whether the task is now at a terminal/blocked state" plus the artifact pointer the design's step 2 requires ("read what each step actually produced, not just its exit code"). `continue` is derived rather than left to the caller because a thin skill must not carry a copy of the state enum.
- Rejected: returning the whole `Status` struct (leaks Shed-owned fields the supervisor has no business reading, and duplicates `lyx loom status`); returning only outcome + exit code (defeats the entire purpose); omitting `next` (the supervisor could not tell a bounce-back from forward progress).

### hard-producer-error-is-an-error-envelope

- Decision: a producer's hard error (non-nil `error` from `Call`, healthy context) makes `lyx loom step` emit a failure envelope with a non-zero exit, mirroring `drive`'s handling of the same condition. `Step` itself returns `(StepResult{}, err)` exactly as `Run` does. The envelope goes out through `output.ErrFields` carrying `kind: "producer"`, so the skill can tell it apart from a pre-producer refusal — see the `kind` vocabulary under `crash-cleanup-is-one-retry-then-hand-back`.
- Rationale: CLI/Cobra Invariant parity across loom's verbs, and the status file already records `state: "failed"` with the error text, so the supervisor loses nothing — it reads `lyx loom status` for detail. Engine-level failure is not a producer verdict and must not be dressed up as one.
- Rejected: an `ok` envelope carrying `state: "failed"` (makes a mechanism fault indistinguishable from a routed verdict at the exit-code level, and diverges from `drive` for no gain).

### step-bootstraps-idempotently-but-never-spawns-a-driver

- Decision: `lyx loom step` performs the same idempotent bootstrap `lyx loom run` does — resolve the recorded parent branch (with the same `--parent` flag), seed the status file tolerating `ErrSeedExists`, verify seed ownership, commit the seed and provenance record into the fabric, `reed.Up()`, and ensure the status strand — and then runs exactly one producer. It never spawns the detached driver and never hands the terminal to tmux.
- Rationale: without this, a step-driven task has no path to a seeded state at all. `lyx loom run` is the only seeding verb, and it spawns the driver that would immediately race the stepper — so "run it first, then step" is not available. `drive`'s refuse-if-unseeded posture exists because `run` is always available to it; for `step` that premise does not hold. The seed/commit ordering is load-bearing (the phase machine's first precondition row scans the fabric including untracked files), so it is extracted into a shared helper rather than re-derived, keeping the property identical for both verbs.
- Rejected: `step` refuses when unseeded, per `drive` (leaves the supervisor with no way to start a task); a separate `lyx loom bootstrap` verb (a third verb for one shared block, and the operator would have to remember to call it); the skill shelling `lyx loom run` then `lyx loom pause` (racy, and the driver may complete several producers before the pause lands).

### step-adds-the-status-strand

- Decision: `step` ensures the `lyx loom status` strand exists, reusing `run`'s existing `resolveStatusStrandAction` / add / replace-a-dead-entry logic through a shared helper.
- Rationale: the design's step 3 makes a human watching `lyx reed attach` a deliberate second layer. That pane is where a stepped run is legible, and the strand is what puts it there. `Up()` and the strand check are both idempotent, so the cost is one reed status read per step.
- Rejected: no strand (the operator attaches to a session with nothing to watch, defeating the design's third requirement); the skill adding the strand itself via `lyx reed` (phase/substrate knowledge leaking into the skill, against Principle 7).

### skill-never-advances-past-a-blocked-or-failed-state

- Decision: on any step whose `continue` is false — `done`, `blocked`, or `paused` — and on any error envelope, the skill stops looping and hands back to the operator with a report. It never clears `state`, never edits the status file, never re-seeds, and never pushes.
- Rationale: this closes `loom-step.md`'s second open question in the direction the doc leans, and matches crucible's own "the push/merge decision is the operator's" rule. A blocked state means a Go gate concluded a human is needed; an LLM deciding otherwise is exactly the judgment-override the whole design avoids.
- Rejected: letting the skill resume past `blocked` when it judges the cause resolved (the skill has no way to verify the fix, and a wrong call burns a full downstream phase); a config knob for it (a knob whose safe setting is the only defensible one is not a knob).

### crash-cleanup-is-one-retry-then-hand-back

- Decision: "cleaning up on a detected crash" means the skill may re-invoke `lyx loom step` **once** after a step that returned an error envelope, and nothing else. If the retry also errors, it stops and reports. It never kills reed panes, removes strands, deletes lock files, touches git, or edits `_lyx` content.
- Scope of "an error envelope", and the discriminator that bounds it: `step` emits its failure envelopes through `output.ErrFields`, not the bare `output.Err`, carrying a `kind` key the skill branches on mechanically. `ErrFields(w, msg, fields)` already exists in `internal/output` for exactly this purpose and is already used this way by `internal/loomcli/validate.go`, `internal/webstercli/beginbatch.go` (`plan_drifted`), `internal/shuttlecli/run.go`, and `internal/fabriccli/envelope.go` — so this is a one-verb addition with no repo-wide surface and no change to `output.Err` itself. The `kind` vocabulary is closed: `busy` (the run lock is held — a driver or another stepper is live), `unseeded` (no status file and seeding itself failed), `ownership` (a `VerifySeedOwnership` mismatch), `bootstrap` (any other pre-producer failure: origin resolve, seed commit, reed substrate), and `producer` (the producer itself returned a hard error).
- The one-retry rule applies to `kind: "producer"` **only**. Every refusal kind is handed straight back to the operator with no retry: none of them can be fixed by running the same command again, so a retry would be a wasted invocation and — worse — would read to the operator as though the skill were trying something. `busy` in particular is a signal the operator has a driver running and must `lyx loom pause` it; retrying that silently is exactly the wrong shape.
- Rejected: keeping a blind retry over any `err` envelope and accepting the wasted invocation (the premise that no discriminator was available was simply false); adding a kind key to `output.Err` itself (unnecessary — `ErrFields` is the existing seam for per-verb keys).
- Rationale: loom's own resume semantics already define the safe recovery — `StateBlocked` and `StateFailed` deliberately do not short-circuit `Run`, so re-calling `current_producer` is the sanctioned way forward, and the OS advisory run lock is reclaimed automatically on process death. One retry distinguishes a transient fault from a real one at zero risk. Anything more destructive is an operator decision.
- Rejected: an unbounded retry loop (turns a deterministic failure into a spend loop); pane/lock cleanup by the skill (the exact class of unverifiable mutation the operator hand-back rule exists to prevent).

### step-invocation-is-backgrounded-not-a-blocking-foreground-call

- Decision: the skill never issues `lyx loom step` as a blocking foreground shell call. It launches the invocation in the background with stdout redirected to a per-step file under `.scratch/` (for example `.scratch/ly-supervise/step-<n>.json`), then waits for that process to exit and reads the envelope from the file.
- Rationale: a single step blocks for the whole producer call, and loom's LLM rows — `Discussion-Write`, `Plan-Write`, and all three `Burler` rounds — are minutes-to-an-hour agent spawns. Every agent shell tool caps a foreground call in the single-digit minutes, so a foreground `lyx loom step` would be killed mid-producer on the rows that matter most, every time. Redirecting to a file rather than reading stdout back through the harness also keeps a step's envelope out of the transcript until the skill chooses to read it, which matters across a 40-step loop.
- Cwd precondition: `lyx loom step` derives everything from the current working directory (Cwd Resolution Invariant — `lyxcwd.Resolve` requires cwd to be a git worktree root), so **the supervising session's cwd must be the task worktree**, and the skill verifies that before its first step rather than discovering it as a resolve failure mid-loop. `.scratch/` is relative to that same cwd, so the envelope files land at `<task-worktree>/.scratch/ly-supervise/step-<n>.json`. `.scratch/` is already gitignored repo-wide, and `plugins/scribe/skills/conversation/SKILL.md`'s file-writing rule makes it the mandated scratch location — never the OS temp directory.
- Note: this is the skill's invocation mechanism only. `lyx loom step` itself stays an ordinary foreground one-shot verb that writes its envelope to stdout and exits — no daemon, no detach, no new process posture in Go. Backgrounding is the caller's business, exactly as it is for any other long `lyx` verb.
- Rejected: a foreground call with a raised timeout (no timeout an agent harness offers covers an hour-long review row, and the failure mode is silent truncation mid-producer); `step` detaching itself like `run`'s driver (re-creates the detached-driver posture the whole design is trying to interpose on, and leaves the skill with nothing to wait on); a `--timeout` flag on `step` (bounds the verb, not the producer, and a producer killed by its caller's clock is exactly the state the next decision has to handle).

### interrupted-step-re-invokes-on-loom-s-own-crash-resume-with-a-consecutive-cap

- Decision: a step whose **invocation** was interrupted — killed, timed out, or exited without writing a parseable envelope — is a distinct case from the error-envelope case and is handled by its own rule. On detecting it the skill reads `lyx loom status` once and branches:
  - `current_producer` moved, or `history_length` grew, or `state` is no longer `running`: the producer finished and only the invocation died. Continue the loop from the fresh status, and do **not** count the step twice.
  - nothing moved, `state` is still `running`, and `current_producer` is **any row other than `Webster`**: re-invoke `lyx loom step` for the same producer. This is loom's own designed crash-resume, not a retry of something unknown.
  - nothing moved, `state` is still `running`, and `current_producer` is **`Webster`**: **stop and hand back to the operator**, saying plainly that a live Master may still be running in its pane and that a later re-invocation will stop and restart it rather than attach to it. See the Webster carve-out below.
  - a cap applies to the re-invoking branch: at most **two consecutive** interrupted-and-re-invoked steps against the same `current_producer`. On a third consecutive interruption, stop and hand back — something is wrong with the invocation mechanism itself, not with the run.
- Rationale: on every row but one, re-calling `current_producer` does **not** double-spawn. `internal/shedadapters/doc.go`'s "Every spawning adapter probes for a live agent first" section is the authority: `SingleLLMProducer`, `Bouncer` (on its seed pass, its judge pass, and again at `Call` entry), and `BurlerProducer` all call `shuttleengine`'s `Attach` seam with the step's own `OutputFiles` and wait on a match, always before any archive. `manifest/designs/loom.md` records that probe as the fix for precisely this hazard: before the review-segment rows gained it, a driver crash left the segment's agent alive and the next `lyx loom run` started a second one over it. So on those rows a re-invocation reattaches to the live agent and waits, exactly as a post-crash `lyx loom run` does, and handing back on the first interruption would discard the crash-resume loom deliberately ships.
- **The `Webster` carve-out, and why it is the exception**: `WebsterProducer` does not attach. The same `doc.go` section is explicit that it "inherits `websterengine`'s own entry-time reclaim, **which stops a leftover Master rather than attaching to it**" — implemented by `reclaimEntryTimeStrands` in `internal/websterengine/runlevel.go` and pinned by `TestRun_EntryTimeReclaimStopsLiveMasterAndRecoveryStrandsButNotAbsent`. Re-invoking an interrupted `Webster` step therefore **kills the in-flight Master and restarts the batch run from `state.json`**. Correctness survives that — recovery from `state.json` is what the reclaim exists for — but the cost does not: `Webster` is the most expensive row in the list, and the restart throws away whatever the live Master had in flight. That cost is the operator's to accept, not the skill's, so the skill hands back and says what a re-invocation would do. This is also the one place an orphan warning is warranted: the Master really is left alive and unowned until someone re-enters the row.
- Why a cap on the re-invoking branch: the attach probe makes each re-invocation safe, but it does not make an endlessly-interrupted loop productive. Two consecutive interruptions against one producer distinguishes a one-off from a systematic problem with the caller's own invocation mechanism, which is the only thing left that a re-invocation cannot fix.
- Outside the `Webster` branch the skill prints no orphaned-agent warning: there is no orphan, because the next step attaches to the agent rather than abandoning it.
- Rejected: one uniform re-invoke rule across all seventeen rows (false — it silently restarts the most expensive row in the list); hand back on the first interruption everywhere (throws away loom's shipped crash-resume and turns every harness timeout into an operator interrupt); unbounded re-invocation (a systematically broken invocation mechanism would spin against the iteration cap); killing the reed pane before re-invoking (destructive mutation the skill is barred from, and on the attaching rows it would destroy the very agent the probe is about to attach to).

### self-report-fires-once-after-the-loop-stops-and-only-with-operator-confirmation

- Decision: the skill may call `lyx selfreport create` **only after** its loop has stopped — terminal state, blocked, hand-back, or iteration cap — never between steps. At most one issue per supervised run. It drafts the title and body, shows both to the operator, and fires only on explicit operator approval. The body is passed via `-b -` on stdin and states the producer, the step envelope fields that were surprising, and the artifact path it read. The default `bug` label stands for a defect; `--label enhancement` is used for friction that is not a defect.
- Rationale: `internal/selfreportcli` files a **real public issue** on `Knatte18/loomyard` through the GitHub API — an outward-facing, hard-to-reverse act, and exactly the class of action that needs confirmation rather than autonomous firing. Deferring to the end of the loop is what gives the skill the full-run context `manifest/designs/self-report-tier2.md` exists to reconstruct after the fact; filing mid-loop would produce several thin issues about symptoms of one cause. One-per-run is the cap that stops a bad run from becoming an issue flood.
- Note: this decision governs *when and how* the skill calls the already-shipped verb. The Scope § Out bullet excludes building Tier 1/Tier 2 machinery, not calling `lyx selfreport create` — the call is in scope and is one of the design doc's three named skill responsibilities.
- Rejected: firing autonomously on noticed friction (files public issues on an LLM's unreviewed judgment); firing mid-loop per anomaly (several issues per root cause, and the skill has not yet seen how the run ends); deferring the call out of this task entirely (`manifest/designs/loom-step.md` step 2 names it as a skill responsibility, so dropping it would leave the task incomplete against its own design doc).

### skill-lives-in-a-new-ly-plugin-named-ly-supervise

- Decision: create `plugins/ly/` with `.claude-plugin/plugin.json` (name `ly`, version `1.0.0`), `skills/INDEX.md`, and `skills/ly-supervise/SKILL.md`; add the plugin to `.claude-plugin/marketplace.json`. Invocation is `/ly:ly-supervise`.
- Rationale: `docs/overview.md` pins `ly` as the skill-plugin name and `/ly-*` as the skill naming, explicitly derived from the millhouse `mill` plugin whose skills are `mill-start`, `mill-go`, etc. (invoked `/mill:mill-start`). This is the first `/ly-*` skill, so the plugin scaffolding lands with it. `ly-supervise` names what the agent does; `ly-step` would name the verb it calls and read as a one-shot wrapper.
- Rejected: putting the skill in `plugins/scribe` (scribe is code-writing conventions, not orchestration); naming the plugin `lyx` or the skill `loom-step` (both banned outright by `docs/overview.md`'s naming rule).

### skill-loop-has-a-hard-iteration-cap

- Decision: the skill's loop carries an explicit iteration cap of **40 steps**. On reaching it the skill stops and reports rather than continuing.
- Rationale: a bounce segment that keeps returning `stuck` within budget produces real forward-looking steps that are nonetheless not progress; loom's own bounce budget bounds each producer but not the whole walk. A cap is the skill's own backstop against an unattended spend loop, and hitting it is itself a friction signal worth reporting.
- Where 40 comes from, and what it is not: loom's producer list is seventeen rows (`internal/loomshed`'s `Name*` constants, `Preflight` through `Finalize`). Three of them are review segments, and one review round inside a segment costs two steps — the `Bouncer` row plus its `Burler` offshoot, whose `OnStuck` always points back at the `Bouncer`. A run taking three review rounds in each of the three segments therefore walks 17 + (3 × 3 × 2) = 35 steps, and 40 is that rounded up. **40 is a typical-run margin, not a derivation from the mechanical ceiling, which is far higher.** That ceiling is worth stating so nobody mistakes 40 for a computed bound:
  - The six review rows carry an explicit `max_bounces: 5` in `contracts/recipes/loom-recipe.yaml` (`Discussion-Bouncer`/`Discussion-Burler`, `Plan-Bouncer`/`Plan-Burler`, `Webster-Bouncer`/`Webster-Burler`), and `effectiveMaxBounces` prefers `ProducerDef.MaxBounces` over the shed-level value — so `internal/loomcli/wiring.go` leaving `ShedPaths.MaxBounces` zero governs only the rows that set nothing. Three segments at five bounces of two steps each is 30 extra steps, not the 3 × 10 × 2 an earlier draft of this section claimed.
  - The three mechanical validator rows bounce too, and at the inherited default of ten: `Discussion-Validate` → `Discussion-Write`, `Plan-Validate` → `Plan-Write`, and `Plan-Revalidate` → `Plan-Write` (the last routes to the writer, not to `Plan-Bouncer`). Each such bounce also costs two steps, adding up to roughly 60 more.
  - Worst case is therefore near 17 + 30 + 60 ≈ 107 steps, not 77.
  So 40 sits well under the ceiling by design: it is an adjustable safety margin chosen against a typical run's shape. A genuinely long but healthy run can reach it, and when it does the skill stops and hands back rather than failing anything — the operator re-invokes the skill to continue, and loom's own state is untouched by the cap.
- Rejected: no cap (an LLM loop with a shell verb and no ceiling); a wall-clock budget (an LLM review row legitimately takes many minutes, so a time cap would fire on healthy runs).

## Technical context

**`internal/shedengine/run.go`** is where the whole refactor happens.
`Run` currently does: `validate()`, `MkdirAll` both lock parents, `lock.TryAcquireWriteLock(s.LockPath)` (returning `ErrShedBusy` when held), `defer runLock.Release()`, then an unbounded `for` whose body is the six steps.
The body reads the status file strictly (`state.ReadJSONStrict[Status]`), short-circuits on `StateDone`, looks up the producer by name via `findProducer` (hard-erroring when absent), checks `PauseRequested || ctx.Err()`, writes the conditional resume persist when `st.State != StateRunning`, calls `def.Producer.Call(ctx)`, builds `nextHistory` via the `appendHistory` closure (which skips the append entirely on an empty outcome), and routes in one `switch` with five arms: cancelled-with-error → paused, error → failed + return error, `Stuck` → blocked (no `OnStuck`) / blocked (budget exhausted, counted by `episodeStuckCount` over `st.History`, never `nextHistory`) / bounce to `OnStuck`, `Done` → done (empty `OnDone`) / jump to `OnDone`, default → unrecognised-outcome failure.
`persist` is the single write path; `Run` performs exactly one persist per iteration except for the conditional resume write.
Every `continue` arm becomes "return a `StepResult` with `State: StateRunning`"; every `return Result{...}` arm becomes a terminal `StepResult`.

`RunOutcome`'s three string values are deliberately identical to `State`'s three clean-exit values (`shed.go` says so explicitly), so `Run` can map a terminal `StepResult.State` to `Result.Outcome` by conversion, not a lookup table.
`Result.HaltedProducer` is in every existing arm the value `persist` wrote as `current_producer`, so it equals `StepResult.Next` universally — the already-done short-circuit, the pause exits, both blocked arms, and the `Done`-terminal arm (which persists `def.Name`) all agree.

**`internal/shedengine/producer.go`** defines the seam: `Call(ctx) (Outcome, OutputPointer, error)`, `Outcome` ∈ {`done`, `stuck`}, `OutputPointer.Path` with `""` meaning no artifact.
`ProducerDef` carries `Name`, `Producer`, `OnStuck`, `OnDone`, `Segment`, `MaxBounces`.

**`internal/shedengine/status.go`** defines `Status` (`current_producer`, `state`, `error`, `pause_requested`, `activity`, `history`, `product`), `State` ∈ {`running`,`paused`,`done`,`blocked`,`failed`}, and `HistoryEntry` (`producer`, `outcome`, `output`, `at`).

**`internal/loomcli/run.go`** holds the bootstrap to extract.
Steps 1-3 are: `fabricengine.ReadOrigin` → `resolveParentBranch(recorded, found, parentFlag)` → conditional `fabricengine.WriteOrigin` → `loomshed.Seed(StatusPath, StatusLockPath, slug, parent)` tolerating `loomshed.ErrSeedExists` → `loomengine.VerifySeedOwnership` → `fabricengine.CommitAnchoredPaths` over `[loomengine.LoomStatusRel(), fabricengine.OriginRecordRel()]` with `fabricengine.EnvSyncOptions()`.
That commit is unconditional on every invocation and must precede any producer call.
`slug` comes from `seedSlug(c.location.WorktreeName)`.
Step 4's strand work is `c.reed.Up()` → `c.reed.Status()` → `resolveStatusStrandAction(statusResult.Strands)` → remove-then-add on `statusStrandReplace` (degrading a removal failure to a warning) → `c.reed.AddStrand` with `statusStrandDisplayName` and `statusStrandCmd(shell.ForGOOS(), exe)`.
**The extracted helpers are lock-agnostic: neither acquires nor releases `loomengine.LoomBootstrapLock`, and each verb wraps its own window around them.**
This matters because the lock's position differs between the two verbs and the helpers must not encode either choice.
`run`'s acquisition point is **unchanged by this task**: it acquires the bootstrap lock at its step 4, *after* the seed, the ownership verify, and `CommitAnchoredPaths` have already run, and holds it across the strand work, the driver spawn, and the run-lock handshake, releasing explicitly at step 7 rather than by `defer`.
`step` spawns no driver and runs no handshake, so it acquires the same lock around its strand block only and releases it **before** calling the producer — it must never hold it across a minutes-long LLM row.
The seed-and-commit helper therefore runs outside the bootstrap lock in both verbs, which is what `run` already does today; only the strand helper's caller-side window differs.

**An inherited property the plan writer must not misread**: the run lock is **not** the mutual-exclusion story for bootstrap.
`Step` acquires the run lock inside `shedengine`, after the whole bootstrap has already run, and the bootstrap lock is taken later than the seed/verify/commit block in `run` today.
So two concurrent `lyx loom step` invocations can both execute `CommitAnchoredPaths` unserialised before either reaches the run lock.
This is inherited from `run` unchanged, not introduced by this task, and it is accepted rather than fixed here: the commit is idempotent (`StageAndCommit` reports `committed == false` on an already-clean, already-tracked path), `Seed` is serialised by its own lock and signals `ErrSeedExists`, and widening the bootstrap lock to cover the commit would change `run`'s behaviour, which this task is explicitly not doing.
Do not present the run lock as covering bootstrap; it covers the producer call.

**`internal/loomcli/drive.go`** is the model for `step`'s shed construction: `fabricengine.Open` → `CurrentBranch` → `OriginURL` (empty on error, by the scalar-read-errors-refuse-or-defer-by-consumer rule) → `ReadOrigin` → `resolveLandingParent` → build `pushBranch` → `landingDeps(...)` into `c.env.Landing` → `loomrecipe.New(c.env, c.shedPaths)`.
`step` reuses this verbatim through a shared helper; `drive` then calls `shed.Run(ctx)` where `step` calls `shed.Step(ctx)`.

**`contracts/recipes/loom-recipe.yaml`** is the authority on per-row routing and budgets, and the plan writer should read it rather than infer them: the six review rows pin `max_bounces: 5`, the three validator rows (`Discussion-Validate`, `Plan-Validate`, `Plan-Revalidate`) carry an `on_stuck` to a writer row with no budget of their own (so they inherit the default ten), and **eight** rows carry no `on_stuck` at all and escalate to a human instead — the five gates `Preflight`, `Loom-Preflight`, `Batchifier`, `Publish`, `Finalize`, plus the three producer rows `Discussion-Write`, `Plan-Write`, and `Webster`.
The recipe's own header is the authority on that split and explains why the two groups differ; read it rather than this paragraph if the two ever disagree.
`Plan-Revalidate`'s `on_stuck` is `Plan-Write`, not `Plan-Bouncer` — the row reports mechanical format findings, so it routes to the writer.
Nothing in this task changes any of those values; they matter only because the skill's iteration cap is reasoned against them.

**`internal/loomcli/cli.go`** registers verbs in `Command()` via `parent.AddCommand(...)` and gates cheap verbs through `verbUsesLightweightWiring`.
`step` drives producers, so it takes the full `wire()` path and is **not** added to the lightweight set.
The parent command's `Long` enumerates the verbs and needs a `step` line and an `Example` entry; `Short` is mandatory on every command (CLI/Cobra Invariant).

**`internal/loomcli/status.go`** shows the envelope idiom: `clihelp.SetExit(ctx, output.Ok(out, map[string]any{...}))`, and refusal via `output.Err`. Every `RunE` starts with `if clihelp.ShouldAbort(cmd.Context()) { return nil }`.

**Doc surfaces that must move in the same commits** (per `CLAUDE.md`'s task-completion rule):

- `manifest/designs/loom-step.md` — flip the status line, replace the two Open questions with the settled contract and the operator-hand-back rule.
- `manifest/designs/loom.md` — line 472's `/ly-*` skills table row currently says "thin wrappers over `lyx loom run`"; the verb list in the module's prose needs `step`.
- `docs/overview.md` line 324 — the loom module line enumerates `lyx loom run|drive|status|pause|validate-discussion|validate-plan` and needs `step`. Line 331 lists loom's two interactive-handoff exceptions; `step` is **not** one (it emits a JSON envelope and hands the terminal to nothing), so that line stays as it is.
- `manifest/roadmap.md` — remove the Planned item on completion; the Done section is deliberately kept empty (cleared 2026-08-25).
- `manifest/designs/self-report-tier2.md` mentions this skill as the supervised-run substitute; check whether its cross-reference needs a pointer update.

**Markdown Link Integrity** is an enforced invariant over `manifest/` and `docs/`: every inline link's file part and `#anchor` must resolve.
New links added by the doc edits are covered by that test.

**Plugin surfaces**: `.claude-plugin/marketplace.json` lists plugins with `name`, `description`, `version`, `author`, `source`, `category`.
`plugins/scribe/.claude-plugin/plugin.json` is the shape to copy (`name`, `description`, `version`, `license` `Apache-2.0`, `author`, optional `hooks`).
`plugins/scribe/skills/INDEX.md` is a table of skill links.
A `SKILL.md` opens with YAML frontmatter carrying `name`, `description`, and — for an explicitly-invoked skill — `disable-model-invocation: true`, which `ly-supervise` wants (it is a deliberate operator action, never something a model should start on its own).

**Versioning**: unpublished loomyard plugins stay at `1.0.0`; no version bump anywhere for this task.

## Constraints

From `CONSTRAINTS.md`, the ones this task touches:

- **CLI / Cobra Invariant** — `step` is a cobra subcommand under the existing `loom` subtree, reached through the module's existing `Command()`/`RunCLI` seam. Non-empty `Short` is mandatory. Errors are JSON via `internal/output`, one object per line. Every `RunE` checks `clihelp.ShouldAbort` first. `step` takes **no** interactive-handoff exception and must not be added to that list.
- **Shed Producer-Seam Invariant** — `internal/shedengine` imports only stdlib, `state`, and `lock`; `StatusPath`/`LockPath`/`StatusLockPath` stay caller-supplied. `Step` adds no import and derives no path.
- **Told-Geometry Invariant** — `shedengine` and `loomrecipe` are bound packages: no direct `internal/lyxcwd` import from an engine. `step` gets its paths from `c.shedPaths` and `c.location`, resolved once in `loomcli`'s `PersistentPreRunE`, exactly as `run` and `drive` do.
- **Fabric Git Invariant** — `step`'s seed commit goes through `fabricengine.CommitAnchoredPaths` with a positive-only scoped pathspec, never raw git. This is inherited unchanged from the extracted `run` helper.
- **Live-Substrate Spawn Observability** — `step` itself starts no OS process, so it adds no spawn site; the producers beneath it already log their own. `reed.Up()`'s spawns are already covered inside `reedengine`.
- **Test Tier Purity Invariant** — no `gitexec`/`exec.Command`/`hubforge.NewHub` in untagged test files. `shedengine`'s `Step` tests are pure unit tests over temp-dir status files; any `loomcli` test that needs a real fabric carries the `integration` build tag and its package's `TestMain` calls `gitkit.HermeticGitEnv()` (Hermetic Git Test Environment Invariant).
- **Documentation Lifecycle** and `CLAUDE.md`'s task-completion rule — the module doc, `docs/overview.md`, and the roadmap move in the same commits as the code.
- **Markdown Link Integrity** — every new inline link in `manifest/`/`docs/` must resolve, anchor included.
- **Quarry CGO Requirement Invariant** — building and testing needs `CGO_ENABLED=1` and a C compiler on `PATH`.
- **Markdown: semantic line breaks** (`CLAUDE.md`) — one sentence per line in every `.md` this task writes, including the new `SKILL.md` and `INDEX.md`.

No new cross-cutting invariant is expected from this task.
If the extracted-iteration seam turns out to need one (for instance, "`Run` must never contain loop-body logic `Step` does not also execute"), record it in `CONSTRAINTS.md` in the same commit.

## Testing

**`internal/shedengine` — the TDD candidate.**
Write `Step`'s tests before the extraction, driving a fixture `Shed` whose producers are scripted fakes over a temp-dir status file.
One test per routing arm, each asserting both the returned `StepResult` and the persisted `Status`:

- a `Done` verdict with a non-empty `OnDone`: `Producer` = the called row, `Outcome` = `done`, `Next` = the `OnDone` target, `State` = `running`, one new history entry;
- a `Done` verdict with an empty `OnDone`: `State` = `done`, `Next` = the called row's own name (never empty);
- a `Stuck` verdict within budget: `Next` = the `OnStuck` target, `State` = `running`;
- a `Stuck` verdict with no `OnStuck`: `State` = `blocked`, `Reason` = "stuck with no OnStuck target";
- a `Stuck` verdict at the budget boundary — a budget of three bounces three times and blocks on the fourth, with `Reason` = "bounce budget exhausted";
- `pause_requested` set before the call: no producer called (`Producer` empty), `State` = `paused`, and the flag cleared on disk;
- a cancelled context with a producer error: `State` = `paused`, no history entry appended;
- a producer hard error: non-nil error returned, zero-value `StepResult`, `State` = `failed` on disk with the error text;
- an unrecognised outcome: non-nil error, `State` = `failed`;
- entry on an already-`done` file: `Producer` empty, `State` = `done`, no new history;
- entry on a `blocked`/`failed`/`paused` file: the producer **is** re-called and the resume write fires (this is how a human resumes);
- a producer returning an empty outcome with an error: no history entry appended at all (the out-of-vocabulary-value regression the current comment documents at length).

**The equivalence test is the one that protects the refactor**: build one fixture, drive it to a terminal state with repeated `Step` calls, drive an identical fixture once with `Run`, and assert the two resulting status files agree.
The comparison is over the **clock-independent** fields only: `current_producer`, `state`, `error`, `activity` (all three of `now`/`last`/`wait` — `composeActivity` derives them from `current_producer`, the last history entry's producer and outcome, the state, and the error text, and embeds no timestamp), and the history entries' `producer`/`outcome`/`output` sequence and length.
`history[].at` is **excluded and must be stated as excluded in the test's own comment**: it is `time.Now()` via `nowRFC3339()`, and `shed.go` pins the absence of an injectable clock on `Shed` as a deliberate design decision, so two drives at different wall-clock instants legitimately differ there.
A byte-identical whole-file assertion would pass only when both drives happened to land inside the same second — flaky by construction, and a test that fails for a reason the code is not responsible for teaches nothing.
Adding a clock seam to `Shed` purely to make this assertion stronger is rejected: it would widen the struct shape the design pins for a field the tests already assert structurally.
Also assert that `Run`'s `Result` agrees with the final `StepResult` — `Outcome` against `State`, `HaltedProducer` against `Next`, `Reason` against `Reason`, and the same history length.
Run it across at least a done-terminal fixture, a blocked-terminal fixture, and a bounce-then-done fixture.

**Lock behaviour**: assert `Step` returns `ErrShedBusy` when the run lock is already held, and — the property that makes stepping possible at all — that two sequential `Step` calls both succeed, proving the lock is released between them.
The existing `Run` suite (`run_routing_test.go`, `run_persist_test.go`, `run_pause_test.go`, `run_commitstatus_test.go`) is the guardrail that the extraction is behaviour-preserving and must pass unchanged, with no edits to accommodate the refactor.

**`internal/loomcli`** — envelope and refusal tests for the `step` verb, following `status_test.go`'s in-process `RunCLI` capture idiom:

- the full envelope key set is present and `continue` is `true` exactly when `state` is `running`;
- a hard producer error produces a failure envelope with `kind: "producer"` and a non-zero exit, matching `drive`;
- each refusal path emits its own `kind` and never reaches a producer — `busy` (run lock held), `ownership` (`VerifySeedOwnership` mismatch), `unseeded`, and `bootstrap`; assert the closed vocabulary so a new refusal cannot ship without a kind;
- a held run lock (a live driver) is refused with `kind: "busy"` and a message naming `lyx loom pause` as the remedy;
- the extracted bootstrap helper is called by both `run` and `step` — assert via the helper's own unit test that seeding is idempotent (`ErrSeedExists` tolerated) and that the commit pathspec is both `LoomStatusRel()` and `OriginRecordRel()` on every invocation;
- the extracted strand helper handles all three `resolveStatusStrandAction` branches, including degrading a removal failure to a warning rather than failing the verb.

Anything needing a real fabric or real reed goes in an `integration`- or `smoke`-tagged file, matching `loomcli`'s existing `smoke_test.go` / `wiring_commitstatus_integration_test.go` split.

**`cmd/lyx`** — add `"step"` to the loom case's `wantSubs` in `helptree_test.go`.
`sandbox_coverage_test.go` needs **no** change and this is settled, not conditional: its own comment states coverage is module-level and that each entry excludes a whole module rather than individual subcommands, so a new verb under an already-covered module adds no obligation there.
Confirm `registration_test.go`, `jsonhelp_test.go`, and `longlist_test.go` likewise need no change — `step` is an ordinary subcommand under the already-registered `loom` subtree with a non-empty `Short` and a `Long` carrying an `Example`.

**Docs** — the Markdown Link Integrity test covers the new links; run the full `go test ./...` (with `CGO_ENABLED=1`) rather than a package subset, because the doc-linting and constraint-chokepoint tests live outside the packages being edited.

**The skill itself is not unit-testable** and gets no test harness.
Its correctness bar is a manual end-to-end pass: drive one real task from an unseeded worktree to a terminal or blocked state with `/ly:ly-supervise`, with `lyx reed attach` open alongside, and confirm the loop reads each step's `output` artifact, halts on the first non-`running` state, and never mutates state itself.
Three behaviours must be exercised deliberately during that pass, because they are the ones a happy path never reaches:

- a step spanning a real multi-minute LLM row, confirming the backgrounded invocation survives it where a foreground call would not;
- an invocation killed mid-producer on an **attaching** row (kill the backgrounded process by hand) while its agent stays alive, confirming the skill reads `lyx loom status`, sees nothing moved, re-invokes, and that the re-invocation **attaches** to the live agent rather than spawning a second one — check the reed panes, not just the envelope;
- the same kill on the **`Webster`** row, confirming the skill hands back instead of re-invoking and says a live Master may remain;
- the same kill against a producer that finishes anyway, confirming the skill notices `current_producer`/`history_length` moved and continues from the fresh status rather than double-counting the step.

The `lyx selfreport create` path is verified by drafting an issue and **declining** at the confirmation prompt, confirming nothing is filed.

## Q&A log

- **Q:** Where does the one-iteration primitive live — extracted from `Run`, duplicated, or loom-local? **A:** [auto-pick] Extract `Run`'s loop body into `(*Shed).Step` on `shedengine`, rewriting `Run` as a loop over it. **Why:** the design doc demands a thin wrapper over existing dispatch, and a duplicated body guarantees drift from a routine carrying a dozen load-bearing crash-safety comments.
- **Q:** Should `Step` acquire the run lock per call, or should the lock move outside both? **A:** [auto-pick] `Run` keeps one lock for the whole call; `Step` acquires and releases per call; the shared body assumes the lock is held. **Why:** preserves `Run`'s semantics byte-for-byte while giving `step` correct mutual exclusion against a live detached driver.
- **Q:** What exactly is `step`'s return contract — the design doc's first open question? **A:** [auto-pick] `producer`/`outcome`/`output`/`next`/`state`/`reason`/`continue`/`history_length`/`status_file`. **Why:** exactly "which phase ran, pass/fail, terminal or not" plus the artifact pointer the supervisor must read; `continue` is derived so the skill never carries a copy of the state enum.
- **Q:** How does `step` report a hard producer error? **A:** [auto-pick] Error envelope, non-zero exit, mirroring `drive`. **Why:** CLI/Cobra Invariant parity, and the status file already records `state: failed` with the text.
- **Q:** Does `step` seed and commit, or refuse on an unseeded worktree like `drive` does? **A:** [auto-pick] It bootstraps idempotently — seed, ownership verify, fabric commit, `reed.Up`, status strand — but never spawns the driver and never attaches the terminal. **Why:** `lyx loom run` is the only other seeding verb and it spawns the driver that would race the stepper, so "run it first" is not an option a stepper has.
- **Q:** Should `step` also ensure the `lyx loom status` strand? **A:** [auto-pick] Yes, reusing `run`'s existing resolve/add/replace logic through a shared helper. **Why:** the design's third requirement is a human watching `lyx reed attach`, and the strand is what gives that pane anything to show.
- **Q:** May the supervisor skill advance past a stuck or blocked gate — the design doc's second open question? **A:** [auto-pick] Never; it halts and hands back to the operator on any non-`running` state. **Why:** a blocked state is a Go gate concluding a human is needed, and it matches crucible's "the push/merge decision is the operator's" rule.
- **Q:** What does "cleanup on a detected crash" concretely license the skill to do? **A:** [auto-pick] One re-invocation of `lyx loom step`, then hand back; no pane-killing, no lock removal, no file edits, no git. **Why:** loom already defines re-calling `current_producer` as the sanctioned resume, and the advisory run lock is reclaimed on process death, so nothing else is needed.
- **Q:** Where does the skill live and what is it called? **A:** [auto-pick] A new `plugins/ly` plugin at version 1.0.0, skill `ly-supervise`, invoked `/ly:ly-supervise`, registered in `.claude-plugin/marketplace.json`. **Why:** mirrors the millhouse `mill`/`mill-start` precedent the `/ly-*` convention was derived from; `ly-supervise` names the job, where `ly-step` would read as a one-shot verb wrapper.
- **Q:** Does the skill's loop need its own ceiling? **A:** [auto-pick] Yes, a hard cap of 40 steps, reported when hit. **Why:** loom's bounce budget bounds each producer but not the whole walk, and an LLM loop over a shell verb needs its own backstop; 40 is seventeen rows plus three review rounds in each of the three segments at two steps a round, sitting well under the ~107-step mechanical ceiling the recipe's real per-row budgets allow.
- **Q:** How does the skill issue a shell call that blocks for a whole multi-minute LLM producer? **A:** [auto-pick] Backgrounded invocation with stdout redirected to a `.scratch/` file, waited on and read after exit; `lyx loom step` itself stays an ordinary foreground one-shot verb. **Why:** every agent shell tool caps a foreground call well below an hour-long review row, so a foreground call would be killed mid-producer on exactly the rows the supervisor exists to watch.
- **Q:** What happens when a step invocation is killed or times out mid-producer? **A:** [auto-pick] Read `lyx loom status` once; if `current_producer`/`history_length`/`state` moved, continue from the fresh status; otherwise re-invoke the same step, capped at two consecutive interruptions per producer — except on the `Webster` row, where the skill hands back instead. **Why:** on the attaching rows (`shedadapters.SingleLLMProducer`/`Bouncer`/`BurlerProducer`) a re-invocation reattaches to the live agent, which is loom's shipped crash-resume; `WebsterProducer` alone *stops* a leftover Master rather than attaching, so re-invoking there restarts the most expensive row in the list and that cost is the operator's call.
- **Q:** Can the skill tell a producer fault from a bootstrap refusal in the failure envelope? **A:** [auto-pick] Yes — `step` emits failures via `output.ErrFields` with a closed `kind` vocabulary (`producer`, `busy`, `ownership`, `unseeded`, `bootstrap`), and the one-retry rule applies to `kind: "producer"` alone. **Why:** `ErrFields` already exists for per-verb keys and is used by four modules, so the discriminator costs one verb's envelope rather than a repo-wide change — and retrying a refusal fixes nothing.
- **Q:** When may the skill file a self-report, and does the operator gate it? **A:** [auto-pick] Only after the loop stops, at most once per run, drafted for the operator and fired only on explicit approval. **Why:** `lyx selfreport create` opens a real public issue on `Knatte18/loomyard` via the GitHub API — outward-facing and hard to reverse — and end-of-loop is also where the skill finally has the whole-run context.
- **Q:** Should `Step` be exported on `shedengine` or hidden behind a loom-only wrapper? **A:** [auto-pick] Exported on `shedengine`, available to every shed. **Why:** single-iteration dispatch is a property of the generic engine, and the Shed Producer-Seam Invariant is unaffected since no import is added.
