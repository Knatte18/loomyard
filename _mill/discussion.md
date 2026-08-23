# Discussion: loom: self-checkable mechanical gates

```yaml
task: 'loom: self-checkable mechanical gates'
slug: loom-self-checkable-mechanical-gates
status: discussing
parent: main
```

## Problem

loom's phase machine gates two of its written artifacts with mechanical producers: `Discussion-Validate` (row 4) checks `_lyx/discussion/`, and `Plan-Validate` (row 8) checks `_lyx/plan/`.
When either gate fails it returns `shedengine.Stuck`, which bounces the run back to the writer row (`Discussion-Write` / `Plan-Write`).
That bounce is not a continued conversation: `shuttleengine.Spec` carries no session-resume field, so the writer restarts as a brand-new agent session with no memory of the turn that just failed, and with no detail about *what* failed — the gate's `Stuck` carries an empty `OutputPointer`.

The mitigation, agreed in a 2026-08-23 discussion and recorded in `manifest/roadmap.md`, is prompt-level: instruct the writer producer to run the same mechanical check itself before handing off, so a well-behaved run clears the Shed-level gate on the first try and the gate stays purely a backstop.
That only works if the check is callable standalone, from a CLI verb the agent can invoke — and it only works *safely* if the verb and the gate share one implementation, so the agent's self-check and the gate can never disagree.

**Why now:** this task is sequenced ahead of `loom: Discussion-Write producer` and `loom: Plan-Write producer` in `manifest/roadmap.md`.
Those two tasks' prompts are what instruct the agent to call these verbs, so the verbs must exist before either can be written.

Today, `Plan-Validate` is already half-solved: `loomshed.planValidate.Call` is a thin wrap over `planparser.ParsePlan` / `planparser.Validate`, both living in `internal/planparser` — a plain package with no `shedengine` or CLI dependency of its own.
A CLI verb there is a direct call into existing code.
`Discussion-Validate` is not: its two checks live inline in `internal/loomshed/discussionvalidate.go`, with no package split between "the check" and "the producer wrapping it".

## Scope

**In:**

- Extract `Discussion-Validate`'s two checks out of `internal/loomshed` into a new leaf package, `internal/discussionparser`, mirroring the existing `internal/planparser` ↔ `loomshed.planValidate` split.
- Reshape the extracted check to return structured findings (`[]Finding`) rather than a bare pass/fail, so a caller can report *what* failed.
- Rewrite `loomshed.discussionValidate.Call` as a thin wrap over the new package, with its outward `Done`/`Stuck`/error contract unchanged.
- Add two zero-argument CLI verbs on the existing `loom` cobra subtree: `lyx loom validate-discussion` and `lyx loom validate-plan`.
  Each calls the exact same package function its `ShedProducer` row calls.
- Move the check-level tests into `internal/discussionparser`; keep a thinner `loomshed` test for the producer's outcome mapping and cancellation.
- Add a parity test per gate asserting the verb and the producer reach the same verdict over the same fixtures.
- Add a `leaf_enforcement_test.go` to `internal/discussionparser`.
- Extend `internal/loomcli/cli_test.go`'s registered-verb assertion to six verbs, and add the two verbs to `cmd/lyx/helptree_test.go`'s `loom` `wantSubs` list.
- Docs in the same commit: `manifest/designs/loom.md`, `docs/overview.md`, `CONSTRAINTS.md` (one new invariant), `manifest/roadmap.md` (item moves to Done).

**Out:**

- **Prompt content.**
  No stencil under `contracts/stencils/loom/` is edited.
  Instructing the writer agent to call these verbs is `loom: Discussion-Write producer` / `loom: Plan-Write producer`'s job, not this task's.
- **Producer outward contract.**
  Neither gate's `Outcome` / `OutputPointer` / error mapping changes.
  Findings are surfaced by the CLI verb only, never through the producer's pointer.
- **Path declaration.**
  `loomengine.DiscussionDir` / `DiscussionDecisionRecord` / `DiscussionSupportLog` stay where they are and keep their `*lyxcwd.Location` signatures.
  `internal/discussionparser` declares no on-disk location of its own.
- **The recipe.**
  `contracts/recipes/loom-recipe.yaml` is unchanged — no row is added, removed, or re-routed.
- **`internal/planparser`.**
  No extraction is needed there and none is done;
  `planparser` is already the shared implementation.
- **`Plan-Sweep`.**
  It stays a stub.
  The new package is shaped so a future `Plan-Sweep` can reuse its section parsing, but this task builds nothing for it.
- **Wiring changes.**
  `loomcli.wire` is not split, refactored, or given a lighter path.

## Decisions

