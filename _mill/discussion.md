# Discussion: PATTERN wiring: conditional constraint-injection into every agent

```yaml
task: 'PATTERN wiring: conditional constraint-injection into every agent'
slug: pattern-wiring
status: discussing
parent: main
```

## Problem

Loomyard has no invariants mechanism of its own. The `CONSTRAINTS.md` at this repo's root is **Millhouse's** artifact — it lives here only because mill is the tool currently developing loomyard, and mill tooling plus `CLAUDE.md` read it every session. When lyx is initialized in some other repo (a consulting host repo, or loomyard developing itself onto `loom`), there is nothing that carries that repo's føringer to the agents lyx spawns there.

PATTERN is that carrier: a weft-resident `_pattern/` directory whose index file `_pattern/PATTERN.md` every code-touching agent is told to read before it writes or reviews code. This task builds **only the wiring** — the plumbing that makes an eventual `PATTERN.md` reach every agent. It is loom-independent and buildable today because it rests entirely on already-shipped `internal/stencil`, `internal/hubgeometry`, `internal/fabricengine` and `internal/initengine`.

**Why now:** the wiring has no dependency on any unbuilt module, and building it now lets the mechanism ship and be tested against a fixture `PATTERN.md` long before the real content migration. The migration itself happens only at loomyard-init-via-lyx and is explicitly out of scope here. Full background: `manifest/designs/pattern.md`.

## Scope

**In:**

