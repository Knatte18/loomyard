# Discussion: loom: interactive Discussion-Write

```yaml
task: 'loom: interactive Discussion-Write'
slug: loom-discussion-write-interactive
status: discussing
parent: loom-webster-review-producer
```

## Problem

`loom`'s `Discussion-Write` row always runs its interview agent autonomously.
`internal/loomcli/wiring.go`'s `wire()` passes the literal `true` for `autonomous` when it builds the `DiscussionSpec` closure, with a comment naming the "autonomous-only Shared Decision" that put it there.
Everything downstream of that literal is already built for both modes — `loomengine.DiscussionSpec` takes an `autonomous bool`, `composePrompt` selects between two fully-written `modeRules` blocks, the `loom-template-discussion` stencil carries a `{{.mode_rules}}` marker, and `shuttleengine.Spec.Interactive` is set to `!autonomous` and honoured by the Claude engine.
The only thing missing on the prompt side is a way to say "interactive".

The reason it was hardcoded is not the prompt plumbing, it is resume.
`manifest/designs/loom.md`'s crash-recovery section records the trap explicitly and leaves it open for this task:

> A resume-on-output-files check that reports `Done` whenever both discussion files exist cannot distinguish an interrupted interview (needs the interview to resume) from a `Discussion-Validate` bounce re-entering the row with both files already present — a naive fix ping-pongs the two cases until the bounce budget is exhausted.
> Solving this is the Planned `loom: interactive Discussion-Write` item's own problem to close, not resolved here.

Exploration for this task found that the doc's trap is one of **two** blocking defects, and that the second one is the more immediate of the pair.

**Defect A — an ask is terminal, so an interactive interview dies at its first question.**
`internal/shuttleengine/wait.go`'s `pollEventsTick` classifies the last parsed event as `OutcomeDone` when every output file exists and `OutcomeAsking` otherwise, and `Run.Wait` returns on the first non-empty classification.
`internal/shedadapters/singlellm.go` maps `shuttleengine.OutcomeAsking` onto `shedengine.Stuck`.
`Discussion-Write` carries no `on_stuck` edge in `contracts/recipes/loom-recipe.yaml`, so a `Stuck` there escalates the whole run to blocked.
An interactive interview asks its first question batch, ends its turn without having written `decision-record.md`/`support-log.md`, and the run is over before the operator has typed a character.
`Spec.Interactive` today changes only the launch command's flags and the settings-hook shape;
it does not reach the wait loop at all.

**Defect B — resume respawns instead of re-attaching.**
`shedengine.Run` resumes by re-`Call`ing whatever `current_producer` names.
`SingleLLMProducer.Call` unconditionally archives every existing output file to a timestamped sibling and starts a fresh `shuttle.Run`.
That is correct and cheap for an autonomous producer — the round is idempotent, so a fresh agent is deterministic — and it is exactly what `loom.md` prescribes as ladder step 3.
For an interview it throws away every answer the operator already gave.
The on-disk Shed state after a crash mid-interview is byte-identical to the state after a `Discussion-Validate` bounce: both leave `current_producer: Discussion-Write`, `state: running`, and a last history entry that is not `Discussion-Write`'s own.
That identity is precisely why a status-file-only fix cannot separate them, and why the doc's naive file-existence shortcut ping-pongs.

Why now: the roadmap's first Planned item, and the last thing keeping loom's discussion phase from being usable the way a human actually wants to use it.

## Scope

**In:**

- A mode selector in `loom.yaml` (`discussion_interactive`, default `false`) carried through `loomengine.Config` into `internal/loomcli`'s `wire()`, replacing the hardcoded `true` at `internal/loomcli/wiring.go:139`.
- A new `shuttleengine.Spec` field, `AwaitOperator bool`, that makes `Run.Wait` treat an `Asking` classification as non-terminal and keep polling.
- A new `shuttleengine.Runner.Attach(Spec) (Result, bool, error)` that finds a still-live run matching the spec's output files and waits on it instead of starting a new one.
- Widening the `shedadapters.Shuttle` seam by that one `Attach` method, and re-ordering `SingleLLMProducer.Call` to probe-then-archive-then-run.
- Test coverage at every seam touched, plus a `loomrecipe` resume test pinning that the `Discussion-Validate` bounce path still respawns.
- Docs in the same commit, at every site the reversal reaches — this design changes the *discipline* `manifest/designs/loom.md` states, not merely the one paragraph that flagged the trap:
  - `manifest/designs/loom.md:286`, the sentence "**loom resumes on output FILES, not on live processes.**"
    It becomes a two-part rule: loom resumes on output files *and* on live-agent evidence, with file existence never used on its own to skip a step.
  - `manifest/designs/loom.md:290-292`, ladder step 1 ("Is there a complete output file? → the step finished").
    It stays in the ladder but is narrowed: it is checked *inside* an attached run, never as a producer-level shortcut — see `ladder-step-1-survives-only-inside-attach` below.
  - `manifest/designs/loom.md:293-294`, ladder steps 2 and 3, which this task implements for the first time and which the doc should stop describing in the future tense.
  - `manifest/designs/loom.md`'s "The interactive-mode trap" paragraph, replaced by the resolution.
  - `manifest/designs/loom.md:318`, the Graceful-pause cross-reference to "the same resume-on-files discipline as crash recovery", which must track the reworded rule.
  - `manifest/designs/shed.md:30`, which restates the resume-on-output-files rule for the generic `ShedProducer` contract this task changes.
  - **The section anchor `#crash-recovery--resume-on-output-files-not-live-processes` is preserved unchanged.**
    `manifest/roadmap.md:17` and `manifest/designs/loom.md:318` both link it, so the `Markdown Link Integrity` invariant binds: the heading text is not to be edited even though the rule beneath it is.
    If a future editor judges the heading actively misleading, renaming it is a separate change that must update both inbound links in the same commit.
  - `internal/shuttleengine/doc.go` and `internal/shedadapters/doc.go`.
  - `docs/overview.md:319`, which enumerates `loom.yaml`'s keys by name ("the `discussion`/`plan` role model-specs and `discussion_timeout_min`/`plan_timeout_min`) and says the module "reconciles via `lyx config reconcile`".
    Both halves need touching: `discussion_interactive` joins the key list, and the reconcile mention should carry `--apply` per the migration note below.
    `:320`'s Discussion-producer description should also gain the fact that the producer now has two modes.
  - `manifest/roadmap.md`, moving the item off Planned.

**Out:**

- Interactive mode for any other producer.
  `Plan-Write` hardcodes `Interactive: false` internally and stays that way;
  `Discussion-Bouncer`/`Discussion-Burler` and both other review segments stay autonomous.
- Decoupling `--dangerously-skip-permissions` from `Spec.Interactive`.
  An interactive run keeps today's behaviour of not passing it, so the operator approves tool calls in the pane.
- Fixing `Discussion-Validate`'s discarded findings, or feeding a bounced-into `Discussion-Write` its prior artifacts or the validator's complaints.
  A bounce still re-interviews from scratch;
  that is a known wart recorded below, not this task's subject.
- Any change to `AskUserQuestion` handling.
  The stencil already forbids the tool in both modes and the interactive settings hook that records it stays exactly as it is.
- The `lyx shuttle run --interactive` CLI's own outcome contract.
  `--interactive` keeps returning `asking`;
  the new behaviour rides on the new field, which the CLI does not expose.
- Any new persisted field in `loomengine.Status` / `contracts/specs/loom-status-spec.md`.

## Decisions

### mode-selector-lives-in-loom-yaml

