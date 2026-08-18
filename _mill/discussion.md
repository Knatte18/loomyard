# Discussion: websterengine + webstercli told-geometry, and Webster standalone entry

```yaml
task: websterengine + webstercli told-geometry, and Webster standalone entry
slug: webster-told-geometry
status: discussing
parent: standalone-producers
```

## Problem

`lyx webster` can only run inside a wired lyx hub.
Its engine takes a `*lyxcwd.Location` and derives everything from it — the `_lyx/webster` state tree, the `.lyx/webster` scratch tree, the stencils directory (`fabricengine.StencilsDir(l.HubPath)`), the plan directory, the fabric ref-scanner, and the bisect handle.
`internal/webstercli`'s `PersistentPreRunE` calls `lyxcwd.Resolve` unconditionally and aborts when it fails, so there is no way to point webster at an ordinary directory.

This is task **T7** of `manifest/designs/producers-standalone.md`, wave 3.
Waves 1 and 2 have landed: `planparser.PlanDir(anchorRoot string)` (T1), `configengine.LoadOrTemplate` (T2), `shuttleengine.NewRunner`'s told signature and `shuttleengine.FindRun(cfg, anchorRoot, guid)` (T3), `pattern.Directive(anchorRoot, stencilsDir, role)` (T4), and `internal/preflight` / `internal/buildinfo` / `internal/standalonestate` (T5).
Everything this task needs already exists;
what is missing is webster's own conversion and its standalone entry.

**Why now:** T7 is one of wave 3's two tasks and the wave is scheduled.
Its sibling T6 (`burler-perch-told-geometry`) runs in parallel.
T8 (the burler/perch standalone entry) lands *after* this task and explicitly reuses what this task establishes, so several things the design doc assigns to T8 have to be landed here instead — see the Decisions on `internal/standalonegeom`, on `batcher`, and on the `CONSTRAINTS.md` rewords.

## Scope

**In:**

- Convert `internal/websterengine`'s `*lyxcwd.Location` surface to told strings: `state.go` (4 accessors), `render.go` (3 render functions), `runlevel.go`, `beginbatch.go`, `recordbatch.go`, `recoverbatch.go`, `audit.go`, and `doc.go`.
- Introduce `websterengine.Geometry` and `hubgeom.WebsterGeometry`.
- Replace `internal/fabricengine` inside `websterengine` with two engine-owned interfaces (`RefMatcher`, and the existing `FabricBisector` always injected).
- New package `internal/standalonegeom`: told-mode geometry builders (reed geometry first, webster geometry beside it), the sibling of `internal/hubgeom`.
- `internal/reedengine`: one additive `Geometry.PaneCwd` field, consumed at the two tmux spawn sites (`lifecycle.go:294,489`), hub-neutral by construction — see the `standalone-pane-cwd-is-told-separately-from-anchorpath` Decision.
- `internal/webstercli`: hub/standalone mode selection in `PersistentPreRunE`, an extracted tier-1-pure wiring function, and the three new persistent flags `--stencils-dir`, `--target-dir`, `--plan-dir`.
- `internal/batcher`: `Active` moves from `configengine.Load` to `configengine.LoadOrTemplate`.
- `cmd/lyx/constructoranchoring_test.go`: webster's four rows rewritten to the told shape.
- `CONSTRAINTS.md`: Stencil Ownership reword, Durable-vs-Ephemeral clarification, Config Strictness pinned-set update, CLI/Cobra help obligations for the three new flags.
- `internal/websterengine/doc.go` and `internal/webstercli/cli.go`'s package doc updated in the same commit (CLAUDE.md's same-commit docs rule).

**Out:**

- `internal/burlerengine`, `internal/perchengine`, `internal/burlercli`, `internal/perchcli` — those are T6 and T8.
- `internal/scoutengine` / `internal/scoutcli` — T9, optional.
- The new three-tier `CONSTRAINTS.md` invariant and `docs/overview.md` consolidation — T10.
- `cmd/lyx/stencilseed.go` and `cmd/lyx/main.go` gating — already done by T5;
  this task must not touch either.
- Any change to what `lyxcwd.Resolve` validates, or to `internal/lyxcwd`'s ownership of cwd resolution.
- A positional-argument shape (`lyx webster run <path>`).
- Making the standalone target a review target: `--target-dir` is a resolution base only.

## Decisions

### webster-geometry-struct

- **Decision:** add `websterengine.Geometry`, mirroring `reedengine.Geometry`, carrying the eight told values webster needs:
  `AnchorRoot`, `WorktreeRoot`, `WebsterDir`, `ReportsDir`, `PromptsDir`, `ScratchDir`, `StencilsDir`, plus `PlanDir`.
  `internal/hubgeom` gains `WebsterGeometry(l *lyxcwd.Location) websterengine.Geometry`;
  `internal/standalonegeom` gains the told-mode builder.
  `RunDeps`, `BeginDeps`, `RecordDeps` and `RecoverDeps` each carry a `Geom Geometry` field replacing `Layout` and the flat `WebsterDir`/`ReportsDir`/`PromptsDir`/`ScratchDir`/`WorktreeRoot`/`PlanDir` fields they hold today.
- **Rationale:** the design doc names `hubgeom` as the single site for the `Location`-to-geometry conversion, and `hubgeom` needs a type to return.
  One struct also keeps the four Deps structs from drifting apart on the same eight values, and it is exactly the shape `reedengine` already proved.
  The told direction stays one-way: `hubgeom` and `standalonegeom` import `websterengine`, never the reverse.
- **Rejected:** keeping the flat fields and having `hubgeom.WebsterGeometry` return a `webstercli`-local bag — the bag would have to be re-declared in `standalonegeom` too, which is the duplication `hubgeom` exists to prevent.

### path-accessors-take-anchorroot

- **Decision:** `websterengine.Dir`, `ReportsDir`, `ScratchDir`, `PromptsDir` change from `func(l *lyxcwd.Location) string` to `func(anchorRoot string) string`.
  `ReportsDir(anchorRoot)` still composes on `Dir(anchorRoot)`, and `PromptsDir(anchorRoot)` on `ScratchDir(anchorRoot)`, so the `_lyx`/`.lyx` mirroring is unchanged.
