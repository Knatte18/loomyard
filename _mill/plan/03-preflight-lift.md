# Batch: preflight-lift

```yaml
task: "lift the orchestrator preflight out of loomengine, plus the shared standalone-CLI foundations"
batch: "preflight-lift"
number: 3
cards: 6
verify: go test ./internal/preflight/... ./internal/loomengine/... ./internal/lyxcwd/... && go test -tags integration ./internal/preflight/... ./internal/loomengine/...
depends-on: []
```

## Batch Scope

This batch is the lift itself: a new `internal/preflight` package holding the orchestrator-agnostic tier-1 and tier-2 checks plus the `CheckID`/`Failure`/`Report` result types, and `internal/loomengine` rewritten to compose it with its own loom-specific check 4.
It is one batch because the two halves are a single type-identity change — `loomengine` re-exposes `preflight`'s types as Go aliases, so splitting the packages across batches would leave the tree uncompilable at the boundary.
The external interface batch 4 consumes is `preflight.HubPresent(cwd)`; `preflight.Wired(cwd)` ships here as the hub-mode trigger T7 and T8 will consume, with no consumer in this task.
Batch-local decision: the two predicates are deliberately distinct and neither may be collapsed into the other — see `## Batch Scope` note below and the `internal/preflight/doc.go` requirement in card 12, which is the durable home for that reasoning.

The behaviour-preservation bar for this batch is exact: `internal/loomengine/preflight_integration_test.go`'s 13 test functions must compile and pass **unmodified**, and that file is not in any card's `Edits:` list.
If the implementer finds itself needing to edit that file, the type-alias decision was implemented as duplicate types and the card must be redone, not the test.

## Cards

### Card 9: create the shared result types in internal/preflight

- **Context:**
  - `internal/loomengine/report.go`
