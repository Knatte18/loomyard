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

- A new leaf package `internal/pattern` exposing `Directive(l *hubgeometry.Layout, role Role) string` — the shared active-check plus directive text, returning the empty string when PATTERN is inactive.
- A `stencil` optional-marker extension: `FillOptional(template []byte, values map[string]string, optional []string) ([]byte, error)`, with `Fill` reduced to `FillOptional(t, v, nil)`.
- A `{{.pattern_directive}}` marker added to exactly five existing templates: `internal/builderengine/implementer-template.md`, `internal/burlerengine/review-prompt-template.md`, `internal/websterengine/fork-template.md`, `internal/websterengine/master-template.md`, `internal/loomengine/plan-template.md`.
- Plumbing the computed directive into those five prompt-assembly sites.
- New `hubgeometry` geometry surface for `_pattern`: `PatternDirName`, `PatternDir(baseDir)`, `PatternFile(baseDir)`, `PatternFileHere()`, `HostPatternLink(slug)`, `HostPatternLinkHere()`, `WeftPatternDir()`, `WeftPatternDirFor(slug)`; `_pattern` added to `IsReservedHubName`, to `HostJunctions()`, and to the enforcement test's geometry-token list.
- Generalising `fabricengine`'s unwire path from its hardcoded single-`_lyx` shape (`unseedLyxJunction`, `UnwireResult.JunctionRemoved`) to a per-junction shape, since `_pattern` is the first second junction.
- Generalising the junction **health check** from its `_lyx`-only form to a per-junction loop at all three of its sites — `fabricengine/reconcile.go:146`, `fabricengine/status.go:148`, `fabricengine/drift.go:73` — plus `checkJunctionHealth`'s hardcoded reason strings.
- Generalising `fabricengine`'s `removeHostJunction` (`weftwiring.go:124`) so `lyx fabric remove` tears down every junction, not only `_lyx`.
- Materialising a junction's weft-side target inside `seedLyxJunction` itself, so every `WireJunctions` caller — not just `Init` — leaves a resolvable junction behind.
- Extending `fabricengine/status.go`'s host-pollution scan to `_pattern`.
- Adding `_pattern` to fabric's default weft `pathspec`, so PATTERN content is actually committed and pushed — **and** making `CommitWeft` tolerate a pathspec entry that currently matches nothing, without which the widened default silently stops every weft commit.
- `initengine.Init` creating the `_pattern/` directory through the junction, exactly as it does for `_lyx`; `initengine.undo` leaving weft `_pattern/` content untouched.
- Pinned CLI output-shape changes in `internal/initcli` for the now-plural junction set.
- Docs in the same commit: `CONSTRAINTS.md` (new Pattern Leaf Invariant; `_pattern` added to the Hub Geometry Invariant's token list), `docs/overview.md` module table, and `manifest/designs/pattern.md` corrections.

**Out:**

- **All content migration.** No loomyard invariant moves out of `CONSTRAINTS.md`. No real `_pattern/PATTERN.md` is authored in this repo. `CONSTRAINTS.md` stays the single live invariants doc while mill develops loomyard.
- **The `_pattern/<topic>/` detail-submap layout.** That is a content-authoring decision belonging to the migration, not the wiring.
- **The three existing hardcoded `CONSTRAINTS.md` mentions** in `loomengine/discussion-template.md:28`, `loomengine/plan-template.md:19` and `websterengine/master-template.md:25`. They stay exactly as they are; the PATTERN directive is added alongside them, not in place of them. Replacing them is migration work.
- **Templates that are not code-touching:** `websterengine/integration-template.md`, `builderengine/orchestrator-template.md`, `loomengine/discussion-template.md`, all four `treadleengine` judge/triage/targeting templates, `reedengine/header-template.md`.
- **Machine enforcement that an agent actually read PATTERN.md.** No report-echo gate, no verify-command hook.
- **Splitting burler's round prompt into separate review and fixer templates.** Raised during discussion and filed as [issue #105](https://github.com/Knatte18/loomyard/issues/105). It is a restructuring of burler's prompt architecture, entirely independent of PATTERN, and folding it in would couple two unrelated changes in one commit. Nothing is lost by deferring: the combined `RoleReviewFix` variant this task lands divides cleanly along the same seam if that split later happens.
- **`internal/gitrepo`.** Untouched, which is what keeps this task parallel-safe with the concurrent `native-clients` task.

## Decisions

### stencil-optional-marker

- **Decision:** add `stencil.FillOptional(template []byte, values map[string]string, optional []string) ([]byte, error)`. `Fill(t, v)` becomes exactly `FillOptional(t, v, nil)`, so all ten existing call sites are unchanged in behaviour and signature. A name listed in `optional` is exempt from **both** guards: it is skipped by `unfilledTopLevelMarkers`, and its absence from `values` must not trigger `missingkey=error` during execution. An optional marker that is absent, empty, or whitespace-only renders as nothing.
- **"Whitespace-only renders as nothing" means `FillOptional` normalises it to `""`, and that must be implemented deliberately.** Seeding listed-but-absent names with `""` in a copied map is not sufficient on its own: a value of `"   "` would still render its spaces verbatim. The copy step therefore applies the same `strings.TrimSpace(v) == ""` test `unfilledTopLevelMarkers` already uses (`stencil.go:119`) and writes `""` for any optional entry that fails it. This keeps one definition of "empty" across the whole package rather than two.
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
- **Stat errors that are not `IsNotExist` are treated as active, not inactive.** A permission or I/O error on `PATTERN.md` must not render identically to "PATTERN is not configured" — that would make the constraints vanish from all five agents with no signal anywhere. `Directive` keeps its `string` return (a second return value would push error handling into five prompt-assembly sites for a case none of them can act on), and resolves the ambiguity by **failing loud in the prompt instead of silent**: on a non-`IsNotExist` stat error it returns the directive anyway. The agent then reads the file itself and reports a real, visible failure if it genuinely cannot. Absent file ⇒ inactive; every other outcome ⇒ active.
- **Rejected — treat an empty file as inactive:** would catch a half-finished migration, but makes the switch two-valued for no real benefit.
- **Rejected — treat an empty file as an error:** turns a benign state into a hard failure across every agent.
- **Rejected — `Directive(...) (string, error)`:** none of the five call sites can do anything useful with a stat error except abort a run that would otherwise succeed, and the fail-loud-in-prompt behaviour above already removes the silent-vanish failure mode that motivated the concern.
- **`PATTERN.md` present as a directory rather than a regular file** counts as **inactive**: it is not a readable index, and the directive would point at something the agent cannot read. Pinned by a test.

### directive-shape-and-wording

- **Decision:** three role variants — `RoleImplementer`, `RoleReviewFix` and `RoleOrchestrator` — each an imperative markdown checklist block, not a single sentence. The block **carries its own `##` heading inside the marker value**, so that when PATTERN is inactive the marker renders as nothing at all and no orphan heading is left behind.
- **Rationale:** an imperative gated checklist is measurably more effective in agent prompts than a lone declarative sentence. The variants exist because the operative obligation genuinely differs across the five templates: three write code, the burler round both judges and writes, and webster Master neither — it forks.
- **The roles are not `implementer` / `reviewer`.** A pure reviewer variant would have no user: the burler round is the only reviewing template in the set, and it also fixes (`review-prompt-template.md:10-17` — *"You have two jobs, in order, in this one session"*, part B being *"Fix every finding you recorded"*). `RoleReviewFix` states what it actually is. `RoleOrchestrator` exists because Master's prompt says outright *"You never edit code yourself"* (`master-template.md:21`), so an implementer-worded instruction would be one it cannot carry out.
- **`RoleImplementer`** (used by builder implementer, webster fork, loom plan):

  ```markdown
  ## Constraints — do this before you write any code

  - [ ] **STOP.** Before you edit a single file, read `_pattern/PATTERN.md` in full.
  - [ ] Read every detail doc under `_pattern/` that PATTERN.md points to and that touches what you are about to change.
  - [ ] These constraints are BINDING. A change that violates one is wrong even if the verify command passes.
  - [ ] If a constraint conflicts with anything else in this prompt, the constraint wins — say so in your report instead of silently picking one.
  ```

- **`RoleReviewFix`** (used by the burler round; covers both of the round's phases in the round's own order):

  ```markdown
  ## Constraints — do this before you judge or change anything

  - [ ] **STOP.** Before you form any judgment, read `_pattern/PATTERN.md` in full.
  - [ ] Read every detail doc under `_pattern/` that PATTERN.md points to and that touches the target.
  - [ ] **In part A:** every violation of a listed constraint is a BLOCKING finding. Record it, no matter how small it looks. Never wave one through because the code works or the tests pass.
  - [ ] **In part B:** your own fix must not introduce a violation. A fix that trades one finding for a constraint breach is not a fix.
  - [ ] If a constraint conflicts with anything else in this prompt, the constraint wins — say so in your review instead of silently picking one.
  ```

- **The pointer stays the relative `_pattern/PATTERN.md`, deliberately — on prompt-idiom grounds, not anchor-divergence grounds.** The two anchors normally cannot disagree: `hubgeometry.Resolve` sets `RelPath = filepath.Rel(WorktreeRoot, Cwd)` (`hubgeometry.go:63`, `:93`), so `WorktreeRoot+RelPath == Cwd` for any `Resolve`-built Layout, and the Go-side check therefore stats exactly the directory the agent's own cwd resolves against. Only a Layout built by the `RelPath`-hardcoding constructor (`hubgeometry.go:170-186`) could diverge. The real reason to keep the relative form is consistency: these templates already say `_lyx/plan/`, `00-overview.md` and similar without interpolating absolute paths. Interpolating a resolved absolute path would make the directive marker's value vary per worktree, defeating the fixed-string tests, to guard a case no other prompt in the repo guards. Pinned by a test asserting the literal pointer.
- **The directive is never gated on target type.** The burler round's target is often prose rather than code, but it is injected there all the same. Loomyard has no mechanism for classifying a target as code-vs-prose, and a file-extension heuristic would be new, fragile logic whose misclassification silently removes the constraints. A prose target carrying a pointer to the invariants is mild noise, not harm — and invariants can govern prose (this repo's own one-line-per-paragraph markdown rule is exactly such a constraint).
- The directive **injects a pointer, never the constraints inline** — the agent reads the file itself, so prompt size is constant regardless of how large PATTERN grows.
- **Rejected — one role-invariant sentence:** simpler, but demonstrably weaker as an instruction, and cannot express the review-fix round's blocking obligation.
- **Rejected — giving burler both variants back to back:** covers the same ground with two `##` headings and two verbatim-repeated opening bullets.
- **Rejected — a role-parameterised template string** composing text from parts: more surface for no proven need. Two literal constants selected by a small `Role` enum is enough.

