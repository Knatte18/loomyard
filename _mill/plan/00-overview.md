# Plan: Producer gates: mechanical gates before session release

```yaml
task: 'Producer gates: mechanical gates before session release'
slug: 'producer-gates'
approved: false
started: '20260920-142558'
parent: 'main'
root: ""
verify: go vet ./...
discussion_sha: cadd0ce7da229bbc03913c786c4d9eb9b81e88fc
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches.
Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: shuttle-gate-loop
    file: 01-shuttle-gate-loop.md
    depends-on: []
    verify: go test ./internal/shuttleengine/...
  - number: 2
    name: seam-and-producers
    file: 02-seam-and-producers.md
    depends-on: [1]
    verify: go test ./internal/burlerengine/... ./internal/shedadapters/... ./internal/shedrecipe/... ./internal/shedbuild/... ./internal/loomrecipe/...
  - number: 3
    name: loomshed-gates
    file: 03-loomshed-gates.md
    depends-on: [1]
    verify: go test ./internal/loomshed/...
  - number: 4
    name: shedrecipe-wiring
    file: 04-shedrecipe-wiring.md
    depends-on: [2, 3]
    verify: go test ./internal/shedrecipe/... ./internal/loomrecipe/... && go test -tags smoke -run '^$' ./internal/loomcli/...
  - number: 5
    name: row-removal
    file: 05-row-removal.md
    depends-on: [4]
    verify: go test ./internal/loomshed/... ./internal/shedrecipe/... ./internal/loomrecipe/... ./internal/loomcli/...
  - number: 6
    name: parity-docs-sweep
    file: 06-parity-docs-sweep.md
    depends-on: [5]
    verify: go test ./contracts/stencils/... ./internal/lyxcwd/... ./internal/loomcli/... ./internal/loomrecipe/... && go test -tags smoke -run '^$' ./internal/loomcli/... && go test -tags integration -run '^$' ./internal/websterengine/...
```

## Shared Decisions

_Cross-cutting decisions every batch inherits: naming conventions, error-handling posture, test frameworks, style/lint constraints.
One subsection per decision.
Batch-local decisions live in each batch file._

### Decision: added forms, never widened signatures

- **Decision:** every seam this task touches gains a *new* method or constructor beside the existing one rather than having the existing one widened.
  `shuttleengine.Runner` gains `RunGated`/`AttachGated`; `shedadapters.Shuttle` gains `RunGated`/`AttachGated`; `burlerengine.Shuttle` gains `RunGated`; `shedadapters` gains `NewSingleLLMProducerGated`.
  Each original delegates to the new form with the zero `GateSpec`.
- **Rationale:** `shedadapters.Shuttle` is shared by three consumers — `SingleLLMProducer`, `BurlerProducer`'s attach probe, and `Bouncer` — and `Bouncer` has no gate and never will, so widening `Run`/`Attach` would rewrite its six call sites to pass a value that means nothing there.
  The same reasoning applies one layer down to `Runner` and one layer up to the producer constructor, where widening would touch roughly thirty test call sites, the generic single-LLM registry row, and the attach-probe smoke harness, none of which has a gate.
  The accepted cost is that every test fake implementing either seam gains the new methods, because Go admits no partial implementation — which is this seam's own precedent, where `Attach` was added to the shared seam rather than type-asserted as an optional interface, reasoned as "a compile error in a test fake is a better failure than a producer that quietly stops probing".
- **Applies to:** all batches

### Decision: the gate verdict is a fact on Result, never a fifth Outcome

- **Decision:** `shuttleengine.Result` gains `Gate *GateOutcome`, nil meaning "this run had no gate" and never "the gate passed".
  `Outcome` stays closed at its four values; a gate that exhausted its budget reports `OutcomeDone` with a failed `GateOutcome`.
  `burlerengine.Result` carries the same field as a 1:1 passthrough.
- **Rationale:** the Completion Signal Invariant governs every path in `shuttleengine` that finalizes a *negative* answer to "did this run finish" and requires each to consult the file contract first.
  A fifth outcome would be negative-shaped but its whole premise is that the output files DO exist and are invalid — a carve-out in the middle of an invariant audited across five crucible rounds and pinned by two AST scans.
  Reporting it as a field leaves that invariant's value set untouched: the gate runs strictly *after* a positive Done, never as part of classifying one.
  The two scans still move, because the gate adds two error returns and renames one enclosing function, and batch 1 re-audits them deliberately rather than silencing them.
- **Applies to:** all batches

