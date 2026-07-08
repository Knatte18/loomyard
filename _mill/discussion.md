# Discussion: Build burler - the review+fix round worker

```yaml
task: Build burler - the review+fix round worker
slug: internal-burler
status: discussing
parent: main
```

## Problem

lyx's orchestration spine is `shuttle ✅ → burler → perch → loom`. shuttle (spawn/drive one
agent over the file contract) and stencil (template filling) are built; the next layer is
**burler** — the round worker that performs ONE review+fix pass over an artifact and returns a
verdict. It is the module that moves the hand-executed review method (`docs/reviews/`) into Go:
A-review (independent findings, verdict to file) then B-fix (fix everything found) in a single
agent, no self-grading. perch (the loop) and loom (the orchestrator) cannot be built until this
round worker exists with a stable `{profile, prior-round files} → {verdict, review, fixer-report}`
contract.

Design is authoritative in `docs/modules/burler.md` (read it in full before planning). This
discussion records the decisions that doc left open, plus two scope changes decided in
discussion: **a debug CLI (`lyx burler run`) is un-deferred**, and **a sandbox suite
(`SANDBOX-BURLER-SUITE.md`) ships in this task**. The review *concepts* from mill (millhouse
repo) are reused: strict machine-readable verdict header, `[SEVERITY]`-tagged findings with
Location/Issue/Fix, "omit findings if none / never invent findings to pad", source-grounding,
and the target-vs-fasit split.

## Scope

**In:**

- `internal/burlerengine` — the whole round-worker library: `Profile`/`RunOpts`/`Result`/
  `Verdict`/`Finding` types, `New(shuttle, layout)` + `Run(profile, opts)`, prompt composition
  from an embedded template via `internal/stencil`, one `shuttle.Run` per round with
  `OutputFiles = [review, fixer-report]`, strict verdict/review-file parse, outcome mapping,
  `cluster-N > 0` → typed error.
- `internal/burlercli` — thin Cobra wrapper: `lyx burler run --profile <yaml>` + tuning flags;
  wires the real `shuttleengine.Runner` (mux + claudeengine); JSON envelope output. Full
  CLI/Cobra-invariant obligations (registration in `cmd/lyx/main.go`, `Short` everywhere,
  pinned-set test updates).
- Embedded prompt template asset (`review-prompt-template.md` inside the package) encoding the
  round discipline (sequencing rule, fix-everything, commit rules, clean-room hydration).
- Unit tests (fake shuttle), one build-tagged smoke test (real engine, toy chair/table fixture),
  and `tools/sandbox/SANDBOX-BURLER-SUITE.md` + `sandbox-burler-suite.cmd` launcher +
  `tools/sandbox` suite registration, with `**Covers:** burler`.
- Docs lifecycle: delete `docs/modules/burler.md`; fold durable design into the package doc
  comment + `docs/overview.md`; fix inbound links (`perch.md`, `loom.md`, `hardener.md`,
  roadmap, `docs/reviews/README.md` if any); add `### Deferred burler enhancements` to
  `docs/roadmap.md`; mark the burler half of roadmap milestone 11 done; add a **Review Round
  Invariant** to `CONSTRAINTS.md` (short).

**Out (deliberate — do not build):**

- **No cluster fan-out.** `cluster-N = 0` only; `N > 0` returns a typed validation error
  (gated on mux own-window, roadmap milestone 24).
- **No weft dependency, no weft commits.** burler returns artifact paths; committing them to the
  weft is the loop owner's job (Weft Git Invariant). burler never imports the weft module.
- **No bulk mode / provider caching** — far-future; recorded in the deferred-enhancements section.
- **No enforced tools-restriction on the shuttle `Spec`** — `tool-use` is prompt-level in v1
  (see Decisions). No shuttleengine changes at all in this task.
- **No perch concerns:** no round loop, no cap, no cycle detection, no cross-round
  canonicalization of finding ids. burler runs one round and exits.