### template-set

- **Decision:** the marker goes into exactly five existing templates. None are created; all five already exist.
  - `internal/builderengine/implementer-template.md` — `RoleImplementer`
  - `internal/burlerengine/review-prompt-template.md` — `RoleReviewFix`
  - `internal/websterengine/fork-template.md` — `RoleImplementer`
  - `internal/websterengine/master-template.md` — `RoleOrchestrator`
  - `internal/loomengine/plan-template.md` — `RoleImplementer`
- **The inclusion criterion is not "writes code".** A template gets the directive if its agent (a) writes code, (b) reviews code, **or** (c) is a context-inheritance root whose in-session forks do. Clause (c) is what admits webster Master, and it is the only defensible reading — `master-template.md:21` says in as many words *"You never edit code yourself"*, so an editing-based criterion would exclude it. The design doc's "reviewer" role has no separate template; it *is* the burler round.
- **Master therefore gets its own third variant, `RoleOrchestrator`** — `RoleImplementer`'s wording (*"before you write any code / before you edit a single file"*) is simply false for Master and would have been an instruction it cannot follow:

  ```markdown
  ## Constraints — do this before you fork anything

  - [ ] **STOP.** Before you fork a single implementer, read `_pattern/PATTERN.md` in full.
  - [ ] Read every detail doc under `_pattern/` that PATTERN.md points to and that touches this plan.
  - [ ] Every fork inherits your context, so reading this once here is what puts these constraints in front of all of them. Do not skip it because you are not editing code yourself.
  - [ ] These constraints are BINDING on the forks you spawn. Treat a batch report that trades a constraint for a passing verify as a failed batch, not a success.
  ```

- **Why `builderengine/orchestrator-template.md` still stays out under the same criterion:** it satisfies neither (a) nor (b) — *"You never edit code yourself"* — and it fails (c) too, because builder's implementers are **separate spawned sessions** that each receive their own prompt carrying `RoleImplementer` directly. There is no context to inherit, so injecting at the orchestrator would buy nothing. Webster's forks are in-session and thin *precisely because* they inherit; that difference is the whole reason one is in and the other is out.
- **Excluded, with reasons:** `websterengine/integration-template.md` (runs the verify once, implements no cards, makes no commit — "not even for a trivial, obviously-correct fix", and spawns nothing); `builderengine/orchestrator-template.md` (see above); `loomengine/discussion-template.md` (interview phase, writes a decision record, not code); the four `treadleengine` templates (judge/triage/targeting — judgment only, no forks); `reedengine/header-template.md` (not an agent prompt at all).

### marker-name

- **Decision:** `{{.pattern_directive}}`, snake_case.
- **Rationale:** matches every existing marker in the repo — `rename_mechanic`, `shared_decisions`, `worktree_root`, `batch_file`, `report_path`, `self_fix_cap`, `prev_digest`, `decision_record_path`. The design doc's `{{.PatternDirective}}` matches no existing marker.

### marker-placement

- **Decision:** the marker sits **immediately before the template's first `##` heading** — that is, after the `#` title and any leading orientation blockquote, and before the first `##` section. One rule, no per-template exceptions.
- **Rationale:** a "STOP, read this first" gate only works if it precedes the description of the work, and a single mechanical rule is something a reviewer can check without judgement. The constraint that forces this exact formulation is that **the directive block carries its own `##` heading** (see directive-shape-and-wording): dropping it *inside* an existing `##` section would silently terminate that section and orphan its remaining prose under the constraints heading. So "after the opening role paragraph" is not implementable as stated — the marker must land at a `##` boundary.
- **Concretely, this supersedes the line numbers named earlier in discussion:** in `master-template.md` the block goes before the first `##`, **not** at line 25 (which sits inside the orientation section and would split it from the `_lyx` rule at line 27); in `plan-template.md` before `## Step 1`, **not** at line 19 (inside `## Step 2`).
- **Consequence, accepted:** in `master-template.md` and `plan-template.md` the PATTERN block therefore sits somewhat *ahead* of the existing `CONSTRAINTS.md` sentences rather than beside them. That is the correct order anyway — PATTERN is the repo-owned invariants doc; `CONSTRAINTS.md` is mill's.
- **Rejected — placing it just before the report/deliverable section:** closer to where code is written, but the agent has already absorbed the whole task by then, which is exactly what the gate is meant to precede.
- **Rejected — choosing per template during implementation:** no shared rule to review against.
- **Rejected — stating per-template exceptions as the rule:** five special cases is not a rule.

### junction-ownership

- **Decision:** `_pattern` becomes a real host junction into weft, exactly like `_lyx`. Responsibility splits as follows, which **corrects `manifest/designs/pattern.md`**:
  - `hubgeometry` **declares** the junction record and owns every `_pattern` path literal.
  - `fabricengine.WireJunctions` / `seedLyxJunction` **need no change** — they already iterate `l.HostJunctions(slug)`.
  - `initengine.Init` is the **caller** that creates it, and must also `MkdirAll` the `_pattern` directory through the junction, mirroring what it does for `_lyx`.
  - `fabricengine`'s unwire path must be generalised (see next decision).
- **Rationale:** the design doc says "the `_pattern` junction is `fabric`'s responsibility, exactly like `_lyx`/`_raddle`", which is imprecise in two ways. First, **`fabric add` deliberately does not wire junctions** — `add.go:295` states outright: *"Add does not wire the host `_lyx` junction (it is dormant) … The junction is wired by lyx init via `WireJunctions`."* `initengine.Init:67` is the sole production caller alongside `checkout.go:152` and `reconcile.go:153` (both re-point existing wiring). Second, **`_raddle` has no junction at all today** — `status.go:194` says *"no junction is wired for `_raddle` in this release"* — so `_pattern` is not joining an established plural pattern; it is creating one.
- **Rejected — weft directory only, no host junction:** the directive would have to name an awkward cross-tree path, and it breaks the `_lyx` sibling symmetry the design depends on.

### unwire-generalisation

- **Decision:** generalise `fabricengine.unseedLyxJunction` to iterate `l.HostJunctions(slug)`, and replace `UnwireResult.JunctionRemoved bool` with **`JunctionsRemoved []string`** — the `Name` of each junction actually removed, in `HostJunctions` order. `initengine.undo` and `UndoResult` are updated to match.
- **Rationale:** the code flags this itself. `junction.go:179-183` reads: *"It is deliberately scoped to the single `_lyx` junction … rather than iterating `l.HostJunctions(slug)` the way `unseedGitExclude` does: `HostJunctions` returns exactly one entry today, and `UnwireResult.JunctionRemoved` is a single bool by design to match. **If `HostJunctions` ever grows a second entry, this function and `UnwireResult` should be revisited together.**"* This task is that event. Note `unseedGitExclude` already iterates correctly and needs no change.
- **Why a name slice and not a count:** the caller is CLI-observable output (`UndoResult.LyxJunction string` today), and "1 of 2 removed" is useless to an operator who needs to know *which*. A slice costs nothing and reports precisely.
- **Partial-failure contract — this is the part the current code gets wrong.** `UnwireJunctions` today returns a **zero** `UnwireResult` when `unseedLyxJunction` errors (`junction.go:163`). With one junction that is merely uninformative; with two it is a **lie**, because the first junction may already have been removed before the second failed. The new contract: on a mid-loop failure, return the `JunctionsRemoved` accumulated **so far** alongside the error — never a zero value. The same rule already governs the post-junction step (`junction.go:168` returns the junction outcome when the exclude update fails); this extends it inside the loop. Unwire continues to abort on the first junction error rather than pressing on, preserving the "any junction inconsistency is a hard error" invariant.
- **Rejected — add the junction but leave `UnwireResult` single-bool:** smaller diff, but knowingly leaves the unwire path wrong for the second junction, against the code's own written instruction.
- **Rejected — a count (`JunctionsRemoved int`):** loses the identity the operator needs.

### junction-health-check

- **Decision:** generalise the junction health check at **all three** of its sites, in the same commit — it is not reconcile-only:
  - `reconcile.go:146-148` — `HostLyxLinkHere()` vs `WeftLyxDir()`, gating repair.
  - `status.go:148-150` — the same pair, feeding `PairStatus.JunctionHealthy` / `JunctionReason` and folded into the in-sync verdict.
  - `drift.go:73-94` — the same pair again, open-coded rather than calling `checkJunctionHealth`, consumed by loom's preflight.
  Each becomes a loop over a **new `HostJunctionsHere()`**: unhealthy if **any** junction is missing, not a link, or mis-pointed.
- **The three health-check sites loop over `HostJunctionsHere()`, not `HostJunctions(slug)`.** All three use the *Here*-anchored pair (`HostLyxLinkHere()` / `WeftLyxDir()`), which is `WorktreeRoot`-derived and needs no slug, whereas `HostJunctions(slug)` is `Hub`/slug-anchored. `PairInSync(l *hubgeometry.Layout)` (`drift.go:38`) takes no slug at all and is documented as stateless, so threading one in would break its contract. `hubgeometry` therefore gains `HostJunctionsHere() []HostJunction` alongside `HostJunctions(slug)`, mirroring the existing `HostLyxLinkHere()` / `HostLyxLink(slug)` and `WeftLyxDir()` / `WeftLyxDirFor(slug)` pairs. Wiring, unwiring and `remove` keep using the slug-anchored form; detection uses the Here-anchored one.
- **`PairStatus` keeps its shape.** `JunctionHealthy bool` and `JunctionReason string` stay singular — first-unhealthy-wins, with the reason string naming *which* junction. No CLI output-shape change for `status`, and the information an operator needs is in the reason.
- **Every reason string is parameterised by junction name — six sites, not two.** With a singular `JunctionReason`, the string is the *only* thing telling an operator which junction is broken, so any one left unparameterised defeats the "the reason names which" premise the `PairStatus`-keeps-its-shape decision rests on. The full set:

  | Site | Today | Becomes |
  |---|---|---|
  | `reconcile.go:318` | `host _lyx junction missing` | `host <name> junction missing` |
  | `reconcile.go:326` | `host _lyx is not a junction` | `host <name> is not a junction` |
  | `reconcile.go:341` | `host _lyx junction points elsewhere` | `host <name> junction points elsewhere` |
  | `drift.go:83` | `junction missing` | `host <name> junction missing` |
  | `drift.go:92` | `host _lyx is not a junction` | `host <name> is not a junction` |
  | `drift.go:110` | `junction points elsewhere` | `host <name> junction points elsewhere` |

