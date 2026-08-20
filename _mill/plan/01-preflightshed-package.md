# Batch: preflightshed-package

```yaml
task: 'preflight: split into two Shed rows -- a generic one, and loom''s own'
batch: 'preflightshed-package'
number: 1
cards: 6
verify: go test ./... -count=1 && go test -tags integration ./... -count=1 && go vet -tags smoke ./internal/loomcli
depends-on: []
```

## Rename mechanic

_For each `Moves:` pair the implementer MUST:_

1. _Run `git mv <old> <new>` FIRST, before making any other change to the moved file._
2. _Make ONLY surgical edits — touch only the lines that must change after the move (package or module declaration, imports, identifier retargeting, seam splits)._
3. _Use a full-file `Creates:` entry only for genuinely new files that have no predecessor._
4. _Never write the relocated file from scratch and delete the original — that breaks git rename history and inflates review diffs._

## Batch Scope

This batch delivers `internal/preflightshed`, the new product-agnostic package holding the generic row-1 `Shed` producer: a content-free `shedengine.ShedProducer` wrapping `internal/preflight.Check`, constructed as `NewPreflight(name, cwd string)`.
Nothing outside the new package's own directory changes except the Tier-2 test that moves into it from `internal/loomshed`, so this batch is independent of batch 2 and leaves the whole repo compiling and green.
`internal/loomshed.NewPreflightProducer` deliberately survives this batch untouched — batch 3 is what retargets `internal/loomcli` onto the new constructor and deletes the old one.
The external interface batch 3 consumes is exactly one exported symbol: `preflightshed.NewPreflight(name, cwd string) shedengine.ShedProducer`.

Batch-local decision: the row-2 half of the split (`loomengine.CheckSeed`, `NameLoomPreflight`) is entirely out of this batch's scope.
Nothing here reads or writes `_lyx/loom/status.json`, and that is the point of the split — row 1's own test fixture must not depend on loom's status contract.

## Cards

### Card 1: preflightshed package doc

- **Context:**
  - `internal/preflight/doc.go`
  - `internal/landingshed/doc.go`
  - `internal/landingshed/ctx.go`
  - `internal/loomshed/doc.go`
  - `CONSTRAINTS.md`