- **No non-Claude engine work**; provider selection stays "whichever engine the caller wired
  into the Runner".
- **No parsing of the fixer-report** — burler returns its path; its internal format is prose
  (template suggests Fixed / Deferred-with-reason sections) with no Go-side schema.

## Decisions

### package-layout

- Decision: `internal/burlerengine` (domain kernel, no cobra) + `internal/burlercli` (thin
  wrapper). cli imports engine; engine never imports cli/cobra. burlerengine imports
  `shuttleengine` for types only (`Spec`, `Result`, `Outcome` — provider-invariant), never
  `claudeengine`; the real engine is wired in burlercli (and in the smoke test, which acts as
  the caller).
- Rationale: CLI/Cobra Invariant's `<module>cli`/`<module>engine` naming; Shuttle Provider-Seam
  Invariant.
- Rejected: engine-only library (original brief) — overridden in discussion: burler is a vital
  standalone part of lyx and must be testable alone, and the sandbox suite needs a black-box
  binary surface.

### profile-shape

- Decision: `Profile` is the content contract, one struct:
  - `Target` — `{Paths []string, Instructions string}`: what to review AND what B may fix.
    Paths are files and/or directories (a directory means everything under it); `Instructions`
    is free text for path-less targets (e.g. "review the diff against main + working tree").
    At least one of the two must be non-empty; every listed path must exist at `Run`
    (fail loud).
  - `Fasit` — same `{Paths, Instructions}` shape: the read-only source of truth the target is
    judged AGAINST (mill parallel: code↔plan, plan↔discussion, discussion↔brief). Same
    non-empty + existence validation — an empty fasit degenerates the review to an
    internal-consistency check and must be rejected.
  - `Rubric string` — markdown criteria + severity rules for this artifact type. Data, not
    code; caller owns the asset. Non-empty required.
  - `FixScope` — enum `{markdown, source}` (see fix-scope-commit-rules).
  - `ToolUse bool` — see tool-use-prompt-level.
  - `ClusterN int` — must be 0 in v1; `> 0` → typed validation error (e.g.
    `ErrClusterUnsupported`-style sentinel or typed error naming milestone 24).
  - `ReviewPath string`, `FixerReportPath string` — caller-supplied output paths (see
    artifact-paths).
  - `PriorReviews []string`, `PriorFixerReports []string` — optional hydration from earlier
    rounds (see prior-round-hydration).
- Rationale: "review anything: a file, a folder, a collection of files" while keeping burler a
  dumb transport (shuttle-style); structured enough to validate loudly.
