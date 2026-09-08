# Discussion: Unify webster/burler CLI wiring into a shared module

```yaml
task: Unify webster/burler CLI wiring into a shared module
slug: unify-webster-burler-wiring
status: discussing
parent: crucible-loom-glyph-hardening
```

## Problem

`internal/webstercli/wiring.go` and `internal/burlercli/wiring.go` each carry their own copy of the same concern: given a verb invocation's flags, decide standalone vs. hub mode, resolve `--target-dir` and `--plan-dir`, derive the state directory, guard against nested standalone geometry, redirect the durable trace sink, seed the standalone stencils directory, and produce the refusal message for each failure mode.
Nothing forces the two copies to agree.
They drifted, and crucible round 6 (`opus-medium-r6`) found six confirmed instances of that drift — R6-7, R6-8, R6-9, R6-15, R6-16, R6-17 — every one of them the same missing abstraction surfacing a different way.

Why now: round 6 fixed all six as **point fixes on the existing duplicated structure**, and those fixes are already on this branch's HEAD (verified during exploration — the `os.Stat` target check, `wireHub`'s `planDirOverridden` assignment, the verb-aware missing-plan refusal, `normalizeForContainment` on both sides of every comparison, `clihelp.ShouldAbort` first in burler's `run` `RunE`, and `resolveToldDir(c.cwd, profilePath)` for `--profile`).
So there is no behaviour left to fix here.
What remains is the structure that produced all six: two copies that a seventh hardening fix would once again land in only one of.
This task removes the duplication and adds the mechanical check that keeps a third copy from appearing.

## Scope

**In:**

- A new package `internal/cliwire` owning the wiring-resolution logic both standalone-capable CLIs share.
- One `ResolveStandalone` entry point performing the whole ordered standalone prologue: target resolution, `standalonestate.Derive`, the nested-geometry refusal, the durable-sink redirect, the stencils resolve-and-seed, and plan-dir resolution with override detection.
- The hub-side pieces that are genuinely shared: the `--target-dir`-in-hub-mode refusal and the plan-dir override detection.
- Rewriting both `wiring.go` bodies to call into `cliwire`, keeping each `wire` method's existing signature.
- Moving the shared-behaviour tests into `internal/cliwire`'s own test file, leaving per-CLI composition tests behind.
- Two enforcement tests in `internal/cliwire`: a production-only `standalonestate.Derive` caller-set pin, and a banned-declaration check over `webstercli`/`burlercli`.
- A new CONSTRAINTS.md invariant, a `internal/cliwire/doc.go` package header carrying the design rationale, and a `docs/overview.md` module-tree entry — same commit.

**Out:**

- Any behaviour change.
  Every refusal message, every resolution rule and every mode decision keeps its current observable behaviour;
  the moved tests' message assertions are the proof.
- Re-fixing R6-7/8/9/15/16/17 — all six are already fixed on HEAD.
  This task moves those fixes into one place, it does not re-apply them.
- Flattening or re-shaping `wire` / `wireHub` / `wireStandalone`.
  Their signatures and the mode-decision-inside-`wire` placement stay exactly as they are.
- A `ResolveHub` sibling.
  The two hub bodies share only the `--target-dir` refusal and the plan-dir override;
  their config loads are genuinely per-engine.
- `manifest/roadmap.md`.
  Per CLAUDE.md the roadmap moves only for completing or adding a planned item;
  this is a consolidation pass.
- Any other CLI.
  Only `webstercli` and `burlercli` are standalone-capable today, and only they are touched.
- `internal/standalonestate`, `internal/standalonegeom`, `internal/hubgeom`, `internal/preflight` — all consumed unchanged.

## Decisions

### Module name and location

- Decision: a new package `internal/cliwire`, sitting alongside `internal/clihelp`, `internal/preflight`, `internal/hubgeom` and `internal/standalonegeom`.
- Rationale: the concern has no owner module today.
  `internal/standalonegeom` is the wrong home because its whole contract is that it never touches disk and never derives anything (its own `doc.go` says so explicitly), while this logic stats, reads directories and seeds stencils.
  `internal/standalonestate` is barred outright by the Standalonestate Leaf Invariant (stdlib-only).
  `internal/clihelp` is the wrong altitude: it is generic cobra plumbing for all twelve CLI modules, and this is specific to the two standalone-capable ones.
- Rejected: folding into `standalonegeom` (breaks its disk-free contract), into `standalonestate` (breaks its leaf invariant), into `clihelp` (wrong altitude).

### API shape — descriptor + methods + package-level pure functions

