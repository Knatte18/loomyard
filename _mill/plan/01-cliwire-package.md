# Batch: cliwire-package

```yaml
task: "Unify webster/burler CLI wiring into a shared module"
batch: "cliwire-package"
number: 1
cards: 5
verify: go test ./... && go test -tags integration ./internal/cliwire/... ./internal/webstercli/... ./internal/burlercli/... ./internal/standalonegeom/...
depends-on: []
```

## Batch Scope

This batch creates `internal/cliwire` in full — package doc, the pure path helpers, the `Module`/`PlanRules` descriptor types with their message-producing methods, the single `ResolveStandalone` prologue, and the package's own tier-1 test file.
Nothing outside `internal/cliwire` is touched, so at the end of this batch both `internal/webstercli/wiring.go` and `internal/burlercli/wiring.go` still carry their own copies and still compile and pass unchanged;
batch 2 is what deletes them.

The external interface batch 2 consumes is exactly the exported surface listed in card 3 and card 4: `cliwire.Module`, `cliwire.PlanRules`, `cliwire.StandaloneRequest`, `cliwire.Standalone`, `(Module).ResolveStandalone`, `(Module).RefuseTargetDirInHubMode`, and the package-level functions `ResolveToldDir`, `NormalizeForContainment`, `SamePlanDir`, `RepositoryRootOf`, `ResolvePlanDir`.

Batch-local decision beyond `## Shared Decisions`: the moved logic is transcribed from `internal/webstercli/wiring.go`, which is the fuller of the two copies (it alone carries the plan-dir rules).
Where the two copies differ only in a message fragment, that fragment becomes a descriptor field;
where they differ in nothing, webster's body is the one that moves.
Every doc comment on a moved function moves with it, edited only where it names a package-private caller that no longer exists.

Where the two copies' doc comments differ materially, this is which one survives.
For `pathContains`, `NormalizeForContainment`, `RepositoryRootOf` and `SamePlanDir` the two are byte-identical or webster-only, so webster's moves unchanged.
For `ResolveToldDir`, webster's is the fuller one — it alone explains the standalone default-vs-override path equality — so webster's moves and burler's is dropped.
For `resolveStandaloneTarget` the two carry genuinely different consequences and the surviving comment is a **merge**: webster's body plus burler's Windows note on why the result must be absolute (`Derive` normalises through `EvalSymlinks`+`Clean` and compares case-insensitively there), burler's half of the R4-26 paragraph (a subdirectory run also resolved the profile's own relative target and fasit paths against the subdirectory rather than the repository), and burler's half of the R6-7 paragraph (its fix phase *writes*, so a mistyped `--target-dir` edited the wrong tree).
Now that one function serves both CLIs, dropping either consequence would leave the shared code documenting only half of what it protects.
For `ResolveStandalone`'s own comment, the ordering paragraph is webster's and burler's merged, and burler's two-asymmetry paragraph moves across in full — including its note that the empty fourth `stencilstore.Reconcile` argument is the "no source tree here" value that keeps the port-back drift warning silent, since standalone genuinely has no `contracts/stencils` source tree beside it.

## Cards

### Card 1: package doc for internal/cliwire

- **Context:**
  - `internal/standalonegeom/doc.go`
  - `internal/preflight/doc.go`
  - `CONSTRAINTS.md`
  - `_mill/discussion.md`
