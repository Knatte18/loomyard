# Discussion: builder: delete internal/builderengine and internal/buildercli, retire builder-contract.md as a reference

```yaml
task: 'builder: delete internal/builderengine and internal/buildercli, retire builder-contract.md as a reference'
slug: builder-retire
status: discussing
parent: main
```

## Problem

`builder` is the older batch-implementation loop: an LLM orchestrator session driving fat `lyx builder` verbs through a pinned plan-format-v2 plan, spawning each batch's implementer as its own reed/tmux strand.
`webster` replaced it — a long-lived Master session that reads the whole flat card-list plan once and forks one implementer per batch in-session — and webster is now the implementer module for the whole stack.

Builder is not dormant, though: `cmd/lyx/main.go` registers `buildercli.Command()`, `internal/configreg` registers a `builder` config module, and the module appears in the CLI help tree, the sandbox coverage guard, and several cross-module test guards.
Parking it in-tree therefore costs recurring maintenance — it stays in the help tree per the CLI/Cobra Invariant, it keeps a second plan parser alive against the Planparser Sole-Parser Invariant, and every future refactor has to carry it.
`manifest/roadmap.md` has called it "superseded as an active plan-format consumer" and "obsolete" since webster shipped; nobody removed it.

**Why now:** the `Shed` producer-model rewrite broke into six follow-up tasks (`manifest/designs/shed-followups.md`), and this is task A — the head of the chain.
Task B cannot reuse the `plan-format.md` filename until this task deletes the v2 doc, and task D's link targets move in this task.
The full task specification is `manifest/designs/shed-followups.md#a--builder-retire`; this discussion extends it with decisions the spec left open and with sites the spec's inventory missed.

This task is **one task producing one compiling commit**.
A package deletion is atomic by nature; splitting it guarantees an intermediate state that does not build.

## Scope

**In:**

- Delete `internal/builderengine` and `internal/buildercli` entirely.
- Unregister builder from `cmd/lyx/main.go` (command registration + the module list in the long help text) and from `internal/configreg`.
- Fix every test the deletion breaks: `cmd/lyx/helptree_test.go`, `cmd/lyx/notransients_test.go`, `cmd/lyx/constructoranchoring_test.go`, `cmd/lyx/rawgitmutation_test.go`, `internal/configreg/configreg_test.go`, `internal/configcli/configcli_test.go`, `internal/scoutcli/cli_test.go`, `internal/webstercli/cli_test.go`, `internal/webstercli/sync_integration_test.go`, `internal/fabricengine/weftgit_exclude_test.go`.
- Delete the builder sandbox suite: `tools/sandbox/SANDBOX-BUILDER-SUITE.md`, `sandbox/builder-suite.cmd`, the `builderSuite` spec and `//go:embed` in `tools/sandbox/suite.go`, the `"builder-suite"` case in `tools/sandbox/main.go`, and scenario **S9** (including its `**Covers:** builder` tag) in `tools/sandbox/SANDBOX-CORE-SUITE.md`.
- **Delete** `docs/reference/builder-contract.md` outright.
- **Create** `docs/reference/webster-contract.md` holding webster's consumer-facing contract, and re-point every inbound deep link to it.
- **Delete** `docs/reference/plan-format.md` (the v2 doc).
- **Rename the loom phase `builder` → `webster`**, including `internal/loomengine/coherence.go`'s `validPhases` map, `docs/reference/status-schema.md`'s enum twin, the `builder-review` gate name, and every prose site.
- **Sweep every remaining `builderengine` / `buildercli` / builder-module reference in the repo**, including the ~40 provenance doc-comments in `websterengine`/`webstercli` that the task spec's inventory does not list.
- Rebuild `docs/reference/model-spec.md`'s worked example on `webster.yaml` instead of `builder.yaml`.
- Update `manifest/designs/shed-followups.md` in the same commit to record this task's two overrides of its own spec (the phase-enum rename, and the `builder-contract.md` deletion) so tasks B, C, D, E and the `Shed` build task are not working from a stale spec.
- `.gitattributes` — remove the three `internal/builderengine/*` LF pins.

**Out:**

- **Dated historical records are not edited.**
  `docs/benchmarks/test-suite-timing.md`, `docs/benchmarks/fixture-copy.md`, `docs/benchmarks/scout-vs-grep.md`, `docs/research/scout-agent-usage-findings.md`, `docs/research/scout-spike.md`, `docs/research/scout-multilang.md`, `crucible/README.md:158`, `crucible/review-prompt-template.md:152`.
  These are timestamped records of what was measured or what happened on a given date; editing them falsifies the record rather than cleaning it.
- **`manifest/designs/loom.md`'s prose claims** at `:91–94` and `:187` — task E is the named owner (`shed-followups.md:379–380`), and `loom.md` has a strict chain-order owner list (B → C → E).
  This task fixes **only the dangling links** in those lines, because their target file ceases to exist; the prose stays E's.
- **`docs/reference/plan-format-v3.md:5`'s "Coexistence, not replacement" section** — entirely task C's, per `shed-followups.md:101` and `:239–240`.
  Its `[plan-format.md v2](plan-format.md)` link is **left dangling on purpose** — see the `plan-format.md` exclusion in `loom-md-links-fixed-prose-deferred`.
- **Every other inbound `plan-format.md` link** — left dangling for the same reason, for the A→B window only.
  `shed-followups.md:183–184` records this as designed behaviour, not oversight.
- **The `plan-format-v3.md` → `plan-format.md` rename itself** — task B's whole job.
  This task only *frees* the filename.
- **Existing `builder.yaml` files in already-created worktrees.**
  Once `builder` leaves `configreg`, `lyx config reconcile` stops emitting `builder.yaml`; existing files are left in place.
  They are inert once no module reads them, and reconcile does not delete files it no longer owns.
  Stated here so nobody files it as a leak.
- **No new tests are written.** The existing suite is the test.