- **Edits:** none
- **Creates:**
  - `internal/preflightshed/doc.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Write the `package preflightshed` doc comment in a `doc.go` holding nothing but the package clause and its doc.
  State four things.
  (1) The package owns the general `Preflight` producer — a content-free `shedengine.ShedProducer` wrapping `internal/preflight.Check` — which any producer list may name, modelled on `internal/landingshed`'s framing of `Publish`/`Finalize` as producers "shared by reference" rather than owned by one product.
  (2) Its told-geometry tier: it is a **tier-2 resolver, not a told package**.
  It deliberately resolves geometry, because that is what `preflight.Check(cwd)` does, so it takes no absolute paths from its caller beyond the cwd it is told to resolve, and it is neither machine-enforced nor a review obligation under the Told-Geometry Invariant's membership predicate (which requires taking absolute paths from the caller and having no direct production import of `internal/lyxcwd`).
  Mirror the shape of `internal/preflight/doc.go`'s own "Told-geometry tier" paragraph and cross-reference `CONSTRAINTS.md`'s Told-Geometry Invariant.
  Say outright that claiming membership would be false, so this package gets no `seam_enforcement_test.go` import allowlist.
  (3) It declares its own unexported `entryErr`/`cancelErr` helpers for the same deliberate-duplication reason `internal/loomshed/doc.go` and `internal/landingshed/ctx.go` already record.
  (4) Its Fabric Vocabulary Invariant position, modelled on `internal/landingshed/doc.go`'s closing paragraph: this package describes one repository, it is not in the invariant's owner set, so none of its identifiers, string literals, or comments may name either fabric-internal side, and that ban is machine-enforced by `internal/lyxcwd`'s `TestEnforcement_FabricVocabulary`.
  Do not name either fabric-internal side anywhere in this file — see the `fabric-vocabulary-ban-on-the-new-package` Shared Decision.
  Follow the one-clause-per-line Go comment style visible in every cited file.
- **Commit:** `docs(preflightshed): add the package doc for the general Preflight producer`

### Card 2: preflightshed context helpers

- **Context:**
  - `internal/loomshed/ctx.go`
  - `internal/landingshed/ctx.go`
  - `internal/shedadapters/ctx.go`
- **Edits:** none
- **Creates:**
  - `internal/preflightshed/ctx.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/preflightshed/ctx.go` declaring exactly two unexported functions, copied from `internal/loomshed/ctx.go`'s **two-argument** shape — `entryErr(ctx context.Context, name string) error` and `cancelErr(ctx context.Context, name string) error` — with the package prefix in both error strings changed from `loomshed:` to `preflightshed:`.
  Do not adopt `internal/shedadapters/ctx.go`'s three-argument form carrying an engine label: `preflightshed` wraps exactly one thing, `preflight.Check`, so the label would be a permanently-constant argument.
  Carry over both doc comments and the file header comment, retargeted at this package (the header states that this is preflightshed's own copy of the identically-shaped helpers, deliberate duplication, see `doc.go`).
  Keep `entryErr`'s "context cancelled before run started" and `cancelErr`'s "context cancelled during run" wordings verbatim, and keep `cancelErr`'s doc paragraph explaining that a `Stuck` returned under a cancelled context is indistinguishable to `Shed` from a genuine verdict.
- **Commit:** `feat(preflightshed): add the entryErr/cancelErr context helpers`

### Card 3: the generic Preflight producer

- **Context:**
  - `internal/loomshed/preflight.go`
  - `internal/preflight/preflight.go`
  - `internal/preflight/report.go`
  - `internal/shedengine/producer.go`
  - `internal/shedadapters/doc.go`
  - `internal/preflightshed/ctx.go`
- **Edits:** none
- **Creates:**
  - `internal/preflightshed/preflight.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/preflightshed/preflight.go` declaring an unexported struct `preflightProducer` with two string fields, `name` and `cwd`, a `var _ shedengine.ShedProducer = (*preflightProducer)(nil)` compile-time assertion, an exported constructor `NewPreflight(name, cwd string) shedengine.ShedProducer` returning `&preflightProducer{name: name, cwd: cwd}`, and a `Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error)` method.
  `Call` reproduces `internal/loomshed/preflight.go`'s existing `Call` body exactly, with one change: it calls `preflight.Check(p.cwd)`, which returns three values `(Report, *lyxcwd.Location, error)`, and discards the `*lyxcwd.Location` with `_` — this package never touches the resolved location.
  The four exit paths stay identical to the existing wrapper's: `entryErr` before anything starts; a non-nil `Check` error consulting `cancelErr` first and otherwise returning the error; a `!report.OK` returning `shedengine.Stuck` after consulting `cancelErr`; and `shedengine.Done` otherwise.
  `NewPreflight`'s doc comment must state that `name` is told rather than hardcoded because a second product names this row independently, and that per `internal/shedadapters`' own package doc the name is used only as a log field and in error text — never compared, parsed, or used for control flow.
  Do not declare a package-private name constant mirroring `internal/landingshed`'s `publishName`.
  Do not import `internal/loomengine`, `internal/loomshed`, or `internal/lyxcwd`.
  Do not name either fabric-internal side in any comment or literal — see the `fabric-vocabulary-ban-on-the-new-package` Shared Decision.
- **Commit:** `feat(preflightshed): add NewPreflight, the general Preflight ShedProducer`

### Card 4: Tier-1 producer tests

- **Context:**
  - `internal/preflightshed/preflight.go`
  - `internal/preflightshed/ctx.go`
  - `internal/loomshed/resume_test.go`
  - `internal/shedengine/producer.go`
  - `CONSTRAINTS.md`
- **Edits:** none
- **Creates:**
  - `internal/preflightshed/preflight_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create an **untagged** (Tier-1) `package preflightshed` test file with two tests.
  `TestNewPreflight_CarriesToldName` asserts `NewPreflight("Some-Row", "/some/cwd")` returns a non-nil `shedengine.ShedProducer` whose concrete type is `*preflightProducer` and whose `name` and `cwd` fields hold the told values verbatim — proving the constructor hardcodes nothing.
  `TestCall_CancelledAtEntryReturnsErrorNotStuck` calls `Call` on an already-cancelled `context.Context` and asserts the returned error is non-nil and the returned outcome is neither `shedengine.Done` nor `shedengine.Stuck` — the same assertion shape `internal/loomshed/resume_test.go`'s `TestCancellation_RealProducersReturnErrorNotStuck` uses.
  This case is Tier-1-legal precisely because `entryErr` returns before `preflight.Check` is ever reached, so nothing spawns; pass a `cwd` of `t.TempDir()` so the test would not accidentally resolve a real repository even if that guarantee ever broke.
  The cancelled-**during** case is deliberately not here — it is Tier 2, card 5, because `preflight.Check` reaches `lyxcwd.Resolve`'s git spawn unconditionally.
  This file must contain none of the substrings named in the `untagged-tests-carry-no-spawn-token` Shared Decision, in code or in comments.