- **Edits:** none
- **Creates:**
  - `internal/cliwire/doc.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/cliwire/doc.go` carrying only the package clause `package cliwire` and the package-level doc comment, matching the shape of `internal/standalonegeom/doc.go` (a one-line file-purpose comment, a blank line, then the `// Package cliwire ...` block).
  The doc comment must state, in prose:
  (a) that `cliwire` owns standalone/hub CLI wiring **resolution** for the standalone-capable CLIs, and that mode *selection* stays in `internal/preflight` — what happens after a mode is chosen is what lives here;
  (b) why it is a new package rather than a home in `internal/standalonegeom` (whose contract is that it never touches disk and derives nothing, while this logic stats, reads directories and seeds stencils), `internal/standalonestate` (barred by the Standalonestate Leaf Invariant), `internal/clihelp` (generic cobra plumbing for every CLI module, the wrong altitude for logic specific to the two standalone-capable ones), or `internal/preflight` (which imports `internal/lyxcwd` and `internal/fabricengine`, neither of which `cliwire` may import);
  (c) that per-CLI variance is carried as data on a `Module` descriptor each CLI declares in its own package, so no production file here names either caller — the `internal/shedrecipe` split, where the shared implementation lives in the module and the varying data lives outside it;
  (d) the fixed dependency set (stdlib plus `internal/standalonestate`, `internal/standalonegeom`, `internal/logger`, `internal/stencilstore`, `internal/buildinfo`, `contracts/stencils`) and the two deliberate exclusions — `internal/lyxcwd`, barred by the Told-Geometry Invariant and never needed since `cwd` arrives from the caller, and `internal/planparser`, kept out so webster's plan layout does not live inside a module burler shares;
  (e) that `cliwire` constructs no `Geometry` struct — it returns told strings each CLI feeds to `internal/hubgeom` and `internal/standalonegeom`, which remain the only `Geometry`-struct constructors;
  (f) that `ResolveStandalone` is a single entry point specifically so the prologue's **ordering** obligation lives in the type rather than in two copies of a prose comment.
  Write it as prose paragraphs, not a bullet list, matching the surrounding package docs' voice.
- **Commit:** `docs(cliwire): package header for the shared CLI wiring module`

### Card 2: pure path helpers

- **Context:**
  - `internal/webstercli/wiring.go`
  - `internal/burlercli/wiring.go`
  - `internal/standalonestate/standalonestate.go`
- **Edits:** none
- **Creates:**
  - `internal/cliwire/paths.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/cliwire/paths.go` in `package cliwire`, holding the helpers that produce no operator-facing message and therefore need no descriptor.
  Transcribe each body from `internal/webstercli/wiring.go` unchanged, carrying its doc comment across:
  - `const gitDirName = ".git"`, with its existing comment explaining that it is a plain literal rather than an `internal/lyxdirs` constant because `lyxdirs` declares loomyard's own `_lyx`/`.lyx` and has never owned git's.
  - `func ResolveToldDir(cwd, flagValue string) string` — exported, body identical to today's `resolveToldDir`.
    Keep webster's version of the doc comment (the one that also explains the standalone default-vs-override path equality a relative spelling can never satisfy), and adjust its closing cross-reference so it names `resolveStandaloneTarget` in this package rather than in `webstercli`.
  - `func pathContains(outer, inner string) bool` — unexported, body identical to today's, including the `runtime.GOOS == "windows"` case-folding branch and the comment explaining that it mirrors an already-stated rule and is never driven live on this project's Linux hosts.
  - `func NormalizeForContainment(path string) string` — exported, body identical to today's `normalizeForContainment`, doc comment carried across including the R6-15 explanation of why plain `standalonestate.Normalize` is not enough.
  - `func RepositoryRootOf(dir string) string` — exported, body identical to today's `repositoryRootOf`, doc comment carried across including the "nearest, never topmost" rule, the `os.Lstat`-not-`os.Stat` rationale, and the paragraph explaining why this is not a cwd query under the Cwd Resolution Invariant.
  - `func SamePlanDir(planDir, defaultPlanDir string) bool` — exported, body identical to today's `samePlanDir`, doc comment carried across including the R6-15 reason it compares through `NormalizeForContainment` rather than `filepath.Clean`.
  - `func ResolvePlanDir(toldPlanDir, defaultPlanDir string) (planDir string, overridden bool)` — new, and the one function in this file with no direct predecessor.
    It must reproduce today's inline plan-dir block in both `wireHub` and `wireStandalone` exactly: an empty `toldPlanDir` returns `(defaultPlanDir, false)`;
    a non-empty `toldPlanDir` returns `(toldPlanDir, !SamePlanDir(toldPlanDir, defaultPlanDir))` — that is, the told spelling always wins as the resolved directory, and `overridden` is true only when the told value names a *different* directory than the default.
    Give it a doc comment stating that it is a package function rather than a `Module` method because it is infallible, produces no message and reads no descriptor field, and that a told value naming the default location through a `"."`, trailing-separator or symlinked spelling is recognised as the default rather than as an override.
  Do not export `pathContains` and do not add any function beyond the seven listed here.
  This file must import only `os`, `path/filepath`, `runtime`, `strings`, and `github.com/Knatte18/loomyard/internal/standalonestate`.