### Decision: a gate error is never "not passed"

- **Decision:** a non-nil `error` from a gate closure fails the whole run as an ordinary producer error.
  It never burns an attempt and never reaches the LLM.
  Only `GateResult{Passed: false}` produces findings, a re-prompt, and an attempt charge.
  The one carve-out is `planparser.ParsePlan`: an error satisfying `errors.As(err, new(*fs.PathError))` is a returned error, and every other `ParsePlan` error is findings.
  Every `planglyph` error stays a returned error in full and is not part of the carve-out.
- **Rationale:** mixing the two spends the budget on infrastructure faults and makes the exhaustion message lie about the cause — the LLM cannot fix a missing quarry binary, and three absurd re-prompts would hide the real fault.
  The `ParsePlan` carve-out exists because a malformed overview is the single most LLM-fixable defect class there is, and routing it to a returned error would abort the run without ever re-prompting, which is the exact failure this task exists to remove.
  It is a reasoned reversal of the deleted plan-validate producer's disposition rather than an oversight: that producer's rationale was about a *cold respawn* that knows nothing of the complaint, and the gate's bounce target is the live session that just wrote the file.
  With the carve-out both gates behave identically on a missing-or-malformed artifact, since the discussion validator already reports a missing file as a finding and only a non-not-exist read failure as an error.
- **Applies to:** all batches

### Decision: findings always ride a file, never the re-prompt text

- **Decision:** a failed attempt writes `GateResult.Findings` to a per-run findings file in the run directory and sends a single line naming its absolute path.
  Each attempt overwrites the same file.
  There is no inline branch for short findings.
- **Rationale:** the send path rejects any text containing a newline outright, with a message that multiline updates ride the file contract, and both validators render findings as an unbounded list — so an inline branch is not reliably available and a two-branch rule would be a latent delivery failure on the long branch.
  One shape, always, is simpler and matches the short-notification/content-in-file split the rest of the system already uses.
  The run directory is ephemeral by construction and is deleted on the Done cleanup every exhausted gate takes, so the durable record of *why* a gate failed is the closure's own `logger.Warn` line, which must carry the same formatted findings the deleted producers' warn lines carry today — and `GateOutcome.FindingsPath` is therefore diagnostic text a producer must never dereference.
- **Applies to:** batches 1, 2, 3

### Decision: one GateSpec at every hop

- **Decision:** the gate closure and its `gate_attempts` budget are packed into a single `GateSpec` at the one point where both are known — the `shedrecipe` entry constructor, which reads the row's `config:` block and `Env` in the same place — and that one value is what every downstream seam carries.
  For the burler rows the carrier is `RunOpts.Gate`, read at both the spawn hop and the resume hop, so no second carrier enters the producer constructor.
- **Rationale:** a gate and its budget cannot be separated at any hop if they never travel apart.
  The resume hop is the one that makes this concrete rather than aspirational: a resumed fix round reaches shuttle through the producer's own attach probe rather than through the round engine, so a design carrying the gate only through the engine would leave every resumed round ungated — the exact hole the task exists to close, reopened on the resume path.
- **Applies to:** batches 2, 4

### Decision: attempts counts re-prompts actually sent, and nothing else

- **Decision:** `GateOutcome.Attempts` is incremented only after a send returns without error.
  A gate that passed first try reports 0; a Done reached with no live session reports however many re-prompts had already been sent before the session was lost, which is 0 when it was lost before the first one; a deadline that expires after N sends reports N; a send that fails ends the loop with the attempts spent so far and is never retried.
- **Rationale:** one rule every case reads off, rather than a per-path convention that has to be remembered at four `finalize` call sites.
  It also makes the exhaustion log honest about what the agent was actually told, which is the only thing an operator reading a halted run can act on.
  The budget is spent in memory inside one wait and is not carried across process boundaries: every gated row is reinvoke-policy, and a re-attached session is strictly further along than a fresh one, so refusing it new attempts would halt a run that is making progress.
  The unbounded case needs repeated external interruption, which is an operator action; an uninterrupted run always terminates at the budget, in `Stuck`.
- **Applies to:** batches 1, 2

### Decision: the two producers' output pointers mean different things, deliberately