- **Edits:** none
- **Creates:**
  - `internal/preflight/report.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Declare `package preflight`. Move the `CheckID` type, the `Failure` struct, and the `Report` struct out of `internal/loomengine/report.go` verbatim in shape, together with exactly five of its nine check-ID constants: `CheckGeometry`, `CheckWorktreeClean`, `CheckFabricReady`, `CheckFabricSync`, `CheckJunction`.
  Keep every constant's string value byte-identical to what `internal/loomengine/report.go` declares today (`"geometry"`, `"worktree-clean"`, `"fabric-ready"`, `"fabric-sync"`, `"junction"`) — the values are observable through `Report` and any drift breaks the compatibility contract.
  Leave `CheckSeedMissing`, `CheckSeedUnreadable`, `CheckSeedIncoherent` and `CheckHalfFinished` behind; they are loom-specific and stay in `loomengine` (card 14).

  Replace today's unexported `func (r *Report) addFailure(check CheckID, reason string)` with an exported `func (r *Report) AddFailure(check CheckID, reason string)` of identical body — appending a `Failure` and setting `r.OK = false` — because `loomengine` now appends to the report from another package.
  Add a second exported method `func (r Report) Has(check CheckID) bool` reporting whether any entry in `r.Failures` carries that `CheckID`.

  Document on `Report` that the invariant `OK == (len(Failures) == 0)` always holds for a `Report` returned with a nil error, and document on `Has` that it is how a composing orchestrator derives its own control-flow signals from the shared report rather than from bespoke fields riding along on the type.
  Reword the constants' doc comments so they describe tier-1/tier-2 checks generically rather than naming loom's four-check numbering — a `Hardener` will read these.
  Keep the note that there is deliberately no at-the-anchor check, since `lyxcwd.Resolve`'s cwd gate already guarantees the property and a subpath-anchored repo is a legal geometry.
  Do not use the tokens `weft` or `warp` anywhere in this file.
- **Commit:** `feat(preflight): add shared CheckID/Failure/Report result types`

### Card 10: lift the tier-1 and tier-2 check bodies

- **Context:**
  - `internal/loomengine/preflight.go`
  - `internal/preflight/report.go`
  - `internal/fabricengine/ready.go`
  - `internal/fabricengine/drift.go`
  - `internal/fabricengine/warpclean.go`
  - `internal/fabricengine/worktreelist.go`
  - `internal/lyxcwd/lyxcwd.go`
  - `internal/lyxcwd/anchor.go`
- **Edits:** none
- **Creates:**
  - `internal/preflight/preflight.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Declare `package preflight` and export two functions.

  `func Check(cwd string) (Report, *lyxcwd.Location, error)` calls `lyxcwd.Resolve(cwd)` and then delegates to `CheckResolved`.
  On a `Resolve` error satisfying `errors.Is(err, lyxcwd.ErrNotAGitRepo)` it returns a determined verdict, not an error: a `Report` with `OK` false and a single `Failure{Check: CheckGeometry, Reason: "not inside a git repository"}`, a nil `*lyxcwd.Location`, and a nil error.
  On any other `Resolve` error it returns `(Report{}, nil, err)`.
  It returns the resolved `*lyxcwd.Location` on success so the caller never re-resolves: `lyxcwd.Resolve` spawns `git rev-parse --show-toplevel`, and forcing an orchestrator to resolve again to reach its own tier-3 paths would double the git spawns of every preflight.

  `func CheckResolved(l *lyxcwd.Location) (Report, error)` carries the bodies of today's checks 1b, 2 and 3 from `internal/loomengine/preflight.go`'s unexported `checkResolved`, in the same order and with the same collect-versus-short-circuit semantics:

  - Check 1b, geometry sanity: a `fabricengine.PrimeName(l)` error short-circuits with a `Report` carrying only `Failure{Check: CheckGeometry, Reason: "no main worktree resolved"}` and a nil error — never an escalated error, preserving the report-not-error contract.
  - Carry across the existing comment explaining why there is deliberately no at-the-anchor check here (`lyxcwd.Resolve`'s strict cwd gate already proves cwd equals `AnchorPath()` exactly, so a non-`"."` `AnchorRel` means the repo is subpath-anchored, and rejecting it failed every subpath-anchored hub unconditionally).
    This comment moves with the code; it must not be dropped.
  - Check 2, worktree pair cleanliness via `fabricengine.Clean(l)`: an error escalates as `(Report{}, err)`; a false result records `AddFailure(CheckWorktreeClean, reason)` and **collects** rather than short-circuiting, so a dirty side does not prevent the remaining checks from also reporting.
  - Check 3 via `fabricengine.Ready(l)` then `fabricengine.Healthy(l)`: an error from either escalates as `(Report{}, err)`; a false `Ready` records `AddFailure(CheckFabricReady, "fabric not ready")`; otherwise an unhealthy result classifies on the typed `reason.Cause`, with `fabricengine.CauseBranchMismatch` recording `CheckFabricSync` and every other cause recording `CheckJunction`, using `reason.Detail` as the failure reason.

  Set `report.OK = len(report.Failures) == 0` before returning, exactly as today.
  Do **not** carry across today's `check3BlocksSeed` local variable — that signal is loom's, and card 14 derives it from the returned report instead.

  Reword the comment on `Check`'s non-`ErrNotAGitRepo` branch rather than lifting it verbatim: today's wording in `internal/loomengine/preflight.go` claims that branch catches a case such as the git subprocess failing to spawn, and that is false.
  `internal/lyxcwd/lyxcwd.go`'s `gitWorktreeRoot` wraps every non-`*gitexec.GitError` failure as `fmt.Errorf("%w: %v", ErrNotAGitRepo, err)`, so an exec-level failure also satisfies `errors.Is(err, ErrNotAGitRepo)` and lands in the short-circuit branch.
  The residual branch is reachable only through `lyxcwd`'s anchor-resolution path, and the comment must say that instead.
  Do not enumerate an exhaustive two-sentinel list: `internal/lyxcwd/anchor.go` reaches that branch by at least three distinct routes — `lyxcwd.ErrCwdOutsideAnchor` from the cwd gate, `lyxcwd.ErrStaleAnchorMarker` from a board carrying only the pre-rename marker, and a `lyxcwd.ErrInvalidAnchor`-wrapping failure when a recorded anchor exists but fails validation — so name the anchor-resolution path generically and cite the sentinels as examples rather than as a closed set.
  A replacement comment that is itself incomplete would repeat the defect this reword exists to remove.

  Do not use the tokens `weft` or `warp` anywhere in this file: call `fabricengine.Ready(l)` and never name `WeftWorktree`, and describe check 2 as "worktree pair cleanliness" the way `internal/loomengine/preflight.go` already does.
- **Commit:** `feat(preflight): lift tier-1 and tier-2 checks out of loomengine`

### Card 11: add the two cheap predicates

- **Context:**
  - `internal/preflight/preflight.go`
  - `internal/fabricengine/ready.go`
  - `internal/fabricengine/junctionnames.go`
  - `internal/lyxdirs/dirs.go`
- **Edits:** none
- **Creates:**
  - `internal/preflight/predicates.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Declare `package preflight` and export two boolean predicates, each returning `(nil, false)` on any error rather than surfacing one, because both are consumed by a CLI pre-run that must never block a command.

  `func Wired(cwd string) (*lyxcwd.Location, bool)` — `lyxcwd.Resolve(cwd)` succeeding **and** `fabricengine.Ready(l)` returning true.
  This is the hub-mode trigger T7 and T8 consume to choose hub mode over standalone: is Fabric wired for this worktree.

  `func HubPresent(cwd string) (*lyxcwd.Location, bool)` — `lyxcwd.Resolve(cwd)` succeeding **and** a single `os.Stat` of `filepath.Join(fabricengine.BoardDir(l.HubPath), lyxdirs.LyxDirName)` succeeding.
  This is the stencil-seed gate: does the hub this write targets actually exist.
  Build that path from `fabricengine.BoardDir` and `lyxdirs.LyxDirName`; never spell either directory name as a literal, which the Lyxdirs Single-Declarer Invariant and the geometry-literal enforcement walk both forbid.

  Neither predicate may spawn a process beyond the one `lyxcwd.Resolve` already performs — no `fabricengine.Clean`, no `fabricengine.Healthy`, no `fabricengine.PrimeName`, since both run before every single `lyx` command.

  Document on each predicate what question it asks and that the two are not interchangeable: `fabricengine.Ready` probes the paired sibling of the current worktree rather than the hub, so it is false at `<hub>/_board`, false in an unpaired sibling, and false in a worktree whose pair was removed — all three of which are real, healthy hub situations that seed stencils correctly today.
  Do not use the tokens `weft` or `warp` anywhere in this file.
- **Commit:** `feat(preflight): add Wired and HubPresent predicates`

### Card 12: write the package doc

- **Context:**
  - `internal/hubgeom/doc.go`
  - `internal/preflight/preflight.go`
  - `internal/preflight/predicates.go`
  - `internal/preflight/report.go`
- **Edits:** none
- **Creates:**
  - `internal/preflight/doc.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Declare `package preflight` with the package-level doc comment, matching the vocabulary and tone of `internal/hubgeom/doc.go`.
  This file is the durable home for two pieces of reasoning that otherwise vanish when this task's worktree is merged, so it must state them in the code that ships:

  1. **The report-not-error contract**, restated in full — `(Report{OK:true}, nil)` for a clean pass, `(Report{OK:false, Failures}, nil)` for a determined negative verdict, `(Report{}, err)` only for "could not determine", and a `fabricengine.PrimeName` failure deliberately reported as a `CheckGeometry` failure rather than escalated.
     A composing orchestrator reads this doc rather than `loomengine`'s.
     Do **not** repeat the false claim that the non-`ErrNotAGitRepo` branch catches a git-subprocess spawn failure; say the residual branch is reachable only through `lyxcwd`'s anchor-resolution path, per card 10, and do not enumerate a closed sentinel list there either.
  2. **Why there are two predicates**, and which one the stencil seed gates on.
     State that `Wired` asks "is Fabric wired for this worktree" and is the hub-mode trigger for standalone-capable CLIs;
     that `HubPresent` asks "does the hub-level directory this write targets exist" and is what `cmd/lyx`'s stencil seed gates on;
     and that gating the seed on `Wired` is wrong because `fabricengine.Ready` probes the paired sibling of the current worktree, making it false at `<hub>/_board`, in an unpaired sibling, and in a worktree whose pair was removed — three real-hub situations that seed correctly today, so the narrowing would be a regression rather than a fix.
     State also why `HubPresent` is not merely a weaker `Wired`: a hub-level directory can exist while this particular worktree is not wired, and that resolved-but-not-wired case is exactly the one a standalone-capable CLI must answer with standalone mode.

  Also record that the package is deliberately read-only — it acquires no lock, writes nothing, and records no mutation — which is why it sits outside `internal/fabricengine` rather than becoming a composite verb on it.
  Do not use the tokens `weft` or `warp` anywhere in this file; note that the Fabric Vocabulary Invariant's `.md` walk does not reach `doc.go`, but its Go walk does.
- **Commit:** `docs(preflight): document the contract and the two predicates`

### Card 13: test the new package

- **Context:**
  - `internal/preflight/report.go`
  - `internal/preflight/preflight.go`
  - `internal/preflight/predicates.go`
  - `internal/loomengine/preflight_integration_test.go`
  - `internal/loomengine/testmain_test.go`
  - `internal/hubforge/hub.go`
- **Edits:** none
- **Creates:**
  - `internal/preflight/report_test.go`
  - `internal/preflight/testmain_test.go`
  - `internal/preflight/preflight_integration_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  `internal/preflight/report_test.go` is untagged and in-package (`package preflight`), spawning nothing: cover `Report.AddFailure` appending and flipping `OK` to false, `Report.Has` returning true for a recorded `CheckID` and false for an unrecorded one including on a zero-value `Report`, and the `OK == (len(Failures) == 0)` invariant.

  `internal/preflight/testmain_test.go` is untagged and in-package (`package preflight`), mirroring `internal/loomengine/testmain_test.go` exactly: a `TestMain` calling `gitkit.HermeticGitEnv()` before `os.Exit(m.Run())`.
  `internal/preflight` becomes a git-spawning test package with this batch, and the Hermetic Git Test Environment Invariant requires the `TestMain` to be present in the package directory.
  Leave it untagged so it is compiled into both the tagged and the untagged test binary.

  `internal/preflight/preflight_integration_test.go` carries `//go:build integration` on its first non-empty line and declares `package preflight_test` — an **external** test package, because `internal/hubforge` imports `internal/fabriccli` and `internal/preflight` sits inside that dependency set, so an in-package fixture test would close a compile cycle.
  Adopt this shape from the start rather than discovering the cycle later.
  Build fixtures with `hubforge.NewHub` following the setup pattern `internal/loomengine/preflight_integration_test.go` already uses, and drive the exported `CheckResolved`, `Check`, `Wired` and `HubPresent` directly — no shim is needed, which is why this task adds no `export_test.go`.

  Cover at least these scenarios:

  - A healthy wired pair reports `OK` true with no failures.
  - Not a git repository, driven through `Check`: the verdict is a single `CheckGeometry` failure **and** the returned error is nil — the report-not-error contract's most easily-regressed row.
  - A `fabricengine.PrimeName` failure short-circuits with only a geometry failure and no other check recorded.
  - A dirty warp side reports `CheckWorktreeClean`.
  - A dirty paired side reports `CheckWorktreeClean`.
  - Fabric not ready reports `CheckFabricReady`.
  - A branch mismatch classifies as `CheckFabricSync`, not `CheckJunction`.
  - A broken junction classifies as `CheckJunction`.
  - A subpath-anchored hub standing at its own anchor is **not** rejected.
  - Multiple simultaneous failures collect rather than short-circuit.

  The last two are the behaviours a naive rewrite silently removes, so neither may be omitted.

  Assert both predicates on their positive path against the ordinary healthy pair the fixture builds: `Wired(cwd)` returns a non-nil `*lyxcwd.Location` and true, and `HubPresent(cwd)` does the same.
  `Wired` is a newly exported predicate with no consumer in this task — T7 and T8 are its first callers — so without this row its true branch ships exercised only indirectly, through the `fabricengine.Ready` call inside `CheckResolved`.

  Add one more assertion that exists purely to pin why both predicates ship: with cwd at `<hub>/_board`, `HubPresent` returns true and `Wired` returns false.
  Reach that directory through the `hubforge` fixture's own accessors and `fabricengine.BoardDir`, never by joining a `_board` literal.
- **Commit:** `test(preflight): cover the lifted checks and the predicate split`

### Card 14: compose loomengine over internal/preflight

- **Context:**
  - `internal/preflight/report.go`
  - `internal/preflight/preflight.go`
  - `internal/loomengine/export_test.go`
  - `internal/loomengine/preflight_integration_test.go`
  - `internal/loomengine/coherence.go`
  - `internal/loomengine/config.go`
- **Edits:**
  - `internal/loomengine/preflight.go`
  - `internal/loomengine/report.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `internal/loomengine/report.go`, delete the local `CheckID` type declaration, the `Failure` struct, the `Report` struct, the five lifted check-ID constants, and the unexported `addFailure` method.
  Replace them with type **aliases** and const aliases:
  `type CheckID = preflight.CheckID`, `type Failure = preflight.Failure`, `type Report = preflight.Report`, and one const alias per lifted ID (`const CheckGeometry = preflight.CheckGeometry`, and the same for `CheckWorktreeClean`, `CheckFabricReady`, `CheckFabricSync`, `CheckJunction`).
  These must be aliases (`=`), never new named types — an alias makes `loomengine.Report` and `preflight.Report` the identical type, which is what lets `internal/loomengine/preflight_integration_test.go` compile unedited.
  Keep the four loom-specific constants `CheckSeedMissing`, `CheckSeedUnreadable`, `CheckSeedIncoherent` and `CheckHalfFinished` declared here, now typed as `preflight.CheckID`, with their existing string values unchanged.

  In `internal/loomengine/preflight.go`, delete the local geometry-resolution block and the bodies of checks 1b, 2 and 3, and compose instead:

  - `Preflight(cwd string) (Report, error)` calls `preflight.Check(cwd)`.
    On a non-nil error it returns `(Report{}, err)`.
    When the returned report has `Has(CheckGeometry)` true, return that report unchanged and do not run check 4 — this reproduces both of today's early returns, the not-a-git-repo one and the no-main-worktree one, since `preflight.Check` short-circuits each with a single geometry failure.
    Otherwise it proceeds to check 4 against the `*lyxcwd.Location` `preflight.Check` handed back, never re-resolving.
  - Keep an unexported `checkResolved(l *lyxcwd.Location) (Report, error)` with its current name and signature, now implemented as `preflight.CheckResolved(l)` followed by check 4 against the same `Location`.
    Preserving the name and signature is what leaves `internal/loomengine/export_test.go`'s `CheckResolvedForTest = checkResolved` valid with no edit, while still exercising checks 1b–4 together.
    Apply the same `Has(CheckGeometry)` short-circuit here before running check 4.
  - Replace today's `check3BlocksSeed` local with a derivation from the report: `report.Has(CheckFabricReady) || report.Has(CheckJunction)`.
    Those are exactly the two conditions that set the flag today — the not-ready branch and the non-branch-mismatch `Healthy` branch, which is the same branch that records `CheckJunction` — so the derivation is equivalence-preserving rather than approximate.
  - Every remaining `addFailure` call site becomes `AddFailure`, and check 4's body is otherwise unchanged: the `os.Stat(LoomStatusFile(l))` gate and its three-way classification, the `os.MkdirAll(filepath.Dir(LoomStatusLock(l)))` workaround **including its long explanatory comment**, the `state.ReadJSONStrict` call with its `state.ErrDecode`-versus-escalate split, the synthesised "seed vanished between stat and read" error on the not-found race, and the `checkCoherence` loop.
  - Set `report.OK = len(report.Failures) == 0` before returning, as today.

  Update `Preflight`'s godoc where it describes which checks it runs, but keep its stated contract — the four preconditions, the caller-resolved seam `cwd`, the must-not-invoke-on-an-advanced-task warning, and the three return shapes — intact.
  Update the file header comment on `internal/loomengine/preflight.go` and on `internal/loomengine/report.go` to describe their reduced scope.
  Do not use the tokens `weft` or `warp` in either file.
  Do not edit `internal/loomengine/preflight_integration_test.go` or `internal/loomengine/export_test.go`: both are in `Context:` because the implementer must read them to confirm the aliases hold, and a needed edit to either means the aliases were built wrong.
- **Commit:** `refactor(loomengine): compose Preflight over internal/preflight`

## Batch Tests

`verify:` runs `go test ./internal/preflight/... ./internal/loomengine/... ./internal/lyxcwd/...` followed by `go test -tags integration ./internal/preflight/... ./internal/loomengine/...`.

The untagged run covers the new `internal/preflight/report_test.go` and every existing untagged `internal/loomengine` test (`coherence_test.go`, `config_test.go`, `discussion_test.go`, `discussionpath_test.go`, `loomstatus_test.go`, `plan_test.go`, `prompt_test.go`), which is the compile-level proof that the alias rewrite in `internal/loomengine/report.go` did not change any type the rest of the package depends on.
The tagged run is required twice over: this batch creates `internal/preflight/preflight_integration_test.go`, which Go does not compile at all without the tag, and `internal/loomengine/preflight_integration_test.go` — the 13-function compatibility contract that is the actual behaviour-preservation proof for the whole lift — is itself integration-tagged and invisible to the untagged run.
`./internal/lyxcwd/...` is included for `TestEnforcement_FabricVocabulary` and `TestEnforcement_GeometryLiterals`; the vocabulary walk is the check most likely to fail on this batch, since the lifted code sits next to `fabricengine` identifiers the new packages are forbidden to name.