## Decisions

### delete-outright-not-freeze

- Decision: delete `internal/builderengine` and `internal/buildercli` from the tree.
- Rationale: builder is live-registered, so it costs help-tree, configreg, test, and refactor maintenance indefinitely.
  The implementation stays one `git show` away, permanently.
- Rejected: keeping the code frozen in-tree (pays that cost forever for a reference binary);
  moving it to a `sandbox/` or `attic/` directory (invents an excluded-directory convention this repo does not have).

### delete-builder-contract-create-webster-contract

- Decision: delete `docs/reference/builder-contract.md` outright.
  Create `docs/reference/webster-contract.md` in its place as webster's own cross-module contract.
- Rationale: "Builder" is no longer a thing — webster is the implementer module, so the reference doc should be named and scoped for webster.
  Keeping a `builder-contract.md` stub would preserve a name for a module that does not exist.
  The 247 lines are git-tracked and recoverable exactly like the code.
  Separately, webster is currently the only live module with **no** reference doc of its own — its entire cross-module surface is described inside a competitor's retired contract — so this fixes a real gap rather than merely relocating text.
- Rejected: a Retired-status stub at the old path (keeps a dead module's name as a live filename);
  keeping the full 247-line body under a Retired banner (preserves a contract nothing consumes, and its plan-format-v2 links break this same commit).
- Consequence accepted: the one piece of forward-valuable design in the old doc — **chain rollback and the recovery ladder**, which webster has no equivalent for (webster has `recover-batch`, a fresh cold strand, but no chain) — is not carried forward.
  It is recoverable via `git show <parent-sha>:docs/reference/builder-contract.md`, and the deleting commit message must say so explicitly.

### webster-contract-is-consumer-facing-only

- Decision: `webster-contract.md` documents only what **other modules may rely on**:
  the `_lyx/webster/summary.md` contract, `outcome.yaml`, the `_lyx/webster/` directory as an ownership boundary, and the plan format as webster's input.
  Webster's internals — bracket verbs, fork-audit policy, crash/resume, integration fork + bisect, per-batch model assertion — stay in `internal/websterengine`'s package documentation, which already covers them in full.
- Rationale: `websterengine/doc.go` is already ~250 lines and already states everything in `builder-contract.md`'s Webster section **except** the `summary.md` contract and the `_lyx/webster/` boundary.
  Copying the rest would create a second, drifting statement of webster's shape.
  The `summary.md` contract is precisely what `manifest/designs/finalize.md` consumes, so it needs a stable, linkable home.
- The `summary.md` contract to carry over, verbatim in substance: first line `# <title>`, then free-form prose narrating what was actually built including deviations from the original task;
  required and fail-loud (presence + non-empty + title line) **only** when `outcome: done`;
  follows archive-never-refuse like every other stale artifact.
  It is Finalize's PR-text source, because a long-lived Master session is the only party with full oversight of what actually shipped.
- **The integration-failure section is part of the consumer-facing contract.**
  `internal/websterengine/summary.go`'s `AppendIntegrationFailure` extends an already-written `summary.md` with a `## Integration suite failed` section naming the bisect-localized offending card and its SHA (`summary.go:101–106`), as the document half of `BisectAndEscalate`'s escalation.
  Because Finalize dumps `summary.md` **verbatim** into the PR body, that appended section reaches the consumer — so `webster-contract.md` must state that a `summary.md` may carry it, not just the title-plus-prose shape.
  The *bisect mechanism* that produces it stays webster-internal, documented in `websterengine/doc.go`; only the fact that the section can appear in the artifact is contract.
- Rejected: putting the content in `websterengine/doc.go` (Markdown cannot anchor into a Go file, so all four inbound deep links would degrade to anchorless path mentions, and webster would be the only module whose cross-module contract lives solely in a Go comment);
  folding it into `plan-format-v3.md` (conflates the plan *format* with the *consumer's* contract, and hands task B a larger file to rename).

### batcher-clause-stays-generic

- Decision: `webster-contract.md` describes batching only as "webster groups a plan's cards into execution batches via a config-selected batcher".
  It must **not** name `webster.yaml`'s `batcher:` key or the `internal/batcher` registry's location.
- Rationale: task F extracts `internal/batcher` out of webster into a standalone `configreg` module with its own `batcher.yaml`.
  Naming today's wiring would make task F an unplanned second owner of this brand-new file, re-opening exactly the multi-owner problem the follow-up chain is structured to avoid.
- Rejected: stating today's wiring and letting F update it.

### phase-rename-builder-to-webster

- Decision: rename the loom phase `builder` → `webster` throughout, **including** `internal/loomengine/coherence.go`'s `validPhases` map and `docs/reference/status-schema.md`'s enum twin.
- Rationale: the phase is named after the module that runs it, and that module is webster now.
  Leaving the enum alone while renaming the prose would ship docs saying "Webster phase" against a validator that rejects `phase: "webster"` and requires `phase: "builder"` — internally contradictory, and worse than either extreme.
- **This overrides `manifest/designs/shed-followups.md:88–94`**, which explicitly defers the enum to the `Shed` build task on the grounds that editing it now means inventing an interim phase set `Shed` would discard.
  The override is deliberate: an enum naming a deleted module is not a neutral interim state, and `Shed` replacing the enum with a flat producer list later is not a reason to leave a wrong value in live validation code now.
- **`shed-followups.md` must be edited in the same commit** to record that task A now owns the phase rename, so task E and the `Shed` build task do not work from a stale ownership claim.
- Rejected: renaming prose only (internally contradictory);
  deferring entirely (leaves live validation code naming a deleted module).

### sweep-everything

- Decision: sweep **every** `builderengine` / `buildercli` / builder-module reference in the repo, not the task spec's partial inventory.
  Acceptance is a repo-wide zero-hit grep, subject only to the exclusions listed under Scope → Out.
- Rationale: a partial sweep is not checkable — it degrades into a per-site judgment call with no completion criterion.
  A zero-hit grep converts "did we get everything" from an opinion into a test.
- Rejected: fixing only compile blockers plus the spec's named sites (leaves e.g. `internal/scoutcli/cli.go:127` shipping a user-facing help example citing a deleted directory).

### provenance-comments-rewritten-to-stand-alone

- Decision: the ~40 provenance doc-comments in `websterengine`/`webstercli` that read "mirroring builderengine's own X" / "a webster-local copy of `builderengine.Y`" are **rewritten to state their reason directly**, dropping the builder reference.
  Example: `websterengine/pause.go:1` "a webster-local copy of builderengine's pause-flag mechanics" → "webster's own pause-flag mechanics, deliberately module-local rather than shared".
- Rationale: no dangling package names, and the reasoning the comment exists to convey is preserved.
- Rejected: deleting the builder clause and leaving the remainder (several comments — `websterengine/fingerprint.go:2`, `classify.go:2`, `runlevel.go:162` — exist almost entirely to state the derivation, and would become fragments explaining nothing);
  marking the derivation historical, e.g. "the since-deleted builderengine" (keeps a deleted package's name in ~40 places, which is what the sweep is for).

### mechanical-rename-vs-hand-rewrite

- Decision: split the work by kind.
  The **phase rename** (`builder` → `webster` as a phase/gate token) is a true rename — do it with a script and gate it with a zero-hit grep.
  The **comment sweep** is a rewrite — do it by hand, one sentence at a time, and gate *it* with the same repo-wide zero-hit grep for `builderengine|buildercli`.
- Rationale: no regex turns "mirroring builderengine's own runlevel.go naming note" into standalone prose;
  scripting that half produces nonsense that needs a full hand pass anyway.
  The grep gate gives both halves the same completion criterion regardless of how they were produced.
- Rejected: scripting both;
  hand-doing both (loses the zero-hit guarantee on the rename).

### loom-md-links-fixed-prose-deferred

- Decision: this task fixes **only the dangling `builder-contract.md` links** at `manifest/designs/loom.md:91`, `:94`, and `:187`.
  The surrounding prose — the "real, separate, already-shipped sibling implementer loop" naming note and the module-decomposition table row repeating it — is left to task E.
- Rationale: `shed-followups.md:379–380` names those exact lines as E's, and `loom.md` has a declared strict chain-order owner list precisely so two tasks never fight over one file.
  But the link fix is not a matter of ownership: `builder-contract.md` ceases to exist in this commit, so leaving the links would ship three dangling references that no downstream grep would catch.
  Fixing a link is mechanical and does not collide with E rewriting the sentence around it.
- **The rule is scoped to permanently-deleted files, and `builder-contract.md` is the only one.**
  Wherever this task deletes a file **that does not come back**, it repairs every inbound link to it, even in another task's territory, and touches nothing else on those lines.
- **`plan-format.md` is explicitly excluded — its links are left to dangle.**
  `manifest/designs/shed-followups.md:183–184` records the window as deliberate: "task A deletes v2 to free the name, this task re-creates it from v3.
  Links to `plan-format.md` dangle in between, by design and briefly."
  The file returns under the same name two tasks later, so repairing those links would retarget them at `plan-format-v3.md` — which task B then renames back to `plan-format.md`, churning every link twice to land where it started.
  Worse, several of those sentences assert v2 exists **as distinct from** v3 (`model-spec.md:3`'s "Pinned alongside [plan-format v2] and the emerging [v3]"), so retargeting makes them self-duplicating rather than merely stale.
- Consequence: this task does **not** touch `manifest/designs/loom.md:29` at all.
  Its only builder-era link is a `plan-format.md` one, and `shed-followups.md:190` makes task B the mechanical owner of that line anyway ("its zero-hit criterion necessarily rewrites `loom.md:29`").
  Nor does it touch `docs/reference/plan-format-v3.md:5`'s link.
- The **prose** that asserts v2 still exists is a separate matter and remains this task's, under the v2-coexistence prose class below — the link mechanics change, the prose ownership does not.
- Rejected: fixing the prose too (violates the chain-order rule and guarantees a conflict with E);
  leaving the links (ships dangling references).

### shed-followups-inventory-repair

- Decision: alongside recording the two overrides, fix `shed-followups.md:165` (task B's file inventory lists `docs/reference/builder-contract.md`, which will not exist) and `:235` (task C's step 5 grounds itself in "`builder-contract.md`'s digest contract").
- Also record a **third override**: `shed-followups.md:388` and `:393` assign `manifest/roadmap.md:68`/`:72`'s "deferred phase slot between Builder and Finalize" to task E as "its remaining roadmap obligation".
  This task's phase rename necessarily touches the word `Builder` on that line.
  Record that A renames the phase **word** there and E's remaining obligation on the line is the slot **semantics**, so E does not find the line already gone and conclude its obligation lapsed.
- **No override is recorded for the `plan-format.md` dangling window** — this task adopts `shed-followups.md:183–184` as written rather than diverging from it (see `loom-md-links-fixed-prose-deferred`).
- Rationale: this task is already editing the file for the override note, and both sites are specs *downstream tasks will execute against*.
  A stale file inventory or a reference to a deleted grounding doc is a live defect in B's and C's instructions, not documentation drift.

### historical-records-untouched

- Decision: benchmark, research, and crucible documents are not edited (full list under Scope → Out).
- Rationale: they record what was measured or what happened on a specific date.
  `docs/benchmarks/test-suite-timing.md`'s "`internal/builderengine` | ~75.9 s" is a true statement about a run on 2026-07-12; rewriting it makes the document lie.
- Note for the reviewer: these are the only in-repo `builderengine`/`buildercli` hits that survive this task, and their exclusion is deliberate.
  The acceptance grep must exclude them explicitly rather than silently.

## Technical context

### Line numbers in the task spec have drifted — locate by content

`manifest/designs/shed-followups.md`'s inventory line numbers were captured earlier and no longer match `main`.
Confirmed drift: `docs/overview.md:264`/`:268` are actually **`:265`/`:269`**;
`CONSTRAINTS.md:205` is actually **`:219`**;
`docs/overview.md:92` is actually **`:93`**;
`roadmap.md:207` is actually **`:211`**;
`builder-contract.md`'s Webster section starts at **`:222`** (as stated).
Every site must be located by its quoted content, never by the spec's line number.

### The only compile blocker invisible to an untagged `go test ./...`

Several files import the deleted packages and break the build directly — `cmd/lyx/main.go:23`, `internal/configreg/configreg.go:10`, `cmd/lyx/notransients_test.go:21`, `cmd/lyx/constructoranchoring_test.go:34` — and are listed under Go sites below.
Those fail loudly on the first build.
The one that does not:

`internal/webstercli/sync_integration_test.go` — imports `github.com/Knatte18/loomyard/internal/builderengine` at `:20` and uses `builderengine.PauseFlagName` at `:135` and `:243`, plus `"builder"` path literals at `:123` and `:131`.
This file is **integration-tagged**, so an untagged `go test ./...` will not catch a botched edit here.
Beyond the direct importers listed above, every remaining cross-package reference is a comment.

### Go sites (non-deleted packages)

Compile-affecting:

- `cmd/lyx/main.go` — `buildercli.Command()` registration, the import, and `builder` in the long help text's module list.
- `internal/configreg/configreg.go` — the `builderengine` import and the `{Name: "builder", Template: builderengine.ConfigTemplate}` entry.
- `internal/configreg/configreg_test.go:17` — the expected module list `{"board", "builder", "burler", "fabric", "loom", "models", "perch", "reed", "shuttle", "webster"}`.
- `internal/configcli/configcli_test.go:311`, `:327–328`, `:455` — asserts the config menu prints `builder (default)` and that builder is deliberately unseeded.
  **A second, non-obvious consequence of the same one-line `configreg` edit**; it is easy to miss because the file never imports builder.
- `cmd/lyx/helptree_test.go:28` (module list) and `:106–107` (the builder help-tree case).
- `cmd/lyx/notransients_test.go:21` (import) and `:57–58` (the `builderengine.Dir` / `ReportsDir` cases).
- `cmd/lyx/constructoranchoring_test.go` — import plus `:78–79`, `:92`, `:130–131`, `:144`.
- `cmd/lyx/rawgitmutation_test.go` — the `internal/builderengine` half of `TestNoRawGitMutation_WebsterBuilderProductionSource`.
  Narrow the test to webster alone and rename it accordingly.
- `internal/loomengine/coherence.go:18` — `"builder": true` in `validPhases` (phase rename).
- `internal/loomengine/preflight_integration_test.go:611`, `:624` — `Phase: "builder"` fixtures (phase rename).
- `internal/fabricengine/weftgit_exclude_test.go` — **two different dispositions, do not treat the three lines alike**:
  - `:279`, `:280` (`.lyx/builder/run.lock`, `.lyx/builder/pause`) — **delete**.
    These are the *negative* control (machine-local artifacts that must stay untracked), and the `webster` fixtures at `:281–282` already prove it at every depth.
  - `:285` (`_lyx/<rel>/builder/state.json`) — **rename to `webster`, do not delete**.
    This is the test's only **durable positive control**, and it is asserted at **`:302–305`** (`durable := lyxRel + "/builder/state.json"`) — a line the task spec's inventory does not list.
    There is no webster durable twin; `:281–282` are `.lyx`-only.
    Deleting `:285` alone leaves the test asserting on a file it no longer writes, and deleting both loses the "committed alongside the `.lyx` artifacts … proving the property is exact and does not over-match real state" property documented at `:268–272`.
    Rename the fixture **and** its assertion together, in the same edit.

Comment-only (the "sweep everything" set — ~50 sites):

- `internal/websterengine/` — `doc.go:2`, `digest.go:4`, `poll.go:2`, `archive.go:2`, `audit.go:236`, `pause.go:1`, `fingerprint.go:2`, `config_test.go:3`, `gitwrap.go:2`, `beginbatch_test.go:7,43`, `render.go:10`, `config.go:62`, `runlevel.go:9,34,43,88,162,352`, `roles_test.go:3`, `report.go:4,44`, `strand.go:2`, `template_test.go:8,65,91`, `template.go:4`, `state_test.go:2,149`, `classify.go:2`, `state.go:13,17`, `summary_test.go:2`, `recoverbatch_test.go:7,10`.
- `internal/webstercli/` — `run.go:3,5`, `cli_test.go:3,4,12`, `pause.go:2`, `beginbatch.go:10`, `verbs_test.go:7,14`, `status.go:2`, `cli.go:6,11–12`.
- `internal/scoutcli/cli.go:127` — a **user-facing CLI help example** reading `lyx scout refs --within internal/builderengine SomeMethod`.
  Must name a package that exists.
- `internal/scoutcli/cli.go:473`, `internal/scoutcli/cli_test.go`.
- `internal/planparser/validate.go:10` — "Unlike the frozen v2 validator (`internal/builderengine/validate.go`)".
- `internal/reedengine/headertemplate.go:3` — "builderengine `*-template.md` precedent".
- `internal/pattern/leaf_enforcement_test.go:3` — the feature-package list.
- `internal/loomengine/configtemplate.go:4`, `internal/loomengine/config_test.go:5`.
- `internal/perchengine/doc.go:13` — `builder-review` (phase/gate rename).
- `internal/fabricengine/trailer_test.go:42–43`, `:54–56` — pins the `"builder: <label>"` weft commit-subject form as the `builder_style_subject` fixture, with a comment reading "the `builder: <label>`/`webster: <label>` form every builder/webster weft commit uses".
  **Not in the task spec's inventory.**
  The builder subject form dies with the module: rename the fixture to a webster-only case and narrow the comment.
- `internal/fabricengine/refscanner_test.go:20`, `:37` and `internal/websterengine/audit_test.go:25`, `:149`, `:165`, `:171–176`, `:202–203`, `:257–286` — `master-builder` / `master-builder-weft` **worktree-name** fixtures, unrelated to the builder module.
  **Leave both files**; they are named exclusions in the acceptance grep, not sweep targets.

**Bare-word `builder` provenance comments** — the class the qualified patterns cannot see, and the largest single group in the sweep.
All are `websterengine`/`webstercli` doc-comments explaining webster's mechanisms by reference to builder's, and all are rewritten to stand alone per `provenance-comments-rewritten-to-stand-alone`:
`archive.go:3–4`, `digest.go:5`, `pause.go:2–3`, `:7`, `roles.go:15`, `beginbatch.go:9`, `:35`, `:39`, `:225`, `poll.go:5`, `state_test.go:5`, `fingerprint.go:1`, `:3–4`, `recordbatch.go:9`, `recoverbatch.go:20`, `recoverbatch_test.go:368`, `outcome.go:4–7`, `runlevel.go:68`, `:70`, `:214`, `:246`, `:249`, `state.go:14`, `strand.go:4`.
Several cite the Shared Decision name `builder-is-frozen-copy-not-move` — a decision this task falsifies outright, so those sentences need their premise replaced, not just the word swapped.
This list is a starting inventory, not a bound; the bare-word acceptance scan is the actual completion criterion.
- `internal/modelspec/modelspec.go:7`, `:35` — "builder's roles", "builder, perch/burler/loom configs".
- `CONSTRAINTS.md:97` and `:106` — feature-package lists naming `builderengine`.
  `:106` mirrors `internal/pattern/leaf_enforcement_test.go:3`; keep the two consistent.
- `CONSTRAINTS.md:219` — the Fabric Git Invariant's Enforced-by block machine-checks `internal/websterengine`/`internal/builderengine` via `TestNoRawGitMutation_WebsterBuilderProductionSource`.
  Narrow to webster.
  `:219` sits inside the **Fabric Git Invariant (warp + weft)**, **not** the Review Round Invariant that follows it — do not edit the wrong invariant.
  Confirm by heading text, not line number.

### Markdown sites

- **`docs/overview.md`** — `:93` (durable-doc list, names both `plan-format.md` and `plan-format-v3.md` and `builder-contract.md`), `:228` (`internal/pattern` tree comment "consumed by builder/webster/burler/loom"), `:265` (the whole builder module-table entry), `:266` (webster defined as "fork-based sibling of builder"), `:269` (the deep link), `:273` (`Preflight → Discussion → Plan → Builder → Raddle → Finalize`), `:293` ("builder implementer" among `internal/pattern`'s prompt consumers), `:376` (the `builder-contract.md` see-also).
- **`README.md`** — `:25` (`lyx builder` in the subcommand tree), `:86` (the builder module bullet), `:87` (webster defined as "a fork-based sibling of `builder`" + "`builder` stays frozen in-tree as the plan-format-v2 consumer"), `:94` (the phase list), `:115` (module topology).
- **`docs/reference/status-schema.md`** — `:3` (the `builder-contract.md` link), `:16`, `:45`, `:53`, `:62`, `:69`, `:73`, `:81`, `:92`, `:122`, `:124` (`builder-review` in a narration example), `:127`.
  Both the enum occurrences and the prose are this task's, per the phase-rename decision.
- **`docs/reference/discussion-format.md`** — `:3` (links `plan-format.md`), `:14` (grounds the two-file split in "Builder's 'distilled digest, never raw prose' rule (see `builder-contract.md`)"), `:30` ("because `lyx builder run` can be invoked standalone, outside loom" — false once this task lands).
  Note task C also touches `:14`; C rewrites its *attribution* in producer-model terms.
  This task's job is only to stop it pointing at a deleted file — keep the edit minimal so C's rewrite is not pre-empted.
- **`docs/reference/model-spec.md`** — `:3` (the banner: "builder's roles" + "Pinned alongside [plan-format v2] and the emerging [v3]" + "land with the first consumer (`builder`)"), and the **worked example built entirely on `builder.yaml`**: `:77`, `:83`, `:87`, `:88`, `:91`, `:93`, `:105`, `:116`, `:117`.
  Rebuild the example on `webster.yaml` (`master`, `recovery`, per `internal/websterengine/template.yaml`) — note webster has only two roles where builder had four, so the example's shape changes, not just its names.
  **The task spec names only `:3`; this is the largest single doc rewrite in the task.**
  **`:105` is the exception in that list** — it sits outside the worked example, under "What is *not* a parameter", and reads "a role that needs a large window (builder's `implementer_oversized`) points at a model/variant that *has* one".
  Webster has no `implementer_oversized` analogue: forks inherit Master's model, so there is no oversized-implementer role to name.
  **Reword generically** ("a role that needs a large window points at a model/variant that has one") rather than substituting a webster role — inventing one would misdescribe webster's two-role shape to preserve a parenthetical.
- **`manifest/roadmap.md`** — `:20` (`buildercli` in the open CLI-wording question), `:30` (the follow-up-chain summary, which describes this task itself — update to match the new decisions), `:42`/`:46` ("builder implementer" template mention), `:72` ("deferred phase slot between Builder and Finalize"), `:200`, `:206`, `:211` ("Coexists with the still-live plan-format v2 — still used by the frozen `builder`").
  `roadmap.md` has two owners in chain order: this task, then E.
- **`manifest/designs/finalize.md:36`, `:50`** — deep links into `builder-contract.md#webster-the-fork-based-sibling`.
  Re-point to `webster-contract.md`'s summary-artifact section.
  **These must be re-pointed by this task before task D runs.**
- **`docs/reference/plan-format-v3.md:343`** — the fourth deep link.
- **`manifest/designs/loom.md:91`, `:94`, `:187`** — the `builder-contract.md` links only (see the deferral decision).
- **`manifest/designs/loom.md:29`** — **not this task's, in any respect.**
  Its only builder-era link targets `plan-format.md`, which is covered by the deliberate dangling window, and `shed-followups.md:190` makes task B the mechanical owner of the line.
  Listed here only so a plan writer does not "helpfully" repair it.
- **`docs/reference/status-schema.md:3`** — links **both** `builder-contract.md` *and* `plan-format.md`; the task spec names only the first.
  **Repair the `builder-contract.md` link** (permanent deletion, retarget to `webster-contract.md`);
  **leave the `plan-format.md` link dangling** (returns under task B).
  The surrounding "the loom analogue of …" prose is this task's under the v2-coexistence class.
- **`manifest/designs/review-finding-classification.md:7`, `:47`** — a `plan-format.md` / `plan-format-v3.md` pair that task B's rename would otherwise collapse into "plan-format.md / plan-format.md".
- **`manifest/designs/self-report.md:20`** — "builder/webster implementer fork".
- **`manifest/designs/hardener.md:63`** — "Discussion/Plan/Builder" (phase rename).
- **`manifest/designs/raddle.md:3`, `:7`, `:45`, `:54`** — "after Builder", "reserved-but-unbuilt phase slot between Builder and Finalize" (phase rename).
  Task D also edits `:3` and `:54`; this task changes only the phase word.
- **`manifest/designs/fabric-unified-view.md:49`** (`BuilderDir` in a list of hubgeometry path constructors), **`:133`** ("builder's pause flag").
  Note: **no `BuilderDir` symbol exists** in the tree today — `internal/hubgeometry` is gone and `internal/lyxcwd` has no builder helpers — so `:49` is already a historical statement about a past refactor.
  Treat this file as a record of a completed migration; leave it, and record the check.
- **`docs/sandbox-howto.md`** — `:8` (launcher list), `:141–147` ("Run the builder suite"), `:190` (the `SANDBOX-BUILDER-SUITE.md` see-also).
- **`docs/sandbox-hub.md:175`** — "Phased runs (Setup → Discussion → Plan → Builder → Finalize)" (phase rename).
- **`docs/skills.md`** — `:14`, `:160` ("loom Builder phase"), `:184`, `:185` ("builder/fixer prompt template").
  `:160` is a phase rename; `:14`/`:184`/`:185` name a *prompt template* that this task deletes — reattribute to webster's fork prompt.
- **`tools/sandbox/SANDBOX-BUILDER-SUITE.md`** — delete the file.
- **`tools/sandbox/SANDBOX-CORE-SUITE.md`** — scenario **S9 in full**.
  It spans **`:224` (the `### S9 -- Builder plan validate/status` heading) through its `**Verdict:**` line at `:284`, up to but not including the closing `---` at `:287`** — including the `**Covers:** builder` tag at `:229`, the plan fixture, and the `lyx builder status` / `lyx builder validate` steps at `:234–:283`.
  The task spec's `:224–232` range covers only the heading and preamble; deleting that alone orphans the scenario body.
  Delete the whole block plus one of its bracketing `---` rules so the remaining separators stay balanced.
  **Two further S9 references sit outside that span and must also go** — neither contains the word "builder", so no acceptance pattern and no test would ever catch them:
  `:99` (`- \`ref\` is the scenario id (\`S0\`-\`S6\`, \`S9\`).`) and `:306` (`S9: <OK|WARN|FAIL> -- <one-line note if not OK>` in the session-log template).
  Leaving them makes the suite instruct operators to report a scenario that no longer exists.
- **`tools/sandbox/SANDBOX-WEBSTER-SUITE.md`** — `:5` (defines webster by "mirroring `SANDBOX-BUILDER-SUITE.md`'s own operating model" and "webster is builder's fork-based sibling"), `:7` ("two scenarios, not builder's nine"), `:28` (`lyx builder` in the wired-worktree list), `:49` ("mirroring the reed/shuttle/burler/builder suites'"), `:123` ("the way builder's separate implementer strands do"), `:193` ("builder's batch-loop scenarios stay in `SANDBOX-BUILDER-SUITE.md`"), `:195` ("deliberately narrower than builder's own").
  **Not in the task spec's inventory.**
  `:193` in particular points at a file this task deletes.
  Note task B also edits this file (`shed-followups.md:168`), for the plan-format rename only — no collision.
- **`tools/sandbox/suite.go:2`, `:4`, `:54` and `tools/sandbox/main.go:6`, `:12`** — doc comments listing `builder-suite` among the suite names.
  These are separate from the code deletions already listed under Scope → In.
- **`.gitattributes:7–9`** — the three `internal/builderengine/*` LF pins.

### The `**Covers:** builder` trap

`cmd/lyx/sandbox_coverage_test.go`'s `TestSandboxCoverage_AllModulesCoveredOrExcluded` hard-fails on a `**Covers:**` token naming a module that is no longer registered.
Leaving `SANDBOX-CORE-SUITE.md`'s S9 in place therefore breaks the build **even after every other builder site is gone** — a late, confusing failure if it is the last thing found.

### Existing webster doc-comment overlap

`internal/websterengine/doc.go` already documents, in full: the fork-based loop shape, plan consumption via `planparser` as sole parser, the `status: OK|FAILED` + `head_sha` + `deviations` fork-return contract, bracket verbs, crash/resume, the integration fork + bisect, and the per-batch model assertion.
`webster-contract.md` must **not** restate any of it — it points at the package doc for internals, exactly the split `builder-contract.md` drew against builder's own package docs.

## Constraints

From `CONSTRAINTS.md` (authoritative, read every session):

- **CLI / Cobra Invariant** — builder must leave the help tree *cleanly*, not be orphaned.
  `cmd/lyx/helptree_test.go` enforces the module list and the per-module help cases; every command needs a `Short`.
- **Planparser Sole-Parser Invariant** — this task's deletion of `builderengine` is what finally makes the invariant literally true (builder held the second, v2 parser).
  Task B changes this invariant's *wording* for the renamed format; this task must not pre-empt that.
- **Sandbox Suite Coverage** — this task trips it by removing a registered module.
  Satisfying it requires deleting both `SANDBOX-BUILDER-SUITE.md` and `SANDBOX-CORE-SUITE.md`'s S9 `**Covers:** builder` tag.
- **Fabric Git Invariant (warp + weft)** — its Enforced-by block at `CONSTRAINTS.md:219` machine-checks module ownership for `internal/websterengine`/`internal/builderengine` via `cmd/lyx/rawgitmutation_test.go`'s `TestNoRawGitMutation_WebsterBuilderProductionSource`.
  Narrow that clause to webster alone.
  `:219` is inside the **Fabric Git Invariant (warp + weft)** — locate it by that heading, not by a line number.
- **Documentation Lifecycle** — `CONSTRAINTS.md:339` defers to `docs/overview.md#documentation-lifecycle`, which lists `builder-contract.md` among the durable, kept `docs/reference/` docs.
  Deleting it and adding `webster-contract.md` means `docs/overview.md:93`'s list must be updated in the same commit, or the lifecycle convention documents a file that no longer exists.
- **Cwd Resolution Invariant** — verified non-issue: `internal/lyxcwd` has no builder helpers, so deleting the packages touches no cwd resolution.
- **Task-completion doc rule** (`CLAUDE.md`) — this task changes observable CLI behavior and removes a module, so `docs/overview.md`, `README.md`, the module docs, and `CONSTRAINTS.md` all update **in the same commit**.
  `manifest/roadmap.md` moves because a planned item completes.

Discovered during discussion:

- **One commit, buildable.** No intermediate commit may leave the tree non-compiling.
- **The four deep links must be re-pointed in this task**, before B and D run.
  Task B's completion check is a zero-hit grep for `plan-format-v3`, which cannot see a dangling `builder-contract.md#…` anchor — so nothing downstream would ever find it.
- **`shed-followups.md` is a live spec for five other tasks**, not just documentation.
  Its edits in this commit are correctness fixes for B, C, and E, not tidying.

## Testing

No new tests are written — the existing suite is the test, and four guards fail loudly on a half-removal.
Expect to be *driven* by them rather than to satisfy them at the end:

- `cmd/lyx/helptree_test.go` — catches an orphaned or half-unregistered command.
- `cmd/lyx/notransients_test.go` — catches a leftover `builderengine.Dir`/`ReportsDir` transient case.
- `internal/configreg/configreg_test.go:17` — catches the module-list drift.
- `cmd/lyx/sandbox_coverage_test.go`'s `TestSandboxCoverage_AllModulesCoveredOrExcluded` — catches the `**Covers:** builder` tag.

Add to those, from this discussion:

- `internal/configcli/configcli_test.go` — the *non-obvious* second consequence of the `configreg` edit; it will fail without ever mentioning builder in an import.
- `internal/loomengine/preflight_integration_test.go` — will fail on the phase rename if `validPhases` and the fixtures diverge.

### Acceptance commands

Run all four; all must be clean:

1. `go build ./...`
2. `go test ./...` (untagged)
3. `go test -tags integration ./...` — **required**, because `internal/webstercli/sync_integration_test.go` (the one real cross-package compile blocker) is integration-tagged and invisible to the untagged run.
4. The completeness grep — this is what makes "sweep everything" checkable rather than a judgment call.
   **Three patterns, not two** — the package names alone do not cover the sweep the `sweep-everything` decision claims:
   - **Package names:** zero hits for `builderengine` and `buildercli` repo-wide.
   - **Phase/gate token:** zero hits for `builder` as a phase or gate value — `"builder"`, `phase: builder`, `builder-review`, and the `→ Builder →` phase-list form.
   - **Module word:** zero hits for the builder *module* — `lyx builder`, `builder.yaml`, `builder-suite`, `builder suite`, `SANDBOX-BUILDER-SUITE`, and `_lyx/builder` / `.lyx/builder`.
     This is the pattern that catches `tools/sandbox/SANDBOX-WEBSTER-SUITE.md`, `docs/sandbox-howto.md`, `README.md:25`, and `model-spec.md`'s `builder.yaml` example — none of which the first two patterns see.
   - **Deleted filename:** zero hits for `builder-contract` in any form.
     Without this the gate is blind to exactly the failure class this task exists to prevent — a link to a file that no longer exists, which breaks nothing and which task B's own `plan-format-v3` grep explicitly cannot see either.
     **`plan-format.md` is deliberately NOT in this pattern.** Its links dangle by design for the A→B window (`shed-followups.md:183–184`), so a zero-hit criterion on it would fail on a state this task intends to produce.
   - **Commit-subject and path fragment:** zero hits for the `builder:` commit-subject prefix and the `/builder/` path fragment.
     This catches `internal/fabricengine/trailer_test.go:42–43`, `:54–56`, which pin the `"builder: <label>"` weft commit-subject form as the `builder_style_subject` fixture — the subject form dies with the module, and no other pattern sees it.

   The `master-builder` exclusion covers **two** files, not one: `internal/fabricengine/refscanner_test.go:20`, `:37` and `internal/websterengine/audit_test.go` (11 occurrences — `:25`, `:149`, `:165`, `:171–176`, `:202–203`, `:257–286`).
   Both use it as an arbitrary *worktree name*, unrelated to the builder module.
   Leave both untouched.

   - **Bare word:** zero hits for a case-insensitive, word-boundary `builder` scan, minus the enumerated exclusions below.
     **This pattern is load-bearing, not belt-and-braces.** The five patterns above all key on a qualified form, but the single largest swept class — the ~30 provenance doc-comments in `websterengine` — writes plain "builder" ("mirroring builder's own fabric-commit-boundary discipline", "Unlike builder, only the cold recovery strand carries its own role", "webster-local copies of a builder mechanism with an in-tree builder caller").
     Without this pattern the `sweep-everything` decision's "opinion → test" claim fails for exactly the class it was written for.

   **Ordinary-English and unrelated-fixture false positives are excluded by an enumerated token list, never by judgment**, so the bare-word scan stays a mechanical gate:
   - `strings.Builder`
   - "fixture builders" (`docs/benchmarks/test-suite-timing.md:739`), "fluent builder method" (`docs/research/scout-multilang.md:29`), "content builder"
   - `master-builder` / `master-builder-weft` — unrelated worktree-name fixtures in `internal/fabricengine/refscanner_test.go` and `internal/websterengine/audit_test.go`
   - "Hub builder:" (`sandbox/build.cmd:2`)

   Enumerating the exclusions is what makes a bare-word scan usable: the list is auditable and finite, whereas "skip the ones that are obviously fine" is the judgment call this gate exists to remove.

   **Excluding** `manifest/designs/shed-followups.md`, the historical records listed under Scope → Out, and this task's own `_mill/` directory.
   The exclusion list must be written into the grep invocation explicitly, so a reviewer sees exactly what was deliberately left behind rather than inferring it.

## Q&A log

- **Q:** Where does `builder-contract.md`'s "Webster: the fork-based sibling" section go, and what do the four deep links target once Markdown can no longer anchor into it? **A:** Into a new `docs/reference/webster-contract.md`, which keeps the links clickable with a live anchor. Options putting it in `websterengine/doc.go` were rejected because a Go file has no Markdown heading anchors, so all four links would degrade to anchorless path mentions.
- **Q:** How much of `builder-contract.md` survives — full body under a Retired banner, a stub, or nothing? **A:** Nothing. "Builder" is not a thing any more, so there should be no `builder-contract.md` at all; `webster-contract.md` replaces it. The 247 lines are git-tracked regardless.
- **Q:** Is `builder-contract.md` entirely obsolete? **A:** Mostly. Its verb surface, v2 plan input, report grammar, digest contract and config keys are dead; its pause/fingerprint/archive/classify mechanics were reimplemented webster-locally and are documented in `websterengine/doc.go`. Only chain rollback and the recovery ladder have no webster equivalent — and those are accepted as lost to `git show`, named in the commit message.
- **Q:** How far does the sweep of the ~50 `builderengine`/`buildercli` references go — the spec's inventory, the false-and-unresolvable ones, or all of them? **A:** All of them. A partial sweep has no completion criterion; a zero-hit grep does.
- **Q:** How are the ~40 provenance comments handled — deleted, rewritten, or marked historical? **A:** Rewritten to stand alone, stating the reason directly without the builder reference. Deleting the clause leaves fragments; marking them historical keeps a deleted package's name in ~40 places.
- **Q:** Should the historical benchmark/research/crucible records be swept too? **A:** No — they are dated records of what was measured, falsified by editing rather than cleaned. Their exclusion is recorded explicitly so a reviewer does not file them as misses.
- **Q:** What about references to "Builder" as a loom *phase*? **A:** Rename them to Webster. Builder is the implementer module being parked; Webster is the implementer module taking over the whole implementer role, and the phase is named after the module that runs it. This includes the `builder-review` gate name.
- **Q:** The phase rename includes `validPhases` and the `status-schema.md` enum, which `shed-followups.md:88–94` explicitly defers to the `Shed` build task. Override it? **A:** Yes — and update `shed-followups.md` in the same commit so E and the `Shed` build task are not working from a stale ownership claim. Renaming the prose while leaving the validator would ship docs that contradict live code.
- **Q:** `weftgit_exclude_test.go` writes `_lyx/builder/` fixture dirs, and `perchengine/doc.go` names a `builder-review` gate. Rename or delete? **A:** Rename the gate to `webster-review`. The fixture dirs split two ways, **not** all-delete as first answered: delete `:279`/`:280` (the `.lyx` negative controls, already proven by the adjacent `webster` fixtures), but **rename** `:285` to webster together with its assertion at `:302` — it is the test's only durable positive control, and deleting it leaves the test asserting a file it never writes.
- **Q:** Should the sweep be scripted? **A:** Split by kind — script the phase rename (a true rename, grep-verifiable), hand-rewrite the ~40 comments (a rewrite no regex can produce), and gate both halves with the same repo-wide zero-hit grep.
- **Q:** What is the verification bar? **A:** `go build ./...`, untagged `go test ./...`, `go test -tags integration ./...` (required — the one real compile blocker is integration-tagged), plus the zero-hit completeness grep with its exclusions written out explicitly.
- **Q:** `loom.md:91`/`:94`/`:187` are task E's per the spec, but deleting `builder-contract.md` leaves their links dangling rather than merely stale. **A:** This task fixes the links only; the prose stays E's. The chain-order ownership rule protects against two tasks rewriting the same sentence, not against one task repairing a reference to a file it deleted.
- **Q:** Does that link-repair rule extend to `plan-format.md`, which this task also deletes? **A:** No — and an earlier round of this discussion had it wrong. `shed-followups.md:183–184` records the A→B window where `plan-format.md` does not exist as **deliberate**: task B re-creates the file under the same name, so repairing the links would retarget them at `plan-format-v3.md` only for B to rename that back to `plan-format.md`. The rule is therefore scoped to permanently-deleted files, of which `builder-contract.md` is the only one.
- **Q:** Then what happens to the sentences asserting v2 exists alongside v3? **A:** Unchanged ownership — they are the v2-coexistence prose class, already this task's. Only the link mechanics differ: the prose is rewritten, the link is left to dangle briefly.
