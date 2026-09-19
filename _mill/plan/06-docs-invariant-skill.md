# Batch: docs-invariant-skill

```yaml
task: "Shed-generic watchdog for ly-drive and loom's CLI verbs"
batch: "docs-invariant-skill"
number: 6
cards: 5
verify: go test ./cmd/lyx/... ./tools/...
depends-on: [5]
```

## Batch Scope

This batch lands the documentation half of the task: the new **Shed Verb-Set Invariant** in `CONSTRAINTS.md`, the `ly-drive` skill's generalization from a loom driver to a recipe driver, and the four doc surfaces the Documentation Lifecycle requires — the module doc, `docs/overview.md`'s module table and execution-stack entries, `manifest/roadmap.md`'s Planned item, and `plugins/ly/skills/INDEX.md`'s row.

It depends on batch 5 and nothing else: every claim it writes down describes behaviour that must already exist, and the skill cannot be told to invoke `lyx shed step --recipe <name>` before that command ships.
The invariant's two mechanically-shaped clauses are already enforced by tests written in earlier batches — `internal/shedverbs`'s seam scan (batch 3, card 19) covers the no-resolver and no-`<module>cli` clauses, and `internal/shedcli`'s table scan (batch 5, card 31) covers the single-declaration-site clause — so this batch writes the invariant's text and points it at those existing tests rather than adding a third.

Batch-local decision: the `ly-drive` rewrite is driven by the six enumeration *rules* `_mill/discussion.md`'s `ly-drive-drives-any-recipe` Decision lists, applied exhaustively across the whole file, rather than by patching the instances the discussion names.
Those instances are illustrations of each rule, not its extent — a hand-listed set has already proved incomplete once.

## Cards

### Card 35: add the Shed Verb-Set Invariant and update the CLI/Cobra entry

- **Context:**
  - `internal/shedverbs/doc.go`
  - `internal/shedverbs/seam_enforcement_test.go`
  - `internal/shedverbs/step.go`
  - `internal/shedverbs/step_test.go`
  - `internal/shedcli/doc.go`
  - `internal/shedcli/table.go`
  - `internal/shedcli/table_test.go`
  - `internal/shedcli/cli.go`
  - `internal/lifecyclecli/cli.go`
  - `cmd/lyx/main.go`
- **Edits:**
  - `CONSTRAINTS.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** add a `## Shed Verb-Set Invariant` section stating four clauses: `internal/shedverbs` owns the generic `run`/`step`/`status`/`pause` verb bodies and no `<module>cli` reimplements one;
  `shedverbs` derives no path and imports no resolver — no `lyxcwd`, no `os.Getwd`, no `git rev-parse` — and imports no `<module>cli`, which is what keeps it a leaf and `internal/shedcli`'s own imports acyclic;
  the `lyx shed` recipe table lives in `internal/shedcli` alone, as one map literal reached through accessors, with every name armed by exactly one arming function and no `init()` self-registration;
  and the `step` refusal-kind vocabulary stays closed at its five values.
  Name the enforcing tests for each mechanically-shaped clause, in the style the Lifecycle Bookend Invariant's own entry already uses for its two partial proxies: `internal/shedverbs/seam_enforcement_test.go` for the leaf clause, `internal/shedcli/table_test.go` for the table clause, and `internal/shedverbs/step_test.go` for the closed-kind clause.
  State plainly that the first clause — no `<module>cli` reimplements a verb body — is review discipline rather than a scan, since "reimplements" has no static shape a scan can see.
  Place the section adjacent to the two existing Shed invariants (`## Shed Producer-Seam Invariant`, `## Shed Recipe Registry Invariant`) so the three read together.
  Update the `## CLI / Cobra Invariant` section's three affected lines: change "twelve of thirteen also carry `RunCLIIn`" to "thirteen of fourteen", since `internal/shedcli` is the new CLI module and carries all three seams;
  add `shedcli` to the deviations list as importing `internal/shedverbs`, `internal/loomcli` and `internal/lifecyclecli` with no engine package of its own, in the same shape as the existing `lifecyclecli` entry;
  and extend the interactive-handoff exception list with `lyx shed status --watch` and `lyx lifecycle status --watch` alongside the existing `lyx loom status --watch` entry, since generalizing the watch tail created two new never-exiting commands.
  State in the new section that `internal/shedverbs` is not a CLI module and is not counted in that tally at all — it exposes no `Command()`/`RunCLI` seam, only the `Verbs(texts, spec)` constructor the three subtrees build from.
  Add neither `shedverbs` nor `shedcli` to the Told-Geometry Invariant's bound-packages list: that list binds engines, both sit above that layer, and their identical no-derived-paths obligation is carried by the new invariant's own no-resolver clause instead.