- **Rationale:** told-geometry, per the design's core decision.
  `websterengine` remains the sole declarer of these paths (Cwd Resolution Invariant) — only the parameter type changes.
- **Rejected:** dropping the accessors and putting the joins in `hubgeom` — that would move path construction out of the owning module and break the invariant.

### render-takes-told-strings

- **Decision:** `RenderForkPrompt`, `RenderRecoveryPrompt` and `RenderMasterPrompt` drop their `l *lyxcwd.Location` parameter for `anchorRoot, stencilsDir string`, matching `RenderIntegrationPrompt`'s existing told shape.
  This removes the `internal/fabricengine` import from `render.go`.
- **Rationale:** the four `fabricengine.StencilsDir(l.HubPath)` derivations (three in `render.go`, one in `runlevel.go:470`) are the only reason webster knows a hub exists at all on the render path.
  `pattern.Directive` already takes `(anchorRoot, stencilsDir, role)` after T4, so both call sites in `render.go` need no adaptation beyond the new parameters.
- **Rejected:** a `Geometry` parameter on the render functions — they need two strings, and taking the whole struct would make them harder to test than they are today.

### worktree-root-token-value-preserved

- **Decision:** the preserve-today's-value ruling **binds hub mode only**.
  - **Hub mode:** `{{.worktree_root}}` in the fork and recovery prompts keeps being filled from **`anchorRoot`** — the exact value `l.AnchorPath()` supplies today — and `RenderIntegrationPrompt` keeps being filled from `Geom.WorktreeRoot`, also `l.AnchorPath()` at every call site today.
    No hub-mode prompt byte changes.
  - **Standalone mode:** every prompt that carries the token fills it from **`Geom.WorktreeRoot`** — the `--target-dir` value — never from `anchorRoot`.

  Concretely, the renderers take both roots and the caller supplies them;
  the mode does not leak into `render.go`, because in hub mode the CLI passes the same value for both.
- **Rationale:** `{{.worktree_root}}` is not a label — it is the working directory the agent is instructed to act in.
  `contracts/stencils/webster/webster-body-implementer.md:28,31` tells the implementer to run `go build ./...` and its unit tests from `{{.worktree_root}}` and to **commit the card** from it;
  `webster-template-integration.md:21,29` tells the integration fork to run the verify command there.
  In standalone `anchorRoot` is `<state>`, which is not a git repository and holds no source — so filling the token from `anchorRoot` would point every implementer and the integration fork at the hidden state directory, and the commit step would fail outright.
  The hub-mode half of the original ruling still stands for its original reason: `webstercli` passes `layout.AnchorPath()` for *both* roots today (`run.go:71`, `beginbatch.go:100`, `recordbatch.go:105`, `recoverbatch.go:126`), so converging hub mode on `WorktreePath()` would silently change prompt text in a subpath-anchored hub with no test looking for it.
- **`RenderMasterPrompt` is not in this list.** It renders no `worktree_root` key at all (`render.go:242-251` fills `batch_index`, `progress`, `outcome_path`, `summary_path`, `integration_prompt_path`, `self_fix_cap`, `poll_wait_s`, `pattern_directive`).
  The plan must not add the token to `webster-template-master`.
- **Rejected:** converging hub mode on `WorktreeRoot` too — a real behaviour change in nested-anchor hubs, out of scope.
- **Rejected:** one uniform value for both modes — the whole point of the two roots is that standalone is where they diverge.

### fabric-seams-become-engine-owned-interfaces

- **Decision:** `internal/websterengine` stops importing `internal/fabricengine`.
  Two seams replace it:
  - `RefMatcher interface { Matches(cmd string) bool }`, declared in `websterengine`.
    `CheckFork` and `CheckParent` take a `RefMatcher` instead of a `*fabricengine.RefScanner`;
    `RecordDeps` and `RunDeps` carry one.
  - `FabricBisector` (already a websterengine-declared interface) becomes **always caller-supplied**.
    `runIntegrationStage`'s `if bisector == nil { fabricengine.Open(deps.Layout) }` fallback is deleted;
    the CLI constructs the real `*fabricengine.Fabric` in hub mode and supplies `nil` in standalone.
- **Rationale:** `*fabricengine.RefScanner` needs nothing from webster but a compiled regexp over `WeftWorktree(l)`, and `fabricengine.Open` needs a `Location` webster will no longer hold.
  Injecting both is what actually removes the hub concept from the engine — the design doc's stated goal for this file set.
  It also removes the last production use of `deps.Layout`.
- **Rejected:**
  - keeping `Layout` in the Deps solely to feed fabric — leaves the engine hub-shaped for two call sites and defeats the migration;
  - passing a told `weftPath string` and constructing the `RefScanner` inside webster — still imports `fabricengine`, and hands webster a fabric concept it has no other use for.

### standalone-has-no-fabric

- **Decision:** standalone mode runs with no fabric at all.
  Concretely:
  - `fabricSync` (`internal/webstercli/sync.go`) is called only in hub mode;
    all four commit points skip the call entirely in standalone.
    Only `run` surfaces the result — `run.go:106`'s `fabricCommitted` output field, which reports `false` in standalone.
    The other three call sites (`beginbatch.go:136`, `recordbatch.go:135`, `recoverbatch.go:155,189`) already discard the bool and only surface a sync *error*, so in standalone they emit nothing new.
  - `RunDeps.Bisector` is `nil` in standalone.
    `runIntegrationStage` must then refuse to bisect and instead report the failing integration suite with an explicit warning naming standalone mode as the reason, rather than nil-panicking or silently skipping the escalation.
  - `RefMatcher` in standalone is a never-matching matcher (a `websterengine`-local zero value or a `standalonegeom`-supplied one) — there is no weft worktree and no fabric verb to police.
- **Rationale:** every one of these is fabric-repo machinery, and standalone has no fabric repo by construction.
  Building a plain `gitrepo.Repo` over `--target-dir` as the Bisector was considered and rejected: `FabricBisector`'s methods `CheckoutDetached`/`ResetHard` **mutate a working tree**, and doing that to a directory the operator merely told webster to work in violates the design's "the target directory receives only what the caller explicitly named" property.
