# Discussion: webster standalone mode: run refuses to start Master; logs write untracked into target repo

```yaml
task: 'webster standalone mode: run refuses to start Master; logs write untracked into target repo'
slug: standalonegeom-webster-run-and-log-hygiene
status: discussing
parent: crucible-loom-glyph-hardening
```

## Problem

`lyx` has two modes.
In **hub mode** the operator stands inside a wired lyx fabric and every path — durable `_lyx`, ephemeral `.lyx`, run directories, reed state — hangs off one resolved anchor inside the worktree.
In **standalone mode** the operator points `lyx` at a plain git checkout with `--target-dir` (or just runs it from inside one), and lyx keeps all of its own state out of that repository, under a derived directory beneath `$XDG_STATE_HOME/lyx/<hash8>` (`internal/standalonestate.Derive`).
Standalone mode's whole promise is: *lyx never litters the repository it is working on*.

Two shipped defects break that promise from opposite directions.
They were found live by the loom glyph-hardening crucible campaign (round 1, `opus-high-r1`, findings F16 and F22) while it was driving webster's bracket verbs against a real fixture repository, and were recorded rather than fixed there because loom always runs hub mode and never constructs `standalonegeom` at all.

**F16 — standalone `lyx webster run` cannot start Master, at all.**
`standalonegeom.WebsterGeometry`/`ReedGeometry` correctly put the anchor at the derived state directory and the worktree root at the target repository, so those two paths are by construction unrelated.
`shuttleengine.NewRunner` then refuses exactly that pair: its `validateToldPaths` asserts the anchor is the worktree root or a subdirectory of it, as a **swap detector** for two adjacent same-typed string parameters.
The observed failure, verbatim from the crucible run:

```
lyx webster run --plan-dir <plan>          # cwd = a plain git checkout, standalone mode
{"error":"webster: start master: shuttle: NewRunner was told an anchor path
 \"/home/knatte/.local/state/lyx/267a789e\" outside its worktree root
 \"<repo>\": the anchor is always the worktree root or a subdirectory of it,
 so this pair is most likely swapped — …","ok":false}
```

`begin-batch`, `record-batch`, `validate` and `status` all work standalone — only `run`, the Master spawn, does not.
That is the mode's documented front door: `lyx webster --help` gives `lyx webster run --target-dir /path/to/repo` as its own worked example (`internal/webstercli/cli.go:186-187`).

**F22 — standalone mode writes trace logs into the target repository, untracked.**
`internal/logger`'s durable trace sink resolves its own directory lazily by calling `lyxcwd.Resolve(cwd)` and joining `.lyx/logs` onto the resolved anchor (`internal/logger/sink.go:91-99`).
That resolution is entirely mode-blind: a plain git checkout resolves fine, so every standalone `lyx` invocation drops `<target>/.lyx/logs/trace-*.log` into the operator's repository.
In hub mode those files are held out of git by `fabricengine`'s `.git/info/exclude` seeding; standalone mode has no fabric and seeds no exclude, so they simply show up as untracked — and `RecordBatch`'s dirty-worktree probe then reports "worktree is dirty after batch N's own commits" on **every** call because of them.
The crucible run observed this throughout, and a plain `git add -A` in the fixture committed eleven trace files.

**Why now:** both are confirmed-live, shipped defects on lyx's advertised no-hub entry point, and F16 is what forced the crucible campaign to drive webster's bracket verbs by hand instead of through `run`.
Any future standalone verification — the campaign's own "what could not be verified" list names this first — stays blocked until `run` works.

## Scope

**In:**

- `internal/shuttleengine`: a second, explicit constructor for the legitimately-detached anchor/worktree pair, with its own validation rule, plus a told pane-cwd carried on `Runner` and used by the fork audit.
- `internal/webstercli/wiring.go` `wireStandalone`: construct the runner through the new detached constructor, and redirect the durable trace sink to the standalone state directory.
- `internal/burlercli/wiring.go` `wireStandalone`: the same two changes — the defect is structurally identical there (`internal/burlercli/wiring.go:159` passes the same non-containing pair).
- `internal/standalonegeom`: a new pure path helper for the standalone logs directory, the sole construction site for it, mirroring the existing `StencilsDir` helper.
- `internal/logger`: promote the durable-sink directory override from a test-only helper to a documented production API; no behavioural change to its body.
- `CONSTRAINTS.md`: record the detached-anchor construction rule as an invariant clause.
- Tests for every one of the above, plus an extension of `internal/webstercli/cli_integration_test.go` asserting a standalone invocation leaves the target repository clean.