- **Commit:** `docs(constraints): add the Shed Verb-Set Invariant and update the CLI/Cobra entry`

### Card 36: generalize the ly-drive skill

- **Context:**
  - `internal/shedcli/cli.go`
  - `internal/shedverbs/step.go`
  - `internal/shedverbs/status.go`
  - `internal/loomshed/interruptpolicy.go`
  - `internal/lifecyclecli/arm.go`
  - `internal/lifecyclecli/refusal.go`
  - `contracts/recipes/loom-recipe.yaml`
  - `contracts/recipes/lifecycle-recipe.yaml`
- **Edits:**
  - `plugins/ly/skills/ly-drive/SKILL.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** rewrite the skill to drive any recipe through `lyx shed step --recipe <name>`, taking a recipe name as its argument and defaulting to `loom`, by applying all six enumeration rules exhaustively across the whole file rather than only to the instances named below, which are illustrations of each rule and not its extent.
  Rule (a) — every `lyx loom <verb>` invocation becomes `lyx shed <verb> --recipe <name>`: that covers the step calls in `## How to invoke a step` and `## The loop`, the pre-loop baseline read in `## The pre-loop baseline`, and both `lyx loom status` reads in `## Interrupted invocations`.
  Rule (b) — every claim about a command's output shape that is not in the generic contract is gated or generalized: that covers `## The pre-loop baseline`'s assertion that an absent status file returns an error envelope naming `lyx loom start`, which is true for loom and false for lifecycle, and the `interrupt_policy` field read in `## Interrupted invocations`, which for a recipe with no policy table is absent from the envelope entirely rather than present-and-empty.
  Add the third arm the branch lacks today: treat an empty or absent policy as `handback`, because the conservative arm is the one that never restarts in-flight work.
  Rule (c) — every claim about the substrate or filesystem a recipe runs over is gated on the recipe name: that covers `## Preconditions`' reed-strand `$TMUX_PANE` self-check and its whole three-outcome branch, the cwd precondition "must be the task worktree root" (lifecycle refuses from anything but the hub's prime worktree), `## Self-report`'s friction directory at `.lyx/loom/friction/`, and that same section's `lyx selfreport create` gate.
  Rule (d) — every numeric claim derived from a recipe's own graph is gated: that covers `## The loop`'s "Loom's list is seventeen rows" arithmetic behind the 40-step cap, including the review-round and bounce-budget arithmetic that follows from it.
  Rule (e) — the skill's own identity metadata is generalized: the frontmatter `description` stops saying "Drive a loom task", and the frontmatter gains `argument-hint: "[recipe]"`, which is how the recipe name is passed, defaulting to `loom` when the operator gives none.
  Rule (f) — every claim about a recipe's producer or adapter semantics is gated: that covers `## Interrupted invocations`' "kills the in-flight agent and restarts that row's work" and its closing "every spawning row's adapter probes for a live agent and waits on it", both of which are statements about loom's own adapters that a recipe built from different registry entries need not satisfy.
  Keep every recipe-agnostic section as written: the 40-step cap itself, the `continue` branch, the five error kinds and the one-retry rule for `producer`, the interrupted-invocation branch on `current_producer`/`history_length`, the never-clean-up rule, and `## Operator choices`' numbered-text-list requirement.
  Keep `disable-model-invocation: true` — the skill stays explicit-invocation-only.
  Write every prose paragraph and list item with semantic line breaks, one sentence per line, per this repo's markdown convention.
- **Commit:** `docs(ly-drive): generalize the skill to drive any recipe through lyx shed step`

### Card 37: move the skill index row with the skill

- **Context:**
  - `plugins/ly/skills/ly-drive/SKILL.md`
