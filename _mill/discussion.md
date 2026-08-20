# Discussion: preflight: split into two Shed rows — a generic one, and loom's own

```yaml
task: 'preflight: split into two Shed rows -- a generic one, and loom''s own'
slug: preflight-loom-agnostic
status: discussing
parent: main
```

## Problem

`internal/loomengine.Preflight` bundles two unrelated things into one function and one `Shed` row.
The first is orchestrator-agnostic: `internal/preflight.Check` validates worktree geometry, worktree-pair cleanliness, and fabric readiness/sync (tiers 1 and 2), and is already a standalone, loom-free package.
The second is `runCheck4` (`internal/loomengine/preflight.go:78-159`), which is entirely loom's own: it stats `_lyx/loom/status.json`, decodes it as `shedengine.Status`, unmarshals its `product` payload against loom's own `Status` shape (`internal/loomengine/status.go:23-27`), and runs `checkCoherence`'s fresh-seed rules (`internal/loomengine/coherence.go`).

The row mechanism is already generic — `Deps.Preflight` is typed as a bare `shedengine.ShedProducer` (`internal/loomshed/loomshed.go:68`), so any producer can back it.
The concrete producer wired in today is not.
`Hardener` will reuse `Publish` and `Finalize` by reference from `internal/landingshed`, but it cannot reuse `loomengine.Preflight` the same way, because half of that function is a check against a file `Hardener` does not own and a product shape it does not have.
`manifest/designs/hardener.md:16,95` already assumes Hardener carries "its own Preflight (sandbox provisioning, live-suite readiness)" — that is the row-2 half, and it only works if the row-1 half is separable.

**Why now:** the `producers standalone` and `Perch → Shed flattening` waves are pushing every reusable producer out of product-specific packages.
This is the last row in loom's list that is structurally shared but implemented as loom-only.

## Scope

**In:**

- A new package `internal/preflightshed` holding the generic row-1 producer: a content-free `shedengine.ShedProducer` wrapping `internal/preflight.Check`, taking `cwd` and nothing else.
- A new row 2 in loom's producer list, `Loom-Preflight`, carrying today's check-4 content as its own independent producer, built inside `internal/loomshed`.
- Retargeting the check-4 logic in `internal/loomengine` from a `*lyxcwd.Location` onto told absolute paths, exported as `CheckSeed`.
- Deleting `loomengine.Preflight`, `loomengine.checkResolved`, `loomengine.runCheck4`, and the `CheckResolvedForTest` export shim — nothing calls them after the split.
- Removing the `check3BlocksSeed` short-circuit and narrowing `CheckSeedUnreadable`'s meaning accordingly.
- Retargeting `checkCoherence`'s `current_producer` and history rules for a two-row world, with the row names told rather than hardcoded.
- Updating `internal/loomcli/wiring.go` to construct row 1 from the new package.
- Test migration: seed-check coverage drops from Tier 2 (integration, real git) to Tier 1 (untagged, `t.TempDir`).
- Doc updates in the same commit: `manifest/designs/loom.md` (producer table), `docs/overview.md`, `CONSTRAINTS.md`'s Told-Geometry Invariant tier list, `contracts/specs/loom-status-spec.md`'s check-4 section, and the stale row-count references listed below.

**Out:**

- Any change to `internal/preflight`'s own check set, `Report` shape, or `report-not-error` contract.
  Row 1 wraps it verbatim.
- Any change to `internal/shedengine`.
  The sequencing this split relies on already exists.
- Building `Hardener`'s own preflight row.
  This task makes row 1 reusable; it does not add a second consumer.
- Renaming row 1.
  It stays `Preflight` — see the decision below.
- `Plan-Sweep`.
  It remains an unbuilt row in the design table only, per `internal/loomshed/loomshed.go:98-102`.
- Any wiki `depends_on` beyond the already-Done `loom: phase-machine scaffolding`.

## Decisions

### row-1-home

- Decision: the generic row-1 producer lives in a new package, `internal/preflightshed`, exposing `NewPreflight(cwd string) shedengine.ShedProducer`.
- Rationale: mirrors `internal/landingshed`'s `<domain>shed` naming for "the `Shed` producers for \<domain\>", which `internal/loomshed` already imports by reference (`loomshed.go:114-121`); `Hardener` imports the new one the same way.
  It keeps `internal/preflight` free of a `shedengine` dependency, which matters because `cmd/lyx`'s pre-run consumes that package for `ResolveMode`.