- **Rejected:** refusing `run` in standalone whenever the plan declares an integration suite — that would make standalone webster useless for most real plans, when degrading to "suite failed, here is the report, bisect unavailable" is honest and still actionable.

### mode-selection-and-the-extracted-wiring-function

- **Decision:** mode is decided from **two `internal/preflight` predicate results**, and the decision itself lives **inside** the extracted function, not upstream of it.

  `PersistentPreRunE` keeps only three things: the bare-`webster` guard, `lyxcwd.CwdFrom`, and calls to `preflight.Wired(cwd)` and `preflight.HubPresent(cwd)`.
  It passes both results — `(wiredLoc *lyxcwd.Location, wired bool)` and `(hubLoc *lyxcwd.Location, hubPresent bool)` — plus the parsed flag values into a package-private function on `*websterCLI`, which computes the mode and builds the whole engine stack.
  The function performs no cwd resolution and spawns no process.

  The truth table it computes:

  | `preflight.Wired` | `preflight.HubPresent` | Mode |
  |---|---|---|
  | true | (either) | **hub** |
  | false | true | **hard error** naming `lyx fabric reconcile` — a wired-but-broken hub is refused, never degraded |
  | false | false | **standalone** — covers the plain git repo *and* the unresolvable cwd alike |

- **Rationale:** two corrections to the naive reading of T8's brief, both forced by the current source.
  - `preflight.Wired` returns `(nil, false)` on **both** its failure branches (`internal/preflight/predicates.go:30-40`), so there is no `*lyxcwd.Location` in hand at the point the discriminator runs — `fabricengine.BoardDir(filepath.Dir(worktreeRoot))` has no `worktreeRoot` to work from.
    Worse, webster's hub `worktreeRoot` is `AnchorPath()`, so `filepath.Dir` of it is not `HubPath` in a subpath-anchored hub, and the hand-derived arithmetic would be wrong even where a value existed.
    `preflight.HubPresent` already answers exactly this question — `lyxcwd.Resolve` plus one `os.Stat` of `fabricengine.BoardDir(l.HubPath)/_lyx` — and T5 built it for this purpose.
  - The mode decision must sit inside the extracted function, because Testing requires the untagged unit test over that function to cover all three rows of the table above.
    A decision made upstream in `PersistentPreRunE` would be unreachable from a tier-1 test, since driving the real pre-run reaches `lyxcwd.Resolve` and its `gitexec.Run` spawn, breaching the Test Tier Purity Invariant.

  The cost of calling both predicates is one extra `lyxcwd.Resolve` (one `git rev-parse`) on the not-wired path only — `cmd/lyx/stencilseed.go:74-76` already documents this exact trade-off and accepts it.
- **Rejected:** triggering on "`lyxcwd.Resolve` errored" — `Resolve` succeeds in any ordinary git repository run from its root, so that trigger would put a downloaded repo into hub mode with a fictional `HubPath`.
- **Rejected:** hand-deriving the board path from a worktree root, per T8's literal wording — no `Location` is available there, and the arithmetic is wrong under a nested `AnchorRel`.

### the-two-roots

- **Decision:** webster carries two told roots, and they are the same value in hub mode and different in standalone:

  | Told value | Hub mode | Standalone |
  |---|---|---|
  | `AnchorRoot` | `l.AnchorPath()` | `<state>` from `standalonestate.Derive(target)` |
  | `WorktreeRoot` | `l.AnchorPath()` (unchanged from today) | the absolute `--target-dir`, defaulting to cwd |
  | `WebsterDir` / `ReportsDir` | `websterengine.Dir/ReportsDir(l.AnchorPath())` | same accessors over `<state>` |
  | `ScratchDir` / `PromptsDir` | `websterengine.ScratchDir/PromptsDir(l.AnchorPath())` | same accessors over `<state>` |
  | `PlanDir` | `planparser.PlanDir(l.AnchorPath())` | `planparser.PlanDir(<state>)`, overridable by `--plan-dir` |
  | `StencilsDir` | `fabricengine.StencilsDir(l.HubPath)`, overridable by `--stencils-dir` | `<state>/_lyx/stencils`, overridable by `--stencils-dir` |
  | config `baseDir` (shuttle, reed, webster, batcher, modelspec) | `l.AnchorPath()` | `<state>` |
  | reed `Geometry.PaneCwd` (pane spawn cwd) | `l.AnchorPath()` — today's value at both spawn sites | the absolute `--target-dir` |
  | `{{.worktree_root}}` in the fork/recovery/integration prompts | `AnchorRoot` (fork, recovery) / `WorktreeRoot` (integration) — both `l.AnchorPath()` today | `WorktreeRoot` |

  Hub mode must keep passing `l.AnchorPath()` for `WorktreeRoot`, exactly as the four CLI call sites do today — this task changes no resolved path in a real worktree.
- **Rationale:** `WorktreeRoot` is what `headSHA`/`dirty` (`gitwrap.go`) and the integration verify command run against — the directory actually being edited.
  `AnchorRoot` is what every `_lyx`/`.lyx` join and every config read hangs off.
  Splitting them is what keeps a standalone run from writing hidden state into the directory it was pointed at.
- **Rejected:** pointing both at the target in standalone — that pushes a `.lyx/webster` tree, `state.json`, `run.lock` and the rendered prompts into the operator's working directory.

### the-three-flags

- **Decision:** three new persistent flags on the `webster` parent command.

  - `--stencils-dir <path>`: optional in both modes, honoured in both, **read-only** in both.
    Hub default `fabricengine.StencilsDir(l.HubPath)`;
    standalone default `<state>/_lyx/stencils`.
  - `--target-dir <path>`: standalone only, defaults to cwd, resolved to absolute at the CLI boundary.
    **Refused in hub mode** with an explicit error saying the value is structurally the worktree — because it decides where the run *writes*, and honouring it in a hub would strand artifacts outside fabric's positive-only commit pathspec.
  - `--plan-dir <path>`: optional in both modes, read-only in both.
    Hub default `planparser.PlanDir(l.AnchorPath())` (identical to today);
    standalone default `planparser.PlanDir(<state>)`.
