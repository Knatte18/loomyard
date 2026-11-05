## Constraints — do this before you judge anything

- These are the repo's rules; read them in full before forming any judgment:

# PATTERN

## Paths and geometry

- `PATTERN-cwd-resolution` — Resolving a cwd, worktree root or anchor: only `internal/lymcwd`, from the worktree's own weft-root `.lym-anchor`; only `fabricengine` writes or git-reads it; engines get absolute paths, never import lymcwd; hub containers and workspace files stay hub-reachable only. (test) — [background](pattern/PATTERN-cwd-resolution.md)
- `PATTERN-lym-temp-root` — Creating temp file or dir: under `internal/lymtemp`'s run dir, never `os.TempDir()`. (test) — [background](pattern/PATTERN-lym-temp-root.md)
- `PATTERN-durable-vs-ephemeral-state` — Adding file: tracked under `_lym`, never-tracked under `.lym`, same subpath; only `internal/lymdirs` names either, bar a `tools/` script's variable block. — [background](pattern/PATTERN-durable-vs-ephemeral-state.md)
- `PATTERN-leaf-packages` — Importing into a leaf package: only closed import set. — [background](pattern/PATTERN-leaf-packages.md)

## CLI and config

- `PATTERN-cli-cobra` — Adding CLI module: a cobra subtree under one root in `cmd/lym/main.go`. (test) — [background](pattern/PATTERN-cli-cobra.md)
- `PATTERN-cliwire-sole-wiring` — Wiring standalone CLI: `internal/cliwire` alone resolves target dir, repo root and state. (test) — [background](pattern/PATTERN-cliwire-sole-wiring.md)
- `PATTERN-config-layers` — Loading config: only `configengine` reads, stacks, locates and judges layers against the template; a caller adopts one of strict `Load` or `LoadOrTemplate`, bar `runprofile`; after a binary change, only `hubreconcile.Ensure` reconciles. — [background](pattern/PATTERN-config-layers.md)
- `PATTERN-sparse-hub-base` — Writing a hub base file, from any package: only the units the hub tuned, never a template fill; only `internal/hubreconcile`'s one-time strip drops a template-equal unit.
- `PATTERN-refusal-way-forward` — Adding webster, shed or loom refusal: message names the way forward and a test reaches it; no refusal, stencil or spec names a step agents' settings deny. (test) — [background](pattern/PATTERN-refusal-way-forward.md)

## Fabric and git

- `PATTERN-fabric-vocabulary` — Naming the wired composite: "fabric". (test) — [background](pattern/PATTERN-fabric-vocabulary.md)
- `PATTERN-fabric-git` — Running git on warp or weft: only `internal/fabricengine`, in-process; destroying only in its `destroy.go`, hub writes via a hub `os.Root`, store gc only in `housekeeping.go`. — [background](pattern/PATTERN-fabric-git.md)
- `PATTERN-gitrepo-client-boundary` — Running git: `gitexec.Run`/`runChecked`, raw only at pinned `//gitexec:raw` sites; in `internal/gitrepo`, local reads via `readGoGit` and `dotgit`; transients via `.git/info/exclude`, never `git add -f`. (test) — [background](pattern/PATTERN-gitrepo-client-boundary.md)
- `PATTERN-push-both-sides` — Pushing run records from Go: code branch goes along; a push refused after retry leaves a failed-push record and creel notice. (test) — [background](pattern/PATTERN-push-both-sides.md)
- `PATTERN-pair-teardown` — Tearing down a pair: only via `internal/pairteardown`; a producer creating or destroying a task worktree never runs inside it. (test) — [background](pattern/PATTERN-pair-teardown.md)
- `PATTERN-parent-lock` — Moving, merging into or landing on a local parent branch: only in a run's Finalize, under `internal/parentlock`'s lock for that parent.
- `PATTERN-owner-machine` — Starting, resuming or landing a run: only where its entry is not owned elsewhere; owners only on root and beam projects, set only by `boardengine`'s owner setter; any other entry reads its `hubgeom.ParentOf` owner; a dobby continue pass is no resume. — [background](pattern/PATTERN-owner-machine.md)
- `PATTERN-pair-parent` — Picking a new pair's parent for a board entry or reading an entry's owner through its parent: only `ParentBeam` and hubgeom's one `ParentOf`, never the acting worktree.
- `PATTERN-github-auth` — Calling GitHub: auth only via `internal/githubclient`, no other production package runs `gh`, and only `lym selfreport create` files an issue. (test)

