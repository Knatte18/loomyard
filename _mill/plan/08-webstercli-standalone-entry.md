# Batch: webstercli standalone entry

```yaml
task: websterengine + webstercli told-geometry, and Webster standalone entry
batch: webstercli standalone entry
number: 8
cards: 10
verify: go test ./internal/webstercli/... ./cmd/lyx/... && go test -tags integration ./internal/webstercli/...
depends-on: [2, 3, 6, 7]
```

## Batch Scope

This batch makes `lyx webster` runnable outside a lyx hub.
`PersistentPreRunE` shrinks to three things — the bare-`webster` guard, `lyxcwd.CwdFrom`, and one `preflight.HubPresent` call — and hands the result plus the parsed flag values to an extracted, package-private wiring function that computes the mode and builds the whole engine stack.
The mode decision lives *inside* that function on purpose: driving the real pre-run would reach `lyxcwd.Resolve` and its git spawn, so a truth-table test could not stay tier 1 if the decision sat upstream.
Three new persistent flags land here — `--stencils-dir` and `--plan-dir`, read-only and honoured in both modes, and `--target-dir`, standalone-only and explicitly refused in hub mode.
The `websterCLI.layout` field is deleted outright rather than left nil-in-standalone, so the compiler enumerates every remaining consumer instead of leaving them as unguarded panics waiting for the first standalone verb.
The `CONSTRAINTS.md` rewords the shipped code makes necessary land in this batch too, per CLAUDE.md's same-commit docs rule.

## Cards

### Card 34: Add the three persistent flags

- **Context:**
  - `internal/planparser/parse.go`
  - `internal/fabricengine/junctionnames.go`
  - `internal/webstercli/run.go`