- **Decision:** a gate-failed writer row returns `Stuck` with the **artifact** pointer; a gate-failed burler round returns `Stuck` with an **empty** pointer.
- **Rationale:** each row's pointer means "what, if anything, the next actor should look at", and the two answers differ because the next actors differ.
  A writer row has no judge downstream of its `Stuck` — the run halts for a human — so its pointer is free to carry the meaning the commit decorators key on, which is what keeps the invalid artifact committed and diagnosable instead of dirtying the weft.
  A burler round's emptiness has a consumer: it tells the segment's Bouncer there is no round artifact to judge, which is the same signal the deleted validate producers used for exactly this meaning.
  A test asserting an empty pointer on the writer side would silently disable commit-on-gate-failure, so both producers get an explicit pointer assertion rather than an inherited one.
- **Applies to:** batches 2, 3

### Decision: every test fake evaluates the gate once, and runs no re-prompt loop

- **Decision:** every `shedadapters.Shuttle` and `burlerengine.Shuttle` test fake implements `RunGated`/`AttachGated` by delegating to its existing `Run`/`Attach` body and then, when the received `Gate` is non-nil and the delegated outcome is Done, invoking the closure exactly once, returning its error if non-nil and otherwise stamping the `GateOutcome` onto the result.
  No fake simulates the re-prompt loop.
- **Rationale:** the fakes exist to drive routing, not tmux — there is no pane to send into.
  Evaluating the closure once is what keeps the wiring genuinely exercised end to end: without it, a gate-failed writer row would silently stop halting the recipe's own sequence tests the moment batch 4 wires the gates, and the whole graph-level coverage would pass for the wrong reason.
  The loop's own coverage belongs against the real wait loop, which is where batch 1 puts it.
- **Applies to:** batches 2, 4, 5

### Decision: batch order is chosen so the tree builds at every batch boundary

- **Decision:** the seam and producer changes land before the recipe is wired, the recipe is wired before the rows are removed, and the documentation sweep lands last.
  Batch 4 leaves the three standalone validate rows in the graph alongside live gates for exactly one batch.
- **Rationale:** Go builds the whole module, so a batch that changes an interface without updating its implementors leaves no green boundary for mill-go to verify at.
  Ordering it this way costs one batch of deliberate redundancy — a gated row's artifact is validated twice, identically, and passes both times — and buys a verifiable green tree after every batch.
  Within batch 5 that guarantee is explicitly per-batch rather than per-card, because a coordinated removal across four packages has no card-sized green intermediate.
- **Applies to:** all batches

### Decision: verify commands are package-scoped, and the module-wide check is a vet pass

- **Decision:** each batch's `verify:` names the packages that batch edits.
  The overview-level module-wide `verify: go vet ./...` runs at every batch boundary after the batch's own command passes.
- **Rationale:** this is the documented justification for the module-wide command's unbounded package pattern, which is deliberate rather than an unscoped default.
  The task widens two shared interfaces, and Go admits no partial implementation, so the failure mode this task most plausibly introduces is a test fake in a package no batch names failing to compile.
  A scoped command cannot see that by construction, and `go vet ./...` type-checks every package *including its tests* — which is exactly the surface at risk — while running no test and costing a fraction of a full suite.
  It is a compile gate, not a test run: the per-batch commands own the assertions.
  The hub's existing repo-wide done gate is left unchanged and already covers the full untagged and integration suites before the task is marked done.
- **Applies to:** all batches

### Decision: the parity invariant binds the function, not the whole disposition

- **Decision:** the rewritten Gate Self-Check Parity Invariant binds a gate's closure and its CLI verb to the same package *function*.
  One parity fixture — an absent plan directory — deliberately reaches different verdicts on the two sides and is marked as an expected divergence rather than made to pass.
- **Rationale:** both sides still call `planglyph.ValidateFormat`, which is what the invariant governs and what keeps the operator's self-check and the automated gate from disagreeing about what "valid" means.
  The divergence lives strictly in the `ParsePlan` pre-step, whose disposition the gate reverses on purpose because its bounce target is a live session rather than a cold respawn.
  Hiding it — by dropping the fixture or weakening the comparison to binary — would retire real coverage to conceal a decision that was made deliberately and is recorded in the plan.
- **Applies to:** batches 5, 6

### Decision: the stale-mention sweep is a method, not a list

- **Decision:** the removed rows' names are swept by running the repo-wide grep and triaging every hit, not by working through an enumeration written into this plan.
  Batch 5 ends with a verification-only card confirming no live identifier survives, and batch 6 ends with one confirming the grep is empty.
- **Rationale:** the five strings hit fifty-three files at planning time, and any list written here would drift before it was implemented.
  Every such mention is part of this task rather than a follow-up, because a stale pointer to a row that no longer exists is worse than no pointer — and three of the hits are deployed normative stencils that actively tell a judge or a fixer its mechanical checks are enforced by rows that will not exist, when those checks now run on that very round's own output.