- **Rationale:** the asymmetry between `--stencils-dir` (honoured in hub) and `--target-dir` (refused in hub) is deliberate and is exactly T8's ruling: a read-only override is useful and harmless, a write-base override is neither.
- **Rejected:** making `--target-dir` a no-op in hub mode — silent acceptance of a flag that does nothing is worse than a refusal.

### plan-dir-absent-in-standalone-is-a-usage-error

- **Decision:** in standalone, a `--plan-dir` (explicit or defaulted) that does not exist or contains no plan files is a plain usage error naming the `--plan-dir` flag.
  There is no bootstrap and no empty-plan fallback.
  In hub mode the behaviour is unchanged from today.
- **Rationale:** a plan is authored content, not a shipped default;
  seeding an empty plan directory would turn a missing-plan mistake into a silent zero-batch run.
- **Rejected:** treating an absent plan directory as a zero-batch plan — `run` already has a zero-batch pre-flight refusal, and routing a missing directory into it would report the wrong cause.

### stencils-bootstrap-only-on-the-standalone-default

- **Decision:** the standalone default `<state>/_lyx/stencils` is seeded on first use via `stencilstore.Reconcile(dir, stencils.Registry(), stencilstore.ModeFor(buildinfo.IsDev()), "")`.
  An explicitly-passed `--stencils-dir` is never reconciled, in either mode.
  The fourth argument stays `""` — "no source tree here" — which keeps the port-back drift warning silent.
  `cmd/lyx/stencilseed.go` and `cmd/lyx/main.go` are not touched;
  T5 already gates the root pre-run's seed on `preflight.HubPresent`.
- **Rationale:** nothing else would ever create the standalone default, and nothing should ever rewrite a directory an operator curated and named explicitly.
  Reusing `buildinfo.IsDev()` rather than hardcoding `ModeProduction` keeps a dev binary's dev-seeding semantics identical standalone and in-hub (Dev/Prod Binary Separation).
- **Rejected:** reconciling any `--stencils-dir` on first use — it would silently overwrite an experimental stencil set.

### new-package-internal-standalonegeom

- **Decision:** add `internal/standalonegeom`, the told-mode sibling of `internal/hubgeom`.
  It exports the standalone `reedengine.Geometry` builder — pinned exactly as the design's T8 table specifies — and the standalone `websterengine.Geometry` builder beside it.

  Reed's standalone values, from `standalonestate.Derive(target) -> (stateDir, hash8)`:

  | Field | Standalone value |
  |---|---|
  | `SocketKey` | `lyx-<hash8>` |
  | `SessionName` | `<basename of target>-<hash8>` |
  | `AnchorPath` | `<state>` |
  | `WorktreeRoot` | the absolute target directory |
  | `LogsDir` | `<state>/logs` — told directly, **not** `fabricengine.HubLogsDir(<state>)` |
  | `RepoName` | the target directory's basename |
  | `HubPath` | `<state>` |
  | `PaneCwd` | the absolute target directory — **new field, see the next Decision** |

- **Rationale:** webster needs this builder **for its own standalone entry**, not merely to save T8 work later.
  `internal/webstercli/cli.go:181-182` constructs `reedengine.New(reedCfg, hubgeom.ReedGeometry(layout))` and hands the result to `shuttleengine.NewRunner`;
  webster spawns Master and every fork through that runner, so a standalone webster that cannot build a reed geometry cannot spawn anything at all.
  The standalone entry T7's brief commits to delivering is therefore blocked on exactly this builder.
  T8 reuse is the secondary reason and is what settles *where* it lives rather than *whether* it exists: T8 needs the identical builder for `burlercli`/`perchcli` a wave later, so inlining it in `webstercli` would guarantee either a duplicate or a later lift, and the design's own "no additive twins" decision says duplication reliably outlives the migration.
  A named package also gives the pinned table a single home a test can assert against.
- **Rejected:** inlining in `webstercli` and letting T8 lift it — the lift is the duplication risk, and it costs T8 a refactor it would otherwise not need.

### standalone-pane-cwd-is-told-separately-from-anchorpath

- **Decision:** add a `PaneCwd string` field to `reedengine.Geometry` and use it at the two tmux spawn sites that currently pass `e.geom.AnchorPath` — `internal/reedengine/lifecycle.go:294` (`new-session -c`) and `:489` (`split-window -c`).
  `stateDir` (`lifecycle.go:33`) keeps using `AnchorPath` unchanged.
  - **Hub mode:** `hubgeom.ReedGeometry` sets `PaneCwd = l.AnchorPath()` — byte-identical to today's behaviour at both spawn sites, in anchored and subpath-anchored hubs alike.
  - **Standalone:** `standalonegeom` sets `PaneCwd` to the absolute target directory while `AnchorPath` stays `<state>`.
- **Rationale:** `reedengine.Geometry.AnchorPath` serves two unrelated purposes that only coincide inside a hub — it is both the base `stateDir` joins `.lyx` onto (`reed.json`, `reed.lock`) and the cwd every pane is spawned with.
  Standalone is exactly where they must diverge: the pinned `AnchorPath = <state>` is correct for reed's state, but it would start Master and every fork with their working directory in `<state>` — a directory that is not a git repository and contains no source.
  The implementer stencil instructs the agent to build, test and **commit** from `{{.worktree_root}}` (`webster-body-implementer.md:28,31`), so a pane rooted in `<state>` puts the agent's shell somewhere its own instructions never point, and any command run without an explicit path — `git status` above all — hits the state directory.
  The same `AnchorPath` is also the audit workdir (`recordbatch.go:102`), which must observe the directory the fork actually edited.
  Splitting the field is additive and hub-neutral: `hubgeom` is the only production constructor of `reedengine.Geometry` today, and it sets `PaneCwd` to the value the spawn sites already used.