- **Edits:**
  - `internal/webstercli/cli.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Declare three persistent string flags on the `webster` parent command, bound to variables the `PersistentPreRunE` closure can read.
  `--stencils-dir` is optional in both modes, honoured in both, and read-only in both;
  its hub default is the hub's stencils directory and its standalone default is the state directory's own `_lyx/stencils`.
  `--plan-dir` is optional in both modes and read-only in both;
  its hub default is `planparser.PlanDir` over the anchor path, identical to today, and its standalone default is the same accessor over the state directory.
  `--target-dir` is standalone-only, defaults to the current working directory, and is resolved to an absolute path at this CLI boundary.
  Each flag's usage string must be accurate about which mode honours it and whether it is read-only, since help accuracy is a review obligation under the CLI / Cobra Invariant and these three are new observable behaviour.
  Extend the parent command's `Long` with a short "Modes" paragraph naming hub mode and standalone mode and listing the three flags with a concrete example of a standalone invocation, so the behaviour is self-discoverable from `lyx webster --help`.
  Declare only the flags and the variables in this card;
  card 35 consumes them.
- **Commit:** `feat(webstercli): add the --stencils-dir, --target-dir and --plan-dir persistent flags`

### Card 35: Extract the wiring function and decide the mode inside it

- **Context:**
  - `internal/preflight/predicates.go`
  - `internal/preflight/doc.go`
  - `internal/hubgeom/webstergeom.go`
  - `internal/hubgeom/hubgeom.go`
  - `internal/standalonegeom/reedgeom.go`
  - `internal/standalonegeom/webstergeom.go`
  - `internal/standalonestate/standalonestate.go`
  - `internal/websterengine/geometry.go`
  - `internal/websterengine/audit.go`
  - `internal/websterengine/config.go`
  - `internal/reedengine/config.go`
  - `internal/shuttleengine/config.go`
  - `internal/shuttleengine/rundir.go`
  - `internal/batcher/config.go`
  - `internal/modelspec/modelspec.go`
  - `internal/fabricengine/refscanner.go`
  - `internal/fabricengine/open.go`
- **Edits:**
  - `internal/webstercli/cli.go`
- **Creates:**
  - `internal/webstercli/wiring.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/webstercli/wiring.go` holding one package-private method on `*websterCLI` that takes the `preflight.HubPresent` result — the resolved `*lyxcwd.Location` and the boolean — plus the three parsed flag values, computes the mode, and builds the whole engine stack onto the receiver.
  The truth table is one row each: hub present means hub mode, because the Location's paths are real;
  hub absent means standalone mode, which covers both the plain downloaded git repository and the unresolvable cwd, since `HubPresent` returns a nil Location for each and makes them indistinguishable.
  `preflight.Wired` is deliberately not consulted;
  record that in the function's doc comment with its reason, namely that `Wired` is a per-worktree question that is false in three healthy hub locations which run webster verbs today, so keying on it would either refuse them or relocate a live hub's state into the standalone state directory.
  In hub mode the function builds: the module configs and the model registry over the anchor path, `hubgeom.WebsterGeometry` and `hubgeom.ReedGeometry`, a `refMatcher` from `fabricengine.NewRefScanner`, and an `openFabric` closure over `fabricengine.Open`;
  it applies `--stencils-dir` and `--plan-dir` as overrides on the built geometry when non-empty;
  and it refuses a non-empty `--target-dir` with an explicit error saying the value is structurally the worktree in hub mode and that honouring it would strand artifacts outside fabric's positive-only commit pathspec.
  In standalone mode the function calls `standalonestate.Derive` on the already-absolute target — this is the only place `Derive` is ever called — and passes its two outputs into `standalonegeom.ReedGeometry` and `standalonegeom.WebsterGeometry`, loads every module config and the model registry over the derived state directory, sets `refMatcher` to `websterengine.NeverMatches{}`, and leaves `openFabric` nil.
  The function must perform no cwd resolution and spawn no process, so that its unit test stays tier 1.
  In `PersistentPreRunE`, keep only the bare-`webster` guard, `lyxcwd.CwdFrom`, one `preflight.HubPresent(cwd)` call, and the delegation to this function, mapping its error onto the existing `output.Err` plus `clihelp.Abort` shape.
  Delete the `lyxcwd.Resolve` call and every config/engine construction that moves into the new file.
- **Commit:** `feat(webstercli): extract the wiring function and select hub or standalone mode inside it`

### Card 36: Delete `websterCLI.layout`

- **Context:**
  - `internal/websterengine/geometry.go`
  - `internal/planparser/validate.go`
  - `internal/webstercli/wiring.go`
- **Edits:**
  - `internal/webstercli/cli.go`
  - `internal/webstercli/validate.go`
  - `internal/webstercli/beginbatch.go`
  - `internal/webstercli/recordbatch.go`
  - `internal/webstercli/recoverbatch.go`
  - `internal/webstercli/run.go`
  - `internal/webstercli/status.go`
  - `internal/webstercli/awaitbatch.go`
  - `internal/webstercli/pause.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Delete the `layout *lyxcwd.Location` field from `websterCLI`, along with the now-redundant `planDir`, `websterDir`, `reportsDir`, `promptsDir` and `websterScratchDir` fields, whose values all live on `c.geom` after batch 7.
  Add an `anchorRel string` field carrying the hub mode Location's own anchor-relative path, empty in standalone;
  it is the one thing the deleted field held that no other replacement supplies, and card 37 needs it.
  Every remaining consumer becomes a `c.geom` read: `c.planDir` becomes `c.geom.PlanDir`, `c.websterDir` becomes `c.geom.WebsterDir`, `c.reportsDir` becomes `c.geom.ReportsDir`, `c.promptsDir` becomes `c.geom.PromptsDir`, and `c.websterScratchDir` becomes `c.geom.ScratchDir`, across all eight verb files.
  In `internal/webstercli/validate.go`, `planparser.Validate(plan, c.layout.AnchorPath())` becomes `planparser.Validate(plan, c.geom.WorktreeRoot)`.
  That parameter is named `worktreeRoot` at its declaration and resolves each card's move source and target files, so the told worktree root is correct on its own terms;
  hub mode's `WorktreeRoot` is the anchor path, exactly the value passed today, so no hub behaviour changes.
  Removing the field rather than leaving it nil is the point of this card: every surviving dereference would otherwise be an unguarded panic on the first standalone verb, and deleting it makes the compiler enumerate the consumers instead.
  Drop the `internal/lyxcwd` import from any file that no longer uses it.
- **Commit:** `refactor(webstercli): delete the layout field and read every path off the told Geometry`

### Card 37: `fabricSync` takes a lazy opener and a told anchor-rel