### new-package-named-discussionparser

- Decision: the extracted checks land in `internal/discussionparser`.
- Rationale: it mirrors `internal/planparser`'s name one-to-one, which is the split the roadmap item explicitly names as the pattern to follow.
  The roadmap's `Plan-Sweep` item already anticipates a second consumer of the same section parsing ("reads `decision-record.md`'s Scope section — the same section-parsing `Discussion-Validate` already does"), so the package genuinely becomes the sole reader of the discussion format, not just a checker.
- Rejected: `internal/discussioncheck` (names the checking rather than the parsing, and reads oddly for the future `Plan-Sweep` consumer);
  `internal/discussionvalidate` (one character from `internal/loomshed/discussionvalidate.go`, so two "validate" names would sit side by side).

### discussionparser-declares-no-paths

- Decision: `internal/discussionparser` takes told absolute paths and declares no path tokens of its own.
  `loomengine.DiscussionDir` / `DiscussionDecisionRecord` / `DiscussionSupportLog` remain the sole declarers of where `_lyx/discussion/` is.
- Rationale: those three accessors take a `*lyxcwd.Location`, which a leaf package may not import (see the Cwd Resolution and Told-Geometry Invariants).
  Full `planparser` parity — where `PlanDirName` / `PlanDirRel` / `PlanDir` / `PlanOverview` live in the parser — would mean re-shaping three accessors this task has no other reason to touch, for no benefit to either caller.
  The gate and the verb both get their paths from the same `shedrecipe.Env` fields, so single-sourcing is preserved regardless.
- Rejected: moving the path tokens into `internal/discussionparser` for full parity.

### findings-not-bool

- Decision: the extracted entry point is `Validate(decisionRecordPath, supportLogPath string) ([]Finding, error)` — a non-empty slice means the gate fails, a returned error means an I/O fault, and the two are never both non-zero.
- Decision (ordering, and it is load-bearing): `Validate` **short-circuits across files in exactly today's order** and accumulates only within the heading check.
  Concretely, reproducing `discussionvalidate.go`'s current control flow step for step: `os.Stat` the support log first — a not-exist returns immediately with exactly one finding and no error, and any other stat error returns immediately as an error with a nil findings slice;
  then `os.ReadFile` the decision record — a not-exist returns immediately with exactly one finding, any other read error returns immediately as an error;
  only then does the heading check run, and only there do findings accumulate, one per missing required heading.
  An error therefore always wins over any finding that a *later* check would have produced, because the later check never runs.
- Rationale for the ordering clause: an accumulating `Validate` would change the producer's outward behaviour in a case that exists today.
  With the support log missing and the decision record unreadable for a reason that is not not-exist, `Call` currently returns `Stuck` (it never reaches the read), whereas an accumulating implementation would surface the read fault as a returned error — `Stuck` persists blocked and bounces to `Discussion-Write`, a returned error persists failed and aborts the run.
  Pinning the order is what makes `producer-contract-unchanged` a checkable claim rather than an aspiration.
- Rationale: this mirrors `planparser.Validate`'s `[]ValidationError` return, which the `Plan-Validate` verb will render.
  The self-checking agent needs to know *which* required heading is missing or *which* file is absent;
  a bare `bool` would leave the verb able to say only "failed", which reproduces the information-free bounce this task exists to avoid.
- Rejected: `(bool, error)`, preserving today's exact information content.

### producer-contract-unchanged

- Decision: `loomshed.discussionValidate.Call` keeps its current outward contract exactly — any non-empty findings slice maps to `shedengine.Stuck` with an empty `OutputPointer`, both checks passing maps to `Done` with the decision record's path as the pointer, and an I/O fault that is not a not-exist maps to a returned error, never to `Stuck`.
  Only the body changes, into a call to `discussionparser.Validate`.
- Rationale: the roadmap item is about making the check callable standalone, not about changing what the gate does, and no consumer has asked for findings on the pointer — `Discussion-Write` is a stub today, and the task that replaces it will read the findings from the CLI verb it calls itself, not from the pointer of a gate it bounced off.
  The existing `entryErr` / `cancelErr` cancellation discipline and the `nonDoneExit` helper stay as they are.
  What makes "unchanged" verifiable rather than asserted is the pinned short-circuit order under `findings-not-bool` above — without it, the accumulate-vs-short-circuit choice silently flips one case from `Stuck` to a returned error.
- Rejected: propagating findings through the producer's `OutputPointer`.

### required-sections-move-unexported

- Decision: `requiredDiscussionSections` moves into `internal/discussionparser`, stays unexported, and carries its existing pointer comment to `contracts/stencils/loom/loom-template-discussion.md`'s Step 5 verbatim.
  `internal/loomshed` keeps no copy.
