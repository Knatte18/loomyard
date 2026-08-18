# Discussion: lift the orchestrator preflight out of loomengine, plus the shared standalone-CLI foundations

```yaml
task: lift the orchestrator preflight out of loomengine, plus the shared standalone-CLI foundations
slug: orchestrator-preflight
status: discussing
parent: standalone-producers
```

## Problem

`loomengine.Preflight` (`internal/loomengine/preflight.go`) is the only implementation in the repo of what `manifest/designs/producers-standalone.md` calls tiers 1 and 2 — geometry resolution via `lyxcwd.Resolve`, then `fabricengine.PrimeName`/`Clean`/`Ready`/`Healthy`.
Its check 4 is `loom`-specific: `_lyx/loom/status.json` presence, readability and coherence.
`Hardener`, and any future `Shed` product, would have to re-implement checks 1–3 verbatim to gate themselves the same way.
The three-tier model that the producers-standalone design rests on — *producers require nothing, orchestrators require tier 3* — is a convention rather than something the code enforces, precisely because tiers 1 and 2 are welded inside one orchestrator's package.

**Why now.** This is task T5 of `manifest/designs/producers-standalone.md`, wave 2.
Two later tasks in that decomposition are blocked on it in a non-obvious way.
T7 (wave 3, standalone `lyx webster run`) and T8 (wave 4, standalone `burler`/`perch`) both need a *hub-mode trigger* — a check that says "Fabric is actually wired here, so run in hub mode; otherwise run standalone" — and that check is a tier-2 check.
A `*cli` package cannot make one today without importing `internal/fabricengine`, the repo's largest and most dangerous package, directly into a CLI.
Both tasks also need two small shared leaves (`internal/buildinfo`, `internal/standalonestate`) that originally lived in T8's brief on the assumption `burlercli`/`perchcli` were the only standalone entry points.
They are not — T7 sits an entire wave earlier than T8, so anything only T8 introduced would not exist when T7 needs it.
T5 precedes both, so the design doc moved all of it here rather than making T7 depend on T8 or duplicating the logic twice.

There is also a live defect in scope.
`cmd/lyx/main.go:97` sets `cobra.EnableTraverseRunHooks`, so root's `seedStencils` (`cmd/lyx/stencilseed.go:33`) runs before *every* module pre-run, and it triggers on bare `lyxcwd.Resolve` success (`stencilseed.go:51-56`).
`lyxcwd.Resolve` succeeds in any ordinary git repository run from its root, handing back a `Location` whose `HubPath` is `filepath.Dir(worktreeRoot)` — never verified to be a hub.
Pointed at a plain downloaded repo, `seedStencilsAt(l.HubPath, …)` therefore writes `<repo-parent>/_board/_lyx/stencils/**` and tries to commit it: a fictional-hub write, happening one layer above any single CLI.
It is invisible today only because nobody points `lyx` at a non-lyx git repo on purpose yet.

## Scope

**In:**

- A new `internal/preflight` package holding the orchestrator-agnostic tier-1 + tier-2 checks lifted out of `loomengine`, plus the `CheckID`/`Failure`/`Report` result types, its `doc.go`, and its tests.
- `internal/loomengine/preflight.go` rewritten to compose `internal/preflight` with its own check-4 (`_lyx/loom/status.json`) logic; `internal/loomengine/report.go` reduced to the loom-specific check IDs plus aliases.
- A new stdlib-only `internal/buildinfo` package owning the ldflags-stamped build channel, plus its `doc.go` and tests.
- A new stdlib-only `internal/standalonestate` package owning the `hash8` + per-OS `<state>` directory derivation, plus its `doc.go` and tests.
- A new `stencilstore.ModeFor(dev bool) Mode` helper — the single mapping site from "is this a dev build" to `stencilstore.Mode`.
- `cmd/lyx/stencilseed.go`: drops its own `buildChannel` variable in favour of `buildinfo`, and gates `seedStencils` on the tier-1-AND-tier-2 wiring predicate.
- `tools/deploy/main.go:62`: the ldflags path repointed from `-X main.buildChannel=dev` to `-X github.com/Knatte18/loomyard/internal/buildinfo.Channel=dev`.
- `CONSTRAINTS.md`: two new leaf invariants (`Buildinfo Leaf Invariant`, `Standalonestate Leaf Invariant`), each with a mechanical enforcement test.
- Docs in the same commit: `doc.go` for each new package, three rows in `docs/overview.md`'s directory tree, and entries in `docs/shared-libs/README.md`.

**Out:**