- **`drift.go`'s two bare strings are worse than merely unparameterised.** `"junction missing"` and `"junction points elsewhere"` name no junction at all, so with two junctions they are ambiguous rather than just imprecise. Bringing them onto `checkJunctionHealth`'s wording also *completes* the alignment `drift.go:92`'s own comment already asks for — *"Same wording as checkJunctionHealth for this drift shape, so status/reconcile and PairInSync describe it identically"* — which today only one of drift's three strings actually honours.
- **These strings have a downstream consumer that classifies on them, and rewording them breaks it.** `loomengine/preflight.go:127-134` switches on `PairInSync`'s reason by prefix: `strings.HasPrefix(reason, "host on ")` ⇒ `CheckWeftSync`, `strings.HasPrefix(reason, "junction")` ⇒ `CheckJunction` **and** `check3BlocksSeed = true`, everything else ⇒ `CheckWeftSync`. Reworded to `host <name> junction missing`, drift's strings match neither case and fall to `default` — so a genuinely broken junction would be reported as a weft-sync failure and would **stop blocking the seed check**. `preflight_integration_test.go:316` catches it.
- **Fix, and the contract it establishes:** `preflight.go`'s junction case becomes `strings.Contains(reason, "junction")`, evaluated after the `"host on "` case (the branch-mismatch reason contains no `"junction"`, so ordering keeps them disjoint). This survives junction-name interpolation, which a prefix match cannot. **`internal/loomengine` is a certain breakage site** and joins the list. *(Alternative considered and rejected as disproportionate: returning a typed reason from `PairInSync` instead of a classified string — cleaner long-term, but expands this task into loomengine's API.)*
- **Rationale:** without this, a missing or mis-pointed `_pattern` junction makes `reconcile` report `ReconcileActionAlreadyHealthy`, `status` report the pair in-sync, and loom's preflight pass — three separate paths reporting success while the fault stands. `WireJunctions` is already idempotent and already repairs every junction, so only detection is blind.
- **Note:** `ReconcileActionJunctionRepointed`'s `Detail` string names a single `hostLink → weftLyxDir` pair today and must be widened to name whichever junctions were repaired.
- **Rejected — making `PairStatus` per-junction:** richer, but breaks the output shape for every consumer to say something the reason string already conveys.
- **Rejected — leaving `status`/`drift` `_lyx`-only:** shipping a second junction alongside three health checks that structurally cannot see it is the kind of half-wiring the Hub Geometry Invariant exists to prevent.

### remove-tears-down-every-junction

- **Decision:** generalise `fabricengine.removeHostJunction` (`weftwiring.go:124-129`) from its `HostLyxLink(slug)`-only form to a loop over `l.HostJunctions(slug)`.
- **Rationale:** `remove.go:91` calls it as step (5) of `lyx fabric remove`, and the step's own doc comment (`remove.go:38-39`) explains exactly why it exists: *"fslink.RemoveLinksIn only scans immediate children and misses nested `_lyx` at `RelPath != "."`; this catches subpath junctions."* Leaving it `_lyx`-only reintroduces precisely the bug that comment documents, for `_pattern`: at `RelPath != "."` the root-level safety net at step (6) never sees the `_pattern` junction, and `remove` deletes a host worktree with a live junction still inside it.
- **The loop is best-effort and continues past a per-junction error — the opposite of `unseedLyxJunction`'s abort-on-first-error rule, deliberately.** The two loops have different contracts and must not be written by analogy. `Remove`'s call site is `_ = removeHostJunction(l, slug)`: the return value is discarded, exactly like the adjacent `removePortal` and `removeLaunchers` calls in the same teardown. Aborting on the first junction's failure would silently leave every later junction un-removed, defeating this decision's own goal. Unwire aborts because a junction inconsistency there is a hard error the operator must see; teardown continues because its whole purpose is to remove as much as it can.
- **Rejected — exempting `remove`:** the exemption would be invisible in the code and would resurface as a stale-junction bug.

### weft-pathspec-tolerance

- **Decision:** `fabricengine.CommitWeft` filters the pathspec before staging, so an entry matching nothing is skipped rather than failing the whole `git add`. This lands in **`internal/fabricengine/weftgit.go`, deliberately not in `internal/gitrepo`** — keeping `gitrepo` untouched is what preserves this task's parallel-safety with the concurrent `native-clients` task.
- **The filter predicate, stated exactly.** An entry is **kept** if either of these holds:
  1. It begins with `:` — a git pathspec-magic entry. These are **always passed through untouched, never evaluated for matches.**
  2. It matches at least one path in the **worktree or the index**.

  Each clause exists because a looser reading breaks a real caller:
  - **Untracked must count.** A brand-new `_pattern/PATTERN.md` is untracked at the moment of its first commit. A `git ls-files`-based predicate sees nothing and would filter `_pattern` out — dropping the very first PATTERN commit, the one case the whole persistence decision exists to enable.
  - **Index-only must count.** `initengine/undo.go:93-94` commits a `_lyx` path that `RemoveAll` has just deleted from the worktree; it survives only in the index. A worktree-existence predicate would filter it out and silently break `lyx init --undo`'s deletion commit.
  - **Exclusion magic must pass through.** `CommitWeft`'s pathspec carries `:(exclude)` entries from `buildercli/weft.go:81-84`, `webstercli/weft.go` and `perchcli/run.go:424`. These never "match" in the ordinary sense; filtering one out re-stages machine-local artifacts (`*.lock`, pause flags, webster prompt files), against the Cross-module exclusions rule.
- **If no positive entry survives, `CommitWeft` returns `("", false, nil)` without calling `StageAndCommit` at all.** This is not a tidiness rule — without it the filter is *more* destructive than the bug it fixes. The `buildercli`, `webstercli` and `perchcli` pathspecs are one positive entry plus several `:(exclude)` entries. In a worktree where the positive entry matches nothing, the filter would hand git a non-empty, **all-negative** pathspec — and git reads exclusions with no positive pathspec as "everything except those", so `git add` would stage the **entire weft worktree**. The early return keeps the no-op semantics `CommitWeft` already promises for "nothing of ours to stage".
- **How the predicate is evaluated, mechanically.** The match check runs `git ls-files --cached --others -- <entry>` (covering index and untracked-in-worktree in one call, matching the two positive clauses above) with **cwd = the weft worktree** (`f.weftPath`), and entries interpreted relative to the weft root — the same anchor `StageAndCommit`'s own `git add` uses, so the filter can never disagree with the command it is filtering for. It runs **inside the already-held weft write lock**, alongside the staging it guards, so no concurrent writer can change what matches between the check and the `git add`.
- **A test with an `:(exclude)`-bearing pathspec is mandatory**, not optional — it is the only guard against a filter that looks correct on plain paths and either silently re-stages excluded artifacts or stages the whole worktree.
- **Rationale — this is the most dangerous consequence of the widened default, and it is not hypothetical.** `git add -- _lyx _pattern` fails *in its entirety* when `_pattern` matches nothing; `gitrepo.StageAndCommit` surfaces that, and `CommitWeft` (`weftgit.go:276-284`) deliberately swallows `"did not match any files"` into `("", false, nil)` — **no error**. So the moment the default pathspec widens, every weft commit in a worktree without PATTERN content stops happening, silently, taking `_lyx` down with it.
- **Materialisation does not rescue this.** Git tracks files, not directories, so a materialised-but-empty `weft/_pattern/` has nothing for a pathspec to match. And an empty `_pattern/` is the **normal, expected state for this entire task**, since content migration is explicitly out of scope. Without this tolerance the wiring would break weft committing in every worktree it touched — the widened pathspec and the tolerance must land together or neither should land.
- **One premise here is asserted, not verified, and the plan must settle it empirically.** Whether `git add` *dies* for an existing-but-empty directory entry, as opposed to only for a wholly absent one, depends on git's unmatched-pathspec guard, which this discussion did not test. Both shapes are already on the mandatory test list; **derive the assertion from what real git does in those tests** rather than pinning it on this paragraph. The decision itself does not depend on the answer — the filter is required for the wholly-absent case regardless — but the regression test's expected value does.
- **Rejected — one `git add` per pathspec entry with per-path tolerance:** same effect, more git invocations, and it moves the tolerance decision into a loop rather than stating it once.
- **Rejected — widening the pathspec dynamically only once `PATTERN.md` exists:** makes the committed pathspec depend on filesystem state at run time; fragile and hard to reason about when it misbehaves.

### pre-existing-host-pattern-directory

- **Decision:** a real (non-junction) `_pattern/` directory already present in the host repo continues to be **refused**, never adopted or moved. What changes is the message: `seedLyxJunction`'s error (`junction.go:113-117`) is reworded to name the actual remedy, and the remedy is documented in fabric's module doc and the affected `Short`/`Long`.
- **Rationale:** PATTERN content is described throughout as the host repo's hand-authored invariants, which makes "create `_pattern/` in the repo and start writing" the natural operator mistake. Today that produces *"host repo already contains a real `_pattern` at …; it predates weft — migrate via the hub-creator"* and hard-fails `lyx init`, `lyx fabric checkout` **and** `lyx fabric reconcile` for that worktree, pointing at a tool that does not address this case. The remedy to state: move the content into the weft `_pattern/` directory (or remove the host directory) and re-run `lyx init`, which then creates the junction.
- **Rejected — auto-migrating the host directory into weft:** convenient, but fabric never moves or deletes user content; `seedLyxJunction`'s refusal is a deliberate host-pristine guard and this task must not be the one that erodes it.
- **Rejected — leaving the wording as-is:** the operator hits a hard failure whose stated remedy does not apply to their situation.

### result-shapes

- **Decision:** pin the CLI-observable shapes now, since three of them change.
  - `fabricengine.UnwireResult.JunctionRemoved bool` → **`JunctionsRemoved []string`** (already decided under unwire-generalisation).
  - `initengine.UndoResult.LyxJunction string` → **`JunctionsRemoved []string`**, emitted by `internal/initcli` under the JSON key **`junctions_removed`**, replacing `lyx_junction` (`initcli.go:120`). `WeftContent` is unchanged and continues to describe `_lyx` only, per undo-leaves-pattern-content.
  - `initengine.InitResult` gains **`PatternDir string`** alongside `LyxDir string`, carrying the same `"created"` / `"exists"` vocabulary, so `lyx init` reports what it actually did.
- **Change sites named explicitly:** `internal/initcli/initcli.go:120` (the emitted key) and `internal/initcli/initcli_test.go:74` (which pins it).
- **This is a breaking change to `lyx init --undo`'s JSON output.** Accepted: the key would otherwise have to be repeated per junction, and it under-reports today. The CLI/Cobra Invariant makes the corresponding help text a review obligation.
- **Rejected — keeping `lyx_junction` and adding `pattern_junction`:** non-breaking, but does not scale to a third junction and duplicates a concept that is now genuinely a list.
- **Rejected — leaving the shape alone:** under-reports what undo did, on a command whose whole purpose is reporting what it undid.