- Rationale: it is the check's own data, and two copies is exactly the disagreement this task is eliminating.
  The existing comment points at the stencil rather than restating its content, which is the right shape and costs nothing to preserve across the move.
  Exporting the list would be export-on-speculation with no caller.
- Rejected: exporting it so docs or tests can enumerate it.

### two-flat-verbs-on-the-loom-subtree

- Decision: two zero-argument verbs on the existing `loom` cobra subtree — `lyx loom validate-discussion` and `lyx loom validate-plan`.
- Rationale: the subtree is already flat (`run` / `drive` / `status` / `pause`), and the names track the `Discussion-Validate` / `Plan-Validate` row names one-to-one, which is what makes the mapping obvious to an agent reading a prompt that names the row.
- Rejected: a `lyx loom validate` group with `discussion` / `plan` children (an extra nesting level for two leaves);
  a new top-level subtree (these are loom-phase gates and nothing else would live there).

### verbs-reuse-the-existing-prerun-wiring

- Decision: both verbs hang off the existing `loomCLI` receiver and read `c.env.DecisionRecordPath` / `c.env.SupportLogPath` and `c.env.AnchorPath` / `c.env.WorktreeRoot`, populated by the unchanged `PersistentPreRunE` → `wire` path.
- Rationale: loom is hub-only, and the agent that calls these verbs runs inside a fully-wired worktree mid-loom-run, so the reed / shuttle / webster / modelspec configs `wire` loads are present by construction.
  A second, lighter paths-only wiring path would be two code paths over exactly the geometry this task is trying to single-source — the one place a drift could reintroduce disagreement.
- Rejected: splitting `wire` into a paths-only fast path so the verbs don't load configs they never read.

### envelope-and-exit-contract

- Decision: a clean gate emits `output.Ok` and exits 0.
  A gate with findings emits `output.ErrFields(out, "<summary>", map[string]any{"findings": ...})` and exits 1.
  An I/O fault (or, for the plan verb, a `planparser.ParsePlan` error) emits `output.Err` and exits 1, distinguished from a findings failure by message text only.
  Every `RunE` checks `clihelp.ShouldAbort` first, ahead of its own work.
- Rationale: the CLI / Cobra Invariant requires one JSON envelope per invocation via the `internal/output` envelope, and `output.ErrFields` already exists for exactly this "error plus structured payload" shape.
  A non-zero exit on a failed gate is what makes the verb usable as a shell-level self-check.
  One non-zero code is enough — the envelope's message already names which kind of failure it is.
- Rejected: emitting findings through `output.Ok` with exit 0 (an agent checking `$?` would read a failed gate as a pass);
  separate exit codes for findings vs I/O fault (redundant with the envelope).
- **Flagged for the plan stage:** this collapses two genuinely different failure modes — "the artifact needs more work" and "something is broken" — onto the same exit code, distinguished only by envelope content.
  That is deliberate, and it is only correct while the anticipated consumer is an LLM agent reading the JSON body.
  `internal/output` offers no natural third state (`Ok` returns 0, both `Err` and `ErrFields` return 1), so a distinct code would mean a new helper, not a flag.
  The plan stage must confirm the LLM agent is still the only anticipated consumer;
  if a shell script that branches on `$?` alone ever becomes a consumer, this decision has to be revisited before that consumer is written, not after.

### zero-argument-verbs

- Decision: neither verb takes a positional argument or a path-override flag.
  Every path is derived exactly as the producer rows derive it.
- Rationale: a path override is the one mechanism by which the self-check and the gate could be pointed at different files, which is the failure mode this task exists to make impossible.
  Tests reach the same code through `RunCLIIn(cwd, …)`, which seeds cwd without a per-verb override.
- Rejected: optional `--path` overrides for testing convenience.

### plan-verb-calls-planparser-directly

- Decision: `validate-plan`'s `RunE` calls `planparser.PlanDir(c.env.AnchorPath)`, then `planparser.ParsePlan`, then `planparser.Validate(plan, c.env.WorktreeRoot)` — the same three calls, in the same order, that `loomshed.planValidate.Call` makes.
  No extraction from `loomshed` and no new shared helper.
- Rationale: `internal/planparser` is already the one shared implementation, which is precisely why the roadmap says `Plan-Validate` "already qualifies".
  What differs between the two call sites is only the outcome mapping — `Stuck` vs. an envelope — which is each caller's own concern.
- Rejected: exporting a shared wrapper from `loomshed` for both to call (`loomcli` importing `loomshed` for a three-line wrap adds coupling without removing duplication of substance).

### parity-tests-per-gate