- **Commit:** `feat(cliwire): pure path helpers shared by webster and burler wiring`

### Card 3: Module and PlanRules descriptors

- **Context:**
  - `internal/webstercli/wiring.go`
  - `internal/burlercli/wiring.go`
  - `internal/shedrecipe/registry.go`
  - `internal/cliwire/paths.go`
  - `_mill/discussion.md`
  - `internal/hubgeom/webstergeom.go`
- **Edits:** none
- **Creates:**
  - `internal/cliwire/module.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/cliwire/module.go` in `package cliwire`, declaring the descriptor types and the two message-producing pieces that read them.

  `type Module struct` with exactly these fields, each carrying its own doc comment naming what it varies:
  - `Name string` — the CLI's own name, used as the `"<name>: "` prefix on every error this package returns (`"webster"`, `"burler"`).
  - `StateArtifacts string` — the nested-geometry refusal's noun phrase for what standalone keeps outside the target.
  - `TargetRole string` — the nested-geometry refusal's noun phrase for the target.
  - `TargetRecourse string` — the reverse-nesting refusal's recourse clause opener.
  - `HubTargetSubject string` — the hub `--target-dir` refusal's subject clause.

  Each of those four field doc comments states only the role the field plays in the message it feeds, and the sentence position it occupies.
  None of them may quote webster's or burler's concrete value, name either CLI, or contrast the two — that would put both callers' exact noun phrases back inside the shared module, against this batch's rule that no production file here names either caller.
  The concrete values live only in each caller's own `wireModule` declaration, which batch 2 cards 6 and 7 spell out.
  - `Plan *PlanRules` — nil when the CLI parses no plan (burler's case), which is what makes `ResolveStandalone` skip plan-dir resolution entirely.

  `type PlanRules struct` with exactly two fields:
  - `DefaultPlanDir func(base string) string` — returns the default plan directory for a told base path.
    Webster fills it with `planparser.PlanDir`, and `ResolveStandalone` is its only caller, invoking it with the `stateDir` the prologue itself derived.
    Document that this is a function field rather than a finished string because standalone's default depends on that derived `stateDir`, so the caller cannot hand over a finished string, and rather than an import of `internal/planparser` because that would put webster's plan layout inside a module burler shares.
    Do not document it as also being called from `wireHub`.
    `_mill/discussion.md`'s exported-surface decision anticipated a hub call site, but card 6 resolves hub mode's override against `geom.PlanDir` — the value `hubgeom.WebsterGeometry` actually built — rather than re-deriving the same path through this field.
    The two are the same string today (`hubgeom.WebsterGeometry(loc).PlanDir` is `planparser.PlanDir(loc.AnchorPath())`), and comparing against the geometry's own field is what keeps the override check correct if `internal/hubgeom` ever changes how it computes `PlanDir`.
    Record that reasoning in the field's doc comment so a later reader does not "restore" the hub call.
  - `MissingPlanRefusal func(planDir, recourse string) string` — produces the whole refusal text for a plan directory that does not exist or holds no plan files.
    Document that it is a function rather than a bare string or a format string because webster's live message interpolates two distinct paths in a fixed order, and a function makes that argument order a compile-time fact rather than a comment.
    The second parameter is named `recourse`, not `defaultPlanDir`, deliberately: it is the location the refusal tells the operator to place the plan at, which is the mode's own default only when `--plan-dir` actually moved the plan off it and is the resolved plan directory itself otherwise.
    See card 4's step 6 for the rule that computes it.

  `func (m Module) RefuseTargetDirInHubMode(flag string) error` — returns `nil` when `flag` is empty, and otherwise the hub refusal built from `m.Name` and `m.HubTargetSubject`.
  The produced string must be byte-identical to today's, which for webster reads:

```
webster: --target-dir is not honoured in hub mode: the worktree is already the target, and honouring any other value would strand its artifacts outside fabric's positive-only commit pathspec
```

  and for burler differs only in the `burler:` prefix and the `the anchor path is already the target` clause.
  That fenced block is a plan-side illustration of the string the method produces at runtime, shown so the format string can be reconstructed exactly;
  it is not text to embed in a doc comment, and the method's own doc comment must not quote it or name either CLI.
  Carry across a doc comment recording that the refusal fires on the flag alone, before any config is loaded.

  `func (m Module) refuseNestedStandaloneGeometry(target, stateDir string) error` — unexported method, body identical to today's `refuseNestedStandaloneGeometry` except that `module` becomes `m.Name` and the two varying noun phrases become `m.StateArtifacts` and `m.TargetRole`, with `m.TargetRecourse` opening the reverse-nesting message's recourse clause.
  Both produced strings must be byte-identical to today's per-CLI messages.
  Carry across the whole existing doc comment, which explains that this guard front-runs `shuttleengine.NewDetachedRunner`'s own containment assertion — one that fires far too late, after a tmux server has been booted and the run lock taken, and blames a hub geometry that was never involved.

  Do not declare a `Module` value for webster or for burler in this package, and do not name either caller package in any production file here.
  This file must import only `fmt`.
- **Commit:** `feat(cliwire): Module and PlanRules descriptors carrying per-CLI variance`

### Card 4: the ResolveStandalone prologue

- **Context:**
  - `internal/webstercli/wiring.go`
  - `internal/burlercli/wiring.go`
  - `internal/cliwire/paths.go`
  - `internal/cliwire/module.go`
  - `internal/standalonegeom/webstergeom.go`
  - `internal/standalonegeom/stencilsdir.go`
  - `internal/standalonegeom/logsdir.go`
  - `internal/standalonestate/standalonestate.go`
  - `internal/logger/sink.go`
  - `internal/stencilstore/reconcile.go`
- **Edits:** none
- **Creates:**
  - `internal/cliwire/standalone.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/cliwire/standalone.go` in `package cliwire`, holding the request/result types and the single ordered standalone prologue.

  `type StandaloneRequest struct` with `Cwd`, `StencilsDirFlag`, `PlanDirFlag`, `TargetDirFlag`, all `string`.
  Document that these are the **raw, as-parsed** flag values exactly as they reach each CLI's `wire`, that `ResolveStandalone` makes them absolute itself via `ResolveToldDir`, and that this deliberately re-resolves values each `wire` has already resolved for its own hub branch — harmless, because `ResolveToldDir` is idempotent on an absolute input, and named here so a later reader does not "fix" the apparent double resolution by hoisting it back out.

  `type Standalone struct` with `Target`, `StateDir`, `Hash8`, `StencilsDir`, `PlanDir`, `DefaultPlanDir` (all `string`) and `PlanDirOverridden bool`.
  Document that `DefaultPlanDir` is the mode's own default, carried so a caller can record it for a refusal's recourse text, and that `PlanDir` and `PlanDirOverridden` are the zero value when the module carries no `Plan`.

  `func (m Module) ResolveStandalone(req StandaloneRequest) (Standalone, error)` performing exactly this sequence, in this order:
  1. `target, err := m.resolveStandaloneTarget(req.Cwd, req.TargetDirFlag)` — return the zero `Standalone` and the error on failure.
  2. `stateDir, hash8, err := standalonestate.Derive(target)` — return the zero `Standalone` and the error on failure, aborting the prologue.
  3. `m.refuseNestedStandaloneGeometry(target, stateDir)` — return the zero `Standalone` and the error on failure, aborting the prologue.
     Every fallible step below does the same: any error returns the zero `Standalone` alongside it, and no step is best-effort.
  4. `logger.SetDurableSinkDirWithWorktreeRoot(standalonegeom.LogsDir(stateDir), target)`.
  5. Stencils: `stencilsDir := ResolveToldDir(req.Cwd, req.StencilsDirFlag)`;
     when it is empty, set it to `standalonegeom.StencilsDir(stateDir)` and call `stencilstore.Reconcile(stencilsDir, stencils.Registry(), stencilstore.ModeFor(buildinfo.IsDev()), "")`, returning `fmt.Errorf("%s: seed the standalone stencils directory %s: %w", m.Name, stencilsDir, err)` on failure.
     When it is non-empty, the told directory is read and never written — no `Reconcile` call at all.
  6. Plan: when `m.Plan` is nil, skip this step entirely and leave `PlanDir`, `DefaultPlanDir` and `PlanDirOverridden` at their zero values.
     Otherwise compute `defaultPlanDir := m.Plan.DefaultPlanDir(stateDir)`, then `planDir, overridden := ResolvePlanDir(ResolveToldDir(req.Cwd, req.PlanDirFlag), defaultPlanDir)`, then check content: if `planDirHasContent(planDir)` is false, return `errors.New(m.Plan.MissingPlanRefusal(planDir, recourse))` where `recourse` is `defaultPlanDir` when `overridden` is true and `planDir` otherwise — reproducing today's `standaloneDefaultPlanDir` rule exactly, so the refusal always names the location `run` requires rather than the override the operator just supplied.
     This branch is why `MissingPlanRefusal`'s second parameter is named `recourse` rather than `defaultPlanDir`: in the not-overridden case the argument is `planDir` itself, so a `defaultPlanDir` name would misdescribe it.

  Carry across, onto `ResolveStandalone` itself, the long ordering comment both `wireStandalone` bodies carry today: the durable sink is armed lazily on the first Info-or-above record, so the redirect binds only if it runs before anything in the sequence can log;
  it cannot be first, because it is `Derive`'s own `stateDir` that tells it where to point;
  the obligation a later editor inherits is therefore that every statement above the redirect stays log-free and no logging statement is added below it that could be hoisted above.
  Also record on the function that the nested-geometry refusal must run after `standalonestate.Derive` and before both the sink redirect and any substrate boot, that the stencils seed's failure is a hard error in standalone because nothing else will ever create that directory (unlike the root pre-run's best-effort logged seed), and that the seed runs only for the derived default so a curated stencil set named by `--stencils-dir` is never rewritten from under the operator.
  State that these asymmetries are deliberate and must not be "simplified" away.

  `func (m Module) resolveStandaloneTarget(cwd, targetDirFlag string) (string, error)` — unexported method, body identical to today's `resolveStandaloneTarget` with the `"webster:"` prefix replaced by `m.Name`.
  Both produced messages must stay byte-identical to today's modulo that prefix;
  note that today's two copies already agree on the not-a-directory message word for word, including its `a repository to drive` phrasing, so no descriptor field is needed for it.
  Carry across the full doc comment, including the R6-7 paragraph explaining why a told `--target-dir` must exist and be a directory, and the note that `cwd` itself is never stat'd.

  `func planDirHasContent(dir string) bool` — unexported, body identical to today's `standalonePlanDirHasContent`, doc comment carried across including the point that a missing directory, an empty one, and one with no `*.md` files are all the same usage error.

  This file must import only `errors`, `fmt`, `os`, `strings`, `github.com/Knatte18/loomyard/contracts/stencils`, `github.com/Knatte18/loomyard/internal/buildinfo`, `github.com/Knatte18/loomyard/internal/logger`, `github.com/Knatte18/loomyard/internal/standalonegeom`, `github.com/Knatte18/loomyard/internal/standalonestate`, and `github.com/Knatte18/loomyard/internal/stencilstore`.
  Do not import `github.com/Knatte18/loomyard/internal/lyxcwd` and do not import `github.com/Knatte18/loomyard/internal/planparser`.