- **Context:**
  - `internal/fabricengine/fabric.go`
  - `internal/fabricengine/open.go`
  - `internal/lyxdirs/dirs.go`
  - `internal/webstercli/wiring.go`
- **Edits:**
  - `internal/webstercli/sync.go`
  - `internal/webstercli/run.go`
  - `internal/webstercli/beginbatch.go`
  - `internal/webstercli/recordbatch.go`
  - `internal/webstercli/recoverbatch.go`
  - `internal/webstercli/cli_test.go`
  - `internal/webstercli/sync_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Change `fabricSync` from `fabricSync(layout *lyxcwd.Location, label string)` to `fabricSync(open func() (*fabricengine.Fabric, error), anchorRel, label string) (bool, error)`.
  It needs two things the deleted field supplied and neither survives on its own: the anchor-relative path for `fabricengine.ScopedPathspec`, and the handle itself.
  `fabricengine.Fabric` exposes no anchor-relative path, so the pathspec scope must be told separately rather than derived from the handle.
  Preserve the existing statement order exactly: read `fabricengine.EnvSyncOptions()` first and open only inside the `if !opts.SkipGit` branch, so the skip-git environment override still means *never open*.
  Add one branch: when `open` is nil, return `(false, nil)` without touching git — that is standalone mode, which has no fabric repo by construction.
  Update all five call sites to pass `c.openFabric` and `c.anchorRel`.
  Only `run` surfaces the returned boolean, through its existing `fabricCommitted` envelope field, which therefore reports false in standalone;
  the other four already discard it and surface only a sync error, so they emit nothing new there.
  Update the two existing test files that call `fabricSync` directly to the new signature, wrapping their Location in an opener closure;
  keep both files' assertions intact, including the skip-git bypass case that proves the filesystem is never touched and the missing-pair case that proves the open failure propagates.
  Update the file-header comment to describe the opener and the told scope.
- **Commit:** `refactor(webstercli): give fabricSync a lazy opener and a told anchor-relative scope`

### Card 38: Seed the standalone default stencils directory

- **Context:**
  - `cmd/lyx/stencilseed.go`
  - `internal/stencilstore/reconcile.go`
  - `internal/stencilstore/stencilstore.go`
  - `internal/buildinfo/buildinfo.go`
  - `contracts/stencils/stencils.go`
- **Edits:**
  - `internal/webstercli/wiring.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In the standalone branch of the wiring function, seed the default stencils directory on first use with `stencilstore.Reconcile` over the shipped registry, using `stencilstore.ModeFor(buildinfo.IsDev())` for the mode and the empty string for the source directory.
  The empty source directory means "no source tree here" and is what keeps the port-back drift warning silent, exactly as the root pre-run's own seed already does when it finds no source tree.
  Reusing `buildinfo.IsDev()` rather than hardcoding a production mode is required by the Dev/Prod Binary Separation invariant and keeps a dev binary's seeding semantics identical standalone and in-hub.
  Seed **only** the standalone default: when `--stencils-dir` was passed explicitly, in either mode, never reconcile it — an operator who names a curated stencil set must not have it rewritten from under them.
  Never seed in hub mode at all;
  the root pre-run already owns that pass and this task must not touch it.
  A reconcile failure here is a hard error the wiring function returns, not a logged warning: unlike the root pre-run's best-effort pass, nothing else will ever create this directory and every prompt render would fail later with a less informative message.
- **Commit:** `feat(webstercli): seed the standalone default stencils directory on first use`

### Card 39: An absent standalone plan directory is a usage error

- **Context:**
  - `internal/planparser/parse.go`
  - `internal/websterengine/runlevel.go`