- **Rejected:** leaving `AnchorPath` to do both jobs and relying on the prompt to `cd` the agent to `worktree_root` — that is precisely the class of fiction this design rejects the synthetic `Location` for: the geometry would assert one working directory while the prompt asserts another, with nothing reconciling them and no compiler or test catching the divergence.
- **Rejected:** pointing standalone `AnchorPath` at the target instead — that relocates `reed.json`/`reed.lock` into `<target>/.lyx`, writing hidden state into the directory webster was only told to work in, which is the property the two-roots split exists to guarantee.
- **Rejected:** switching the spawn sites to the existing `Geometry.WorktreeRoot` field instead of adding one — `WorktreeRoot` is `l.WorktreePath()` in hub mode (`hubgeom.ReedGeometry`), so that would change pane cwd in every subpath-anchored hub, for burler, perch and treadle as well as webster.
- **Note for T8:** `burlercli`/`perchcli` inherit this field for free through `standalonegeom`;
  their forks have the identical need.

### batcher-moves-to-the-degrading-side

- **Decision:** `batcher.Active` switches from `configengine.Load` to `configengine.LoadOrTemplate` (`internal/batcher/config.go:35`), and `CONSTRAINTS.md`'s Config Strictness Invariant pinned sets move `batcher` from strict to degrading in the same commit.
  The `strings.Contains(err.Error(), "not initialized")` rewrap becomes unreachable for the absent-`_lyx` case and is removed or narrowed accordingly.
- **Rationale:** the Config Strictness Invariant already records this as "a watch item for T7/T10": `batcher` is strict only because it had no standalone entry, and a standalone webster reaches `batcher.Active` on every verb.
  Leaving it strict makes standalone webster hard-fail on a config the invariant's own membership rule says should degrade.
- **Rejected:** having `webstercli` catch the strict error and substitute a default batchifier — that puts config-degradation policy in a CLI and contradicts the invariant's membership rule.

### constraints-rewords-land-here-not-in-t8

- **Decision:** the `CONSTRAINTS.md` edits the design doc assigns to T8 land in **this** task's commit, worded generally rather than for burler/perch:
  - **Stencil Ownership Invariant**, read-location bullet: reworded to name a told absolute directory, with `<hub>/_board/_lyx/stencils/` as what hub mode resolves to.
  - **Stencil Ownership Invariant**, seed-pass bullet: "once per process: at `cmd/lyx`'s root pre-run in hub mode, or at the producer CLI's own pre-run in standalone mode."
    The load-bearing half — *never lazily inside `stencilstore.Read`* — is preserved verbatim.
  - **Durable-vs-Ephemeral State Invariant**: add that standalone's `_lyx`/`.lyx` pair are ordinary siblings under `<state>`, satisfying the mirrored-subpath rule rather than deviating from it.
  - **Config Strictness Invariant**: the `batcher` pinned-set move above.
  - **CLI / Cobra Invariant**: no text change, but the three new flags carry the `Short`/`Long` and help-accuracy obligations it imposes.
- **Rationale:** T7 is the first task to ship a told stencils directory and a `<state>` tree, so from this commit onward the shipped code contradicts the current wording.
  CLAUDE.md's same-commit docs rule and the design's own reasoning ("deferring them would leave the shipped code contradicting a live invariant across two waves") both apply here, and apply to T7 first because T7 lands first.
  T8 then confirms the wording rather than writing it.
- **Rejected:** deferring to T8 as the design doc literally says — the doc assumed T8 would be first to need it, which the wave order contradicts.

### no-pre-run-git-gate-on-the-standalone-target

- **Decision:** standalone does not check whether `--target-dir` is a git repository.
  The failure surfaces at the verb that needs a SHA (`begin-batch`'s `headSHA`, `record-batch`'s cross-check, `run`), as the existing `websterengine: head sha in <dir>: ...` error.
- **Rationale:** T7's required integration test asserts the pre-run reaches the run verb's **own flag validation** rather than a resolution error, driving `RunCLIIn` from a temp dir outside any git repo.
  A pre-run git gate would fail that test by design.
- **Rejected:** an explicit pre-run refusal with a friendlier message — it contradicts the pinned integration test and would also block `validate` and `status`, neither of which needs git.

### constructoranchoring-rows-rewritten-in-place

- **Decision:** `cmd/lyx/constructoranchoring_test.go`'s four webster rows (`websterengine.Dir`, `ReportsDir` in the `_lyx` group;
  `PromptsDir`, `ScratchDir` in the `.lyx` group, in both the unanchored and subpath-anchored test functions) are rewritten in place to pass `l.AnchorPath()`.
  The file is not split or retired.
  A textual merge conflict with T6's adjacent `perchengine.RunsDir`/`ScratchDir` rows is expected;
  whichever of T6/T7 merges second rebases.
- **Rationale:** the design's explicit disposition for this file.
  Note the caveat the file itself already records: rows that pass `l.AnchorPath()` in and compare against an anchor-derived expectation become tautological with respect to anchoring.
  The non-tautological proof for webster stays in `internal/webstercli/verbs_test.go`'s subpath-anchored `PersistentPreRunE` case, which this task must keep passing.
- **Rejected:** deleting the webster rows as tautological — they still pin the join arithmetic and the `_lyx`-vs-`.lyx` group placement.

## Technical context

**Current `*lyxcwd.Location` surface in `internal/websterengine`** (production files only):

- `state.go:41,48,56,64` — `Dir`, `ReportsDir`, `ScratchDir`, `PromptsDir`.
- `render.go:139,167,230` — `RenderForkPrompt`, `RenderRecoveryPrompt`, `RenderMasterPrompt`;
  `fabricengine.StencilsDir(l.HubPath)` at `152`, `173`, `236`;
  `l.AnchorPath()` fills `worktree_root` at `149`, `183`, and feeds `pattern.Directive` at `174`, `237`.
- `runlevel.go:109` — `RunDeps.Layout`;
  used at `470` (`fabricengine.StencilsDir(deps.Layout.HubPath)`), `486` (`RenderMasterPrompt`), `530` (`shuttleengine.FindRun(..., deps.Layout.AnchorPath(), ...)`), `721`/`724`/`728` (`fabricengine.NewRefScanner` + audit workdir), `832` (`fabricengine.Open(deps.Layout)`).