### weft-target-materialisation

- **Decision:** materialise the weft-side target inside `seedLyxJunction`, as **the first statement of each loop iteration** — `os.MkdirAll(j.Target)` **before** the `os.Lstat(link)` re-check, not adjacent to either `CreateDirLink` call — so every `WireJunctions` caller leaves a resolvable junction behind. `initengine.Init`'s own `MkdirAll` through the junction becomes redundant but is harmless and stays.
- **The placement is the whole point, and "before `CreateDirLink`" would not have worked.** `seedLyxJunction` has two `CreateDirLink` calls (`junction.go:105` re-point, `:125` create), but the hard error this decision exists to remove is at `junction.go:83`, reached from the **link-already-exists** branch via `filepath.EvalSymlinks(target)` at line 81 — *before either call*. A `MkdirAll` sited next to `CreateDirLink` therefore still hard-errors on exactly the worktree the rationale is about: one whose junction a pre-fix `checkout` or `reconcile` already left dangling. Top-of-iteration placement is what gives that worktree a self-repair path.
- **Consequence for `Init`'s reporting — must be handled in the same change.** `Init` calls `WireJunctions` at `init.go:67` and only *then* stats `cwd/_lyx` at line 75 to decide `"created"` vs `"exists"`. Once the seeder materialises the target, that stat succeeds through the fresh junction and `LyxDir` reports `"exists"` on a first-ever init — silently inverting an existing CLI observable (`lyx_dir`, `initcli.go:98`) and making the required `PatternDir` `"created"` test unpassable. The fix: **stat the weft-side target before `WireJunctions` runs**, and derive `"created"`/`"exists"` for both `LyxDir` and `PatternDir` from that pre-wiring observation. The vocabulary keeps meaning what an operator can actually use it for — "did this invocation create it?".
- **Rationale:** `WireJunctions` has three production callers — `initengine/init.go:67`, `fabricengine/checkout.go:152` and `fabricengine/reconcile.go:153` — but **only `Init` materialises the weft directory**. `fslink.CreateDirLink` happily creates a link to a nonexistent target (Windows writes a raw reparse point; Linux a dangling symlink), so a `_pattern` junction created by checkout or reconcile points at nothing. The *next* `WireJunctions` on that worktree then takes `seedLyxJunction`'s already-exists branch, calls `filepath.EvalSymlinks(target)`, and hard-errors at `junction.go:83` — *"weft directory does not exist at %s; cannot validate junction target"* — breaking init, checkout **and** reconcile for that worktree with no self-repair path.
- **Why inside the seeder rather than at the callers:** one site instead of three, no ordering rule for future callers to forget, and it closes the same latent hole for `_lyx` (today masked only because `Init` happens to run first in practice).
- **Rejected — make the already-exists re-check tolerate a missing target:** would treat "target vanished" and "target never created" identically, weakening a check that exists to catch genuine weft corruption.
- **Rejected — `MkdirAll` at each of the three callers:** three places to forget it the next time a caller is added.

### weft-persistence

- **Decision:** add `_pattern` to fabric's default weft pathspec — `internal/fabricengine/template.yaml:2` becomes `pathspec: _lyx _pattern`.
- **Rationale:** without this nothing ever stages `_pattern`. The default is `pathspec: _lyx`, and `initengine/undo.go:93` builds its pathspec as `ScopedPathspec(l.RelPath, []string{hubgeometry.LyxDirName})`. A `PATTERN.md` written through the junction would never be committed or pushed, so it would never reach another machine or another worktree's weft pull — the mechanism would be inert in exactly the way that matters. `manifest/designs/finalize.md` already anticipates `_pattern` travelling with weft merge-back.
- **Migration consequence — confirmed, not deferred.** `configsync.ReconcileAll` → `yamlengine.Reconcile` keeps a `pathspec:` key that is already present and adds no key when one exists. Every already-initialised worktree therefore keeps `pathspec: _lyx` and never persists PATTERN content, no matter how many times `lyx init` is re-run. Operators must widen it by hand.
- **No detection or warning surface is in scope for this task.** Nothing — not `lyx fabric status`, not `lyx init` — reports a narrow pathspec, so an existing worktree stays silently inert. That is accepted here rather than papered over: adding a "your pathspec predates PATTERN" warning means a new diagnostic class in `fabric status`, and PATTERN has no content to persist in this repo yet anyway. The consequence is documented in fabric's module doc so the next operator meets it in writing rather than by surprise.

### legacy-worktree-junction-migration

- **Decision:** every worktree wired before this change lacks the `_pattern` junction, and the generalised health check **correctly reports that as a fault** until the operator re-runs `lyx init` or `lyx fabric reconcile`. This is accepted, not suppressed, and it is documented as an upgrade step.
- **The concrete blast radius, stated so nobody meets it as a surprise:**
  - `lyx fabric status` reports the pair **not in sync**, with the reason naming `_pattern`.
  - `lyx fabric reconcile` reports `ReconcileActionJunctionRepointed` rather than `AlreadyHealthy` — and repairs it, so reconcile *is* the remedy.
  - `loom`'s preflight fails `CheckJunction`, which sets `check3BlocksSeed` and **blocks the run**.
  - **This repo's own live worktrees are affected**, including whichever one lands this change.
- **Remedy:** `lyx init` (idempotent; wires the missing junction and materialises the weft directory) or `lyx fabric reconcile` (repairs it in place). One invocation per worktree.
- **Why not suppress it instead** — e.g. by treating a missing `_pattern` junction as healthy for a grace period: the junction genuinely *is* missing, and a health check that lies about a missing junction is the exact fault the junction-health-check generalisation exists to remove. A blocked preflight that names the fault and has a one-command remedy is the correct behaviour, not a regression.
- **Note the asymmetry with the pathspec migration, which is deliberate:** the junction side self-heals on the next `init`/`reconcile` and reports loudly until it does; the pathspec side never self-heals and reports nothing. Different because `WireJunctions` owns junction state outright, while `pathspec` is an operator-editable config value `configsync` must not overwrite.
- **Rejected — declare content persistence out of scope:** would ship wiring that provably cannot carry content, requiring a second task before PATTERN is usable at all.
- **Rejected — a separate `pattern_pathspec` config key:** more schema for configurability nobody has asked for.

### undo-leaves-pattern-content

- **Decision:** `lyx init --undo` removes the `_pattern` **junction** and its git-exclude entry, but does **not** touch weft `_pattern/` content. Only `_lyx` content is cleared. The weft commit message and pathspec stay `_lyx`-scoped.
- **Rationale:** `undo.go:72-96` does `os.RemoveAll(WeftLyxDirFor(slug))`, commits, and **pushes** the deletion. That is right for `_lyx`, which is lyx's own runtime state. It would be badly wrong for `_pattern`: PATTERN content is the host repo's hand-authored invariants, and deactivating lyx must not destroy the repo's own føringer and push that deletion to the remote where it cannot be casually undone.
- **`UndoResult` consequence:** the struct's `LyxJunction string` field is CLI-observable and now under-reports — it must name both junctions removed (mirroring `UnwireResult.JunctionsRemoved`), while `WeftContent` continues to describe `_lyx` only.
- **Rejected — symmetric deletion of both:** conceptually tidy, destructive in a way that survives a push.
- **Rejected — a `--purge-pattern` flag:** new CLI surface for a corner case nobody has needed; the operator can delete the directory themselves.

### no-machine-gate

- **Decision:** the checklist is prompt text only. No verification that the agent actually read `PATTERN.md`.
- **Rationale:** consistent with how every other instruction in these prompts is handled today. Nothing else in the repo machine-verifies prompt compliance.
- **Rejected — a report echo** (implementer/burler must write a `pattern-read:` line that Go validates): touches the report parsers in builder, webster and burler, and an agent can write the line without having read the file, so it buys the appearance of enforcement rather than enforcement.
- **Rejected — gating in the verify command:** the verify command is owned by the plan, not by PATTERN.

### call-site-plumbing

