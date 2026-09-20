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

- A gate contract (`Gate`, `GateResult`, `GateOutcome`) declared in `internal/shuttleengine`, and the attempt loop that drives it.
- `internal/shuttleengine`: `Wait` consults an optional per-run gate on its Done path *before* `finalize`; on failure it `Send`s a one-line re-prompt naming a findings file and keeps polling for the next turn boundary; bounded by an attempt budget.
- Gated entry points on `Runner` reaching both the spawn path and the attach/resume path, so a resumed run is gated exactly as a fresh one is.
- `internal/burlerengine`: `RunOpts` gains an optional `Gate`; its `Shuttle` seam widens to the gated form so a fix round's own output is gated before the round reports back to its `Bouncer`.
- Four gate sites, two validators: `Discussion-Write` and `Discussion-Burler` share `discussionparser.Validate`; `Plan-Write` and `Plan-Burler` share `planglyph.ValidateFormat`.
- Removal of the `Discussion-Validate`, `Plan-Validate` and `Plan-Revalidate` rows from `contracts/recipes/loom-recipe.yaml`, their `loomshed` `Name*` constants, their `interruptPolicy` entries, their `shedrecipe` registry entries, and the `loomshed` producers `NewDiscussionValidate`/`NewPlanValidate` — the recipe goes from seventeen rows to fourteen.
- A `gate_attempts:` row-config key, read by the `shedrecipe` entry constructors that build the four gated rows.
- `CONSTRAINTS.md`: the `Gate Self-Check Parity Invariant` rewritten around the new pairing.
- Docs in the same commit (see **Docs**).

**Out:**

- `discussionparser`, `planparser` and `planglyph` themselves — the gate *is* their existing functions, called from a new site. No check is added, removed, or changed.
- The `lyx loom validate-discussion` / `validate-plan` CLI self-check verbs. They stay, including `--require-approved`; only the invariant that binds them to the recipe is rewritten.
- The compound quiescence probe the design doc sketches (turn-idle ∧ no pane child processes ∧ empty hook-maintained pending-work ledger). See the **Per-attempt done-signal** decision: it is deliberately not built in this task, and nothing is exported from `reedengine` for it.
- Any new hook event in `internal/shuttleengine/claudeengine/settings.go`. The Stop hook already in place is the whole mechanism.
- Gating `Webster-Burler`. There is no mechanical validator over a committed diff, so the third segment gets no gate and `burlerengine`'s `Gate` stays nil there.
- The `Bouncer` rows, the `approve_seam`, the `commit_seam`, and every bounce budget — untouched.
- Any resume migration for in-flight runs parked on a removed row (see **Row removal and resume**).
- Adding a fifth `shedengine.Outcome` value, or any change to the Completion Signal Invariant's negative-answer set.

## Decisions

### The gate loop lives inside `shuttleengine.Wait`, not above it