- **Commit:** `test(preflightshed): add Tier-1 constructor and cancellation tests`

### Card 5: move the Tier-2 wrapper test into preflightshed

- **Context:**
  - `internal/preflightshed/preflight.go`
  - `internal/preflight/preflight_integration_test.go`
  - `internal/loomshed/loomshed.go`
  - `internal/hubforge/hub.go`
- **Edits:** none
- **Creates:** none
- **Deletes:** none
- **Moves:**
  - `internal/loomshed/preflight_integration_test.go` -> `internal/preflightshed/preflight_integration_test.go`
- **Requirements:** `git mv` the file first, then make only surgical edits.
  Change the package clause from `package loomshed_test` to `package preflightshed` (in-package — see the `preflightshed-integration-test-is-in-package` Shared Decision).
  Drop the `internal/loomshed`, `internal/loomengine` and `internal/shedengine`-adjacent imports that become unused, keeping only what the file still references; `shedengine` itself is still needed for `shedengine.Done`/`shedengine.Stuck`.
  Retarget both `loomshed.NewPreflightProducer(h.PrimeWorktree())` call sites to `NewPreflight("Preflight", h.PrimeWorktree())` — an unqualified in-package call now, with the told name supplied as a plain literal because this package declares no name constant.
  In `setupPreflightWrapperFixture`, delete the `statusPath`/`statusLockPath` locals and the `loomshed.Seed(...)` call entirely, and **keep** the `gitkit.MustRun` `git add -A` + `git commit` pair that follows, retitling the commit message from `"seed status"` to `"seed junctions"`.
  Keeping the commit is not optional and is not a judgement call left open: `internal/preflight/preflight_integration_test.go`'s own `setupFixture` is this same fixture minus the seed and still performs exactly that add-and-commit, because `fabricengine.WireJunctions` leaves untracked entries behind that the cleanliness check would otherwise report.
  Rewrite the fixture's doc comment accordingly — it currently justifies seeding through `loomshed.Seed`, and post-split row 1 never reads that file at all.
  Rewrite the file-level doc comment: it must now say this file covers `preflightshed.NewPreflight`'s wrapper against a `hubforge` fixture hub, that it is in-package because nothing in `hubforge`'s dependency set reaches this new leaf, and that it stays to the wrapper's outcome mapping only.
  Rename both tests to `TestPreflight_AllPreconditionsPass` and `TestPreflight_BrokenPreconditionMapsToStuck`.
  Add one new Tier-2 test, `TestPreflight_CancelledDuringRunReturnsError`: build the healthy fixture, create a `context.WithCancel` context, cancel it, then call `Call` and assert a non-nil error with an outcome that is neither `Done` nor `Stuck`.
  Its doc comment must state why this case is Tier 2 rather than Tier 1 — `preflight.Check` reaches `lyxcwd.Resolve`'s git spawn unconditionally, so every path that calls `Check` at all is Tier 2 regardless of what it returns, and the producer holds no injectable seam between `entryErr` and the `Check` call.
  If the in-package form turns out to close a compile cycle, fall back to `package preflightshed_test` and qualify the two constructor calls as `preflightshed.NewPreflight(...)`.