- **Decision:** `Directive` takes a `*hubgeometry.Layout`, **not** a worktree root — signature `Directive(l *hubgeometry.Layout, role Role) string` — and resolves the file through a new `l.PatternFileHere()` that mirrors the existing `HostLyxLinkHere()`. It is called **one level up** from the pure string-composing functions, at the layer that already holds a Layout. The already-computed directive string is then passed **down** as an ordinary parameter: `burlerengine.composePrompt` and `loomengine.composePlanPrompt` receive a `patternDirective string`, never a path or a Layout.
- **Rationale — why a Layout and not a worktree root.** The junction does not sit at the worktree root. `HostLyxLinkHere()` (`hubgeometry.go:679`) is `filepath.Join(WorktreeRoot, RelPath, LyxDirName)`, and `HostLyxLink(slug)` (line 524) is `filepath.Join(Hub, slug, RelPath, LyxDirName)` — both carry `RelPath`. The obvious sources at the call sites (`deps.WorktreeRoot`, webster's `worktreeRoot` parameter) are worktree-root-anchored and omit `RelPath`. In any nested-hub geometry — `RelPath != "."` — a root-anchored stat misses the file entirely and **PATTERN renders silently as inactive in all five agents**, which is the worst possible failure mode for this mechanism: no error, no signal, constraints simply gone. Taking a Layout makes the RelPath-correct path the only reachable one.
- **Rationale — why the composers stay pure.** `composePrompt(p *Profile)` and `composePlanPrompt(decisionRecordPath, planDir, overviewPath string)` are pure string functions with no filesystem access today; keeping them that way keeps them trivially testable.
- **Concrete availability at each of the five sites** (verified during exploration):
  - `builderengine/spawn.go:471` — `SpawnDeps.Layout *hubgeometry.Layout` already present (`spawn.go:94`).
  - `websterengine/render.go:123` (`RenderForkPrompt`) — takes `worktreeRoot string` today. **The parameter is replaced by the Layout**, so there is one source rather than two. Callers `beginbatch.go:212` and `recoverbatch.go:216` pass `deps`.
  - `websterengine/render.go:306` (`RenderMasterPrompt`) — no root or Layout parameter today; gains a Layout. Caller `runlevel.go:502` passes `deps`.
- **Critical: `{{.worktree_root}}` must keep rendering `l.Cwd`, not `l.WorktreeRoot`.** The `Deps.WorktreeRoot` field is misleadingly named — every caller assigns `c.layout.Cwd` to it (`webstercli/beginbatch.go:103`, `buildercli/run.go:61`, `buildercli/spawnbatch.go:143`), and `render.go:127` feeds that straight into the marker. At `RelPath != "."` — the exact geometry this whole plumbing decision exists for — `Cwd != WorktreeRoot`, so filling the marker from `Layout.WorktreeRoot` after the swap would silently change what the fork prompt calls its worktree root. Fill it from `l.Cwd` and the rendered value is byte-identical to today's.
- **The two anchors then agree by construction.** `PatternFileHere()` resolves `WorktreeRoot+RelPath`, which equals `Cwd` for any `Resolve`-built Layout — so the directive's active-check and the `worktree_root` the prompt states are anchored at the same directory. That is the correct outcome, but it holds because both are Cwd-equivalent, **not** because `WorktreeRoot` and `Cwd` are interchangeable. Renaming the misleading `Deps.WorktreeRoot` field is out of scope; mill-plan should leave it alone and simply not trust its name.
  - `burlerengine/prompt.go:22` (`composePrompt`) — caller `engine.go:102` has `e.layout` (already used at `engine.go:98`).
  - `loomengine/plan.go:42` (`composePlanPrompt`) — caller `PlanSpec` at `plan.go:87` takes `layout *hubgeometry.Layout` directly.
- **Open plumbing detail for mill-plan — correct type names:** webster's Deps structs are `BeginDeps` (`beginbatch.go:68`), `RecoverDeps` (`recoverbatch.go:66`), `RecordDeps` (`recordbatch.go:46`) and `RunDeps` (`runlevel.go:126`) — **not** `BeginBatchDeps` / `RecoverBatchDeps` / `RecordBatchDeps`, which do not exist. Where one lacks a `Layout` field, add it rather than reconstructing a Layout inside the engine.
- **`Directive` with a nil Layout returns the empty string** — inactive, no panic. Several Deps structs are assembled field-by-field by CLI callers that could leave `Layout` unset, and a nil dereference inside prompt assembly would take down all five agent paths for a programming slip that has nothing to do with PATTERN. Pinned by a test, alongside the zero-`Role` case.
- **Rejected — `Directive(baseDir string, role Role)` with `baseDir` documented as `WorktreeRoot+RelPath`:** less invasive, but it pushes the RelPath join out to five call sites — which is precisely the mistake this gap caught.
- **Rejected — `Directive(patternFilePath string, role Role)`:** maximally pure, but the call sites need a Layout to derive the path anyway, so it buys nothing and moves a geometry decision out of the one package allowed to make it.

## Technical context

### `internal/stencil` — the fill leaf

`internal/stencil/stencil.go` is 129 lines, standard-library only, and has one exported function. Its guarantee: `unfilledTopLevelMarkers` (line 87) walks **only** `t.Tree.Root.Nodes` — depth 0 — and collects every bare `{{.X}}` whose value is absent, empty, or whitespace-only, reporting all of them in one sorted error before executing anything. Markers reached only inside a taken `{{if}}`/`{{with}}`/`{{range}}` branch are instead caught at execution time by `Option("missingkey=error")` (line 43), which fires only on an **absent** key — a present-but-empty branch-internal value renders as a silent blank.

`FillOptional` must therefore do two things, not one: skip listed names in `unfilledTopLevelMarkers`, **and** ensure a listed name that is absent from `values` does not trip `missingkey=error`. The simplest correct implementation is to copy `values` and seed every listed-but-absent optional name with `""` before executing, leaving the caller's map untouched.

`stripLeadingComment` (line 70) drops a leading `<!-- … -->` banner before parsing. Every template asset starts with one; those banners must be updated (see Testing).

### `internal/hubgeometry` — the geometry owner

`HostJunction` (line 690) is `{Name, Link, Target}`. `HostJunctions(slug)` (line 703) currently returns a one-element slice for `_lyx`. `IsReservedHubName` (line 393) switches on `LyxDirName, "_raddle", BoardDirName, "_portals", "_launchers"`. The weft-side accessors to mirror are `WeftLyxDirFor(slug)` (line 651) and `WeftRaddleDir()` (line 658); the host-side one is `HostLyxLink(slug)` (line 522).

**Critical interaction:** `internal/hubgeometry/enforcement_test.go:224` lists the banned geometry tokens (`_board`, `-weft`, `-HUB`, `_portals`, `_launchers`, `_raddle`, `_lyx`). Adding `_pattern` to that list — which the Hub Geometry Invariant requires — means `internal/pattern` **cannot** build `filepath.Join(root, "_pattern", "PATTERN.md")` itself. The path must come from a `hubgeometry` accessor. This is forced, not optional. `"PATTERN.md"` is not a geometry token, but the whole path is cleanest kept in `hubgeometry` as `PatternFile(baseDir)` / `PatternFileHere()`.

**What that guard does and does not catch.** `hasGeometryLiteralInConstructionContext` (`enforcement_test.go:237-303`) flags a matching literal in exactly three positions: a `filepath.Join(...)` argument, an operand of a binary `+`, and a string const declaration value — production files only, whole-token equality. It does **not** flag a literal in a comparison or in a git-pathspec slice literal, and `CONSTRAINTS.md` explicitly carves both out. That carve-out is what makes `status.go`'s new work legal: its `git ls-files -- _lyx _raddle _pattern` pathspec slice and its `strings.HasPrefix(tracked, "_pattern")` comparison both keep the literal without violating the invariant.

**`RelPath` is the trap.** Every junction accessor carries it — `HostLyxLink(slug)` is `Join(Hub, slug, RelPath, LyxDirName)` (line 524), `HostLyxLinkHere()` is `Join(WorktreeRoot, RelPath, LyxDirName)` (line 679). Any new PATTERN accessor must mirror that, and any caller reaching for a bare worktree root is wrong in nested-hub geometry.

### `internal/fabricengine` — junction machinery

`WireJunctions` (`junction.go:40`) = `seedLyxJunction` + `seedGitExclude`; both already loop over `HostJunctions(slug)` and are individually idempotent, so **neither needs changing**. `seedLyxJunction` creates, verifies, or re-points; it refuses a real (non-link) directory as host-pristine violation.

The unwire side is asymmetric and is the actual work: `unseedLyxJunction` (`junction.go:191`) hardcodes the single `_lyx` junction, and `UnwireResult.JunctionRemoved` is a single bool to match; on a junction error it returns a zero result (`junction.go:163`), which becomes inaccurate once one junction may already have been removed. `unseedGitExclude` (line 254) already iterates correctly.

**The junction health check is open-coded in three places, all `_lyx`-only.** `checkJunctionHealth(hostLink, weftLyxDir)` lives at `reconcile.go:313` and is called from `reconcile.go:148` (gating repair) and `status.go:150` (feeding `PairStatus.JunctionHealthy`/`JunctionReason` and the in-sync verdict). `drift.go:73-94` does not call it at all — it re-implements the same lstat/IsLink sequence inline for loom's preflight, and `drift.go:92` **deliberately repeats the literal `"host _lyx is not a junction"`** so status, reconcile and `PairInSync` describe that fault identically (its own comment says so). Repair, by contrast, goes through the already-generic `WireJunctions` — so detection is narrower than repair at all three sites.

`removeHostJunction` (`weftwiring.go:124-129`) is `HostLyxLink(slug)`-only and is called as step (5) of `Remove` (`remove.go:91`). Step (6) is `fslink.RemoveLinksIn(target)`, a **root-level-only** safety net — which is exactly why step (5) exists, per `remove.go:38-39`.

`status.go:199` scans the host index with `git ls-files -- _lyx _raddle`; `_lyx` matches offer an automated restore, `_raddle` matches are report-only *because* no junction exists for it. Once `_pattern` has a junction, its pollution should be treated like `_lyx` (restorable), not like `_raddle`.

`template.yaml:2` is fabric's config template and carries `pathspec: _lyx  # directory path(s) relative to worktree root, whitespace-separated`. This is the single place the default weft-staging pathspec is declared.

**`CommitWeft`'s no-match tolerance is whole-pathspec, not per-entry.** `weftgit.go:276-284` catches `"did not match any files"` from `gitrepo.StageAndCommit` and converts it to `("", false, nil)` — a silent no-op, by design, for "nothing of ours to stage". With a single-entry pathspec that is correct. With `_lyx _pattern` it is a trap: `git add` fails as a unit when *any* entry matches nothing, so an empty `_pattern` suppresses the `_lyx` commit too, and the swallow hides it. Git tracks files, not directories, so a materialised-but-empty `weft/_pattern/` still matches nothing.

### `internal/initengine` — the actual junction caller

`Init(cwd)` (`init.go:49`) resolves the layout, refuses if there is no weft pairing, calls `fabricengine.WireJunctions(l, slug)` at line 67, then `MkdirAll(cwd/_lyx)` at line 82 — which lands in weft *through* the junction, materialising the weft-side directory.

**`Init` is the only `WireJunctions` caller that materialises anything.** `checkout.go:152` and `reconcile.go:153` call it without any `MkdirAll`, which is what motivates the weft-target-materialisation decision above.

`undo.go:53` calls `UnwireJunctions` and consumes `UnwireResult`, so it changes with the result shape. `undo.go:60-101` then clears weft content: it stats `WeftLyxDirFor(slug)`, `RemoveAll`s it, commits with `ScopedPathspec(l.RelPath, []string{hubgeometry.LyxDirName})` and message `"lyx init --undo: clear _lyx"`, and pushes unconditionally. `UndoResult` carries `LyxJunction string` and `WeftContent string` (`"cleared"` / `"not_present"`), both CLI-observable.

`fslink.CreateDirLink` does not require the target to pre-exist (Windows creates a raw reparse point; Linux creates a symlink), but `seedLyxJunction`'s *idempotent re-check* path calls `filepath.EvalSymlinks(target)` at `junction.go:81` and hard-errors at line 83 if the target is missing. That asymmetry — creation tolerant, re-check strict — is exactly the trap the materialisation decision closes.

### The five prompt-assembly sites

There is **no shared prompt-assembly layer**; each engine builds its own `map[string]string` inline. Exact locations and their existing marker counts (each template banner states its own count and must be updated):

| Template | Fill site | Markers today |
|---|---|---|
| `builderengine/implementer-template.md` | `spawn.go:471-478` | 5 |
| `burlerengine/review-prompt-template.md` | `prompt.go:22-34` | 9 |
| `websterengine/fork-template.md` | `render.go:123-132` | 6 (one branch-internal) |
| `websterengine/master-template.md` | `render.go:306` | 7 |
| `loomengine/plan-template.md` | `plan.go:42-48` | 3 |

`RenderForkPrompt` and `RenderMasterPrompt` are **exported and called from outside the package's own compilation unit** — `internal/websterengine/template_test.go` is an external `websterengine_test` package and calls `RenderForkPrompt` at lines 484, 498, 522, 536, 559, 578, 603, 612, 637 and 648. Changing their signatures is therefore not a package-internal edit; all ten call sites break and are listed as certain breakages in the Testing section.

### Existing conditional-marker precedent

`websterengine/fork-template.md:30-33` wraps `{{.rename_mechanic}}` in `{{if .rename_mechanic}}`, and `render.go:118-120` sets it to `""` when the batch has no Moves-bearing card. `template_test.go:156` seeds it as `""` in the marker-values map and excludes it from the per-marker deletion sweep. This is the working precedent that shows the `{{if}}` route is viable — and the reason the design doc's "first genuinely-conditional token" claim is wrong. It was considered and rejected (see the stencil-optional-marker decision); the new `FillOptional` does **not** retroactively change `rename_mechanic`, which stays as it is.

## Constraints

From `CONSTRAINTS.md`, in force for this task:

- **Hub Geometry Invariant.** `_pattern` becomes a geometry token owned solely by `internal/hubgeometry`. No other package may use the literal in a path-construction context (a `filepath.Join` argument, a `+` operand, or a string `const` value). Machine-enforced by `internal/hubgeometry/enforcement_test.go` (`TestEnforcement_GeometryLiterals`) on every `go test`; the token must be added to that test's list in the same commit as the accessors. Geometry is structural, never config- or env-overridable — there is no "pattern dir name" config key.
- **CLI / Cobra Invariant.** This task adds no new CLI module and no new subcommand, so the `Command()`/`RunCLI` seam is untouched. But the observable behaviour of `lyx init`, `lyx init --undo`, `lyx fabric reconcile`, `lyx fabric status`, `lyx fabric remove` and `lyx fabric checkout` all change — init creates a second junction and a `_pattern/` directory, undo removes a second junction while deliberately preserving `_pattern` content, reconcile and status detect and repair a second junction, remove tears one down, and checkout is the third `WireJunctions` caller (`checkout.go:152`) so it now wires and materialises both — so every affected `Short`/`Long` must be re-read and updated. `fabric checkout`'s existing help already speaks generically of "junctions" in the plural, so it is likely a re-read-and-confirm rather than a rewording — but confirming it is still the obligation. Help accuracy is a review-blocking obligation, and the reworded host-pristine error for a pre-existing `_pattern/` directory is part of it. `lyx init --undo`'s JSON output changes shape (`lyx_junction` → `junctions_removed`), which is a breaking output change and must be called out in the help text. Errors stay on the `internal/output` envelope.
- **lyxtest Leaf Invariant.** `internal/lyxtest` must not gain a PATTERN helper (see the testing decision below); it stays a leaf importing only stdlib and `internal/hubgeometry`.
- **Test Tier Purity Invariant.** This task mandates a dozen tests that must spawn real git or copy fixture trees — `remove` at `RelPath != "."`, the `:(exclude)` pathspec case, the weft-pathspec tolerance cases, materialisation, undo. **Every new test file containing `gitexec.RunGit`, `exec.Command`/`exec.CommandContext`, or `lyxtest.Copy*` must carry a `//go:build integration` constraint as its first non-empty line**, or `cmd/lyx/tierpurity_test.go` (`TestTierPurity_UntaggedTestsSpawnNothing`) fails on every plain `go test`. The match is a raw substring, so even a comment mentioning one of those tokens in an untagged file trips it.
- **Hermetic Git Test Environment Invariant.** Any new **package** whose tests spawn git — directly or through a `lyxtest` fixture helper — needs a `TestMain` calling `lyxtest.HermeticGitEnv()` before `m.Run()`. `fabricengine` and `initengine` already have one (`fabricengine/testmain_test.go`); a new git-spawning package would need its own. Enforced by `cmd/lyx/hermeticenv_test.go`, which scans **every** test file regardless of build tag.
- **`internal/pattern`'s own tests spawn nothing and stay untagged.** The package is a leaf over `os.Stat` and `t.TempDir()`; keeping its whole suite in Tier 1 is deliberate, and neither invariant above applies to it.
- **Documentation Lifecycle.** This task adds a module (`internal/pattern`) and changes observable CLI behaviour, so `docs/overview.md`'s module table and `CONSTRAINTS.md` update **in the same commit**. `manifest/designs/pattern.md` is corrected in the same commit too (see below). `manifest/roadmap.md` moves only if the PATTERN item is completed or added — the wiring landing is a roadmap-relevant event for the Planned `PATTERN.md` item.

Prose that goes stale the moment the default pathspec widens or the second junction lands, and must be corrected in the same commit:

- `internal/fabriccli/weft_verbs.go:167` — *"Staging is scoped to the directories listed in the fabric config (default: `_lyx`)"*, in `lyx fabric commit`/`push`/`sync`'s `Long`.
- `internal/fabricengine/template.yaml:2` — the inline comment *"`_lyx` is the default"*.
- `docs/overview.md:96-99` — the junction list, which names exactly one junction today.
- **Pre-existing error in the same passage, fixed while we are in it:** `docs/overview.md:97` claims a `<host>/_raddle` → `<hub>/<slug>-weft/_raddle` junction exists. It does not. `fabricengine/status.go:194` states plainly that *"no junction is wired for `_raddle` in this release"*, and `HostJunctions()` returns a single `_lyx` entry. Leaving that line while adding `_pattern` beside it would produce a list of three junctions where two exist.

New invariant introduced by this task, to be recorded in `CONSTRAINTS.md` in the same commit:

- **Pattern Leaf Invariant.** `internal/pattern` production code imports only the standard library and `internal/hubgeometry`. The reverse import (`pattern` → any feature package) is never allowed. Enforced by `internal/pattern/leaf_enforcement_test.go` (`TestLeafInvariant_AllowlistOnly`), modelled on `internal/modelspec/leaf_enforcement_test.go`, `internal/tokenvocab/leaf_enforcement_test.go` and `internal/codeintelengine/leaf_enforcement_test.go`.

Design-doc corrections to land in `manifest/designs/pattern.md` in the same commit:

1. The claim that PATTERN is "the first genuinely-conditional token in the system" is wrong — `websterengine`'s `rename_mechanic` predates it. The optional-marker extension is a deliberate design choice, not a forced one; the rationale must be restated accordingly.
2. "The `_pattern` junction is `fabric`'s responsibility" is imprecise: `fabricengine` owns the primitive, `initengine.Init` is the caller. `fabric add` explicitly does not wire junctions.
3. "sibling of `_lyx` and `_raddle`" overstates `_raddle`: `_raddle` has no junction today, so `_pattern` is the first *second* junction rather than a third peer.
4. The doc says `fabric` "**always** creates the `_pattern/` directory in `weft` … and the junction". Neither half is true of `fabric` as the code stands: `fabric add` wires nothing, and no `WireJunctions` caller except `Init` materialises a weft directory. State the corrected split (hubgeometry declares, `seedLyxJunction` materialises, `initengine` calls).
5. The doc is silent on weft persistence. Note that activation also requires `_pattern` in fabric's weft pathspec, or content never leaves the machine.
6. The Open Questions section is resolved by this discussion and should be replaced with the decisions.

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
- An optional marker whose value is **whitespace-only** renders as nothing — not as its whitespace. This is precisely the case a naive copy-and-seed implementation gets wrong.
- The caller's `values` map is not mutated by the optional-seeding step.
- Determinism: repeated calls produce identical output and identical error text (the existing suite already asserts this shape for `Fill`).

### `internal/pattern` — TDD

New package, new test file, using `t.TempDir()` per the fixture decision. Scenarios:

- `_pattern/PATTERN.md` present ⇒ non-empty directive for each role.
- `_pattern/` present but `PATTERN.md` absent ⇒ empty string.
- Neither present ⇒ empty string.
- `PATTERN.md` present but **empty** ⇒ **active** (non-empty directive) — asserting the accepted degenerate case explicitly so a later "improvement" cannot silently flip it.
- `PATTERN.md` present as a **directory** rather than a regular file ⇒ inactive.
- **`RelPath != "."`** — a Layout whose `RelPath` is a nested subdirectory finds `PATTERN.md` at `<WorktreeRoot>/<RelPath>/_pattern/PATTERN.md`, and does **not** find one planted at `<WorktreeRoot>/_pattern/PATTERN.md`. This is the regression guard for the silent-inactive failure mode; without it the bug is invisible.
- A non-`IsNotExist` stat error ⇒ **active** (fail loud in the prompt, never silently inactive). Exercised by whatever mechanism is portable — an unreadable parent directory on POSIX, or by injecting the stat via an unexported seam if a portable filesystem trick is not available on Windows. If neither is portable, state that in the plan rather than skipping the case silently.
- The `RoleImplementer`, `RoleReviewFix` and `RoleOrchestrator` variants are pairwise different, and each contains the literal `_pattern/PATTERN.md` pointer.
- Each variant begins with its own `##` heading, so an inactive render leaves no orphan heading.
- An unknown/zero `Role` value has a defined behaviour — pin it rather than leaving it to `switch` fallthrough.
- **A nil `*hubgeometry.Layout` returns `""`** and does not panic.
- A `leaf_enforcement_test.go` asserting the import allowlist, modelled on the three existing ones.

### `internal/hubgeometry`

- The new accessors return the expected joins, **including a `RelPath != "."` case** for each.
- `IsReservedHubName("_pattern")` is true.
- `HostJunctions(slug)` returns two entries with the expected `Name`/`Link`/`Target` for each.
- `HostJunctionsHere()` returns the same two junctions resolved against `WorktreeRoot+RelPath` rather than `Hub`/slug, and agrees with `HostJunctions(slug)` for the ordinary case where the two anchors coincide.
- `TestEnforcement_GeometryLiterals` passes with `_pattern` added — the guard catches a stray `"_pattern"` in a `filepath.Join` argument, a `+` operand, or a string const value, in production files, anywhere outside `internal/hubgeometry`.

### `internal/fabricengine`

- Wire then unwire with two junctions: both removed, both exclude lines removed, `JunctionsRemoved` names both.
- **Partial unwire failure:** the second junction fails to unwire after the first succeeded — the returned `JunctionsRemoved` names the first, and is not the zero value. This is the bug the generalisation fixes, so it needs its own test.
- Idempotency: wiring twice is a no-op; unwiring an already-unwired worktree is the legitimate no-op case, not an error, and reports an empty `JunctionsRemoved`.
- The existing refusal behaviours still hold **per junction**: a real (non-link) directory at either host path is refused; a dangling or wrong-target link is re-pointed.
- **Materialisation:** `WireJunctions` called with no weft-side `_pattern/` present creates it, so the junction resolves immediately. Then a second `WireJunctions` on the same worktree succeeds rather than erroring at `junction.go:83` — the checkout/reconcile path that is broken today.
- **Health check, one case per site — all three must be covered separately, since `drift.go` does not share `checkJunctionHealth`'s code path:**
  - `reconcile`: healthy `_lyx` + missing or mis-pointed `_pattern` ⇒ repaired, **not** `ReconcileActionAlreadyHealthy`.
  - `status`: the same state ⇒ `JunctionHealthy` false, `JunctionReason` naming `_pattern`, and the pair not reported in-sync.
  - `drift`: the same state ⇒ drifted, with the reason string matching `checkJunctionHealth`'s wording for that fault (the alignment `drift.go:92` exists to preserve).
- **`loomengine` preflight classification, all three drift shapes:** a missing, non-link and mis-pointed `_pattern` junction each classify as `CheckJunction` **and** set `check3BlocksSeed` — not `CheckWeftSync`. This is the regression guard for the reason-string reword; `preflight_integration_test.go:316` already covers the `_lyx` shape and must keep passing.
- **Legacy-worktree upgrade:** a worktree with `_lyx` wired and no `_pattern` junction reports not-in-sync from `status`, is repaired by `reconcile`, and blocks loom's preflight — then passes after one `lyx init`.
- **`remove`:** a worktree with both junctions wired at `RelPath != "."` — after `Remove`, no junction survives. At `RelPath == "."` the root-level safety net masks the bug, so the nested case is the one that matters.
- **`remove` best-effort loop:** with the first junction in a state that makes its removal fail, the second is still removed. This is the guard against an abort-on-first-error implementation written by analogy to `unseedLyxJunction`.
- **Weft-pathspec tolerance — the regression guard for the whole task:** with `pathspec: _lyx _pattern` and **no files under `_pattern`** (an existing but empty directory, and separately a wholly absent one), a `_lyx` change still commits. Without this test the failure is invisible: `CommitWeft` returns no error either way, so only asserting that the commit actually happened catches it.
- **Pathspec filter, one case per clause of the predicate:**
  - An **untracked** new file under `_pattern` counts as a match and is committed — the first-PATTERN-commit case.
  - An **index-only** path (present in the index, deleted from the worktree) counts as a match — the `undo.go` case; assert the deletion still commits.
  - A pathspec carrying `:(exclude)` entries passes them through untouched, and the excluded artifacts stay unstaged. **Mandatory** — this is the only guard against a filter that behaves on plain paths and silently re-stages machine-local files.
- Partial state: `_lyx` wired but `_pattern` not yet (the upgrade path for a worktree initialised before this change) — wiring must complete without error.
- `status.go`'s pollution scan reports a tracked `_pattern/` path and offers the same restore remedy as `_lyx`.
- A pre-existing **real** `_pattern/` directory in the host repo is refused, with the reworded error naming the remedy.
- Existing tests that assert a one-element `HostJunctions` or a single-bool `UnwireResult` will fail and must be **updated, not deleted**. Certain breakages, verified: `internal/hubgeometry/weft_test.go:212-290` (`wantJunctionCount: 1`, and index-`[0]` access into the slice) and `internal/hubgeometry/hubgeometry_test.go:590-602` (`want 1`). Likely further sites: `fabricengine/junction_repoint_test.go`, `fabricengine/reconcile_stale_registration_test.go`, and the `initengine` undo tests.
- **`internal/loomengine` is a certain breakage site**, not merely a likely one: `preflight.go:127-134` classifies `PairInSync`'s reason by prefix, and `preflight_integration_test.go:316` pins the resulting `CheckJunction` classification. Both change with the reason-string reword.

### `internal/initengine`

- After `Init`, both junctions resolve and both weft directories exist.
- Second `Init` on the same worktree is idempotent.
- `Init --undo` removes both junctions and both exclude entries, and `UndoResult` names both.
- **`Init --undo` leaves weft `_pattern/` content in place** while clearing `_lyx` — seed a `PATTERN.md`, run undo, assert the file survives and that no deletion of it was committed. This is the destructive-behaviour guard; it must exist.
- An `Init` on a worktree wired before this change adds `_pattern` without disturbing `_lyx`.
- **`InitResult.PatternDir` reports `"created"` on first run and `"exists"` on the second — and `LyxDir` must do the same.** This is the regression guard for the pre-wiring stat: if the stat stays after `WireJunctions`, both report `"exists"` on a first init and the test fails. `LyxDir`'s existing behaviour needs an explicit assertion too, since it is what silently changes.

### `internal/initcli`

- `lyx init --undo` emits `junctions_removed` as a list naming both junctions; the `lyx_junction` key is gone. `initcli_test.go:74` is updated, not deleted.
- `lyx init` reports the `_pattern` directory status.

### Weft persistence

- A `PATTERN.md` written through the junction is staged by the default pathspec and reaches a weft commit.
- The `pathspec: _lyx _pattern` default parses into two paths (the value is whitespace-separated, so a whitespace-splitting bug here is silent and would drop `_pattern`).

### The five engine template tests

Each of `builderengine`, `burlerengine`, `websterengine` and `loomengine` has a `template_test.go` with a `*MarkerValues()` map plus a per-marker deletion sweep that asserts `Fill` errors when any single marker is removed. For the affected templates:

- **Certain breakage, verified:** `internal/websterengine/template_test.go` calls `RenderForkPrompt` at lines 484, 498, 522, 536, 559, 578, 603, 612, 637 and 648. It is an external `websterengine_test` package, so the signature change breaks every one of them; they are updated, not deleted.
- Add `pattern_directive` to the marker-values map with a placeholder string.
- **Exclude it from the deletion sweep** — deleting an optional marker must *not* error. `websterengine/template_test.go:417` already does exactly this for `rename_mechanic`; follow that precedent, including its explanatory comment.
- Add a positive case per template: filling with an empty `pattern_directive` succeeds and the rendered output contains no leftover `{{`, no orphan `## Constraints` heading, and no stray blank-line block where the directive would have been.
- Add a positive case per template: filling with a non-empty directive places it in the rendered output before the first work instruction.
- Update each template's leading banner comment — the marker count ("requires all five non-empty") and the statement about conditionals — so the banner stays true. This is a documentation obligation, not cosmetic: those banners are what the next reader trusts.

### Test tiering

- Every new test file that spawns git or copies a fixture tree carries `//go:build integration` as its first non-empty line — `cmd/lyx/tierpurity_test.go` fails the build otherwise, on a raw-substring match.
- `internal/pattern`'s tests stay **untagged**: `os.Stat` and `t.TempDir()` only, no spawn, no fixture copy.
- Any new git-spawning package gets a `TestMain` calling `lyxtest.HermeticGitEnv()`; the existing `fabricengine` and `initengine` ones already satisfy this.

### Repo-wide

- `go test ./...` must pass — plain, and with `-tags integration`. `go vet ./...` clean.
- No new `_pattern` string literal **in a path-construction context** (a `filepath.Join` argument, a `+` operand, or a string const value) in production code outside `internal/hubgeometry` — enforced automatically by `TestEnforcement_GeometryLiterals`. Comparisons and git-pathspec slice literals are exempt by the invariant's own carve-out, which is what permits `status.go`'s new pathspec entry and prefix check.

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
- **Q:** Where in each template does the marker sit? **A:** As early as possible, ahead of the work description. Refined in review round 1 to the mechanically checkable form: immediately **before the template's first `##` heading**. The block carries its own `##`, so any placement inside an existing section would terminate it and orphan its remaining prose.

### Review round 1 (2026-07-28)

- **Q:** Nothing stages weft-side `_pattern` — `template.yaml:2` defaults to `pathspec: _lyx`. Widen the default, or declare persistence out of scope? **A:** Widen it to `pathspec: _lyx _pattern`. Without it the mechanism provably cannot carry content, and `finalize.md` already anticipates `_pattern` travelling with weft merge-back. The `configsync` migration consequence for existing worktrees is documented rather than solved.
- **Q:** Does `lyx init --undo` delete weft `_pattern/` the way it deletes `_lyx`? **A:** No. It removes the junction and the exclude entry, but leaves the content. `_lyx` is lyx's runtime state; `_pattern` is the host repo's hand-authored invariants, and `undo` commits *and pushes* its deletions — destroying a repo's own føringer on deactivation, irreversibly, is not acceptable.
- **Q:** `Directive(worktreeRoot string, ...)` misses the file when `RelPath != "."`, since the junction sits at `WorktreeRoot+RelPath`. **A:** Take a `*hubgeometry.Layout` instead and resolve via a new `PatternFileHere()` mirroring `HostLyxLinkHere()`. The failure mode being avoided is the worst available one: PATTERN renders silently inactive in all five agents, with no error anywhere. A `RelPath != "."` test case is now mandatory.
- **Q:** `reconcile.go`'s junction health check is `_lyx`-only, so a broken `_pattern` junction reports healthy. Generalise or defer? **A:** Generalise, same commit. Detection must not be narrower than repair — `WireJunctions` already fixes any junction; only the check is blind.
- **Q:** `checkout.go` and `reconcile.go` call `WireJunctions` without materialising the weft target, so a `_pattern` link there dangles and the next re-check hard-errors. Where does the directory get created? **A:** Inside `seedLyxJunction` itself, before the link is created. One site instead of three, no ordering rule to forget, and it closes the same latent hole for `_lyx`.
- **Q:** The burler round both reviews *and* fixes, so a reviewer-only checklist omits the obligation not to write a violation in part B. **A:** Give burler a single combined variant. This collapses the role set to `RoleImplementer` and `RoleReviewFix` — a pure reviewer variant would have had no user, since burler is the only reviewing template in the set.
- **Q:** Should the directive be gated on whether burler's target is code rather than prose? **A:** No. Loomyard has no target-type classification, a file-extension heuristic would be new fragile logic whose misclassification silently drops the constraints, and invariants can govern prose anyway.
- **Q:** Should burler's round prompt be split into separate review and fixer templates, with the round pointed at the fixer file when it reaches part B? **A:** Good idea, deferred — filed as [issue #105](https://github.com/Knatte18/loomyard/issues/105). The real argument for it is contamination (part B's "fix everything you found" sits in context during part A and can suppress findings), and webster's `Read this file and follow it exactly: <path>` fork idiom is the in-repo precedent. But it restructures burler's prompt architecture independently of PATTERN, so folding it in would couple two unrelated changes. The combined `RoleReviewFix` variant divides cleanly along that seam later.