- **Edits:**
  - `plugins/ly/skills/INDEX.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** update the `ly-drive` table row's description so it repeats the skill's new frontmatter `description` verbatim, as it repeats the old one verbatim today, and update the prose line following the table that repeats the explicit-invocation claim if card 36's rewrite changed its wording.
  The table cell stays on one line per this repo's markdown convention, which exempts table cells from the semantic-line-break rule.
- **Commit:** `docs(ly): update the ly-drive index row for the generalized skill`

### Card 38: update the module doc, overview and roadmap

- **Context:**
  - `internal/shedverbs/doc.go`
  - `internal/shedcli/doc.go`
  - `internal/shedbuild/newshed.go`
  - `internal/lifecycleshed/innerrun.go`
  - `CONSTRAINTS.md`
  - `manifest/designs/hardener.md`
  - `manifest/designs/shed.md`
  - `plugins/ly/skills/ly-drive/SKILL.md`
- **Edits:**
  - `manifest/designs/shed-generic-watchdog.md`
  - `docs/overview.md`
  - `manifest/roadmap.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** rewrite `manifest/designs/shed-generic-watchdog.md` from a speculative sketch into the shipped module doc: replace `## Why still speculative` with what shipped, since the second `shedrecipe` consumer it was waiting on now exists and has been validated against.
  Record the delivered shape — `internal/shedverbs` owning the four generic verb bodies, `internal/shedcli` owning the `lyx shed` subtree and its named-recipe table, `shedbuild.ShedPaths`/`NewShed` as the deduplicated assembler, and the two existing subtrees rearmed with their surface unchanged.
  Keep the settled verdict that `start` does not generalize, and update its "is itself open" phrasing, which this task closed.
  Add the note `_mill/discussion.md`'s Scope requires: Hardener is the future consumer of both the verb set and the neutralized inner-run engine, and neither a Hardener recipe nor any Hardener artefact was written here, since `manifest/designs/hardener.md` still carries a DRAFT banner saying not to implement from it yet.
  In `docs/overview.md`, add a module-table row for `internal/shedverbs` and one for `internal/shedcli`, placed adjacent to the existing `internal/shedengine`/`internal/shedrecipe`/`internal/shedbuild` rows, each with a one-line description matching that package's own `doc.go`;
  update the `loom` module entry so it no longer claims `internal/loomcli` hosts the driver, status and pause verb *bodies*, and update the `lifecycle` entry's verb list from `lyx lifecycle run|status` to `lyx lifecycle run|status|pause`;
  extend the `shed` section's execution-stack prose to record the verb set and the subtree as shipped, alongside its existing implemented-status notes for the engine, the adapters, the registry and the loader.
  Add links only where they resolve — the Markdown Link Integrity checker must stay green.
  In `manifest/roadmap.md`, move the Planned item to whichever section this repo's convention uses for completed work, following the format of the entries already there;
  make no other roadmap movement, since `manifest/roadmap.md` moves only on completing or adding a planned item.
  Write every prose paragraph and list item with semantic line breaks, one sentence per line.
- **Commit:** `docs: record the shipped Shed-generic verb set across the module doc, overview and roadmap`

### Card 39: confirm the doc guards stay green

- **Context:**
  - `CONSTRAINTS.md`
  - `docs/overview.md`
  - `manifest/roadmap.md`
  - `manifest/designs/shed-generic-watchdog.md`
  - `plugins/ly/skills/ly-drive/SKILL.md`
  - `plugins/ly/skills/INDEX.md`
  - `cmd/lyx/constraintchokepoint_test.go`
  - `cmd/lyx/retiredverbs_test.go`
- **Edits:** none
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** run this batch's `verify:` command and confirm both packages pass, then run the repo's Markdown Link Integrity checker over the four edited markdown files and confirm every link added by card 38 resolves.
  Locate that checker by grepping the repo for the invariant's name rather than assuming a path, and run it the way its own documentation says to.
  Confirm `cmd/lyx/constraintchokepoint_test.go` still passes against the new `CONSTRAINTS.md` section, since it is the guard that reads that file's structure, and confirm `cmd/lyx/retiredverbs_test.go` still passes, since this task retires no verb and that guard must not have started reporting one.
  Confirm by grep that no file under `plugins/ly/skills/ly-drive/` still contains the literal string `lyx loom step`, which is the mechanical check that card 36's rule (a) was applied exhaustively rather than partially.
  Confirm by grep that `internal/shedverbs` and `internal/shedcli` each appear in `docs/overview.md`, which is the mechanical check that card 38's module-table rows landed.
  This card changes no file.
- **Commit:** none

## Batch Tests

`verify:` runs `cmd/lyx` and `tools/...`: the first because `constraintchokepoint_test.go` reads `CONSTRAINTS.md`'s structure and `retiredverbs_test.go` reads the live command tree against the docs, the second because the sandbox suite files and their guard live under `tools/sandbox`.

This is a docs-and-skill batch with no Go production surface of its own, so most of its verification is the mechanical confirmation card 39 performs: the Markdown Link Integrity checker over the four edited markdown files, and two greps proving the two exhaustive rewrites — no surviving `lyx loom step` under the skill directory, and both new packages present in the module table.

The narrower scope is deliberate rather than an omission: no card in this batch edits a `.go` file, so the wider suites cannot have changed, and the repo-wide done gate covers them at task end regardless.
