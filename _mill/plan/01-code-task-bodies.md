# Batch: code-task-bodies

```yaml
task: "Scope the Shed producer-model rewrite into buildable tasks"
batch: "code-task-bodies"
number: 1
cards: 3
verify: null
depends-on: []
```

## Batch Scope

This batch authors the three code-touching follow-up task bodies — A (`builder-retire`), B (`plan-format-drop-v3-suffix`), and F (`batcher-standalone-split`) — as staged files under `_mill/followup/`.
It is one batch because all three are transcribed from the same source section of `_mill/discussion.md` (`### follow-up-task-set`) and share one output format, so a single session holds the whole ownership map — which task owns which file, and which sites are deliberately excluded — in its head at once.
The external interface batch 3 consumes is the staged-file format fixed in `## Shared Decisions`: a leading fenced yaml header with `slug`/`title`/`brief`/`depends_on`, then the body verbatim.
No batch-local decisions differ from the overview.

## Cards

### Card 1: Body for task A — builder-retire

- **Context:**
  - `_mill/discussion.md`
  - `_mill/plan/00-overview.md`
- **Edits:** none
- **Creates:**
  - `_mill/followup/A-builder-retire.md`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Write the staged file for follow-up task A.
  The fenced yaml header carries `slug: builder-retire`, a double-quoted `title` of "builder: delete internal/builderengine and internal/buildercli, retire builder-contract.md as a reference", `depends_on: []`, and a one-paragraph `brief` block scalar naming the deletion, the doc retirement, and the fact that builder is live-registered today rather than dormant.
  `## Why` transcribes the Decision `builder-is-deleted-contract-doc-is-retired-not-code` from `_mill/discussion.md`: builder is not dormant (cmd/lyx/main.go:107 registers buildercli.Command(), and it appears in cmd/lyx/helptree_test.go's module list and cmd/lyx/notransients_test.go), so parking it in-tree costs recurring maintenance — it stays in the CLI help tree per the CLI / Cobra Invariant, keeps a second plan parser alive against the Planparser Sole-Parser Invariant, and every future refactor must carry it.
  Record that the reusable asset is the design — the recovery ladder, chain rollback, the mutate.lock state-mutation lease, the three fabric-commit points, and crash/resume semantics — which already lives in builder-contract.md's 247 lines and is rewritable onto the flat card list later, while the implementation stays one `git show` away.
  Carry both rejected alternatives (keeping the code frozen in-tree; moving it to a sandbox/ or attic/ directory that invents an excluded-directory convention this repo does not have) and the supporting roadmap.md:196 / :202 citations.
  `## What needs to happen` is a numbered list whose steps transcribe, in order and without dropping a bullet, the *Code deletion*, *Config disposition*, *Doc retirement*, comment-only-residue, and v2-coexistence-prose-class inventories from the **A — `builder-retire`** subsection of `_mill/discussion.md`'s `### follow-up-task-set`.
  Preserve every file path and line citation exactly as the discussion states them.
  Three points from that inventory must appear as explicit, standalone statements rather than being folded into a bullet list, because each one is a trap a fresh session would otherwise walk into:
  the four deep links into builder-contract.md's "Webster: the fork-based sibling" section are re-pointed by A **before** B runs, since B's zero-hit grep cannot catch a dangling anchor;
  tools/sandbox/SANDBOX-CORE-SUITE.md's S9 scenario and its `**Covers:** builder` tag must go, because cmd/lyx/sandbox_coverage_test.go's drift guard hard-fails on a `**Covers:**` token naming an unregistered module even after every other builder site is gone;
  and existing builder.yaml files in already-created worktrees are left in place, inert, because reconcile does not delete files it no longer owns.
  Add a short subsection recording what A does **not** own: the phase enum in internal/loomengine/coherence.go and its twin in docs/reference/status-schema.md are deliberately left alone and realign with the Shed build — transcribe the Decision `phase-enum-realignment-is-deferred-to-the-shed-build`, including its "What is not deferred" note that status-schema.md's builder-specific prose and its builder-contract.md link *are* A's.
  `## Scope` states that A is one task producing one compiling commit, because a package deletion is atomic by nature and splitting it guarantees an intermediate state that does not build.
  It also states A's ownership rule for the v2-coexistence prose class — A owns every site whose claim A itself falsifies — and names the two exclusions: plan-format-v3.md:5's own "Coexistence, not replacement" section belongs to C, and loom.md:29 belongs to E.
  Note that manifest/roadmap.md has two owners, A then E, in chain order rather than concurrently.
  `## Sequencing` records `depends_on:` nothing, and names the two tasks that depend on A and why: B, because the plan-format.md filename is not free until v2's doc is deleted, and D, because finalize.md:36 and :50's link targets move in A.
  `## Acceptance` transcribes A's bullet from `_mill/discussion.md`'s `## Testing` section: the existing suite is the test, `go build ./...` and `go test ./...` must pass with builderengine and buildercli gone, no new tests are written, and four guards fail loudly on a half-removal — cmd/lyx/helptree_test.go, cmd/lyx/notransients_test.go, internal/configreg/configreg_test.go:17's module-list assertion, and cmd/lyx/sandbox_coverage_test.go's TestSandboxCoverage_AllModulesCoveredOrExcluded.
  Name the four CONSTRAINTS.md invariants A must satisfy, as the discussion's `## Constraints` section states them: CLI / Cobra Invariant, Planparser Sole-Parser Invariant, Sandbox Suite Coverage, and the Fabric Git Invariant (warp + weft) clause at CONSTRAINTS.md:205 that must be narrowed to webster alone — including the discussion's clarification that :205 sits inside that invariant, which begins at :173, not inside the Review Round Invariant, which begins at :209.
  Also name the Documentation Lifecycle, which governs the extraction of the Webster section into internal/websterengine's package doc before builder-contract.md is retired.
- **Commit:** `scoping: stage follow-up task body A (builder-retire)`

### Card 2: Body for task B — plan-format-drop-v3-suffix

- **Context:**
  - `_mill/discussion.md`
  - `_mill/plan/00-overview.md`
- **Edits:** none
- **Creates:**
  - `_mill/followup/B-plan-format-drop-v3-suffix.md`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Write the staged file for follow-up task B.
  The header carries `slug: plan-format-drop-v3-suffix`, a double-quoted `title` of "plan-format: drop the v3 suffix and sweep every reference by script", `depends_on: ["builder-retire"]`, and a one-paragraph `brief` naming the rename, the mechanical-sweep discipline, and the zero-hit grep criterion.
  `## Why` transcribes the Decision `plan-format-v3-renamed-to-plan-format-mechanically`: v3 is the only live format once builder is gone, and a version suffix on the sole format is the kind of stale guard discussion-format.md already argues against via its no-schema-version reference to status-schema.md.
  State the failure mode the decision is guarding against — a half-done rename is worse than either end state, because planparser and websterengine identifiers and template prose must move with the filename — and carry the two rejected alternatives (a docs-only rename with Go identifiers deferred; renaming the file but keeping in-text "v3" as a historical label).
  `## What needs to happen` opens with the step the discussion insists on: B's first action re-derives the affected file list by grep rather than trusting any count written down beforehand.
  Then list the affected clusters exactly as the discussion names them — internal/planparser, internal/websterengine, internal/webstercli, internal/loomengine including plan-template.md, internal/batcher/doc.go, docs/overview.md, docs/reference/model-spec.md, docs/reference/builder-contract.md, manifest/roadmap.md, several manifest/designs/*.md files, and tools/sandbox/SANDBOX-WEBSTER-SUITE.md — plus CONSTRAINTS.md's Planparser Sole-Parser Invariant, whose wording changes for the renamed format.
  Present the file list as a starting inventory and say so, since the grep is what bounds it.
  Two rules must appear as their own standalone subsections, not as bullets, because both are build-breaking if missed:
  the hard exclusion of the `gopkg.in/yaml.v3` import token, which appears in ten Go files including internal/planparser/parse.go:21 — the sole plan parser, i.e. the file B is most certain to be editing — so B's script names the exclusion explicitly rather than relying on the pattern set being narrow enough;
  and the execution discipline — a scripted find/replace followed by a full `go test ./...`, never a hand-edit pass, and per this repo's own tooling rules the script must not use `sed`.
  Record the deliberate window between A and B where docs/reference/plan-format.md does not exist at all: A deletes v2 to free the name, B re-creates it from v3, and links to plan-format.md dangle in between, by design and briefly.
  Record what B deliberately leaves broken: B's rewrite of loom.md:29 knowingly leaves that sentence self-contradicting, because a pure find/replace cannot repair an argument about two formats when only one survives.
  State plainly that this is accepted, not overlooked — B's grep criterion passes while the sentence reads wrong, and E repairs the prose as loom.md's final owner.
  `## Scope` states B's position in loom.md's three-owner chain B → C → E: B is the *mechanical* owner, because its zero-hit criterion necessarily rewrites loom.md:29 and table rows 5–7 at :53–55, which spell plan-format-v3.md.
  Say explicitly that B changes paths and names only, never prose, and that because B runs before both C and E this is chain order rather than concurrency — no two owners hold the file at once.
  `## Acceptance` states the completion criterion as the discussion's own **case-insensitive** repo grep returning zero hits for the full pattern set — `plan-format-v3`, `plan_format_v3`, `plan-format v3`, `plan-v3`, and `Plan-format v3` — plus a passing `go test ./...`.
  Include the discussion's reason the narrower three-pattern set was rejected: it would leave loom.md:58's "plan-v3's card contract", loom.md:94's "Webster/plan-v3 equivalent", and internal/planparser/doc.go:32's "Plan-format v3" all passing while contradicting the decision's own intent.
  Note that internal/planparser's existing tests and internal/webstercli/cli_test.go cover behaviour preservation, and that the meaningful failure mode is incompleteness, checked by grep rather than by an assertion in a test file.
  `## Sequencing` records `depends_on: builder-retire` with its reason (the filename is not free until v2's doc is deleted) and names C and F as the two tasks that depend on B because both edit the renamed file.
- **Commit:** `scoping: stage follow-up task body B (plan-format-drop-v3-suffix)`

### Card 3: Body for task F — batcher-standalone-split

- **Context:**
  - `_mill/discussion.md`
  - `_mill/plan/00-overview.md`
- **Edits:** none
- **Creates:**
  - `_mill/followup/F-batcher-standalone-split.md`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Write the staged file for follow-up task F.
  The header carries `slug: batcher-standalone-split`, a double-quoted `title` of "batcher: split out of webster into a standalone configreg module with its own batcher.yaml", `depends_on: ["plan-format-drop-v3-suffix"]`, and a one-paragraph `brief` naming the config move, the two call sites, and the report-don't-honour migration rule.
  `## Why` transcribes two Decisions from `_mill/discussion.md` together, because neither stands alone: `batchifier-splits-out-of-webster` establishes that Batchifier is a Shed-level producer at position 8 in loom's list, between Plan-Review and Webster, so batching is no longer webster-internal execution policy;
  `batcher-extracts-standalone-now-absorbed-by-shed-later` establishes the two-step consequence — extract batcher as a standalone module now, and absorb the Batchifier producer into loom via Shed later, when the batchifier choice becomes part of loom's producer-list configuration.
  Carry the reason the key cannot go straight to loom.yaml today: both live `batcher.Select` call sites are webster's, and Shed — the thing that would own a loom.yaml batchifier key — does not exist, so making webster read loom.yaml would either break standalone `lyx webster run` or couple two modules' configs for no live benefit.
  Carry the rejected alternatives from both decisions, including the loom.yaml-key-with-webster.yaml-fallback option and why it was rejected (a transition mechanism with no transition to serve).
  `## What needs to happen` is a numbered list with four parts, each transcribed from the **F — `batcher-standalone-split`** subsection.
  Part one, module shape: "standalone" means a configreg-registered config module, not a `lyx batcher` command, because a batchifier has no user-facing verb.
  State both consequences explicitly, since the opposite was briefly assumed — the CLI / Cobra Invariant does not apply because nothing is registered on the cobra root, and Sandbox Suite Coverage does not apply because cmd/lyx/sandbox_coverage_test.go:38–47 enumerates `newRoot().Commands()`, i.e. cobra registration rather than configreg, so adding a `**Covers:** batcher` tag would actively fail that test's drift assert.
  Part two, config wiring: internal/batcher gains the loading of batcher.yaml and exposes an entry point returning the active `Batcher` — the natural extension of the `Select`-by-name seam it already has — and `websterengine.Config.Batcher` is therefore removed, not retained.
  Say why the earlier "retained" note is superseded: retaining the field would leave webster holding a yaml key it no longer owns, and populating it from batcher.yaml would be exactly the cross-module config coupling the loom.yaml option was rejected for.
  Part three, the inventory: transcribe all seven sites the discussion lists — internal/websterengine/runlevel.go:332, internal/webstercli/cli.go:160 (the `PersistentPreRunE` fail-fast gate, whose behaviour is preserved and only whose source changes), internal/websterengine/template.yaml:3 where the `batcher: ""` key physically lives with its explanatory comment, internal/webstercli/verbs_test.go:221–223 plus the whole gate-test pair at :696–732, internal/websterengine/doc.go:12 and :25–27, docs/overview.md:267, and internal/websterengine/config_test.go:125's `cfg.Batcher == "identity"` assertion, which moves into internal/batcher's own tests with the field.
  State why the gate-test pair is taken whole rather than as the :697 comment alone: both `TestPersistentPreRunE_UnknownBatcherFailsFast` and `TestPersistentPreRunE_DefaultBatcherResolves` string-replace `batcher: ""` out of the template, so both break the moment the key leaves it.
  Part four, migration: reconcile reports a leftover webster.yaml `batcher:` value once as an orphaned key and otherwise ignores it — never silently dropped, never read.
  Carry the rationale (honouring it would reinstate the cross-module read this split exists to remove, invisibly, so two worktrees with identical batcher.yaml files could batch differently) and both rejected alternatives (honouring the old key as a fallback; silently ignoring it).
  Add the doc-amendment list as its own numbered step: internal/batcher/doc.go's package comment, which must stop saying batching is "100% webster's own execution-policy decision" and instead say it is a standalone step webster consumes today and Shed will drive as producer #8 once built;
  CONSTRAINTS.md's Batcher Registry+Config Invariant, both the ownership claim and the webster.yaml config-key pin;
  docs/overview.md:271's batcher module-table entry, which pins the key to webster.yaml;
  and the renamed plan-format.md's "Batch is gone / the card is the unit" section, where the card stays the plan's unit but the "entirely internal to webster" framing goes.
  `## Scope` states what F does not change: the `Batcher` interface, the registry, and `Select` itself stay untouched — what changes is where the name fed to `Select` is configured, plus the module's registration and docs.
  State that F does not edit loom.md; row 8 of loom.md's producer table is E's, written after F lands.
  `## Sequencing` records `depends_on: plan-format-drop-v3-suffix`, because F edits the renamed file, and records that E depends on F in turn, since loom.md:56's row 8 must reflect whatever F lands.
  `## Acceptance` transcribes F's bullet from `_mill/discussion.md`'s `## Testing` section: the config relocation is the only behavioural change, and the TDD candidates are a test asserting the active batchifier resolves from batcher.yaml through batcher's own entry point, a configreg test asserting batcher appears in the module list mirroring internal/configreg/configreg_test.go:17's existing shape, and a migration test covering an existing worktree whose webster.yaml still carries a `batcher:` value.
  State that internal/batcher's existing registry and `Select` tests must pass untouched, since that is the evidence that only the configuration source moved and not the batching itself.
  Name the Cwd Resolution Invariant as relevant if the config-key move touches path resolution.
- **Commit:** `scoping: stage follow-up task body F (batcher-standalone-split)`

## Batch Tests

`verify: null`.
This batch creates three markdown files and runs no code — there is no runnable surface to assert against.
Correctness is "does each body carry the discussion's decisions completely and without contradiction", which is a review judgement, not a test.
The staged files' machine-readable half — the fenced yaml header — is exercised in batch 3, which parses all six files and fails on a malformed header before it reaches the wiki.