## Shed and loom

- `PATTERN-shed-recipe-registry` — Adding producer or recipe row: one `Constructor` map in `internal/shedrecipe`, bar `Implementer`'s; a row's value looked up by key, a side action a `Command` row, a config-off row its `entry_gate`. — [background](pattern/PATTERN-shed-recipe-registry.md)
- `PATTERN-shed-driver` — Driving a shed run: generic verb bodies only in `internal/shedverbs`, the step-until-stop loop only in `internal/sheddriver`, run paths, ids and `Seed` only in `internal/shedrun`. (test) — [background](pattern/PATTERN-shed-driver.md)
- `PATTERN-shed-stop-chain` — A shed run's stop: rule table, tackler, arbiter, then parent, for every recipe; a blocked stop the driver's arbiter seam marks as judged first, by its row and reason, goes rule table, arbiter, then parent; arbiter keys in shed.yaml with module overrides.
- `PATTERN-dobby-starts-runs` — Starting or driving a board task's run: its parent's dobby, from an empty status, in batten passes; a hand `lym batten run` is one pass.
- `PATTERN-transient-stop` — Marking a failure transient: only `internal/shedengine`'s mark, translated by `internal/shedtransient` at a producer or step-bootstrap boundary; never on a gate verdict.
- `PATTERN-friction-capture` — Halting or crash-resuming a loom run, or refusing in webster: Go writes a note via `internal/friction`; none is archived before a reflection covers it. (test)
- `PATTERN-tolerant-records` — Go-written JSON records: never refuse an unknown key; rewrite via `internal/state`. (test) — [background](pattern/PATTERN-tolerant-records.md)
- `PATTERN-gate-slot` — Running gate build, test or verify: in held `internal/gateslot` slots, verify via `verifytree.Verify`, a tier skipped only on `lym gate test`'s green record; a gate and its self-check call one function. (test) — [background](pattern/PATTERN-gate-slot.md)
- `PATTERN-sole-parsers` — Parsing plan, discussion, summary, recipe or halt note, or classifying a plan ref: only its sole parser. (test) — [background](pattern/PATTERN-sole-parsers.md)

## Agents and prompts

- `PATTERN-stencil-ownership` — Reading prompt or spec: at call time via `internal/stencilstore`, never embedded; an instruction file points at another producer's format contract, never restating it. — [background](pattern/PATTERN-stencil-ownership.md)
- `PATTERN-parent-directive` — Writing a spawned role's stencil: it renders the parent and edit directives; its skills and color go on the launch spec, never asked of the agent. (test) — [background](pattern/PATTERN-parent-directive.md)
- `PATTERN-shuttle-provider-seam` — Touching a provider or shuttle run: specifics only under `internal/shuttleengine/<engine>`, model-specs via `NewModelChoice`, a strand stopped via the stop verb, a negative finish after `allOutputFilesExist`. (test) — [background](pattern/PATTERN-shuttle-provider-seam.md)
- `PATTERN-pane-text-injection` — Typing into an agent pane: reed's guarded paste-submit, an interrupt key only via shuttle's guards, a covered session held; the orch pane and creel journal only via creel. (test) — [background](pattern/PATTERN-pane-text-injection.md)
- `PATTERN-agent-name` — Forming, parsing or matching an agent name: only `internal/agentname`, a strand's full name formed once by reed; a spawned role is its module's constant.
- `PATTERN-wait-mark-display-only` — Marking a pane waiting: only reed's `SetWaitMark`; no Go decision reads `@lym_wait` or `@lym_wait_start`.
- `PATTERN-shell-guard-table` — Guarding a shell command or building a pane-shell string: only `internal/shellguard`'s table, every engine rendering every row, and `internal/shell`, importing only stdlib.

## Processes and waits

- `PATTERN-spawn-observability` — Starting OS process from `lym`: log the spawn via `internal/logger`; logging in a loop or poll follows its Level policy. — [background](pattern/PATTERN-spawn-observability.md)
- `PATTERN-run-process-owner` — A run's processes: registered in `internal/runproc`, ended only via `runproc`'s `End`, `EndSparing` or `EndRole`. — [background](pattern/PATTERN-run-process-owner.md)
- `PATTERN-daemon-wakeups` — Unbounded wait wakes at most once a second and backs off while idle. (test) — [background](pattern/PATTERN-daemon-wakeups.md)

## Testing

