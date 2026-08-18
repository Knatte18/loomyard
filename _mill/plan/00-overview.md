# Plan: the standalone CLI path

```yaml
task: "the standalone CLI path"
slug: "standalone-cli-entry"
approved: true
started: "20260818-131358"
parent: "standalone-producers"
root: ""
verify: go vet ./...
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches.
Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: preflight-resolvemode
    file: 01-preflight-resolvemode.md
    depends-on: []
    verify: go test ./internal/preflight/... && go test -tags integration ./internal/preflight/...
  - number: 2
    name: standalonegeom-builders
    file: 02-standalonegeom-builders.md
    depends-on: []
    verify: go test ./internal/standalonegeom/...
  - number: 3
    name: webstercli-repoint
    file: 03-webstercli-repoint.md
    depends-on: [1]
    verify: go test ./internal/webstercli/... && go test -tags integration ./internal/webstercli/...
  - number: 4
    name: burlercli-standalone-entry
    file: 04-burlercli-standalone-entry.md
    depends-on: [1, 2]
    verify: go test ./internal/burlercli/... && go test -tags integration ./internal/burlercli/...
  - number: 5
    name: perchcli-standalone-entry
    file: 05-perchcli-standalone-entry.md
    depends-on: [1, 2, 4]
    verify: go test ./internal/perchcli/... && go test -tags integration ./internal/perchcli/...
  - number: 6
    name: docs-and-cross-cutting-verification
    file: 06-docs-and-cross-cutting-verification.md
    depends-on: [3, 4, 5]
    verify: go test ./cmd/lyx/...
```

## Shared Decisions

### Decision: `preflight.ResolveMode` is the single mode trigger for all three CLIs