- **Edits:**
  - `internal/webstercli/wiring.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In the standalone branch only, after the plan directory is resolved (defaulted or supplied via `--plan-dir`), refuse with a plain usage error naming the `--plan-dir` flag when that directory does not exist or contains no plan files.
  There is no bootstrap and no empty-plan fallback: a plan is authored content, not a shipped default.
  Do not route the absence into the zero-batch refusal `run` already performs — that refusal exists for a plan that parses to nothing and would report the wrong cause for a directory that is simply not there.
  Hub mode's behaviour is unchanged: no new gate, no new error.
  Do not add a git-repository check on `--target-dir`;
  the missing-repository failure must surface at the verb that needs a commit SHA, which is what keeps `validate` and `status` usable in a plain directory and what the batch's tagged test depends on.
- **Commit:** `feat(webstercli): refuse an absent standalone plan directory with a --plan-dir usage error`

### Card 40: Update `webstercli`'s package doc

- **Context:**
  - `internal/webstercli/wiring.go`
  - `internal/webstercli/sync.go`
  - `internal/websterengine/geometry.go`
- **Edits:**
  - `internal/webstercli/cli.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Rewrite the package doc at the top of `internal/webstercli/cli.go`.
  Its current description of `PersistentPreRunE` resolving cwd through layout to the engine stack, and its claim that every plan and webster path "is anchored at layout.AnchorPath() -- the directory lyx init ran in", are both false after this batch.
  The replacement must state: the pre-run resolves cwd and one `preflight.HubPresent` probe, then delegates to the wiring function, which selects hub or standalone mode and builds the stack;
  every path the module touches arrives as a `websterengine.Geometry` from `hubgeom` or `internal/standalonegeom`;
  the module holds no `*lyxcwd.Location`;
  and fabric is reached only through a lazy opener that is nil in standalone.
  Keep the existing paragraph explaining the three adapted views of the one constructed shuttle Runner — it is unaffected and still accurate.
- **Commit:** `docs(webstercli): describe the two-mode pre-run and the told geometry`

### Card 41: The tier-1 wiring truth-table test

- **Context:**
  - `internal/webstercli/wiring.go`
  - `internal/webstercli/cli.go`
  - `internal/webstercli/validate.go`
  - `internal/standalonegeom/reedgeom.go`
  - `internal/standalonegeom/webstergeom.go`
  - `internal/hubgeom/webstergeom.go`
  - `internal/websterengine/geometry.go`
  - `internal/websterengine/audit.go`
  - `internal/preflight/predicates.go`
  - `internal/webstercli/cli_test.go`
- **Edits:** none
- **Creates:**
  - `internal/webstercli/wiring_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create an untagged test file driving the wiring function directly with a told `preflight.HubPresent` result, never through the real pre-run.
  It must spawn no process and resolve no cwd, so the package stays inside the Test Tier Purity Invariant;
  it must not call `standalonestate.Derive` in the assertions it can avoid it in, and where the wiring function itself reaches `Derive`, redirect the environment first with the per-OS variable that function reads before any such case runs, and do not mark those cases parallel.
  Cover, at minimum:
  hub-present true selects hub mode, including three fixtures standing in for the healthy-but-unwired locations — a board-level worktree, an unpaired sibling, and a worktree whose pair was removed — none of which may be refused and none of which may land in standalone;
  hub-present false selects standalone mode for both the plain downloaded repository and the unresolvable cwd, which the predicate makes indistinguishable;
  the plan directory resolves to the hub default, the standalone default, and an explicit override, and an absent standalone plan directory produces a usage error whose message names `--plan-dir`;
  `--target-dir` is refused in hub mode with an error that says why;
  `planparser.Validate` receives the told worktree root in both modes, and the `websterCLI` struct carries no Location field at all;
  in standalone the pane cwd, the worktree root, the audit workdir and the prompt worktree-root token all resolve to the target while every `_lyx` and `.lyx` path and every module config base resolves under the derived state directory;
  and the two seams that must never be nil-or-eager — the matcher is non-nil in both modes, and the fabric opener is nil in standalone and, in hub mode, is left **uninvoked** by the wiring itself.
  That last assertion is what keeps the three healthy-but-unwired locations working and must be written as an explicit check that no fabric handle was opened, not inferred from the absence of an error.
- **Commit:** `test(webstercli): pin the wiring function's mode truth table at tier 1`

### Card 42: The tagged standalone pre-run test

- **Context:**
  - `internal/perchcli/cli_integration_test.go`
  - `internal/webstercli/testmain_test.go`
  - `internal/webstercli/cli.go`
  - `internal/webstercli/wiring.go`
  - `internal/standalonestate/standalonestate.go`
  - `internal/standalonestate/doc.go`