- `PATTERN-test-speed` — Writing or running a test: via `lym gate test` by gate key; one `tierN` tag, the lowest that fits; it pins behavior no test covers; injected clocks, `t.Parallel` by default. (test) — [background](pattern/PATTERN-test-speed.md)
- `PATTERN-test-isolation` — Testing package: hermetic git and `tmuxkit.Main`; shared support in one `internal/testkit/<kit>/`; hub fixtures via `hubforge`. (test) — [background](pattern/PATTERN-test-isolation.md)


- Read every background file under pattern/ that the entries above link to and that touches what you are about to judge.
- Every violation of a listed constraint, an entry the declared changes list judged by its declared text, is a BLOCKING finding: record it no matter how small it looks, and never wave it through because the code works or the tests pass.
- If a constraint conflicts with anything else in this prompt, the constraint wins — say so in your report instead of silently picking one.
## Focus directive for this round

Read the file at `/home/knatte/Code/loomyard-LYMHUB/codex-code-review-bench/.scratch/bench/inputs/STB/work/.bench/instr/focus.md` before forming any finding.
It names where to look and which question to settle this round.

How the directive relates to the rubric:

1. The rubric binds over the focus directive.
   A directive never licenses a finding the rubric's `Do not flag` list or symmetry rule forbids, and never suppresses a finding the rubric requires.
2. A directive steers attention and order, not verdicts.
   "The concrete instance to judge" means reach it and state a verdict on it, and "not a defect" is a valid verdict.
3. Severity comes from the rubric's mapping and the evidence you find.
   A severity cap in a directive is advisory and yields to evidence the round turns up.
4. `exclude_lenses` keeps its mechanical meaning: it trims the cluster fan, and this rule does not govern it.

The directive never licenses reading a prior round's files before your review is saved.
A directive that points into one is followed only as far as its own text goes, and you record the gap under `## Focus departures`.

If you depart from a directive, record it in the review file's `## Focus departures` section, as the review-file format describes.
## What to review (the target)

The subject under review is the committed diff. The rubric supplied in this prompt is
the single definition of the review range: read its "Determining the review range"
section and derive the range exactly as written there.

## What to judge it against (the fasit)

- `/home/knatte/Code/loomyard-LYMHUB/codex-code-review-bench/.scratch/bench/inputs/STB/work/_lym/discussion/board-entry.md`
- `/home/knatte/Code/loomyard-LYMHUB/codex-code-review-bench/.scratch/bench/inputs/STB/work/_lym/discussion/decision-record.md`

The board entry snapshot named above is the task's scope, as amended by the
`Added after Discussion` entries of the decision record named above, which win where
they differ. The record's other sections are the writer's own claim, checked against
the snapshot and never the answer key. The diff's job is to make the change the
amended entry asks for.

The fasit is the source of truth the target is judged AGAINST.
A review that ignores the fasit degenerates into a pure internal-consistency check — always read it and hold the target to it.

## Rubric

# Code-Review rubric

The subject under review is the **committed diff**, not a file and not a directory.
Ordinary diff review is the base: read the diff as code, with no checklist supplied, and judge it the way a careful reviewer judges any change.
The dimensions under `## Also flag` are added on top of that base, never a replacement for it.

The measuring stick is the answer key this prompt names: the files it lists as the artifacts or the answer key, read as the task's settled scope.
The answer key takes one of two forms, and the sections below that apply to only one of them say so:

- **A plan.**
  The decision record, `_lym/discussion/decision-record.md`, settles what the task's change must do, and the plan, `_lym/plan/00-overview.md` with the card files its Card Index names, is how the cards carry it out.
  The Card model the plan implements, and its format contract, are in `/home/knatte/Code/loomyard-LYMHUB/codex-code-review-bench/.scratch/bench/inputs/STB/work/.bench/specs/loom/loom-plan-spec.md`; this rubric points at it and restates neither.
- **A board entry snapshot.**
  `_lym/discussion/board-entry.md` is the task's board entry, written by lym, and the decision record's `Added after Discussion` entries amend it, an entry by the parent, the operator or the arbiter winning over the snapshot where they differ.
  The record's other sections are the writer's own claim, made before coding and reviewed by no one: check them against the snapshot, never use them as the measuring stick.

The change description at `_lym/landing/summary.md`, when present, is the writer's own account of the diff: read it as a claim to check against the diff, never as the measuring stick.