- Decision: a `cliwire.Module` descriptor value carrying the per-CLI data, with **methods** for the fallible and message-producing steps, and **package-level functions** for the pure helpers that produce no message.
- Rationale: this is exactly the `internal/shedrecipe` split the task body demands — one implementation, callers varying only in their own data.
  The pure helpers (`ResolveToldDir`, `NormalizeForContainment`, `SamePlanDir`, `RepositoryRootOf`) need no descriptor because none of them produces an operator-facing string, so requiring one would be ceremony.
- Rejected: flat functions each taking a `module string` (extends what `refuseNestedStandaloneGeometry` already does, but every message-varying call grows another string parameter);
  an interface each CLI implements (more indirection than two static call sites justify).

### One `ResolveStandalone` entry point owning the whole ordered prologue

- Decision: `ResolveStandalone` performs the full standalone sequence in order — target resolve → `standalonestate.Derive` → nested-geometry refusal → durable-sink redirect → stencils resolve-and-seed → plan-dir resolution — and returns a result struct.
  Each CLI then builds only its own engines from that result.
- Rationale: this is the only shape that also captures the **ordering** obligation.
  The durable sink is armed lazily on the first Info-or-above record, so the redirect binds only if it runs before anything in the sequence can log — a rule enforced today by two long comments in two files, which is precisely the kind of thing that drifts.
  Putting the sequence in one function locks the ordering in the type rather than in prose.
- Rejected: helper-level extraction only, leaving both `wireStandalone` bodies to call the moved helpers in their own order (lower risk, but the ordering rule stays duplicated as prose);
  moving hub as well (see the hub decision below).

### Per-CLI variance carried as descriptor data, not as branching

- Decision: the two varying noun phrases in the nested-geometry refusal stay as data on `Module`.
  Webster's messages keep "state, locks, rendered prompts and trace logs" and "the repository it drives";
  burler's keep "instruction files, shuttle run directories and trace logs" and "the repository it reviews".
  The hub `--target-dir` refusal likewise keeps webster's "the worktree is already the target" and burler's "the anchor path is already the target".
- Rationale: both wordings are accurate for their own CLI, and there is still exactly one implementation producing them.
  Data-driven variance is what makes the copy-paste-and-diverge failure structurally unreachable — the same reason the perch pattern cannot drift.
- Rejected: unifying to one generic wording (smallest surface, but every operator-facing message gets vaguer for no structural gain).

### Plan-dir logic moves in, even though burler has no plan

- Decision: `samePlanDir`, the plan-dir content check, override detection and the verb-aware missing-plan refusal all move into `cliwire`.
  Burler simply never populates the plan rules.
- Rationale: R6-8 and R6-9 **are** this shape — a rule present on one path and absent or contradictory on its sibling.
  Leaving the plan-dir logic outside the module built to prevent that failure would place exactly the bug class this task exists for outside its own guard.
- Rejected: strict YAGNI, leaving plan-dir logic in `webstercli` and moving only what is duplicated today.

### Plan opt-in is a nil descriptor field, not a second entry point

- Decision: the standalone request carries `PlanDirFlag string`, and `Module` carries a `Plan *PlanRules` field — nil for burler, populated for webster with webster's own refusal text.
- Rationale: keeps one entry point and no second function, and makes "burler has no plan" a nil field rather than a branch each caller writes for itself.
  Keeping webster's `run`-specific recourse text on webster's own descriptor also stops that text from being hardcoded inside a module burler shares.
- Rejected: two entry points `ResolveStandalone` / `ResolveStandaloneWithPlan` (re-splits the prologue that was just unified);
  a `Plan bool` with the refusal text hardcoded in `cliwire` (bakes webster-specific text into shared code).

### The default plan directory arrives as a function on `PlanRules`

- Decision: `PlanRules` carries `DefaultPlanDir func(base string) string`.
  Webster passes `planparser.PlanDir`.
  The prologue calls it with `stateDir` to obtain standalone's default;
  `wireHub` calls the same field with the hub anchor path for the hub-mode override check.
  `internal/cliwire` therefore does **not** import `internal/planparser`.
- Rationale: `cliwire` needs the default for `SamePlanDir` and for the missing-plan refusal's recourse text, but it constructs no `Geometry`, and standalone's default depends on the `stateDir` the prologue itself derives — so the caller cannot hand over a finished string.
  A function field keeps `cliwire` agnostic to webster's plan format and matches the already-decided split where varying data lives with the caller.
  Today's two sources reduce to the same call: standalone's `standalonegeom.WebsterGeometry(target, stateDir).PlanDir` is `planparser.PlanDir(stateDir)`, and hub's `hubgeom.WebsterGeometry(loc).PlanDir` is `planparser.PlanDir(anchorPath)` — one function, two bases.
