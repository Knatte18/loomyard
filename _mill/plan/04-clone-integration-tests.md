# Batch: clone integration tests

```yaml
task: 'fabric: store the warp-URL binding in weft:main; fold bootstrap into clone (slice 10)'
batch: 'clone integration tests'
number: 4
cards: 2
verify: go test -tags integration ./internal/fabricengine/
depends-on: [3]
```

## Batch Scope

This batch delivers the end-to-end proof of the binding's clone-side behaviour against real local bare-repo fixtures: the five conflict-rule rows, the probe's three-way failure taxonomy, the old-order guard in all three of its outcomes, `--reset` in both argument forms, and the ordering guarantee that a two-argument re-clone against an existing hub never runs the probe at all.
It is one batch because every test shares the same fixture vocabulary and the same new file, and because these are the tests that prove the footgun is closed — splitting them across batches would let the guard ship unproven.

No external interface is produced.
Both cards write to the same new file: card 11 creates it with the conflict-rule and taxonomy coverage, card 12 appends the guard, reset, and ordering coverage.

Batch-local decision: no new fixture helper is invented where an existing one fits.
`makeBareRemote`, `makeEmptyBareRemote`, `commitFileOnBranch`, `gitOutput`, and `assertBoardIsWeftWorktree` all already live in `package fabricengine_test` and are reused directly.

## Cards

### Card 11: conflict-rule and probe-taxonomy integration tests

- **Context:**
  - `internal/fabricengine/clone.go`
  - `internal/fabricengine/warpprobe.go`
  - `internal/fabricengine/warpbinding.go`
  - `internal/fabricengine/clone_adopt_test.go`
  - `internal/lyxcwd/anchor.go`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/warpbinding_clone_integration_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  New file, first non-empty line `//go:build integration`, `package fabricengine_test`, sharing the existing `TestMain` in the package's `testmain_test.go` — do not add another one.
  Open with a file-level comment naming what it proves and stating that its fixture helpers are reused from `clone_adopt_test.go`.

  Add one local helper, `seedWeftBinding(t *testing.T, dir, bareRemote, warpURL string)`, that wraps `commitFileOnBranch` to commit a `.lyx-warp` file containing `warpURL + "\n"` onto the bare weft remote's `main` branch — this is what a prior clone would have left.
  Every other fixture need is met by the existing helpers.

  Tests:

  - `TestCloneHub_BootstrapWritesBinding` — a two-argument clone against a weft fixture with no record, run with `ForceBootstrap: true`, writes the record at the board root with the supplied URL, and the file is tracked on the board worktree (`git ls-files` at the board dir, via `gitOutput`).
    Assert `CloneResult.WarpBindingRecorded == true` and `CloneResult.WarpURL` equals the supplied URL.
  - `TestCloneHub_DerivesWarpFromBinding` — a one-argument clone against a weft whose `main` carries a record produces the same geometry as the two-argument form: assert `HubPath`, `Anchor`, `BoardDir`, `WeftBase`, and `PrimeCwd` are consistent with a hub named after the recorded warp repo, that `assertBoardIsWeftWorktree` holds, and that `WarpBindingRecorded == false`.
  - `TestCloneHub_MatchingBindingIsNoOp` — a two-argument clone whose supplied URL is byte-identical to the record succeeds and leaves the record's bytes unchanged (read the file before and after; the on-disk content and the committed blob must both be untouched).
  - `TestCloneHub_NormalizedBindingMatch` — the supplied URL differs from the record only by a trailing `.git` and is still treated as matching: the clone succeeds and the record is unchanged.
  - `TestCloneHub_ConflictLeavesNoHub` — a two-argument clone against a record naming a different warp fails;
    the error text contains both URLs and `refusing to re-point`;
    and — this is the property the pre-hub probe buys — assert on the filesystem that no `<name>-HUB` directory exists under the clone parent for either URL's derived name, and that no directory whose name begins with the probe prefix survives in the clone parent.
  - `TestCloneHub_UnboundWeftNamesTwoArgForm` — a one-argument clone against a weft with no record fails with a message containing `has no recorded warp binding` and `lyx fabric clone <weft-url> <warp-url>`, and creates no hub.
  - `TestCloneHub_EmptyWeftRemoteTaxonomy` — two subtests against a `makeEmptyBareRemote` weft (unborn HEAD): the one-argument form reports unbound rather than a git error, and the two-argument form bootstraps successfully and writes the record, preserving today's orphan-create path through `ensureBoardWorktree`.
  - `TestCloneHub_UnreachableWeftIsHardError` — a clone against a weft path that does not exist fails in BOTH argument forms with a message carrying the `probe weft ` prefix, and in neither case reports "unbound" and in neither case bootstraps (assert no hub directory).
  - `TestCloneHub_AbsenceDiscriminatorDistinguishesMissingFromBroken` — a weft whose HEAD exists but lacks the record is classified absent (the one-argument form reports unbound, not a git error), while a weft whose HEAD object is unreadable is a hard error.
    Produce the second case by cloning a valid bare weft fixture into a scratch bare repo and then corrupting or removing the object file HEAD resolves to, so `ls-tree` itself fails;
    assert the error carries the `probe weft ` prefix and does not contain `has no recorded warp binding`.
    This is the test that justifies the `ls-tree` discriminator existing at all — if it cannot be made to fail deterministically on the platform under test, keep the missing-record half and record the broken-HEAD half as a subtest with an explicit skip reason rather than deleting it.
  - `TestCloneHub_BackfillsBindingOnPreBindingHub` — a weft carrying `.lyx-anchor` but no `.lyx-warp` (seeded with `commitFileOnBranch`), plus an explicit warp URL, writes the record and reports `WarpBindingRecorded == true` without `ForceBootstrap` — the anchor alone must satisfy the guard.

  All local fixture paths passed as URLs go through `filepath.ToSlash`, matching every existing test in this package.