**Out:**

- **Hub mode behaviour, in any respect.**
  Every change is either standalone-only at the call site or a strict no-op for the values hub geometry produces.
  `NewRunner` itself keeps its current signature, its current containment assertion, and its current semantics — no hub caller is touched.
- **Relaxing, weakening, or deleting the existing containment assertion.**
  It stays exactly as it is for `NewRunner`.
- **The other twenty crucible findings (F1–F15, F17–F21).**
  Those are glyph-surface defects owned by the crucible campaign and its own worktrees;
  this task fixes F16 and F22 only.
- **Trace-log leakage from standalone-capable CLIs that derive no state directory** — today `lyx quarry` and parts of `lyx loom` resolve a non-hub mode (`preflight.ResolveMode`) without ever calling `standalonestate.Derive`, so they still write `<repo>/.lyx/logs`.
  Fixing that means either giving those CLIs a derived state directory or moving mode resolution into the root pre-run, both of which are a materially larger design change than this task.
  Recorded here explicitly so it is not mistaken for done;
  it deserves its own task.
- **Seeding `.git/info/exclude` in the target repository.**
  Rejected on the merits, see the F22 decision below.
- **`manifest/roadmap.md`.**
  This is a bugfix, not a planned-item completion.

## Decisions

### detached-runner-constructor

- Decision: add a second constructor to `internal/shuttleengine` — a detached-anchor variant alongside `NewRunner` — that accepts an anchor path which is deliberately outside the worktree root, and switch both standalone CLI wirings to it.
  `NewRunner` is untouched: same signature, same containment assertion, same error text.
- Rationale: the containment clause is a *swap detector*, not a geometric preference — `run.go:75-78` says so in as many words.
  It exists because `anchorPath` and `worktreeRoot` are adjacent parameters of the same type with four semantically distinct consumers, so a caller that swaps them compiles cleanly and silently reads the wrong tree.
  Standalone mode is not a swap;
  it is a genuinely detached pair, and the only honest way to express that is for the caller to *say so at the call site*, in the constructor it picks.
  A distinct constructor makes the claim greppable, gives it a doc comment that records the tradeoff, and keeps every one of `NewRunner`'s four callers (`burlercli`, `webstercli`, `shuttlecli`, `loomcli` hub paths, plus `treadleengine`'s smoke judge) protected exactly as they are today.
- Rejected:
  **(a) Relax `validateToldPaths`' containment clause unconditionally** — this deletes the swap detector for every caller in the tree to serve one mode, which is precisely the "silent inline patch" the crucible finding warned against.
  **(b) Put the knob in `shuttleengine.Config`** — `Config` is loaded from `shuttle.yaml`, so this would make a structural fact about how the process was wired into an operator-settable option, against the Config Strictness Invariant's spirit.
  **(c) Move standalone's anchor inside the target repository** so containment holds — this contradicts `standalonegeom`'s explicit design (`burlergeom.go:14-19`: pointing the anchor at target "is exactly what would push a hidden `.lyx` tree into the reviewed folder"), violates the Durable-vs-Ephemeral State Invariant's standalone clause, and makes F22 strictly worse.
  **(d) Pass the state directory as `worktreeRoot` too** — `worktreeRoot` resolves relative `OutputFiles` entries, as the `run` verb's own help promises, and is the `{{.worktree_root}}` token's value;
  pointing it at the state directory would silently resolve an implementer's output files outside the repository they are working in.

### detached-pair-validation