- **Edits:** none
- **Creates:**
  - `internal/webstercli/cli_integration_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create a `//go:build integration` test driving `RunCLIIn` with a temporary directory that is outside any git repository, and assert the pre-run reaches the `run` verb's **own** flag validation rather than failing with a cwd-resolution error.
  Follow the shape `internal/perchcli/cli_integration_test.go` already uses for a tagged CLI-level test;
  this package already has a hermetic `TestMain` and an existing tagged file, so no new test-main wiring is needed.
  This test reaches the real `standalonestate.Derive` and the standalone stencil seed, so it must redirect the state directory to a temporary directory first, using the per-OS environment variable `Derive` actually reads on the running platform.
  Without that redirect it would write into the operator's real home directory, outside any temporary directory, surviving the run and differing per machine.
  The redirect forbids marking this test parallel, which is correct.
  Add a second case asserting that after the invocation the target directory itself is unchanged — no hidden state tree, no lock file, no rendered prompt — which is the property the two-roots split exists to guarantee and the one thing no unit test in this batch observes.
- **Commit:** `test(webstercli): drive the standalone pre-run from a non-repository temp dir`

### Card 43: Reword the invariants the shipped code now contradicts

- **Context:**
  - `internal/webstercli/wiring.go`
  - `internal/standalonegeom/webstergeom.go`
  - `internal/websterengine/audit.go`
  - `internal/stencilstore/reconcile.go`
- **Edits:**
  - `CONSTRAINTS.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Four edits, each in its own invariant, all worded generally rather than for this one module — a sibling wave applies the same shape to the other producers and should confirm this wording rather than rewrite it.
  In the Stencil Ownership Invariant, reword the opening read-location statement so it names a told absolute stencils directory, with the board-level path given as what hub mode resolves to rather than as the only possibility.
  Reword the seed-pass bullet to say the pass runs once per process either at the root pre-run in hub mode or at the producer CLI's own pre-run in standalone mode, preserving its load-bearing second half — never lazily inside the stencil read path — verbatim.
  In the Durable-vs-Ephemeral State Invariant, add that a standalone session's durable and never-tracked trees are ordinary directory siblings under the per-OS state directory, which satisfies the mirrored-subpath rule rather than deviating from it.
  In the Fabric Git Invariant, the enforcement bullet states that the agent half "is machine-checked for webster runs by `fabricengine.RefScanner`";
  add a one-clause qualifier limiting that to hub-mode runs, since standalone supplies a never-matching matcher instead.
  Say why in the same sentence: standalone has no weft worktree and no fabric verb for a fork to drive, so there is nothing there for the check to catch — a reader must not be left believing the guard is universal.
  Do not touch the Config Strictness Invariant, which an earlier batch already updated, and do not add a new invariant — the three-tier one belongs to a later task.
  Use semantic line breaks throughout, per CLAUDE.md.
- **Commit:** `docs(constraints): reword the stencil, state-tree and fabric-check invariants for standalone`

## Batch Tests

`verify:` runs `go test ./internal/webstercli/... ./cmd/lyx/...` for the untagged suites and `go test -tags integration ./internal/webstercli/...` for the tagged half, which is required rather than optional: card 42 creates a `//go:build integration` file, and the tagged suite is the only place the real `standalonestate.Derive`, the standalone stencil seed, and the end-to-end pre-run are exercised at all.
`./cmd/lyx/...` is in scope because the new flags flow into the help tree that package's drift and help-tree guards walk, and because its seam-signature guard pins the two exported entry points this batch leaves unchanged.
Card 41 is the batch's main assertion surface and is only reachable at tier 1 because the mode decision lives inside the extracted function — driving the real pre-run would spawn git and breach the Test Tier Purity Invariant.
Its two hardest assertions are the ones that would otherwise pass under a broken implementation: the three healthy-but-unwired fixtures, which a `Wired`-keyed implementation would silently send to standalone, and the uninvoked-opener check, which an eager fabric open would break in exactly those three locations and nowhere else.
Card 42's untouched-target-directory assertion covers the one property no unit test can see.
The remaining manual acceptance step, outside this batch's automated scope, is a real standalone run from a scratch directory confirming that Master's own pane starts in the target rather than in the state directory.