- Decision: add `discussion_interactive: false` to `internal/loomengine/template.yaml` and to `loomengine.Config` as `DiscussionInteractive bool`, and pass `!cfg.DiscussionInteractive` as `DiscussionSpec`'s `autonomous` argument in `wire()`.
- Rationale: `lyx loom run` does not run the phase machine — it spawns a **detached** `lyx loom drive` child (`internal/loomcli/run.go`, step 5) and then hands the terminal to tmux.
  A flag on `run` therefore cannot reach the process that builds the wiring without being persisted somewhere the child reads.
  `loom.yaml` is already loaded by both `run` and `drive` through the same `resolvePersistentPreRun`, it is already per-worktree (it lives in the weft's `_lyx/`, so it is per-task by construction), and `Config` already carries the discussion role's other per-task knobs (`discussion`, `discussion_timeout_min`) right beside it.
  Adding the key is one template line, one struct field, one `wire()` expression.
- Rejected: a `--interactive` flag on `lyx loom run` persisted into `loomengine.Status`'s product blob.
  It is the more discoverable spelling, but it costs a new field in a documented cross-module contract (`contracts/specs/loom-status-spec.md`), a `loomshed.Seed` signature change, and a t=0-immutability property nothing in this design actually needs — see `mode-is-not-load-bearing-for-resume` below.
- Rejected: both a config key and a flag that overrides it.
  Two spellings for one boolean, with a precedence rule to document and test, for no capability the single key lacks.

### mode-is-not-load-bearing-for-resume

- Decision: the mode is read fresh on every `wire()`, and flipping it between a crash and a resume is permitted and benign.
  No code compares the current mode against the mode a live run was started with.
- Rationale: mode affects exactly two things — which `modeRules` block the prompt carries, and whether `AwaitOperator` is set on the spec.
  The resume decision (attach vs. respawn) is made purely on live-agent evidence, never on mode.
  So a flip means only that the *next* spawn is interviewed differently, which is what an operator flipping the key would want.
  A live run that was launched autonomous is re-attached as what it is;
  its already-running agent has the prompt it has.
- Rejected: pinning the mode into the run's persisted state and refusing a mismatched resume.
  It buys an immutability nothing depends on and turns a benign operator edit into a hard refusal.

### asking-non-terminal-via-a-new-spec-field

- Decision: add `AwaitOperator bool` to `shuttleengine.Spec`.
  When it is true, `Run.Wait` does not return on an `OutcomeAsking` classification — it logs the observation and keeps polling, so the run still terminates on `OutcomeDone` (every output file exists), on `OutcomeDied`, on a liveness mechanism failure, or on `OutcomeTimeout`.
  `loomengine.DiscussionSpec` sets it to `!autonomous`, alongside the existing `Interactive: !autonomous`.
- Rationale: this is the field that makes an interview possible at all.
  An interviewing agent ends a turn every time it asks a question batch, and every one of those turn-ends is an `Asking` classification under today's rules.
- Rejected: widening `Spec.Interactive` to also mean "asking is non-terminal".
  What the separation actually buys, stated precisely rather than generously: it preserves the real-time asking signal for **`lyx shuttle run --interactive`**, and for that caller only.
  `internal/shuttleengine/claudeengine/settings.go` installs a `PreToolUse(AskUserQuestion)` hook in every interactive run that appends to `events.jsonl` specifically so "the run loop can classify it as a real-time asking signal instead of waiting for the timeout" (its own package comment).
  Widening `Interactive` would suppress that classification for every caller, turning the CLI's `--interactive` probe from fast-returning into a blocking wait.
  Note honestly that loom's own interactive `Discussion-Write` sets `AwaitOperator: !autonomous` alongside `Interactive: !autonomous`, so on a loom interactive run that real-time signal is dropped anyway — deliberately, since the whole point there is to wait rather than report back.
  The two fields are still the right shape, but for the narrower reason: "an operator is present" (which governs launch flags and the recording hook) and "wait for the operator rather than reporting back" (which governs the wait loop) are orthogonal, and one caller wants the first without the second.
- Rejected: keeping `Asking` terminal and having `SingleLLMProducer` loop, re-entering `Wait` after each ask.
  The producer would have to reconstruct the run handle it just consumed, re-derive the event offset, and re-derive the deadline — all of which are `shuttleengine`'s private state.
  The wait loop is where the polling already lives.
- Note on the failure mode this accepts: an interactive run whose agent is genuinely wedged now hangs until `discussion_timeout_min` rather than reporting `Stuck` promptly.
  That is the correct trade for a mode whose entire premise is that a human is watching the pane;
  the human sees the wedge directly, and `Wait` logs each observed ask so the driver log records it too.

### resume-discriminates-on-live-agent-evidence-only

- Decision: the crash-vs-bounce question is answered by asking whether an agent for this producer is **still alive**, never by asking whether the output files exist.
  Concretely: scan the shuttle run-dir root for a `run.json` whose `OutputFiles` set equals the spec's, then ask reed whether that record's `StrandGUID` is still tracked **and** still holds a live pane.
  A match means attach and wait;
  no match means archive the stale outputs and spawn fresh, exactly as today.
- Rationale: this is `manifest/designs/loom.md`'s own crash-recovery ladder, applied where it was never actually implemented — step 2 ("is the agent's session still alive? → re-attach, just wait on its `Stop` hook; do **not** respawn — that would duplicate") followed by step 3 ("dead, no output → respawn a fresh agent").
  It is also the only signal that differs between the two states.
  After a `Discussion-Validate` bounce the previous `Discussion-Write` run reached `OutcomeDone`, so `Run.finalize` ordinarily removed its strand and deleted its run dir;
  there is nothing live to find, and the row respawns.
  "Ordinarily" is load-bearing: that cleanup is skipped entirely under `spec.KeepPane`, and both of its steps are best-effort (a failed `RemoveStrand` or `RemoveAll` is a `logger.Warn`, not an error), so the leftover-directory case is real and is given its own disposition in `leftover-run-dir-from-a-completed-run` below.
  After a crash mid-interview no cleanup ran, so the run dir and its `run.json` survive and — because the tmux server is detached and outlives the driver process — the pane usually does too.
  Ladder step 1 ("is there a complete output file?") is deliberately **not** implemented at the producer level: it is the exact shortcut the doc's trap warns about.
  It does not buy *nothing*, though — and the honest statement of the trade matters, because the previous phrasing of this decision was wrong.
  Step 1 would buy the **completed-crash window** described in `accepted-residual-the-completed-crash-window` below, at the price of the bounce ping-pong.
  We accept losing it: the ping-pong is a hard failure on an ordinary, frequent path, and the window it would rescue is narrow and costs only rework.
- Rejected: an "interview in progress" marker file written at spawn and cleared on `Done`.
  It is a second, hand-maintained liveness record that a crash can leave stale in the one situation it exists to describe.
- Rejected: having `Discussion-Validate` leave a bounce marker for `Discussion-Write` to read.
  It makes the fix specific to one row's one predecessor, when the underlying defect (respawning over a live agent) belongs to every `SingleLLMProducer` row.

### attach-lives-in-shuttleengine

- Decision: add `Runner.Attach(spec Spec) (Result, bool, error)` to `internal/shuttleengine`.
  It resolves the run-dir root, scans for a `run.json` whose `OutputFiles` equal `spec`'s resolved output files, confirms via reed that the record's `StrandGUID` is tracked with a live pane, reconstructs a `*Run` over the persisted `EventsPath`/`StrandGUID`/`SessionID`/`RunDir`, and returns `run.Wait()`.
  The bool reports whether a live run was found;
  `false` comes back with a zero `Result` and a nil error, and means "nothing to attach to, start one".
- Rationale: every ingredient is already private to this package — `runDirRoot`, `loadRunState`, `findRunByStrand`'s scan-and-skip discipline, the reed liveness probe, the `Run` struct, and `Wait`'s poll loop.
  Reimplementing any of it above the seam would duplicate `shuttleengine`'s most carefully-reasoned code (see the `errStrandNotTracked` / `errStrandPaneBindingCleared` comments) in a package that cannot see it.
- Rejected: `shedadapters` scanning run dirs itself.
  It would have to learn the run-dir layout, the `run.json` schema, and reed's three-way liveness answer, none of which are exported.
- Rejected: a dedicated `loomshed` producer or decorator for the attach behaviour.
  The behaviour belongs to every `SingleLLMProducer`, not to `Discussion-Write` alone, so putting it in the loom-specific package would leave `Plan-Write` and the generic `SingleLLM` row with the duplicate-agent bug.
- **On widening the shared seam rather than adding an optional one.**
  `shedadapters.Shuttle` is not `SingleLLMProducer`'s private seam: `shedrecipe.Env.Shuttle` feeds the `Bouncer` row (`entries_bouncer.go:101,141`) and the `PlanWrite`, `DiscussionWrite`, and generic `SingleLLM` rows, and the landing wiring supplies the same `*shuttleengine.Runner` to all of them.
  Adding `Attach` puts a method on that seam which only `SingleLLMProducer` calls.
  Widen it anyway: the sole production implementor is `*shuttleengine.Runner`, which gains the method for free, and every other implementor is a test fake in this repo — so the cost is a one-line addition to each fake, paid once.
- Rejected: keeping `Shuttle` at one method and type-asserting an optional `Attacher` interface inside `Call`.
  It makes the attach behaviour silently absent for any implementor that forgets it, which is exactly how the duplicate-agent bug would come back — and a compile error in a test fake is a better failure than a producer that quietly stops probing.

### attach-reconstructs-the-run-explicitly

A `*Run` built by `Start` carries four pieces of state `RunState` does not persist: `offset`, `deadline`, `clock`, and `Wait`'s two loop-local startup variables (`started`, `startupDeadline`).
`Attach` reconstructs a `Run` without ever calling `Start`, so each is decided here rather than left to whatever the zero value happens to be.
Getting any of them wrong destroys the interview that `Attach` exists to save.

- **`offset` starts at `0` — the whole `events.jsonl` is deliberately replayed.**
  Rationale: replay is harmless in exactly the case where it could be wrong, and load-bearing in the case that would otherwise be missed.
  `pollEventsTick` classifies the *last* event of the batch it parses and checks `allOutputFilesExist` first, so a replay whose backlog ends in a completion classifies `OutcomeDone` — which is how an agent that finished while the driver was down is noticed at all.
  A replay whose backlog ends in an ask classifies `OutcomeAsking`, and that is correct in both modes: under `AwaitOperator` it is non-terminal, so a stale ask the operator has already answered costs one dropped tick and nothing else;
  without `AwaitOperator` nothing in loom ever sends a stopped agent new input, so a backlog ask is a genuine terminal ask and `Stuck` is the right verdict.
- Rejected: seeding `offset` at the file's EOF so only post-attach events classify.
  Named failure mode: a terminal `Stop` — including a *successful* one — that landed while the driver was down is then never observed.
  A finished autonomous run would sit unclassified until its whole deadline expired and then report `OutcomeTimeout`, converting a completed step into a failure.
- **`started` is seeded `true`, and `startupDeadline` is therefore never consulted.**
  Rationale: `Wait` seeds `started := false` and `startupDeadline := now + StartupTimeoutS` (`wait.go:126-129`), and `checkLivenessTick`'s `if *started { return "", nil }` short-circuit is the only thing standing between an attached run and the startup probe.
  Without the seed, an attached run re-runs `CapturePane` + `engine.Startup` against a pane that is mid-turn;
  `claudeengine.Startup` returns `StartupPending` for any capture lacking `❯` or `shortcuts` (`startup.go:26-37`), so `classifyStartupWindow` reports `OutcomeDied` one `startup_timeout_s` after attach — killing a live interview roughly ninety seconds in.
  Worse, a capture that happens to trip `trustDialogNeedles` would return `StartupTrustPrompt` and *play the dismiss key sequence into a live agent's pane*.
  Seeding `started = true` is also simply true: `Attach` only proceeds having confirmed a persisted `run.json` with a `SessionID` and a reed strand holding a live pane, which is strictly stronger evidence of "it started" than the capture heuristic provides.
  The not-tracked / not-live branches of `checkLivenessTick` sit *above* the `started` short-circuit, so an attached run keeps full liveness coverage;
  only the startup probe is skipped.
- **`clock` is the runner's production clock**, injectable in tests exactly as `Start`'s is.
- **`deadline` is `now + spec.Timeout`** — see `attach-restarts-the-deadline` below.
- **`spec` is the caller's own normalized spec**, not one reconstructed from `run.json`.
  `RunState` persists only `OutputFiles` out of the whole `Spec`, so reconstructing one is not even possible;
  and it is not wanted, because `Attach` is only ever called with the same spec the row would otherwise have passed to `Run` (`SingleLLMProducer` sources both from one `SpecSource`).
  Three of that spec's fields are read by machinery an attached run reaches, so each gets an explicit disposition rather than being inherited by accident:
  - **`Display.Anchor`** — read by `checkLivenessTick`'s `errStrandPaneBindingCleared` carve-out (`run.spec.Display.Anchor != render.AnchorHidden`), where a hidden strand's empty pane id is normal rather than a cleared binding.
    It must therefore carry `Spec.validate`'s `AnchorBelowParent` default (`spec.go:157-159`) — the *third* normalization, alongside the two in `attach-normalizes-the-spec-it-matches-on`.
    An empty `Anchor` on the attach path would make every binding-cleared strand take the hidden-strand carve-out and classify `OutcomeDied` instead of surfacing the sentinel.
  - **`KeepPane`** — honoured from the caller's spec, governing the attached run's own `Done` cleanup exactly as it governs a started run's.
  - **`ForkSubagents`** — honoured from the caller's spec.
    The post-`Done` `AuditForks` call it gates runs against `run.state.SessionID`, which comes from the persisted record, so the audit covers the whole session including the pre-crash portion.
  `Timeout` is covered by `attach-restarts-the-deadline`, and `Prompt`/`Model`/`Effort`/`Version`/`Role`/`Round`/`Parent` are launch-time-only and unread by `Wait`, so they need no disposition.
  A caller passing a spec whose `Display.Anchor` disagrees with how the live run was actually launched is a caller error, not something `Attach` reconciles;
  no production caller can do it, since each row has exactly one `SpecSource`.

### ladder-step-1-survives-only-inside-attach

- Decision: `allOutputFilesExist` remains a terminal signal, but only *inside* an attached or started `Run`'s own wait loop, where it already lives (`pollEventsTick`, and both not-tracked/not-live branches of `checkLivenessTick`).
  It is never lifted to the producer level as a "files exist, therefore `Done`" precheck.
- Rationale: this is the precise line between `loom.md`'s ladder step 1, which is sound, and the doc's trap, which is not.
  Inside a run, file existence answers "did *this agent* finish", and there is an agent to attribute it to.
  At the producer level it answers only "do files exist", which a `Discussion-Validate` bounce makes true without any agent having run — the ping-pong.
  The `manifest/designs/loom.md` edit must make this distinction explicit rather than deleting step 1, since step 1 is what lets an attached run notice a completion that landed during the outage.

### accepted-residual-the-completed-crash-window

- Decision: a crash in the window between `Run.finalize` returning `OutcomeDone` and `shedengine` persisting that outcome causes the completed step to be **re-run from scratch** on resume.
  This is accepted, recorded, and not fixed here.
- The window, precisely: `shedengine.Run` calls `def.Producer.Call(ctx)` and persists history and the next `current_producer` only *after* it returns — deliberately as a single write, since "written as two writes, a crash between them leaves `current_producer` still naming the producer that just finished" (`internal/shedengine/run.go:126-130`, its own comment).
  A crash inside that window therefore leaves `current_producer: Discussion-Write` with both output files present and complete, and — because `finalize` already ran its cleanup — nothing live to attach to.
  `Attach` correctly reports `found == false`, and the producer archives a *finished* interview and re-interviews from the top.
  `leftover-run-dir-from-a-completed-run` routes the variant where the run dir survived to the same respawn, for the same reason.
- Rationale for accepting: the only thing that would rescue it is a producer-level file-existence check, which is the doc's trap verbatim — on a `Discussion-Validate` bounce the files are also present and complete-looking, and returning `Done` there ping-pongs until the bounce budget blocks the run.
  Trading a frequent hard failure for a narrow one that costs only rework is the wrong direction.
- Cost, stated plainly so nobody discovers it as a surprise: in interactive mode the operator answers the whole interview twice.
  The window is small (one status-file write) and requires a crash inside it, but it is not zero.
- A future fix, if this ever bites in practice, is a marker distinguishing "these files were produced by a run that reached `Done`" from "these files are merely present" — for instance recording the completing run's id alongside the artifacts.
  That is a new durable-state contract and is deliberately not designed here.

### attach-is-unconditional-not-interactive-only

- Decision: `SingleLLMProducer.Call` probes `Attach` on **every** call, regardless of `spec.Interactive` or `spec.AwaitOperator`.
- Rationale: respawning over a still-live agent is a correctness bug in autonomous mode too, not merely a UX annoyance in interactive mode.
  It puts two agents on one worktree writing the same output files, which is the exact duplicate-agent hazard `internal/shuttleengine`'s own `errStrandNotTracked` and `sweepOrphansOpportunistic` comments spend paragraphs avoiding one layer down, and which `loom.md`'s ladder step 2 forbids in as many words.
  Gating the fix on interactive would leave the documented discipline unimplemented for the majority of rows.
- Rejected: gating on `spec.Interactive`.
  Smaller blast radius, but it knowingly ships the duplicate-agent path for `Plan-Write` and every generic `SingleLLM` row.
- **The `Bouncer`/`Burler` review rows are explicitly out of scope, and they keep the duplicate-agent path.**
  They drive the same `Shuttle` seam (`shedrecipe/entries_bouncer.go:141` hands them `env.Shuttle`) but do not go through `SingleLLMProducer`, so widening that producer does not reach them and this task does not touch their own spawn/resume behaviour.
  The reason for leaving them is scope, not a claim that they are safe: the argument above — respawning over a live agent is a correctness bug for every row — applies to them too.
  Fixing them means auditing `Bouncer`'s round-state and run-directory handling, which the sibling roadmap item (`Discussion-Burler`'s `fix-scope: source`) is already scheduled to open, and which has its own stale-run-directory defect recorded there.
  Doing both in one task would have two changes editing the same rows.
