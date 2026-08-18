# Batch: docs-and-cross-cutting-verification

```yaml
task: "the standalone CLI path"
batch: "docs-and-cross-cutting-verification"
number: 6
cards: 3
verify: go test ./cmd/lyx/...
depends-on: [3, 4, 5]
```

## Batch Scope

This batch lands the one documentation edit the task owes and runs the two cross-cutting checks no single-package batch can make.
It depends on batches 3, 4 and 5 because the design-doc correction note describes what actually shipped — the trigger, the scope amendment onto `webstercli`, and the already-landed `CONSTRAINTS.md` rewords — and writing it before the code exists would be writing it from the plan rather than from the tree.
The two verification cards produce no diff of their own; they exist because `cmd/lyx` holds the repo's structural enforcement suites (tier purity, hermetic git environment, help tree, constructor anchoring) and those are precisely the invariants a five-package wiring change is able to break without any of the per-package batches noticing.

**Batch-local decision:** `CONSTRAINTS.md` is verified, not edited.
The discussion pins the expected outcome as no change, and a cross-cutting invariant edit is a review-gated act — so card 28 reports a discrepancy rather than silently fixing one.

## Cards

### Card 27: add the T8 correction note to the design doc

- **Context:**
  - `internal/preflight/predicates.go`
  - `internal/preflight/doc.go`
  - `internal/webstercli/cli.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `manifest/designs/producers-standalone.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  T8's brief in this document is stale on two points, and the document survives until T10 deletes it, so a reader between now and then would be handed a brief that contradicts the merged code.
  Add a short, clearly-marked correction note to T8's entry — placed immediately after the `**T8 — the standalone CLI path**` heading and its `slug:` line, before the `**Brief.**` paragraph, so it is read before the stale text rather than after it.
  Follow the shape the document's own `## Corrections to the originating discovery task` section already establishes.

  The note must state two corrections.

  First, the shipped mode trigger is `preflight.ResolveMode`, not the `fabricengine.Ready`-class check with a `fabricengine.BoardDir(filepath.Dir(worktreeRoot))` discriminator this brief describes.
  `Ready` probes the paired sibling of the current worktree rather than the hub, so it is false at `<hub>/_board`, false in an unpaired sibling, and false in a worktree whose pair was removed — three real, healthy hub situations that run producer verbs today, and keying mode selection on it would relocate a live hub's state into the per-OS state directory.
  The hub probe is `HubPresent`-shaped instead: a `lyxcwd.Resolve` plus one `os.Stat` of `<hub>/_board/_lyx`.
  `ResolveMode` wraps that probe in a three-way resolver so a `Resolve` failure that is not `ErrNotAGitRepo` refuses rather than degrading — and so that an ordinary subdirectory of a *plain* repo, which raises the same `ErrCwdOutsideAnchor` sentinel, still resolves to standalone.
  Say that this correction supersedes the brief's own `**A wired-but-broken hub is refused, never silently degraded to standalone.**` paragraph, whose requirement is met by a different mechanism: a damaged hub stays in hub mode and fails loudly at the point of use.

  Second, the brief's `**Invariant rewords land in this task's own commit, not deferred to T10.**` instruction is already satisfied.
  T7, commit `3255efa6`, landed both the Stencil Ownership read-location and seed-pass rewords and the Durable-vs-Ephemeral standalone bullet, in generalised, producer-agnostic wording that names no module.
  T8 therefore verified them and made no edit; re-wording correct text to name burler and perch would make a general invariant less general.

  Also record that T8's scope was widened to repoint `internal/webstercli` onto `ResolveMode` in the same commit, because the trigger decision rests on all three standalone-capable CLIs selecting modes by one rule.

  Do not delete or rewrite the stale text itself — the correction-note convention this document uses is to leave the original brief legible and mark what superseded it.
  Follow the repo's semantic-line-break convention: one sentence per line, no fixed-column hard wrap.
- **Commit:** `docs(designs): correct T8's stale mode-trigger and CONSTRAINTS instructions`

### Card 28: verify the two invariants this task falsifies nothing in

- **Context:**
  - `CONSTRAINTS.md`
  - `internal/burlercli/wiring.go`
  - `internal/perchcli/wiring.go`
  - `internal/standalonegeom/stencilsdir.go`
