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
- Docs in the same commit: `manifest/designs/loom.md`'s crash-recovery section (the open-trap paragraph is replaced by the resolution), `internal/shuttleengine/doc.go`, `internal/shedadapters/doc.go`, and the roadmap item's move off Planned.

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
  `internal/shuttleengine/claudeengine/settings.go` already gives `Interactive` a directly contradictory meaning: in interactive runs it installs a `PreToolUse(AskUserQuestion)` hook that appends to `events.jsonl` specifically so "the run loop can classify it as a real-time asking signal instead of waiting for the timeout" (its own package comment).
  Making `Interactive` suppress that classification would destroy the one feature that hook exists to deliver, and would silently change `lyx shuttle run --interactive` from a fast-returning probe into a blocking wait.
  Two orthogonal facts — "an operator is present" and "wait for the operator rather than reporting back" — need two fields.
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
  After a `Discussion-Validate` bounce the previous `Discussion-Write` run reached `OutcomeDone`, so `Run.finalize` removed its strand and deleted its run dir;
  there is nothing live to find, and the row respawns.
  After a crash mid-interview no cleanup ran, so the run dir and its `run.json` survive and — because the tmux server is detached and outlives the driver process — the pane usually does too.
  Ladder step 1 ("is there a complete output file?") is deliberately **not** implemented here: it is the exact shortcut the doc's trap warns about, and with steps 2 and 3 in place it buys nothing, since a run whose output files are complete has already reached `Done` and cleaned itself up.
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

### attach-is-unconditional-not-interactive-only

- Decision: `SingleLLMProducer.Call` probes `Attach` on **every** call, regardless of `spec.Interactive` or `spec.AwaitOperator`.
- Rationale: respawning over a still-live agent is a correctness bug in autonomous mode too, not merely a UX annoyance in interactive mode.
  It puts two agents on one worktree writing the same output files, which is the exact duplicate-agent hazard `internal/shuttleengine`'s own `errStrandNotTracked` and `sweepOrphansOpportunistic` comments spend paragraphs avoiding one layer down, and which `loom.md`'s ladder step 2 forbids in as many words.
  Gating the fix on interactive would leave the documented discipline unimplemented for the majority of rows.
- Rejected: gating on `spec.Interactive`.
  Smaller blast radius, but it knowingly ships the duplicate-agent path for `Plan-Write` and every generic `SingleLLM` row.
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
For `Attach`'s purposes, only "tracked **and** pane live" counts as attachable;
both error states mean "do not attach", and — importantly — they must also mean "do not respawn blindly", since respawning is what the two sentinels exist to prevent.
The plan should decide whether `Attach` surfaces those as an error (safest, and consistent with the sentinels' own rationale) or as `found == false`.
Prefer the error.

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
- A matching run dir whose strand is absent from reed's table: not attached, surfaced as the mechanism failure rather than as a silent respawn.
- A matching run dir whose strand is tracked but holds no pane id: same.
- A matching run dir whose strand is tracked with a live pane: `found == true`, and `Wait` runs against the persisted `EventsPath`.
- Two matching live run dirs: error, not a pick.
- An unreadable or truncated `run.json` mid-scan does not abort the scan.
- The attached run's deadline is `now + spec.Timeout`, proven by attaching to a record whose `CreatedAt` is already older than `spec.Timeout` and asserting it does not immediately time out.
- Output-file matching is on the **resolved absolute** set, so a spec with relative entries matches a `run.json` written with absolute ones.

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