- Blast radius, stated plainly: this changes `Plan-Write` and any future generic `SingleLLM` row as well as `Discussion-Write`.
  The behaviour change is confined to the case where a live matching run exists at `Call` time, which today only happens on a resume after a crash — on the ordinary first call the probe finds nothing and the existing archive-then-run path is taken unchanged.

### probe-before-archive

- Decision: `SingleLLMProducer.Call`'s order becomes: entry-check context → build spec → reject relative `OutputFiles` → **probe `Attach`** → if attached, map its `Result` through the existing outcome switch and return → else `archiveStaleOutputs` → `shuttle.Run` → same outcome switch.
- Rationale: archiving is a rename of the very files a live agent may be about to write.
  Doing it before the probe would break the live run's file contract — `Wait` polls for bare existence at the spec's paths, so a renamed file means the run can never classify `Done` — and would do so in exactly the case the probe exists to protect.
- Rejected: archiving first and restoring on a successful probe.
  A rename-and-maybe-rename-back window over a live agent's output files, for no benefit.

### attach-restarts-the-deadline

- Decision: an attached run's deadline is `now + spec.Timeout`, computed at attach time.
  `RunState.CreatedAt` is not used for deadline arithmetic.
- Rationale: `Run.finalize` cleans up only on `OutcomeDone`, so a run that hit `OutcomeTimeout` leaves both its strand and its run dir behind.
  If an attached run inherited `CreatedAt + Timeout`, that surviving run would be re-attached on the next resume and immediately re-classify `OutcomeTimeout`, on every resume, forever — a fail loop with no operator escape short of killing the pane by hand.
  Restarting the clock also matches what the timeout means: a bound on one continuous unattended wait, and a resume is a new, operator-initiated wait.