### Review round 2 (2026-07-28)

- **Q:** The junction health check is `_lyx`-only in two more places round 1 never named — `status.go:148` and `drift.go:73`. Extend, or document the exemption? **A:** Extend all three, and parameterise `checkJunctionHealth`'s hardcoded reason strings by junction name. `PairStatus` keeps its singular `JunctionHealthy`/`JunctionReason` shape — first-unhealthy-wins, reason names which — so `status`'s output shape does not change. Note `drift.go` re-implements the check inline rather than calling `checkJunctionHealth`, and duplicates one reason string verbatim on purpose, so the two must move together.
- **Q:** `lyx fabric remove` calls an `_lyx`-only `removeHostJunction`, leaving a nested `_pattern` junction behind. **A:** Generalise it. The step's own comment (`remove.go:38-39`) exists precisely because the root-level safety net misses nested junctions at `RelPath != "."`; leaving it narrow reintroduces the documented bug for `_pattern`.
- **Q:** With `pathspec: _lyx _pattern` and nothing under `_pattern`, `git add` fails as a unit and `CommitWeft` swallows it — so `_lyx` silently stops committing too. **A:** Make `CommitWeft` filter the pathspec to entries that match something, in `fabricengine/weftgit.go` and **not** in `internal/gitrepo` (which would forfeit parallel-safety with `native-clients`). This is not an edge case: git tracks files rather than directories, so a materialised-but-empty `_pattern/` matches nothing, and an empty `_pattern/` is the normal state for this whole task since content migration is out of scope. The widened pathspec and this tolerance must land together or neither should land.
- **Q:** An operator who hand-creates `_pattern/` in the host repo hits `seedLyxJunction`'s host-pristine refusal, which hard-fails init, checkout and reconcile and points at the hub-creator, which does not address it. **A:** Keep refusing — fabric never moves or deletes user content — but reword the error to name the real remedy (move the content into weft, or remove the host directory, then re-run `lyx init`) and document it.
- **Q:** What are the concrete result shapes? **A:** `UndoResult.LyxJunction string` → `JunctionsRemoved []string`, emitted as `junctions_removed` and replacing `lyx_junction` at `initcli.go:120` (with `initcli_test.go:74` updated); `InitResult` gains `PatternDir string`. Breaking change to `lyx init --undo`'s JSON, accepted — the alternative repeats the key per junction and does not scale.
- **Q:** The directive's pointer is the relative `_pattern/PATTERN.md`, but the active-check resolves `WorktreeRoot+RelPath`; the agent's cwd is `layout.Cwd`, so the anchors can disagree in a nested hub. **A:** Keep the relative pointer deliberately. It matches the existing prompt idiom (`_lyx/plan/`, `00-overview.md`), and interpolating an absolute path would make the marker value vary per worktree, defeating the fixed-string tests to guard a case no other prompt guards. *(Rationale corrected in round 3 — see below; the anchors normally cannot diverge at all.)*