`Code-Review` is the LLM producer, not the mechanical one — over-flagging is a judgment failure mode a mechanical producer, which has only checks and never judgment, cannot exhibit.
This perch sits downstream of mechanical gates, among them the verify gate that ran on this tree, so a finding re-derived from one of their subjects is duplicated work rather than coverage.

## Determining the review range

This section is the single definition of the review range;
nothing else states it.

Review `git diff 161a4a7f..HEAD` — every commit the current branch introduces over the merge base the run's last merged parent tip leaves.
Nothing else widens the range.

The step is read-only.
If the range above reads `git diff none..HEAD`, its base could not be read: raise a BLOCKING finding stating that the review range could not be determined, and review nothing.
Silently reviewing a guessed range is a worse failure than an honest block.

## Do not flag

Do not flag any of the following as a finding:

- **Anything a mechanical gate already decides.**
  A failing build or test is the verify gate's to report.
  With a plan, the plan's *format* is enforced deterministically by `Plan-Write`'s and `Plan-Burler-Fix`'s own gates and is not this perch's subject.
- **Findings raised against the answer key itself.**
  The answer key is the measuring stick and never the subject.
  A finding about what the plan, the decision record or the board entry should have said cannot be satisfied by changing the diff, which is the only thing this segment can fix.
- **A missing `ImpactSummary` on any card, or an incomplete `DependsOn`/`Produces` list.**
  Both belong to `Plan-Review`, which has already passed.
- **Anything that is not the diff.**
  The discussion files and the plan directory under `_lym`, and this segment's own round artifacts under `_lym/reviews/code/`, are never the subject of a finding.

## Also flag

- **Code in general.**
  Review the code for correctness beyond compliance with the answer key: logic errors, unhandled failure paths, broken invariants and behavior that contradicts the answer key, each judged against the code as a careful reviewer reads it.
- **Scope against the answer key.**
  A change the answer key asks for that the diff does not make, and a change the diff makes that the answer key does not license, are each a finding.
- **The repository's own rules.**
  The diff follows the target repository's constraints document if it has one, and the docs it names for the kind of change the diff makes land in the same diff.
  A rule written in that document outranks a convention inferred from the surrounding code.
- **Comment-convention compliance.**
  Any new or changed doc comment follows the target repository's own conventions — its own constraints document if it has one, and the conventions the surrounding code already follows.
  A line width is never inferred from the surrounding code.
  A comment's line breaks and wrapping are never a finding; the gate's `lym loom lint-comments` alone checks them.
  This rubric checks compliance with the target repository's own standard, not loomyard's.
- **Per-card mechanical check, with a plan.**
  Confirm every one of the card's own groups' type-specific mechanical checks actually ran and passed, each against that group's own targets, not just the first label's, per the per-type table in `/home/knatte/Code/loomyard-LYMHUB/codex-code-review-bench/.scratch/bench/inputs/STB/work/.bench/specs/loom/loom-plan-spec.md` — not merely that the diff compiles and its tests pass.
  For a `Rename` group, check the outcome the diff shows: no reference to the old name is left and the symbol's doc-comment opening is updated; never check that a tool ran.
  For a `Delete` group, check `assert-no-callers`.
- **Commit messages and departures.**
  Read the commit messages of the review range (`git log` over the same range) as well as the diff.
  A difference from the answer key that a `Pattern-Departure:` trailer explains is judged on that reason, and holds when the named entry forbids the answer key's way, read in its declared new text where the directive's declared changes list it.
  A trailer naming an unknown entry, or an entry that does not forbid that way, is a finding.
  A trailer excuses only the difference it names.
- **Declared PATTERN changes landed.**
  Check that the branch's `PATTERN.md` and `pattern/` diff carries each entry the constraints section's declared changes list, its add, change or remove with the declared text, and raise a BLOCKING finding for each declared entry the diff does not land.
  Read the declarations rendered into your directive, never the plan's or the record's section, so a bullet appended after the gate is checked too.
  Raise nothing when every declared entry lands, or when your directive declares none.
- **The change description.**
  A claim in `_lym/landing/summary.md` that the diff does not bear out is a finding.
- **An unrecorded parent correction, with a board entry snapshot.**
  A deviation from the amended board entry that the change description names as an unrecorded parent correction is never fixed by revert.
  File it as class `decision` at severity MEDIUM.
  Its fix text tells the fixer to leave the code in place and defer the finding to the run's parent, since its only sound change, recording or refusing the correction with `lym loom decision add`, is the parent's and lies outside the fixer's write surface.