- Rejected: `CreatedAt + spec.Timeout`.
  Strictly more honest about total elapsed time, and it creates the fail loop above.
- Rejected: refusing to attach to a run already past `CreatedAt + Timeout`.
  It avoids the loop by falling through to respawn, which for an interactive run discards the interview for the one reason the operator can least control.

### attach-normalizes-the-spec-it-matches-on

- Decision: before scanning, `Attach` performs two of `Spec.validate`'s three normalizations on its own copy of the spec — the third, `Display.Anchor`'s default, is covered in `attach-reconstructs-the-run-explicitly` above because it matters to the *attached run*, not to the match — resolving every relative `OutputFiles` entry against the runner's `worktreeRoot`, and replacing a zero `Timeout` with `cfg.RunTimeoutMin` minutes — and performs none of its checks.
  Matching is then **set** equality between the resolved absolute `OutputFiles` and the `run.json` record's `OutputFiles`, order-insensitive and duplicate-insensitive.
- Rationale: `Attach` never calls `Start`, so it never reaches `Spec.validate`, which is today the only place either normalization happens (`spec.go:126-155`).
  Skipping the path resolution would fail to match a caller that passed relative entries against a `run.json` that always records absolute ones.
  Skipping the `Timeout` default is worse: a zero `Timeout` would make the attached run's deadline `now + 0`, so it would classify `OutcomeTimeout` on its very first tick — the exact footgun `Timeout`'s own doc comment warns about, reintroduced on the attach path.
  Set rather than ordered equality because `RunState.OutputFiles` is an ordered slice recording whatever order the caller happened to supply, and two specs naming the same files in a different order describe the same run.
- Rejected: running the full `Spec.validate`.
  Its reject-if-already-exists check would refuse every attach whose agent has written one of its two output files — and a partially-written output set is a normal mid-interview state.
  Its negative-`Timeout` rejection is worth keeping in spirit;
  the plan may reuse it, but it must not drag the existence check along.

### mechanism-failures-do-not-attach-and-do-not-blindly-respawn

- Decision (promoted from what was previously left open in Technical context): reed's three answers about a matched record's strand are given three distinct dispositions, and the disposition is stated here rather than deferred to the plan.
  - **Tracked, with a live pane** → attach.
  - **No matching `run.json` at all** → `found == false`, nil error → archive and respawn.
    This is both the ordinary first-call case and the `Discussion-Validate` bounce case.
  - **Matched record, but reed does not track the strand (`errStrandNotTracked`) or tracks it with no pane binding (`errStrandPaneBindingCleared`)** → resolved by age, below.
  - **reed's state file absent or unreadable** → error, never `found == false`, at any dir age.
    An absent strand table is not evidence that anything is dead, and respawning on it is precisely the duplicate-agent hazard;
    `sweepOrphansOpportunistic` already refuses to act on either answer for the same reason, and `Attach` mirrors that refusal.
  - **A matched record whose output files all already exist**, reached only in the non-attachable branches → `found == false`, at any dir age.
    See `leftover-run-dir-from-a-completed-run` below.