- `beginbatch.go:78` — `BeginDeps.Layout`, used at `198` (`RenderForkPrompt`).
- `recordbatch.go:51` — `RecordDeps.Layout`, used at `102` (`AuditForksIncremental` workdir), `126` (`NewRefScanner`), `129`/`133` (audit workdir).
- `recoverbatch.go:61` — `RecoverDeps.Layout`, used at `156` (`RenderRecoveryPrompt`), `182` (`shuttleengine.FindRun`).

**`RenderIntegrationPrompt` (`render.go:201`) is already the told shape** — `(plan, reportPath, worktreeRoot, stencilsDir string)`.
It is the model the other three converge on.

**`internal/webstercli` call sites that pass `layout`:** `run.go:62,71,85`, `beginbatch.go:100,101,136`, `recordbatch.go:104,105,135`, `recoverbatch.go:125,126,155,189`, `validate.go:73`, `sync.go:22`, and `cli.go:139-197`.
Every one passes `layout.AnchorPath()` for `WorktreeRoot` — so hub-mode `WorktreeRoot` is `AnchorPath`, not `WorktreePath`.
Preserve that.

**`internal/hubgeom`** currently exports `ReedGeometry` only.
Its `doc.go` already names `WebsterGeometry` as T7's addition and states that standalone CLIs do not call `hubgeom` — that sentence stays true and is why `standalonegeom` is a separate package.

**`reedengine.Geometry`'s two consumers of `AnchorPath`** (`internal/reedengine/lifecycle.go`): `stateDir` at `:33` (`Join(AnchorPath, ".lyx")`), and the pane spawn cwd at `:294` (`new-session -c`) and `:489` (`split-window -c`).
`Geometry.WorktreeRoot` is consumed only by `strand.go:175-176` (strand name resolution and the `Strand.Worktree` stamp).
`shuttleengine.Spec.validate` already resolves relative file entries against its own told `worktreeRoot` (`spec.go:115,132`).

**The `worktree_root` token's consumers** are `contracts/stencils/webster/webster-body-implementer.md:28,31` (build, unit tests, and the card commit) and `webster-template-integration.md:21,29` (the verify command).
No other stencil references it, and `RenderMasterPrompt` never fills it.

**T5 foundations, already landed:**

- `preflight.Wired(cwd) (*lyxcwd.Location, bool)` — `lyxcwd.Resolve` + `fabricengine.Ready`, no extra spawn.
- `preflight.HubPresent(cwd) (*lyxcwd.Location, bool)` — the stencil-seed gate;
  **not** the mode trigger.
- `standalonestate.Derive(target string) (stateDir, hash8 string, err error)` — requires an absolute target;
  normalizes symlinks and Windows case before hashing.
- `buildinfo.IsDev() bool` — exact `"dev"` match on the ldflags-stamped `Channel`.

**Guards this task runs into:**

- `cmd/lyx/rawgitmutation_test.go` machine-checks that `internal/websterengine` non-test sources contain neither `gitrepo.New(` nor `gitexec.Run(` outside its allowlist.
  `gitwrap.go` is the grandfathered file;
  nothing in this refactor should add a new one.
- `internal/buildinfo` and `internal/standalonestate` each carry a `leaf_enforcement_test.go` with an empty import allowlist — `standalonegeom` must not be added to either, and must not be made a leaf itself (it imports `reedengine` and `websterengine`).
- `cmd/lyx/seamsignature_test.go` pins `RunCLI`/`RunCLIIn` shapes;
  neither changes.
- `cmd/lyx/tierpurity_test.go` — the extracted wiring function must spawn no process, so its unit test stays tier 1.

## Constraints

From `CONSTRAINTS.md`:

- **Cwd Resolution Invariant** — `internal/lyxcwd` stays the sole owner of cwd resolution.
  `standalonestate.Derive` requires an absolute target precisely so the relative-to-absolute step happens at the CLI argument boundary, never inside a leaf.
  Each module keeps owning its own relative subpath: `websterengine.Dir` and friends stay the sole declarers of `_lyx/webster`.
- **Durable-vs-Ephemeral State Invariant** — `_lyx` and `.lyx` stay directory siblings under the anchor root, and every never-tracked file keeps its mirrored subpath.
  Standalone's pair under `<state>` satisfies this;
  say so in the invariant's own text.
- **Stencil Ownership Invariant** — stencils are read at call time from a directory, never from embedded bytes;
  `internal/stencilstore` stays the sole owner of seeding, stamping and edit detection;
  a hash-mismatched file is never overwritten.
  Reworded by this task for the told directory and the standalone pre-run.
- **Config Strictness Invariant** — a module belongs on the degrading side once it has a standalone entry point.
  `batcher` moves;
  the pinned sets are updated in the same commit.
- **CLI / Cobra Invariant** — `Command()`/`RunCLI`/`RunCLIIn` seam unchanged;
  non-empty `Short` on every command;
  `Long` with concrete examples where self-discoverable;
  JSON envelopes via `internal/output` on every error path;
  **help accuracy is a review obligation** and three new flags are new observable behaviour.
- **Test Tier Purity Invariant** — the mode-truth-table test must be tier 1, which is only possible via the wiring extraction.
- **Fabric Git Invariant (warp + weft)** and its `rawgitmutation_test.go` guard — untouched;
  no new raw git handle in `websterengine`.
- **Dev/Prod Binary Separation** — the standalone stencil seed reuses `buildinfo.IsDev()`, never a hardcoded mode.
- **Documentation Lifecycle** — `internal/websterengine/doc.go`, `internal/webstercli/cli.go`'s package doc, `internal/hubgeom/doc.go` (which currently promises `WebsterGeometry`), `internal/reedengine/geometry.go`'s field docs (the new `PaneCwd` needs its own comment explaining why it is not `AnchorPath`), and the new `internal/standalonegeom/doc.go` all land in this commit, plus `CONSTRAINTS.md`.
  `manifest/roadmap.md` is **not** moved by this task — wave 3 completes only when T6 also lands, and the roadmap move belongs to whichever task closes the wave, or to T10.