- **Applies to:** batches 5, 6

## All Files Touched

_Full union of every `Creates:` / `Edits:` / `Moves:` **target** path across every batch, sorted alphabetically (Move **source** paths are excluded — they disappear, like `Deletes:` tokens).
Cards are the source of truth;
this section is the input `_plan_validate.py`'s `all-files-touched-mismatch` check cross-references against the derived union of every card's `Edits:`/`Creates:`/Move-target paths, to catch drift between the hand/agent-maintained list here and that derived union._

- `CONSTRAINTS.md`
- `README.md`
- `contracts/recipes/loom-recipe.yaml`
- `contracts/specs/loom-plan-spec.md`
- `contracts/specs/specs.go`
- `contracts/stencils/loom/loom-rubric-discussion-review.md`
- `contracts/stencils/loom/loom-rubric-plan-review.md`
- `contracts/stencils/loom/loom-rubric-webster-review.md`
- `contracts/stencils/rubric_test.go`
- `docs/overview.md`
- `internal/burlerengine/engine.go`
- `internal/burlerengine/engine_test.go`
- `internal/burlerengine/profile.go`
- `internal/discussionparser/validate.go`
- `internal/friction/friction.go`
- `internal/loomcli/cli.go`
- `internal/loomcli/parity_test.go`
- `internal/loomcli/smoke_gate_test.go`
- `internal/loomcli/smoke_test.go`
- `internal/loomcli/start.go`
- `internal/loomcli/status_test.go`
- `internal/loomcli/validate.go`
- `internal/loomcli/validate_test.go`
- `internal/loomcli/wiring.go`
- `internal/loomrecipe/approveseam_test.go`
- `internal/loomrecipe/coverage_guard_test.go`
- `internal/loomrecipe/fixture_test.go`
- `internal/loomrecipe/gatequiescence_test.go`
- `internal/loomrecipe/recipe_test.go`
- `internal/loomrecipe/resume_test.go`
- `internal/loomrecipe/sequence_test.go`
- `internal/loomrecipe/shape_test.go`
- `internal/loomshed/cancellation_test.go`
- `internal/loomshed/discussionwrite.go`
- `internal/loomshed/discussionwrite_test.go`
- `internal/loomshed/fixture_test.go`
- `internal/loomshed/gatefindings_test.go`
- `internal/loomshed/gates.go`
- `internal/loomshed/gates_test.go`
- `internal/loomshed/interruptpolicy.go`
- `internal/loomshed/interruptpolicy_test.go`
- `internal/loomshed/loomshed.go`
- `internal/loomshed/planwrite.go`
- `internal/loomshed/planwrite_test.go`
- `internal/loomshed/seam_enforcement_test.go`
- `internal/planglyph/planglyph.go`
- `internal/planparser/validate.go`
- `internal/shedadapters/burler.go`
- `internal/shedadapters/burler_test.go`
- `internal/shedadapters/singlellm.go`
- `internal/shedadapters/singlellm_test.go`
- `internal/shedbuild/fixture_test.go`
- `internal/shedengine/run_routing_test.go`
- `internal/shedrecipe/entries_bouncer_test.go`
- `internal/shedrecipe/entries_burler.go`
- `internal/shedrecipe/entries_burler_test.go`
- `internal/shedrecipe/entries_discussionwrite.go`
- `internal/shedrecipe/entries_discussionwrite_test.go`
- `internal/shedrecipe/entries_gate.go`
- `internal/shedrecipe/entries_planwrite.go`
- `internal/shedrecipe/entries_planwrite_test.go`
- `internal/shedrecipe/entries_simple.go`
- `internal/shedrecipe/entries_simple_test.go`
- `internal/shedrecipe/fixture_test.go`
- `internal/shedrecipe/registry.go`
- `internal/shedrecipe/registry_test.go`
- `internal/shuttleengine/attach.go`
- `internal/shuttleengine/completionsignal_enforcement_test.go`
- `internal/shuttleengine/config_test.go`
- `internal/shuttleengine/gate.go`
- `internal/shuttleengine/gate_test.go`
- `internal/shuttleengine/run.go`
- `internal/shuttleengine/wait.go`
- `internal/websterengine/runlevel_test.go`
- `manifest/designs/loom.md`
- `manifest/designs/producer-gates.md`
- `manifest/designs/shed-recipe.md`
- `manifest/designs/shed.md`
- `manifest/roadmap.md`