- Decision: the detached constructor keeps the non-empty and absolute-path checks verbatim, and replaces the containment clause with a **strict disjointness** requirement: the two paths must not be equal, and neither may contain the other.
- Rationale: a detached constructor that validated nothing would be a hole rather than a seam.
  Disjointness is the strongest structural claim available without importing `standalonestate` into `shuttleengine` (which the Told-Geometry Invariant's bound-package list and the leaf invariants both argue against).
  It catches the realistic misuse — hub geometry handed to the detached constructor by mistake — in both its shapes: a root-anchored hub gives `AnchorPath == WorktreePath` (rejected by the equality clause), and a subpath-anchored hub gives an anchor strictly inside the worktree (rejected by the containment clause).
  What it cannot catch is a pure swap of two already-disjoint directories;
  that residual risk is bounded by there being exactly two call sites, both pinned by wiring tests.
  Say this out loud in the constructor's doc comment rather than implying the check is stronger than it is.
- Rejected:
  **(a) No validation beyond absolute/non-empty** — loses the hub-geometry-by-mistake catch for free.
  **(b) Require the anchor to live under the OS state directory** — drags `standalonestate` (and `$XDG_STATE_HOME` reads) into an engine that is told its paths and derives none, breaking the Told-Geometry Invariant.

### fork-audit-pane-cwd

- Decision: carry a told `paneCwd` on `Runner` and use it as the fork audit's workdir at `internal/shuttleengine/wait.go:431`, in place of `r.anchorPath`.
  `NewRunner` sets `paneCwd = anchorPath`, preserving today's hub behaviour byte-for-byte;
  the detached constructor takes it as an explicit parameter, and both standalone wirings pass `reedGeom.PaneCwd` (the target repository).
- Rationale: without this, F16's fix produces a `run` that *starts* and then classifies every fork wrongly, which is not a fix.
  The fork audit derives the provider's transcript directory from the **pane's own process cwd**, and reed is separately told that cwd as `Geometry.PaneCwd`.
  In hub mode `hubgeom.ReedGeometry` sets `PaneCwd = l.AnchorPath()`, which is the same value `NewRunner` is told as `anchorPath` — which is why passing `anchorPath` has been correct so far, and why `run.go:66-71` describes `anchorPath` as "the pane's own process cwd".
  In standalone `ReedGeometry` sets `PaneCwd = target` while the anchor is the state directory, so the two diverge and the existing code would look for transcripts under a directory no pane ever ran in.
  Note this is a latent hub-mode fragility too: `hubgeom.ReedGeometry` sets `WorktreeRoot = l.WorktreePath()` and `PaneCwd = l.AnchorPath()`, so in a subpath-anchored hub *neither* of the runner's two current fields is unconditionally the pane cwd — making the third told value the honest representation rather than an extra parameter for standalone's convenience.
  `internal/websterengine/recordbatch.go:124` already gets this right from the other side by passing `deps.Geom.WorktreeRoot`, which equals the pane cwd in **both** modes for webster's geometry;
  the inconsistency is `shuttleengine`'s alone.
- Rejected:
  **(a) Pass `worktreeRoot` as the audit workdir** — correct in standalone, but wrong in a subpath-anchored hub, where `WorktreePath()` is not where panes start.
  **(b) Record it as a follow-up task** — it would ship an F16 fix that does not make `run` work end to end, which is worse than not shipping it.

### standalone-scope-includes-burler

- Decision: fix `internal/burlercli/wiring.go`'s standalone branch in the same task, alongside `webstercli`.
- Rationale: `burlercli/wiring.go:159` passes exactly the same non-containing pair from exactly the same `standalonegeom.ReedGeometry` call, so `lyx burler` standalone is broken in the same way for the same reason — the crucible campaign simply never exercised it.
  The fix at that call site is the same one-line constructor swap plus the same sink redirect.
  Leaving it would file a second task for two identical lines and would leave a known-broken shipped path in the tree.
- Rejected: webster-only, deferring burler — pure duplicated overhead for no isolation benefit;
  both call sites are already being read and tested in this task.

### log-sink-redirect-not-gitignore

- Decision: fix F22 by **redirecting** the durable trace sink to the standalone state directory, at `<stateDir>/.lyx/logs`.
  Nothing is written into the target repository at all.
- Rationale: the Durable-vs-Ephemeral State Invariant already names the answer — never-tracked files are siblings under the anchor, "hub: `BoardDir(hub)`; standalone: `standalonestate.Derive`".
  Today's behaviour is a straight violation of it, not merely an untidiness.
  Redirecting also fixes the *consequence* the crucible actually tripped over — `RecordBatch`'s dirty-worktree probe firing spuriously on every call — whereas an exclude entry only hides the files from `git status` while still writing into the operator's repository, and only for a repository whose `.git/info/exclude` lyx has taken it upon itself to mutate.
  `<stateDir>/.lyx/logs` is the shape that mirrors hub mode's `<anchor>/.lyx/logs`, with `stateDir` playing the anchor's role — the same substitution `standalonegeom.StencilsDir` already makes for `<stateDir>/_lyx/stencils`.
- Rejected:
  **(a) Seed `.git/info/exclude` in the target repository** — writes lyx state into a repository lyx does not own, and mutates that repository's git configuration as a side effect of a read-shaped command.
  **(b) Do both** — the exclude entry is dead weight once nothing is written there, and would leave a stale line behind in every repository ever touched.
  **(c) Disable the durable sink entirely in standalone mode** — throws away the trace log that made F16 and F22 diagnosable in the first place.

### logs-dir-single-declarer

- Decision: add `standalonegeom.LogsDir(stateDir) string` returning `<stateDir>/.lyx/logs`, built from `lyxdirs.DotLyxDirName`, as the sole construction site for the standalone logs directory — directly mirroring the existing `standalonegeom.StencilsDir(stateDir)`.
- Rationale: two call sites (`webstercli`, `burlercli`) need the same path, and the Lyxdirs Single-Declarer Invariant forbids naming `.lyx` in path-construction context outside `lyxdirs`.
  A pure function taking only `stateDir` keeps `standalonegeom` hermetic exactly as its package doc promises: no `Derive` call, no environment read, no disk touch.
  It is a path helper, not a geometry struct, so it fits beside `StencilsDir` rather than inside any `Geometry`.
- Rejected:
  **(a) Have `standalonegeom` call `logger.SetDurableSinkDir` itself** — that is a process-global side effect from a package documented as touching nothing;
  the *path* belongs here, the *effect* belongs at the CLI wiring boundary.
  **(b) Inline `filepath.Join(stateDir, ".lyx", "logs")` at both call sites** — violates the Lyxdirs Single-Declarer Invariant and duplicates the shape.

### logger-production-sink-api

- Decision: promote `logger.SetDurableSinkDir` to a documented production API by rewriting its doc comment;
  keep the name and the body unchanged.
- Rationale: the function's body is already exactly what production needs — set the override, reset the `sync.Once` and all derived sink state — and the only thing marking it test-only is the words "for testing" in its doc comment.
  Renaming would churn the existing `cmd/lyx/main_test.go` call sites for no gain.
  The new comment must state both uses and, critically, must state the **ordering obligation**: the override only takes effect if it is set before the first record that arms the sink.
- Rejected:
  **(a) Add a separate production wrapper delegating to the same body** — two names for one behaviour, with the test-only name still present and still misleading.
  **(b) Make `ensureDurableSink` mode-aware by calling `preflight`/`standalonestate` itself** — `internal/logger` would then derive its own geometry, against the Cwd Resolution and Told-Geometry Invariants, and would pull a heavy dependency into the lowest-level package in the tree.

### redirect-call-site-and-ordering

- Decision: call the sink redirect inside each `wireStandalone`, immediately after `standalonestate.Derive` returns and **before** any other work in that function that can log.
- Rationale: `wireStandalone` is the only place `stateDir` exists, and module `PersistentPreRunE` hooks run **after** root's (`cobra.EnableTraverseRunHooks = true`, `cmd/lyx/main.go:78`), so the redirect lands after root pre-run and before any module work.
  Root pre-run's own logging risk is `seedStencils`, whose only log lines sit past `stencilSeedTarget`, which returns `ok == false` in standalone (no hub) — so it emits nothing and never arms the sink first.
  That is a real dependency and the plan must pin it with a test rather than leave it as a comment, because an added log line in root pre-run would silently re-break F22 by arming the sink at the target repository before the override is set.
- Rejected:
  **(a) Redirect in the root pre-run** — root pre-run does not resolve mode and does not derive a state directory;
  giving it both would move `standalonestate.Derive`'s call site and change every command's startup path, which is the larger design change this task explicitly scopes out.
  **(b) Redirect lazily on first use** — reintroduces exactly the ordering hazard the eager call removes.

### documentation-surface

- Decision: record the detached-anchor rule as a clause in `CONSTRAINTS.md` (extending the shuttle seam's entry rather than opening a new top-level invariant), and update the package/function doc comments that assert the old behaviour: `shuttleengine/doc.go`, `shuttleengine/run.go`'s `validateToldPaths` and `Runner` comments, `standalonegeom/doc.go`'s contract sentence, and `logger`'s sink comments.
  No `manifest/designs/` file changes and no `docs/overview.md` change.
- Rationale: `manifest/designs/` has no webster, shuttle, or standalone-mode document to update — the closest, `reed-fabric-standalone-api.md`, is about reed's fabric API rather than this geometry.
  `docs/overview.md`'s module table and execution stack are unchanged: no module is added, removed, or re-layered.
  Per `CLAUDE.md`, the roadmap moves only on completing or adding a planned item, and this is a bugfix.
  The doc comments matter more than usual here because several of them currently make claims the fix falsifies — notably `run.go`'s "the anchor is always the worktree root or a subdirectory of it" and its description of `anchorPath` as the pane's own process cwd.
- Rejected: authoring a new `manifest/designs/standalone-mode.md` — real value, but it is a documentation task for the whole mode, not something to smuggle into a two-defect bugfix.

## Technical context

**The refusal itself.**
`internal/shuttleengine/run.go:79-91` is `validateToldPaths`.
`NewRunner` (`run.go:51-61`) stays total and stores the verdict as `Runner.toldErr` (`run.go:34-37`), which every public entry point returns — so the failure surfaces at `run` time as `webster: start master: shuttle: …`, not at construction.
The containment clause is `run.go:86-89`.
`run.go:63-78` is the doc comment explaining *why* the clause exists;
read it before changing anything, and update it in the same commit.

**The four consumers of `anchorPath`**, per that same comment: the run-directory root (`runDirRoot(r.cfg, r.anchorPath)`, `run.go:160` and `run.go:290`), reed's state lookup for the orphan sweep (`run.go:273`), the fork audit's workdir (`wait.go:431`), and the pane's process cwd.
The first two genuinely want the state directory in standalone, which is what makes the detached pair correct rather than merely tolerated.
The third is the one that must move to the new told `paneCwd`.

**Geometry, both modes.**
Hub: `internal/hubgeom/hubgeom.go:19-29` (`ReedGeometry`: `AnchorPath = PaneCwd = l.AnchorPath()`, `WorktreeRoot = l.WorktreePath()`) and `internal/hubgeom/webstergeom.go:17-22` (`WebsterGeometry.WorktreeRoot = l.AnchorPath()`, deliberately not `WorktreePath()`).
Standalone: `internal/standalonegeom/reedgeom.go:19-33` (`AnchorPath = stateDir`, `PaneCwd = WorktreeRoot = target`), `webstergeom.go:31-42` (`AnchorRoot = stateDir`, `WorktreeRoot = target`), `burlergeom.go:22-27`, `stencilsdir.go:25-27`.
`standalonegeom/doc.go` states the package's hermeticity contract and its explicit non-leaf status;
the new `LogsDir` helper must not disturb either, and `doc.go`'s "contract today is …" sentence enumerates the exported surface, so it needs the new name added.

**The two call sites to change.**
`internal/webstercli/wiring.go:141-215` (`wireStandalone`;
`Derive` at `:147`, geometry at `:152-153`, `NewRunner` at `:200`) and `internal/burlercli/wiring.go:122-165` (`Derive` at `:129`, geometry at `:156`, `NewRunner` at `:159`).
Both packages already import `standalonegeom` and `standalonestate`.
These are the **only** two `standalonestate.Derive` call sites in the tree — both files' doc comments assert that, so both assertions stay true.

**The log sink.**
`internal/logger/sink.go:33-35` is `LogsDir(l *lyxcwd.Location)`, the hub-mode path.
`sink.go:71-135` is `ensureDurableSink`: it reads `sinkDirOverride` first, and only falls back to `lyxcwd.Getwd()` + `lyxcwd.Resolve()` + `LogsDir(layout)` when the override is empty — which is the whole mechanism the fix uses.
Note it also sets `header.WorktreeRoot = layout.WorktreePath()` on the fallback path only, so a redirected sink's header currently records no worktree root;
the plan should decide whether the standalone redirect also supplies that value (the target repository is the honest answer) or leaves it empty, and pin whichever it picks.
`sink.go:196-208` is `SetDurableSinkDir`.
The sink is armed lazily on the first Info+ record, `sync.Once`-guarded, and the whole trace machinery is suppressed under `testing.Testing()` unless `LYX_TRACE=1` (`sink.go:79-84`) — which is exactly how a test drives it, and how `cmd/lyx/main_test.go:81-83` already does.

**Pre-run ordering.**
`cmd/lyx/main.go:64-88` builds the root;
`PersistentPreRunE` sets verbosity, mints/exports the trace ID and calls `logger.Arm()` (header only — this does **not** arm the sink), then calls `seedStencils(cmd)`.
`cobra.EnableTraverseRunHooks = true` (`main.go:96`) is what makes module hooks run after root's.
`cmd/lyx/stencilseed.go:31-57`: `seedStencils` returns early under `testing.Testing()`, early on the skip annotation, and early when `stencilSeedTarget` reports `ok == false` — which is the standalone case, since no hub exists.
Its log calls are at `stencilseed.go:122`, `:131` and `:134`, all past that gate.

**Existing tests to extend rather than duplicate.**
`internal/shuttleengine/run_test.go:39-105`: `TestNewRunner_RefusesUnusableToldPaths` (the table for empty/relative/swapped) and `TestNewRunner_AcceptsHubGeometryShapes` (every pair `hubgeom.ReedGeometry` can produce).
`internal/shuttleengine/run_inject_test.go:18-29` documents the fixture convention: anchor and worktree must be distinct paths so a swapped argument pair fails, while still satisfying the relation the validator asserts.
`internal/webstercli/wiring_test.go` already drives both wiring branches with a told `preflight.Mode` and redirects `XDG_STATE_HOME` via `t.Setenv` (`:157-166` for the `hash8For` helper).
`internal/webstercli/cli_integration_test.go` is the standalone end-to-end test, already redirecting `XDG_STATE_HOME` (`:27`, `:43-45`) and already asserting emptiness against a derived state directory (`:115`).
`cmd/lyx/main_test.go:81-99` is the pattern for driving the durable sink in a test.

**Gotchas.**

- `Runner.toldErr` means a bad pair produces no error until a verb runs.
  A wiring test that only checks `NewRunner` returned non-nil proves nothing;
  it must reach a public entry point, or the plan must expose the verdict deliberately.
- The Shuttle Provider-Seam Invariant: `shuttleengine` must not reference Claude specifics and must not import `claudeengine`.
  The fork-audit workdir change stays on the `shuttleengine` side of the `Engine` interface (`engine.go:123-130`);
  the Claude-side derivation in `claudeengine/audit.go` is not touched.
- Build prerequisite: `CGO_ENABLED=1` and a C compiler, because `lyx` links quarry's tree-sitter grammars.
- Both `wireStandalone` functions have long doc comments enumerating what they do, in order.
  Both need the new steps folded in;
  `burlercli`'s comment additionally has a "two asymmetries worth calling out" paragraph that a reader will otherwise try to simplify away.

## Constraints

From `CONSTRAINTS.md`, in the order they bite:

- **Told-Geometry Invariant.** An engine is handed absolute paths and derives none;
  no direct `internal/lyxcwd` import.
  `shuttleengine` is on the bound-package list.
  This is the reason the detached validator cannot ask "is this anchor under the OS state directory", and the reason `logger` must be *told* its directory rather than resolving mode itself.
  It also names `hubgeom`/`standalonegeom` as the only `Geometry`-struct constructors — the new `LogsDir` returns a plain string, not a struct, so it does not disturb that.
- **Durable-vs-Ephemeral State Invariant.** Every never-tracked file lives under `.lyx`, at the mirrored subpath of its `_lyx` content, as a sibling under the anchor — "hub: `BoardDir(hub)`; standalone: `standalonestate.Derive`".
  Today's standalone trace-log location violates this;
  `<stateDir>/.lyx/logs` satisfies it.
- **Lyxdirs Single-Declarer Invariant.** `internal/lyxdirs` is the sole declarer of `_lyx` and `.lyx`;
  no other production file names either literal in path-construction context.
  The new `LogsDir` helper must build its path from `lyxdirs.DotLyxDirName`.
- **Cwd Resolution Invariant.** `internal/lyxcwd` owns cwd resolution alone;
  raw `os.Getwd`/`git rev-parse --show-toplevel` are banned elsewhere.
  Nothing in this task may add a new resolution site.
- **Shuttle Provider-Seam Invariant.** Provider specifics live only under `internal/shuttleengine/claudeengine`;
  `shuttleengine` never imports it.
- **Test Tier Purity Invariant.** The wiring functions are deliberately tier-1-drivable — they perform no cwd resolution and spawn no process, which is exactly why `wire` takes a told `preflight.Mode`.
  Keep it that way: the redirect call must not introduce a resolution or a spawn.
- **Config Strictness Invariant.** Relevant as a rejected-alternative rationale — the detached-anchor decision is structural, not an operator-settable config key.
- **CLI / Cobra Invariant.** No command is added or renamed, so the help-tree tests should be unaffected;
  if any help text changes, the help-tree fixtures move in the same commit.
- **Standalonestate Leaf Invariant.** `standalonestate` stays a leaf — nothing in this task may add an import to it.

Project-level, from `CLAUDE.md`:

- Docs land in the same commit as the change.
- Markdown uses semantic line breaks — one sentence per line, no fixed-column hard wrap.
- `manifest/roadmap.md` does not move for a bugfix.

## Testing

Tier 1 throughout except where noted;
existing tables are extended rather than duplicated.

**`internal/shuttleengine`**

- Detached constructor, accepted shapes: the standalone pair (two disjoint absolute directories) constructs a runner whose told-path verdict is clean, observed through a public entry point rather than by reading the struct.
- Detached constructor, refused shapes — one table, mirroring `TestNewRunner_RefusesUnusableToldPaths`: empty either side, relative either side, the two paths equal, the anchor strictly inside the worktree, the worktree strictly inside the anchor.
  The last three are the hub-geometry-by-mistake catch and are the point of the whole decision, so each gets its own row.
- `NewRunner` regression: the existing refusal table and `TestNewRunner_AcceptsHubGeometryShapes` must still pass **unchanged** — that is the load-bearing proof that hub mode did not move.
  Add a row asserting `NewRunner` still refuses the standalone pair, so the two constructors are provably not interchangeable.
- Fork-audit workdir: a fake `Engine` records the workdir it is handed by `AuditForks`, driven once through a `NewRunner`-built runner (expect the anchor path, today's behaviour, in both a root-anchored and a subpath-anchored hub shape) and once through a detached runner built with a distinct pane cwd (expect the pane cwd).
  **TDD candidate** — write it first;
  it is the assertion that would otherwise be quietly forgotten, and it is the difference between `run` starting and `run` working.

**`internal/standalonegeom`**

- `LogsDir(stateDir)` returns `<stateDir>/.lyx/logs`, is pure, and touches no disk — extend `standalonegeom_test.go` alongside the existing `StencilsDir` coverage.

**`internal/logger`**

- Setting the durable-sink directory before the first record puts the trace file there and none in the cwd-derived location.
- Setting it *after* the sink is already armed does **not** move the file — pin the ordering obligation as a test, not just a doc sentence, so the constraint the wiring depends on is executable.

**`internal/webstercli` and `internal/burlercli`**

- `wireStandalone` builds a runner that is usable — reach a public entry point so `toldErr` would surface, and assert no told-path error.
  **TDD candidate** in `webstercli`: this is F16's direct regression test, and it fails today.
- `wireStandalone` sets the durable-sink directory to `standalonegeom.LogsDir(stateDir)` for the derived state directory, with `XDG_STATE_HOME` redirected via `t.Setenv` exactly as the existing wiring tests do.
- `wireHub` is unchanged in both packages: it still constructs through the containment-checked constructor, and it does **not** touch the sink directory.
  Assert both, so a later refactor cannot quietly route hub mode through the detached path.

**`cmd/lyx`**

- The root pre-run emits no Info+ record in a standalone (non-hub) invocation, so nothing arms the sink before a module hook can redirect it.
  This is the executable form of the ordering dependency named in the `redirect-call-site-and-ordering` decision;
  without it, an added log line in root pre-run silently re-breaks F22.

**Integration (tier 2, `internal/webstercli/cli_integration_test.go`)**

- After a standalone invocation against a real temporary git repository, the target repository contains no `.lyx` directory and `git status --porcelain` reports it clean.
  This is F22's end-to-end regression test and the one that would have caught the eleven trace files the crucible run committed.
- Trace files land under the derived state directory instead.
  Drive the durable sink deliberately (`LYX_TRACE=1`, per `sink.go`'s testing gate) rather than relying on incidental logging, so the assertion cannot pass vacuously.

**Whole-tree**

- `go build ./...` and `go test ./...` with `CGO_ENABLED=1`.
  Pay attention to `cmd/lyx`'s guard tests — `constructoranchoring_test.go`, `notransients_test.go` and `helptree_test.go` all make assertions about anchored paths and the command tree, and `constructoranchoring_test.go:99` and `:155` both pin `logger.LogsDir(l)` for hub shapes, which must not move.

## Q&A log

- **Q:** How should F16 be fixed — relax `shuttleengine.NewRunner`'s containment assertion, or express standalone's detached pair some other way? **A:** [auto-pick] Add an explicit detached-anchor constructor alongside `NewRunner`, and switch the standalone wirings to it. **Why:** the containment clause is a swap detector every hub caller relies on;
  a distinct constructor makes the "this pair is legitimately detached" claim explicit, greppable, and local to two call sites, instead of deleting the detector tree-wide to serve one mode.
- **Q:** What should the detached constructor validate, given it cannot assert containment? **A:** [auto-pick] Non-empty and absolute as today, plus strict disjointness — not equal, neither containing the other. **Why:** it catches hub geometry handed to the detached constructor in both its shapes (root-anchored gives equality, subpath-anchored gives containment) without importing `standalonestate` into an engine that is told its paths and derives none.
- **Q:** Does this task fix `burlercli`'s standalone branch too, or webster only? **A:** [auto-pick] Both. **Why:** `burlercli/wiring.go:159` passes the identical non-containing pair from the identical `standalonegeom.ReedGeometry` call, so `lyx burler` standalone is broken the same way;
  the fix is the same two lines at a call site already being read and tested here.
- **Q:** `shuttleengine/wait.go:431` passes `r.anchorPath` as the fork audit's workdir, but standalone panes start in the target repository. Fix here or defer? **A:** [auto-pick] Fix here, by carrying a told `paneCwd` on `Runner` — `NewRunner` sets it to `anchorPath`, preserving hub behaviour exactly. **Why:** otherwise F16's fix ships a `run` that starts and then audits the wrong transcript directory, which is not a fix.
  Passing `worktreeRoot` instead was considered and rejected: `hubgeom.ReedGeometry` sets `WorktreeRoot = WorktreePath()` while panes start at `AnchorPath()`, so that would break subpath-anchored hubs.
- **Q:** Fix F22 by redirecting the trace sink out of the target repository, or by seeding a `.git/info/exclude` entry in it? **A:** [auto-pick] Redirect to `<stateDir>/.lyx/logs`;
  write nothing into the target repository. **Why:** the Durable-vs-Ephemeral State Invariant already names the derived state directory as standalone's anchor, so today's location is an invariant violation rather than untidiness;
  and only redirecting stops `RecordBatch`'s dirty-worktree probe from firing, which an exclude entry would merely hide.
- **Q:** `logger.SetDurableSinkDir` is documented "for testing" but its body is exactly what production needs. Promote it, wrap it, or avoid it? **A:** [auto-pick] Promote it — rewrite the doc comment, keep the name and body. **Why:** the defect is the comment, not the function;
  a second name for one behaviour would leave the misleading one in place.
  The new comment must state the ordering obligation, since the override only binds before the sink is armed.
- **Q:** Where does the redirect get called, given `standalonegeom` is documented as touching nothing? **A:** [auto-pick] In each `wireStandalone`, right after `standalonestate.Derive`, using a new pure `standalonegeom.LogsDir(stateDir)` path helper. **Why:** the path belongs at the single construction site (Lyxdirs Single-Declarer), the process-global effect belongs at the CLI wiring boundary, and module pre-run hooks run after root's so the ordering holds.
- **Q:** How far does the F22 fix reach — every standalone-capable CLI, or the two that derive a state directory? **A:** [auto-pick] The two that derive one (`webstercli`, `burlercli`);
  record the rest explicitly as out of scope. **Why:** `lyx quarry` and parts of `lyx loom` resolve a non-hub mode without ever calling `standalonestate.Derive`, so covering them means either giving them a state directory or moving mode resolution into the root pre-run — a materially larger design change that deserves its own task rather than being smuggled into a two-defect bugfix.
- **Q:** What documentation moves? **A:** [auto-pick] A `CONSTRAINTS.md` clause on the shuttle seam recording the detached-anchor rule, plus the package and function doc comments the fix falsifies;
  no `manifest/designs/` file and no roadmap entry. **Why:** no module is added or re-layered, so `docs/overview.md` is unchanged and the roadmap does not move for a bugfix;
  but `run.go`'s "the anchor is always the worktree root or a subdirectory of it" and its description of `anchorPath` as the pane's own process cwd both become false and must move in the same commit.