- A new leaf package `internal/pattern` exposing `Directive(worktreeRoot string, role Role) string` — the shared active-check plus directive text, returning the empty string when PATTERN is inactive.
- A `stencil` optional-marker extension: `FillOptional(template []byte, values map[string]string, optional []string) ([]byte, error)`, with `Fill` reduced to `FillOptional(t, v, nil)`.
- A `{{.pattern_directive}}` marker added to exactly five existing templates: `internal/builderengine/implementer-template.md`, `internal/burlerengine/review-prompt-template.md`, `internal/websterengine/fork-template.md`, `internal/websterengine/master-template.md`, `internal/loomengine/plan-template.md`.
- Plumbing the computed directive into those five prompt-assembly sites.
- New `hubgeometry` geometry surface for `_pattern`: `PatternDirName`, `PatternDir(baseDir)`, `PatternFile(baseDir)`, `HostPatternLink(slug)`, `WeftPatternDirFor(slug)`; `_pattern` added to `IsReservedHubName`, to `HostJunctions()`, and to the enforcement test's geometry-token list.
- Generalising `fabricengine`'s unwire path from its hardcoded single-`_lyx` shape (`unseedLyxJunction`, `UnwireResult.JunctionRemoved`) to a per-junction shape, since `_pattern` is the first second junction.
- Extending `fabricengine/status.go`'s host-pollution scan to `_pattern`.
- `initengine.Init` creating the `_pattern/` directory through the junction, exactly as it does for `_lyx`.
- Docs in the same commit: `CONSTRAINTS.md` (new Pattern Leaf Invariant; `_pattern` added to the Hub Geometry Invariant's token list), `docs/overview.md` module table, and `manifest/designs/pattern.md` corrections.

**Out:**

- **All content migration.** No loomyard invariant moves out of `CONSTRAINTS.md`. No real `_pattern/PATTERN.md` is authored in this repo. `CONSTRAINTS.md` stays the single live invariants doc while mill develops loomyard.
- **The `_pattern/<topic>/` detail-submap layout.** That is a content-authoring decision belonging to the migration, not the wiring.
- **The three existing hardcoded `CONSTRAINTS.md` mentions** in `loomengine/discussion-template.md:28`, `loomengine/plan-template.md:19` and `websterengine/master-template.md:25`. They stay exactly as they are; the PATTERN directive is added alongside them, not in place of them. Replacing them is migration work.
- **Templates that are not code-touching:** `websterengine/integration-template.md`, `builderengine/orchestrator-template.md`, `loomengine/discussion-template.md`, all four `treadleengine` judge/triage/targeting templates, `reedengine/header-template.md`.
- **Machine enforcement that an agent actually read PATTERN.md.** No report-echo gate, no verify-command hook.
- **`internal/gitrepo`.** Untouched, which is what keeps this task parallel-safe with the concurrent `native-clients` task.

## Decisions

### stencil-optional-marker

- **Decision:** add `stencil.FillOptional(template []byte, values map[string]string, optional []string) ([]byte, error)`. `Fill(t, v)` becomes exactly `FillOptional(t, v, nil)`, so all ten existing call sites are unchanged in behaviour and signature. A name listed in `optional` is exempt from **both** guards: it is skipped by `unfilledTopLevelMarkers`, and its absence from `values` must not trigger `missingkey=error` during execution. An optional marker that is absent, empty, or whitespace-only renders as nothing.
- **Rationale:** the whole point of the mechanism is that inactive PATTERN renders *nothing*, in five templates, silently and correctly. Declaring that in Go next to the helper call makes it testable per call site. "Optional" must mean optional — if a caller had to remember to supply an empty string, a single forgotten key would become a runtime failure in all five agents, which is precisely the fragility the optional concept exists to remove.
- **Rejected — reuse the existing `{{if}}` idiom (no stencil change at all).** This *would work*: `unfilledTopLevelMarkers` (`internal/stencil/stencil.go:96`) walks only `t.Tree.Root.Nodes`, so a marker inside `{{if}}` is never checked, and `websterengine/fork-template.md:30` already does exactly this in production for `{{.rename_mechanic}}`. **The design doc's claim that PATTERN is "the first genuinely-conditional token in the system" is factually wrong** — `rename_mechanic` came first, and stencil strictly speaking needs no extension. It was rejected on four grounds: (1) optionality would live only in markdown, so a later cleanup that unwraps the `{{if}}` silently makes the marker required again, failing in production the moment PATTERN is inactive; (2) a typo in the `{{if}}` condition yields a permanently-empty block that nothing detects; (3) every template banner in the repo states "there are no `{{if}}`/`{{range}}` conditionals anywhere in this file" and explains why — adding conditionals to five more templates erodes a rule that is currently uniform, with webster's fork template as the single documented, self-justifying exception; (4) the directive is a multi-line bullet block, so `{{if}}`-wrapping means fiddly whitespace management in five files.
- **Rejected — a naming convention** (a marker spelled `Opt*` is exempt): couples semantics to spelling, and gives no place to assert the name in a test.

### pattern-package-home

- **Decision:** a new leaf package `internal/pattern`, importing only the standard library and `internal/hubgeometry`. It carries its own Leaf Invariant enforcement test and a corresponding `CONSTRAINTS.md` entry, mirroring `internal/modelspec`, `internal/tokenvocab` and `internal/codeintelengine`.
- **Rationale:** the helper is imported by four feature packages (`builderengine`, `websterengine`, `burlerengine`, `loomengine`). Only a leaf can be imported that widely without cycle risk, and the repo already has three precedents with exactly this shape.
- **Rejected — put it in `internal/stencil`:** stencil is a pure text leaf with no filesystem or geometry dependency; an `os.Stat` there destroys what makes it trivially testable.
- **Rejected — put it in `internal/fabricengine`:** fabricengine owns the junction primitive, but importing it into builder/webster/loom/burler is a heavy, wrong-direction dependency.

### active-check-semantics

- **Decision:** PATTERN is active **iff `_pattern/PATTERN.md` exists**. Pure existence check, nothing more. The `_pattern/` directory may exist without the file — that is the normal inactive state, since `initengine` always creates the directory. A `PATTERN.md` that exists but is **empty** counts as **active**.
- **Rationale:** the file's presence is the whole switch, with no "PATTERN not configured" branch anywhere. This is what the design doc specifies and it is the cheapest thing to reason about.
- **Consequence, accepted knowingly:** an empty `PATTERN.md` injects a directive pointing at an empty file. Degenerate, but harmless, and preferable to either a content-inspecting check (which turns a benign empty file into a runtime error in five agents) or a size heuristic.
- **Rejected — treat an empty file as inactive:** would catch a half-finished migration, but makes the switch two-valued for no real benefit.
- **Rejected — treat an empty file as an error:** turns a benign state into a hard failure across every agent.

### directive-shape-and-wording

- **Decision:** two role variants — implementer-flavoured and reviewer-flavoured — each an imperative markdown checklist block, not a single sentence. The block **carries its own `##` heading inside the marker value**, so that when PATTERN is inactive the marker renders as nothing at all and no orphan heading is left behind.
- **Rationale:** an imperative gated checklist is measurably more effective in agent prompts than a lone declarative sentence. The two variants exist because the operative verb genuinely differs: an implementer must not *write* a violation, a reviewer must *block* on one.
- **Implementer variant** (used by builder implementer, webster fork, webster Master, loom plan):

  ```markdown
  ## Constraints — do this before you write any code

  - [ ] **STOP.** Before you edit a single file, read `_pattern/PATTERN.md` in full.
  - [ ] Read every detail doc under `_pattern/` that PATTERN.md points to and that touches what you are about to change.
  - [ ] These constraints are BINDING. A change that violates one is wrong even if the verify command passes.
  - [ ] If a constraint conflicts with anything else in this prompt, the constraint wins — say so in your report instead of silently picking one.
  ```

- **Reviewer variant** (used by the burler round):

  ```markdown
  ## Constraints — do this before you judge anything

  - [ ] **STOP.** Before you form any judgment, read `_pattern/PATTERN.md` in full.
  - [ ] Read every detail doc under `_pattern/` that PATTERN.md points to and that touches the target.
  - [ ] Every violation of a listed constraint is a BLOCKING finding. Record it, no matter how small it looks.
  - [ ] Never wave a violation through because the code works or the tests pass.
  ```

- The directive **injects a pointer, never the constraints inline** — the agent reads the file itself, so prompt size is constant regardless of how large PATTERN grows.
- **Rejected — one role-invariant sentence:** simpler, but demonstrably weaker as an instruction, and cannot express the reviewer's blocking obligation.
- **Rejected — a role-parameterised template string** (`Directive(root, role)` composing text from parts): more surface for no proven need. Two literal constants selected by a small `Role` enum is enough.

### template-set

- **Decision:** the marker goes into exactly five existing templates. None are created; all five already exist.
  - `internal/builderengine/implementer-template.md` — implementer variant
  - `internal/burlerengine/review-prompt-template.md` — reviewer variant
  - `internal/websterengine/fork-template.md` — implementer variant
  - `internal/websterengine/master-template.md` — implementer variant
  - `internal/loomengine/plan-template.md` — implementer variant
- **Rationale:** these are the templates whose agents write or review code. Webster **Master** is included even though `manifest/designs/pattern.md` does not list it, because every fork inherits Master's context and Master already reads `CONSTRAINTS.md` in full (`master-template.md:25`) — it is the cheapest, highest-leverage injection point in webster. The design doc's "reviewer" role has no separate template; it *is* the burler round.
- **Excluded, with reasons:** `websterengine/integration-template.md` (runs the verify once, implements no cards, makes no commit — "not even for a trivial, obviously-correct fix"); `builderengine/orchestrator-template.md` ("You never edit code yourself"); `loomengine/discussion-template.md` (interview phase, writes a decision record, not code); the four `treadleengine` templates (judge/triage/targeting — judgment only); `reedengine/header-template.md` (not an agent prompt at all).

### marker-name

- **Decision:** `{{.pattern_directive}}`, snake_case.
- **Rationale:** matches every existing marker in the repo — `rename_mechanic`, `shared_decisions`, `worktree_root`, `batch_file`, `report_path`, `self_fix_cap`, `prev_digest`, `decision_record_path`. The design doc's `{{.PatternDirective}}` matches no existing marker.

### marker-placement

- **Decision:** the marker sits as early as possible in each template — immediately after the opening role paragraph, **before the first concrete work instruction**. Concretely: for `master-template.md` next to the existing `CONSTRAINTS.md` paragraph at line 25; for `plan-template.md` at the "Step 2 — Explore the codebase" boundary near line 19; for the other three, immediately after the opening role statement and before the first task step.
- **Rationale:** a "STOP, read this first" gate only works if it precedes the description of the work. A uniform placement rule is also something a reviewer can check.
- **Rejected — placing it just before the report/deliverable section:** closer to where code is written, but the agent has already absorbed the whole task by then, which is exactly what the gate is meant to precede.
- **Rejected — choosing per template during implementation:** no shared rule to review against.

### junction-ownership

- **Decision:** `_pattern` becomes a real host junction into weft, exactly like `_lyx`. Responsibility splits as follows, which **corrects `manifest/designs/pattern.md`**:
  - `hubgeometry` **declares** the junction record and owns every `_pattern` path literal.
  - `fabricengine.WireJunctions` / `seedLyxJunction` **need no change** — they already iterate `l.HostJunctions(slug)`.
  - `initengine.Init` is the **caller** that creates it, and must also `MkdirAll` the `_pattern` directory through the junction, mirroring what it does for `_lyx`.
  - `fabricengine`'s unwire path must be generalised (see next decision).
- **Rationale:** the design doc says "the `_pattern` junction is `fabric`'s responsibility, exactly like `_lyx`/`_raddle`", which is imprecise in two ways. First, **`fabric add` deliberately does not wire junctions** — `add.go:295` states outright: *"Add does not wire the host `_lyx` junction (it is dormant) … The junction is wired by lyx init via `WireJunctions`."* `initengine.Init:67` is the sole production caller alongside `checkout.go:152` and `reconcile.go:153` (both re-point existing wiring). Second, **`_raddle` has no junction at all today** — `status.go:194` says *"no junction is wired for `_raddle` in this release"* — so `_pattern` is not joining an established plural pattern; it is creating one.
- **Rejected — weft directory only, no host junction:** the directive would have to name an awkward cross-tree path, and it breaks the `_lyx` sibling symmetry the design depends on.

### unwire-generalisation

- **Decision:** generalise `fabricengine.unseedLyxJunction` to iterate `l.HostJunctions(slug)`, and replace `UnwireResult.JunctionRemoved bool` with a per-junction shape (e.g. `JunctionsRemoved []string`, or a count). `initengine.undo` is updated to match.
- **Rationale:** the code flags this itself. `junction.go:179-183` reads: *"It is deliberately scoped to the single `_lyx` junction … rather than iterating `l.HostJunctions(slug)` the way `unseedGitExclude` does: `HostJunctions` returns exactly one entry today, and `UnwireResult.JunctionRemoved` is a single bool by design to match. **If `HostJunctions` ever grows a second entry, this function and `UnwireResult` should be revisited together.**"* This task is that event. Note `unseedGitExclude` already iterates correctly and needs no change.
- **Rejected — add the junction but leave `UnwireResult` single-bool:** smaller diff, but knowingly leaves the unwire path wrong for the second junction, against the code's own written instruction.

### no-machine-gate

- **Decision:** the checklist is prompt text only. No verification that the agent actually read `PATTERN.md`.
- **Rationale:** consistent with how every other instruction in these prompts is handled today. Nothing else in the repo machine-verifies prompt compliance.
- **Rejected — a report echo** (implementer/burler must write a `pattern-read:` line that Go validates): touches the report parsers in builder, webster and burler, and an agent can write the line without having read the file, so it buys the appearance of enforcement rather than enforcement.
- **Rejected — gating in the verify command:** the verify command is owned by the plan, not by PATTERN.

### call-site-plumbing

- **Decision:** `internal/pattern.Directive` is called **one level up** from the pure string-composing functions, at the layer that already holds a `*hubgeometry.Layout` or a worktree root. The already-computed directive string is then passed **down** as an ordinary parameter. `burlerengine.composePrompt` and `loomengine.composePlanPrompt` receive a `patternDirective string`, **not** a root path.
- **Rationale:** both are pure string functions today (`composePrompt(p *Profile)`, `composePlanPrompt(decisionRecordPath, planDir, overviewPath string)`) with no filesystem access, and keeping them pure keeps them trivially testable. Every call site already has what it needs one level up.
- **Concrete availability at each of the five sites** (verified during exploration):
  - `builderengine/spawn.go:471` — `deps.WorktreeRoot` already in the values map.
  - `websterengine/render.go:123` (`RenderForkPrompt`) — `worktreeRoot` already a parameter.
  - `websterengine/render.go:306` (`RenderMasterPrompt`) — **no worktree root parameter today**; needs one added. Its caller `runlevel.go:502` has `deps`, which carries `WorktreeRoot` (used at `beginbatch.go:212`).
  - `burlerengine/prompt.go:22` (`composePrompt`) — caller `engine.go:102` has `e.layout.WorktreeRoot` (already used at `engine.go:98`).
  - `loomengine/plan.go:42` (`composePlanPrompt`) — caller `PlanSpec` at `plan.go:87` takes `layout *hubgeometry.Layout` directly.

## Technical context

### `internal/stencil` — the fill leaf

`internal/stencil/stencil.go` is 129 lines, standard-library only, and has one exported function. Its guarantee: `unfilledTopLevelMarkers` (line 87) walks **only** `t.Tree.Root.Nodes` — depth 0 — and collects every bare `{{.X}}` whose value is absent, empty, or whitespace-only, reporting all of them in one sorted error before executing anything. Markers reached only inside a taken `{{if}}`/`{{with}}`/`{{range}}` branch are instead caught at execution time by `Option("missingkey=error")` (line 43), which fires only on an **absent** key — a present-but-empty branch-internal value renders as a silent blank.

`FillOptional` must therefore do two things, not one: skip listed names in `unfilledTopLevelMarkers`, **and** ensure a listed name that is absent from `values` does not trip `missingkey=error`. The simplest correct implementation is to copy `values` and seed every listed-but-absent optional name with `""` before executing, leaving the caller's map untouched.

`stripLeadingComment` (line 70) drops a leading `<!-- … -->` banner before parsing. Every template asset starts with one; those banners must be updated (see Testing).

### `internal/hubgeometry` — the geometry owner

`HostJunction` (line 690) is `{Name, Link, Target}`. `HostJunctions(slug)` (line 703) currently returns a one-element slice for `_lyx`. `IsReservedHubName` (line 393) switches on `LyxDirName, "_raddle", BoardDirName, "_portals", "_launchers"`. The weft-side accessors to mirror are `WeftLyxDirFor(slug)` (line 651) and `WeftRaddleDir()` (line 658); the host-side one is `HostLyxLink(slug)` (line 522).

**Critical interaction:** `internal/hubgeometry/enforcement_test.go:224` lists the banned geometry tokens (`_board`, `-weft`, `-HUB`, `_portals`, `_launchers`, `_raddle`, `_lyx`). Adding `_pattern` to that list — which the Hub Geometry Invariant requires — means `internal/pattern` **cannot** build `filepath.Join(root, "_pattern", "PATTERN.md")` itself. The path must come from a `hubgeometry` accessor. This is forced, not optional. `"PATTERN.md"` is not a geometry token, but the whole path is cleanest kept in `hubgeometry` as `PatternFile(baseDir)`.

### `internal/fabricengine` — junction machinery

`WireJunctions` (`junction.go:40`) = `seedLyxJunction` + `seedGitExclude`; both already loop over `HostJunctions(slug)` and are individually idempotent, so **neither needs changing**. `seedLyxJunction` creates, verifies, or re-points; it refuses a real (non-link) directory as host-pristine violation.

The unwire side is asymmetric and is the actual work: `unseedLyxJunction` (`junction.go:191`) hardcodes the single `_lyx` junction, and `UnwireResult.JunctionRemoved` is a single bool to match. `unseedGitExclude` (line 254) already iterates correctly.

`status.go:199` scans the host index with `git ls-files -- _lyx _raddle`; `_lyx` matches offer an automated restore, `_raddle` matches are report-only *because* no junction exists for it. Once `_pattern` has a junction, its pollution should be treated like `_lyx` (restorable), not like `_raddle`.

### `internal/initengine` — the actual junction caller

`Init(cwd)` (`init.go:49`) resolves the layout, refuses if there is no weft pairing, calls `fabricengine.WireJunctions(l, slug)` at line 67, then `MkdirAll(cwd/_lyx)` at line 82 — which lands in weft *through* the junction, materialising the weft-side directory. `_pattern` follows the same two-step: wire, then `MkdirAll` through the junction. `undo.go:53` calls `UnwireJunctions` and consumes `UnwireResult`, so it changes with the result shape.

`fslink.CreateDirLink` does not require the target to pre-exist (Windows creates a raw reparse point; Linux creates a symlink), but `seedLyxJunction`'s *idempotent re-check* path calls `filepath.EvalSymlinks(target)` and errors if the target is missing. So the `MkdirAll`-through-the-junction step in `Init` is what makes the second and later runs clean — the same sequencing `_lyx` already relies on.

### The five prompt-assembly sites

There is **no shared prompt-assembly layer**; each engine builds its own `map[string]string` inline. Exact locations and their existing marker counts (each template banner states its own count and must be updated):

| Template | Fill site | Markers today |
|---|---|---|
| `builderengine/implementer-template.md` | `spawn.go:471-478` | 5 |
| `burlerengine/review-prompt-template.md` | `prompt.go:22-34` | 9 |
| `websterengine/fork-template.md` | `render.go:123-132` | 6 (one branch-internal) |
| `websterengine/master-template.md` | `render.go:306` | 7 |
| `loomengine/plan-template.md` | `plan.go:42-48` | 3 |

`RenderForkPrompt` and `RenderMasterPrompt` are exported within the package; adding parameters to them is a package-internal signature change only.

### Existing conditional-marker precedent

`websterengine/fork-template.md:30-33` wraps `{{.rename_mechanic}}` in `{{if .rename_mechanic}}`, and `render.go:118-120` sets it to `""` when the batch has no Moves-bearing card. `template_test.go:156` seeds it as `""` in the marker-values map and excludes it from the per-marker deletion sweep. This is the working precedent that shows the `{{if}}` route is viable — and the reason the design doc's "first genuinely-conditional token" claim is wrong. It was considered and rejected (see the stencil-optional-marker decision); the new `FillOptional` does **not** retroactively change `rename_mechanic`, which stays as it is.

## Constraints

From `CONSTRAINTS.md`, in force for this task:

- **Hub Geometry Invariant.** `_pattern` becomes a geometry token owned solely by `internal/hubgeometry`. No other package may use the literal in a path-construction context (a `filepath.Join` argument, a `+` operand, or a string `const` value). Machine-enforced by `internal/hubgeometry/enforcement_test.go` (`TestEnforcement_GeometryLiterals`) on every `go test`; the token must be added to that test's list in the same commit as the accessors. Geometry is structural, never config- or env-overridable — there is no "pattern dir name" config key.
- **CLI / Cobra Invariant.** This task adds no new CLI module and no new subcommand, so the `Command()`/`RunCLI` seam is untouched. But `lyx init`'s observable behaviour changes (it now creates a second junction and a `_pattern/` directory), so its `Short`/`Long` must be re-read and updated if they enumerate what init does — help accuracy is a review-blocking obligation. Errors stay on the `internal/output` envelope.
- **lyxtest Leaf Invariant.** `internal/lyxtest` must not gain a PATTERN helper (see the testing decision below); it stays a leaf importing only stdlib and `internal/hubgeometry`.
- **Documentation Lifecycle.** This task adds a module (`internal/pattern`) and changes observable CLI behaviour, so `docs/overview.md`'s module table and `CONSTRAINTS.md` update **in the same commit**. `manifest/designs/pattern.md` is corrected in the same commit too (see below). `manifest/roadmap.md` moves only if the PATTERN item is completed or added — the wiring landing is a roadmap-relevant event for the Planned `PATTERN.md` item.

New invariant introduced by this task, to be recorded in `CONSTRAINTS.md` in the same commit:

- **Pattern Leaf Invariant.** `internal/pattern` production code imports only the standard library and `internal/hubgeometry`. The reverse import (`pattern` → any feature package) is never allowed. Enforced by `internal/pattern/leaf_enforcement_test.go` (`TestLeafInvariant_AllowlistOnly`), modelled on `internal/modelspec/leaf_enforcement_test.go`, `internal/tokenvocab/leaf_enforcement_test.go` and `internal/codeintelengine/leaf_enforcement_test.go`.

Design-doc corrections to land in `manifest/designs/pattern.md` in the same commit:

1. The claim that PATTERN is "the first genuinely-conditional token in the system" is wrong — `websterengine`'s `rename_mechanic` predates it. The optional-marker extension is a deliberate design choice, not a forced one; the rationale must be restated accordingly.
2. "The `_pattern` junction is `fabric`'s responsibility" is imprecise: `fabricengine` owns the primitive, `initengine.Init` is the caller. `fabric add` explicitly does not wire junctions.
3. "sibling of `_lyx` and `_raddle`" overstates `_raddle`: `_raddle` has no junction today, so `_pattern` is the first *second* junction rather than a third peer.
4. The Open Questions section is resolved by this discussion and should be replaced with the decisions.

Non-negotiable project rules that bear on the work:

- Markdown is written one line per paragraph, never hard-wrapped at a column. This applies to the five template `.md` files and every doc touched.
- This worktree may not touch any other worktree, and may not push to `main`.

## Testing

TDD candidates are marked. Assertion shapes are left to mill-plan.

### `internal/stencil` — TDD

`stencil_test.go` is an existing black-box, table-driven contract test; extend it rather than starting a new file. Scenarios that must be covered:

- An optional marker absent from `values` renders as nothing, no error.
- An optional marker present but empty or whitespace-only renders as nothing, no error.
- An optional marker present and non-empty renders its value.
- A **non**-optional marker that is empty still produces the existing `unfilled top-level marker(s)` error — the optional list must not weaken the guarantee for anything else.
- A mix: one optional-and-empty plus one required-and-empty reports only the required one.
- `Fill(t, v)` and `FillOptional(t, v, nil)` are byte-identical on the same input, including the error path.
- An optional name listed but not present anywhere in the template is a harmless no-op.
- Determinism: repeated calls produce identical output and identical error text (the existing suite already asserts this shape for `Fill`).

### `internal/pattern` — TDD

New package, new test file, using `t.TempDir()` per the fixture decision. Scenarios:

- `_pattern/PATTERN.md` present ⇒ non-empty directive for each role.
- `_pattern/` present but `PATTERN.md` absent ⇒ empty string.
- Neither present ⇒ empty string.
- `PATTERN.md` present but **empty** ⇒ **active** (non-empty directive) — asserting the accepted degenerate case explicitly so a later "improvement" cannot silently flip it.
- `PATTERN.md` present as a **directory** rather than a file ⇒ decide and pin one behaviour (recommend: treated as inactive, since it is not a readable index).
- The implementer and reviewer variants differ, and each contains the literal `_pattern/PATTERN.md` pointer.
- Each variant begins with its own `##` heading, so an inactive render leaves no orphan heading.
- A `leaf_enforcement_test.go` asserting the import allowlist, modelled on the three existing ones.

### `internal/hubgeometry`

- The new accessors return the expected joins.
- `IsReservedHubName("_pattern")` is true.
- `HostJunctions(slug)` returns two entries with the expected `Name`/`Link`/`Target` for each.
- `TestEnforcement_GeometryLiterals` passes with `_pattern` added — this is the test that will catch any stray `"_pattern"` literal anywhere in production code, including in `internal/pattern` itself.

### `internal/fabricengine`

- Wire then unwire with two junctions: both removed, both exclude lines removed, result reports both.
- Idempotency: wiring twice is a no-op; unwiring an already-unwired worktree is the legitimate no-op case, not an error.
- The existing refusal behaviours still hold **per junction**: a real (non-link) directory at either host path is refused; a dangling or wrong-target link is re-pointed.
- Partial state: `_lyx` wired but `_pattern` not yet (the upgrade path for a worktree initialised before this change) — wiring must complete without error.
- `status.go`'s pollution scan reports a tracked `_pattern/` path and offers the same restore remedy as `_lyx`.
- Existing tests that assert a one-element `HostJunctions` or a single-bool `UnwireResult` will fail and must be updated, not deleted: `junction_repoint_test.go`, `reconcile_stale_registration_test.go`, and the `initengine` undo tests are the likely sites.

### `internal/initengine`

- After `Init`, both junctions resolve and both weft directories exist.
- Second `Init` on the same worktree is idempotent.
- `Init --undo` removes both junctions and both exclude entries.
- An `Init` on a worktree wired before this change adds `_pattern` without disturbing `_lyx`.

### The five engine template tests

Each of `builderengine`, `burlerengine`, `websterengine` and `loomengine` has a `template_test.go` with a `*MarkerValues()` map plus a per-marker deletion sweep that asserts `Fill` errors when any single marker is removed. For the affected templates:

- Add `pattern_directive` to the marker-values map with a placeholder string.
- **Exclude it from the deletion sweep** — deleting an optional marker must *not* error. `websterengine/template_test.go:417` already does exactly this for `rename_mechanic`; follow that precedent, including its explanatory comment.
- Add a positive case per template: filling with an empty `pattern_directive` succeeds and the rendered output contains no leftover `{{`, no orphan `## Constraints` heading, and no stray blank-line block where the directive would have been.
- Add a positive case per template: filling with a non-empty directive places it in the rendered output before the first work instruction.
- Update each template's leading banner comment — the marker count ("requires all five non-empty") and the statement about conditionals — so the banner stays true. This is a documentation obligation, not cosmetic: those banners are what the next reader trusts.

### Repo-wide

- `go test ./...` must pass. `go vet ./...` clean.
- No new `_pattern` string literal outside `internal/hubgeometry` (enforced automatically).

## Q&A log

- **Q:** Should `stencil` get a real optional-marker extension, or reuse the existing `{{if}}` idiom? **A:** Extension. The user asked for elaboration first; on the facts, `{{if}}` *would* work — `websterengine/fork-template.md` already uses it in production for `rename_mechanic`, so the design doc's "first genuinely-conditional token" premise is wrong — but the extension was chosen because it puts optionality in Go where it is testable per call site, keeps the "no conditionals in templates" banner rule intact across five more files, and avoids a permanently-silent failure mode when a marker name is typo'd inside an `{{if}}` condition.
- **Q:** Where does the active-check helper live — `stencil`, `fabricengine`, or a new package? **A:** A new leaf package `internal/pattern`, mirroring `modelspec`/`tokenvocab`/`codeintelengine`. `stencil` is a pure text leaf with no filesystem access; `fabricengine` would be a wrong-direction dependency for four feature packages.
- **Q:** Which templates get the marker? **A:** The five code-touching ones. All templates already exist — nothing is created. Webster **Master** is added beyond the design doc's list, because forks inherit Master's context and Master already reads `CONSTRAINTS.md` in full. Webster integration, builder orchestrator, loom discussion, the four treadle judges and the reed header are excluded.
- **Q:** Directive wording — one line or role-specific? **A:** Two role variants, and phrased far more imperatively than a single sentence: a gated `- [ ]` checklist under its own `##` heading, since checklists are more effective in agent prompts. The heading lives inside the marker value so an inactive render leaves nothing behind.
- **Q:** Should the checklist be machine-gated? **A:** No. Prompt text only. A report-echo gate would touch three report parsers and could be satisfied without reading the file, so it buys the appearance of enforcement rather than enforcement.
- **Q:** Junction or weft-directory-only? **A:** Junction — the directory lives in weft and the warp reaches it through a junction, exactly like `_lyx`.
- **Q:** Is it `fabric` or `lyx init` that creates the junctions? **A:** `lyx init`. The user's recollection was correct and it corrects the design doc: `initengine.Init:67` is the caller; `add.go:295` states outright that `fabric add` does not wire junctions. `fabricengine` owns the primitive only. Also surfaced: `_raddle` has no junction at all today (`status.go:194`), so `_pattern` is the first *second* junction, which forces the `unseedLyxJunction`/`UnwireResult` generalisation the code's own godoc (`junction.go:179-183`) asks for.
- **Q:** Marker name? **A:** Whatever matches existing marker names — so `{{.pattern_directive}}`, snake_case, not the design doc's `{{.PatternDirective}}`.
- **Q:** What counts as active — file existence, non-empty content, or directory presence? **A:** Pure existence of `_pattern/PATTERN.md`. The directory may exist without the file. An empty `PATTERN.md` therefore counts as active; that degenerate case is accepted knowingly and pinned by a test.
- **Q:** Test fixture strategy? **A:** Per-package `t.TempDir()`. No shared helper in `internal/lyxtest`, which stays a guarded leaf; the engine template tests just use a placeholder string like every other marker.
- **Q:** `FillOptional` semantics when an optional marker is absent from `values` entirely? **A:** Renders empty, no error — exempt from both the top-level emptiness check and `missingkey=error`. "Optional" must mean optional; requiring the key to be present with `""` would turn one forgotten call site into a runtime failure in all five agents.
- **Q:** Where in each template does the marker sit? **A:** As early as possible — right after the opening role paragraph, before the first concrete work instruction. A "STOP, read this first" gate only works ahead of the work description, and a uniform rule gives reviewers something to check.