- **The three-tier invariant in `CONSTRAINTS.md`.**
  That belongs to T10 (`standalone-docs-and-invariants`), whose own brief states the rule is only true once every package obeys it, and that writing it before T5 ships would pin a model the code does not implement.
  T5 makes the rule *true for the preflight*; it does not write the rule down.
- **Rewording the Cwd Resolution Invariant.** Also T10.
- **Any consumer of `internal/standalonestate`.**
  This task lands the derivation and its tests; `burlercli`/`perchcli`/`webstercli` wire it up in T7 and T8.
  Nothing in `cmd/` or any `*cli` package imports `standalonestate` at the end of this task, and that is expected, not an oversight.
- **Any change to `internal/fabricengine`.**
  `internal/preflight` calls its existing exported functions and adds nothing to it.
- **Behaviour changes to `loomengine.Preflight`.**
  Same `Report`, same failure classification, same report-not-error contract, same short-circuit semantics for every case its current tests pin.
- **The `stencilstore.Reconcile` machinery itself**, the stamping mechanism, and the Dev/Prod Binary Separation invariant.
  Only the ldflags *symbol's home* moves.
- **`manifest/roadmap.md`.**
  Per `CLAUDE.md`, the roadmap moves on completing a planned item; the producers-standalone wave-2 entry moves when the wave completes, not per-task. Leave it alone.

## Decisions

### result-types-live-in-preflight-loomengine-aliases

- **Decision.** `internal/preflight` declares `CheckID`, `Failure`, `Report`, and the five tier-1/tier-2 check-ID constants (`CheckGeometry`, `CheckWorktreeClean`, `CheckFabricReady`, `CheckFabricSync`, `CheckJunction`).
  `internal/loomengine/report.go` keeps its four loom-specific constants (`CheckSeedMissing`, `CheckSeedUnreadable`, `CheckSeedIncoherent`, `CheckHalfFinished`) — now typed as `preflight.CheckID` — and adds type aliases (`type Report = preflight.Report`, `type Failure = preflight.Failure`, `type CheckID = preflight.CheckID`) plus const aliases for the five lifted IDs (`const CheckGeometry = preflight.CheckGeometry`, and so on).
- **Rationale.** Type *aliases*, not new named types, mean `loomengine.Report` and `preflight.Report` are the identical type: every existing reference in `internal/loomengine/preflight_integration_test.go` (which names `loomengine.CheckGeometry`, `loomengine.CheckWorktreeClean`, etc. across 13 test functions) keeps compiling with no edit, and any future caller can pass a `preflight.Report` where a `loomengine.Report` is expected.
  A single `Report` type also means an orchestrator composes tier-3 failures onto the tier-1/2 report by appending, never by converting between two parallel shapes.
- **Rejected.**
  *Duplicate types in both packages with a conversion function in `loomengine`* — every check-ID constant would need a translation table, and the two `CheckID` string sets would drift silently the first time one side added a value.
  *`preflight` returns a bare `[]Failure` and `loomengine` keeps `Report`* — pushes the `OK == (len(Failures) == 0)` invariant and the `addFailure` bookkeeping into every consumer, which is exactly the duplication the lift exists to remove; `Hardener` would re-implement `Report` verbatim.

### preflight-exposes-three-entry-points

- **Decision.** `internal/preflight` exports exactly three functions:
  - `Check(cwd string) (Report, *lyxcwd.Location, error)` — resolves geometry itself, runs tiers 1 and 2, and hands the resolved `Location` back so the caller never re-resolves.
    On `lyxcwd.ErrNotAGitRepo` it returns `(Report{OK:false, Failures:[{CheckGeometry, "not inside a git repository"}]}, nil, nil)`; on any other `Resolve` error it returns `(Report{}, nil, err)`.
  - `CheckResolved(l *lyxcwd.Location) (Report, error)` — the same tier-2 body against an already-resolved `Location`, for callers that hold one and for tests that synthesise one with no backing directory on disk.
  - `Wired(cwd string) (*lyxcwd.Location, bool)` — the cheap boolean hub-mode predicate (see the next Decision).
- **Rationale.** `Check` is what an orchestrator calls; `CheckResolved` is what `loomengine`'s existing `CheckResolvedForTest` seam becomes, preserving the ability to drive checks against a hand-built `Location`; `Wired` is what a CLI pre-run calls.
  Returning the `*lyxcwd.Location` from `Check` matters: `lyxcwd.Resolve` spawns `git rev-parse --show-toplevel`, so a shape that forced `loomengine` to resolve again to reach `LoomStatusFile(l)` would double the git spawns of every preflight.
- **Rejected.**
  *`CheckResolved` only, caller resolves* — every orchestrator would re-implement the `ErrNotAGitRepo`-is-a-verdict-not-an-error special case, which is subtle enough that duplicating it is how the classification drifts.
  *`Check(cwd) (Report, error)` with no `Location` return* — forces the double resolve above.