- **Decision:** the attempt loop runs inside `internal/shuttleengine`, on `Wait`'s Done path, before `finalize` is reached.
  `*Run` gains an unexported gate field plus the budget; `Runner` gains gated entry points (`RunGated`, and the attach-side equivalent) that thread it in.
  Plain `Runner.Run`/`Runner.Attach` keep today's behaviour by supplying no gate, so every ungated row is bit-for-bit unchanged.
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

  // GateOutcome is what a gated run reports back about its gate.
  type GateOutcome struct {
      Passed       bool
      Attempts     int    // re-prompt attempts spent; 0 means the gate passed first try, or no session was live to re-prompt
      FindingsPath string // the findings file of the LAST failing attempt; empty when Passed
  }
  ```

  All three live in `internal/shuttleengine` beside `Spec`.
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
  Attempts is `0` rather than `1` because no attempt was offered to the agent — the number reads as "re-prompts spent", and spending one on a dead session would be a lie.
- **Rejected:** skipping the gate when no session is live (reopens the hole); treating a dead session as an automatic pass (same hole, louder).

### The four sites and how each gets its gate

- **Decision:**
  - `Discussion-Write` and `Plan-Write`: the `shedrecipe` entry constructors (`singleLLMEntry`-family in `internal/shedrecipe/entries_simple.go`) build the gate closure from the same `Env` fields the removed `discussionValidateEntry`/`planValidateEntry` read today (`Env.DecisionRecordPath`, `Env.SupportLogPath`, `Env.AnchorPath`, `Env.WorktreeRoot`) and hand it to `shedadapters.NewSingleLLMProducer`, which passes it through to the gated `Runner` entry point.
    `shedadapters.Shuttle` widens accordingly.
  - `Discussion-Burler` and `Plan-Burler`: `burlerengine.RunOpts` gains a `Gate` field (nil = ungated, which is what `Webster-Burler` passes), threaded into the gated shuttle call in `burlerengine.Engine.Run`.
    `burlerengine.Shuttle` widens the same way.
    The gate runs after the round's own handoff and before `Run` parses the review file, so a round can never report back an artifact it made invalid.
- **Rationale:** the gate is told, never derived, in both places, which keeps `burlerengine` and `shedadapters` free of any `discussionparser`/`planglyph` import and keeps both inside the Told-Geometry Invariant.
  It also matches the doc: "which validator a row gets is declared in the row's config/deps, read by Go — exactly as `commit_seam`/`approve_seam` are declared today".
- **Rejected:** putting the gate on `burlerengine.Profile` — `Profile` is validated and path-resolved data that the round renders into prompts; a func field there would have to be excluded from `validate` and from every profile test's comparison.
  `RunOpts` already carries the per-invocation, non-rendered knobs (`Model`, `Effort`, `Timeout`, `Round`, `NoteID`), which is what a gate is.
  Also rejected: leaving the two `Burler` rounds ungated this task — that is the hole the task exists to close, and it is the only one the deleted rows never covered.

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
- **Careful:** `internal/loomshed/discussionwrite.go` and `planwrite.go` both carry doc comments referencing `Discussion-Validate`/`Plan-Validate` as the downstream judge, and `internal/shuttleengine/attach.go`, `internal/websterengine/runlevel_test.go`, `internal/shedengine/run_routing_test.go`, `internal/loomcli/start.go`, `internal/loomcli/wiring.go`, `internal/burlerengine`'s two gated profiles in the recipe (`fasit.instructions`, which tell the fixer those checks are "already enforced upstream by Discussion-Validate / Plan-Validate") all name the removed rows in prose.
  Every such mention is part of this task, not a follow-up — a stale pointer to a row that no longer exists is worse than no pointer, and the fixer instructions in particular would be actively misleading (the checks are now enforced on that very round's own output, not upstream).

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
- Nil gate → byte-for-byte today's behaviour (this one is the regression guard for every ungated row).

**`internal/burlerengine`.**
A round whose gate fails reports back without parsing the review file; a round with a nil gate behaves exactly as today (guards `Webster-Burler`).
Reuse the package's existing `Shuttle` fake.

**`internal/shedadapters`.**
`SingleLLMProducer` maps `OutcomeDone` + failed `GateOutcome` onto `shedengine.Stuck` with an empty `OutputPointer`, and `OutcomeDone` + passed onto `Done` with the first output file as pointer.
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
- `docs/overview.md` — only if the module table or execution stack changes; no new module is expected, so likely untouched.

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
- **Q:** Is `Webster-Burler` gated? **A:** [auto-pick] No — nil gate. **Why:** there is no mechanical validator over a committed diff.
- **Q:** Do the plan gates run `ValidateFormat` or `Validate`? **A:** [auto-pick] `ValidateFormat` at both sites. **Why:** both run before `Plan-Bouncer`'s approve seam writes the flag, so `Validate` would fail every fix round; the flag's guarantee rests on the approve seam failing loudly, per the design doc.
- **Q:** Does anything replace `Plan-Revalidate`'s post-approval re-check? **A:** [auto-pick] No. **Why:** its format half moves into `Plan-Burler`'s own gate, which is strictly earlier and un-skippable; its approval half was never an LLM-uncertainty problem.
- **Q:** Is the fail-closed severity predicate shared or re-derived? **A:** [auto-pick] Shared — `hasBlockingFinding` moves with the gate closure. **Why:** it encodes a crucible-round finding (`planglyph.Severity` is an open string type, so test not-informational, never equals-blocking); two copies would fork that reasoning.
- **Q:** Does an informational-only findings set fail the gate? **A:** [auto-pick] No, it passes, logged as a Warn. **Why:** same reason it passes today — a create-new-unit finding is not something the writer can fix, so re-prompting on it burns the whole budget on a condition that was never wrong.
- **Q:** Do the removed rows get a resume migration? **A:** [auto-pick] No — an in-flight run parked on a removed row is restarted. **Why:** permanent carrying cost for a pre-release, single-operator system; the recipe header is updated to say so.
- **Q:** Does the `Gate Self-Check Parity Invariant` survive? **A:** [auto-pick] Yes, rewritten to two pairs binding the gate closure and the CLI verb to one package function. **Why:** the divergence it prevents is as possible with a closure as with a producer.
- **Q:** Are the prose mentions of the removed rows (fixer `fasit.instructions`, producer doc comments, `attach.go`, `start.go`, `wiring.go`) part of this task? **A:** [auto-pick] Yes, all of them. **Why:** the fixer instructions in particular would actively mislead — they tell a round its checks are enforced upstream by a row that no longer exists, when they are now enforced on that round's own output.