- Rejected: putting the producer in `internal/preflight` itself (drags `shedengine` into the `cmd/lyx` pre-run path); putting it in `internal/shedadapters` (its package doc pins "no adapter calls lyxcwd, os.Getwd, or git", which a preflight producer transitively violates through `preflight.Check` → `lyxcwd.Resolve` → `git rev-parse`).

### row-2-name

- Decision: row 2 is named `Loom-Preflight`, declared as `NameLoomPreflight` beside the existing name constants in `internal/loomshed/loomshed.go:23-36`.
- Rationale: the roadmap's own suggestion, and it parallels `manifest/designs/hardener.md`'s "Hardener's own Preflight".
  The name is the durable on-disk identity in `current_producer`, so it is chosen once and not revisited.
- Rejected: `Loom-Seed` and `Seed-Check` — both describe the artifact rather than the tier, and neither matches how `hardener.md` already talks about its counterpart row.

### why-the-name-needs-a-constant

- Decision: `NameLoomPreflight` is a constant, not a repeated literal.
- Rationale: `loomshed.go:15-17` already states the rule and the reason — the same string has to appear at several sites that must agree, and Go gives no cross-file check on string literals.
  For this name the sites are the row definition in `New`, and the two `CheckSeed` arguments described under `coherence-names-are-told` below.
  The constant does not make a rename safe (a rename still breaks every in-flight task's on-disk `current_producer`); it makes all spellings change together or fail the build.
- Rejected: an inline literal — defensible for a name used once, which this is not.

### row-2-paths-are-told

- Decision: the check-4 logic is retargeted from `*lyxcwd.Location` onto told absolute paths and exported as `loomengine.CheckSeed(statusPath, statusLockPath string, ...) (Report, error)`.
  `internal/loomshed` builds row 2 from `deps.StatusPath` and `deps.StatusLockPath`.
- Rationale: those two `Deps` fields already hold exactly the values `LoomStatusFile(l)` and `LoomStatusLock(l)` return — `internal/loomcli/wiring.go:88-90` fills them from those same accessors.
  Telling the paths keeps `internal/loomshed` inside its Told-Geometry membership with no new allowlist entry, and it makes the seed checks pure file I/O over a caller-supplied directory, which is what enables the Tier-1 migration below.
- Rejected: passing a `*lyxcwd.Location` into row 2's producer — that would force `internal/loomshed` to take a direct production import of `internal/lyxcwd` and break `TestToldGeometryInvariant_AllowlistOnly` (`internal/loomshed/seam_enforcement_test.go`).

### delete-the-composite

- Decision: `loomengine.Preflight`, `checkResolved`, `runCheck4`, and `internal/loomengine/export_test.go`'s `CheckResolvedForTest` are deleted outright.
- Rationale: after the split nothing calls them.
  `internal/loomcli` reaches preflight only through the producer constructor (`wiring.go:99`), and row 1 now calls `preflight.Check` directly.
  A composite carrying the old bundled semantics would be a second, divergent path with no consumer.
- Rejected: keeping `Preflight` as a thin deprecated composite.

### coherence-names-are-told

- Decision: `CheckSeed` takes the expected `current_producer` and the tolerated history-producer set from its caller.
  `internal/loomshed` passes `NameLoomPreflight` and `{NamePreflight, NameLoomPreflight}`.
- Rationale: `checkCoherence` currently hardcodes the literal `"Preflight"` twice (`coherence.go:41` and `coherence.go:91`), duplicating `loomshed`'s own constant across a package boundary with nothing enforcing agreement.
  The split doubles the exposure — two names to keep in sync instead of one — so this is the moment to collapse it to a single source of truth.
- Rejected: declaring the names again as `loomengine` constants.

### coherence-rules-after-the-split

- Decision: the two rules become "`current_producer` must equal the told expected name" and "a history entry naming a producer outside the told tolerated set is `CheckHalfFinished`".
- Rationale: this is forced by `shedengine`'s existing sequencing, not a preference.
  `Run` persists the *next* row's name into `current_producer` and appends the finished row's history entry **before** calling that next row (`internal/shedengine/run.go:223-244`).
  So at the instant row 2's producer runs, the file reads `current_producer: "Loom-Preflight"` with `history: [{"producer": "Preflight", "outcome": "done"}]`.
  Both of today's rules reject that shape, so leaving them unchanged fails every single run.
  Row 2's own name stays in the tolerated set for exactly the reason `coherence.go:84-89` already documents for row 1: `Run` appends a history entry before persisting `StateBlocked` on the `OnStuck: ""` escalation path, so a `Stuck` at row 2 leaves a `Loom-Preflight` entry behind and a resumable blocked run must not fail this check forever.
- Rejected: nothing credible — the alternative is a broken orchestrator.

### drop-check3blockseed

- Decision: the `check3BlocksSeed` derivation and its branch are removed (`internal/loomengine/preflight.go:79-101`).
  A stat failure that is `os.IsNotExist` is `CheckSeedMissing` unconditionally; any other stat failure is `CheckSeedUnreadable`.
- Rationale: `shedengine`'s own sequencing already provides what the short-circuit was hand-rolling.
  Row 1 returning `Stuck` with `OnStuck: ""` persists `StateBlocked` and returns `RunBlocked` (`run.go:187-200`); `Shed` never advances to row 2 at all.
  So row 2 can assume tiers 1–3 passed rather than re-deriving whether its own failure is a downstream consequence of an earlier one.
- Rejected: keeping the derivation — it would read a tier-1/2 report row 2 no longer receives.

### checkseedunreadable-doc-narrows

- Decision: `CheckSeedUnreadable`'s doc comment (`internal/loomengine/report.go:51-55`) drops its second clause, "or when a stat failure (including not-exist) is attributable to fabric not being ready or healthy".
  `contracts/specs/loom-status-spec.md`'s check-4 section gets the same narrowing.
- Rationale: that clause described the `check3BlocksSeed` branch exactly, and it becomes unreachable.
- Rejected: leaving the doc as-is — it would describe behaviour the code no longer has, which the Documentation Lifecycle forbids in the same commit.

### row-2-onstuck-escalates

- Decision: row 2 is defined with `OnStuck: ""`.
- Rationale: same posture as row 1 and for the same reason `loomshed.go:90-96` gives — no row in the list produces `_lyx/loom/status.json`, so there is nothing to bounce to and a human is the only thing that can fix an incoherent or half-finished seed.
- Rejected: bouncing to row 1 — re-running tier-1/2 checks cannot repair a seed file.

### no-new-deps-field

- Decision: `loomshed.Deps` gains no field for row 2.
  `New` builds row 2 internally from `deps.StatusPath` and `deps.StatusLockPath`, the way it already builds `Discussion-Validate`, `Plan-Validate`, and `Batchifier` from told values.
  `Deps.Preflight` stays as the injected row-1 seam.
- Rationale: `Deps.Preflight` is injected pre-constructed because row 1 is the one row that spawns git (`loomshed.go:67`) and because a Tier-1 test needs to substitute a fake.
  Row 2 spawns nothing and reads one JSON file under a caller-supplied path, so it has neither justification.
- Rejected: a second injected field — it would double the wiring surface for a producer with no external dependency.

### row-1-keeps-the-name-preflight

- Decision: row 1's producer name remains the string `Preflight`.
- Rationale: it is the durable on-disk identity a fresh seed is written with — `internal/loomshed/seed.go:57` sets `CurrentProducer: NamePreflight`, and `contracts/specs/loom-status-spec.md:33,39` pins it as part of the seed contract.
  Renaming it would break every in-flight task's resume at `run.go:110` and force a spec revision for no gain.
- Rejected: renaming row 1 to something like `Fabric-Preflight` to signal its new generic status — the name is already generic; only the implementation was not.

### file-layout

- Decision: `internal/loomshed/preflight.go` is repurposed as row 2's home and renamed `loompreflight.go`; row 1's wrapper (`preflightProducer`, `NewPreflightProducer`) moves out to `internal/preflightshed`.
- Rationale: keeps one producer per file in `loomshed`, matching `batchifier.go` / `planvalidate.go` / `discussionvalidate.go` / `webster.go`.
- Rejected: keeping both producers in one file.

### row-count-reconciliation

- Decision: the code's producer list goes from **12 rows to 13**, not 13 to 14 as the roadmap item states; `manifest/designs/loom.md`'s producer table goes from 13 rows to 14 with rows 2–13 renumbered to 3–14.
  The three already-stale references are corrected in the same commit.
- Rationale: `loomshed.New` builds 12 rows today (`loomshed.go:123-136`), and `loomshed.go:1,82,85`, `stub.go:2,12`, and `sequence_test.go:33` all say so correctly.
  The roadmap's "13" counts `loom.md`'s design table, which carries an unbuilt `Plan-Sweep` at row 5 (`loomshed.go:98-102` explains why it is not a code row).
  The two counts are legitimately different and must be moved separately.
- Rejected: taking the roadmap's numbers literally — it would introduce three wrong counts rather than fixing the existing ones.

### stale-row-count-references

- Decision: these three pre-existing errors are fixed alongside the split.
  - `internal/loomshed/loomshed_test.go:144` — "the thirteen rows" → thirteen is correct only *after* this task; today it should read twelve, so it lands directly at the new count.
  - `internal/loomcli/smoke_test.go:21` — "eight of its thirteen rows with stub producers" is doubly wrong: `stub.go:12` says stubs back **five** rows, and the list is 12 today, 13 after.
  - `docs/overview.md:237` — "loom's own 13-row producer list" → 13 is correct only after this task.
- Rationale: the Documentation Lifecycle requires the module doc and `docs/overview.md` to move with the change; leaving a comment that is coincidentally right for the wrong reason is worse than either state.
- Rejected: deferring them to a separate cleanup task.

### constraints-tier-3-retarget

- Decision: `CONSTRAINTS.md:64`'s tier-3 bullet, currently "`loomengine.Preflight` — tiers 1+2 plus the orchestrator's own status seed", is retargeted to name `loomengine.CheckSeed` and to state that tier 3 is now a **separate producer row** from tier 2 rather than a function composing it.
- Rationale: the Told-Geometry Invariant's three-tier list names a function that this task deletes.
  A cross-cutting invariant naming a deleted symbol is exactly what the Documentation Lifecycle's same-commit rule exists to prevent.
- Rejected: leaving the bullet — it would name a symbol that no longer exists.

### no-told-geometry-guard-on-preflightshed

- Decision: `internal/preflightshed` gets no `seam_enforcement_test.go` import allowlist, and is not added to the Told-Geometry Invariant's machine-enforced or review-obligation lists.
  Its `doc.go` states its tier-2 position instead, mirroring `internal/preflight/doc.go`'s "Told-geometry tier" paragraph.
- Rationale: the invariant's membership predicate is "takes its absolute paths from its caller and has no direct production import of `internal/lyxcwd`".
  `preflightshed` deliberately resolves geometry — that is what `preflight.Check(cwd)` does — so it is a tier-2 resolver, not a told package.
  Claiming membership would be false.
- Rejected: adding the allowlist test — it would either fail immediately or have to allowlist the very import the invariant exists to exclude.

### preflightshed-ctx-helper-shape

- Decision: `internal/preflightshed`'s context helpers take the two-argument form `entryErr(ctx, name)` / `cancelErr(ctx, name)`, modelled on `internal/loomshed/ctx.go`, not `internal/shedadapters/ctx.go`'s three-argument form with an engine label.
- Rationale: `shedadapters` carries the engine label because one package wraps three different engines (`shuttle`, `perch`, `Webster`) and its error text has to say which one failed.
  `preflightshed` wraps exactly one thing, `preflight.Check`, and will keep wrapping one thing — a label whose value is constant across every call site is noise in the error string.
  The two-function entry/exit split itself is shared by both files and is what gets copied.
- Rejected: the three-argument form — it would carry a permanently-constant argument, and this codebase treats a parameter with one possible value as a design smell rather than future-proofing.

### test-tier-migration

- Decision: seed-check coverage moves from Tier 2 to Tier 1.
  `internal/loomengine/preflight_integration_test.go` (655 lines, `//go:build integration`, `hubforge.NewHub`) is retired; its three seed cases become untagged tests over `t.TempDir`, and its two tier-1/2 cases with no `internal/preflight` equivalent are migrated there.
- Rationale: once `CheckSeed` takes told paths it performs no git spawn and needs no hub — it stats a file, `MkdirAll`s a lock parent, and decodes JSON.
  Per `docs/benchmarks/running-tests.md:13`, that belongs in the default offline loop.
  Keeping it behind the integration tag would pay a real-hub fixture cost for a pure JSON decode.
- Rejected: leaving the file tagged — a needless Tier-2 cost, and the Test Tier Purity Invariant's direction of travel is the opposite.

## Technical context

**The sequencing fact the whole split rests on.**
`(*Shed).Run` (`internal/shedengine/run.go:36-260`) is a six-step loop.
On `Done` at a non-final row it computes the next row's name, then makes **one** persist call writing `current_producer = nextName`, `state = running`, and the appended history entry (`run.go:223-244`), then loops and calls that next row.
On `Stuck` with `OnStuck: ""` it appends the history entry, persists `StateBlocked`, and returns `RunBlocked` (`run.go:187-200`) — the loop ends, so no later row is called.
Both facts are load-bearing: the first dictates the new coherence rules, the second is what makes `check3BlocksSeed` redundant.

**Files that change.**

- `internal/loomengine/preflight.go` — `Preflight`, `checkResolved`, `runCheck4` deleted; `CheckSeed` added in their place (or in a new `seed.go`, planner's call).
  It keeps the `os.MkdirAll(filepath.Dir(lockPath), 0o755)` guard at `preflight.go:114-116` verbatim, including its comment: `internal/lock` opens with `O_CREATE` but never creates parents, and without it a missing-`.lyx` worktree escalates to a hard infra error instead of honouring the report-not-error contract.
  It also keeps the TOCTOU guard at `preflight.go:128-134` (stat succeeded but `ReadJSONStrict` reports not-found → synthesize a non-nil error, never `Report{}, nil`).
- `internal/loomengine/coherence.go` — `checkCoherence` gains the two told-name parameters; `coherence.go:41` and `coherence.go:91` lose their literals.
- `internal/loomengine/report.go:51-55` — `CheckSeedUnreadable` doc narrowed.
- `internal/loomengine/export_test.go` — deleted.
- `internal/preflightshed/` — new package: `doc.go`, the producer, and its two-argument `entryErr(ctx, name)` / `cancelErr(ctx, name)` helpers copied from `internal/loomshed/ctx.go` rather than `internal/shedadapters/ctx.go:14-30`'s three-argument form — see the `preflightshed-ctx-helper-shape` decision.
- `internal/loomshed/loomshed.go` — `NameLoomPreflight` constant; row 2 inserted after row 1 in `New`'s `producers` slice; the doc comments at lines 1, 15, 82, 85-96 updated for 13 rows and the new row's backing and `OnStuck`.
- `internal/loomshed/preflight.go` → `loompreflight.go` — row 2's producer, mapping `CheckSeed`'s `Report` onto `Done`/`Stuck`/error exactly as the existing wrapper maps `Preflight`'s (`preflight.go:44-65`).
- `internal/loomshed/stub.go:2,12` — row-count wording.
- `internal/loomcli/wiring.go:96-99` — `Preflight:` now takes `preflightshed.NewPreflight(cwd)`; the comment explaining why the adapter and not the bare function is used needs rewording, since the bare function is gone.

**Tests that change.**

- `internal/loomshed/loomshed_test.go:27-40` `wantProducerTable` — insert `{NameLoomPreflight, ""}` at index 1; the comment at line 144 fixed.
- `internal/loomshed/sequence_test.go:20-32` `wantSequenceOrder` — insert `NameLoomPreflight`; the row numbering in its doc comment and in `TestSequence_FullRunBlocksAtPublish`'s comment (lines 15-18, 33-35) shifts by one (rows 1–12 run, blocking at `Publish` which is now row 12, `Finalize` row 13).
  Note the sequence fixture's stub row 2 must return `Done` for the run to reach `Publish`.
- `internal/loomshed/fixture_test.go` `testDeps` — already supplies `StatusPath`/`StatusLockPath` under a `t.TempDir` (`loomshed_test.go:42-47`), but a *real* row 2 in the list means the fixture's status file must now be a coherent fresh seed rather than whatever the stub-backed run needed.
  This is the likeliest hidden cost in the whole task: `buildSequenceFixture` and `testDeps` may need `loomshed.Seed` called against them.
- `internal/loomshed/preflight_integration_test.go` — its two tests drive `loomshed.NewPreflightProducer`, which moves; the file either moves to `internal/preflightshed` or is rewritten against row 2.
  Its header comment explains why it is an external-package `loomshed_test` file (an in-package test importing `hubforge` would close a compile cycle through `internal/loomengine`); that reasoning must be re-evaluated for wherever it lands.
- `internal/loomengine/coherence_test.go:41` `TestCheckCoherence` — table-driven, gains the told-name arguments and cases for the new two-name tolerated set.
- `internal/loomengine/preflight_integration_test.go` — retired per the decision above.
  Case-by-case disposition: `TestPreflight_HealthyPairAndSeed`, `_NotAGitRepo`, `_SubpathAnchoredHubIsNotRejectedForItsAnchor`, `_WarpDirty`, `_FabricNotReady`, `_WarpFabricDifferentBranches`, `_JunctionBroken`, `_MultipleSimultaneousFailures` all have direct equivalents in `internal/preflight/preflight_integration_test.go` (`TestCheckResolved_HealthyPair`, `TestCheck_NotAGitRepo`, `TestCheck_SubpathAnchoredHubIsNotRejected`, `TestCheckResolved_Dirty`, `TestCheckResolved_FabricNotReady`, `TestCheckResolved_BranchMismatch`, `TestCheckResolved_BrokenJunction`, `TestCheckResolved_MultipleSimultaneousFailures`) and are dropped.
  `TestPreflight_ConfigLoadFailed` and `TestPreflight_MissingOptionalJunctionIsAJunctionFault` have **no** equivalent there — the planner must verify this and migrate them into `internal/preflight/preflight_integration_test.go` rather than deleting them.
  `TestPreflight_SeedMissing`, `_SeedUnknownField`, `_SeedHalfFinished` become Tier-1 tests against `CheckSeed`.
- `internal/loomcli/smoke_test.go:21` — comment corrected.
- `internal/loomcli/wiring_test.go` — asserts the assembled `Deps`; check whether it names the row-1 constructor.

**Docs that change** (Documentation Lifecycle, same commit):

- `manifest/designs/loom.md:29-41` — the producer table gains `Loom-Preflight` as row 2, renumbering rows 2–13 to 3–14; `loom.md:43-46`'s paragraph about `Preflight` being built as `internal/loomengine.Preflight` and validating "the four preconditions" is rewritten for the two-row shape; `loom.md:241`'s module-decomposition row likewise.
- `docs/overview.md:237` — row count; a new tree line for `internal/preflightshed` beside `internal/landingshed` (line 238) and `internal/preflight` (line 242); `overview.md:336`'s precondition-layer paragraph mentions the new package.
- `CONSTRAINTS.md:64` — tier-3 bullet, per the decision above.
- `contracts/specs/loom-status-spec.md:31,77,82` — the check-4 section: line 77's "`shed.current_producer` must equal `"Preflight"` — the only way check 4 is ever reached" becomes the row-2 name, and line 82's fresh-start rule gains the second tolerated name.
  Line 33's *seed* shape is unchanged — a fresh seed still carries `current_producer: "Preflight"`.
- `manifest/roadmap.md:66-71` — the Planned item moves to Done; its "13 rows to 14" wording is superseded by this discussion's reconciliation and should not be copied forward.

## Constraints

From `CONSTRAINTS.md`:

- **Told-Geometry Invariant.** `internal/loomshed` is machine-enforced via `seam_enforcement_test.go`'s `TestToldGeometryInvariant_AllowlistOnly`; its allowlist is `shedengine`, `shedadapters`, `websterengine`, `loomengine`, `planparser`, `batcher`, `state`, `landingshed`.
  Row 2 must be buildable without adding to it — which the told-paths decision guarantees, since `loomengine` is already on the list.
  The invariant's three-tier list at lines 62-64 is itself edited by this task.
- **Cwd Resolution Invariant.** A module's own durable subdirectory is that module's private relative-path constant joined onto `AnchorPath()`, never a `lyxcwd` call.
  `LoomStatusFile`/`LoomStatusLock`/`LoomRunLock` (`internal/loomengine/config.go:62-118`) stay exactly as they are; only their *consumers* change.
- **Durable-vs-Ephemeral State Invariant.** `_lyx/loom/status.json` is durable; its `.lock` sidecar lives at the mirrored subpath under `.lyx`.
  This is why the `MkdirAll` before locking exists and must survive the move.
- **Test Tier Purity Invariant** (line 487).
  An untagged test file must not call `gitexec.Run`, `exec.Command`, `gitkit.Copy*`, or `hubforge.NewHub`.
  The new Tier-1 seed tests must touch none of those — `t.TempDir` plus `os.WriteFile` only.
- **Documentation Lifecycle.** Every doc listed above moves in the same commit as the code.
- **CLI/Cobra Invariant.** Untouched — this task adds no command.
- **Fabric Vocabulary Invariant.** `internal/preflightshed` describes one repository; if it falls in the owner set it must not name either fabric-internal side.
  `internal/landingshed/doc.go:40-43` documents the equivalent position and is the model.

Project rules from `CLAUDE.md`: markdown uses semantic line breaks, never fixed-column wrapping; docs land in the same commit as the code.

## Testing

**Verify command:** `go test ./... -count=1` followed by `go test -tags integration ./... -count=1`.
Both must pass; the second is where the fixture-shape risk in `loomshed` and the migrated `internal/preflight` cases surface.

**TDD candidates** — write the test first, in this order:

1. `internal/loomengine` `TestCheckCoherence` (Tier 1, extend the existing table at `coherence_test.go:41`).
   The rule change is pure and in-memory, so the table can be extended to red before `checkCoherence`'s signature changes.
   Scenarios that must be covered: `current_producer` equal to the told expected name passes; equal to row 1's name now **fails**; a history containing only a row-1 `Done` entry passes; a history containing only row-2 entries passes; a history mixing both passes; a history naming any third producer is `CheckHalfFinished`; the existing mandatory-field, state, outcome-validity, and RFC3339 cases still hold unchanged.
2. `internal/loomengine` seed-check tests (Tier 1, new, replacing the retired integration cases).
   Scenarios: file absent → `CheckSeedMissing`; file present but undecodable / carrying an unknown field → `CheckSeedIncoherent`; a `product` that does not unmarshal as loom's `Status` → `CheckSeedIncoherent` with the "product does not decode" reason; a coherent fresh seed → `OK` true; a lock-parent directory that does not exist → still a determined verdict, not an infra error (this is the `MkdirAll` guard's regression test, and it has no Tier-2 equivalent today).
3. `internal/loomshed` `TestNew_ProducerTable` and `TestSequence_FullRunBlocksAtPublish` (Tier 1).
   Row 2 present at index 1 with `OnStuck: ""`; the full 13-row order; a clean run reaching `Publish` and blocking there.
4. `internal/preflightshed` producer tests.
   Tier 1 for the contract shape — a fake is not possible here since the producer calls `preflight.Check` directly, so the outcome-mapping coverage is Tier 2 against a `hubforge` hub, inherited from the two existing tests in `internal/loomshed/preflight_integration_test.go`: all preconditions pass → `Done`; an untracked file in the warp worktree → `Stuck`.
   Add the context-cancellation cases the `shedadapters` adapters each carry: cancelled at entry → error without starting; cancelled during → error on the non-success path.
5. `internal/loomshed` row-2 producer test (Tier 1, new).
   Outcome mapping only, over a `t.TempDir` status file: coherent seed → `Done`; incoherent seed → `Stuck`; unreadable-for-infra-reasons → error.
   Deliberately **not** a re-test of the check set — that is `loomengine`'s job, and duplicating it would couple this package's tests to another package's checks, the same reasoning `internal/loomshed/preflight_integration_test.go:10-12` already records.

**Scenarios that must not regress:**

- A `Stuck` at row 1 leaves exactly one `Preflight` history entry and stays resumable — re-running must not fail `CheckHalfFinished`.
- A `Stuck` at row 2 leaves a `Preflight` `Done` entry plus a `Loom-Preflight` `Stuck` entry and stays resumable for the same reason.
- A broken fabric blocks at row 1 and never reaches row 2, so no phantom seed verdict is ever reported (this is what replaces `check3BlocksSeed`).
- `internal/loomcli`'s bootstrap smoke path still seeds, spawns, and reaches a blocked state — `smoke_test.go`'s `TestSmokeBootstrap_CleanlinessOrderingAfterSeedCommit` is the specific regression home for loom's own seed dirtying the weft and failing loom's own first precondition row, and inserting a row changes which row that is.

**Known risk the planner must confirm before writing batches:** whether `internal/loomshed`'s Tier-1 fixtures (`testDeps`, `buildSequenceFixture`) currently write a status file coherent enough for a *real* row 2 to pass.
Today row 2 does not exist and row 1 is a fake in those fixtures, so the seed may be minimal.
If it is, every Tier-1 `loomshed` test that runs the list needs `loomshed.Seed` against its temp dir.

## Q&A log

- **Q:** Where does the generic row-1 producer live — a new `internal/preflightshed`, `internal/preflight` itself, or `internal/shedadapters`? **A:** [auto-pick] New package `internal/preflightshed`. **Why:** mirrors `internal/landingshed`'s `<domain>shed` naming for shared-by-reference `Shed` producers, keeps `shedengine` out of the `cmd/lyx` pre-run path, and avoids contradicting `shedadapters`' documented "no adapter calls lyxcwd, os.Getwd, or git" contract.
- **Q:** What is row 2's durable name? **A:** [auto-pick] `Loom-Preflight`. **Why:** the roadmap's own suggestion, and it parallels `hardener.md`'s "Hardener's own Preflight"; the alternatives name the artifact rather than the tier.
- **Q:** How does row 2 get its paths — told strings, or a `*lyxcwd.Location`? **A:** [auto-pick] Told strings via `loomengine.CheckSeed(statusPath, statusLockPath, ...)`. **Why:** `deps.StatusPath`/`deps.StatusLockPath` already hold those exact values, it keeps `loomshed` inside its Told-Geometry allowlist, and it is what makes the seed checks Tier-1 testable.
- **Q:** What happens to `loomengine.Preflight` and `checkResolved`? **A:** [auto-pick] Delete both, plus `runCheck4` and `CheckResolvedForTest`. **Why:** nothing calls them after the split, and a composite with no consumer is a second divergent path.
- **Q:** Are the coherence rules' producer names told by the caller or hardcoded in `loomengine`? **A:** [auto-pick] Told. **Why:** `coherence.go:41,91` already duplicate `loomshed`'s name constant across a package boundary with nothing enforcing agreement, and the split doubles the exposure.
- **Q:** Is `check3BlocksSeed` removed? **A:** [auto-pick] Yes, and `CheckSeedUnreadable` narrows to genuine stat/read failures. **Why:** `shedengine` blocks at row 1 on `Stuck` with `OnStuck: ""` and never advances, so row 2 can assume tiers 1–3 passed.
- **Q:** How are the row counts reconciled, given the roadmap says 13→14? **A:** [auto-pick] Code goes 12→13, the design table 13→14, and the three already-stale references are fixed in the same commit. **Why:** `loomshed.New` builds 12 rows today; the roadmap's 13 counts the design table's unbuilt `Plan-Sweep` row. The two counts are legitimately different.
- **Q:** Do the seed checks stay behind the integration tag? **A:** [auto-pick] No — they move to Tier 1. **Why:** with told paths the check spawns no git and needs no hub; it stats a file and decodes JSON.
- **Q:** What is row 2's `OnStuck`? **A:** [auto-pick] `""` (escalate). **Why:** no row in the list produces `status.json`, so there is nothing to bounce to.
- **Q:** Where does the new row sit in `loom.md`'s design table? **A:** [auto-pick] Row 2, renumbering rows 2–13 to 3–14. **Why:** it runs immediately after row 1 and before `Discussion-Write`.
- **Q:** Does row 1 keep the name `Preflight`? **A:** [auto-pick] Yes. **Why:** it is the durable seed identity pinned by `seed.go:57` and `loom-status-spec.md:33`; renaming breaks every in-flight resume for no gain.
- **Q:** Does `loomshed.Deps` gain a field for row 2? **A:** [auto-pick] No — `New` builds it internally from told values. **Why:** row 2 spawns nothing and needs no test-injection seam, unlike row 1.
- **Q:** What happens to `internal/loomshed/preflight.go`? **A:** [auto-pick] Repurposed as row 2's home, renamed `loompreflight.go`. **Why:** one producer per file, matching the package's existing layout.
- **Q:** What is the verify command? **A:** [auto-pick] `go test ./... -count=1` then `go test -tags integration ./... -count=1`. **Why:** the repo's two standard tiers; the split touches both.
- **Q:** Does `internal/preflightshed` get a Told-Geometry allowlist test? **A:** [auto-pick] No. **Why:** it deliberately resolves geometry through `preflight.Check`, so it is a tier-2 resolver, not a told package; claiming membership would be false.
- **Q:** Which context-helper shape does `internal/preflightshed` use — `shedadapters`' three-argument form with an engine label, or `loomshed`'s two-argument form? **A:** [auto-pick] The two-argument form. **Why:** `shedadapters` carries the label because one package wraps three engines; `preflightshed` wraps exactly one, so the label would be a permanently-constant argument. Raised by the orchestrator review as the one unpinned ambiguity.
- **Q:** Does `CONSTRAINTS.md` change? **A:** [auto-pick] Yes — line 64's tier-3 bullet retargets from `loomengine.Preflight` to `loomengine.CheckSeed` and states that tier 3 is now a separate producer row. **Why:** the invariant currently names a symbol this task deletes.