- **Edits:** none
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Read the Stencil Ownership Invariant and the Durable-vs-Ephemeral State Invariant in `CONSTRAINTS.md` and confirm three specific things.

  One: the Stencil Ownership read-location wording already covers burler and perch.
  It should read that `<hub>/_board/_lyx/stencils/` is what the told directory resolves to in hub mode, not the only possibility, and that a standalone-capable CLI's own producer resolves it under the per-OS state directory.
  Two: the seed-pass bullet already covers a producer CLI's own pre-run, reading that the pass runs once per process either at `cmd/lyx`'s root pre-run in hub mode or at the producer CLI's own pre-run in standalone — with its load-bearing "never lazily inside `stencilstore.Read`" half intact.
  Three: the Durable-vs-Ephemeral standalone bullet already names `internal/standalonestate.Derive`'s state directory as a legitimate root at which the mirrored-subpath rule holds.

  Confirm none of the three names webster or `webstercli` specifically where it should be general.
  The expected outcome is no edit: T7 landed all three in producer-agnostic form.

  If any of the three does name webster specifically, or is otherwise falsified by what batches 4 and 5 shipped, do **not** edit `CONSTRAINTS.md` here.
  Report it as a blocking finding naming the exact bullet and the exact wording problem.
  Editing a cross-cutting invariant is a review-gated act under the repo's own rules, and a silent edit inside a verification card is the one way that gate gets bypassed.

  This card writes no code and produces no commit.
- **Commit:** none

### Card 29: run the cross-cutting `cmd/lyx` enforcement suites

- **Context:**
  - `cmd/lyx/helptree_test.go`
  - `cmd/lyx/constructoranchoring_test.go`
  - `cmd/lyx/tierpurity_test.go`
  - `cmd/lyx/hermeticenv_test.go`
  - `cmd/lyx/drift_test.go`
  - `cmd/lyx/seamsignature_test.go`
  - `internal/burlercli/cli.go`
  - `internal/burlercli/testmain_test.go`
  - `internal/perchcli/cli.go`
- **Edits:** none
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Run `go test ./cmd/lyx/...` and confirm every structural enforcement suite passes against the tree batches 3, 4 and 5 produced.
  Four of them are the ones this task is able to break, and each must be checked for the specific reason it could fail rather than merely observed green:

  `TestTierPurity_UntaggedTestsSpawnNothing` — the three new untagged `wiring_test.go` files must contain none of the banned tokens, and the match is a raw substring match, so a token appearing inside a comment or a string literal trips it too.
  `TestHermeticGitEnv_GitSpawningPackagesHaveTestMain` — this suite passes trivially for `internal/burlercli` and that is expected, not a success signal.
  The guard scans for literal tokens (`gitexec.Run`, `exec.Command`, `gitkit.Copy*`, `hubforge.NewHub`), none of which appear in burlercli's test files after batch 4, since its integration test reaches git indirectly through `RunCLIIn`.
  So the guard never classifies the package as git-spawning and has nothing to check — exactly the blind spot batch 4 card 17 documents, and the reason that card's `TestMain` is required by review discipline rather than by this suite.
  Confirm the `TestMain` is present by reading `internal/burlercli/testmain_test.go`; do not treat a green run here as evidence it exists.
  The help-tree and `Short`-completeness suites — both CLIs gained two persistent flags and a rewritten `Long`, so help output changed even though these suites assert module presence and `Short` rather than flags.
  `cmd/lyx/constructoranchoring_test.go` — already in its post-T6 told-string shape and expected to need no row change, but it is what would catch a geometry constructor accidentally re-anchored by batch 4 or 5.

  If a suite fails, fix the `cmd/lyx` test that fails only when the failure is a genuine expectation change caused by this task's new flags or help text — and say exactly which expectation moved and why in the batch report.
  If the failure instead indicates a real invariant breach in `internal/burlercli`, `internal/perchcli`, `internal/webstercli`, `internal/preflight` or `internal/standalonegeom`, do not adjust the enforcement test to accommodate it: report it as a blocking finding against the batch that introduced it.
  An enforcement suite edited to make a breach pass is worse than the breach.

  On a clean run this card produces no commit.
- **Commit:** none

## Batch Tests

`verify:` runs `go test ./cmd/lyx/...`, which is card 29's whole content and the gate for cards 27 and 28 alike.
No `-tags integration` invocation is chained: card 29's four named suites are untagged, and the task-wide integration coverage is already run by batches 1, 3, 4 and 5's own verify commands plus the repo's configured done gate (`go test ./... && go test -tags integration ./...`), which runs from the repo root before the task is marked done.

Cards 28 and 29 are verification-only and contribute no diff, so this batch's only commit is card 27's design-doc note.
That is deliberate: the batch exists to make the cross-cutting checks an explicit, reviewable step rather than something folded silently into the done gate, where a failure would surface with no record of which invariant was being protected.