- **A step that edits the task's own board entry, with a board entry snapshot.**
  A step that changes this task's own board entry's body or scope is a finding, because the run's board lock refuses such a change.
  A step writing another entry, the entry's status or its `run_settings`, or changing board-module code is not flagged by this bullet.
  Such a step goes through `lym board` into the records repository, so the committed diff never shows it: the evidence is a claim in the change description that the run changed this task's own entry's body or scope, read as the writer's claim like the rest of that description.
  A decision-record step asking for such a change is evidence only of the request.
  A step left out through the writer's own-entry route is no scope finding once the record carries its `Added after Discussion` entry;
  before then the omission is an unrecorded parent correction, filed under that bullet as a MEDIUM `decision` finding deferred to the parent.
- **Stencil wording.**
  A diff that changes a stencil is read against the measuring stick, the plan card's `Intent` with a plan and the amended board entry with a snapshot; a rule the answer key did not ask to change that the diff drops, weakens or contradicts is a finding.
- **Test economy.**
  Each of these is a finding, and each names what grounds it:
  - a new test that duplicates an existing test's coverage, naming the covering test;
  - a per-helper test the public surface's test already covers, naming that public-surface test;
  - a review-fix test added for a finding that was not a coverage gap, naming that finding;
  - a new assertion on a stencil's static prose, naming the assertion;
  - a behavior the diff adds with no coverage, naming the uncovered behavior; a stencil's static prose is exempt.

  A redundancy finding is raised only by naming the existing test that covers the behavior, so it cannot block on a vague suspicion.

## Amendment

none

When the note above is not `none`, this round reviews an amendment: review only the change it names, and report nothing about the settled remainder.

## Review input

When the amendment note names a review input, that file is a report written after this perch last settled: the merge fixer's report of the latest Parent-Align.
Read it as an input besides the diff: judge from it whether the alignment it reports, and the parent changes it names, keep the code consistent with the answer key.
A finding about the report itself is never raised; each finding is about the code.

## Finding class

Every finding carries one class: `design`, `scope`, `decision` or `consistency`.
Their generic meanings are in burler's review step; this section does not restate them.
In this segment, `design` means the implementation does not make the change the answer key asks for, breaks the repository's invariants, or is incorrect.
A recurring enumeration gap is filed once as a `design` finding about the method, not once per missing item.
Class never changes whether a finding is fixed.

The rubric tells you what counts as BLOCKING, MEDIUM, LOW, or NIT for THIS target.
It maps its own criteria onto that fixed four-value severity vocabulary;
it never introduces a new severity name, and neither do you.
Each finding also carries one of the four classes — `design`, `scope`, `decision`, `consistency` — and the rubric says what `design` means for this target.

## Tool-use rules — how you gather evidence

Read-only analysis in the explore and review steps: read files, run nothing.

## Reading for the fork reviewers

This round fans out: in step 2 you spawn fork reviewers, and each fork inherits your context as it stands at the spawn.
Your reading in this step is the forks' reading, so read here what the review will need, once, instead of leaving each fork to read it again.

Before you leave this step, read:

- every file the target names, at the definitions of the symbols it cites;
- for each symbol the target changes, guards or removes: its direct callers and callees, and every path that reaches it from another process, such as a child the code spawns or a command that calls the guarded verb in-process;
- for each state, setting or file a decision introduces or changes: the existing code paths that also write, reset or reapply it, such as other verbs, hooks, resize or apply steps, and restart paths, and the code that reads it;
- for each output, key, string, file shape or behaviour the target changes: every test, caller, doc and fixture that reads or asserts it;
- the board entry snapshot at `/home/knatte/Code/loomyard-LYMHUB/codex-code-review-bench/.scratch/bench/inputs/STB/work/_lym/discussion/board-entry.md`;
- the `pattern/` background file of every PATTERN entry the target names or touches;
- the failure, restart and resume paths of each change the target makes.

Read definitions and call sites, not whole files: a fork re-reads its inherited context on every turn.
Judge nothing while you read: write no finding, verdict or assessment, not even in a status line or a note to yourself.
A judgement in your context before the spawn colours every fork; your own review starts after the forks are spawned.

Close this step with a reading manifest: one line per file and symbol you read, grouped by the part of the target it serves.
The manifest names what you read and nothing about what you found.
Write it as text in the same turn and go straight on to step 2: the manifest never ends your turn.