- Decision: each gate gets a test that drives both the producer's `Call` and the verb's CLI path over the same fixture set and asserts the pass/fail verdicts agree.
- Rationale: "one shared implementation, so the agent's self-check and the mechanical gate can never disagree" is the roadmap item's whole point, and nothing asserts it today.
  For `Discussion-Validate` the shared call site is `discussionparser.Validate`;
  for `Plan-Validate` it is `planparser.Validate`.
  A test is what keeps a later refactor from quietly forking one of them.
- Rejected: doc comments only, leaving the guarantee to review discipline.

### tests-follow-the-logic

- Decision: the check-level cases in `internal/loomshed/discussionvalidate_test.go` — heading presence and absence, missing decision record, missing support log, and the line-level edge cases `hasAllSections` encodes (trailing-whitespace tolerance, a heading inside a fenced block or mid-sentence not counting) — move into `internal/discussionparser`'s own test file.
  What stays in `loomshed` is the producer-level mapping: findings → `Stuck` with an empty pointer, clean → `Done` with the decision record as pointer, I/O fault → returned error, plus the existing cancellation cases.
- Rationale: tests belong beside the logic they test.
  Leaving them in `loomshed` would ship the new package untested and make `loomshed` assert another package's internals through a wrapper.
- Rejected: leaving all tests in `loomshed`.

### discussionparser-is-a-leaf

- Decision: `internal/discussionparser` imports the standard library and nothing else, enforced by a `leaf_enforcement_test.go` with a stdlib-only allowlist, in the shape the repo's other genuine leaves already use.
- Rationale: the package must be callable from `internal/loomshed` (whose own `seam_enforcement_test.go` allowlist it must be added to) and from a future `Plan-Sweep`, without dragging geometry resolution or engine types along.
  A stdlib-only leaf can never violate the Told-Geometry Invariant.
- Rejected: no enforcement test.

### new-constraints-invariant

- Decision: add one new section to `CONSTRAINTS.md` — a **Discussionparser Sole-Parser Invariant** mirroring the existing Planparser Sole-Parser Invariant, with four bullets: `internal/discussionparser` is the sole reader of `_lyx/discussion/`'s format;
  it declares no on-disk location of its own;
  it imports the standard library and nothing else, enforced by `internal/discussionparser/leaf_enforcement_test.go`'s allowlist;
  and a mechanical gate's `ShedProducer` row and its CLI self-check verb call the same package function, enforced by the parity tests named under `parity-tests-per-gate`.
  The import-cap bullet is not optional decoration — every one of the repo's existing `leaf_enforcement_test.go` files is backed by a `CONSTRAINTS.md` section naming its allowlist, so a machine check with no stated rule behind it would be the odd one out.
- Rationale: both rules are cross-cutting and review-enforced, and CLAUDE.md requires recording a new cross-cutting invariant in the same commit.
  Without it, the "can never disagree" rule lives only in a roadmap entry that gets archived when this item completes.
- Rejected: no new invariant.

### docs-in-the-same-commit