- **Commit:** `test(preflightshed): move the Tier-2 wrapper test over from loomshed`

### Card 6: relocate the hermetic-git TestMain

- **Context:**
  - `internal/loomshed/testmain_integration_test.go`
  - `internal/loomengine/testmain_test.go`
  - `internal/preflightshed/preflight_integration_test.go`
  - `CONSTRAINTS.md`
- **Edits:** none
- **Creates:**
  - `internal/preflightshed/testmain_integration_test.go`
- **Deletes:**
  - `internal/loomshed/testmain_integration_test.go`
- **Moves:** none
- **Requirements:** Create `internal/preflightshed/testmain_integration_test.go` carrying a `//go:build integration` constraint, `package preflightshed`, and a `TestMain(m *testing.M)` calling `gitkit.HermeticGitEnv()` before `os.Exit(m.Run())`, modelled line-for-line on the file being deleted.
  Its header comment names `preflight_integration_test.go` as the sibling that spawns git via `hubforge` fixtures and cross-references the Hermetic Git Test Environment Invariant.
  The package clause must match whichever package card 5 settled on for the integration test — Go permits only one `TestMain` per directory across both the in-package and external test packages.
  Delete `internal/loomshed/testmain_integration_test.go`: after card 5's move, `internal/loomshed` has no test file that spawns git at all, so a `TestMain` there would run `HermeticGitEnv` for nothing.
  Do **not** touch `internal/loomengine/testmain_test.go` in this batch — it is still needed by `internal/loomengine/preflight_integration_test.go`, which batch 4 retires.
- **Commit:** `test(preflightshed): move the hermetic-git TestMain over from loomshed`

## Batch Tests

Verified by the batch's three-command `verify:` chain (see the `verify-is-the-discussion-s-three-commands` Shared Decision for why it is repo-wide rather than package-scoped).

Tier 1 (`go test ./... -count=1`) covers the new `internal/preflightshed/preflight_test.go`, plus the three `cmd/lyx` guard suites this batch is most exposed to: `tierpurity_test.go` (the new untagged test file must carry no spawn token), `hermeticenv_test.go` (the moved integration test's package must contain `HermeticGitEnv`, which card 6 supplies and which would fail between cards 5 and 6), and `internal/lyxcwd/enforcement_test.go`'s `TestEnforcement_FabricVocabulary` (the three new production files must name neither fabric-internal side).
It also re-runs `internal/loomshed`'s untagged suite unchanged — nothing this batch does should move it.

Tier 2 (`go test -tags integration ./... -count=1`) is where the moved wrapper test actually runs, against a real `hubforge` hub: all preconditions pass → `Done`, an untracked file in the pair → `Stuck`, and the new cancelled-during-run case → error.
It is also the only place the dropped-`Seed`/kept-commit fixture change is proven: if `WireJunctions` did in fact leave nothing untracked and the commit were superfluous, the healthy case would still pass, but if the seed drop had broken cleanliness the healthy case would report `Stuck` instead of `Done`.

`go vet -tags smoke ./internal/loomcli` is unaffected by this batch and is run only to keep the three-command chain uniform across batches.