### Review round 3 (2026-07-28)

- **Q:** Moving `MkdirAll` into `seedLyxJunction` makes `Init`'s `"created"`/`"exists"` unreachable, since `init.go:67` wires before the `init.go:75` stat — so `LyxDir` would report `"exists"` on a first init and the required `PatternDir` `"created"` test could never pass. **A:** Stat the weft-side target **before** `WireJunctions`, and derive both fields from that pre-wiring observation. Keeps the vocabulary meaning the only thing an operator can use it for, and avoids silently inverting the existing `lyx_dir` output.
- **Q:** The prescribed health-check loop source `HostJunctions(slug)` is Hub/slug-anchored, but all three sites use the Here-anchored pair, and `PairInSync(l)` carries no slug and is documented stateless. **A:** Add `HostJunctionsHere()`, mirroring the existing `HostLyxLinkHere()`/`HostLyxLink(slug)` and `WeftLyxDir()`/`WeftLyxDirFor(slug)` pairs. Detection loops over the Here form; wiring, unwiring and `remove` keep the slug form.
- **Q:** "Entries that currently match something" is undefined, and three readings break real callers. **A:** Predicate pinned: keep an entry if it begins with `:` (pathspec magic, never evaluated) **or** it matches in the worktree **or** the index. Untracked must count (the first PATTERN commit), index-only must count (`undo.go:93-94` commits a just-deleted `_lyx`), and `:(exclude)` entries from `buildercli`/`webstercli`/`perchcli` must pass through untouched or machine-local artifacts get re-staged. An `:(exclude)` test is mandatory.
- **Q:** Which help and docs go stale when the default pathspec widens? **A:** `fabriccli/weft_verbs.go:167`, the `template.yaml:2` inline comment, and `docs/overview.md:96-99`. Also fix a pre-existing error in that same passage: `overview.md:97` claims a `_raddle` junction that does not exist (`status.go:194`, `HostJunctions()`), which would otherwise leave the list naming three junctions where two exist.
- **Q:** Do existing worktrees pick up the widened pathspec? **A:** No — confirmed, not deferred: `configsync.ReconcileAll` → `yamlengine.Reconcile` keeps a present `pathspec:` value and adds no key, so an already-initialised worktree stays on `_lyx` forever and never persists PATTERN. No detection or warning surface is in scope; the consequence is documented instead.
- **Q:** Can the pointer's cwd anchor and the active-check's Layout anchor really diverge? **A:** Normally no — `Resolve` sets `RelPath = Rel(WorktreeRoot, Cwd)`, so `WorktreeRoot+RelPath == Cwd`; only the `RelPath`-hardcoding constructor at `hubgeometry.go:170-186` could differ. The round-2 rationale overstated this. The relative pointer stands, on prompt-idiom consistency alone.