- Rejected: `cliwire` importing `internal/planparser` and computing `planparser.PlanDir(base)` itself (fewer moving parts, and `standalonegeom` already imports `planparser` anyway, but it puts webster's plan layout inside a module burler shares, breaking the descriptor-carries-varying-data rule);
  splitting the prologue so the caller builds geometry between two calls (re-splits the single entry point that was deliberately unified).

### Stencils resolve-and-seed moves in, returned as a plain string

- Decision: the prologue resolves and seeds the standalone stencils directory and returns it as a plain string in the result;
  each CLI assigns it where its own shape wants it (webster into `geom.StencilsDir`, burler into `c.stencilsDir` and `burlerengine.New`'s fourth argument).
- Rationale: the resolve-and-seed is byte-identical on both sides — verified: `standalonegeom.WebsterGeometry(target, stateDir).StencilsDir` is filled from `standalonegeom.StencilsDir(stateDir)`, which is exactly what burler calls directly.
  Only the destination field differs.
  Leaving it out would keep the "seed only the default, never an explicit override" rule duplicated in two places, and it is the same rule.
- Rejected: leaving stencils entirely out of the shared prologue.

### Hub side — only the two genuinely-shared pieces move

- Decision: the `--target-dir`-in-hub-mode refusal moves (as a `Module` method producing the verb-correct message), and the plan-dir override detection moves (shared by `wireHub` and `wireStandalone`).
  Every module config load stays in each CLI's own `wireHub`.
- Rationale: the two hub bodies share roughly six lines;
  a full `ResolveHub` would be symmetry without content.
  But leaving the hub side entirely untouched is what R6-8 already was — a rule enforced in standalone and silently absent in hub — so the shared pieces do move.
- Rejected: a full `ResolveHub` sibling (form without content);
  moving nothing on the hub side (leaves R6-8's exact shape reachable).

### `cliwire` performs the durable-sink redirect itself

- Decision: `ResolveStandalone` calls `logger.SetDurableSinkDirWithWorktreeRoot(standalonegeom.LogsDir(stateDir), target)` at the correct point inside the prologue.
  `internal/cliwire` therefore imports `internal/logger`.
- Rationale: this is the ordering obligation the single-entry-point decision exists to lock into the type.
  Returning `LogsDir` for each caller to redirect itself would put the rule straight back into two copies of the same comment.
- Rejected: returning `LogsDir` in the result and letting each CLI redirect (keeps `cliwire` free of a logging dependency, at the cost of the exact rule this design centralises).

### Descriptor values are declared by their owners, not by `cliwire`

- Decision: each CLI declares its own descriptor in its own package — `var wireModule = cliwire.Module{Name: "webster", ...}` in `webstercli`, likewise in `burlercli`.
  No **production** file in `cliwire` names either caller.
  Its enforcement test necessarily does, since an enforcement test's whole job is to name the packages it polices;
  that is the same arrangement `internal/gitkit/callerset_enforcement_test.go` already has, where production `gitkit` never names `lyxcwd` and the test names it in a const.
- Rationale: this is the actual `shedrecipe` analogy.
  The shared `Constructor` functions live in `shedrecipe`;
  the varying data lives outside it, in `contracts/recipes/loom-recipe.yaml`'s rows.
  A `cliwire.Webster` / `cliwire.Burler` pair would make the shared module import-aware of its own callers, which the recipe-row arrangement specifically avoids.
- Rejected: `cliwire` declaring both as package-level vars (puts both messages side by side for easy comparison, at the cost of the shared module owning caller-specific text).

### Mechanical enforcement — two complementary checks

- Decision: two enforcement tests in `internal/cliwire`.
  A **caller-set pin** asserting that `internal/cliwire` is the only production caller of `standalonestate.Derive`, modelled on `internal/gitkit/callerset_enforcement_test.go` but **skipping `_test.go` files**, the way `internal/treadleengine/seam_enforcement_test.go:52` already does.
  A **banned-declaration check** failing if `internal/webstercli` or `internal/burlercli` declares a function named `resolveStandaloneTarget`, `repositoryRootOf`, `refuseNestedStandaloneGeometry`, `normalizeForContainment`, `pathContains`, `resolveToldDir`, `samePlanDir`, `standalonePlanDirHasContent`, or `standaloneDefaultPlanDir`.
- Rationale: the two cover each other's blind spots.
  The `Derive` pin catches a whole third copy built from the bottom up;
  the name check catches a partial re-implementation that still calls into `cliwire` for the rest.
  The task body's own argument is that `shedrecipe` is safe partly *because* `shedengine.validate` and `shedcheck.Check` give it mechanical drift detection — a documentation-only invariant here would lean on someone remembering the rule, which is the failure the whole task is about.
- Rejected: the `Derive` pin alone (a CLI re-hand-rolling target resolution while still calling `ResolveStandalone` slips through);
  the name check alone (misses a new CLI deriving its own state dir from scratch);
  an import allowlist barring `webstercli`/`burlercli` from importing `standalonestate`/`standalonegeom` (too blunt — `standalonegeom`'s geometry builders are legitimately needed after the prologue returns).

The pin is production-only because the invariant is about production wiring.
Six test call sites exist today and stay where they are: `internal/burlercli/wiring_test.go:68,143,211` (whose helpers move to `cliwire` anyway under the test decision), `internal/webstercli/cli_integration_test.go:46,99`, and `internal/standalonegeom/reedgeom_symlink_integration_test.go`.
Those tests call `Derive` to build a fixture and to assert the real derivation end-to-end — they are not a second copy of the wiring, and forcing them through `cliwire` would make packages that have no reason to depend on it do so.
An explicit test-file allowlist was rejected as maintenance on something a code review would see anyway;
production drift is what slips in silently.

### Exported surface

- Decision: the exported surface is

  - `cliwire.Module` — the descriptor: `Name string`, the nested-geometry refusal's two noun phrases, the hub `--target-dir` refusal's subject phrase, and `Plan *PlanRules`.
  - `cliwire.PlanRules` — `DefaultPlanDir func(base string) string` plus the missing-plan refusal's text.
    Nil on `Module` means the CLI parses no plan.
  - `cliwire.StandaloneRequest` — `Cwd`, `StencilsDirFlag`, `PlanDirFlag`, `TargetDirFlag`.
  - `cliwire.Standalone` — the result: `Target`, `StateDir`, `Hash8`, `StencilsDir`, `PlanDir`, `PlanDirOverridden`, `DefaultPlanDir`.
  - `(Module).ResolveStandalone(StandaloneRequest) (Standalone, error)`.
  - `(Module).RefuseTargetDirInHubMode(flag string) error`.
  - `(Module).ResolvePlanDir(toldPlanDir, defaultPlanDir string) (planDir string, overridden bool)` — the plan-dir override resolver, shared by `wireHub` and the standalone prologue.
  - Package-level pure functions `ResolveToldDir`, `NormalizeForContainment`, `SamePlanDir`, `RepositoryRootOf`.
- Rationale: reads cleanly at the call site (`wireModule.ResolveStandalone(...)`), and the pure helpers stay callable without a descriptor since none of them produces a message.
- Rejected: a minimal surface keeping the pure helpers unexported (viable, since the shared tests live inside the package anyway, but `ResolveToldDir` has a live cross-file caller — burler's `run.go` resolves `--profile` through it — so it must be exported regardless).

### Both `wire` signatures stay exactly as they are

- Decision: `(*websterCLI).wire(loc, mode, cwd, stencilsDirFlag, planDirFlag, targetDirFlag)` and `(*burlerCLI).wire(loc, mode, cwd, stencilsDirFlag, targetDirFlag)` keep their current signatures;
  only their bodies change.
- Rationale: keeps `cli.go`'s `resolvePersistentPreRun` and every existing composition test compiling unchanged, so the diff stays confined to the two `wiring.go` files plus the new package.
  The mode decision lives inside `wire` for a stated tier-1 test reason (driving the real pre-run would reach `lyxcwd.Resolve` and its git spawn), and that placement is not disturbed.
- Rejected: collapsing `wire`/`wireHub`/`wireStandalone` into a flatter shape now that the prologue moved out — churn in files this task does not otherwise need to touch.

### Proof of no behaviour change

- Decision: the moved tests are the proof.
  Every existing assertion — including the exact refusal-message strings — moves into `cliwire`'s test file unchanged in substance, so a reworded message fails the suite.
  The verify command is the two-command pair in the Testing section below, not `go test ./...` alone.
- Rejected: additional golden-file tests pinning each refusal message verbatim per module (real value, but a new artifact to maintain for text the moved tests already assert);
  relying on an untagged run alone without insisting the message assertions survive the move (that is exactly how the wording quietly drifts again).

### Where the design rationale lives

- Decision: no `manifest/designs/` file.
  The rationale goes in `internal/cliwire/doc.go`'s package header, `docs/overview.md` gains a tree entry for `internal/cliwire` beside `hubgeom`/`standalonegeom`/`preflight`, and `CONSTRAINTS.md` gains the new invariant — all in the landing commit.
- Rationale: `docs/overview.md:91`'s Documentation lifecycle — the authority `CONSTRAINTS.md` points at — says `manifest/designs/<module>.md` are drafts for planned, not-yet-built modules, **deleted when their module lands**, with the purpose and design rationale then living in the Go package header.
  Writing one in the same commit that lands the module would create a doc that is immediately deletable.
  `docs/overview.md` also lists these packages in a tree rather than a table, so "module-table row" was the wrong shape;
  a `docs/shared-libs/` entry is likewise wrong, since `hubgeom`, `standalonegeom` and `preflight` — this module's nearest neighbours — have tree entries and no shared-lib doc.
- Rejected: a `manifest/designs/cli-wiring.md` (contradicts the lifecycle the same commit is supposed to honour).

### The two current copies

- `internal/webstercli/wiring.go` (532 lines) and `internal/burlercli/wiring.go` (397 lines).
- **Byte-identical modulo the module-name string:** `gitDirName` (a `.git` literal, deliberately not a `lyxdirs` constant), `pathContains`, `normalizeForContainment`, `resolveToldDir`, `repositoryRootOf`.
- **Identical logic, differing only in the `"webster:"` / `"burler:"` error prefix:** `resolveStandaloneTarget`.
- **Identical structure, differing in two noun phrases:** `refuseNestedStandaloneGeometry`, which already takes a `module string` parameter.
  Webster: "state, locks, rendered prompts and trace logs" / "the repository it drives" / "Drive a target outside the state home".
  Burler: "instruction files, shuttle run directories and trace logs" / "the repository it reviews" / "Review a target outside the state home".
- **Webster-only:** `samePlanDir`, `standalonePlanDirHasContent`, `(*websterCLI).standaloneDefaultPlanDir`, and the `planDirOverridden` / `planDirDefault` fields.
- **Hub `--target-dir` refusal:** near-identical, differing in "the worktree is already the target" (webster) vs. "the anchor path is already the target" (burler).

### Ordering facts that must survive the move

- The durable-sink redirect must run before any statement in the prologue that can log, because the sink is armed lazily on the first Info-or-above record.
  It cannot be first — it needs `Derive`'s `stateDir` to know where to point — so the real obligation is: every statement above it stays log-free, and nothing logging is added below it that could be hoisted above.
  Both files carry a long comment saying this;
  after the move there is one function, and the comment belongs on it.
- `refuseNestedStandaloneGeometry` must run **after** `Derive` and **before** the sink redirect and before any substrate is booted.
  It front-runs `shuttleengine.NewDetachedRunner`'s own containment assertion, which fires far too late and blames the wrong cause.
- The stencils seed's failure is a **hard** error in standalone (unlike the root pre-run's best-effort logged seed), because nothing else will ever create that directory.
- The stencils seed runs **only** for the derived default, never for an explicitly-told `--stencils-dir`, so a curated stencil set is never rewritten from under the operator.
  Both files state this as a deliberate asymmetry a reader might try to "simplify" away.

### Dependencies the new package will take

`internal/standalonestate` (`Derive`, `Normalize`), `internal/standalonegeom` (`StencilsDir`, `LogsDir`), `internal/logger`, `internal/stencilstore`, `internal/buildinfo`, `contracts/stencils`, plus stdlib.

Two exclusions are deliberate:

- **`internal/lyxcwd`** — barred by the Told-Geometry Invariant, and never needed: `cwd` and `loc` arrive from the caller.
- **`internal/planparser`** — the plan-directory default arrives as `PlanRules.DefaultPlanDir func(base string) string`, which webster fills with `planparser.PlanDir`.
  `cliwire` never parses a plan;
  `standalonePlanDirHasContent`'s check is a `ReadDir` for a `*.md` entry, which is not plan parsing and does not touch the Planparser Sole-Parser Invariant.

### Values the result struct must carry

`target`, `stateDir`, `hash8` (webster and burler both need it for `standalonegeom.ReedGeometry`), `stencilsDir`, `planDir`, whether the plan dir was overridden off its default, and the default's own path (for the missing-plan refusal's recourse text, which must always name the location `run` requires rather than the override the operator just supplied).

### Precedent to read before designing

`internal/shedrecipe/registry.go` — the "one implementation, callers vary only in their own data" split this design copies.
`internal/gitkit/callerset_enforcement_test.go` — the AST-based sole-caller pin the `Derive` check copies.
`internal/standalonestate/leaf_enforcement_test.go` — the import-allowlist enforcement style used across this repo.

### Confirmed already-fixed on HEAD

All six round-6 findings are fixed on this branch's HEAD and need no re-application: R6-7 (`os.Stat` on the resolved `--target-dir`, both packages), R6-8 (`wireHub` sets `planDirOverridden`), R6-9 (the missing-plan refusal names the default location and explains `run`'s own refusal), R6-15 (`normalizeForContainment` on both sides of the guard and of `samePlanDir`), R6-16 (`clihelp.ShouldAbort` checked first in burler's `run` `RunE`), R6-17 (`--profile` resolved via `resolveToldDir(c.cwd, ...)`).

### Test-side duplication

`internal/webstercli/wiring_test.go` (881 lines) and `internal/burlercli/wiring_test.go` (732 lines) duplicate the helpers `hubLocation`, `hash8For`, `seedGitRepositoryRoot` and roughly six near-identical test functions: `TestWire_TargetDirRefusedInHubMode`, `TestResolveStandaloneTarget_RefusesATargetThatIsNotAReadableDirectory`, `TestResolveStandaloneTarget_LiftsToRepositoryRoot`, `TestWireStandalone_RefusesStateDirNestedInTarget`, `TestRefuseNestedStandaloneGeometry_SeesThroughASymlinkedStateHome`, `TestWireStandalone_RedirectsDurableSinkToStandaloneLogsDir` / `TestWireHub_LeavesDurableSinkDirUntouched`.
Both files are untagged tier-1 and must stay that way;
the symlink test already `t.Skipf`s on hosts without symlink support rather than taking an `integration` tag.
Standalone-mode cases redirect both `XDG_STATE_HOME` and `LOCALAPPDATA` to a `t.TempDir()` before calling `wire`, and none of them is `t.Parallel()` because `t.Setenv` panics under a parallel test — this pattern moves with the tests.

## Constraints

From `CONSTRAINTS.md`:

- **Cwd Resolution Invariant** — `internal/lyxcwd` owns cwd resolution alone.
  `cliwire` must never call `os.Getwd` or `git rev-parse`, and must not import `lyxcwd`.
  `repositoryRootOf` is not a cwd query: it lifts an already-resolved absolute path to the root of the tree it lives in, and both current copies say so explicitly.
  That justification moves with the code.
- **Told-Geometry Invariant** — engines are handed absolute paths and derive none of their own;
  `hubgeom` and `standalonegeom` remain the only `Geometry`-struct constructors.
  `cliwire` constructs no `Geometry`;
  it returns the told strings each CLI feeds to those two builders.
  The invariant's bound-package list should gain `cliwire`.
  Its `NewDetachedRunner` clause stays satisfied: standalone runners are still constructed from each CLI's own wiring, and `NewRunner`'s containment assertion is not relaxed.
- **Standalonestate Leaf Invariant** — `internal/standalonestate` imports only stdlib and `Derive` creates nothing on disk.
  Unchanged;
  this task does not touch that package.
- **CLI / Cobra Invariant** — each module exposes `Command()` and `RunCLI`, non-empty `Short` on every command, JSON errors via `internal/output`, every `RunE` checks `clihelp.ShouldAbort` first.
  Unchanged, and untouched: no command, flag or `RunE` is added or reordered.
- **Test Tier Purity Invariant** — the moved tests must stay tier 1 (untagged, no process spawn, no cwd resolution).
  The whole prologue is filesystem reads and path arithmetic, so this holds.
- **Documentation Lifecycle** — `docs/overview.md#documentation-lifecycle` is the authority CONSTRAINTS.md points at, and it says `manifest/designs/<module>.md` are drafts for **planned, not-yet-built** modules, deleted when the module lands, with the purpose and design rationale then living in the module's Go package header.
  A landing commit therefore writes no `manifest/designs/` file — see the docs decision below.
- **Planparser Sole-Parser Invariant** — `internal/planparser` is the sole parser and writer of the on-disk plan format.
  `cliwire` does not import it (see the plan-dir default decision);
  it never parses a plan, only checks whether a directory holds `*.md` files.

New invariant to record in `CONSTRAINTS.md`, same commit:

**Cliwire Sole-Wiring Invariant.** `internal/cliwire` is the sole owner of standalone/hub CLI wiring resolution for standalone-capable CLIs.
A `<module>cli` never re-implements `--target-dir` resolution, repository-root lift, mode-derived state/plan/stencils resolution, the nested-geometry guard, or the durable-sink redirect;
it declares its own `cliwire.Module` descriptor and calls in.
`internal/cliwire` is the only **production** caller of `standalonestate.Derive`;
test files may call it to build fixtures and to assert the real derivation.
Enforced by the two tests in `internal/cliwire`.

From CLAUDE.md:

- Build requires `CGO_ENABLED=1` and a C compiler (quarry's tree-sitter grammars).
- Semantic line breaks in every `.md` file touched.
- Docs land in the same commit as the code.
- This is a task worktree — never push to `main`, never touch another worktree.

## Testing

Verify command for every batch and for the done gate, both commands:

```
go test ./...
go test -tags integration ./internal/cliwire/... ./internal/webstercli/... ./internal/burlercli/... ./internal/standalonegeom/...
```

The tagged second command is not optional here.
`internal/webstercli/verbs_test.go` and both `cli_integration_test.go` files carry `//go:build integration`, so an untagged run executes none of them — and `webstercli/cli_integration_test.go` exists precisely to exercise the real `standalonestate.Derive` and the real standalone stencil seed end-to-end, which is the code this task moves.
An untagged-only gate would have left the moved prologue's only end-to-end coverage unrun.
`smoke_test.go` (`//go:build smoke`) stays out: it drives live agent substrate and covers nothing this change touches.

### `internal/cliwire` — the new package's own tests

TDD candidates, all tier 1 and untagged:

- **Target resolution.** A told `--target-dir` that does not exist is refused;
  one naming a file is refused;
  a relative value resolves against the told cwd;
  an absolute value is cleaned;
  an empty value falls back to cwd and is never stat'd.
  The refusal messages must be asserted verbatim per descriptor, since a message rewrite is exactly the drift this task ends.
- **Repository-root lift.** A subdirectory of a repository resolves to the repository root;
  the *nearest* `.git` wins, never the topmost (a nested repository or submodule stays its own repository);
  a directory with no repository above it is returned unchanged;
  a `.git` **file** (linked worktree) counts.
- **Nested-geometry refusal.** State dir inside target is refused with the state-home lever;
  target inside state dir is refused with the target lever;
  disjoint passes;
  the R6-15 case — a state home reaching inside the target only through a symlink, with the `<stateHome>/lyx/<hash8>` leaf deliberately absent — is still refused.
  Both descriptors' wordings are asserted, which is what pins the per-CLI noun phrases as data.
- **`SamePlanDir`.** A `"."` or trailing-separator spelling of the default is recognised as the default, not as an override;
  a symlinked spelling of the default likewise (the second half of R6-15).
- **Plan rules.** A missing plan dir, an empty one, and one with no `*.md` files are all the same refusal;
  the refusal names the *default* location even when `--plan-dir` moved the plan off it;
  a nil `Plan` field means the prologue never performs the check at all (burler's case).
- **Prologue ordering.** The durable sink is pointed at `standalonegeom.LogsDir(stateDir)` and the worktree root at the target;
  a prologue that fails at the nested-geometry refusal leaves the sink untouched, which is what proves the redirect sits after the guard.
- **Stencils.** The derived default is seeded;
  an explicitly-told stencils dir is read and never written;
  a seed failure on the default is a hard error naming the directory.

### Enforcement tests in `internal/cliwire`

- `standalonestate.Derive` caller-set pin — AST-based, walking `internal/` and `cmd/`, skipping `standalonestate` itself **and every `_test.go` file**, matching a selector call whose receiver is the file's `standalonestate` import.
  Model: `internal/gitkit/callerset_enforcement_test.go` for the AST match, `internal/treadleengine/seam_enforcement_test.go:52` for the `_test.go` skip.
- Banned-declaration check over `internal/webstercli` and `internal/burlercli`, on declared function names, AST-based so a doc comment naming the function cannot trip it.

### `internal/webstercli` and `internal/burlercli` — composition tests that stay

Each package keeps the tests that verify *its own* composition, not the shared prologue's behaviour:

- The mode truth table: `ModeHub` selects hub, `ModeStandalone` selects standalone, with a told `preflight.Mode` and never through the real pre-run.
- That `wire` threads its flags into the prologue and assigns the result where its own struct wants it — webster's `geom`, `roles`, `refMatcher`, `openFabric`, `batcher`, `anchorRel`, `planDirOverridden`;
  burler's `engine`, `mode`, `stateDir`, `stencilsDir`.
- Mode-specific pinned facts already covered today: webster's `NeverMatches` matcher and nil `openFabric` in standalone, burler's `wireStandalone` never reading `loc`, the `reedUp` seam present in standalone and absent in hub, and that the standalone runner reaches a public entry point without a told-path error (`NewDetachedRunner` vs. `NewRunner`).
- Webster's `--plan-dir` override marking `run`'s refusal.

The duplicated helpers `hash8For` and `seedGitRepositoryRoot` move to `cliwire`;
`hubLocation` stays in each CLI package, since only the hub composition tests need it and it builds a `lyxcwd.Location`, which `cliwire` must not import.

### Regression surface to watch

`internal/burlercli/run.go` calls `resolveToldDir` for `--profile` (R6-17's fix).
That call site must switch to `cliwire.ResolveToldDir` and keep behaving identically against the seam cwd `c.cwd`, not the process cwd.
`internal/burlercli/cli_test.go` (untagged) and `internal/webstercli/verbs_test.go` plus both `cli_integration_test.go` files (all `//go:build integration`) exercise the CLIs end-to-end and should pass untouched — if any of them needs editing, that is a behaviour change and a signal to stop.
This is why the verify pair above includes the tagged run: the integration files are the only end-to-end coverage of the moved prologue, and an untagged-only gate would never execute them.

## Q&A log

- **Q:** Where does the shared module live? **A:** New package `internal/cliwire` — `standalonegeom` is deliberately disk-free while this logic stats/reads/seeds, and `clihelp` is the wrong altitude (generic for all twelve CLIs).
- **Q:** What API shape? **A:** Descriptor + methods + package-level pure functions — the `shedrecipe` form exactly: one implementation, callers vary only in their own data.
- **Q:** How much of the standalone sequence moves? **A:** One `ResolveStandalone` entry point owning the whole prologue — the only variant that locks the ordering obligation (sink redirect before anything can log) into the type instead of into two copies of the same comment.
- **Q:** Keep or unify the two varying noun phrases in the nested-geometry refusal? **A:** Keep them as descriptor data — precise messages for both CLIs, still one implementation.
- **Q:** Does plan-dir logic move even though burler has no plan? **A:** Yes, burler just leaves it unused — this *is* the R6-8/R6-9 pattern, and YAGNI here would put exactly the bug class the task exists to prevent outside the module.
- **Q:** How does webster opt into plan-dir resolution and burler not? **A:** A `Request` with `PlanDirFlag` plus a `Plan *PlanRules` descriptor field, nil for burler — one entry point, and "burler has no plan" becomes a nil field rather than a branch each caller writes.
- **Q:** Where does the stencils step land? **A:** The prologue resolves and seeds it and returns it as a string — resolve-and-seed is identical on both sides (confirmed), only the destination field varies.
- **Q:** Does the hub side get a shared entry point? **A:** Only the `--target-dir` refusal and the plan-dir override move;
  config loads stay per-CLI.
  The hub bodies genuinely differ where it counts, so a full `ResolveHub` would be form without content — but leaving the R6-8 pattern unmoved is precisely what the task exists to prevent.
- **Q:** Test consolidation? **A:** The shared package owns the shared tests, each CLI keeps only composition tests — otherwise exactly the test duplication this task removes survives.
- **Q:** Does `cliwire` get a CONSTRAINTS invariant? **A:** Yes, **and** mechanical enforcement.
  The proposal itself argued `shedrecipe` is safe partly *because* `shedengine.validate` / `shedcheck.Check` give mechanical drift detection;
  a doc-only invariant here would lean on someone remembering the rule, which is the exact failure the whole task is about.
- **Q:** What does the mechanical check assert? **A:** Both a `Derive` caller-set pin and a banned-declaration check — the pin catches a third copy built from scratch, the name check catches a partial re-implementation that still calls into `cliwire` for the rest;
  each alone covers only half.
- **Q:** Does `cliwire` perform the sink redirect itself? **A:** Yes — that *is* the ordering obligation the single entry point was chosen to lock into the type.
- **Q:** Documentation? **A:** Package doc, overview tree entry and the invariant, same commit.
  (Originally answered "design doc"; corrected in review round 1 — `docs/overview.md:91`'s lifecycle deletes `manifest/designs/` files when their module lands, so writing one in the landing commit contradicts the rule the commit is honouring.)
- **Q:** Exported surface naming? **A:** The full named surface — descriptor methods for the message-producing steps, package functions for the pure helpers, which need no descriptor since they produce no messages.
- **Q:** How is "no behaviour change" proven? **A:** Moved tests with unchanged message strings plus `go test ./...` — proves it without a new artifact to maintain.
- **Q:** Where do the two descriptor values live? **A:** Each CLI owns its own descriptor var — the real `shedrecipe` analogy: shared logic in the module, varying data with the callers, just as the `Constructor` functions live in `shedrecipe` while the data lives in the YAML rows outside it.
- **Q:** Verify command? **A:** `go test ./...` **plus** a scoped `-tags integration` run over `cliwire`, `webstercli`, `burlercli` and `standalonegeom`.
  (Originally answered "`go test ./...` alone, since `-tags integration` adds runtime without new coverage"; corrected in review round 1 — `webstercli/cli_integration_test.go` is tagged `integration` and exists specifically to exercise the real `Derive` and the real stencil seed, so an untagged-only gate never runs the moved prologue's end-to-end coverage.)
- **Q:** Does the `Derive` caller-set pin cover test files? **A:** Production files only, skipping `_test.go` the way `treadleengine/seam_enforcement_test.go` does.
  The invariant is about production wiring;
  `cli_integration_test.go` and `standalonegeom`'s symlink test call `Derive` to build fixtures and verify the real derivation, not as a second wiring copy.
  A test-file allowlist was rejected — it is maintenance on something a code review would catch anyway, whereas production drift is what slips in silently.
- **Q:** Where does `cliwire` get the default plan directory, given it builds no `Geometry` and standalone's default depends on the `stateDir` the prologue itself derives? **A:** `PlanRules.DefaultPlanDir func(base string) string`, with webster supplying `planparser.PlanDir`.
  Keeps `cliwire` agnostic to webster's plan format, consistent with the already-chosen "varying data lives with the caller" split;
  importing `planparser` into `cliwire` would break exactly that principle, and splitting the prologue in two would undo the single entry point.
- **Q:** Do the `wire` signatures change? **A:** No, only the bodies — keeps `cli.go` and the existing composition tests compiling unchanged, confining the diff to `wiring.go` plus the new package.