### seed-gate-is-tier1-plus-Ready-not-the-full-tier-2-report

- **Decision.** `preflight.Wired(cwd)` is `lyxcwd.Resolve(cwd)` succeeding **and** `fabricengine.Ready(l)` returning true; it returns `(nil, false)` on any error rather than surfacing one, since it is a predicate for a pre-run that must never block a command.
  `cmd/lyx/stencilseed.go`'s `seedStencils` gates on exactly this.
- **Rationale.** `fabricengine.Ready` (`internal/fabricengine/ready.go:17`) is a single `os.Stat` of `WeftWorktree(l)` — the `<hub>/<worktree-base>-weft` sibling — with zero process spawns.
  In a plain downloaded repo at `/x/repo`, `HubPath` is the fiction `/x`, so it stats `/x/repo-weft`, finds nothing, and the gate closes: exactly the defect this task fixes, at no per-command cost.
  The full tier-2 set is wrong for *this* gate on two counts.
  `Clean` spawns `git status` on both sides of the pair, and `seedStencils` runs before every single `lyx` command via `EnableTraverseRunHooks` — that is a per-invocation regression on every command in the CLI.
  Worse, `Clean` failing would mean a *dirty hub does not get its stencils seeded*, which is a behaviour change nobody asked for and which would surface as stencils mysteriously going stale mid-work.
  `Healthy`'s branch-mismatch case has the same problem.
  `PrimeName` spawns `git worktree list` and adds nothing the `Ready` stat does not already prove for the purpose of "is there a real hub here".
- **Note for the reviewer.** T5's brief says "the identical tier-1-AND-tier-2 check".
  This is that check in the sense the design doc means it — the tier-2 *wiring* predicate, `fabricengine.Ready`-class, named in T8's own brief as "`fabricengine.Ready`-class, reached through the `internal/preflight` package T5 lifts" — not the full four-function tier-2 report.
  `Wired` living in `internal/preflight` is what satisfies the real constraint, which is that no `*cli` package imports `internal/fabricengine` to make the check.
- **Rejected.**
  *Gate on `preflight.Check(cwd).OK`* — the `Clean`/`Healthy` problems above.
  *Stat `<hub>/_board/_lyx` inline in `stencilseed.go`* — re-implements the gate outside `preflight`, so T7's and T8's copies would be a third and fourth implementation, and the fabric-vocabulary knowledge leaks into `package main`.

### report-exposes-Has-instead-of-a-blocks-seed-field