From `CLAUDE.md`:

- Markdown uses semantic line breaks — one sentence per line, no fixed-column wrapping — in every `.md` file this task touches.
- Docs land in the same commit as the code that makes them true.

## Testing

**`internal/websterengine`** — the existing suites are the regression net;
the migration must keep them green with only mechanical construction changes.

- **Seven test files hold `*lyxcwd.Location` fixtures and all need conversion** — inventory them before starting, since only two are obvious:
  `webstergeom_test.go`, `template_test.go` (which owns `testLayout`, `patternActiveLayout` and `patternActiveMissingPatternStencilsLayout` at `:184,198,818` and is the main consumer of the render functions' `Location` parameter), `beginbatch_test.go`, `recordbatch_test.go`, `recoverbatch_test.go`, `runlevel_test.go`, `audit_test.go`.
- `webstergeom_test.go` is rewritten: the four accessors now take `anchorRoot string`, so the fixture builds `l.AnchorPath()` once and passes it in.
  Keep both the unanchored and subpath-anchored cases — they still pin the `_lyx`/`.lyx` split.
- New: `CheckFork`/`CheckParent` against a fake `RefMatcher`, proving the audit's fabric-reference violation fires on a matcher hit and stays silent on a never-matching one.
  This is a genuine TDD candidate — write the interface's test before deleting the `*fabricengine.RefScanner` parameter.
- New: `runIntegrationStage` with a `nil` Bisector must report the failing suite with a standalone-mode warning and must not panic.
  TDD candidate — the nil-fallback deletion is what makes nil reachable.
- `render.go`'s existing tests move from a `Location` fixture to told `anchorRoot`/`stencilsDir` strings.
  Two assertions, not one — this is a TDD candidate, since the standalone case is the one the first draft of this discussion got wrong:
  **hub** (`anchorRoot == worktreeRoot`) renders `worktree_root` as that value, and **standalone** (`anchorRoot != worktreeRoot`) renders it as `worktreeRoot`, never `anchorRoot`.
  A test asserting only the hub case passes under both the correct and the broken implementation.

**`internal/hubgeom`** — a table test that `WebsterGeometry(l)` returns exactly the values the four accessors plus `planparser.PlanDir` and `fabricengine.StencilsDir` produce for the same `l`, at both `AnchorRel == "."` and a nested `AnchorRel`.

**`internal/standalonegeom`** — pure path-math table tests pinning every row of both tables in the two-roots and `standalonegeom` decisions above, for a fixed target and a stubbed `<state>`/`hash8`.
`LogsDir` must be asserted as `<state>/logs` explicitly, since the wrong answer (`<state>/_board/.lyx/logs`) is one `HubLogsDir` call away.
`PaneCwd` must be asserted as the target and `AnchorPath` as `<state>` **in the same test**, since the whole point of the field is that the two differ here.

**`internal/reedengine`** — assert `hubgeom.ReedGeometry(l).PaneCwd == l.AnchorPath()` at both `AnchorRel == "."` and a nested `AnchorRel`, and that the two spawn sites pass `PaneCwd` rather than `AnchorPath`, so the hub-neutrality claim is pinned rather than asserted in prose.
The existing reed suite must stay green unchanged — that is the actual regression proof for burler, perch and treadle, which this task does not otherwise touch.

**`internal/webstercli`** — the two tiers T7's Verify line requires:

- **Untagged unit test** over the extracted wiring function, driving it with told `(wired, hubPresent)` predicate results and covering the mode-selection truth table explicitly:
  `Wired == true` ⇒ hub;
  `(false, HubPresent == false)` ⇒ standalone — this row covers both the downloaded plain git repo and the unresolvable cwd, which `preflight.Wired`/`HubPresent` make indistinguishable by returning `(nil, false)` for each;
  `(false, HubPresent == true)` ⇒ hard error naming `lyx fabric reconcile`.
  Plus the plan-dir cases: hub default, standalone default, explicit override, and missing-in-standalone ⇒ usage error naming `--plan-dir`.
  Plus `--target-dir` refused in hub mode.
  Plus the geometry the function builds in standalone: `PaneCwd` and `worktree_root` resolve to the target while every `_lyx`/`.lyx` path and every config `baseDir` resolves under `<state>`.
  The function must perform no cwd resolution and no spawn, so this stays tier 1 — which is only possible because the mode decision lives inside it.
- **`//go:build integration` test** driving `RunCLIIn(<temp dir outside any git repo>, …)` and asserting the pre-run reaches the run verb's own flag validation rather than a resolution error.
  `internal/perchcli/cli_integration_test.go` is the shape to follow;
  `internal/webstercli` already has a hermetic `TestMain` and an existing `sync_integration_test.go`.
- `verbs_test.go`'s subpath-anchored `PersistentPreRunE` case must keep passing unchanged — it is the non-tautological anchoring proof for webster.
- `cli_test.go`/`smoke_test.go` grow the three new flags' `Short` text assertions if that is the existing shape there.

**`internal/batcher`** — a test that `Active` returns the template-selected batchifier when `_lyx/` is absent, mirroring the per-loader tests T2 added.

**`cmd/lyx`** — `constructoranchoring_test.go` rows rewritten;
`helptree_test.go`, `drift_test.go`, `longlist_test.go` cover the new flags' `Short`/`Long` obligations.

**Verify commands:**

- `go test ./internal/websterengine/... ./internal/webstercli/... ./internal/hubgeom/... ./internal/standalonegeom/... ./internal/reedengine/... ./internal/batcher/... ./cmd/lyx/...`
- `go test -tags integration ./internal/webstercli/...`
- `go test ./...` as the task-wide baseline.
- One manual acceptance run: `lyx webster run --plan-dir p/ --stencils-dir s/` from a scratch directory outside any git repository, confirming it reaches the run verb rather than a resolution error, and confirming nothing was written into that directory.
  While Master's pane is live, also confirm the pane's own working directory is the target and not `<state>` — that is the one property no unit test observes end-to-end, and it is what the `PaneCwd` split exists to deliver.

## Q&A log

- **Q:** How does `websterengine` receive its fabric-bound seams (`NewRefScanner` ×2, `Open` ×1)? **A:** [auto-pick] websterengine-owned narrow interfaces (`RefMatcher`, always-injected `FabricBisector`). **Why:** it is what actually removes `internal/fabricengine` from the engine, which the task brief names as the goal;
  the alternatives leave the engine hub-shaped for two call sites.
- **Q:** What does standalone do about fabric — the four commit points and the bisect escalation? **A:** [auto-pick] no fabric at all: `fabricSync` hub-only, `nil` Bisector with a loud degraded-escalation warning, never-matching `RefMatcher`. **Why:** a `gitrepo.Repo` Bisector over `--target-dir` would `CheckoutDetached`/`ResetHard` a directory webster was only told to work in, breaking the design's "only what the caller explicitly named" property.
- **Q:** `{{.worktree_root}}` is filled from `AnchorPath` in three renderers and from `WorktreeRoot` in the fourth — converge? **A:** [auto-pick] preserve today's values exactly;
  record the mismatch in the doc comment. **Why:** this is a signature migration;
  converging would silently change prompt text in subpath-anchored hubs with no test looking for it.
- **Q:** What shape does the geometry hand-off take? **A:** [auto-pick] a `websterengine.Geometry` struct mirroring `reedengine.Geometry`, built by `hubgeom.WebsterGeometry` and by `standalonegeom`. **Why:** `hubgeom` needs a type to return, and one struct keeps the four Deps structs from drifting on the same six values.
- **Q:** Where does the standalone reed geometry live, given T8 needs the identical builder a wave later? **A:** [auto-pick] a new `internal/standalonegeom`, hubgeom's told-mode sibling. **Why:** the design's own "no additive twins" reasoning — inlining it in `webstercli` guarantees either a duplicate or a lift.
- **Q:** What triggers hub mode? **A:** [auto-pick] `preflight.Wired`, with a `_board`-present discriminator turning "not wired" into a hard error rather than standalone. **Why:** `Resolve` succeeds in any ordinary git repo, so it is not a usable trigger;
  and standalone must not become where broken hubs hide.
- **Q:** Is `--target-dir` honoured in hub mode? **A:** [auto-pick] refused with an explicit error. **Why:** it decides where the run writes, and honouring it in a hub strands artifacts outside fabric's commit pathspec — deliberately the opposite ruling from read-only `--stencils-dir`.
- **Q:** What happens when the standalone plan directory is missing or empty? **A:** [auto-pick] a plain usage error naming `--plan-dir`. **Why:** a plan is authored content with no shipped default;
  routing it into the zero-batch refusal would report the wrong cause.
- **Q:** Which stencils directories get bootstrapped? **A:** [auto-pick] the standalone default only, never an explicit `--stencils-dir`, in either mode. **Why:** an operator who names a curated stencil set must never have it reconciled out from under them.
- **Q:** Should the pre-run refuse a standalone `--target-dir` that is not a git repository? **A:** [auto-pick] no gate;
  the git error surfaces at the verb that needs a SHA. **Why:** T7's required integration test asserts the pre-run reaches the run verb's own flag validation in a non-repo temp dir, and a gate would also block `validate` and `status`, which need no git.
- **Q:** `batcher` is on the Config Strictness Invariant's strict side but standalone webster calls `batcher.Active` — what gives? **A:** [auto-pick] migrate `Active` to `LoadOrTemplate` and move the pinned set in the same commit. **Why:** the invariant already records this as a T7 watch item, and its own membership rule puts `batcher` on the degrading side once a standalone caller exists.
- **Q:** The design doc assigns the Stencil Ownership and Durable-vs-Ephemeral rewords to T8, which lands later — where do they go? **A:** [auto-pick] here, worded generally;
  T8 then confirms rather than writes. **Why:** T7 is the first task to ship a told stencils directory and a `<state>` tree, so from this commit the current wording is false — and CLAUDE.md requires the doc in the same commit.
- **Q:** [review r1 gap] In standalone, `anchorRoot` is `<state>` — so does `{{.worktree_root}}` really keep being filled from it? **A:** [recommended] No: the preserve-today's-value ruling binds hub mode only;
  standalone fills the token from `WorktreeRoot`, pinned by its own test. **Why:** the token is the directory the implementer builds, tests and **commits** in (`webster-body-implementer.md:28,31`) and the integration fork verifies in;
  `<state>` is not a git repository, so the original ruling would have pointed every agent at the hidden state directory and broken the commit step outright.
- **Q:** [review r1 gap] `reedengine` spawns every pane with `-c geom.AnchorPath`, which standalone pins to `<state>` — how does a fork reach the directory it is meant to edit? **A:** [recommended] Add `Geometry.PaneCwd`, consumed at `lifecycle.go:294,489`;
  hub sets it to `AnchorPath` (behaviour-identical), standalone to the target, while `stateDir` keeps using `AnchorPath`. **Why:** `AnchorPath` conflates "where reed's state lives" with "where panes start", which only coincide inside a hub;
  relying on the prompt to `cd` the agent instead would make the geometry assert one working directory while the prompt asserts another — the same fiction this design rejects the synthetic `Location` for.
- **Q:** [review r1 gap] The `_board` discriminator needs a `worktreeRoot`, but `preflight.Wired` returns `(nil, false)` on both failure branches — where does it come from? **A:** [recommended] It does not;
  use `preflight.HubPresent(cwd)` as the discriminator, and move the mode decision inside the extracted wiring function. **Why:** T5 built `HubPresent` to answer exactly this question, and hand-derived hub arithmetic would be wrong anyway under a nested `AnchorRel` since webster's hub `worktreeRoot` is `AnchorPath()`;
  the decision has to sit inside the extracted function for the truth-table test to reach it at tier 1.
- **Q:** What happens to `cmd/lyx/constructoranchoring_test.go`? **A:** [auto-pick] webster's four rows rewritten in place in both test functions;
  accept the expected textual conflict with T6's adjacent rows. **Why:** the design's explicit disposition for the file, and the non-tautological anchoring proof lives in `verbs_test.go` regardless.