### Review round 4 (2026-07-28)

- **Q:** How many reason strings actually hardcode `_lyx`? **A:** Six, not two. `checkJunctionHealth` has a third at `reconcile.go:341` ("points elsewhere"), and `drift.go`'s inline copy has two that name **no** junction at all (`:83`, `:110`) plus one that matches verbatim (`:92`). With a singular `JunctionReason` the string is the only carrier of *which* junction broke, so every one of the six is parameterised.
- **Q:** Is "`MkdirAll` before `CreateDirLink`" the right placement? **A:** No — it does not fix the case the decision exists for. `seedLyxJunction` reaches the hard error at `junction.go:83` from the link-already-exists branch, *before* either `CreateDirLink` call, so a dangling junction left by a pre-fix `checkout`/`reconcile` would still hard-error. Corrected to `MkdirAll(j.Target)` as the **first statement of each loop iteration**, before the `os.Lstat` re-check.
- **Q:** What happens when the filter drops every positive entry and only `:(exclude)` entries survive? **A:** `CommitWeft` must return `("", false, nil)` without calling `StageAndCommit` at all. Git reads exclusions with no positive pathspec as "everything except", so handing it an all-negative pathspec would stage the **entire weft worktree** — the filter would then be more destructive than the bug it fixes.
- **Q:** Are the webster render-function signature changes package-internal? **A:** No — that claim was wrong. `internal/websterengine/template_test.go` is an external `websterengine_test` package and calls `RenderForkPrompt` at ten line numbers, all now listed as certain breakages. Also decided: `worktreeRoot string` is **replaced** by the Layout rather than kept beside it, since `Layout.WorktreeRoot` already carries the value and two sources of one fact can disagree.
- **Q:** Do the mandated real-git tests conflict with a test-tier invariant? **A:** Yes, and it was missing from the Constraints section. Every new test file spawning git or copying fixtures carries `//go:build integration`, or `cmd/lyx/tierpurity_test.go` fails on a raw-substring match. `internal/pattern`'s own tests spawn nothing and stay untagged, deliberately. The Hermetic Git Test Environment Invariant is also now named.
- **Q:** Is "an existing-but-empty directory matches nothing, so `git add` dies" verified? **A:** No — it was asserted. Both shapes (empty directory, wholly absent) are already on the mandatory test list; the plan derives the assertion from real git rather than from this document. The filter decision is unaffected either way, since the wholly-absent case requires it regardless.

### Review round 5 (2026-07-28)

- **Q:** Does anything consume the junction reason strings we are rewording? **A:** Yes — `loomengine/preflight.go:127-134` classifies them by **prefix**, and `strings.HasPrefix(reason, "junction")` is what maps a fault to `CheckJunction` and sets `check3BlocksSeed`. The reworded strings match neither case and fall to `default`, so a broken junction would report as a weft-sync failure and stop blocking the seed check. Fixed by matching `strings.Contains(reason, "junction")` after the `"host on "` case; `internal/loomengine` added to the certain-breakage list.
- **Q:** Is `Layout.WorktreeRoot` really the value the render call sites pass today? **A:** No — that assertion was wrong. Every caller assigns `c.layout.Cwd` to the misleadingly-named `Deps.WorktreeRoot` field (`webstercli/beginbatch.go:103`, `buildercli/run.go:61`, `spawnbatch.go:143`), and it feeds `{{.worktree_root}}` directly. At `RelPath != "."` the two differ, so the marker must be filled from `l.Cwd` after the Layout swap to stay byte-identical. The active-check anchor agrees with it because `WorktreeRoot+RelPath == Cwd`, not because the two fields are interchangeable.
- **Q:** What happens to worktrees wired before this change, which have no `_pattern` junction? **A:** `status` reports not-in-sync, `reconcile` reports (and performs) a repair, and loom's preflight fails `CheckJunction` and blocks the run — including on this repo's own live worktrees. Accepted and documented rather than suppressed: the junction genuinely is missing, and one `lyx init` or `lyx fabric reconcile` fixes it. Suppressing it would reintroduce exactly the lying health check the generalisation exists to remove.
- **Q:** Master gets `RoleImplementer`, but `master-template.md:21` says "You never edit code yourself" — the verbatim reason given for excluding builder's orchestrator. **A:** Genuine self-contradiction. The criterion is restated as: writes code, **or** reviews code, **or** is a context-inheritance root whose in-session forks do. Master qualifies under the third clause and gets its own `RoleOrchestrator` variant worded for forking rather than editing. Builder's orchestrator still fails all three — its implementers are separate spawned sessions that each receive `RoleImplementer` directly, so there is no inherited context to seed.
- **Q:** Does the prescribed `FillOptional` implementation actually make a whitespace-only value render as nothing? **A:** No — copy-and-seed alone leaves `"   "` rendering verbatim. The copy step must apply the same `strings.TrimSpace` test `unfilledTopLevelMarkers` already uses and write `""`, so the package keeps one definition of "empty".
- **Q:** Are webster's Deps type names right? **A:** No — they are `BeginDeps`, `RecoverDeps`, `RecordDeps` and `RunDeps`; the `*BatchDeps` names do not exist. Corrected so mill-plan edits the right structs.
- **Q:** What does `Directive` do with a nil Layout? **A:** Returns `""`. Deps structs are assembled field-by-field by CLI callers, and a nil dereference inside prompt assembly would break all five agent paths over a slip unrelated to PATTERN.