- **Decision.** `Report` gains two exported methods: `Has(CheckID) bool` and `AddFailure(CheckID, string)` (the latter replacing today's unexported `addFailure`, since `loomengine` now appends from another package).
  `loomengine` derives its two control-flow signals from the returned report rather than from bespoke fields:
  - geometry short-circuit: `report.Has(CheckGeometry)` → return the report unchanged, do not run check 4 (matching today's two early returns at `preflight.go:43` and `preflight.go:65`).
  - today's `check3BlocksSeed`: `report.Has(CheckFabricReady) || report.Has(CheckJunction)`.
- **Rationale.** Those are exactly the two conditions that set `check3BlocksSeed = true` in the current code (`preflight.go:100` on `!ready`, `preflight.go:118` on the non-branch-mismatch `Healthy` causes, which is the same branch that adds `CheckJunction`), so the derivation is equivalence-preserving rather than approximate.
  Deriving keeps `Report` a plain value type with no orchestrator-specific state riding along, which matters because `Hardener` will want its own downstream-consequence rule and it will not be this one.
- **Rejected.**
  *A `BlocksSeed bool` field on `Report`* — bakes one orchestrator's check-4 concern into the shared type.
  *A second return value from `CheckResolved`* — same coupling, plus it widens the signature for every caller that does not care.

### buildinfo-exposes-IsDev-not-StencilMode

- **Decision.** `internal/buildinfo` exports `var Channel string` and `func IsDev() bool` (`Channel == "dev"`), and imports nothing but the standard library — in practice, nothing at all.
  The mapping to a stencil mode moves into `internal/stencilstore` as a new `func ModeFor(dev bool) Mode`.
  `cmd/lyx/stencilseed.go` becomes `mode := stencilstore.ModeFor(buildinfo.IsDev())`; T7's and T8's CLIs do the same.
- **Rationale.** T5's brief names the accessor `StencilMode()`, but `stencilstore.Mode` is a named `int` type declared in `internal/stencilstore/stencilstore.go:135`, and `internal/stencilstore` imports `internal/logger` and `internal/stencil`.
  A `buildinfo.StencilMode()` returning it would make `buildinfo` a non-leaf, contradicting the stdlib-only requirement in the *same paragraph* of the brief — and that requirement is the load-bearing one, since it is what lets `cmd/lyx` and every `*cli` package import `buildinfo` with no cycle risk.
  `ModeFor` in `stencilstore` keeps a single mapping site (no duplication across three future CLIs) and puts the `Mode` knowledge in the package that owns `Mode`.
  Semantics are preserved exactly: an unstamped binary leaves `Channel` empty, and empty maps to `ModeProduction`, which is also `Mode`'s zero value.
- **Rejected.**
  *`buildinfo` declaring its own mirror `Mode` type* — two `Mode` types one conversion apart is worse than one accessor rename.
  *Relaxing the leaf to stdlib + `stencilstore`* — `stencilstore` pulls in `logger` and `stencil`, and the whole point of the leaf is that a CLI can read the build channel without inheriting anyone's dependency set.
- **Consequence for the brief.** This is a deliberate, documented deviation from T5's `StencilMode()` wording.
  Record it in `internal/buildinfo/doc.go` so the next reader of the design doc does not "fix" it back.

### standalonestate-is-pure-derivation-with-an-injectable-seam

- **Decision.** `internal/standalonestate` exports one function:

  ```go
  func Derive(target string) (stateDir string, hash8 string, err error)
  ```

  `hash8` is SHA-256 over the normalised absolute target path, hex-encoded, truncated to the first eight characters.
  Normalisation is `filepath.Abs` → `filepath.EvalSymlinks` → `filepath.Clean`, falling back to `Clean` alone when `EvalSymlinks` fails (the target may not exist yet), and lower-cased before hashing when `runtime.GOOS == "windows"`.
  `stateDir` is `%LOCALAPPDATA%\lyx\<hash8>\` on Windows, and `$XDG_STATE_HOME/lyx/<hash8>/` falling back to `~/.local/state/lyx/<hash8>/` everywhere else.
  `Derive` creates nothing on disk.
  It returns an error only when the state root cannot be determined at all (`LOCALAPPDATA` unset on Windows; `XDG_STATE_HOME` unset *and* `os.UserHomeDir` failing elsewhere).
  Internally it is a thin wrapper over an unexported `derive(goos, localAppData, xdgStateHome, home, target string) (string, string, error)`, re-exported through `export_test.go`.
- **Rationale on normalisation.** Two spellings of the same directory — a symlinked path, a differing-case path on Windows or macOS — must hash identically, or two standalone runs against the same target get different sockets, sessions and state directories, silently destroying the "one tmux server per target directory, resumable" property this whole derivation buys.
  The semantics deliberately mirror `internal/lyxcwd/anchor.go`'s `normalizePath`/`samePath` (`anchor.go:112-129`), which already solve this exact class of problem: `EvalSymlinks` with a `Clean` fallback, plus case-insensitive comparison on Windows.
  Note the one intentional difference — `samePath` compares case-insensitively *after* normalising, whereas hashing has no comparison step, so the case fold must happen to the string before it is hashed.
  macOS is also case-insensitive in practice, but `lyxcwd` folds only on Windows, and matching `lyxcwd`'s rule exactly is worth more than being marginally more correct on a platform where the two derivations would then disagree.
- **Rationale on the injectable seam.** `runtime.GOOS` is a compile-time constant, so without a seam the Windows row of the `<state>` table is untestable on Linux and the POSIX row is untestable on Windows — meaning CI would only ever exercise one of the two branches this task ships.
  Passing `goos` and the three environment values as parameters makes both rows testable everywhere, and `export_test.go` keeps the seam out of the public API.
  This is the same `export_test.go` idiom `internal/loomengine/export_test.go` already uses for `CheckResolvedForTest`.
- **Rejected.**
  *`Derive` also doing `os.MkdirAll`* — a pure derivation is testable with no filesystem and no cleanup, and the consumer that actually writes there (T7/T8) is better placed to decide when the directory should exist.
  *Reading `runtime.GOOS` and `os.Getenv` directly with `t.Setenv`-driven tests* — `t.Setenv` handles the env vars but cannot change `runtime.GOOS`, so half the table stays untested.
  *Reusing `lyxcwd`'s unexported `normalizePath` by exporting it* — widens `internal/lyxcwd`'s API for one caller, and `standalonestate` must stay stdlib-only, so it cannot import `lyxcwd` regardless.

### two-new-leaf-invariants-now-three-tier-rule-in-T10

- **Decision.** `CONSTRAINTS.md` gains `## Buildinfo Leaf Invariant` and `## Standalonestate Leaf Invariant` in this commit, each enforced by a `leaf_enforcement_test.go` in its own package.
  The three-tier invariant and the Cwd Resolution Invariant reword are **not** touched.
- **Rationale.** Both new packages are load-bearing *because* they are leaves — T7 and T8 import them from CLI packages specifically to avoid cycles — and an unenforced leaf claim rots the first time someone adds a convenience import.
  The enforcement tests are a direct copy of `internal/tokenvocab/leaf_enforcement_test.go`'s allowlist walk (`go/parser` with `parser.ImportsOnly`, stdlib detected as "no `.` in the first path segment"), with the allowlist set to empty for both packages since neither has a permitted non-stdlib import.
  Deferring the three-tier rule is T10's explicit instruction, quoted in its brief.
- **Rejected.**
  *Landing the three-tier invariant here* — contradicts T10's brief and would state a rule that is still false for the producer packages T6–T8 have not converted yet.
  *Leaf claims as review obligations only* — the repo's own precedent (`modelspec`, `tokenvocab`, `pattern`) is a mechanical test per leaf, and the CONSTRAINTS format requires naming an enforcement basis.

### preflight-tests-are-an-external-test-package

- **Decision.** `internal/preflight`'s git-fixture tests live in `package preflight_test` (external), integration-tagged, driving `CheckResolved` through a `preflight/export_test.go` shim where they need a synthetic `Location`.
  `internal/loomengine`'s existing `preflight_integration_test.go` stays where it is and keeps testing `loomengine.Preflight` end-to-end unchanged.
- **Rationale.** The comment at the head of `internal/loomengine/preflight_integration_test.go` records why: `internal/hubforge` (the fixture helper) imports `internal/fabriccli`, so an *in-package* test importing `hubforge` from a package inside `fabriccli`'s dependency set closes a compile cycle.
  `internal/preflight` will sit in that same dependency set the moment anything under `fabriccli` reaches it, so adopting the external-test-package shape from the start avoids discovering the cycle later.
  Keeping `loomengine`'s existing 13 test functions running against the composed `loomengine.Preflight` is the actual proof that the lift changed no behaviour — they are the regression net, and none of them should need editing.
- **Rejected.**
  *Moving the tier-1/tier-2 test functions out of `loomengine` into `preflight`* — that would delete the very tests that prove `loomengine.Preflight` still behaves identically after the lift.
  New `preflight` tests are additive; `loomengine`'s are untouched.

### seed-gate-tested-through-an-extracted-target-seam

- **Decision.** `cmd/lyx/stencilseed.go` extracts the gate into `func stencilSeedTarget(ctx context.Context) (hub, worktree string, ok bool)`, which does the `lyxcwd.CwdFrom` → `preflight.Wired` sequence and returns `ok == false` whenever seeding must be skipped.
  `seedStencils` keeps its `testing.Testing()` early return and becomes a three-line wrapper over `stencilSeedTarget` + `seedStencilsAt`.
  An integration-tagged `cmd/lyx` test drives `stencilSeedTarget` against a real plain git repository and asserts both `ok == false` and that no `_board` directory exists beside the repo afterwards.
- **Rationale.** `seedStencils` returns immediately under `testing.Testing()` (`stencilseed.go:40-42`), so a test can never observe its gate through the exported entry point — the test would pass vacuously and prove nothing.
  Extracting the decision into a value-returning function makes the gate directly assertable, and mirrors the existing rationale for `seedStencilsAt` being separate ("so a test can drive it directly against a real hub without going through `seedStencils`' `testing.Testing()` guard").
  Asserting the absence of `_board` on disk, not just `ok == false`, is what pins the actual defect rather than an implementation detail.
- **Rejected.**
  *Testing `seedStencils` directly* — vacuous under the `testing.Testing()` guard.
  *Removing the `testing.Testing()` guard* — it exists to keep dozens of untagged `cmd/lyx` tests from spawning git, which the Test Tier Purity Invariant forbids.

## Technical context

**The lift, concretely.** `internal/loomengine/preflight.go` today is one exported `Preflight(cwd string) (Report, error)` plus an unexported `checkResolved(l *lyxcwd.Location) (Report, error)`.
The split runs straight through `checkResolved`: check 1b (`fabricengine.PrimeName`, with its short-circuit), check 2 (`fabricengine.Clean`), and check 3 (`fabricengine.Ready` then `fabricengine.Healthy`, with the `CauseBranchMismatch` → `CheckFabricSync` / everything-else → `CheckJunction` classification) all move to `internal/preflight`.
Check 4 — everything from the `os.Stat(LoomStatusFile(l))` at `preflight.go:125` to the `checkCoherence` loop — stays in `loomengine`, including the `os.MkdirAll(filepath.Dir(LoomStatusLock(l)))` workaround and its long comment, and including the TOCTOU `seed vanished between stat and read` synthesised error.

**`loomengine.Preflight` has no production callers today.** The only references anywhere are `internal/loomengine/preflight_integration_test.go` and `internal/loomengine/export_test.go`.
That makes the blast radius of the signature work small, and it means the integration test file *is* the compatibility contract.

**Existing helpers to reuse, not re-derive.**

- `internal/fabricengine`: `PrimeName(l)` (`worktreelist.go:86`), `Clean(l)` (`warpclean.go:20`), `Ready(l)` (`ready.go:17`), `Healthy(l)` (`drift.go:52`, returning a typed `HealthReason` with a `Cause` field and `CauseBranchMismatch` constant), `StencilsDir(hub)` (`junctionnames.go:126`), `WeftWorktree(l)` (`fabric.go:115`).
- `internal/lyxcwd`: `Resolve(cwd)`, `ErrNotAGitRepo`, `CwdFrom(ctx)`, and — as the semantic reference for `standalonestate`, not as an import — `normalizePath`/`samePath` (`anchor.go:112-129`).
- `internal/tokenvocab/leaf_enforcement_test.go` — copy this file's structure for both new leaf enforcement tests; it is the current idiom (`go/parser`, `parser.ImportsOnly`, allowlist map, stdlib-by-first-segment heuristic).
- `internal/loomengine/export_test.go` — the `export_test.go` shim idiom, for both `preflight` and `standalonestate`.
- `internal/hubgeom/doc.go` — the told-geometry vocabulary and tone the new `doc.go` files should match.

**Gotchas.**

- `lyxcwd.Resolve` validates far less than its name suggests: `git rev-parse --show-toplevel` must succeed, an absent `_board/.lyx-anchor` marker is *not* an error (`AnchorRel` falls back to `"."`), and `HubPath` is assigned `filepath.Dir(worktreeRoot)` unconditionally with no hub check.
  Every "is this really a hub" question must be answered at tier 2, never by a successful `Resolve`.
- There is deliberately **no** at-the-anchor check in `checkResolved` — `Resolve`'s cwd gate already proves `cwd == AnchorPath()`, and re-adding one broke every subpath-anchored hub.
  The comment at `preflight.go:70-74` explaining this must move to `internal/preflight` with the code, not be dropped.
- `Preflight`'s contract is *report, not error*: `(Report{}, err)` means "could not determine", and a `PrimeName` failure is deliberately reported as a `CheckGeometry` failure rather than escalated.
  `internal/preflight` must preserve this exactly, and its `doc.go` should restate it, since `Hardener` will be reading that doc rather than `loomengine`'s.
- `stencilstore.Mode` is `ModeProduction Mode = iota` then `ModeDev` — `ModeProduction` is the zero value, which is why an unstamped binary safely means production.
  `ModeFor(false)` must return `ModeProduction`.
- Changing the ldflags path in `tools/deploy/main.go` and the variable's home in `cmd/lyx/stencilseed.go` must land together: a stale `-X main.buildChannel=dev` against a removed `main.buildChannel` fails silently (Go's linker does not error on an unmatched `-X`), producing a dev binary that behaves as production.
  Grep confirms exactly three sites: `tools/deploy/main.go:60,62` and `cmd/lyx/stencilseed.go:24,29,74`.
- New `cmd/lyx` test files must respect the Test Tier Purity Invariant — the plain-repo gate test spawns git, so it needs a `//go:build integration` first line, and `cmd/lyx` already has a `TestMain` for the Hermetic Git Test Environment Invariant.
- `internal/preflight` and `internal/standalonestate` both need their package name checked against nothing existing — `ls internal/` confirms neither name is taken.

**Docs to touch (same commit, per `CLAUDE.md`).** `docs/overview.md`'s directory tree around lines 228-244 (add three rows, alongside `internal/hubgeom`, `internal/modelspec`, `internal/tokenvocab`) and its shared-infrastructure sentence at line 315; `docs/shared-libs/README.md`'s `## Libraries` section.
`manifest/designs/producers-standalone.md` itself is deleted by T10, not edited here.

## Constraints

From `CONSTRAINTS.md`, binding this task:

- **Cwd Resolution Invariant** — `internal/lyxcwd` alone owns cwd resolution.
  `internal/preflight` *calls* `lyxcwd.Resolve`; it must never re-derive a worktree root, a hub path, or an anchor itself, and `internal/standalonestate` must not resolve a cwd at all (it is told a target path).
- **Test Tier Purity Invariant** — untagged test files spawn nothing.
  Every new test that touches a real git repo or a `hubforge`/`gitkit` fixture needs a `//go:build integration` constraint on its first non-empty line, and is enforced by `cmd/lyx/tierpurity_test.go`.
- **Hermetic Git Test Environment Invariant** — any new git-spawning test *package* needs a `TestMain` calling `gitkit.HermeticGitEnv()`.
  `internal/preflight` will be one; `cmd/lyx` and `internal/loomengine` already have theirs.
- **Dev/Prod Binary Separation** — unchanged in substance, but this task moves the stamped symbol.
  The `-dev` build must still land in `.dev-bin` and must still be the only thing that produces `ModeDev`.
- **Fabric Vocabulary Invariant / Fabric Write-Side Containment** — `internal/preflight` is read-only; it must not acquire a lock, write, or record a mutation.
  Keeping it out of `internal/fabricengine` is precisely so a read-only check does not inherit that blast radius.
- **Documentation Lifecycle** — new packages get a `doc.go`; `docs/overview.md` moves when the module table changes.
- **CLI / Cobra Invariant** — `cmd/lyx/stencilseed.go` changes must not alter command registration, `Short` strings, or the help tree; the existing `cmd/lyx/helptree_test.go` and `registration_test.go` should be untouched and still pass.

From the design doc, binding this task:

- Placement in `internal/preflight` is decided, not open — never a composite verb on `internal/fabricengine`.
- `internal/buildinfo` and `internal/standalonestate` are stdlib-only leaves.
- `loomengine.Preflight`'s observable behaviour must be unchanged.
- T5 is parallel-safe with T4 and must not touch files T4 touches; it owns `cmd/lyx/stencilseed.go` and `tools/deploy/main.go` for this wave.

## Testing

**`internal/preflight` — TDD candidate, and the main new test surface.**
Untagged, in-package tests for anything that needs no git: `Report.Has`, `Report.AddFailure`, and the `OK == (len(Failures) == 0)` invariant.
An integration-tagged `package preflight_test` file for the check bodies, driving `CheckResolved` via the `export_test.go` shim against `hubforge` fixtures.
Scenarios to cover: healthy wired pair; not a git repo (via `Check`, asserting the `CheckGeometry` verdict *and* a nil error); `PrimeName` failure short-circuiting with only a geometry failure; dirty warp; dirty weft; fabric not ready; branch mismatch classifying as `CheckFabricSync`; a broken junction classifying as `CheckJunction`; a subpath-anchored hub *not* being rejected for standing at its own anchor; and multiple simultaneous failures collecting rather than short-circuiting.
The last two are the ones most likely to regress silently in a lift, since both are behaviours a naive rewrite removes.

**`internal/loomengine` — the regression net, ideally edited not at all.**
`preflight_integration_test.go`'s 13 existing test functions are the compatibility contract: same `Report`, same failure classification, same report-not-error contract.
If the type aliases are done right, this file needs zero edits, and "it compiles and passes unmodified" is itself a meaningful assertion — treat any required edit to it as a signal that the alias decision was implemented as duplicate types instead.
`export_test.go`'s `CheckResolvedForTest` should now delegate to the composed path so those tests still exercise checks 1b–4 together.

**`internal/buildinfo` — trivial but worth pinning.**
Untagged: empty `Channel` → `IsDev() == false`; `Channel == "dev"` → true; any other value (`"prod"`, `"Dev"`, whitespace) → false, so the comparison stays exact rather than drifting to a prefix or case-insensitive match.
Plus `leaf_enforcement_test.go` with an empty allowlist.

**`internal/standalonestate` — TDD candidate.**
Untagged in-package tests over the injected `derive` seam.
Scenarios: the Windows row produces `%LOCALAPPDATA%\lyx\<hash8>` and the POSIX row produces `$XDG_STATE_HOME/lyx/<hash8>`, both driven on any host via the seam; the `XDG_STATE_HOME`-unset fallback to `~/.local/state`; `LOCALAPPDATA` unset on Windows returning an error; both spellings of a case-differing path producing the *same* `hash8` under `goos == "windows"` and *different* hashes under `goos == "linux"`; a relative and an absolute spelling of the same path agreeing; `hash8` being exactly 8 lowercase hex characters; and stability — the same input yielding the same hash across calls, which is the property the whole resumability story rests on.
A symlink case needs a real filesystem, so it belongs in a small integration-tagged test (creating a symlink is a filesystem spawn, not a git one, but tagging keeps tier 1 clean).
Also assert `Derive` creates nothing on disk.
Plus `leaf_enforcement_test.go` with an empty allowlist.

**`cmd/lyx` — the defect test named in T5's verify line.**
Integration-tagged: build a plain git repository with no `_board` sibling, drive `stencilSeedTarget` against it, assert `ok == false`, then assert no `_board` directory was created beside the repo.
A companion positive case against a `hubforge` fixture asserting `ok == true` and the returned `hub`/`worktree` matching the fixture's, so the gate is not trivially always-false.

**Task-wide verify.**
`go test ./...` from the worktree root, plus the task's named check: `go test ./internal/loomengine/... ./internal/fabricengine/... ./internal/preflight/... ./internal/buildinfo/... ./internal/standalonestate/...`, and the integration-tagged runs for the same packages and `cmd/lyx`.
`internal/lyxcwd/docslink_test.go` covers markdown link integrity for the `CONSTRAINTS.md` and `docs/overview.md` edits.

## Q&A log

- **Q:** Where do `CheckID`/`Failure`/`Report` live after the lift? **A:** [auto-pick] `internal/preflight` owns them; `loomengine` type-aliases and const-aliases. **Why:** aliases keep `loomengine.Report` and `preflight.Report` the identical type, so the 13 existing integration tests compile unedited and remain a real regression net.
- **Q:** What entry points does `internal/preflight` export? **A:** [auto-pick] `Check(cwd)`, `CheckResolved(l)`, and `Wired(cwd)`. **Why:** `Check` returning the resolved `*lyxcwd.Location` avoids a second `git rev-parse` per preflight; `CheckResolved` preserves the synthetic-`Location` test seam; `Wired` is the CLI-side predicate.
- **Q:** How does `loomengine` learn "check 3 blocks the seed read" and "geometry short-circuited"? **A:** [auto-pick] derive both from the returned `Report` via a new exported `Report.Has(CheckID)`. **Why:** `Has(CheckFabricReady) || Has(CheckJunction)` is exactly today's `check3BlocksSeed` condition, so the derivation is equivalence-preserving and no orchestrator-specific field rides along on the shared type.
- **Q:** What exactly is the `seedStencils` gate — full tier 2, or something narrower? **A:** [auto-pick] tier 1 plus `fabricengine.Ready` only, as `preflight.Wired`. **Why:** `Ready` is a single `os.Stat` with zero spawns and already closes the plain-repo hole; adding `Clean` would spawn `git status` before every `lyx` command *and* would stop seeding stencils in a dirty hub, a behaviour change nobody asked for.
- **Q:** Does `buildinfo` expose `StencilMode()` as the brief says? **A:** [auto-pick] no — `Channel` + `IsDev()`, with a new `stencilstore.ModeFor(dev bool)` holding the mapping. **Why:** `stencilstore.Mode` is non-stdlib, so returning it would break the stdlib-only leaf the same paragraph of the brief requires, and the leaf property is the load-bearing half.
- **Q:** What is `standalonestate`'s API, and does it create the directory? **A:** [auto-pick] one `Derive(target) (stateDir, hash8, err)`, pure derivation, no `MkdirAll`, over an injectable `derive(goos, env…, target)` seam. **Why:** `runtime.GOOS` is a compile-time constant, so without the seam exactly one of the two `<state>` table rows would ever be exercised in CI.
- **Q:** Which `CONSTRAINTS.md` entries land in this commit? **A:** [auto-pick] two new leaf invariants with mechanical enforcement tests; the three-tier rule is deferred. **Why:** T10's brief explicitly reserves the three-tier invariant and says writing it before T5 ships would pin a model the code does not implement; the leaf claims, by contrast, are only true if enforced.
- **Q:** Do the docs land in the same commit? **A:** [auto-pick] yes — `doc.go` per new package, `docs/overview.md` tree rows, `docs/shared-libs/README.md` entries. **Why:** the project's task-completion rule requires it for a task adding modules; `manifest/roadmap.md` is excluded because it moves per completed wave, not per task.
- **Q:** How is the plain-repo no-op tested, given `seedStencils` returns early under `testing.Testing()`? **A:** [auto-pick] extract `stencilSeedTarget(ctx) (hub, worktree, ok)` and drive that from an integration-tagged test. **Why:** a test against `seedStencils` itself would pass vacuously through the `testing.Testing()` guard and prove nothing about the gate.
- **Q:** Do the tier-1/tier-2 test functions move from `loomengine` to `preflight`? **A:** [auto-pick] no — `loomengine`'s tests stay and are untouched; `preflight` gets additive new ones in an external `package preflight_test`. **Why:** those tests are the only proof the lift changed no behaviour, and `hubforge` imports `fabriccli`, so an in-package fixture test in `preflight` would close a compile cycle later.