- **Decision:** hub-vs-standalone selection is answered by exactly one function, `preflight.ResolveMode(cwd) (*lyxcwd.Location, Mode, error)`, consumed by `burlercli`, `perchcli` and `webstercli` alike.
  `preflight.HubPresent` and `preflight.Wired` are left byte-identical and keep their existing consumers (`cmd/lyx`'s stencil-seed gate, composing orchestrators).
  A non-nil error from `ResolveMode` means **refuse** — surface it verbatim and abort the pre-run; it is never degraded to standalone.
- **Rationale:** the design's whole reason for widening scope onto `webstercli` is that three CLIs selecting modes by three rules is the divergence the decision exists to prevent.
  Refusal is a *resolution verdict*, not a wiring choice, so it stays upstream in `resolvePersistentPreRun` and `wire` never sees the refused case at all — which is what keeps `wire`'s truth table two rows wide.
- **Applies to:** all batches.

### Decision: the mode decision lives inside `wire`, and `wire` resolves nothing

- **Decision:** each CLI gains `internal/<mod>cli/wiring.go` with `func (c *<mod>CLI) wire(loc *lyxcwd.Location, mode preflight.Mode, cwd, stencilsDirFlag, targetDirFlag string) error`, delegating to `wireHub(loc, ...)` and `wireStandalone(cwd, ...)`.
  `wire` performs no cwd resolution, spawns no process, and reads no environment variable beyond what a config load already does.
  `wireStandalone` never receives or reads `loc`.
- **Rationale:** the Test Tier Purity Invariant bans a process spawn from an untagged test, and driving the real `PersistentPreRunE` reaches `lyxcwd.Resolve`'s `git rev-parse`.
  Passing `mode` as a parameter is precisely what lets an untagged test drive both arms.
  `wireStandalone` not taking `loc` is the structural guarantee that a standalone session can never read a fictional `Location`.
- **Applies to:** batches 3, 4, 5.

### Decision: the refuse case is pinned at the integration tier, not in `wiring_test.go`

- **Decision:** every CLI's untagged `wiring_test.go` covers a **two-row** truth table — `(loc non-nil, ModeHub)` and `(loc nil, ModeStandalone)`.
  The refusal is pinned instead by (a) `preflight`'s own integration table, which owns `ResolveMode`'s seven-row behaviour, and (b) one integration-tagged test per repointed CLI driving `RunCLIIn` from a subdirectory of a real wired hub worktree and asserting the gated `ErrCwdOutsideAnchor` message.
- **Rationale:** the discussion asks for the repoint to be "covered rather than assumed", and `webstercli`'s wiring test to gain "the refuse row".
  A literal refuse row inside `wiring_test.go` is unwritable: `wire` never receives the refused case (the error return aborts upstream in `resolvePersistentPreRun`), and manufacturing one would require driving the real pre-run, which spawns git and breaches Test Tier Purity — the exact invariant the `wire` extraction exists to satisfy.
  The coverage intent is honoured at the tier where the behaviour is actually reachable.
  A `(loc non-nil, ModeStandalone)` row is deliberately never written: no caller can produce it.
- **Applies to:** batches 1, 3, 4, 5.

### Decision: `standalonegeom.StencilsDir(stateDir)` is the sole construction site

- **Decision:** the `<state>/_lyx/stencils` literal is constructed in exactly one place, `standalonegeom.StencilsDir`, and carried by each CLI as a plain `stencilsDir string` local/receiver field — never as an engine geometry field.
  `standalonegeom.WebsterGeometry` is repointed at the same helper, replacing its inline `filepath.Join`.
- **Rationale:** neither `burlerengine.Geometry` (`{WorktreeRoot, AnchorPath}`) nor `perchengine.Geometry` (`{GateDir, AnchorPath}`) has a `StencilsDir` field, and both engines already accept the stencils directory as a told parameter (`burlerengine.New`'s fourth argument, `perchengine.Engine.Run`'s fourth argument), so a geometry field would be a competing home.
  One helper is what makes "standalone's `<state>` plays the hub's role" checkable rather than repeated by hand in three packages.
- **Applies to:** batches 2, 4, 5.

### Decision: `--stencils-dir` is read-only in both modes; `--target-dir` is standalone-only

- **Decision:** both are `PersistentFlags()` on the module's parent command, bound to receiver fields, empty string meaning "not passed".
  `--stencils-dir` is honoured in both modes and never written when explicitly passed; only the derived standalone default is `stencilstore.Reconcile`-seeded, and a `Reconcile` failure is a hard pre-run error.
  `--target-dir` is honoured in standalone only (defaulting to cwd) and is a hard error in hub mode.
- **Rationale:** `--stencils-dir` names a directory that is only ever read, so pointing a real worktree at an experimental stencil set is harmless and is the flag's most useful application.
  `--target-dir` decides where the round *writes*, so honouring it in hub mode would place artifacts outside Fabric's positive-only commit pathspec and silently strand them.
  Never seeding an explicitly-told directory is what makes the read-only characterisation literally true.
- **Applies to:** batches 3, 4, 5.

### Decision: hub mode stays byte-identical, with exactly four named deviations

- **Decision:** every hub-mode config base directory, geometry output, stencils directory, run/scratch anchoring, and fabric pathspec/commit message resolves exactly as it does on `main` today.
  The complete deviation list is: (1) a plain git repo with no `<hub>/_board/_lyx` beside it moves from fictional hub mode to standalone; (2) a cwd outside any git repository now succeeds standalone instead of aborting; (3) three additive envelope fields (`mode`, `stateDir`, `stencilsDir`) appear in both modes' success envelopes for **`lyx burler run` and `lyx perch run` only**; (4) `webstercli` in a subdirectory of a wired worktree refuses instead of starting a silent standalone session.
  A fifth deviation discovered during implementation is a bug in this plan, not a licence.

  Deviation (3) is scoped to two verbs, and the two exclusions are deliberate rather than oversights.
  `lyx webster run` does **not** gain the three fields: the discussion records that T7 shipped webster's standalone entry without them, that this is a gap in T7 rather than a precedent to copy, and that fixing it is out of scope here — so batch 3 stays the narrow mode-trigger repoint it declares itself to be, changing no verb, no flag and no envelope.
  `lyx perch pause` does not gain them either, because its success envelope already reports an absolute `pauseFile`, which names `<state>` by construction, and pause is not where an operator first meets standalone mode.
  Both exclusions are named here so the list stays exhaustive as claimed, which is what lets a reviewer treat any other observed output change as a regression without re-deriving the intent.
- **Rationale:** stated rather than smuggled, per the design.
  The enumeration is what lets a reviewer treat any other observed change as a regression without re-deriving the intent.
- **Applies to:** all batches.

### Decision: every test reaching `wireStandalone` redirects the state root

- **Decision:** any test — new or pre-existing — whose call path reaches `wireStandalone` must call both `t.Setenv("XDG_STATE_HOME", t.TempDir())` and `t.Setenv("LOCALAPPDATA", t.TempDir())` before the call, and must not be marked `t.Parallel()`.
  `internal/burlercli` additionally gains a `testmain_test.go` calling `gitkit.HermeticGitEnv()`.
- **Rationale:** `standalonestate.Derive` reads live `XDG_STATE_HOME`/`LOCALAPPDATA` via `os.Getenv` and `HOME` via `os.UserHomeDir()`, so an unredirected test seeds a stencils tree into the operator's real state directory.
  Both variables are set so both `Derive` branches land inside the test's own tree on every platform.
  Git-config isolation (`TestMain`) and state-root isolation (`t.Setenv`) are two separate halves and this task needs both: `burlercli`'s new integration test reaches a real `git rev-parse` through `ResolveMode` in a package that spawns git from no test today, and the Hermetic Git Test Environment guard is token-keyed so it will not catch a spawn reached indirectly through a CLI seam.
  `t.Setenv` panics under `t.Parallel()`.
- **Applies to:** batches 3, 4, 5.

### Decision: Go verify commands carry no `PYTHONPATH=` prefix

- **Decision:** every `verify:` in this plan is a native `go test` invocation with no `PYTHONPATH= ` prefix.
  Batches whose tests include an `//go:build integration` file chain a second, `-tags integration` invocation with ` && `.
- **Rationale:** the `PYTHONPATH= ` reset exists to stop a Python test subprocess inheriting the mill cache scripts dir; this is a Go repo with no Python test surface.
  The chained `-tags integration` form is required because Go's untagged build excludes tagged files entirely, so a single untagged invocation would silently not compile the integration tests this task adds.
- **Applies to:** all batches.

## All Files Touched

- `internal/burlercli/cli.go`
- `internal/burlercli/cli_integration_test.go`
- `internal/burlercli/cli_test.go`
- `internal/burlercli/run.go`
- `internal/burlercli/testmain_test.go`
- `internal/burlercli/wiring.go`
- `internal/burlercli/wiring_test.go`
- `internal/perchcli/cli.go`
- `internal/perchcli/cli_integration_test.go`
- `internal/perchcli/cli_test.go`
- `internal/perchcli/run.go`
- `internal/perchcli/run_test.go`
- `internal/perchcli/wiring.go`
- `internal/perchcli/wiring_test.go`
- `internal/preflight/doc.go`
- `internal/preflight/predicates.go`
- `internal/preflight/preflight_integration_test.go`
- `internal/standalonegeom/burlergeom.go`
- `internal/standalonegeom/doc.go`
- `internal/standalonegeom/perchgeom.go`
- `internal/standalonegeom/standalonegeom_test.go`
- `internal/standalonegeom/stencilsdir.go`
- `internal/standalonegeom/webstergeom.go`
- `internal/webstercli/cli.go`
- `internal/webstercli/cli_integration_test.go`
- `internal/webstercli/wiring.go`
- `internal/webstercli/wiring_test.go`
- `manifest/designs/producers-standalone.md`