- **Decision on how the absent case is observed, because it is not observable through the seam `Attach` would naturally reach for.**
  `Attach` must read reed's state file **directly**, via `reedengine.LoadState(filepath.Join(anchorPath, lyxdirs.DotLyxDirName))` — the identical call `sweepOrphansOpportunistic` already makes in `run.go` — and must **not** try to derive the answer from `ReedOps.Status()`.
  `ReedOps` (`internal/shuttleengine/reed.go:18-25`) exposes only `Status()`, and `reedengine`'s `loadOrInitStateLocked` (`spawn.go:187-196`) substitutes an empty `&ReedState{}` whenever `LoadState` returns "not found".
  `Status()` therefore *succeeds with zero strands* for an absent state file, which is byte-for-byte indistinguishable from a healthy table that simply does not list this guid — so an absent-file disposition built on `Status()` could never fire, and the test for it could never pass.
  Only the unreadable/corrupt case surfaces as an error through `Status()`, because `LoadState` returns `unreadableStateError` there (`state.go:130-133`).
  `LoadState` returns `(nil, nil)` for absent and `(nil, err)` for unreadable, which is exactly the three-way answer this decision needs, and reaching for it costs nothing: `run.go` already imports `reedengine` and `lyxdirs` and already makes this call.
- Decision on the age question, which is what stops this from deadlocking: a matched record whose strand is untracked or binding-cleared is treated as `found == false` (archive and respawn) **once its run directory is older than `2 * StartupTimeoutS`** — the identical `minAge` guard `sweepOrphans` already applies — and as an error while it is younger than that.
- Rationale: erroring unconditionally on those two answers deadlocks resume permanently.
  The only thing that ever removes such a directory is `sweepOrphansOpportunistic`, which runs inside `Runner.Start` — which the error path never reaches — so a single stale `run.json` whose strand reed has forgotten would fail every subsequent `lyx loom run` forever, with no in-band recovery.
  Reusing the sweep's own `minAge` predicate resolves it without inventing a second policy: the repo has already decided that an untracked run dir past that age is an orphan, to the point of *deleting* it.
  Treating it as respawnable is strictly gentler than what the sweep already does to it, and the respawn path reaches `Start`, so the sweep then clears the directory as a side effect.
  Below `minAge` the error stands, because that window is exactly the concurrently-starting run the guard was written to protect.
- Both error dispositions must log loudly — naming the run dir, the strand guid, and `lyx reed status` — because the operator escape is out of band: check `lyx reed status`, and either kill the orphaned pane or delete the run directory by hand.
  That is the same remedy reed's own corrupt-state error already prescribes.
- Rejected: `found == false` for untracked/binding-cleared regardless of age.
  It respawns over an agent that may still be working in a pane reed can no longer address, which is the hazard both sentinels were written to prevent.
- Rejected: an unconditional error with a new CLI verb to clear stale run dirs.
  New surface for a state the existing sweep already handles.

### leftover-run-dir-from-a-completed-run

- Decision: in the **non-attachable** branches only (untracked strand, or binding-cleared strand), a matched record whose `OutputFiles` **all exist on disk** is treated as `found == false` at any directory age, ahead of the age rule — a leftover from a run that already finished, not an interrupted one.
  The strand-liveness question is answered **first**, so a run that is tracked-and-live with all its output files written is *attached*, never treated as leftover;
  its own first tick then classifies `OutcomeDone` off the existing `allOutputFilesExist` check, which is the correct outcome and archives nothing.
- Rationale: `resume-discriminates-on-live-agent-evidence-only` claims that a bounce finds nothing live because the prior run reached `Done` and `finalize` cleaned it up.
  That claim is weaker than it reads.
  `Run.finalize` skips cleanup entirely when `spec.KeepPane` is set (reachable via `lyx shuttle run --keep-pane`), and when it does run, both `RemoveStrand` and `os.RemoveAll` are best-effort — each failure is a `logger.Warn`, not an error.
  So a completed run can leave its directory behind with its strand already gone.
  On a fast bounce that directory is untracked *and* younger than `2 * StartupTimeoutS`, so the bare age rule would **error** — blocking the bounce on a run that finished perfectly well.
  Output-file completeness is the file contract's own definition of "that run finished", so it is the right tie-breaker, and it is safe precisely because it is consulted only after liveness has already been ruled out.
- This does not reintroduce the doc's trap.
  The trap is a producer-level "files exist → `Done`" shortcut, which fires on a bounce where no agent ran.
  This rule fires only when a matching `run.json` exists — i.e. only when some agent demonstrably *did* run for this exact output-file set — and its verdict is "respawn", never "`Done`".
- Rejected: making the age rule the sole arbiter.
  It converts two ordinary best-effort cleanup failures, and every `KeepPane` run, into a hard resume refusal.

### one-live-match-or-none

- Decision: `Attach` returns an error, not a pick, when the scan finds more than one live run matching the spec's output files.
- Rationale: two live agents on one output-file set is the duplicate-agent state this whole decision exists to prevent, already realised.
  Choosing one silently would hide it and leave the other writing the same files.
- Rejected: preferring the newest by `CreatedAt`.
  It makes the broken state survivable and therefore permanent.

### interactive-keeps-todays-permission-coupling

- Decision: no change to the `Interactive` → `--dangerously-skip-permissions` relationship.
  An interactive `Discussion-Write` run launches without the flag, so the operator approves tool calls in the pane.
- Rationale: it is an orthogonal knob, and the operator who asked for an interview is present at the pane by definition.
  Claude Code's per-pattern "allow always" makes the cost front-loaded rather than continuous.
- Rejected: decoupling permission mode from `Interactive` so an interactive run can still skip permissions.
  A defensible future change, but it widens this task into the `Shuttle Provider-Seam Invariant`'s territory and changes a documented, deliberate contract for a convenience.
  If the approval traffic proves intolerable in practice it is a small, independent follow-up.

### bounce-still-re-interviews-from-scratch

- Decision: out of scope, recorded rather than fixed.
  A `Discussion-Validate` bounce re-enters `Discussion-Write`, finds no live run, archives the artifacts, and spawns a fresh agent — which in interactive mode means the operator is interviewed again from the top, with no knowledge of the validator's complaint.
