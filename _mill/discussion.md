# Discussion: Producer gates: mechanical gates before session release

```yaml
task: 'Producer gates: mechanical gates before session release'
slug: producer-gates
status: discussing
parent: main
```

## Problem

A `ShedProducer` that runs an LLM hands its artifact off the moment the agent believes it is done.
Whether the artifact is *valid* — the discussion pair carries its seven required headings, the plan parses and passes `planglyph`'s checks — is answered only afterwards, by a separate recipe row (`Discussion-Validate`, `Plan-Validate`, `Plan-Revalidate`) whose only recovery is `Stuck` back to the writer.
That bounce respawns a cold agent that rebuilds the whole context from nothing, to fix what is usually a missing heading.

The recovery is also masking: it is a heavy, expensive path that makes a bug in the check look like an ordinary bounce, and it leaves a genuine hole — nothing at all re-checks what a `Burler` fix round rewrote until the *next* standalone row (which, for the discussion segment, does not exist at all: `Discussion-Burler`'s overlay round can hand back an artifact it just made invalid, and `Discussion-Bouncer` then approves it).

**Why now:** `manifest/designs/producer-gates.md` settles the direction and `manifest/roadmap.md` lists it as the sole Planned item, to land before the batten end-to-end crucible campaign (wiki: `crucible-batten-end-to-end`).
The design doc is explicit that it is "not a row-level spec — validate details against the code before implementing from it", which this discussion does.

## Scope

**In:**

- A gate contract (`Gate`, `GateSpec`, `GateResult`, `GateOutcome`) declared in `internal/shuttleengine`, and the attempt loop that drives it.
- `internal/shuttleengine`: `Wait` consults an optional per-run gate on its Done path *before* `finalize`; on failure it `Send`s a one-line re-prompt naming a findings file and keeps polling for the next turn boundary; bounded by an attempt budget.
- Gated entry points on `Runner` reaching both the spawn path and the attach/resume path, so a resumed run is gated exactly as a fresh one is.
- `internal/burlerengine`: `RunOpts` gains one `Gate GateSpec` field and `Result` gains a `Gate *shuttleengine.GateOutcome` passthrough; its `Shuttle` seam gains the gated method, so a fix round's own output is gated before the round reports back to its `Bouncer`.
- `internal/shedadapters`: `Shuttle` gains `RunGated`/`AttachGated` alongside `Run`/`Attach`; `SingleLLMProducer` and `BurlerProducer` (both its spawn path and its `probeLiveRound` resume path) call the gated forms and map a failed gate onto `shedengine.Stuck`.
- Four gate sites, two validators: `Discussion-Write` and `Discussion-Burler` share `discussionparser.Validate`; `Plan-Write` and `Plan-Burler` share `planglyph.ValidateFormat`.
- Removal of the `Discussion-Validate`, `Plan-Validate` and `Plan-Revalidate` rows from `contracts/recipes/loom-recipe.yaml`, their `loomshed` `Name*` constants, their `interruptPolicy` entries, their `shedrecipe` registry entries, and the `loomshed` producers `NewDiscussionValidate`/`NewPlanValidate` — the recipe goes from seventeen rows to fourteen.
- A `gate_attempts:` row-config key, read by the `shedrecipe` entry constructors that build the four gated rows and packed with the closure into the `GateSpec` they hand downstream.
- `CONSTRAINTS.md`: the `Gate Self-Check Parity Invariant` rewritten around the new pairing.
- Docs in the same commit (see **Docs**).

**Out:**

- `discussionparser`, `planparser` and `planglyph` themselves — the gate *is* their existing functions, called from a new site. No check is added, removed, or changed.
- The `lyx loom validate-discussion` / `validate-plan` CLI self-check verbs. They stay, including `--require-approved`; only the invariant that binds them to the recipe is rewritten.
- The compound quiescence probe the design doc sketches (turn-idle ∧ no pane child processes ∧ empty hook-maintained pending-work ledger). See the **Per-attempt done-signal** decision: it is deliberately not built in this task, and nothing is exported from `reedengine` for it.
- Any new hook event in `internal/shuttleengine/claudeengine/settings.go`. The Stop hook already in place is the whole mechanism.
- Gating `Webster-Burler`. There is no mechanical validator over a committed diff, so the third segment gets no gate and its `RunOpts.Gate` stays the zero `GateSpec`.
- The `Bouncer` rows' behaviour and wiring, the `approve_seam`, the `commit_seam`, and every bounce budget — untouched. `Bouncer` gets no gate and its `Shuttle` call sites do not change; the only thing this task asks of it is that it continues to compile against a seam that has gained two methods.
- Any resume migration for in-flight runs parked on a removed row (see **Row removal and resume**).
- Adding a fifth `shedengine.Outcome` value, or any change to the Completion Signal Invariant's negative-answer set.

## Decisions

### The gate loop lives inside `shuttleengine.Wait`, not above it

- **Decision:** the attempt loop runs inside `internal/shuttleengine`, on `Wait`'s Done path, before `finalize` is reached.
  `*Run` gains one unexported `GateSpec` field; `Runner` gains gated entry points (`RunGated`, and the attach-side equivalent) that thread it in.
  Plain `Runner.Run`/`Runner.Attach` keep today's behaviour by supplying the zero `GateSpec`, so every ungated row is bit-for-bit unchanged.
- **Rationale:** `finalize` (`internal/shuttleengine/wait.go:554`) removes the reed strand and deletes the run directory whenever the outcome is `OutcomeDone` and `spec.KeepPane` is false.
  By the time `Runner.Run` returns Done, the session the gate is supposed to re-prompt is already gone.
  `Wait`'s Done path is the only point in the system that holds both a classified Done and a live session, which is exactly the fixed point in the producer's own control flow the design doc's step 3 describes.
  It also covers the attach path for free: `Attach` → `reconstructAndWait` → `Wait` is the same funnel, so a resumed run is gated without a second implementation.
- **Rejected:** a new `internal/shedgate` package driving `Start`/`Wait`/`Send` with `KeepPane: true` and doing its own teardown — the attach path returns a `Result`, not a `*Run` handle, so a resumed run could not be gated at all without widening `Attach` anyway, and the caller would inherit `finalize`'s cleanup obligations (strand removal, run-dir deletion, the ForkAudit branch) in two places.
  Also rejected: duplicating the loop inline in `shedadapters` and `burlerengine` — same loop, two copies, and both still blocked by the `finalize` problem.

### Contract shape: declared in `shuttleengine`, closure takes no argument

- **Decision:**

  ```go
  // GateResult is one gate verdict over the artifact.
  type GateResult struct {
      Passed   bool
      Findings string // detailed WHAT-is-wrong; empty when Passed
  }

  // Gate is the mechanical validator a gated run holds its handoff on.
  type Gate func() (GateResult, error)

  // GateSpec is a gate and its attempt budget, carried as one value.
  // The zero value (nil Gate) means "ungated", which is what every row but the four gated ones supplies.
  type GateSpec struct {
      Gate     Gate
      Attempts int // re-prompt budget; 0 means the package default
  }

  // GateOutcome is what a gated run reports back about its gate.
  type GateOutcome struct {
      Passed       bool
      Attempts     int    // re-prompt attempts spent; 0 means the gate passed first try, or no session was live to re-prompt
      FindingsPath string // informational only -- see below; empty when Passed
  }
  ```

  All four live in `internal/shuttleengine` beside `Spec`.

  `GateOutcome.FindingsPath` is **diagnostic text, never a path to dereference after `Wait` returns**.
  The findings file lives in the run directory, and `finalize` deletes that directory on the Done cleanup every exhausted gate takes, so by the time a producer reads the field the file is already gone.
  It is carried so the producer's `Stuck` log line can name where the findings were written while the run was live; the durable record of *what* they said is the gate closure's own `logger.Warn`, which carries the formatted findings text.
  A producer that opens it is a defect, and the field's doc comment must say so.
- **Rationale:** `shuttleengine` is the only consumer of the seam, so declaring the types there adds no package and no entry to any import allowlist or seam-enforcement scan.
  The closure takes no argument because the two validators take materially different path shapes — `discussionparser.Validate(decisionRecordPath, supportLogPath)` takes two file paths, `planglyph.ValidateFormat(plan, worktreeRoot)` needs a parsed plan plus the worktree root, and `planparser.PlanDir` needs the anchor path — so no single `artifactDir string` parameter fits either, and inventing one would make `shuttleengine` know artifact geometry it has no business knowing.
  Every path in this repo is told at construction; a closure that captures its own told paths is the established shape (`loomshed.NewDiscussionValidate`, `NewPlanValidate` already take exactly these paths).
- **Rejected:** the design doc's literal `Gate func(artifactDir string) (GateResult, error)`, for the reason above.
  Also rejected: a new leaf package `internal/shedgate` for the three types — one consumer does not justify a package, and it would need an entry in the Told-Geometry Invariant's bound list.

### `error` is never "not passed"

- **Decision:** a non-nil `error` from the gate closure fails the whole run as an ordinary producer error (a returned `error` from `Wait`, which `shedadapters`/`burlerengine` map to a returned error, which `shedengine` persists as `failed`).
  It never burns an attempt and never reaches the LLM.
  Only `GateResult{Passed: false}` produces findings, a re-prompt, and an attempt charge.
- **Rationale:** verbatim from the design doc, and it matches what `loomshed`'s existing producers already do: `planValidate.Call` maps *every* `planglyph` error to a returned error, explicitly because "a gate that could not read the code has not found a plan defect to bounce", and `discussionValidate.Call` does the same for a non-not-exist read failure.
  Mixing the two spends the budget on infrastructure faults and makes the exhaustion message lie about the cause.
- **Rejected:** treating an error as a failed attempt with the error text as findings — the LLM cannot fix a missing quarry binary, and three absurd re-prompts would hide the real fault.

### Exhaustion is reported in `Result`, not as a new `Outcome`

- **Decision:** `shuttleengine.Result` gains `Gate *GateOutcome` (nil means "this run had no gate").
  `Outcome` stays closed at `OutcomeDone`/`OutcomeAsking`/`OutcomeDied`/`OutcomeTimeout`.
  A gate that exhausted its budget returns `Outcome: OutcomeDone, Gate: &GateOutcome{Passed: false, ...}`.
  The producers map `OutcomeDone && result.Gate != nil && !result.Gate.Passed` onto `shedengine.Stuck`, exactly as `discussionValidate`/`planValidate` map a findings slice onto `Stuck` today.
- **Rationale:** the Completion Signal Invariant governs every code path in `shuttleengine` that finalizes a *negative* answer to "did this run finish", and requires each to consult `allOutputFilesExist` first.
  A new `OutcomeGateFailed` would be a fifth outcome that is negative-shaped but whose whole premise is that the output files DO exist and are invalid — a carve-out in the middle of an invariant that has been audited across five crucible rounds and is pinned by two AST scans in `completionsignal_enforcement_test.go`.
  Reporting it as a field on `Result` leaves that invariant's value set and its enforcement scans untouched: the gate runs strictly *after* a positive Done, never as part of classifying one.
- **Rejected:** `OutcomeGateFailed`, for the reason above.
  Also rejected: returning an `error` on exhaustion — that persists `failed` and aborts the run, where the design doc calls for "ordinary `Stuck`", which persists `blocked` and bounces.

### Findings always ride a file, never the re-prompt text

- **Decision:** on a failed attempt the gate loop writes `GateResult.Findings` to a per-run findings file under the ephemeral `.lyx` tree (inside the run directory, alongside `prompt.md`/`settings.json`/`events.jsonl`), and `Send`s a single line naming that absolute path.
  Each attempt overwrites the same file, so the agent always reads the current complaint.
- **Rationale:** `validateSendText` (`internal/shuttleengine/run.go:437`) rejects any text containing a newline outright, with the message "multiline updates ride the file contract (write a file, Send a one-line pointer to it)".
  Both validators render findings as a semicolon-joined list today, but `discussionparser`/`planglyph` findings are unbounded in count, so the inline branch the design doc sketches ("inline for a line or two") is not reliably available and a two-branch rule would be a latent `Send` failure on the long branch.
  One shape, always, is simpler and matches the short-notification/content-in-file split the rest of the system uses.
- **Rejected:** the doc's inline-or-file split.
  Also rejected: a per-attempt findings file (`gate-findings-1.md`, `-2.md`) — the run directory is deleted on Done anyway, so history there buys nothing the driver log does not already carry.
- **Note for the plan:** the findings file lives in the run directory, which is ephemeral by construction (`.lyx`, per the Durable-vs-Ephemeral State Invariant) and is deleted by `finalize` on a Done cleanup.
  The durable record of *why* a gate failed is the `logger.Warn` line, which must carry the same formatted findings the removed producers' own warn lines carry today — `formatDiscussionFindings`/`formatPlanFindings` move to the gate closures rather than being deleted with their producers.

### Per-attempt done-signal: the next turn boundary, nothing more

- **Decision:** after `Send`, the gate loop does not introduce any new done-detection.
  It continues `Wait`'s existing poll loop; the next event parsed out of `events.jsonl` is the attempt's done-signal, and the gate runs again on it.
  The compound quiescence definition in the design doc (turn-idle ∧ no child processes under the pane ∧ empty hook-maintained pending-work ledger) is **not built**.
- **Rationale:** the hazard that definition exists to close is an agent that ends its turn while an async in-process subagent is still working.
  On all four gate sites that cannot happen, because `buildSettings` (`internal/shuttleengine/claudeengine/settings.go`) installs a `PreToolUse` hook that **denies the in-process `Agent` tool outright** whenever `cfg.ClaudeDenyAgentTool` is set, and none of the four gated specs sets `ForkSubagents` (`DiscussionSpec` and `PlanSpec` leave it zero; `burlerengine` sets it only for a cluster round, `ClusterFan != ""`, which the two gated segments do not configure).
  The remaining case — a real background shell child — is a genuine OS process, but reaching it would require exporting a pane-descendant probe from `reedengine` (`descendantClosurePIDs` is unexported and built for reap/kill) plus a new hook vocabulary, for a failure mode that degrades to one burned attempt on a half-written artifact rather than to an invalid artifact escaping.
  The budget bounds it; the gate still runs.
  This is a deliberate narrowing of the design doc, taken under its own instruction to validate against the code first.
- **Rejected:** building the compound probe now (large, touches two more packages, guards against a tool that is denied at these sites).
  Also rejected: a per-attempt receipt file named in the re-prompt — it adds a second thing the agent must remember to do, and the doc itself frames it only as a fallback if hooks turn out insufficient, which they are not here.
- **Plan must assert:** that `cfg.ClaudeDenyAgentTool` is on in the shipped `internal/shuttleengine/template.yaml`, and that none of the four gated sites sets `ForkSubagents`.
  If either is false the narrowing does not hold and the task must stop and re-open this decision rather than ship a gate that can fire mid-turn.
- **Known limitation, recorded not fixed:** in an interactive `Discussion-Write` run (`autonomous == false`), `buildSettings` installs the `AskUserQuestion` *marker* hook, which appends to the same `events.jsonl`.
  `pollEventsTick` classifies any newly parsed event as `OutcomeDone` once the output files exist, regardless of event kind, so an `AskUserQuestion` mid-turn ends the attempt's wait early.
  This is already true of the first `Wait` today and the gate loop neither creates nor worsens it; it degrades to a gate run against a mid-turn artifact, i.e. one burned attempt.

### Attempt budget: a row-config key, default 3

- **Decision:** a `gate_attempts:` key in the recipe row's `config:` block, read by the `shedrecipe` entry constructor for each gated row and passed into the producer.
  `0` (absent) means "use the package default", which is `3`.
  The value never travels on its own: it is packed into a `GateSpec` alongside the closure at the point of construction, and that single value is what every downstream seam carries (see the next decision), so a gate and its budget cannot be separated at any hop.
- **Rationale:** the design doc calls for "the same idiom as `max_bounces`" — a per-row declared budget.
  `max_bounces` itself is a top-level `shedengine.ProducerDef` field, but the gate loop lives *inside* a producer and is invisible to the FSM, so the row `config:` block is the right home: it is exactly where `run_subdir`, `require_approved`, `commit_seam`, `approve_seam` and `rubric_stencil` already live, and it keeps the value tunable without a rebuild.
- **Rejected:** a run-wide value in `loom.yaml` — the four sites have different costs (a plan rewrite is not a heading fix) and a single knob cannot express that.
  Also rejected: a bare Go constant — untunable, and the doc explicitly wants it "recorded in the row's envelope/history so the status file shows it", which implies it is a declared value.
- **Surfacing:** the spent attempt count reaches the operator through two channels, and `shedengine.HistoryEntry` is **not** widened for it — its four fields (`Producer`, `Outcome`, `Output`, `At`) stay closed.
  (1) A `logger.Warn` on every failed attempt and on exhaustion, carrying producer name, attempt number, budget, and the formatted findings.
  (2) `GateOutcome.Attempts` on the `Result`, which the producer includes in the `Stuck` log line.
  Widening `HistoryEntry` would change the on-disk status schema every `shed status` consumer reads, for a value only these four rows can ever populate.

### One deadline covers every attempt

- **Decision:** the gate loop runs inside the existing `Wait`, under the `run.deadline` already set at `Start` from `spec.Timeout`.
  No attempt gets a fresh deadline, and the deadline is never extended.
- **Rationale:** the deadline is the operator's configured bound on the whole row (`cfg.DiscussionTimeoutMin`, `cfg.PlanTimeoutMin`, `opts.Timeout` for a burler round).
  A loop that extends its own bound can outlive it by `gate_attempts ×` the original, silently.
  Deadline expiry with the files present already classifies `OutcomeDone` via `classifyDeadlineExpiry`, which then runs the gate one final time — so a timeout mid-gate-loop still cannot let an invalid artifact through.
- **Rejected:** a fresh deadline per attempt; a separate `gate_timeout_s`.

### A Done with no live session still runs the gate

- **Decision:** the gate runs on **every** Done classification `Wait` reaches, including the ones where no session remains to re-prompt: `checkLivenessTick`'s not-tracked and not-live branches, `classifyDeadlineExpiry`, and `finishedDespiteMechanismFailure`.
  When the gate fails on one of those, the loop does not attempt a `Send`; it returns immediately with `GateOutcome{Passed: false, Attempts: 0}`.
  A `Send` that fails for any reason (pane gone, delivery unverified) likewise ends the loop with the attempts spent so far rather than being retried.
- **Rationale:** the whole justification for deleting the three standalone rows is that "no path remains where an invalid artifact leaves a row as `Done`".
  Gating only the live-session path would reopen exactly one: an agent that writes a malformed plan and whose pane then dies is classified `OutcomeDone` by the file contract and would sail past.
  **`Attempts` counts re-prompts actually sent on this run, and nothing else.**
  That is the single rule, and every case reads off it: a gate that passed first try reports `0`; a Done reached with no live session reports however many re-prompts had already been sent before the session was lost, which is `0` when it was lost before the first one; a deadline that expires after N sends reports N.
  The earlier phrasing of this decision implied a deadline-expiry Done always reports `0`, which is wrong whenever attempts preceded it.
- **Rejected:** skipping the gate when no session is live (reopens the hole); treating a dead session as an automatic pass (same hole, louder).

### The four sites and how each gets its gate

- **Decision:** one value, `GateSpec`, is the carrier at every hop — the gate closure and its `gate_attempts` budget are packed together where both are known (the `shedrecipe` entry constructor, which reads the row's `config:` block and `Env` in the same place) and travel as a unit from there.
  - `Discussion-Write` and `Plan-Write`: the two rows' own entry constructors — `discussionWriteEntry` (`internal/shedrecipe/entries_discussionwrite.go:26`) and `planWriteEntry` (`internal/shedrecipe/entries_planwrite.go:33`), **not** the generic `singleLLMEntry` — build the closure from the same `Env` fields the removed `discussionValidateEntry`/`planValidateEntry` read today (`Env.DecisionRecordPath`, `Env.SupportLogPath`, `Env.AnchorPath`, `Env.WorktreeRoot`), pack it with the row's `gate_attempts` into a `GateSpec`, and hand that to `shedadapters.NewSingleLLMProducer`, which passes it through to the gated `Runner` entry point.
    Both rows carry **no `config:` block at all today** and both constructors call `configRejectUnknown(cfg)` with an empty permitted set, so each gains its first config key here: the recipe rows gain `config: {gate_attempts: N}`, the two `configRejectUnknown` calls gain `"gate_attempts"`, and `planWriteEntry`'s doc comment ("The row carries no Config keys of its own, per the Config Strictness Invariant") is corrected in the same change.
  - `Discussion-Burler` and `Plan-Burler`: `burlerengine.RunOpts` gains one `Gate GateSpec` field — the zero value is ungated, which is what `Webster-Burler` passes — threaded into the gated shuttle call in `burlerengine.Engine.Run`.
    `burlerengine.Shuttle` gains the gated method the same way.
    The `shedrecipe` Burler entry constructor builds that `GateSpec` from the row's own `config:` block, and `entries_burler.go`'s `configRejectUnknown` allowlist (today `target`, `fasit`, `rubric`, `rubric_stencil`, `fix-scope`, `tool-use`, `cluster-fan`) gains `gate_attempts`, so all four sites read the budget from one key in one place.
    The gate runs after the round's own handoff and before `Run` parses the review file, so a round can never report back an artifact it made invalid.
  - **The burler resume path carries the same gate.** `shedadapters.BurlerProducer` does not reach a resumed round through `burlerengine.Engine.Run` at all: `probeLiveRound` (`internal/shedadapters/burler.go:447`) calls `p.attach.Attach(spec)` on the shared `Shuttle` seam directly, with a spec it builds itself.
    That path takes the gate from `p.opts.Gate` — the `RunOpts` the producer already holds — and calls the gated attach form, so an attached Discussion/Plan fix round is gated exactly as a freshly-spawned one is.
    This is what makes the "one value, `GateSpec`, at every hop" claim true rather than aspirational: the same field is read at both the spawn hop (`Engine.Run`) and the resume hop (`probeLiveRound`), and no second carrier is introduced into `NewBurlerProducer`.
    `probeLiveRound`'s `OutcomeDone` branch consults the returned `GateOutcome` before reporting the attached round successful, mapping a failed gate exactly as the spawn path does (see the next decision).
- **Seam shape — added methods, not widened ones.** `shuttleengine.Runner` keeps `Run(Spec)` and `Attach(Spec)` exactly as they are (`Run(spec)` becomes `RunGated(spec, GateSpec{})`) and gains `RunGated`/`AttachGated`.
  `shedadapters.Shuttle` gains `RunGated`/`AttachGated` **alongside** its existing `Run`/`Attach`; `burlerengine.Shuttle` gains `RunGated` alongside `Run`.
  This is deliberate rather than widening the existing signatures: `shedadapters.Shuttle` is shared by three consumers — `SingleLLMProducer`, `BurlerProducer`'s attach probe, and `Bouncer` (`internal/shedadapters/bouncer.go:338,364`) — and `Bouncer` has no gate and never will, so widening `Run`/`Attach` would rewrite its call sites to pass a zero value that means nothing there.
  Added methods leave every ungated call site untouched.
- **Churn this creates, in scope and expected:** every test fake implementing `shedadapters.Shuttle` or `burlerengine.Shuttle` gains the new methods, because Go interfaces admit no partial implementation.
  That is the intended cost and matches this seam's own precedent — `Attach` was added to the shared `Shuttle` seam rather than type-asserted as an optional interface, reasoned in `singlellm.go` as "a compile error in a test fake is a better failure than a producer that quietly stops probing".
  The Scope **Out** line "the `Bouncer` rows … untouched" means their *behaviour and wiring*, not that no file they touch compiles differently; `Bouncer`'s own call sites do not change under the added-method shape.
- **Rationale:** the gate is told, never derived, in both places, which keeps `burlerengine` and `shedadapters` free of any `discussionparser`/`planglyph` import and keeps both inside the Told-Geometry Invariant.
  It also matches the doc: "which validator a row gets is declared in the row's config/deps, read by Go — exactly as `commit_seam`/`approve_seam` are declared today".
- **Rejected:** putting the gate on `burlerengine.Profile` — `Profile` is validated and path-resolved data that the round renders into prompts; a func field there would have to be excluded from `validate` and from every profile test's comparison.
  `RunOpts` already carries the per-invocation, non-rendered knobs (`Model`, `Effort`, `Timeout`, `Round`, `NoteID`), which is what a gate is.
  Also rejected: leaving the two `Burler` rounds ungated this task — that is the hole the task exists to close, and it is the only one the deleted rows never covered.

### A `Burler` round's failed gate: carrier and outcome mapping

- **Decision — the carrier.** `burlerengine.Result` gains `Gate *shuttleengine.GateOutcome`, a 1:1 passthrough of the shuttle `Result`'s own field, exactly as `RunDir` and `ForkAudit` already are.
  Nil means the round ran ungated (`Webster-Burler`, always).
  `Engine.Run`'s existing error contract is unchanged: a failed gate is **not** an error and **not** a synthesised `Outcome` — the round classified `OutcomeDone` and the gate is a separate fact about it.
  `Run` consults `result.Gate` before its review-file read and returns the populated `Result` with `Verdict`/`Findings` left empty, because a round whose fix left the artifact invalid has no verdict worth parsing.
- **Decision — the mapping.** In `shedadapters.BurlerProducer.Call`, the `OutcomeDone` branch (today an unconditional `return shedengine.Stuck, OutputPointer{Path: reviewPath}, nil`) splits:
  - gate passed, or no gate → today's behaviour, unchanged.
  - gate failed → archive the round's two output paths through the same `archiveRound()` helper every other non-success exit uses, log a `Warn` carrying the producer, round, attempts spent and the findings text, and return `shedengine.Stuck` with an **empty** `OutputPointer`.
  The empty pointer is the signal, and it is the same one the deleted `discussionValidate`/`planValidate` producers used for exactly this meaning: `Stuck` with a pointer names an artifact for the `Bouncer` to judge, `Stuck` with an empty pointer says there is none.
  Routing is `on_stuck: <the segment's Bouncer>`, unchanged, so the hand-back spends one unit of this row's own `max_bounces` budget and the segment escalates from there when it runs out — which is precisely the escalation the design doc says `Plan-Revalidate`'s old `on_stuck: Plan-Write` edge hands over to.
- **Decision — no retry.** A failed gate does **not** consume or trigger `Call`'s attempt-1/attempt-2 retry.
  That retry exists for `OutcomeDied`/`OutcomeTimeout` — infrastructure — while gate exhaustion is a determinate verdict the gate already re-prompted `gate_attempts` times inside the session.
  A second full round on the same input would re-spend an LLM generation to reach the same answer.
- **Rationale:** archiving is what keeps the hand-back honest.
  `Call`'s own resume logic treats the highest complete round with no recorded verdict as "hand back for judgment, spawn nothing" (`internal/shedadapters/burler.go:295`), so leaving a gate-failed round's review file in place would offer the `Bouncer` a review written over an artifact a Go validator already proved invalid.
  Archiving it means the next `Bouncer` pass judges the artifact itself and, finding it broken, produces a fresh round — costly, but correct, and bounded by the segment's bounce budget rather than unbounded.
- **Rejected:** returning an `error` from the gate-failed branch (`shedengine` persists `failed` and aborts the whole run — too harsh for a condition the segment's own budget is designed to absorb, and it contradicts the design doc's "fails the round back to the Bouncer").
  Also rejected: `Stuck` with the review path (offers the judge a review over a known-invalid artifact).
  Also rejected: a synthesised `burlerengine.Outcome` value (the round genuinely reached `OutcomeDone`; overwriting that loses the fact and breaks every existing branch on it).

### Both plan gates run `ValidateFormat`, not `Validate`

- **Decision:** `Plan-Write`'s gate and `Plan-Burler`'s gate both call `planglyph.ValidateFormat` — the format-only check set.
  `planglyph.Validate` (the full set, including the `plan-unapproved` approval check) keeps exactly one production caller: the `lyx loom validate-plan --require-approved` CLI verb.
  Nothing re-checks the approval flag after `Plan-Bouncer` writes it.
- **Rationale:** `Plan-Bouncer` writes the approval flag through its Go `approve_seam` *after* the review segment settles, so both gate sites run strictly before approval exists.
  Demanding the flag at either would fail every single fix round.
  The approval flag's guarantee rests on the approve seam failing loudly on its own, which is precisely what the design doc states: "The plan approval flag never needed a row … there was never LLM uncertainty there."
- **Rejected:** a third gate on `Plan-Bouncer` running `planglyph.Validate` post-approve — `Bouncer` is not an LLM-running producer in the gate's sense (its judge is, its approve seam is Go), and the design doc is explicit that a pure-Go step codes its check inline in its own control flow, which the approve seam already does.

### Severity handling is shared, not re-derived

- **Decision:** `hasBlockingFinding` (today unexported in `internal/loomshed/planvalidate.go`) moves with the plan gate closure and stays the single implementation of the fail-closed severity predicate: a finding is blocking unless its `Severity` is explicitly `planglyph.SeverityInformational`.
  An informational-only findings set is a **pass** — logged as a Warn, no re-prompt, no attempt charged.
- **Rationale:** the predicate carries a hard-won rationale (crucible round `opus-medium-r6`, R6-27: `planglyph.Severity` is an open string type, so testing "not informational" rather than "equals blocking" is what keeps an unrecognized or zero-valued severity from silently passing).
  Re-deriving it at the gate would fork that reasoning into two copies.
  Informational-only must pass for the same reason it passes today: a create-new-unit finding on a brand-new package is not something the writer can fix, so re-prompting on it would burn the whole budget on a condition that was never wrong.
- **Rejected:** failing the gate on any finding at all; leaving the predicate behind in a package whose producer is being deleted.

### Row removal and resume

- **Decision:** delete the three rows and everything that exists only to serve them — `contracts/recipes/loom-recipe.yaml`'s `Discussion-Validate`, `Plan-Validate`, `Plan-Revalidate` blocks and the rewiring of the `on_done` edges around them (`Discussion-Write` → `Discussion-Bouncer`, `Plan-Write` → `Plan-Bouncer`, `Plan-Bouncer` → `Batchifier`); `loomshed.NameDiscussionValidate`/`NamePlanValidate`/`NamePlanRevalidate` and their `interruptPolicy` entries; `internal/loomshed/discussionvalidate.go` and `internal/loomshed/planvalidate.go` with their tests; the `"DiscussionValidate"` and `"PlanValidate"` registry entries and their `entries_simple.go` constructors.
  **No resume migration.** A run parked on a removed row when the new binary lands is restarted, not migrated.
- **Rationale:** the recipe header's own note is that row names are durable on-disk identities and a rename breaks resume for in-flight tasks — that stays true, and removal is a stronger version of it.
  A migration table mapping three retired names onto successors would be permanent carrying cost for a single-developer, pre-release system where the operator can simply re-run.
  The recipe header comment is updated in the same change to say so, and to correct its own "seventeen row names" count to fourteen.
- **Rejected:** keeping the producers as dead code for a release (nothing constructs them; the coverage guard would need an exemption); shipping a migration table.
- **Stale-mention sweep — mechanical, not a hand list.** The removed names appear far more widely than any enumeration written here would stay correct about: a repo-wide grep for the five strings `Discussion-Validate`, `Plan-Validate`, `Plan-Revalidate`, `DiscussionValidate`, `PlanValidate` hits **53 files** at the time of writing.
  The method is therefore the sweep itself, not a list: run that grep over the whole worktree (excluding `.git` and `_mill`), triage every hit, and land every edit in this task.
  Every such mention is part of this task, not a follow-up — a stale pointer to a row that no longer exists is worse than no pointer.
  Four classes the sweep must not treat as cosmetic:
  - **Deployed normative stencils** — `contracts/stencils/loom/loom-rubric-plan-review.md`, `loom-rubric-discussion-review.md`, `loom-rubric-webster-review.md` tell judges and fixers that mechanical checks are "already enforced upstream" by rows that will not exist.
    They are actively misleading, because the checks now run on that very round's own output rather than upstream.
    Editing them interacts with `internal/stencilstore`'s no-overwrite-on-hash-mismatch rule (Stencil Ownership Invariant): a hash-mismatched file on an operator's disk is never overwritten, with no force-sync carve-out, so the plan must say how an already-seeded worktree picks up the corrected text.
  - **The recipe's own two `fasit.instructions` blocks**, which carry the same misleading claim inline.
  - **Deployed specs and docs** — `contracts/specs/loom-plan-spec.md`, `docs/overview.md`, `README.md`, `manifest/designs/shed.md`, `manifest/designs/shed-recipe.md`, `manifest/designs/loom.md`.
    `docs/overview.md` is among them, which corrects this discussion's own earlier guess that it was "likely untouched".
  - **Go prose** — `internal/loomshed/discussionwrite.go` and `planwrite.go` doc comments, `internal/shuttleengine/attach.go`, `internal/loomcli/start.go`, `internal/loomcli/wiring.go`, and test files including `internal/websterengine/runlevel_test.go` and `internal/shedengine/run_routing_test.go`.

### `Gate Self-Check Parity Invariant` rewritten

- **Decision:** the invariant survives, rewritten around the new shape: *a mechanical gate's closure and its CLI self-check verb call the same package function*.
  Its pair list becomes two entries — `Discussion-Write` + `Discussion-Burler` ↔ `validate-discussion`: `discussionparser.Validate`; `Plan-Write` + `Plan-Burler` ↔ `validate-plan`: `planglyph.ValidateFormat`.
  The `Plan-Revalidate ↔ validate-plan --require-approved: planglyph.Validate` row is dropped, and the verb's `--require-approved` mode is recorded as having no recipe counterpart by design (see the plan-gates decision above).
  Its closing rule — "adding a mechanical gate means adding its verb and its parity check in the same task" — is kept verbatim.
  `internal/loomcli/parity_test.go` is updated to assert the new pairs.
- **Rationale:** the invariant's purpose is that an operator's self-check verb and the automated gate can never disagree about what "valid" means.
  Moving the gate from a row into a producer changes *where* the call sits, not the property.
- **Rejected:** retiring the invariant with the rows — the divergence it prevents is exactly as possible with a closure as with a producer, and arguably easier to introduce.

## Technical context

**The decisive mechanical fact.**
`internal/shuttleengine/wait.go:554` `finalize` removes the reed strand and `os.RemoveAll`s the run directory whenever `outcome == OutcomeDone && !run.spec.KeepPane`.
Everything about where the gate can live follows from this.

**How Done is classified.**
`pollEventsTick` (`wait.go:288`) reads new bytes from `events.jsonl`, parses them via the engine, and — if `allOutputFilesExist(run.spec.OutputFiles)` — returns `OutcomeDone` regardless of the last event's kind.
So in an autonomous run, where the only hook appending to that file is the `Stop` hook, "a new event arrives" *is* "a turn ended".
That is the property the per-attempt done-signal rests on.
Three other paths also reach Done: `checkLivenessTick`'s not-tracked and not-live branches, `classifyDeadlineExpiry`, and `finishedDespiteMechanismFailure` — all four must route through the gate.

**Re-prompt delivery.**
`Run.Send` (`run.go:425`) requires a ready agent pane, rejects multiline and blank text (`validateSendText`, `run.go:437`), and verifies delivery by scanning the pane capture, replaying once if the text never appears (`sendVerified`, `run.go:716`).
`Run.Interrupt` is available but must **not** be used — the gate fires at a turn boundary, when there is no in-progress turn to interrupt.

**Spec validation forbids a second Start.**
`Spec.validate` refuses a spec whose output file already exists ("a pre-existing file would satisfy the file contract immediately"), which is why the re-prompt must reuse the live run rather than starting a new one — and also why `SingleLLMProducer` archives stale outputs before spawning (`archiveStaleOutputs`, `shedadapters/singlellm.go`).

**The attach path.**
`Runner.Attach` → `collectAttachCandidates` → `soleFinishedCandidate` → `reconstructAndWait` rebuilds a `*Run` over a matched `run.json` and calls `Wait` on it.
`SingleLLMProducer.Call` probes via `Attach` *before* archiving anything, deliberately.
A gate threaded through `Wait` is therefore automatically honoured on resume; a gate bolted on above `Runner.Run` would not be.

**The two validators, as they stand.**

- `discussionparser.Validate(decisionRecordPath, supportLogPath) ([]Finding, error)` — stdlib-only package, sole reader of `_lyx/discussion/`'s format (Discussionparser Sole-Parser Invariant).
- `planglyph.ValidateFormat(plan, worktreeRoot) ([]Finding, error)` and `planglyph.Validate(...)` — both take a `*planparser.Plan` from `planparser.ParsePlan(planparser.PlanDir(anchorPath))`.
  `planparser` is the sole parser of the on-disk plan format (Planparser Sole-Parser Invariant), so the gate closure parses through it and never itself.

**What the deleted producers do that must survive.**
`discussionValidate.Call` and `planValidate.Call` each carry a `logger.Warn` whose comment explains it is "the only record anywhere" of why an artifact was refused, plus the two formatter helpers (`formatDiscussionFindings`, `formatPlanFindings`) that make the log line and the CLI verb's envelope describe a violation identically by calling each `Finding`'s own `Error()`.
Both helpers and both warn lines move into the gate closures.
`internal/loomshed/gatefindings_test.go` is the test that pins this property and must be carried over, not deleted.

**Where the gated rows' specs come from.**
`loomengine.DiscussionSpec` (`internal/loomengine/discussion.go:22`) — `OutputFiles: [decisionRecordPath, supportLogPath]`, `Interactive`/`AwaitOperator` both `!autonomous`, `Timeout: cfg.DiscussionTimeoutMin`.
`loomengine.PlanSpec` (`internal/loomengine/plan.go:69`) — `OutputFiles: [overviewPath]`, `Interactive: false`, `Timeout: cfg.PlanTimeoutMin`.
Note that `Plan-Write`'s declared output file is the overview alone, while the plan gate validates the whole parsed plan directory — that asymmetry exists today and is not changed here.

**`burlerengine.Engine.Run`** (`internal/burlerengine/engine.go:105`) validates the profile, materializes three instruction files under `.lyx/burler/round-*`, builds a `shuttleengine.Spec` with `OutputFiles: [p.ReviewPath, p.FixerReportPath]`, calls `e.shuttle.Run(spec)`, and only on `OutcomeDone` reads and strictly parses the review file.
The gate belongs between the shuttle call and the review-file read: a round whose fix left the artifact invalid must not get as far as reporting a verdict.

**Recipe plumbing.**
`contracts/recipes/loom-recipe.yaml` is embedded by `contracts/recipes/recipes.go`, parsed by `internal/shedbuild` (Recipe-Format Sole-Parser Invariant), and its engine names resolve through `internal/shedrecipe`'s `map[string]Constructor` reached only via `Lookup`/`Names` (Shed Recipe Registry Invariant — no `init()`, no runtime `Register`).
`internal/loomrecipe` holds the guards: `coverage_guard_test.go`, `shape_test.go` (asserts the concrete producer type per row), `sequence_test.go`, `resume_test.go`, `revalidate_test.go` (this one goes away with the row), `approveseam_test.go`, `overlay_seam_guard_test.go`, `interruptpolicy_meta_test.go`.
All of them move with the recipe.

## Constraints

From `CONSTRAINTS.md`, the ones this task actually binds against:

- **Gate Self-Check Parity Invariant** — rewritten by this task (see the decision above).
  This is the one cross-cutting invariant the task changes, and per `CLAUDE.md` it lands in the same commit.
- **Completion Signal Invariant** — every `shuttleengine` path finalizing a *negative* answer to "did this run finish" must consult `allOutputFilesExist` first, pinned by two AST scans in `completionsignal_enforcement_test.go` over `wait.go`/`attach.go`.
  The gate runs strictly after a positive Done and adds no new outcome, so no audited return site changes disposition — but the scans pin *the set of return sites*, so any new `return` added inside `Wait` will trip them and must be reviewed deliberately, not silenced.
- **Shuttle Provider-Seam Invariant** — provider specifics live only under `internal/shuttleengine/claudeengine`; `shuttleengine` never imports it.
  The gate loop must ask "is a new event in" through the existing `Engine.ParseEvents` seam, never by knowing anything about Claude's hook payloads.
- **Told-Geometry Invariant** — `shuttleengine`, `burlerengine`, `shedrecipe` and `loomshed` are all bound packages: no `internal/lyxcwd` import, every path told.
  The gate closure is built where the paths already are (`shedrecipe`'s entry constructors, from `Env`), never inside `shuttleengine`.
- **Planparser Sole-Parser Invariant** / **Discussionparser Sole-Parser Invariant** — the gate closures call the existing parsers; no plan or discussion parsing may be written anywhere new.
- **Shed Recipe Registry Invariant** — removing two registry entries keeps the single-map, `Lookup`-only shape; no `init()` registration may be introduced for the gate.
- **Shed Producer-Seam Invariant** — `internal/shedengine` imports only stdlib, `state`, `lock`.
  This is why the attempt counter does not become a `HistoryEntry` field.
- **Test Tier Purity Invariant** — the gate-loop unit tests must be untagged and offline: no `exec.Command`, no `gitexec`, no `hubforge.NewHub`, no `time.Sleep` ≥ 1s.
  Anything needing a real pane is `integration`/`smoke`-tagged.
- **Live-Substrate Spawn Observability** — the re-prompt is not a spawn, so it needs no spawn log, but the gate's failure and exhaustion lines are required by the "only record anywhere" property the deleted producers carry.
- **Documentation Lifecycle** — see **Docs**.
- **Quarry CGO Requirement Invariant** — building and testing this repo needs `CGO_ENABLED=1` and a C compiler on `PATH`.
  `planglyph` resolves refs through quarry, so the plan gate's tests must either supply a resolvable fixture or assert on the error path, which maps to a returned error (never a failed attempt).

## Testing

**`internal/shuttleengine` — the gate loop (Tier 1, untagged, offline; the TDD candidate).**
Drive it through the existing fake `Engine`/`ReedOps` fakes in `fakes_test.go` with a scripted events file and a fake clock.
Scenarios that must be covered:

- Gate passes on the first Done → `Result.Gate.Passed` true, `Attempts` 0, no `Send` issued, `finalize` cleanup runs exactly as an ungated run's does.
- Gate fails once, agent's next turn fixes it → exactly one `Send`, whose text is a single line naming the findings file; findings file contains the first `GateResult.Findings`; `Attempts` 1; final outcome Done/passed.
- Gate fails every attempt → exactly `gate_attempts` `Send` calls, `Gate.Passed` false, `Attempts == gate_attempts`, `Outcome` still `OutcomeDone`, `FindingsPath` naming the last attempt's file.
- Gate returns a non-nil `error` → `Wait` returns an error, no `Send` issued, no attempt charged, and the error wraps the gate's own.
- Gate fails on a Done reached with no live session — one case per path: `checkLivenessTick` not-tracked, `checkLivenessTick` not-live, `classifyDeadlineExpiry`, `finishedDespiteMechanismFailure` — each yields `Gate.Passed` false, `Attempts` 0, no `Send`.
- `Send` itself fails mid-loop → loop ends with the attempts spent so far, no retry, no panic.
- The run deadline expires between attempts → the loop stops, the gate runs once more on the deadline-expiry Done, and the result reports honestly.
- Zero `GateSpec` → byte-for-byte today's behaviour (this one is the regression guard for every ungated row).

**`internal/burlerengine`.**
A round whose gate fails returns a `Result` with `Gate` populated, `Verdict`/`Findings` empty, `Outcome` still `OutcomeDone`, and a nil error — the review file is never read.
A round carrying the zero `GateSpec` behaves exactly as today, `Gate` nil (guards `Webster-Burler`).
Reuse the package's existing `Shuttle` fake, extended with the gated method.

**`internal/shedadapters`.**
`SingleLLMProducer` maps `OutcomeDone` + failed `GateOutcome` onto `shedengine.Stuck` with an empty `OutputPointer`, and `OutcomeDone` + passed onto `Done` with the first output file as pointer.
`BurlerProducer.Call` maps a gate-failed round onto `Stuck` with an empty pointer **after** archiving the round's two output paths, does not consume the attempt-1/attempt-2 retry, and leaves the gate-passed path byte-for-byte as today.
`BurlerProducer.probeLiveRound` passes `p.opts.Gate` into the gated attach call and maps an attached round's failed gate identically — this is the regression guard for the resume hole, and it must fail if the probe ever reverts to the ungated `Attach`.
The existing attach-before-archive ordering tests must still pass unchanged.

**`internal/loomshed`.**
`gatefindings_test.go` carried over onto the gate closures: a failing gate's `logger.Warn` must carry the formatted findings.
Plus the fail-closed severity predicate's existing table (unrecognized severity, zero-value severity, informational-only).

**`internal/loomcli`.**
`parity_test.go` updated to the two new pairs; `validate_test.go` unchanged (the verbs do not change).

**`internal/loomrecipe`.**
Coverage guard, shape, sequence and resume tests updated for the fourteen-row recipe; `revalidate_test.go` deleted with its row; `interruptpolicy_meta_test.go` updated for the three removed entries.

**Integration / smoke (tagged).**
One end-to-end that a real agent, re-prompted after a deliberately-invalid first artifact, fixes it and the row reports `Done` — the only test that exercises real `Send` delivery against a real pane.

**Whole-repo gate.**
`go build ./... && go test ./...` with `CGO_ENABLED=1`, plus the sandbox suite per the Sandbox Suite Coverage invariant.

## Docs

All in the same commit, per `CLAUDE.md`'s task-completion rule:

- `CONSTRAINTS.md` — `Gate Self-Check Parity Invariant` rewritten.
- `manifest/designs/loom.md` — the recipe walkthrough and any gate/validate-row prose.
- `manifest/designs/producer-gates.md` — marked shipped, with the two deliberate narrowings recorded against the doc's own text: the per-attempt quiescence definition (turn boundary only, with the `Agent`-tool-denied justification) and the findings delivery (always a file).
- `manifest/roadmap.md` — the Planned item moves on completion.
- `contracts/recipes/loom-recipe.yaml` — header comment: seventeen rows → fourteen, the escalate-set sentence re-counted, the removed rows' rationale paragraphs dropped, and the two `fasit.instructions` blocks that tell fixers their checks are "already enforced upstream" rewritten to say the round's own gate enforces them.
- `docs/overview.md` — it names the removed rows (confirmed by the sweep), so it changes regardless of whether the module table does; no new module is expected.
- `contracts/specs/loom-plan-spec.md`, `README.md`, `manifest/designs/shed.md`, `manifest/designs/shed-recipe.md`, and the three `contracts/stencils/loom/loom-rubric-*.md` files — everything the stale-mention sweep turns up, per the **Row removal and resume** decision.

## Q&A log

- **Q:** Where does the gate loop live — inside `shuttleengine`, in a new package above it, or duplicated per producer? **A:** [auto-pick] Inside `shuttleengine`, on `Wait`'s Done path before `finalize`. **Why:** `finalize` removes the strand and deletes the run dir on Done, so no caller above shuttle still has a session to re-prompt; `Wait` is also the single funnel both the spawn and the attach paths pass through.
- **Q:** Where are `Gate`/`GateResult` declared? **A:** [auto-pick] In `internal/shuttleengine` beside `Spec`. **Why:** one consumer, no new package, no import-allowlist or seam-enforcement churn.
- **Q:** Does `Gate` take an `artifactDir string` as the design doc sketches? **A:** [auto-pick] No — `func() (GateResult, error)`, closing over its told paths. **Why:** the two validators take different path shapes (two file paths vs anchor + worktree root), so no single directory argument fits either, and shuttle must not know artifact geometry.
- **Q:** How does gate exhaustion reach the producer — a fifth `Outcome`, or a field on `Result`? **A:** [auto-pick] `Result.Gate *GateOutcome`; `Outcome` stays closed at four values. **Why:** a new negative-shaped outcome whose premise is that the output files exist would need a carve-out in the Completion Signal Invariant, which two AST scans pin.
- **Q:** Findings inline for short complaints, or always a file? **A:** [auto-pick] Always a file, with a one-line `Send` naming it. **Why:** `validateSendText` rejects any newline outright, and findings counts are unbounded, so the inline branch is a latent `Send` failure.
- **Q:** Build the design doc's compound quiescence probe (turn-idle ∧ proctree ∧ hook ledger) for per-attempt done? **A:** [auto-pick] No — the next turn boundary alone. **Why:** `buildSettings` denies the in-process `Agent` tool at all four gated sites and none sets `ForkSubagents`, so the async-subagent hazard the compound definition guards cannot arise there; the plan must assert both facts and stop if either is false.
- **Q:** Where is the attempt budget declared, and what is the default? **A:** [auto-pick] A `gate_attempts:` recipe row `config:` key, default 3. **Why:** same block as `run_subdir`/`require_approved`/`commit_seam`; per-row because the four sites have different re-prompt costs.
- **Q:** Does the attempt count widen `shedengine.HistoryEntry`? **A:** [auto-pick] No — `logger` lines plus `GateOutcome.Attempts`. **Why:** `HistoryEntry` is the on-disk status schema every `shed status` consumer reads; only four rows could ever populate the field.
- **Q:** One deadline for all attempts, or a fresh one per attempt? **A:** [auto-pick] One — the existing `run.deadline` from `spec.Timeout`. **Why:** a self-extending loop can silently outlive the operator's configured bound by the budget multiple.
- **Q:** What happens to the pane on gate exhaustion? **A:** [auto-pick] Ordinary Done-path cleanup. **Why:** no new pane-leak class; the durable record is the log line, and the row's `Stuck` carries the reason.
- **Q:** Does the gate run on a Done reached with no live session (pane died, reed lost the strand, deadline expired with files present)? **A:** [auto-pick] Yes, with no `Send` and `Attempts: 0`. **Why:** gating only the live path reopens exactly the hole the three deleted rows were covering.
- **Q:** How do the two `Burler` fix-step sites get their gate — `Profile` or `RunOpts`? **A:** [auto-pick] `RunOpts`. **Why:** `Profile` is validated, path-resolved data rendered into prompts; `RunOpts` already holds the per-invocation non-rendered knobs, which is what a gate is.
- **Q:** Is `Webster-Burler` gated? **A:** [auto-pick] No — the zero `GateSpec`. **Why:** there is no mechanical validator over a committed diff.
- **Q:** Do the plan gates run `ValidateFormat` or `Validate`? **A:** [auto-pick] `ValidateFormat` at both sites. **Why:** both run before `Plan-Bouncer`'s approve seam writes the flag, so `Validate` would fail every fix round; the flag's guarantee rests on the approve seam failing loudly, per the design doc.
- **Q:** Does anything replace `Plan-Revalidate`'s post-approval re-check? **A:** [auto-pick] No. **Why:** its format half moves into `Plan-Burler`'s own gate, which is strictly earlier and un-skippable; its approval half was never an LLM-uncertainty problem.
- **Q:** Is the fail-closed severity predicate shared or re-derived? **A:** [auto-pick] Shared — `hasBlockingFinding` moves with the gate closure. **Why:** it encodes a crucible-round finding (`planglyph.Severity` is an open string type, so test not-informational, never equals-blocking); two copies would fork that reasoning.
- **Q:** Does an informational-only findings set fail the gate? **A:** [auto-pick] No, it passes, logged as a Warn. **Why:** same reason it passes today — a create-new-unit finding is not something the writer can fix, so re-prompting on it burns the whole budget on a condition that was never wrong.
- **Q:** Do the removed rows get a resume migration? **A:** [auto-pick] No — an in-flight run parked on a removed row is restarted. **Why:** permanent carrying cost for a pre-release, single-operator system; the recipe header is updated to say so.
- **Q:** Does the `Gate Self-Check Parity Invariant` survive? **A:** [auto-pick] Yes, rewritten to two pairs binding the gate closure and the CLI verb to one package function. **Why:** the divergence it prevents is as possible with a closure as with a producer.
- **Q:** Are the prose mentions of the removed rows (fixer `fasit.instructions`, producer doc comments, `attach.go`, `start.go`, `wiring.go`) part of this task? **A:** [auto-pick] Yes, all of them. **Why:** the fixer instructions in particular would actively mislead — they tell a round its checks are enforced upstream by a row that no longer exists, when they are now enforced on that round's own output.
- **Q:** How does a `Burler` round report a failed gate, and how does `BurlerProducer.Call` map it? **A:** [auto-pick] `burlerengine.Result` gains a `Gate *shuttleengine.GateOutcome` passthrough (nil = ungated); `Call` archives the round's two output paths and returns `shedengine.Stuck` with an empty pointer, spending one unit of the row's bounce budget, with no attempt-2 retry. **Why:** the empty pointer is the repo's existing "no artifact to judge" signal (the deleted validate producers used exactly it); archiving stops the `Bouncer` being offered a review written over a known-invalid artifact; `Stuck`-to-Bouncer is the escalation the design doc hands `Plan-Revalidate`'s old edge to, where a returned error would abort the whole run.
- **Q:** A resumed `Burler` round reaches shuttle through `probeLiveRound`'s own `Attach`, not through `Engine.Run` — who supplies its gate? **A:** [auto-pick] `p.opts.Gate`, the `RunOpts` the producer already holds, passed into the gated attach form. **Why:** no second carrier is needed, and it makes "one `GateSpec` at every hop" true at the resume hop as well as the spawn hop — otherwise a resumed fix round completes ungated, which is the exact hole the task exists to close.
