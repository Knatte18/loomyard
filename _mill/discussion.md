# Discussion: loom: Plan-Write producer

```yaml
task: 'loom: Plan-Write producer'
slug: loom-plan-write-producer
status: discussing
parent: main
```

## Problem

loom's thirteen-row producer list runs `Plan-Write` as a `loomshed.NewStub` — a no-op that reports `Done` with an empty pointer and writes nothing.
Every downstream row therefore runs against an empty `_lyx/plan/`: `Plan-Validate` cannot parse a plan that does not exist, and the whole loom pipeline is unusable end-to-end past `Discussion-Review`.

The Go half of the real producer already exists and is fully unit-tested: `internal/loomengine`'s `PlanSpec` (`internal/loomengine/plan.go`) composes a `shuttleengine.Spec` from the format-4 stencil `contracts/stencils/loom/loom-template-plan.md`, and `internal/loomengine/plan_test.go` pins its field mapping and prompt content.
Nothing in production calls it — `grep -rn PlanSpec --include=*.go` finds only tests.
This task closes that gap: wire `PlanSpec` into the recipe as a real `SingleLLMProducer`, exactly the way the Done **loom: Discussion-Write producer** item wired `DiscussionSpec` one row earlier, and finish the stencil for the Card format.

**Why now:** Wave 2 of the "loom: rewrite for the new Plan Card format" group is complete.
`internal/planparser` parses the `Edits`/`Uses` Card shape today, and both `contracts/specs/loom-plan-spec.md` and the plan stencil were already rewritten to format 4 by that migration.
A prompt targeting the new shape is only useful once the parser accepts it, and it now does.

## Scope

**In:**

- A new `PlanWrite` engine entry in `internal/shedrecipe`'s registry (`internal/shedrecipe/entries_planwrite.go`), taking the registry from thirteen entries to fourteen.
- Two new `shedrecipe.Env` seams — `PlanSpec shedadapters.SpecSource` and `CommitPlan func() error` — filled by `internal/loomcli`'s `wire()`.
- A new `loomshed.NewPlanWrite` decorator (`internal/loomshed/planwrite.go`): rotate the stale plan directory, delegate to the wrapped producer, commit on a `Done`-with-nil-error outcome.
- A pure name-token declarer for the archive subdirectory in `internal/planparser`, beside `PlanDirRel`.
- `contracts/recipes/loom-recipe.yaml`: the `Plan-Write` row's `engine:` flips from `Stub` to `PlanWrite`.
- `contracts/stencils/loom/loom-template-plan.md`: a Step 0 skill-loading section, a degraded-mode (no quarry inventory) section, a Verify-model paragraph, and a `lyx loom validate-plan` self-check step.
- Tests across `internal/loomshed`, `internal/shedrecipe`, `internal/loomcli`, `internal/loomrecipe`, and `internal/loomengine`.
- Docs: `manifest/designs/loom.md` and `manifest/roadmap.md` (move the item to Done), in the same commit.

**Out:**