- Rejected: paths-only (loses diff/free-text targets); free-strings-only (loses existence
  validation); typed target kinds (v1 doesn't need them).

### run-tuning-off-profile

- Decision: `Run(profile Profile, opts RunOpts)` with `RunOpts{Model, Effort string,
  Timeout time.Duration, Round string}` mapping 1:1 onto the shuttle `Spec` fields; zero values
  defer to engine/config defaults. `Spec.Role` is fixed `"burler"`; `Spec.Interactive` stays
  false (autonomous); `Display`/`KeepPane` are not exposed (defaults).
- Rationale: the design doc pins run-tuning as config-driven selection resolved by the caller,
  kept OFF the content profile. perch will vary model/effort per round, so tuning cannot live
  on `New`.
- Rejected: tuning on `New` (wrong granularity); accepting a partial `shuttleengine.Spec`
  (leaks shuttle's surface).

### shuttle-seam

- Decision: `type Shuttle interface { Run(shuttleengine.Spec) (shuttleengine.Result, error) }`
  (name/shape final-checked in plan), satisfied by `*shuttleengine.Runner` in prod and a fake in
  unit tests. Constructor: `New(shuttle Shuttle, layout *hubgeometry.Layout) *Engine` (layout is
  used to resolve relative profile paths against the worktree root, mirroring `Spec.validate`).
- Rationale: pinned by the design doc; keeps burler engine-agnostic and unit-testable.
- Rejected: burler constructing its own Runner (would drag mux/engine wiring into the library).

### artifact-paths

- Decision: the caller supplies `ReviewPath` and `FixerReportPath` in the Profile. burler
  validates them non-empty, passes them into `Spec.OutputFiles` (order: review, fixer-report)
  and into the prompt, and returns them in `Result`. burler never constructs `_lyx/...` paths.
  Note: `Spec.validate` REJECTS pre-existing output files — the caller must mint fresh paths
  per round; the CLI surfaces that error as-is.
- Rationale: Hub Geometry Invariant (geometry tokens are hubgeometry-only); perch owns round
  naming; keeps burler weft-blind.
- Rejected: burler deriving paths via a new hubgeometry helper — geometry surface for a
  consumer (perch) that doesn't exist yet.

### result-and-outcome-mapping

- Decision: `Result{Outcome shuttleengine.Outcome, Verdict, ReviewPath, FixerReportPath,
  SessionID, StrandGUID, LastAssistantMessage string}`. `Run` returns nil error for ANY
  shuttle outcome: `done` → verdict parsed and set; `asking`/`died`/`timeout` → Result carries
  the outcome (+ `LastAssistantMessage` for asking) with empty Verdict, caller branches on
  Outcome. Errors are reserved for hard failures: invalid profile, shuttle start failure, and
  verdict parse failure on a `done` run (fail loud — a defaulted verdict could terminate
  perch's loop on a malformed round).
- Rationale: mirrors shuttle's own Result; non-done outcomes are normal loop events for perch,
  not error ceremony.
- Rejected: typed errors per non-done outcome.

### review-file-format-and-parse

- Decision: the review file is YAML frontmatter (`---`-delimited) over prose, exactly as pinned
  in the design doc: `verdict: APPROVED | BLOCKING` plus `findings:` — a list where each entry
  carries `id`, `severity`, `location`, `summary`. Parse with `gopkg.in/yaml.v3` (already a
  dependency). **Strict everywhere, fail loud:**
  - missing file content / no frontmatter / unparseable YAML → error;
  - `verdict` must be exactly `APPROVED` or `BLOCKING`;
  - every finding must have all four keys non-empty; `severity` must be one of
    `BLOCKING | MEDIUM | LOW | NIT` (the hand-run method's vocabulary);
  - duplicate finding `id`s → error (perch keys on them);
  - `verdict: BLOCKING` with zero BLOCKING-severity findings → error;
  - `APPROVED` with findings is legal (non-blocking polish).
  Go types: `Verdict` string type with two constants; `Finding{ID, Severity, Location,
  Summary}`. Prose below the frontmatter is unconstrained (the template shapes it with mill's
  Location/Issue/Fix finding blocks).
- Rationale: "structured from birth"; perch's cycle detection is pure Go over stable keys; a
  malformed round must never look approved. Mill concept reuse with burler's pinned container
  (frontmatter instead of mill's fenced-yaml block).
- Rejected: lenient findings parse (silently degrades the stable keys).

### prompt-template

- Decision: one generic embedded asset (`go:embed`) filled via `stencil.Fill`. Required
  top-level markers (stencil enforces non-empty): target, fasit, rubric, review path,
  fixer-report path, fix-scope rules, tool-use rules, prior-rounds section. Go composes the
  variable blocks (path lists, commit rules, tool rules, prior-rounds text — "first round, no
  prior files" when empty) as marker VALUES so every marker stays top-level and validated;
  template conditionals are avoided for required content (stencil only empty-checks top-level
  markers). Static template content carries the round discipline, adapted from
  `docs/reviews/review-prompt-template.md` + mill's reviewer/fixer templates:
  - **Sequencing rule (BLOCKING):** job A complete and the review file saved to disk before
    touching any target file; findings are recorded, not fixed on sight.
  - **Fix everything (BLOCKING):** every recorded finding gets fixed in B — all severities
    including LOW and NIT ("severity affects how a finding is reported, not whether it is
    fixed"); the only legitimate deferral is something the agent cannot do alone this round,
    stated explicitly in the fixer-report's deferred section.
  - **Review-file format contract:** the exact frontmatter schema above, with mill's
    per-finding prose block (Location/Issue/Fix), "omit findings if none, never invent
    findings to pad", and source-grounding ("never fabricate file contents — read them").
  - **Fixer-report:** what B changed, plus a deferred-with-reason section. Prose, no schema.
  - **Clean-room hydration:** when prior-round files are listed — form your OWN findings
    first, consult prior reviews/fixer-reports only AFTER, for regression checks and deferred
    items.
- Rationale: the discipline is the load-bearing part of the module; stencil's top-level rule
  dictates the Go-composes-blocks pattern.
- Rejected: N per-artifact-type templates (rubric is data in the profile — one template
  suffices).

### tool-use-prompt-level

- Decision: `ToolUse` toggles prompt instructions only — A-phase "drive the real substrate
  (build/test/run)" vs "read-only analysis". NO enforced tools-restriction on the shuttle
  `Spec` in v1, and no shuttleengine changes.
- Rationale (structural, not just scope): the burler agent is ONE session doing A then B;
  claudeengine writes `settings.json` once at launch, so a launch-time restriction applies to
  the whole session — and B always needs Edit/Write (+ git for source scope), so the session
  can never be launch-restricted read-only. Even a text round writes (review file,
  fixer-report, the markdown target). Enforcement is only meaningful for cluster reviewers
  (separate, pure-review sessions — mill's READ-ONLY reviewer header), which are mux-gated and
  deferred; a generic `Spec` tools-restriction belongs to that task (recorded in deferred
  enhancements).
- Rejected: extending `shuttleengine.Spec` now.

### fix-scope-commit-rules

- Decision: `FixScope` enum `{markdown, source}` selects the commit rules Go splices into the
  prompt:
  - `source` → **commit-per-fix on the host repo**, message format
    `<module-or-target>: fix <finding-id> — <one-line what/why>`, green build/vet/test before
    each commit, **never push**.
  - `markdown` → fix the target file(s), **no git at all** from the agent: markdown targets
    will typically live in `_lyx`/weft where the Weft Git Invariant bans agent commits, and
    asking the agent to reason about host-vs-weft geometry is fragile. The loop owner commits.
- Rationale: hand-run discipline (crash-legible progress via `git log`) for source; Weft Git
  Invariant safety for markdown.
- Rejected: "commit once if the file is in the host repo, never under `_lyx`" (the design
  doc's literal line) — requires geometry reasoning inside the agent prompt.

### debug-cli

- Decision: `lyx burler` cobra group (with `clihelp.GroupRunE`) + `run` subcommand:
  `lyx burler run --profile <yaml> [--model M] [--effort E] [--timeout D] [--round R]`.
  The profile YAML file maps 1:1 onto `Profile` (target/fasit/rubric/fix-scope/tool-use/
  cluster-n/review-path/fixer-report-path/prior files); flags map onto `RunOpts`. The command
  wires `shuttleengine.NewRunner(mux, claudeengine.New(), layout, shuttleCfg)` exactly like
  shuttlecli, calls `Run`, and emits the `internal/output` JSON envelope (Result on Ok, typed
  errors — cluster-N, validation, parse — through `output.Err`). Requires an initialized
  worktree (`lyx init`), same as shuttle. No burler-specific config file / configreg entry —
  the profile is the file, tuning is flags.
- Rationale: user decision — burler is a vital standalone part of lyx and must be testable
  alone; the sandbox model is a black-box binary; the design doc itself called a debug CLI
  "useful in isolation for developing the round agent". Un-defers that item.
- Rejected: library-only (no black-box surface for the suite); driving the suite via
  `go test -tags smoke` from the source tree (breaks the black-box model, duplicates smoke).

### sandbox-suite

- Decision: `tools/sandbox/SANDBOX-BURLER-SUITE.md` + root `sandbox-burler-suite.cmd` launcher
  + suite registration in `tools/sandbox` (new suite name alongside `shuttle-suite`), following
  the shuttle suite's shape (pre-conditions: deploy, sandbox-build, live psmux + logged-in
  claude, `lyx init`; black-box rule; fingerprint header). Scenarios (sketch — plan refines):
  - **S1 (the toy round, `**Covers:** burler`):** fixture text file in the hub host repo with a
    chair/table color mismatch + rubric "the chair's color must match the table's color" +
    profile YAML; run `lyx burler run`; verify `BLOCKING` verdict with ≥1 finding in the
    frontmatter, target actually fixed, fixer-report written, JSON envelope sane.
  - **S2 (APPROVED path):** colors already match → `APPROVED`; non-blocking polish permitted;
    verdict parse still clean.
  - **S3 (error paths, black-box):** `cluster-n: 1` → typed error in the JSON envelope; empty
    fasit → validation error; pre-existing review path → shuttle's already-exists rejection.
- Rationale: user decision — burler must be sandbox-exercisable on its own; a registered module
  requires suite coverage or an allowlist entry anyway (Sandbox Suite Coverage), and the suite
  provides the `**Covers:** burler` tag.
- Rejected: allowlist exclusion.

### tests-smoke

- Decision: unit tests with a fake Shuttle cover the deterministic surface only: profile
  validation (incl. every fail-loud path), prompt composition (markers filled, discipline
  blocks present per fix-scope/tool-use/prior-rounds), spec construction (OutputFiles order,
  Role/tuning mapping), verdict/review-file parse (valid, malformed, every strictness rule),
  cluster-N>0 typed error, outcome mapping (done/asking/died/timeout). Simple, not exhaustive;
  never test LLM judgment in Go. ONE smoke test (`//go:build smoke`), external test package in
  `internal/burlerengine`, wiring the real `claudeengine` + `muxengine` + `Runner` (the test IS
  the caller; the provider-seam invariant restricts shuttleengine/muxengine, not test code),
  following `shuttlecli/smoke_*.go` conventions (skip when no claude binary, lyxtest fixture
  hub, teardown guard, self-contained helpers): the toy chair/table round end-to-end —
  BLOCKING verdict parsed, target fixed, fixer-report present. Trivial on purpose; it proves
  the A→B machinery + file contract + parse against a real engine, not review quality.
- Rationale: brief + design doc; burler lands before perch, so smoke is the only real-engine
  exercise during its build.
- Rejected: exhaustive LLM-behavior tests (flaky, wrong layer).

### docs-and-constraints

- Decision:
  - Delete `docs/modules/burler.md` when the module lands (Documentation Lifecycle — an
    immediate staleness source otherwise); fold durable design (A/B round, contract, profile
    table, weft-blindness rationale) into the `internal/burlerengine` package doc comment and
    the `docs/overview.md` module table / execution stack.
  - Every not-delivered idea moves to a new `### Deferred burler enhancements` section in
    `docs/roadmap.md` (mirroring `### Deferred mux enhancements`): cluster-N>0 (milestone-24
    gated), generic tools-restriction on `Spec` for cluster reviewers, bulk mode + provider
    caching WITH the shared-prefix-plus-suffixes modelling rationale (must survive or caching
    is foreclosed), per-round provider selector when a second engine lands. (The debug CLI
    leaves this list — it ships now.)
  - Fix inbound links in `perch.md`, `loom.md`, `hardener.md`, `docs/roadmap.md`,
    `docs/reviews/README.md` (point at the package/overview instead of the deleted doc).
  - Mark the **burler half** of roadmap milestone 11 done (the milestone is joint
    burler+perch; do not mark the whole milestone).
  - Add a **Review Round Invariant** to `CONSTRAINTS.md` — KEEP IT SHORT (user: existing
    entries tend to be too long; aim well under the smallest current entry). Content: one
    review+fix round follows the round discipline — (1) A before B: the review is fully
    written to disk before any target file is touched; (2) **every recorded finding is fixed
    in B, all severities including LOW/NIT**; (3) no self-grading: a round's fix is judged by
    the NEXT round's fresh reviewer; (4) commit-per-fix on host source, never push. Enforced
    by a `burlerengine` test asserting the embedded template states the load-bearing rules
    (sequencing + fix-everything); the rest is review obligation. Applies to burler now,
    hardener later.
- Rationale: CLAUDE.md task-completion rules; user decisions Q11/Q12.
- Rejected: separate `burler-enhancements.md` file (new staleness source; roadmap precedent
  exists).

## Technical context

- **`internal/shuttleengine`** (read `spec.go`, `run.go`, `engine.go`, `wait.go`):
  - `Spec{Prompt, OutputFiles, Model, Effort, Interactive, Role, Round, Parent, Display,
    Timeout, KeepPane}`; `validate` requires non-empty Prompt and ≥1 OutputFiles entry,
    resolves relative entries against the worktree root, and **rejects pre-existing output
    files** (stale file = instant false "done"). Negative Timeout rejected; 0 → config default.
  - `Runner.Run(Spec) (Result, error)` = Start+Wait; `Result{Outcome, SessionID, StrandGUID,
    LastAssistantMessage, RunDir}`; `Outcome ∈ {done, asking, died, timeout}` (`engine.go`).
    `done` means every OutputFiles entry exists.
  - Wiring precedent: `internal/shuttlecli/cli.go:103` —
    `shuttleengine.NewRunner(muxEngine, claudeengine.New(), layout, shuttleCfg)`.
- **`internal/stencil`** — `Fill(template, values)`: top-level `{{.X}}` markers must be present
  AND non-empty (all offenders reported in one sorted error); branch-internal markers are only
  absence-checked at execution (present-but-empty renders silently blank). Consequence: every
  required prompt block is a top-level marker whose VALUE Go composes; no `{{if}}` for required
  content.
- **`internal/hubgeometry`** — owns all geometry; burler code (tests included) never spells
  `_lyx` in path construction. `Layout.WorktreeRoot` is what profile-path resolution and
  `Spec.validate` anchor on.
- **CLI plumbing to reuse:** `internal/clihelp` (`Execute`, `GroupRunE`), `internal/output`
  (`Ok`/`Err` JSON envelope). Registration touches `cmd/lyx/main.go` (`newRoot()`: import,
  `AddCommand`, root `Long` module list) and the pinned sets in `cmd/lyx/drift_test.go`,
  `helptree_test.go`, `registration_test.go`, `longlist_test.go`, plus
  `sandbox_coverage_test.go` expectations (new module needs the suite `**Covers:** burler` tag).
  shuttlecli's config loading shows how to obtain `shuttleCfg`/`layout`/mux for wiring.
- **Smoke conventions:** `internal/shuttlecli/smoke_run_test.go` — `//go:build smoke`, claude
  binary discovery via `LYX_MUX_CLAUDE`/PATH with skip, `smokePwshPath` const, hub-holder
  teardown guard, helpers reproduced per-file (self-contained convention), lyxtest fixture hub.
- **Sandbox tooling:** `tools/sandbox/main.go` + `suite.go` dispatch named suites
  (`shuttle-suite` etc.) — the new `burler-suite` name must be registered there; root
  `sandbox-shuttle-suite.cmd` is the launcher template to copy. Suite doc conventions
  (pre-conditions, black-box rule, fingerprint header, `**Goal:**/**Watch:**/**Verdict:**`
  scenario shape, `**Covers:**` tags) per `SANDBOX-SHUTTLE-SUITE.md` and
  `docs/sandbox-howto.md`.
- **Mill sources of the reused review concepts** (millhouse repo,
  `plugins/mill/templates/`): `review-code-holistic.md` / `review-discussion.md`
  (READ-ONLY header idea → cluster-only later; criteria + strict output format; finding blocks
  with Location/Issue/Fix; "omit findings if none / never pad"; source-grounding),
  `review-output.schema.md` (verdict vocabulary discipline), `fixer-holistic-brief.md`
  (fix-in-order, commit-per-fix, deferred-with-reason). burler's container differs by design:
  YAML frontmatter, verdict `APPROVED|BLOCKING`, A+B in one agent.
- **Hand-run method:** `docs/reviews/review-prompt-template.md` — the sequencing rule,
  commit-per-fix and fix-everything prose to adapt into the embedded template;
  `docs/reviews/README.md` for the method's history.
- **YAML:** `gopkg.in/yaml.v3` already in `go.mod` (used by `yamlengine`, config). Frontmatter
  split (leading `---` block) is done by burler itself (simple delimiter scan) before
  `yaml.Unmarshal` of the header.
- **Design doc to fold/delete:** `docs/modules/burler.md` (authoritative until this task
  lands). Roadmap milestone 11 (`docs/roadmap.md:140`) is the joint burler+perch milestone;
  milestone 24 (`roadmap.md:247`) is the own-window gate cluster-N cites.

## Constraints

From `CONSTRAINTS.md` (read it in full before planning):

- **Hub Geometry Invariant** — no geometry tokens in burler code or tests; cwd/worktree via
  `hubgeometry`; caller-supplied artifact paths keep burler out of `_lyx` construction.
- **lyxtest Leaf Invariant** — tests may use `lyxtest`; never make lyxtest import feature
  packages.
- **CLI / Cobra Invariant** — burlercli follows the full seam (`Command()`/`RunCLI`, `Short`
  everywhere, JSON envelope, `GroupRunE`, registration + pinned-set updates in the same
  commit).
- **Shuttle Provider-Seam Invariant** — burlerengine never imports claudeengine; burlercli is
  the wiring point (like shuttlecli). No Claude marker strings outside claudeengine.
- **Weft Git Invariant** — burler is weft-blind: writes ride the file contract; the prompt
  never instructs a weft git op; `markdown` fix-scope does no git at all.
- **Sandbox Suite Coverage** — registering `lyx burler` requires the `**Covers:** burler`
  suite tag (this task ships the suite) — keep `sandbox_coverage_test.go` green in the same
  commit.
- **Documentation Lifecycle** — module doc deleted on landing; durable parts fold into package
  doc + overview.
- **New in this task:** add the **Review Round Invariant** (short entry) to `CONSTRAINTS.md`
  in the same commit that ships the template test enforcing it.

## Testing

- **`internal/burlerengine` unit (fake Shuttle)** — TDD candidates, table-driven where natural:
  - Profile validation: empty target/fasit/rubric, nonexistent paths, relative-path resolution
    via layout, empty review/fixer paths, `ClusterN > 0` typed error.
  - Prompt composition: all stencil markers filled; fix-scope/tool-use/prior-rounds blocks
    switch correctly; template asset parses; the enforcement test asserting the embedded
    template contains the sequencing + fix-everything rules (the Review Round Invariant's
    machine half).
  - Spec construction: OutputFiles = [review, fixer-report] resolved order; Role="burler";
    Model/Effort/Timeout/Round mapped; Interactive false.
  - Verdict parse: happy APPROVED/BLOCKING; missing frontmatter; bad YAML; unknown verdict;
    missing/empty finding keys; unknown severity; duplicate ids; BLOCKING-without-blocking-
    finding; APPROVED-with-findings OK.
  - Outcome mapping: fake shuttle returns each of done/asking/died/timeout → Result contents
    per the result-and-outcome-mapping decision; verdict parse error on done → error.
- **`internal/burlercli` unit** — profile YAML → Profile decode (full/partial/invalid), flag →
  RunOpts mapping, envelope output for Ok and each typed error; help-tree/registration/drift/
  longlist pinned sets updated; `RunCLI` seam test per convention.
- **Smoke (`-tags smoke`, opt-in, one test)** — real claudeengine+mux+Runner, toy chair/table
  fixture: BLOCKING verdict parsed, target file actually changed to match, fixer-report
  exists. Deterministic waits (poll with deadline), skip without claude, full teardown.
- **Sandbox suite (operator-driven, not CI)** — S1 toy round / S2 APPROVED / S3 error paths as
  sketched in the sandbox-suite decision; `**Covers:** burler`.
- **Never test LLM judgment in Go** — no assertions on review prose quality anywhere.

## Q&A log

- **Q:** What Go type are target/fasit/rubric? **A:** Target and Fasit are
  `{Paths []string, Instructions string}` (paths = files/dirs, dirs recursive; instructions for
  path-less targets like diffs; ≥1 field non-empty, paths must exist); Rubric is a markdown
  string. "We must be able to review anything: a file, a folder, a collection of files."
- **Q:** Who names the review/fixer-report paths? **A:** The caller, in the Profile; burler
  returns them in Result and stays out of `_lyx` geometry.
- **Q:** How does run-tuning enter? **A:** `Run(Profile, RunOpts{Model, Effort, Timeout,
  Round})` — kept off the content Profile per the design doc.
- **Q:** Non-done outcomes? **A:** Result carries Outcome with empty Verdict, nil error;
  errors reserved for hard failures (invalid profile, start failure, verdict parse on done).
- **Q:** Parse strictness? **A:** Strict everywhere, fail loud — and the user's standing rule:
  **ALL findings get fixed in B, including LOW and NIT.** Severity affects reporting, never
  whether it is fixed.
- **Q:** Reuse mill's review concepts? **A:** Yes — verdict header discipline, severity-tagged
  Location/Issue/Fix findings, omit-if-none/never-pad, source-grounding, target-vs-fasit
  split; inside burler's pinned container (frontmatter, A+B in one agent).
- **Q:** Does v1 need a tools-restriction on shuttle Spec? **A:** No — structurally
  meaningless for a one-session A→B agent (B must write; settings are launch-time); prompt-level
  only; enforcement belongs to cluster reviewers (deferred, recorded in roadmap).
- **Q:** fix-scope commit rules? **A:** `source` → commit-per-fix on host, never push;
  `markdown` → no agent git at all (weft safety beats the doc's literal "commit once" line).
- **Q:** Prior-round hydration in v1? **A:** Yes — optional `PriorReviews`/`PriorFixerReports`
  paths with the clean-room rule (own findings first, consult priors after).
- **Q:** Smoke placement? **A:** `internal/burlerengine` external test package, `-tags smoke`,
  shuttlecli conventions; the test is the caller and may wire claudeengine.
- **Q:** Where do undelivered burler ideas go when `burler.md` is deleted? **A:** A
  `### Deferred burler enhancements` section in `docs/roadmap.md` (mux precedent) — burler.md
  is deleted on delivery precisely to avoid staleness.
- **Q:** CONSTRAINTS.md entry? **A:** Yes — one **Review Round Invariant** covering the round
  discipline with fix-everything at its core; **keep the entry short** (existing entries tend
  to be too long).
- **Q:** Sandbox suite for burler? **A:** Required, part of this task. Burler is a vital
  standalone part of lyx and must be testable on its own → un-defer the debug CLI
  (`lyx burler run`), a thin wrapper that unpacks the profile YAML into `Profile` and flags
  into `RunOpts`; the suite drives the deployed binary black-box.