- **Commit:** `feat(cliwire): single ordered ResolveStandalone prologue`

### Card 5: cliwire's own tier-1 tests

- **Context:**
  - `internal/webstercli/wiring.go`
  - `internal/burlercli/wiring.go`
  - `internal/webstercli/wiring_test.go`
  - `internal/burlercli/wiring_test.go`
  - `internal/cliwire/paths.go`
  - `internal/cliwire/module.go`
  - `internal/cliwire/standalone.go`
  - `internal/logger/sink.go`
  - `internal/standalonegeom/logsdir.go`
  - `internal/standalonegeom/stencilsdir.go`
  - `internal/planparser/parse.go`
- **Edits:** none
- **Creates:**
  - `internal/cliwire/cliwire_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/cliwire/cliwire_test.go` in `package cliwire` (an internal test file, so it can drive the unexported `resolveStandaloneTarget`, `refuseNestedStandaloneGeometry` and `planDirHasContent`).
  It carries no build tag.

  Declare two package-level test fixtures standing in for the two real descriptors, named `websterFixture` and `burlerFixture`, each a `Module` value carrying that CLI's exact field values as listed in card 3 — `websterFixture` additionally carrying a `Plan` whose `DefaultPlanDir` is `func(base string) string { return filepath.Join(base, "_lyx", "plan") }` and whose `MissingPlanRefusal` reproduces webster's live message.
  Source that message from the `standalone plan directory ... does not exist or contains no plan files` refusal in `internal/webstercli/wiring.go`'s `wireStandalone`, copying it verbatim including its two interpolated paths in their existing order — the resolved plan directory first, the recourse location second.
  Add a comment stating that these are test fixtures mirroring the real descriptors declared in `webstercli` and `burlercli`, and that they exist so this package's tests never import either caller.
  The comment must also name what actually catches a divergence between a fixture and its real descriptor: the per-package `TestWireModule_DescriptorIsVerbatim` test that batch 2 cards 6 and 7 add, which pins each real `wireModule`'s own field values and produced refusals.
  Do not claim the existing composition tests or the two `cli_integration_test.go` files catch it — they do not;
  after batch 2 no surviving test in either CLI package asserts any descriptor field's text, and the retained `TestWire_TargetDirRefusedInHubMode` only checks that the error mentions `--target-dir`.

  Declare the test helpers, each with the doc comment its predecessor in `internal/webstercli/wiring_test.go` carries:
  - `hash8For(t *testing.T, target string) string` — returns `standalonestate.Derive`'s `hash8` under the environment `t.Setenv` has already installed.
  - `hash8AndStateDir(t *testing.T, target string) (stateDir, hash8 string)` — returns both of `standalonestate.Derive`'s values under that same already-installed environment, the predecessor of burler's helper of the same name.
    Every case that drives `ResolveStandalone` needs `stateDir` — to seed the default plan directory, to assert against `standalonegeom.LogsDir(stateDir)` and `standalonegeom.StencilsDir(stateDir)`, and to make the derived stencils path uncreatable — so this is the helper those cases call, not `hash8For`.
  - `seedGitRepositoryRoot(t *testing.T, dir string) string` — creates `dir/.git` as a directory and returns `dir`, spawning no git.
  - `seedPlanDir(t *testing.T, dir string)` — `MkdirAll` plus one minimal `00-overview.md`, the predecessor of webster's `seedStandalonePlanDir`.
  - `setStandaloneStateRoot(t *testing.T)` — redirects both `XDG_STATE_HOME` and `LOCALAPPDATA` to fresh `t.TempDir()` values, the predecessor of burler's helper of the same name.

  Write these tests, each carrying a doc comment stating what it pins and, where it is a regression test, which crucible finding:
  - Target resolution, table-driven over both fixtures where the message differs: an unset `--target-dir` resolves to cwd and is never stat'd;
    an absolute value is cleaned;
    a relative value resolves against the told cwd;
    a value naming an absent path is refused;
    a value naming a file is refused (both refusals are R6-7).
    Assert each refusal message verbatim per fixture, since a message rewrite is exactly the drift this task ends.
  - Repository-root lift: a subdirectory of a repository resolves to the repository root;
    the **nearest** `.git` wins rather than the topmost, so a nested repository or submodule stays its own repository;
    a directory with no repository above it comes back unchanged;
    a `.git` **file**, as a linked worktree records it, counts as a repository root.
  - Nested-geometry refusal, over both fixtures: a state directory inside the target is refused with the state-home lever;
    a target inside the state directory is refused with the target lever;
    disjoint paths pass.
    Assert both fixtures' wordings verbatim — this is what pins the per-CLI noun phrases as descriptor data.
  - The R6-15 symlink case: a state home that reaches inside the target only through a symlink, with the `<stateHome>/lyx/<hash8>` leaf deliberately absent, is still refused.
    `t.Skipf` when `os.Symlink` fails.
  - `SamePlanDir` and `ResolvePlanDir`: a `"."` spelling and a trailing-separator spelling of the default are recognised as the default rather than as an override;
    a symlinked spelling of the default likewise (the second half of R6-15);
    an empty told value resolves to the default with `overridden` false;
    a genuinely different told directory resolves to itself with `overridden` true.
  - Plan rules driven through `ResolveStandalone`: a missing plan directory, an empty one, and one holding no `*.md` files all produce the same refusal;
    the refusal names the **default** location even when `--plan-dir` moved the plan off it;
    a nil `Plan` field (the `burlerFixture` case) means the prologue performs no plan check at all and leaves `PlanDir`, `DefaultPlanDir` and `PlanDirOverridden` at their zero values.
  - Prologue ordering: on success the durable sink is pointed at `standalonegeom.LogsDir(stateDir)` with the worktree root at the target — assert by setting a sentinel sink directory before the call, checking it stayed empty afterwards, then emitting one `logger.Info` record and finding a `trace-*.log` under the logs directory.
    On a prologue that fails at the nested-geometry refusal, the sentinel sink is still the one that receives the record, which is what proves the redirect sits after the guard.
    Restore the sink with `t.Cleanup(func() { logger.SetDurableSinkDir("") })` in every case that touches it.
  - Stencils: the derived default is seeded on disk;
    an explicitly-told stencils directory is returned as given and gains no entries;
    the returned `StencilsDir` is `standalonegeom.StencilsDir(stateDir)` for the default case;
    and a seed failure on the derived default is a hard error naming both `m.Name` and the directory.
    Drive that last case by making the derived stencils path uncreatable before the call — write a regular file where `standalonegeom.StencilsDir(stateDir)` needs a directory, or at an ancestor of it under `stateDir` — so `stencilstore.Reconcile` fails, then assert the returned error's text.
    This is `ResolveStandalone` step 5's only error return, and this package is now its sole owner;
    leaving it unexercised would put the one hard-error asymmetry the prologue documents outside its own coverage.
  Every case that reaches `standalonestate.Derive` must call `setStandaloneStateRoot` first and must not be `t.Parallel()`.