- `Plan-Sweep`. It stays a stub and is not even a row in the producer list (see `manifest/roadmap.md`'s Someday **loom: build `Plan-Sweep` for real**). This task only makes `Plan-Write` correct in its absence.
- `Plan-Review`. It stays `engine: Stub`; its `Plan-Bouncer`/`Plan-Burler` perch is the separate Planned **loom: Plan-Review producer** item.
- `Discussion-Review` and `Webster-Review`. Untouched, still stubs.
- `webster: DAG-derived card sequencing`. Deriving execution order from `Edits`/`Uses` is a separate Wave 3 item.
- Any change to `internal/planparser`'s parsing or validation logic. The parser is already format-4-complete; this task adds one pure string helper to it and nothing else.
- Any change to `internal/shedadapters`. `SingleLLMProducer` and its unexported `archiveStaleOutputs` are used as-is.
- Interactive mode for `Plan-Write`. The Plan producer is autonomous by design (`PlanSpec` hard-codes `Interactive: false`); there is no mode selector to flip, unlike the Planned **loom: interactive Discussion-Write** item.
- Any new `on_stuck` routing. `Plan-Validate` and `Plan-Review` already bounce back to `Plan-Write`; `Plan-Write` itself keeps no `on_stuck`, so a genuinely stuck writer escalates to a human.

## Decisions

### plan-write-is-a-dedicated-registry-entry

- Decision: add a dedicated `"PlanWrite"` engine to `internal/shedrecipe`'s registry map, backed by a new `planWriteEntry` constructor in its own file `internal/shedrecipe/entries_planwrite.go`. It validates `Env.PlanSpec`, `Env.CommitPlan`, and `Env.Shuttle` via the existing `requireSeam` helper and `configRejectUnknown`, then returns `loomshed.NewPlanWrite(name, shedadapters.NewSingleLLMProducer(name, env.PlanSpec, env.Shuttle, env.Now), …)`.
- Rationale: this is the exact shape `discussionWriteEntry` (`internal/shedrecipe/entries_discussionwrite.go`) already ships, for reasons that transfer verbatim. The generic `"SingleLLM"` entry cannot be reused: building the Spec needs a `*lyxcwd.Location`, which the Shed Recipe Registry Invariant bars `internal/shedrecipe` from importing, and a generic row's own `model`/`effort` `Config` keys would bypass the `plan` role's model-spec resolution (`cfg.Plan`) and its `plan_timeout_min` timeout entirely. A separate file rather than `entries_simple.go` because that file's header comment describes only the plain single-constructor shape.
- Rejected: reusing `"SingleLLM"` (loses role resolution and cannot carry a `Location`); a dedicated entry with no decorator (leaves rotation and commit unsolved — see the next two decisions).

### stale-plan-dir-rotation-into-an-archive-subdirectory

- Decision: `loomshed.NewPlanWrite`'s decorator rotates the plan directory **before** delegating: it creates `_lyx/plan/archive-<UTC-compact-stamp>/` and moves every top-level `*.md` entry of `_lyx/plan/` into it, leaving subdirectories in place. An absent or empty plan directory is a no-op, never an error. Same-second collisions get a `-1`, `-2`, … suffix, mirroring `shedadapters.firstFreeArchivePath` and `websterengine.ArchiveStaleSummary`.
- Rationale: `Plan-Write` is a bounce target — `Plan-Validate`'s `on_stuck` and `Plan-Review`'s `on_stuck` both route back to it — so a re-entry always finds a populated `_lyx/plan/`. Two concrete failures follow if nothing rotates it. First, `planparser`'s `index-file-mismatch` check (`internal/planparser/validate.go`, the `os.ReadDir(plan.Dir)` block around line 104) reports every `*.md` file in the plan directory that the new Card Index does not reference; a previous run's leftover `07-old-card.md` is a guaranteed finding, so `Plan-Validate` returns `Stuck`, bounces to `Plan-Write`, and the pair ping-pongs until the bounce budget is exhausted. Second, `SingleLLMProducer`'s own `archiveStaleOutputs` (`internal/shedadapters/archive.go`) would rename `00-overview.md` to a **timestamped sibling inside the same directory** — `_lyx/plan/00-overview-20260824T170000Z.md` — which is itself a fresh `index-file-mismatch` finding. The default archive behaviour is therefore actively wrong for this producer, not merely insufficient.
  Moving files into a subdirectory rather than out to a sibling directory is what makes the fix cheap: `index-file-mismatch` skips `e.IsDir()` entries and `ParsePlan` reads only top-level `*.md`, so an archive subdirectory is invisible to both, and the commit pathspec stays the single, always-present `_lyx/plan`. Rotation runs before the wrapped `Call`, so by the time `archiveStaleOutputs` runs, `00-overview.md` is already gone and it is a no-op — the two mechanisms compose instead of fighting.
  Only `*.md` files move, never directories, so a second rotation cannot nest a previous `archive-*` directory inside a new one and archives never recurse.
- Rejected: renaming the whole directory to a sibling `_lyx/plan-<ts>/` (one atomic `os.Rename`, but it invents a second top-level `_lyx` directory name and forces the commit pathspec to become a two-element list whose second element may not exist); leaving rotation to stencil prose (an LLM instruction is not a mechanism, and this failure mode is a hard infinite-bounce, not a quality regression); relaxing `index-file-mismatch` to ignore archived files (weakens a validator check to work around a producer defect).

### planparser-declares-the-archive-name-loomshed-moves-the-files

- Decision: `internal/planparser` gains one pure, stdlib-only string helper beside `PlanDirRel` — given a timestamp string and a collision suffix, it returns the archive subdirectory's name (`archive-<stamp><suffix>`). It performs no filesystem work. `internal/loomshed`'s `planWrite` does the `os.MkdirAll` and the `os.Rename` calls.
- Rationale: the Planparser Sole-Parser Invariant makes `planparser` "the sole declarer of the plan directory's path", and a subdirectory of the plan directory is part of that path vocabulary — a name literal invented in `loomshed` would be a second declarer. A pure string function keeps `planparser` stdlib-only and adds no parsing, so the invariant's parser half is untouched. The mutation belongs in `loomshed` because that is where the producer lives and because a parser package performing renames would be a genuine charter violation.
- Rejected: `loomshed` declaring the literal (second declarer of a plan-directory path); `loomengine` declaring it (`planparser`, not `loomengine`, owns `_lyx/plan`); putting an `ArchiveStalePlan` mutating function in `planparser` (turns the parser into a filesystem mutator).

### commit-the-plan-on-done

- Decision: `planWrite` invokes an injected `commit func() error` after the wrapped producer reports `Done` with a nil error, exactly as `discussionWrite` does. `internal/loomcli`'s `wire()` fills `Env.CommitPlan` with a closure calling `fabricengine.CommitAnchoredPaths(fabricengine.NewMutations(""), location, []string{planparser.PlanDirRel()}, fmt.Sprintf("loom: plan artifacts for %s", seedSlug(location.WorktreeName)), fabricengine.EnvSyncOptions())`, discarding the returned sha and `committed` bool and returning only the error.
- Rationale: symmetry with `CommitDiscussion` (`internal/loomcli/wiring.go`), for the same three reasons stated there — it keeps the working tree clean for the rows that follow, makes the artifact durable across a crash or a resume, and sweeps the archive subdirectory into git rather than leaving it as untracked dirt. Committing before `Plan-Validate` has judged the plan is intentional and matches the discussion precedent: the commit keeps the artifact durable, it does not certify it. A re-commit over an already-clean tree is a no-op, not an error, because `CommitAnchoredPaths` reports `committed == false` for an already-tracked clean path.
- Rejected: no commit until `Publish` (leaves a dirty tree across every `Plan-Validate` bounce and loses the plan on a crash); committing inside `PlanSpec` (`PlanSpec` is a pure composer and must stay one).

### commit-failure-is-an-error-never-stuck

- Decision: a non-nil `commit` error maps to a returned error from `planWrite.Call`, never to `shedengine.Stuck`.
- Rationale: verbatim the reasoning `discussionWrite` records — a git fault is not something re-writing the plan can fix, and the two dispositions differ materially: `Stuck` persists blocked and bounces, a returned error persists failed and aborts the run.
- Rejected: mapping to `Stuck` (bounces the agent for a problem the agent cannot fix).

### rotation-failure-is-an-error-never-stuck

- Decision: a failure to create the archive directory or to move a file into it maps to a returned error from `planWrite.Call`, before the wrapped producer is ever invoked.
- Rationale: same class as the commit failure — a filesystem fault is infrastructure, not plan quality. Running the agent against a directory that could not be cleared would guarantee the `index-file-mismatch` bounce the rotation exists to prevent, so failing fast is strictly better than delegating.
- Rejected: logging and continuing (produces the exact failure the rotation prevents); mapping to `Stuck`.

### decorator-does-not-re-check-cancellation

- Decision: `planWrite` does not consult `entryErr`/`cancelErr` itself.
- Rationale: `discussionWrite` records the same decision — the wrapped `*SingleLLMProducer` entry-checks the context as its first act, so a second check at the same seam is duplicate work, and the wrapped producer owns the whole cancellation obligation. One deviation is worth stating explicitly: rotation runs **before** the inner `Call`, so a run cancelled between the two leaves an archive directory behind and no new plan. That is acceptable — the archive is committed content, not dirt, and the next `Plan-Write` entry rotates an already-empty directory as a no-op.
- Rejected: adding an entry check in the decorator (duplicate seam check, contradicts the sibling's recorded decision).

### recipe-row-flips-engine-only

- Decision: in `contracts/recipes/loom-recipe.yaml`, the `Plan-Write` row changes `engine: Stub` to `engine: PlanWrite`. `on_done: Plan-Validate` is unchanged, and no `on_stuck` is added.
- Rationale: the row's name, position, and routing are already correct and are durable on-disk identities — the row-name constants in `internal/loomshed/loomshed.go` are the authority and a rename breaks resume for any in-flight task. `Plan-Write` deliberately carries no `on_stuck`: it is the bounce *target* of two gates, and nothing in the list produces what a stuck writer would need, so escalation to a human is the correct terminal behaviour, consistent with the recipe file's own header comment.
- Rejected: adding `on_stuck` (there is no row to bounce to); renaming the row.

### stencil-step-0-loads-scribe-prose-and-testing

- Decision: prepend a `## Step 0 — Load the writing skills` section instructing the agent to load `scribe:prose` then `scribe:testing`, in that order, best-effort — a skill that fails to resolve is not an error.
- Rationale: `manifest/designs/plan-card-format.md`'s Docstring-convention section pins the wiring mechanism ("a 'Load these skills: ...' section composed into every code-writing producer's stencil — not left to model discretion to invoke"), and `plugins/scribe/skills/INDEX.md` documents the lyx-stencil half of that mechanism, already shipped in `loom-template-discussion.md`'s Step 0. `prose` is the always-active writing discipline for every piece of text an agent writes, and the plan is a text artifact. `testing` is included because the Card-granularity rule ("one card per independently reviewable/testable unit") and the bundled-test rule are testing judgments, not prose judgments. `conversation` is excluded — it is chat-reply discipline and no operator is present. `code-quality` and the three `golang-*` skills are excluded — `Plan-Write` writes no code; those belong in webster's implementer prompt.
- Rejected: loading `scribe:conversation` (autonomous session, no operator); loading `scribe:code-quality` (writes no code); loading nothing (contradicts the design doc's explicit wiring instruction).

### degraded-mode-is-static-prose-not-a-marker

- Decision: add a section to the stencil stating plainly that no quarry inventory exists, that the agent must therefore do the mechanical lookups itself — `go doc <pkg> <Symbol>` for symbol existence and definition, `grep -rn` scoped to the right package for call sites, manual read for blast radius — and that the absence of an inventory is the normal state, never an error and never a reason to stop. No new stencil marker is introduced.
- Rationale: the roadmap item's own constraint is that `Plan-Write` "must treat `Plan-Sweep`'s empty stub output as 'no quarry inventory available yet,' not as an error." `Plan-Sweep` is not a row in the thirteen-row producer list at all, so there is no artifact to read, no path to interpolate, and nothing for a marker to carry — a `{{.quarry_inventory}}` marker would render a constant string. `manifest/designs/plan-card-format.md`'s Quarry-integration section already states the exact degraded-mode substitutes; the stencil restates them at the point of use. When `Plan-Sweep` is eventually built for real, adding a marker is a one-line change in `composePlanPrompt` — YAGNI until then.
- Rejected: a `{{.quarry_inventory}}` marker filled with an empty-state sentence (a marker that can only ever hold one value); saying nothing (leaves the agent free to treat a missing inventory as a blocker, which is precisely the failure the roadmap item names).

### stencil-self-check-runs-validate-plan

- Decision: add a self-check step instructing the agent to run `lyx loom validate-plan` (zero arguments) before ending its turn, fix whatever the findings report, and re-run until it exits 0.
- Rationale: the Done **loom: self-checkable mechanical gates** item shipped `lyx loom validate-plan` for exactly this purpose and recorded that "instructing the writer agent to call these verbs belongs to the Done `loom: Discussion-Write producer` item and the Planned `loom: Plan-Write producer` item, which this item was sequenced ahead of precisely so the verbs would exist first." The Gate Self-Check Parity Invariant guarantees the verb and the `Plan-Validate` row call the same `planparser.Validate`, so a clean self-check is a genuine prediction of the gate's verdict, not an approximation. This converts a `Plan-Validate` bounce (a whole extra shuttle run) into an in-session fix.
- Rejected: omitting it (wastes a full producer round on defects the agent can see itself); teaching the stencil to re-implement the checks (violates the Gate Self-Check Parity Invariant).

### stencil-documents-the-three-tier-verify-model

- Decision: add two or three sentences to the stencil stating the *authoring rule only* — a per-card `**Verify:**` field is exceptional, written only for what a package-scoped automatic test run cannot catch, and the plan-level `## verify:` section is the single integration check for the whole plan. Point at `manifest/designs/plan-card-format.md`'s Verify-model section for the tier definitions rather than restating them.
- Rationale: `manifest/designs/plan-card-format.md` makes the optional-and-exceptional character of `Verify:` load-bearing — "what keeps `Verify:` from becoming the long, hand-maintained list it was in millhouse's own equivalent." The current stencil says only that every verify value must be runnable shell rather than prose; it never tells the agent when *not* to write one, so the default drifts toward writing one per card. The authoring rule is the producer's own instruction and belongs in the prompt; the tier taxonomy is format-contract content and must stay in the one file that owns it, per the Producer Pointer-Rule Invariant.
- Rejected: reproducing the three-tier definitions in the stencil (duplicates format-contract content — the exact thing the Producer Pointer-Rule Invariant forbids, and it would drift); leaving the current single-`verify:` framing (the design's exceptional-by-default rule never reaches the agent).

### env-gains-two-named-fields-not-a-keyed-map

- Decision: `shedrecipe.Env` gains `PlanSpec shedadapters.SpecSource` and `CommitPlan func() error` as two plainly named fields.
- Rationale: `Env` already carries per-producer named fields (`DiscussionSpec`, `CommitDiscussion`, `WebsterRun`, `WebsterDeps`), and `DiscussionSpec`'s own field doc records why: it arrives as a closure rather than as recipe `Config` because building the Spec needs a `*lyxcwd.Location` the Shed Recipe Registry Invariant bars the package from importing, "and it is named per-producer rather than carried in a generic keyed map because `Env` already carries per-producer named fields." Introducing a keyed map now, for the second instance, would fork the convention mid-stream.
- Rejected: a `map[string]SpecSource` keyed by row name (loses compile-time checking and contradicts the sibling field's recorded rationale); recipe `Config` keys (cannot carry a `Location`).

### registry-grows-to-fourteen-entries

- Decision: `internal/shedrecipe`'s registry map goes from thirteen to fourteen keys. `internal/shedrecipe/registry_test.go`'s `TestRegistry_ShipsThirteenEntries` is renamed and its expected name list extended. `internal/loomrecipe/coverage_guard_test.go`'s `loomRowEngines` maps `loomshed.NamePlanWrite` to `"PlanWrite"` instead of `"Stub"`, and `coverageGuardAllowedUnreachableEngines` keeps all three of `SingleLLM`, `Bouncer`, and `BurlerRound`.
- Rationale: the registry's own doc comment states "The table is complete at thirteen keys. Any fourteenth entry must arrive with a coverage-guard update in the same commit" — this is that fourteenth entry, and both guard updates land here. The allowed-unreachable list is unchanged because `Plan-Write` consumes the new `PlanWrite` engine rather than the generic `SingleLLM`, and the three remaining stubbed rows (`Discussion-Review`, `Plan-Review`, `Webster-Review`) still leave `SingleLLM`/`Bouncer`/`BurlerRound` unreferenced. `Stub` stays in the registry — three rows still use it.
- Rejected: dropping `SingleLLM` from the allowed list (it remains genuinely unreferenced).

### no-new-invariant

- Decision: `CONSTRAINTS.md` is not modified.
- Rationale: nothing here is cross-cutting. The rotation rule is one producer's internal mechanism, and the archive-name declaration is already covered by the existing Planparser Sole-Parser Invariant's "sole declarer of the plan directory's path" clause rather than being a new rule. Per this repo's `CLAUDE.md`, `CONSTRAINTS.md` moves only for a genuinely new cross-cutting invariant.
- Rejected: recording a "plan directory rotation" invariant (a single call site is not a cross-cutting invariant).

## Technical context

**The producer chain, top to bottom.**
`contracts/recipes/loom-recipe.yaml` names the row and its engine.
`internal/loomrecipe`'s `New` assembles the thirteen rows, resolving each `engine:` through `internal/shedrecipe`'s `Lookup`.
The registry's `Constructor` (`func(name string, cfg Config, env Env) (shedengine.ShedProducer, error)`) builds the producer from the caller-filled `Env`.
`internal/loomcli`'s `wire()` (`internal/loomcli/wiring.go`) is what fills that `Env`.

**The model to copy is one row up.**
`git show c2638bb3` is the Done **loom: Discussion-Write producer** commit and is the closest possible template for this task — same seam, same decorator shape, same `Env` extension, same test topology.
Its touched files map almost one-to-one onto this task's:
`internal/loomshed/discussionwrite.go` + `_test.go`, `internal/shedrecipe/entries_discussionwrite.go` + `_test.go`, `internal/shedrecipe/recipe.go` (Env fields), `internal/shedrecipe/registry.go` (map key), `internal/loomcli/wiring.go` + `_test.go`, `contracts/recipes/loom-recipe.yaml`, `contracts/stencils/loom/loom-template-discussion.md`, `internal/loomrecipe/{coverage_guard,fixture,resume,sequence,shape}_test.go`, `internal/shedbuild/{build_engines,fixture}_test.go`, `manifest/designs/loom.md`, `manifest/roadmap.md`.

**What already exists and must not be rebuilt.**

- `internal/loomengine/plan.go` — `PlanSpec(layout *lyxcwd.Location, stencilsDir string, cfg Config, reg modelspec.Registry) (shuttleengine.Spec, error)` and its `composePlanPrompt`. Already resolves the `plan` role model-spec, already reads `contracts/stencils/loom/loom-template-plan.md` at call time via `stencilstore.Read` + `stencil.FillOptional`, already sets `OutputFiles: []string{overviewPath}`, `Interactive: false`, `Role: "plan"`, `Timeout: cfg.PlanTimeoutMin`. Already fills the optional `pattern_directive` marker via `pattern.Directive(layout.AnchorPath(), stencilsDir, pattern.RoleImplementer)`.
- `internal/loomengine/plan_test.go` — 387 lines of tier-1 coverage over the above, including anchoring proofs (`TestPlanSpec_AnchoredUnderAnchorPathNotWorktreePath`) and eight prompt-content assertions.
- `internal/loomengine/template.yaml` — already carries `plan: opus[effort=high]` and `plan_timeout_min: 120`.
- `contracts/stencils/loom/loom-template-plan.md` — already rewritten to format 4 by the Wave-2 planparser migration. It documents the card definition, on-disk layout, `00-overview.md` frontmatter and sections, per-card field order and grammar, the `Uses:`/overlap contradiction rule, the `## Rename mechanic` verbatim block, and a minimal skeleton. The four markers are `{{.decision_record_path}}`, `{{.plan_dir}}`, `{{.overview_path}}`, and the optional `{{.pattern_directive}}`.
- `internal/planparser` — format-4-complete. `PlanDirName`/`PlanDirRel()`/`PlanDir(anchorPath)`/`PlanOverview(anchorPath)`, `ParsePlan`, `Validate` with sixteen check IDs.
- `internal/loomcli/validate.go` — the `validate-plan` verb, zero arguments, calling `planparser.Validate`.
- `internal/shedadapters/singlellm.go` — `SingleLLMProducer`, `SpecSource`, `Shuttle`. Its `Call` rejects any non-absolute `OutputFiles` entry, archives stale outputs, runs the seam once, and maps `OutcomeDone`→`Done` (pointer = `OutputFiles[0]`), `OutcomeAsking`→`Stuck`, `OutcomeDied`/`OutcomeTimeout`→error.

**The two files that make the rotation decision concrete.**

- `internal/planparser/validate.go`, the `index-file-mismatch` block: `os.ReadDir(plan.Dir)`, skipping `e.IsDir()`, non-`.md` names, and `overviewFileName`; every remaining name absent from the Card Index becomes a finding. This is why an archive **subdirectory** is invisible and an archive **sibling file** is not.
- `internal/shedadapters/archive.go`, `archiveStaleOutputs`: renames each existing output file to `<dir>/<base>-<stamp><suffix><ext>` — a sibling in the same directory. For `_lyx/plan/00-overview.md` that sibling is a `.md` file inside the plan directory.

**Naming and file placement.**

- New producer file: `internal/loomshed/planwrite.go`, alongside `discussionwrite.go`, with the same unexported-struct + exported-constructor-returning-the-seam-interface shape (`func NewPlanWrite(...) shedengine.ShedProducer`), so `internal/shedrecipe` can construct it while the type stays unexported.
- New registry entry file: `internal/shedrecipe/entries_planwrite.go`.
- `internal/loomshed/stub.go`'s doc comment currently says the stub backs "four rows … Discussion-Review, Plan-Write, Plan-Review, and Webster-Review" — it must become three rows and drop `Plan-Write`, exactly as the Discussion-Write commit edited that same sentence.
- `internal/loomshed/doc.go` was touched by the Discussion-Write commit for the same reason; check it for a row inventory that now needs updating.

**Clock injection.** `NewSingleLLMProducer` takes a `now func() time.Time` and defaults nil to `time.Now`. The rotation needs a stamp too, so `NewPlanWrite` takes the same `now func() time.Time` parameter and defaults nil identically — that is what lets a test pin the archive directory name and exercise the collision suffix. `Env.Now` supplies it in production and is currently nil in `wire()`, which is legal.

**Anchoring.** Every path this task touches resolves against `location.AnchorPath()`, never `WorktreePath()`. `planparser.PlanDir` takes the anchor path; `planparser.Validate` takes the worktree root; `NewPlanValidate`'s doc comment already records that these are not the same value and must not be conflated.

## Constraints

From `CONSTRAINTS.md`, the ones this task can violate:

- **Shed Recipe Registry Invariant** — every registry value constructs a `shedengine.ShedProducer` and nothing else; the registry stays one central map literal reached only through `Lookup`/`Names`, never `init()` self-registration and never a runtime `Register`. `internal/shedrecipe` takes every absolute path from its caller and has **no direct production import of `internal/lyxcwd`** — which is precisely why `PlanSpec` must arrive as an injected closure rather than be built inside the entry. Enforced by `internal/shedrecipe/seam_enforcement_test.go` and the two coverage tests.
- **Planparser Sole-Parser Invariant** — `internal/planparser` is the sole parser of `_lyx/plan/` and the sole declarer of the plan directory's path; it never resolves cwd and never imports `internal/lyxcwd`; the caller supplies `AnchorPath()`. The new archive-name helper must be a pure string function that keeps the package stdlib-only.
- **Lyxdirs Single-Declarer Invariant** — no production file may name the `_lyx` literal in path-construction context. The archive path is built from `planparser`'s existing accessors, never from a fresh `filepath.Join(..., "_lyx", ...)`. Enforced by `internal/lyxcwd/enforcement_test.go`'s `TestEnforcement_GeometryLiterals`.
- **Durable-vs-Ephemeral State Invariant** — `_lyx` holds tracked content only. The archive subdirectory lives under `_lyx/plan/` and is therefore git-tracked by design, which is why it must be swept into the commit pathspec rather than left untracked.
- **Stencil Ownership Invariant** — the prompt is read at call time from a told absolute stencils directory; `//go:embed` in `contracts/stencils` carries seed defaults only. Editing `contracts/stencils/loom/loom-template-plan.md` is the correct and only way to change the prompt; do not add an embedded live-read path. Note the practical consequence: a hub whose seeded copy has been hand-edited will not be overwritten (a body hash mismatching its stamp is never overwritten), so a stencil change reaches an existing hub only through the normal seed/refresh pass.
- **Gate Self-Check Parity Invariant** — the `validate-plan` verb and the `Plan-Validate` row both call `planparser.Validate` and neither re-implements the other. This task adds no new gate, so no new verb or parity test is required; it only points the stencil at the existing verb.
- **Told-Geometry Invariant** and **Cwd Resolution Invariant** — `internal/lyxcwd` alone resolves cwd; every root reaching `shedrecipe`/`loomshed`/`planparser` is told by the caller.
- **Config Strictness Invariant** — `planWriteEntry` must call `configRejectUnknown(cfg)` first, exactly as `discussionWriteEntry` does; the row carries no config keys of its own.
- **Test Tier Purity Invariant** — tier-1 tests resolve no cwd and spawn no process. Every test named below is tier 1: the decorator's seam is a fake `ShedProducer` and a fake commit closure, and the rotation runs against a `t.TempDir()`.
- **Documentation Lifecycle** (this repo's `CLAUDE.md`) — a task changing observable CLI behaviour updates the module doc in `manifest/designs/` in the same commit; `manifest/roadmap.md` moves on completing a planned item.
- **Markdown: semantic line breaks** — one sentence per line, breaking inside long sentences at independent-clause boundaries, in every `.md` file touched, including the stencil.
- **Producer Pointer-Rule Invariant** — an instruction file must never duplicate or paraphrase another producer's format-contract content, only point at it, so editing the one format-contract file is sufficient to change what both producer and consumers do. This binds every stencil edit in this task. The stencil is already the pinned LLM-facing subset of the plan format (`contracts/specs/loom-plan-spec.md`'s own opening states the arrangement: the spec is the Go parser's fuller contract, the stencil is what `Plan-Write` reads, and the prompt never duplicates the spec). New stencil text must stay on the producer's own side of that line — instructions about what to do, pointers for everything else.

One mechanical note, not an invariant: the decorator returns the inner producer's `OutputPointer` verbatim (`_lyx/plan/00-overview.md`, `OutputFiles[0]`), as `discussionWrite` does; it does not substitute the plan directory.

## Testing

Tier-1 throughout. No tier-2 or integration test is warranted: nothing here spawns a process, touches a real repo, or drives an LLM.

**`internal/loomshed/planwrite_test.go`** (new; model it on `discussionwrite_test.go`, 136 lines):

- Delegation: `Done` + nil error from the inner producer invokes `commit` exactly once and returns the inner outcome and pointer verbatim.
- Pass-through: `Stuck` from the inner producer does **not** invoke `commit`, and returns the inner three results unchanged.
- Pass-through: a non-nil inner error does not invoke `commit`.
- Commit failure maps to a returned error, not `Stuck`, and the error text names the producer.
- Rotation happens **before** the inner `Call` — assert ordering, e.g. by having the fake inner producer record the plan directory's contents at call time and asserting it saw no `*.md` files.
- Rotation moves every top-level `*.md` including `00-overview.md` into `archive-<stamp>/`, preserving each file's content.
- Rotation leaves existing subdirectories in place: a pre-existing `archive-<older>/` is still there afterwards and was not nested inside the new one.
- Rotation over an absent plan directory is a no-op with a nil error.
- Rotation over an existing but empty plan directory is a no-op and creates no archive directory.
- Same-second collision: two rotations with a pinned clock produce `archive-<stamp>` and `archive-<stamp>-1`.
- Rotation failure returns an error and never calls the inner producer (drive it by making the archive path unwritable, or by pre-creating a regular file where the archive directory must go — pick whichever is portable to Windows).
- A nil `now` defaults to `time.Now` without panicking.

**`internal/planparser`** — one table test over the archive-name helper: name shape for an empty suffix and for a `-1` suffix. Add it to whichever existing `_test.go` file already covers `PlanDirRel`/`PlanDir`.

**`internal/shedrecipe/entries_planwrite_test.go`** (new; model it on `entries_discussionwrite_test.go`, 165 lines):

- A fully filled `Env` constructs a non-nil producer with a nil error.
- Each of the three required seams missing (`PlanSpec`, `CommitPlan`, `Shuttle`) produces a distinct error naming both `"PlanWrite"` and the missing field.
- An unknown `Config` key is rejected.
- The returned producer drives the injected `SpecSource` and `Shuttle` — proving the entry actually wired them rather than constructing something inert.

**`internal/shedrecipe/registry_test.go`** — the exact-names test grows to fourteen entries and its name changes from `TestRegistry_ShipsThirteenEntries`.

**`internal/loomrecipe`** — follow the Discussion-Write commit's own set: `coverage_guard_test.go` (`loomRowEngines[NamePlanWrite] = "PlanWrite"`), `shape_test.go` (the `Plan-Write` row's expected type becomes the `NewPlanWrite` type; the `Discussion-Review`→`Plan-Write` and `Plan-Write`→`Plan-Validate` routing rows keep their targets), `sequence_test.go`, `resume_test.go`, and `fixture_test.go` (the shared test `Env` must now fill `PlanSpec` and `CommitPlan`, or `New` will fail construction for every test in the package — this is the single most likely source of a wide, confusing test break, so do it first).

**`internal/shedbuild/{build_engines,fixture}_test.go`** — same fixture-`Env` widening, same reason.

**`internal/loomcli/wiring_test.go`** — assert `wire()` fills `Env.PlanSpec` and `Env.CommitPlan` non-nil, and that evaluating the `PlanSpec` closure against a hand-built `*lyxcwd.Location` returns a Spec whose single `OutputFiles` entry is the `AnchorPath()`-rooted `_lyx/plan/00-overview.md`. Mirror the `DiscussionSpec` assertions the Discussion-Write commit added (96 lines of them).

**`internal/loomengine/plan_test.go`** — extend the existing prompt-content assertions, one per new stencil section, in the style of the eight already there (`TestPlanSpec_PromptStates…`): the Step 0 skill names both appear; the degraded-mode section states that a missing inventory is not an error; `lyx loom validate-plan` appears; the Verify-model paragraph appears. Keep asserting no leftover `{{` marker.

**TDD candidates.** Two, both because the expected behaviour is fully specified before any code exists: the rotation helper (the eleven rotation cases above are a complete behavioural spec) and the decorator's outcome mapping (a four-row truth table over inner-outcome × commit-result). Write both test files before their implementations.

**Verification gate.** `go build ./... && go test ./...` from the worktree root. Watch specifically for `internal/loomrecipe` and `internal/shedbuild` fixture breaks — a widened `Env` that one fixture forgets to fill surfaces as a `New` construction error in unrelated tests.

## Q&A log

- **Q:** How should the `Plan-Write` producer be wired — dedicated registry entry, or reuse the generic `SingleLLM` entry? **A:** [auto-pick] Dedicated `PlanWrite` entry wrapping `SingleLLMProducer` behind a `loomshed.NewPlanWrite` decorator. **Why:** the generic entry cannot carry a `*lyxcwd.Location` (Shed Recipe Registry Invariant) and would bypass the `plan` role's model-spec and timeout resolution — the identical reasoning `discussionWriteEntry` already records.
- **Q:** How is the stale `_lyx/plan/` handled on a `Plan-Validate` or `Plan-Review` bounce back into `Plan-Write`? **A:** [auto-pick] The decorator moves every top-level `*.md` into `_lyx/plan/archive-<ts>/` before delegating. **Why:** leftover cards are a guaranteed `index-file-mismatch` finding and would ping-pong the two rows until the bounce budget is exhausted; a subdirectory is invisible to both `ParsePlan` and the validator, and it keeps the commit pathspec a single always-present path.
- **Q:** Who declares the archive subdirectory's name? **A:** [auto-pick] `internal/planparser`, as a pure stdlib-only string helper beside `PlanDirRel`; `internal/loomshed` performs the filesystem move. **Why:** the Planparser Sole-Parser Invariant makes `planparser` the sole declarer of the plan directory's path, but a parser package must not mutate the filesystem.
- **Q:** Should the plan be committed on a `Done` outcome? **A:** [auto-pick] Yes — `Env.CommitPlan`, pathspec `planparser.PlanDirRel()`, mirroring `CommitDiscussion`. **Why:** keeps the tree clean across every `Plan-Validate` bounce, makes the plan durable across a crash, and sweeps the archive subdirectory into git rather than leaving it as untracked dirt.
- **Q:** Does the recipe row need routing changes? **A:** [auto-pick] No — flip `engine: Stub` to `engine: PlanWrite` and nothing else. **Why:** `Plan-Write` is a bounce target, not a bouncer; no row produces what a stuck writer needs, so escalation to a human is correct, per the recipe file's own header comment.
- **Q:** Which scribe skills should the stencil load? **A:** [auto-pick] `scribe:prose` then `scribe:testing`, best-effort. **Why:** the plan is a text artifact, and card granularity plus the bundled-test rule are testing judgments; `conversation` is chat discipline with no operator present, and `code-quality`/`golang-*` belong in webster's implementer prompt.
- **Q:** How does `Plan-Write` learn there is no quarry inventory? **A:** [auto-pick] Static stencil prose naming the manual substitutes (`go doc`, `grep -rn`) and stating that absence is normal, never an error. **Why:** `Plan-Sweep` is not a row and produces no artifact, so a marker could only ever render one constant string; adding one when `Plan-Sweep` ships is a one-line change.
- **Q:** Should the stencil tell the agent to self-check? **A:** [auto-pick] Yes — `lyx loom validate-plan`, re-run until it exits 0. **Why:** the Gate Self-Check Parity Invariant guarantees the verb and the gate call the same function, so a clean self-check genuinely predicts the gate's verdict and saves a whole shuttle round.
- **Q:** Does the stencil need to explain the three-tier Verify model? **A:** [auto-pick] It states the authoring rule only — `Verify:` is exceptional — and points at `plan-card-format.md` for the tier definitions. **Why:** the current stencil never tells the agent when *not* to write a per-card `Verify:`, but restating the tier taxonomy would duplicate format-contract content, which the Producer Pointer-Rule Invariant forbids.
- **Q:** Named `Env` fields or a keyed map for the second producer's injected seams? **A:** [auto-pick] Two named fields, `PlanSpec` and `CommitPlan`. **Why:** `DiscussionSpec`'s own field doc records the named-per-producer choice explicitly; forking to a keyed map on the second instance would abandon a convention one commit old.
- **Q:** Does this task record a new invariant in `CONSTRAINTS.md`? **A:** [auto-pick] No. **Why:** rotation is one producer's internal mechanism and the archive-name declaration already falls under the existing Planparser Sole-Parser Invariant; a single call site is not cross-cutting.