- **Commit:** `test(fabricengine): cover the warp-binding conflict rule and probe taxonomy`

### Card 12: old-order guard, reset, and ordering integration tests

- **Context:**
  - `internal/fabricengine/clone.go`
  - `internal/fabricengine/warpprobe.go`
  - `internal/fabricengine/clone_adopt_test.go`
- **Edits:**
  - `internal/fabricengine/warpbinding_clone_integration_test.go`
  - `internal/fabricengine/export_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Append to the file created in card 11, reusing its `seedWeftBinding` helper and the package's existing fixture helpers.

  - `TestCloneHub_OldOrderInvocationIsRefused` — the regression test that matters most.
    Build an ordinary source repo fixture (a seeded bare repo with a README and no lyx markers) and pass it as the FIRST positional with a second repo as the warp, i.e. the pre-change argument order, with `ForceBootstrap` left false.
    Assert the call fails;
    the error contains `refusing to bootstrap` and `check the argument order`;
    no hub directory is created;
    and — asserted explicitly, because this is the corruption being prevented — the repo passed as the weft candidate has the same commit count and the same ref list after the call as before it.
    Capture both with `gitOutput` before and after (`rev-list --count --all` and `for-each-ref --format=%(refname)`).
  - `TestCloneHub_AnchorBearingWeftPassesGuard` — a weft whose HEAD carries `.lyx-anchor` but no `.lyx-warp` passes the guard and bootstraps with `ForceBootstrap` false.
    This overlaps card 11's backfill test in setup but asserts the guard specifically;
    keep it as its own test so a guard regression names itself.
  - `TestCloneHub_ForceBootstrapOverridesGuard` — the same ordinary-source-repo weft candidate that `TestCloneHub_OldOrderInvocationIsRefused` rejects succeeds when `ForceBootstrap: true` is set, and writes the record.
    Together the two tests establish that the flag is the only way through.
  - `TestCloneHub_ResetInBothArgumentForms` — two subtests.
    Two-argument: clone, then re-clone the same pair with `Reset: true`, asserting the second call succeeds and the hub exists with the expected geometry.
    One-argument: after a bound hub exists, re-clone with only the weft URL and `Reset: true`, asserting the hub is torn down and re-created and the warp is still derived from the record.
    The one-argument case is the point of the whole reset-folding change — once a weft is bound, the one-argument form is the normal invocation.
  - `TestCloneHub_HubExistsCheckPrecedesProbeInTwoArgForm` — clone once, then attempt a second two-argument clone of the same pair with `Reset` false.
    Assert the error is today's `hub already exists` and, as the observable difference, that no directory whose name begins with the probe prefix was ever created in the clone parent — the check short-circuits before the probe runs.
    `warpProbeDirPrefix` is declared unexported in card 3, so this card also adds a re-export to the package's existing `export_test.go` seam — `const WarpProbeDirPrefixForTest = warpProbeDirPrefix`, with a doc comment in the shape of that file's existing `NewPairedForTest`/`WarpForTest` entries — and the test reads the prefix through it.
    Never duplicate the literal in the test.
    This is not conditional: the constant is unexported by construction, so the re-export is always required.

  Do not assert the one-argument form's ordering symmetrically: in that form the probe necessarily precedes the hub-exists check, and that asymmetry is deliberate.
  State it in a comment on this test so a future reader does not "fix" it.
- **Commit:** `test(fabricengine): cover the old-order guard, reset folding, and probe ordering`

## Batch Tests

`verify:` is `go test -tags integration ./internal/fabricengine/`.

The tag is mandatory: both cards write to `internal/fabricengine/warpbinding_clone_integration_test.go`, whose first non-empty line is `//go:build integration`, so an untagged run would not compile the file at all.
The scope is the single package because every test in this batch is a `fabricengine` test against local bare-repo fixtures;
nothing in `internal/fabriccli` or `cmd/lyx` changes here.

The run also re-executes the rest of the package's integration suite, including the invocations batch 2 rewrote, which is the cheapest available regression check on the clone flow as a whole.
The overview's module-wide `go build ./...` runs at the batch boundary as the compile gate.