- **Commit:** `test(cliwire): tier-1 coverage for the shared wiring prologue`

## Batch Tests

`verify:` runs the full untagged suite plus a scoped tagged run over the four packages this task's behaviour lives in.

The untagged half is deliberately unscoped, and this is the batch's `verify-full-suite` justification: `internal/cliwire` is a new package that batch 2 makes a cross-cutting dependency of two CLI packages, and batch 3 adds two AST-walking enforcement tests that parse every `.go` file under `internal/` and `cmd/` — so a change anywhere in the tree can fail this task's own gates.
`cmd/lyx/prerunlogging_test.go` also asserts the standalone durable-sink redirect that this batch relocates into `ResolveStandalone`.
A scoped run would leave both of those unchecked.
It also matches the hub's configured `pipeline.done_gate`, so the batch gate and the task gate agree.

The tagged half is not optional: `internal/webstercli/cli_integration_test.go`, `internal/burlercli/cli_integration_test.go` and `internal/standalonegeom/reedgeom_symlink_integration_test.go` all carry `//go:build integration`, and the webster one exists specifically to exercise the real `standalonestate.Derive` and the real standalone stencil seed end-to-end — the exact code this batch creates a second copy of and batch 2 consolidates onto.
An untagged-only gate would never execute it.
`smoke_test.go` (`//go:build smoke`) stays out: it drives live agent substrate and covers nothing this change touches.

Within this batch the new coverage is `internal/cliwire/cliwire_test.go` (card 5).
Both existing `wiring_test.go` files must still pass unchanged — this batch deletes nothing.