- Decision: `manifest/designs/loom.md` (rows 4 and 8 in the producer table, plus the "Validation checks" section, note the standalone verb), `docs/overview.md` (module table gains `internal/discussionparser`;
  the loom entry's verb list gains the two verbs), `CONSTRAINTS.md` (the new invariant above), and `manifest/roadmap.md` (the item moves to Done) all land in this task's commits.
- Rationale: CLAUDE.md's task-completion rule — a task adding a module, changing observable CLI behaviour, or introducing cross-cutting infrastructure updates docs in the same commit.
  This task does all three.
  `manifest/roadmap.md` moves because a planned item is completing, which is exactly the case the rule allows.
- Rejected: code only.

## Technical context

**The two producers as they stand.**

- `internal/loomshed/discussionvalidate.go` holds `discussionValidate` (fields `name`, `decisionRecordPath`, `supportLogPath`), its `NewDiscussionValidate(name, decisionRecordPath, supportLogPath) shedengine.ShedProducer` constructor, `requiredDiscussionSections` (seven `## ` headings: Goal, Scope, Decisions, Constraints, Auto-mode assumptions, Open risks, Acceptance criteria), the `nonDoneExit` helper, and the `hasAllSections(content string, required []string) bool` line scanner.
  `Call` stats the support log, reads the decision record, then runs `hasAllSections`.
  Its godoc records three deliberate non-checks that must survive the extraction verbatim: `## Notes for the plan writer` is optional and its absence is never a violation, section *order* is not validated, and an extra unexpected `## ` heading is not a violation.
- `internal/loomshed/planvalidate.go` holds `planValidate` (fields `name`, `anchorPath`, `worktreeRoot`) and `NewPlanValidate(name, anchorPath, worktreeRoot) shedengine.ShedProducer`.
  `Call` is `planparser.PlanDir(anchorPath)` → `planparser.ParsePlan(planDir)` → `planparser.Validate(plan, worktreeRoot)`.
  The two fields are separate because `PlanDir` takes the anchor path and `Validate` takes the worktree root, and they are not the same value.

**How the rows get their paths.**
`internal/shedrecipe/entries_simple.go` holds the two registry constructors: `DiscussionValidate` validates `Env.DecisionRecordPath` / `Env.SupportLogPath` via `requireAbsRoot` and calls `loomshed.NewDiscussionValidate`;
`PlanValidate` validates `Env.AnchorPath` / `Env.WorktreeRoot` the same way and calls `loomshed.NewPlanValidate`.
The recipe rows themselves are in `contracts/recipes/loom-recipe.yaml`: `Discussion-Validate` (engine `DiscussionValidate`, `on_stuck: Discussion-Write`, `on_done: Discussion-Review`) and `Plan-Validate` (engine `PlanValidate`, `on_stuck: Plan-Write`, `on_done: Plan-Review`).

**Where the CLI verbs get the same values.**
`internal/loomcli/wiring.go`'s `wire` assembles `c.env` (a `shedrecipe.Env`) with `AnchorPath: location.AnchorPath()`, `WorktreeRoot: location.WorktreePath()`, `DecisionRecordPath: loomengine.DiscussionDecisionRecord(location)`, and `SupportLogPath: loomengine.DiscussionSupportLog(location)`.
Both verbs read those four fields off `c.env` — the same values the registry constructors hand the producers, from the same accessors.

**CLI shape to follow.**
`internal/loomcli/cli.go`'s `Command()` builds the parent with `RunE: clihelp.GroupRunE` and `PersistentPreRunE: c.resolvePersistentPreRun`, then `parent.AddCommand(c.runCmd(), c.driveCmd(), c.statusCmd(), c.pauseCmd())` — the two new verbs are appended there, and the parent's `Long` example list gains two lines.
`internal/loomcli/status.go`'s `statusCmd` is the closest existing shape to copy: a `*cobra.Command` with `Use` / `Short` / `Long`, a `RunE` whose first statement is `if clihelp.ShouldAbort(cmd.Context()) { return nil }`, then `out := cmd.OutOrStdout()`, then `clihelp.SetExit(cmd.Context(), output.Err(out, …))` on failure.
The pre-run skips resolution only when `cmd.Name() == "loom"`, so both new verbs get a wired `c.env` for free.

**Output helpers.**
`internal/output` exposes `Ok(w, fields map[string]any) int` (injects `ok: true`, returns 0), `Err(w, msg) int` (returns 1), and `ErrFields(w, msg, fields map[string]any) int` (lays the caller's fields down first, then forces `ok: false` and `error`, returns 1).
`ErrFields` is what carries the findings payload.

**Findings rendering.**
`planparser.ValidationError` is a struct with an `Error() string` method (`internal/planparser/validate.go`).
Whatever `Finding` shape `internal/discussionparser` grows should be renderable into the envelope the same way the plan verb renders `[]planparser.ValidationError`, so the two verbs' payloads read alike;
the exact field set is mill-plan's call.

**Registration tests — only two files actually move, and only one of them is forced.**
Most of `cmd/lyx`'s tree tests derive their expectations from the live tree and need no edit at all: `drift_test.go` walks `newRoot()` collecting empty `Short`s, `longlist_test.go` iterates `root.Commands()` at module granularity, and `registration_test.go` is an AST scan for packages exposing `Command()` — none of the three has a per-verb list.
The one `cmd/lyx` file to touch is `helptree_test.go`, whose `TestHelpTree_VerbModuleSubcommands` pins `wantSubs: []string{"run", "drive", "status", "pause"}` for `loom`;
its assertions are documented superset checks, so it would not fail without the addition — adding the two verbs there is deliberate coverage, not a forced update.
The one genuinely forced update is in `internal/loomcli/cli_test.go`: `TestCommand_AllFourVerbsRegistered` (line 35) asserts the exact registered-verb set and will fail until it is extended to six (and renamed accordingly).
`cmd/lyx/seamsignature_test.go` is unaffected — no new seam module.

**Allowlist to update.**
`internal/loomshed/seam_enforcement_test.go` holds `loomshedAllowedImports`, currently seven entries (`shedengine`, `shedadapters`, `websterengine`, `loomengine`, `planparser`, `batcher`, `state`).
`internal/discussionparser` must be added, or the extraction fails that test.

## Constraints

From `CONSTRAINTS.md`:

- **CLI / Cobra Invariant.**
  `Short` non-empty on every command;
  self-discoverable commands also carry a `Long` with concrete examples.
  Errors are JSON via the `internal/output` envelope, one object per line — no bare plain-text error paths.
  One envelope per invocation: every `RunE` checks `clihelp.ShouldAbort` **first**, ahead of its own validation, because `clihelp.Abort` records an exit code but does not stop cobra from running `RunE`.
  Help accuracy is a review obligation: the parent `loom` command's `Long` example block must gain the two new verbs.
  cli imports engine;
  engine never imports cli or cobra.
- **Told-Geometry Invariant.**
  `internal/loomshed` takes told absolute paths and has no direct production import of `internal/lyxcwd`;
  `seam_enforcement_test.go` enforces the allowlist.
  `internal/discussionparser` inherits the same discipline as a stdlib-only leaf.
- **Cwd Resolution Invariant.**
  `internal/lyxcwd` alone resolves cwd.
  `root` always means the git worktree/repo root;
  `cwd` is the current working directory.
  Never name a parameter or field `root` for a value that is a cwd, or vice versa.
- **Planparser Sole-Parser Invariant.**
  `internal/planparser` is the sole parser of `_lyx/plan/` and the sole declarer of that directory's path.
  The `validate-plan` verb consumes it through its public API and parses nothing itself.
- **Documentation Lifecycle** (`docs/overview.md#documentation-lifecycle`) plus CLAUDE.md's task-completion rule: module doc, `docs/overview.md`, and `CONSTRAINTS.md` land in the same commit as the change.
- **Test Tier Purity Invariant.**
  Tier-1 tests resolve no cwd and spawn no process.
  `internal/discussionparser`'s tests are naturally tier 1 (told paths into a `t.TempDir()`).
  The CLI verb tests stay tier 1 only by avoiding `RunCLIIn` and driving the leaf `*cobra.Command` against a hand-populated `*loomCLI` receiver — see the `internal/loomcli` paragraph under `## Testing`, which explains why the `RunCLIIn` route is not available here.
  `internal/loomcli` already carries a `//go:build smoke` suite, so anything that genuinely needs a wired hub has a tagged home to go to.

From this repo's `CLAUDE.md`:

- Markdown uses semantic line breaks, never fixed-column hard-wrap — applies to every `.md` file touched here.
- `manifest/roadmap.md` moves only on completing or adding a planned item;
  this task completes one, so the move is in scope.

## Testing

**`internal/discussionparser` (TDD candidate — write the tests first).**
This is a pure, stdlib-only, told-paths leaf, which makes it the cleanest TDD target in the task.
Cases to carry over and extend, all against a `t.TempDir()`:

- all seven required headings present, both files exist → zero findings.
- each required heading missing individually → a finding naming that heading.
- several headings missing at once → a finding per missing heading, not a single aggregate.
- decision record absent → a finding, not a returned error.
- support log absent → a finding, not a returned error.
- a required heading present but with trailing whitespace → still counts as present.
- a required heading appearing inside a fenced code block or mid-sentence → does **not** count as present.
- `## Notes for the plan writer` absent → not a finding (documented non-check).
- headings present but out of stencil order → not a finding (documented non-check).
- an extra unexpected `## ` heading → not a finding (documented non-check).
- an unreadable path that is not a not-exist (e.g. a directory where a file is expected) → returned error with an empty findings slice.

**`internal/loomshed` (retained, narrowed).**
`discussionvalidate_test.go` keeps only the producer-level mapping and the existing cancellation cases: non-empty findings → `Stuck` with an empty `OutputPointer`;
zero findings → `Done` with the decision record path as the pointer;
a returned error from the package → a returned error from `Call`, never `Stuck`;
a cancelled context at entry and at each `nonDoneExit` site → a non-nil error, never `Stuck`.
`planvalidate_test.go` is unchanged.
`seam_enforcement_test.go` needs `internal/discussionparser` on its allowlist.

**`internal/loomcli` (new) — and the mechanism matters, because the obvious one does not work.**
These tests must NOT go through `RunCLIIn`.
The two existing `RunCLIIn` calls in `cli_test.go` (lines 76 and 90) are untagged only because they exercise the `cmd.Name() == "loom"` group-guard path, where `resolvePersistentPreRun` returns before resolving anything.
A real verb takes the other branch: `lyxcwd.Resolve` spawns `git rev-parse`, and `wire` then calls `loomengine.LoadConfig`, which is strict about an absent `_lyx/` or config file.
Driving a verb through `RunCLIIn` therefore needs a real wired hub and breaches the Test Tier Purity Invariant in an untagged suite.

The mechanism to use instead is the one `TestVerbRefusals` (`cli_test.go:106`) already establishes for `drive` and `pause`: build the leaf `*cobra.Command` from `(*loomCLI).validateDiscussionCmd` / `(*loomCLI).validatePlanCmd` against a **hand-populated `*loomCLI` receiver** — `c.env` filled with a `shedrecipe.Env` whose `DecisionRecordPath` / `SupportLogPath` / `AnchorPath` / `WorktreeRoot` point into a `t.TempDir()` — and run it via `clihelp.Execute(cmd, &out, nil)`.
No `PersistentPreRunE`, no `wire`, no git spawn, so these stay tier 1.
The cases, per verb:

- clean gate → exit 0, exactly one JSON line, `ok: true`.
- gate with findings → exit 1, exactly one JSON line, `ok: false`, with the findings payload present and naming the specific failure.
- I/O fault (discussion) and unparseable plan (plan) → exit 1, one line, `ok: false`, with a message distinguishable from the findings case.
- exactly one envelope per invocation in every case, `clihelp.ShouldAbort` checked first — the smoke-suite single-object unmarshal is what this protects.

A genuine end-to-end run through `RunCLIIn` against a `hubforge.NewHub` fixture would add value but needs a tagged suite (the package already carries a `//go:build smoke` file), and it is **not** required by this task: nothing in the verbs' behaviour depends on how `c.env` got populated, and `wiring_test.go`'s `TestWire_PathFieldsMatchLoomengineAccessors` already pins that population separately.
mill-plan may add one if it judges the fixture cost worthwhile;
it is explicitly optional.

**Parity tests (the task's own guard) — both halves stay tier 1.**
One per gate, driving the producer path and the CLI path over the same fixture set and asserting the verdicts agree.
Both halves take told paths, so neither needs a repository: the producer half constructs the producer via `NewDiscussionValidate` / `NewPlanValidate` with the fixture's paths and calls `Call`;
the CLI half uses the hand-populated-receiver mechanism above with the same paths.
Reuse one fixture builder for both directions rather than writing two fixture sets — two sets could drift and hide exactly the divergence the test exists to catch.
Where the test lives follows its imports: the discussion parity test can sit in `internal/loomcli` (which may import `loomshed`'s exported constructors), and the same for the plan one.

**Registered-verb assertions.**
`internal/loomcli/cli_test.go`'s `TestCommand_AllFourVerbsRegistered` pins the exact verb set and must be extended to six and renamed — this is the one update the compiler-adjacent suite forces.
In `cmd/lyx`, only `helptree_test.go`'s `loom` `wantSubs` entry gains the two verbs, as deliberate coverage;
`drift_test.go`, `longlist_test.go`, and `registration_test.go` derive from the live tree and need no edit.
See the Registration-tests paragraph under `## Technical context` for why.

**Leaf enforcement.**
`internal/discussionparser/leaf_enforcement_test.go`, in the shape the repo's other leaves already use: walk the package's non-test `.go` files, parse imports only, fail on anything outside stdlib.

## Q&A log

- **Q:** What should the extracted package be named? **A:** [auto-pick] `internal/discussionparser`. **Why:** mirrors `internal/planparser` one-to-one — the split the roadmap names as the pattern — and the roadmap's own `Plan-Sweep` item already anticipates a second consumer of the same section parsing, so "parser" is accurate rather than aspirational.
- **Q:** Does the new package also declare the `_lyx/discussion/` path tokens, as `planparser` declares `PlanDir`? **A:** [auto-pick] No — `loomengine` stays the declarer;
  the package takes told absolute paths only. **Why:** those accessors take a `*lyxcwd.Location`, which a leaf may not import;
  full parity would force a reshape of three accessors this task has no other reason to touch.
- **Q:** What API shape does the extracted check expose? **A:** [auto-pick] `Validate(decisionRecordPath, supportLogPath string) ([]Finding, error)`. **Why:** mirrors `planparser.Validate`'s `[]ValidationError`, and the self-checking agent needs to know which heading or file failed — a bare `bool` reproduces the information-free bounce this task exists to avoid.
- **Q:** Does the `discussionValidate` producer's outward contract change? **A:** [auto-pick] No — only its body changes. **Why:** the roadmap item is about callability, not about changing what the gate does, and no consumer asks for findings on the pointer — the task that replaces the `Discussion-Write` stub reads them from the CLI verb it calls itself.
- **Q:** (review r1 gap) Does `Validate` short-circuit across the two files or accumulate, and which wins when a finding on one file coexists with an I/O error on the other? **A:** Short-circuit, in exactly today's stat-then-read order;
  an error always wins over a later check's finding because the later check never runs. **Why:** an accumulating implementation flips one real case — support log missing plus an unreadable decision record — from `Stuck` to a returned error, i.e. from persist-blocked-and-bounce to persist-failed-and-abort, which would make `producer-contract-unchanged` false.
- **Q:** (review r1 gap) How do the CLI-verb tests stay tier 1, given that a real verb's pre-run spawns `git rev-parse` and strictly loads config? **A:** They avoid `RunCLIIn` entirely and drive the leaf `*cobra.Command` against a hand-populated `*loomCLI` receiver, the mechanism `TestVerbRefusals` already uses;
  both halves of each parity test do the same. **Why:** the existing `RunCLIIn` calls in this package are untagged only because they take the group-guard path that skips resolution — that precedent does not extend to a real verb, and a hub-fixture suite is optional extra coverage, not a prerequisite.
- **Q:** Where does `requiredDiscussionSections` live after the extraction? **A:** [auto-pick] In the new package, unexported, with its stencil pointer comment carried over verbatim. **Why:** it is the check's own data, and a second copy in `loomshed` is precisely the disagreement being eliminated.
- **Q:** How are the CLI verbs placed and named? **A:** [auto-pick] Two flat verbs on the existing subtree — `lyx loom validate-discussion` and `lyx loom validate-plan`. **Why:** the subtree is already flat, and the names track the `Discussion-Validate` / `Plan-Validate` row names one-to-one, which is what makes the mapping obvious to an agent reading a prompt that names the row.
- **Q:** Do the verbs reuse loom's existing heavy `PersistentPreRunE` wiring, or get a lighter paths-only path? **A:** [auto-pick] Reuse it unchanged. **Why:** loom is hub-only and the calling agent runs inside a fully-wired worktree mid-run, so every config `wire` loads is present by construction;
  a second wiring path would be two code paths over exactly the geometry this task is single-sourcing.
- **Q:** What are the output and exit-code contracts? **A:** [auto-pick] Clean → `output.Ok`, exit 0;
  findings → `output.ErrFields` with a `findings` payload, exit 1;
  I/O fault → `output.Err`, exit 1, distinguished by message text only. **Why:** a non-zero exit is what makes the verb usable as a shell-level self-check — findings on `output.Ok` would let an agent checking `$?` read a failed gate as a pass — and one non-zero code is enough because the envelope already names the failure kind.
- **Q:** Do the verbs take arguments or path-override flags? **A:** [auto-pick] Neither — both are zero-argument and derive every path exactly as the producer rows do. **Why:** a path override is the one mechanism by which the self-check and the gate could be pointed at different files.
- **Q:** How is `validate-plan` implemented? **A:** [auto-pick] It calls `planparser.PlanDir` → `ParsePlan` → `Validate` directly, the same three calls the producer makes. **Why:** `planparser` is already the shared implementation — which is why the roadmap says `Plan-Validate` "already qualifies";
  a shared wrapper exported from `loomshed` would add coupling without removing duplication of substance.
- **Q:** What guards the "verb and gate can never disagree" guarantee? **A:** [auto-pick] A parity test per gate, driving both paths over the same fixtures and asserting the verdicts agree. **Why:** it is the roadmap item's whole point and nothing asserts it today;
  a doc comment leaves it to review discipline across future refactors.
- **Q:** Do the existing check-level tests move? **A:** [auto-pick] Yes — check-level cases move to `internal/discussionparser`;
  `loomshed` keeps only outcome mapping and cancellation. **Why:** tests belong beside the logic they test, and leaving them behind would ship the new package untested.
- **Q:** Does the new package get a leaf-invariant enforcement test? **A:** [auto-pick] Yes, stdlib-only allowlist. **Why:** the package must stay callable from both `loomshed` and a future `Plan-Sweep` without dragging geometry resolution or engine types in.
- **Q:** Is a new `CONSTRAINTS.md` invariant recorded? **A:** [auto-pick] Yes — a Discussionparser Sole-Parser Invariant mirroring the Planparser one, plus the rule that a gate's row and its CLI self-check verb call the same package function. **Why:** both are cross-cutting and review-enforced, and without the entry the "can never disagree" rule survives only in a roadmap item that gets archived on completion.
- **Q:** Which docs land in the same commit? **A:** [auto-pick] `manifest/designs/loom.md`, `docs/overview.md`, `CONSTRAINTS.md`, and `manifest/roadmap.md` (item to Done). **Why:** CLAUDE.md's task-completion rule covers all three triggers here — a new module, changed observable CLI behaviour, and new cross-cutting infrastructure — and the roadmap moves because a planned item is completing.