- Rationale: the underlying cause is that `loomshed.discussionValidate.Call` discards its `findings` slice and nothing carries prior artifacts into a re-entered `Discussion-Write` prompt.
  Fixing that means changing what the bounce edge conveys, which is a producer-contract change of its own size and touches the same shared rows the sibling roadmap item (`Discussion-Burler`'s `fix-scope: source`) is already scheduled to edit.
- Rejected: folding it in.
  Two independent contract changes to the same rows in one task, when the interactive-mode change is already the harder of the two.

## Technical context

**The literal being replaced.**
`internal/loomcli/wiring.go`, in the `c.env = shedrecipe.Env{...}` block, roughly line 139:

```go
DiscussionSpec: func() (shuttleengine.Spec, error) {
    return loomengine.DiscussionSpec(location, websterGeom.StencilsDir, loomCfg, registry, seedSlug(location.WorktreeName), true)
},
```

The comment two lines above it names the "autonomous-only Shared Decision" and must be rewritten, not merely edited around.
`loomCfg` is already in scope at that point, so the change is `!loomCfg.DiscussionInteractive` in place of `true`.
Note the sibling `PlanSpec` closure's comment explicitly contrasts itself against this one ("Unlike `DiscussionSpec` beside it, `PlanSpec` takes no autonomous argument") — it stays accurate and needs no change.

**The prompt side is already complete.**
`internal/loomengine/prompt.go`'s `modeRules(autonomous bool)` returns two finished blocks.
The interactive one already tells the agent to ask as plain numbered-list text in the pane, to batch at most five questions with the recommendation as option 1, and never to call `AskUserQuestion`.
`contracts/stencils/loom/loom-template-discussion.md` carries `{{.mode_rules}}` at line 70 and a standing `## Never use AskUserQuestion` section at line 129 that applies "in either mode".
`internal/loomengine/prompt_test.go` already pins that the two renderings differ, that the autonomous one says "best-judgment", that the interactive one says "operator", and that neither mentions a `--auto` flag.
No stencil edit is expected;
if one turns out to be needed, the `Stencil Ownership Invariant` and the `Producer Pointer-Rule Invariant` both bind.

**Config.**
`internal/loomengine/config.go` holds `Config` and `LoadConfig`;
`internal/loomengine/configtemplate.go` embeds `template.yaml`, which currently has six keys.
`configengine.Load` is strict about unknown and missing keys, so the new key must be added to `template.yaml` **and** the struct together.
`loomengine` is on the strict side of the `Config Strictness Invariant`'s pinned sets and stays there.

**Migration obligation for existing worktrees — not optional, and not automatic.**
Adding a key to a strict-side template breaks every worktree whose `loom.yaml` was written before it.
`configengine.Load` hard-errors with `config file <path>: missing keys: discussion_interactive; run "lyx config reconcile"` (`internal/configengine/config.go:113`), and nothing reconciles outside that explicit verb — so the very next `lyx loom run` in any already-initialized worktree fails until the operator reconciles.
**The remedy is `lyx config reconcile --apply`, not the bare verb the error text names.**
`lyx config reconcile` is a **dry run**: it reports added and removed keys and writes nothing, and `--apply` is what writes the reconciled files (`internal/configcli/configcli.go:338-343`, and the flag at `:343`).
An operator who follows the error message literally therefore reconciles nothing and fails the next `lyx loom run` identically.
The change note / commit message must carry the `--apply` form, and it applies to every in-flight worktree, not only new ones.
Whether the error text's own wording should gain `--apply` is a separate, repo-wide question this task does not settle.
`config_test.go` is the existing home for key-coverage assertions.

**The wait loop.**
`internal/shuttleengine/wait.go`.
`Run.Wait` is a `for tick := 1; ; tick++` loop doing three things per tick: `pollEventsTick`, a periodic `checkLivenessTick`, and a deadline comparison, then `run.clock.Sleep(interval)`.
`pollEventsTick` returns `("", "", nil)` when there is nothing new;
`Wait` returns via `run.finalize(outcome, message)` on any non-empty outcome.
The `AwaitOperator` change is confined to the branch that consumes `pollEventsTick`'s result: an `OutcomeAsking` under `AwaitOperator` is logged and dropped instead of finalized.
`OutcomeDone` must still finalize — including under `AwaitOperator` — and the existing comment noting that a genuine success verdict survives cancellation applies unchanged.
The `clock` interface at the top of the file is the injection point the existing tests use to replay whole poll sequences instantly.

**The run-dir layer.**
`internal/shuttleengine/rundir.go` already has everything `Attach` needs: `runDirRoot(cfg, anchorPath)`, `loadRunState(runDir)`, the `RunState` struct (which already persists `StrandGUID`, `SessionID`, `Interactive`, `OutputFiles`, `PromptPath`, `SettingsPath`, `EventsPath`, `CreatedAt` — its doc comment already says it carries what "a resumed or re-attached session" needs), and `findRunByStrand`'s precedent for a scan that skips unreadable `run.json` files rather than aborting.
`Attach`'s scan is the same shape as `findRunByStrand`'s with a different predicate.

**Reed liveness.**
`Wait`'s `checkLivenessTick` is the existing probe and encodes a three-way answer that must not be flattened: a strand reed does not track (`errStrandNotTracked`) and a strand reed tracks with no pane bound (`errStrandPaneBindingCleared`) are *mechanism failures*, not "dead", precisely because the agent may still be working.
For `Attach`'s purposes, only "tracked **and** pane live" counts as attachable.
The disposition of the other answers is decided in `mechanism-failures-do-not-attach-and-do-not-blindly-respawn` above, not left to the plan: error while the run dir is younger than `2 * StartupTimeoutS`, `found == false` once it is older, and error unconditionally when reed's state file is absent or unreadable.
Note also that `checkLivenessTick`'s not-tracked and not-live branches each check `allOutputFilesExist` *before* reporting anything, so an agent that finished and then vanished still classifies `OutcomeDone`;
that behaviour is inherited by an attached run unchanged and must not be duplicated in `Attach` itself.

**The orphan sweep interacts with this.**
`Runner.Start` calls `sweepOrphansOpportunistic`, which deletes run dirs whose `StrandGUID` is absent from reed's strand table, guarded by `minAge = 2 * StartupTimeoutS`.
The probe must run **before** anything that could sweep the dir it is looking for.
In the attach path nothing sweeps (no `Start`);
in the respawn path the sweep is exactly right, because the probe already established there is nothing live.

**The producer.**
`internal/shedadapters/singlellm.go` holds both the `Shuttle` seam (`Run(Spec) (Result, error)`, with `var _ Shuttle = (*shuttleengine.Runner)(nil)` beneath it) and `SingleLLMProducer.Call`.
`internal/shedadapters/archive.go` holds `archiveStaleOutputs`.
`singlellm_test.go` has the existing fake `Shuttle`;
widening the seam means widening that fake and any other implementor.

**The recipe graph.**
`contracts/recipes/loom-recipe.yaml`: `Loom-Preflight --on_done--> Discussion-Write --on_done--> Discussion-Validate`, and `Discussion-Validate --on_stuck--> Discussion-Write`.
`Discussion-Write` has **no** `on_stuck`, so a `Stuck` from it escalates the run to blocked rather than bouncing.
The bounce budget in the doc's trap is `Discussion-Validate`'s, inherited from `shedengine`'s two-level default (`ProducerDef.MaxBounces` → `Shed.MaxBounces` → `defaultMaxBounces`) since neither row sets `max_bounces`.
The file's header warns that the seventeen row names are durable on-disk identities pinned against `internal/loomshed`'s `Name*` constants;
no rename is in scope.

**The Shed resume contract.**
`internal/shedengine/run.go` re-`Call`s `st.CurrentProducer` on entry, under a whole-run lock (`LoomRunLock`), distinct from the per-persist status lock.
History is persisted only after a `Call` returns, which is why a crash mid-`Call` leaves state indistinguishable from a just-routed bounce.
`internal/shedengine` may import only stdlib, `internal/state`, and `internal/lock` (`Shed Producer-Seam Invariant`) — no change to that package is anticipated or permitted here.

**Detached-driver topology, for anyone reasoning about the operator's terminal.**
`lyx loom run` seeds and commits status, ensures the reed session and its status strand, spawns `lyx loom drive` **detached** with stdout/stderr to `LoomDriverLog`, waits for the handshake that the child took the run lock, then `exec`s a tmux attach.
So the operator's terminal is attached to the tmux session, and an interactive agent's pane is a strand inside it.
That is what makes interactive mode viable at all, and it is also why the mode selector cannot be a plain flag on `run`.

**Paths and files most likely to change:**

- `internal/loomengine/template.yaml`, `config.go`, `config_test.go`
- `internal/loomcli/wiring.go`, `wiring_test.go` (`wiring_test.go:371` currently asserts `spec.Interactive == false` with the comment "autonomous-only")
- `internal/shuttleengine/spec.go` (the new field + its doc comment), `wait.go`, `rundir.go`, a new attach source file, `doc.go`
- `internal/shedadapters/singlellm.go`, `singlellm_test.go`, `doc.go`
- `manifest/designs/loom.md`, `manifest/roadmap.md`

## Constraints

From `CONSTRAINTS.md`, the ones this task actually touches:

- **Shuttle Provider-Seam Invariant.**
  `internal/shuttleengine` stays provider-invariant and never imports `internal/shuttleengine/claudeengine`.
  `AwaitOperator` and `Attach` are both provider-invariant by construction — neither may grow a Claude specific.
  Enforced for the import half by `internal/shuttleengine/seam_enforcement_test.go`.
- **Shed Producer-Seam Invariant.**
  `internal/shedengine` production code imports only stdlib, `internal/state`, `internal/lock`.
  Nothing in this task should need to touch that package;
  if the plan finds it does, that is a signal the design drifted.
  Enforced by `internal/shedengine/seam_enforcement_test.go`.
- **Shed Recipe Registry Invariant.**
  `internal/shedrecipe` takes every absolute path from its caller and has no direct production import of `internal/lyxcwd`.
  Adding a config value to `Env` or to a registry entry must respect that.
  `internal/shedrecipe/registry_test.go` pins the registry's exact fourteen names;
  no new entry is in scope.
- **Cwd Resolution Invariant.**
  `internal/lyxcwd` alone owns cwd resolution;
  `loomengine` owns the `_lyx/discussion/` and `_lyx/loom/` path accessors and no other package may construct those paths.
  `Attach` resolves its scan root through the existing `runDirRoot(cfg, anchorPath)` and must not build a `.lyx` path of its own.
- **Lyxdirs Single-Declarer Invariant.**
  No new production code may name the `_lyx` or `.lyx` literals;
  `lyxdirs.LyxDirName` / `lyxdirs.DotLyxDirName` and the existing accessors are the only spellings.
- **Told-Geometry Invariant.**
  `Runner` is told `anchorPath` and `worktreeRoot` and derives neither;
  `Attach` inherits that and must take its root from the `Runner` it hangs off, never re-derive it.
- **Durable-vs-Ephemeral State Invariant.**
  Shuttle run dirs live under `.lyx/shuttle` and are ephemeral and never tracked.
  A run dir surviving a crash is local, machine-scoped evidence — the design must not assume it survives a `git clean -xdf` or a move to another machine, and the respawn fallback is what covers those cases.
- **Config Strictness Invariant.**
  `loomengine` is on the strict side.
  The new `discussion_interactive` key must land in `template.yaml` and `Config` together, or `configengine.Load` fails on unknown/missing keys.
- **CLI / Cobra Invariant.**
  No new CLI verb or flag is in scope.
  If the plan adds one anyway, `Short` on every command and the help-tree tests bind.
- **Test Tier Purity Invariant** and **Live-Substrate Spawn Observability.**
  `Attach` and the `AwaitOperator` wait change must be testable without a real tmux or a real `claude`;
  `wait.go`'s injected `clock` and the existing `fakes_test.go` seams are the intended route.
- **Documentation Lifecycle.**
  Docs land in the same commit — see the Scope list.

## Testing

**`internal/loomengine` — config.**
Table test that `LoadConfig` parses `discussion_interactive` in both spellings and defaults to `false` when the template's own value is taken.
Assert the template and the struct agree, in the shape `config_test.go` already uses for the six existing keys.
Missing-key and unknown-key behaviour is `configengine`'s, already covered;
do not re-test it here.

**`internal/loomengine` — spec.**
Extend `discussion_test.go`: `DiscussionSpec(..., autonomous=false)` must produce `Interactive == true` **and** `AwaitOperator == true`;
`autonomous=true` must produce both `false`.
The existing `prompt_test.go` mode-rules coverage needs no change.

**`internal/loomcli` — wiring.**
`wiring_test.go:371` currently asserts `spec.Interactive == false` with an "autonomous-only" comment.
Replace it with a two-case test driving `wire()` with `DiscussionInteractive` true and false and asserting the resulting `DiscussionSpec()` closure's `Interactive`/`AwaitOperator`.
`wiring_test.go:501`'s `PlanSpec` assertion ("autonomous by design") must stay green untouched — it is the regression guard that this change did not leak into the plan producer.

**`internal/shuttleengine` — `Wait` under `AwaitOperator`. TDD candidate.**
This is the defect-A test and should be written first, red, against the existing `clock` seam and event fixtures in `wait_test.go`:

- A run with `AwaitOperator: false` and one turn-end event with no output files present returns `OutcomeAsking` (pins today's behaviour).
- The same run with `AwaitOperator: true` does **not** return on that event;
  it keeps polling, and returns `OutcomeDone` once the output files appear on a later tick.
- Several asks in a row followed by a done, so a multi-batch interview is covered, not just one ask.
- `AwaitOperator: true` still returns `OutcomeTimeout` once the deadline passes with the files absent.
- `AwaitOperator: true` still returns `OutcomeDied` for a tracked strand with a dead pane, and still surfaces `errStrandNotTracked` / `errStrandPaneBindingCleared` as mechanism failures.

**`internal/shuttleengine` — `Attach`. TDD candidate.**
Over a temp run-dir root with hand-written `run.json` files and a fake reed:

- No run dirs at all, and a root that does not exist: `found == false`, nil error.
- A run dir whose `OutputFiles` do not match the spec's: not selected.
- A matching run dir whose strand is absent from reed's table, dir **younger** than `2 * StartupTimeoutS`: error, not a respawn.
- The same, dir **older** than `2 * StartupTimeoutS`: `found == false`, nil error, so the caller archives and respawns.
- Both of the above again for a strand that is tracked but holds no pane id.
- reed's state file absent, and separately unreadable: error in both cases, never `found == false`, at any dir age.
  The absent case is only reachable because `Attach` reads `reedengine.LoadState` directly;
  a test written against a fake `ReedOps.Status()` cannot express it, since an absent file surfaces there as a successful empty table.
  Add a companion assertion that `Attach` does **not** consult `Status()` for this question.
- A matched record in a non-attachable branch whose output files **all exist**: `found == false` at any dir age, including a dir younger than `2 * StartupTimeoutS` (the fast-bounce-after-failed-cleanup case) and a `KeepPane` leftover.
- The same output files present but the strand **tracked and live**: attached, not treated as leftover, and the first tick classifies `OutcomeDone` — with the output files still on disk, unarchived.
- `Display.Anchor` defaulting: a spec with an empty `Anchor` attaches to a binding-cleared strand and surfaces `errStrandPaneBindingCleared`, rather than taking the hidden-strand carve-out and classifying `OutcomeDied`.
- `KeepPane` on an attached run suppresses the `Done` cleanup, and its absence performs it.
- A matching run dir whose strand is tracked with a live pane: `found == true`, and `Wait` runs against the persisted `EventsPath`.
- Two matching live run dirs: error, not a pick.
- An unreadable or truncated `run.json` mid-scan does not abort the scan.
- The attached run's deadline is `now + spec.Timeout`, proven by attaching to a record whose `CreatedAt` is already older than `spec.Timeout` and asserting it does not immediately time out.
- A spec with a **zero** `Timeout` attaches with the config default applied, and does **not** classify `OutcomeTimeout` on its first tick.
- Output-file matching is on the **resolved absolute** set, so a spec with relative entries matches a `run.json` written with absolute ones, and a spec naming the same files in a **different order** still matches.
- `offset` starts at 0: a pre-existing `events.jsonl` whose last event is a completion (output files present) classifies `OutcomeDone` on the first tick after attach — the missed-terminal-Stop case.
- The same backlog with output files absent classifies `OutcomeAsking`: terminal without `AwaitOperator`, dropped and polling continues with it.
- `started` is seeded true: attaching to a strand whose `CapturePane` returns a mid-turn capture (no `❯`, no `shortcuts`) must **not** classify `OutcomeDied` after `startup_timeout_s`, and must **not** play the trust-dismiss sequence when the capture happens to contain a `trustDialogNeedles` phrase.
  Both are direct regression guards, since both are what an unseeded attach does today.
- An attached run whose strand later goes not-live with output files present still classifies `OutcomeDone`, confirming the inherited `checkLivenessTick` branches are reached.

**`internal/shedadapters` — `SingleLLMProducer` ordering. TDD candidate.**
With a fake `Shuttle` recording call order and a temp dir holding pre-existing output files:

- Probe returns `found == false` → the files are archived to timestamped siblings and `Run` is called (today's path, unchanged).
- Probe returns `found == true` → `Run` is **not** called and the files are **not** archived (a temp file's inode/mtime or a plain existence check at the original path proves it).
- An attached `OutcomeDone` maps to `shedengine.Done` with the first output file as the pointer;
  an attached `OutcomeDied`/`OutcomeTimeout` maps to a returned error;
  an attached `OutcomeAsking` maps to `shedengine.Stuck` — i.e. the outcome switch is shared, not duplicated.
- A probe error propagates as an error and neither archives nor spawns.
- Context cancellation is still honoured at entry and around the probe.

**`internal/loomrecipe` — the bounce path. The regression test that closes the doc's trap.**
`resume_test.go` is the existing home.
Drive `Discussion-Validate` to `Stuck` with both discussion artifacts present on disk and no live run in the run-dir root, and assert that `Discussion-Write` **respawns** — that it does not report `Done` off bare file existence, and that the run does not ping-pong until `Discussion-Validate`'s bounce budget is exhausted.
Then the complement: the same artifacts present **with** a live matching run, and assert it attaches instead.
Those two cases together are the whole point of the task and should be named so a future reader sees that.

**Not to be tested here:** real tmux, real `claude`, real permission dialogs, and the interactive prompt's actual conversational quality.
The sandbox suite and a manual `lyx loom run` with `discussion_interactive: true` are how the end-to-end behaviour gets confirmed;
the unit tiers above prove the seams.

## Q&A log

- **Q:** Where does the interactive/autonomous mode selector live? **A:** [auto-pick] A `loom.yaml` key `discussion_interactive` (default `false`) threaded through `loomengine.Config` into `wire()`. **Why:** `lyx loom run` spawns a *detached* `lyx loom drive` child, so a flag on `run` cannot reach the process that builds the wiring without new persisted state; `loom.yaml` is already loaded by both paths and is already per-task, and `Config` already carries the discussion role's other knobs.
- **Q:** How does an interactive run stop dying at its first question? **A:** [auto-pick] A new explicit `shuttleengine.Spec` field, `AwaitOperator bool`, honoured by `Run.Wait`. **Why:** `Spec.Interactive` already means the opposite thing to the settings layer — `claudeengine/settings.go` installs an `AskUserQuestion` recording hook in interactive runs *specifically* so an ask classifies as a real-time asking signal — so overloading it would destroy that feature and silently change `lyx shuttle run --interactive`.
- **Q:** What evidence distinguishes an interrupted interview from a `Discussion-Validate` bounce? **A:** [auto-pick] Live-agent evidence only — a surviving `run.json` matching the spec's output files whose `StrandGUID` reed still tracks with a live pane. Never file existence. **Why:** it is `loom.md`'s own crash-recovery ladder step 2, and it is the only signal that differs; the Shed status file is byte-identical in both states, which is exactly why the doc's file-existence shortcut ping-pongs.
- **Q:** Which package owns the re-attach? **A:** [auto-pick] `internal/shuttleengine`, as `Runner.Attach(Spec) (Result, bool, error)`, with the `shedadapters.Shuttle` seam widened by that one method. **Why:** the run-dir layout, the `run.json` schema, reed's three-way liveness answer, and the `Run`/`Wait` machinery are all private to `shuttleengine`; reimplementing them above the seam would duplicate its most carefully-reasoned code.
- **Q:** Does the attach probe apply to every `SingleLLMProducer` row or only interactive ones? **A:** [auto-pick] Unconditional, every row. **Why:** respawning over a live agent is the duplicate-agent hazard `shuttleengine`'s own sentinels and `loom.md`'s ladder both forbid, and it is live in autonomous today; gating on interactive would knowingly ship the bug for `Plan-Write`. Blast radius is recorded in Scope and in the decision.
- **Q:** Probe before or after `archiveStaleOutputs`? **A:** [auto-pick] Before. **Why:** archiving renames the very files a live agent is about to write, and `Wait` polls for bare existence at the spec's paths — archiving first would make an attached run unable to ever classify `Done`.
- **Q:** What deadline does an attached run get? **A:** [auto-pick] A fresh `now + spec.Timeout` at attach time, ignoring `RunState.CreatedAt`. **Why:** `finalize` cleans up only on `Done`, so a timed-out run's strand and dir survive; inheriting `CreatedAt + Timeout` would re-attach it and re-time-it-out on every resume forever, with no operator escape.
- **Q:** What if two live runs match the same spec? **A:** [auto-pick] Error, not a pick. **Why:** that state *is* the duplicate-agent failure already realised; choosing one silently hides it and leaves the other writing the same files.
- **Q:** Should interactive runs keep dropping `--dangerously-skip-permissions`? **A:** [auto-pick] Yes, unchanged. **Why:** orthogonal knob, deliberate documented contract, and the operator is at the pane by definition; "allow always" front-loads the cost. Recorded as a small independent follow-up if the approval traffic proves intolerable.
- **Q:** Does a `Discussion-Validate` bounce get to tell the re-spawned interview what was wrong? **A:** [auto-pick] No — out of scope, recorded as a known wart. **Why:** the cause is `discussionValidate.Call` discarding its findings plus nothing carrying prior artifacts into a re-entered prompt; that is a producer-contract change of its own size, on rows the sibling roadmap item is already scheduled to edit.
- **Q:** Do the review segment or `Plan-Write` gain an interactive mode? **A:** [auto-pick] No. **Why:** `PlanSpec` hardcodes `Interactive: false` by design and `wiring_test.go:501` guards it; the `Bouncer`/`Burler` rows are an autonomous review loop, not an interview.
- **Q:** Is the mode allowed to change between a crash and a resume? **A:** [auto-pick] Yes, and nothing compares them. **Why:** the resume decision is made on live-agent evidence, never on mode; mode only selects the prompt block and `AwaitOperator` for the *next* spawn, so a flip does exactly what an operator flipping it would want.
- **Q:** What is the default? **A:** [auto-pick] `false` — autonomous, today's behaviour. **Why:** interactive requires a human at the pane; a task spawned unattended must not silently wait for one.
- **Q:** Does ladder step 1 ("is there a complete output file? → Done") get implemented too? **A:** [auto-pick] No, deliberately not. **Why:** it is the exact shortcut the doc's trap warns about, and with steps 2 and 3 in place it buys nothing — a run whose output files are complete already reached `Done` and cleaned itself up.
