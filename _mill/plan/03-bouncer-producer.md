# Batch: bouncer-producer

```yaml
task: 'Bouncer: the generic review-gate producer'
batch: 'bouncer-producer'
number: 3
cards: 4
verify: go test ./internal/shedadapters/...
depends-on: [1, 2]
```

## Batch Scope

This batch lands the producer itself: `BouncerConfig`, the validating `NewBouncer`, the whole four-mode `Call`, and the package doc update that records the new member.
It depends on batch 1 for every parser, writer, and path helper `Call` consumes, and on batch 2 for the two stencil names `Call` reads.
It is one batch because `Call`'s four branches share one mode-resolution step and one degradation helper, so splitting them across batches would leave a `Call` whose own switch is half-written.
The external interface batch 4 consumes: `NewBouncer`, `Bouncer.Call`, and the package-level test fixtures this batch's own tests establish.

Batch-local decision, differing from nothing in the overview but worth stating once here: an unreadable `RunDir` surfacing from `ResolveRound` is a **hard error**, not a degradation to `Stuck`.
Every other failure this producer meets is fixable by another round, so bouncing to the round producer is the right recovery; a run dir neither producer can read is not, and `ResolveRound`'s own contract already says an unreadable run dir must never look like a fresh segment.
That path still consults `cancelErr` before returning its own error, exactly as every other non-success exit in this producer does — hard-versus-degraded governs which error is returned, never whether cancellation is checked.

## Cards

### Card 5: `BouncerConfig` and the validating constructor

- **Context:**
  - `internal/shedadapters/singlellm.go`
  - `internal/shedadapters/perch.go`
  - `internal/shedadapters/round.go`
  - `internal/stencilstore/reconcile.go`
  - `internal/shuttleengine/spec.go`
  - `internal/shedengine/producer.go`
  - `contracts/stencils/registry_test.go`
  - `_mill/discussion.md`
- **Edits:** none
- **Creates:**
  - `internal/shedadapters/bouncer.go`
  - `internal/shedadapters/bouncer_config_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create `internal/shedadapters/bouncer.go` in package `shedadapters`, with a file header comment stating that it implements `Bouncer`, the generic review-gate producer, that it is the one member of this package that is new logic over `shuttleengine` rather than a translation of an already-shipped engine, and that it is parametrized purely by a rubric stencil name and a report-name convention rather than by which round producer sits opposite it.

  Declare `const bouncerEngineLabel = "bouncer"`, the short engine label this producer's log lines and error text carry, matching `singleLLMEngineLabel`'s role in `internal/shedadapters/singlellm.go`.
  Reuse the package's existing `Shuttle` interface and its existing `var _ Shuttle = (*shuttleengine.Runner)(nil)` compile-time proof as they stand.
  Declare neither a second `Shuttle` interface nor a second such `var _` line — both already exist in this package and a redeclaration fails to compile.

  Declare `BouncerConfig` with exactly these exported fields, each carrying a doc comment: `Name string`, `RunDir string`, `ArtifactPaths []string`, `ReportName func(round int) string`, `StencilsDir string`, `RubricStencil string`, `Model string`, `Effort string`, `Version string`, `Shuttle Shuttle`, `Now func() time.Time`.
  Document `Name` as a log-field and error-text identity only, never compared, parsed, or used for control flow.
  Document `ArtifactPaths` as the subject under review — what the rubric is applied *to* — as opposed to `RunDir`/`ReportName`, which name the round producer's report, a document *about* the subject.
  Document `Model`, `Effort`, and `Version` as an already-resolved triple threaded verbatim into `shuttleengine.Spec`, resolved at the caller's own config-load time.
  Document `Now` as the injected clock resolving only the archive filename's same-second collision suffix.

  Declare `type Bouncer struct { cfg BouncerConfig }`.
  Do not add the `var _ shedengine.ShedProducer = (*Bouncer)(nil)` compile-time assertion in this card — `*Bouncer` has no `Call` method until card 6, so the assertion would fail to build at this card's commit.
  Card 6 adds it alongside `Call`, which is where every existing adapter in this package keeps its own.

  Declare `NewBouncer(cfg BouncerConfig) (*Bouncer, error)`, validating in a fixed order and returning a `shedadapters: NewBouncer: `-prefixed error naming the offending field: `Name` non-empty; `RunDir` non-empty and `filepath.IsAbs`; `ArtifactPaths` non-empty with every entry non-empty and `filepath.IsAbs`; `ReportName` non-nil; `StencilsDir` non-empty and `filepath.IsAbs`; `RubricStencil` non-empty; `Shuttle` non-nil.
  `Model`, `Effort`, and `Version` are accepted empty and defer to the provider default.
  A nil `Now` defaults to `time.Now`.
  Nothing stats an `ArtifactPaths` entry: an artifact that does not exist yet is legitimate, since the segment may be gated behind a producer that writes it.

  After the field checks pass, probe the rubric eagerly with `stencilstore.Read(cfg.StencilsDir, cfg.RubricStencil)` and return a wrapped error naming `RubricStencil` when it fails.
  Give this probe its own doc-comment paragraph explaining why it is deliberate I/O in a constructor: `stencilstore.Read` never falls back to a shipped default, and the once-per-process seed pass only ever seeds registry-registered names, so an unregistered or mistyped rubric name would otherwise degrade every judge call to `Stuck` until the whole segment's bounce budget was spent.
  Probe only the rubric; the two generic templates are registry-guaranteed and covered by `contracts/stencils/registry_test.go`, so the caller-supplied name is the only one that can be wrong.

  Add a doc-comment paragraph on `NewBouncer` stating the budget rule its callers need: a Bouncer configured with `MaxBounces: N` gets `N` judged rounds, and the `N`th blocks the run if it comes back `BLOCKING`.
  The seed call's unconditional `Stuck` permanently consumes one unit because the Bouncer's only `Done` exits the segment and its episode therefore never resets.
  State that this offset is documented rather than compensated for in code, because silently adding one here would make `MaxBounces` mean something different for this producer than for every other row in the list.
  Add a second paragraph stating the wiring obligation: this producer is its segment's entry point, its `OnStuck` names the round producer for both the seed call and a rejection, and its `OnDone` is set explicitly to whatever follows the segment — an empty `OnDone` is load-bearing and silent, ending the whole run rather than advancing the pipeline.

  Create `internal/shedadapters/bouncer_config_test.go` with a table test over `NewBouncer`, one case per validation rule, each asserting the error text names the offending field: empty `Name`; empty `RunDir`; relative `RunDir`; empty `ArtifactPaths`; an `ArtifactPaths` entry that is relative; nil `ReportName`; empty `StencilsDir`; relative `StencilsDir`; empty `RubricStencil`; nil `Shuttle`.
  Assert explicitly that empty `Model`, empty `Effort`, and empty `Version` are accepted, so a later tightening cannot silently break the provider-default path.
  Assert that a nil `Now` yields a non-nil clock on the constructed value.
  Assert that an `ArtifactPaths` entry naming a file that does not exist yet succeeds.
  Add the two rubric-probe cases: a `RubricStencil` naming a file absent under `StencilsDir` fails at construction, and a readable one succeeds.
  Add a package-level test helper in this file that builds a stencils fixture directory under `t.TempDir()`: it writes each named stencil at `<dir>/bouncer/<name>.md`, matching `stencilstore.RelPath`'s family-from-first-token derivation, and gives each file a realistic leading `<!-- lyx-stencil: sha256=... -->` stamp banner.
  Batches 3 and 4's later test files reuse this helper.
- **Commit:** `feat(shedadapters): add BouncerConfig and the validating NewBouncer constructor`

### Card 6: `Call` — four modes, harvest, replay, and focus synthesis

- **Context:**
  - `internal/shedadapters/singlellm.go`
  - `internal/shedadapters/ctx.go`
  - `internal/shedadapters/archive.go`
  - `internal/shedadapters/round.go`
  - `internal/shedadapters/bouncerfiles.go`
  - `internal/treadleengine/judge.go`
  - `internal/shuttleengine/spec.go`
  - `internal/shedengine/producer.go`
  - `internal/stencil/stencil.go`
  - `internal/stencilstore/reconcile.go`
  - `_mill/discussion.md`
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/shedadapters/bouncer.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add `func (b *Bouncer) Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error)` to `internal/shedadapters/bouncer.go`, plus the unexported helpers below.
  Add the compile-time assertion `var _ shedengine.ShedProducer = (*Bouncer)(nil)` in this card, beside `Call`, matching where `internal/shedadapters/singlellm.go` keeps its own.
  It belongs here rather than in card 5 because `*Bouncer` satisfies the interface only once `Call` exists.
  Import `internal/logger`, `internal/stencil`, `internal/stencilstore`, `internal/shuttleengine`, and `internal/shedengine`.
  Import neither `internal/burlerengine`, nor `internal/loomengine`, nor `internal/treadleengine`.
  Call no `lyxcwd` function, no `os.Getwd`, and no `filepath.Abs`, and write neither the literal `_lyx` nor the literal `.lyx` anywhere in this file — every path is joined onto the told `RunDir`.

  `Call`'s sequence.
  First `entryErr(ctx, b.cfg.Name, bouncerEngineLabel)`, returning immediately on a non-nil result.
  Then `n, err := ResolveRound(b.cfg.RunDir, b.cfg.ReportName)`; on a non-nil `err`, consult `cancelErr(ctx, b.cfg.Name, bouncerEngineLabel)` first and return that when it is non-nil, otherwise return the resolve failure wrapped as a hard error.
  It is a hard error rather than a degradation because no later round can repair an unreadable run dir, and it consults `cancelErr` first for the same reason every other non-success exit in this producer does — `internal/shedadapters/ctx.go`'s own doc states that every non-success return path consults `cancelErr` first, and the only carve-out anywhere in this design is a genuinely parsed verdict.
  Then branch on `n`.

  Declare `judged(round int) bool`: true only when `verdictPath` exists and `parseVerdict` accepts its bytes **and** `ledgerPath` exists and `parseLedger` accepts its bytes.
  Document that it deliberately excludes the focus file, because that file is an input to the next round rather than evidence about this one, and is synthesizable — including it would let a missing focus file invalidate a judgment that provably happened.

  Declare `degrade(ctx context.Context, msg string, args ...any) (shedengine.Outcome, shedengine.OutputPointer, error)`: consult `cancelErr(ctx, b.cfg.Name, bouncerEngineLabel)` first and return that error when non-nil, otherwise log `logger.Warn` with the producer name, the round, and the cause, and return `shedengine.Stuck` with an empty `shedengine.OutputPointer` and a nil error.
  Every judge-call infrastructure failure routes through this helper and none of them ever returns `shedengine.Done`.

  Declare `ensureFocus(round int)`: when `focusPath(b.cfg.RunDir, round)` is absent, or present but rejected by `parseFocus`, archive whatever is there via `archiveStaleOutputs` and then `writeFocus` a synthetic `focusFile{Round: round}` with both lists empty, logging a `logger.Warn`.
  Archive rather than overwrite: a focus file the judge wrote but the parser rejected is the evidence of whatever malformed the judge's output, and overwriting it with two empty lists would erase the only record.
  A present, parsing file is left byte-identical.

  Declare `settle(ctx context.Context, round int, spawned bool)`: read and `parseVerdict` the round's verdict file, which `judged(round)` has already proved parses.
  On `verdictApproved` return `shedengine.Done` with `shedengine.OutputPointer{Path: ledgerPath(b.cfg.RunDir, round)}` and a nil error.
  On `verdictBlocking` call `ensureFocus(round + 1)`, then return `shedengine.Stuck` with the same ledger pointer and a nil error.
  Both returns survive cancellation: a genuinely parsed verdict is the one exception `cancelErr` never applies to, exactly as `internal/shedadapters/singlellm.go` treats a shuttle `OutcomeDone`.
  When `spawned` is false and the verdict is `verdictBlocking`, log a `logger.Warn` naming the producer and the round, because a `BLOCKING` replay means the round producer handed control back without producing a new report; an `APPROVED` replay is not a warning condition.

  **Seed call** — `n == 0` and `focusPath(RunDir, 1)` is absent or rejected by `parseFocus`.
  Archive the round-1 focus path via `archiveStaleOutputs`, degrading on failure.
  Read `bouncer-template-seed` via `stencilstore.Read(b.cfg.StencilsDir, "bouncer-template-seed")`, read the rubric via `stencilstore.Read(b.cfg.StencilsDir, b.cfg.RubricStencil)`, and pass the rubric bytes through `stencil.StripLeadingComment` before using them.
  The strip is load-bearing: `stencil.Fill` strips a stamp banner from the template it parses but never from a marker value, so raw rubric bytes would inject a `<!-- lyx-stencil: sha256=... -->` line into the middle of the prompt.
  Fill the four seed markers — `rubric`, `artifacts` as `strings.Join(b.cfg.ArtifactPaths, "\n")`, `round` as `"1"`, `focus_path` as the absolute round-1 focus path — with `stencil.Fill`.
  Build a `shuttleengine.Spec` carrying the filled prompt, `OutputFiles` of exactly the one round-1 focus path, `Model`/`Effort`/`Version` verbatim from the config, `Role: "bouncer-seed"`, and `Round: "1"`; run it through `b.cfg.Shuttle`.
  Each of the read, strip, fill, run-error, and non-`OutcomeDone` steps logs a `logger.Warn` and falls through to the fallback rather than returning early.
  After the spawn returns, whatever it reported, call `ensureFocus(1)` — the seed-side twin of harvest, keyed on the file's state rather than on the spawn's outcome, so a spawn that wrote real targeting and then hit a run error keeps its file instead of having it replaced with two empty lists.
  Return `shedengine.Stuck` with an empty pointer, consulting `cancelErr` first since a seed `Stuck` is not a verdict.

  **Re-bounce** — `n == 0` and `focusPath(RunDir, 1)` is present and parses.
  Spawn nothing, touch nothing, log a `logger.Warn` naming the producer and stating the segment was already seeded and the round producer handed control back without producing round 1's report, and return `shedengine.Stuck` with an empty pointer, consulting `cancelErr` first.

  **Replay** — `n >= 1` and `judged(n)`.
  Spawn nothing and call `settle(ctx, n, false)`.

  **Judge call** — `n >= 1` and not `judged(n)`.
  Read the report at `filepath.Join(b.cfg.RunDir, b.cfg.ReportName(n))`; an unreadable file, or one whose content is empty after `strings.TrimSpace`, degrades.
  Reading it is what makes those two cases visible: `ResolveRound` proved only that `os.Stat` succeeded, so a truncated or empty report written by a round producer that died mid-write is still reachable here.
  Resolve the previous-ledger marker: for `n >= 2`, when `ledgerPath(RunDir, n-1)` is readable and `parseLedger` accepts it, use that absolute path; when it is unreadable or rejected, log a `logger.Warn` and fall back to the literal `"(none)"`; for `n == 1`, use `"(none)"` with no warning.
  A malformed previous ledger degrades to running the judge with no prior ledger rather than to `Stuck`.
  The literal rather than an empty string is required because `stencil.Fill` errors on any marker resolving to empty.
  Read `bouncer-template-judge` and the rubric the same way the seed call does, stripping the rubric's leading comment; each read failure degrades.
  Build `outputs` as exactly three absolute paths, in this order and unconditionally: `verdictPath(RunDir, n)`, `ledgerPath(RunDir, n)`, `focusPath(RunDir, n+1)`.
  The list is never conditional on the verdict, and the reason belongs in a comment here: `shuttleengine` classifies a run complete only when every declared output file exists, so a third entry written only on `BLOCKING` would make every approval classify non-complete, degrade, and render `shedengine.Done` unreachable.
  Fill the eight judge markers — `rubric`, `artifacts`, `round` as `strconv.Itoa(n)`, `report_path`, `previous_ledger`, `verdict_path`, `ledger_path`, `focus_path` — with `stencil.Fill`; a fill failure degrades.
  Archive stale outputs over all three paths via `archiveStaleOutputs`, degrading on failure; this is what clears any debris from an unfinished earlier judge call so `shuttleengine`'s own spec validation does not reject a pre-existing output file.
  Build a `shuttleengine.Spec` with the filled prompt, those three `OutputFiles`, the `Model`/`Effort`/`Version` triple verbatim, `Role: "bouncer-judge"`, and `Round: strconv.Itoa(n)`; run it through `b.cfg.Shuttle`.
  **Harvest**: after the run returns, and before classifying its outcome, evaluate `judged(n)` against what is now on disk.
  When it holds, call `settle(ctx, n, true)` regardless of what the run reported — a judgment that provably happened is acted on rather than discarded.
  Only when it does not hold does a run error, or a non-`OutcomeDone` outcome, or an unreadable or unparseable verdict or ledger, `degrade`.

  The pointer rule, which every return above already satisfies and which belongs as a comment on `Call`: `OutputPointer.Path` names a file this producer has verified exists, or it is empty.
  `shedengine.Done` and a `BLOCKING` `shedengine.Stuck` are reachable only through harvest or replay, both of which require `judged(n)`, which requires the ledger to exist and parse.
  Every other outcome — the seed call, the re-bounce, every degraded path, every error return — reports an empty pointer.
- **Commit:** `feat(shedadapters): implement Bouncer.Call with seed, re-bounce, judge, and replay modes`

### Card 7: seed-side and prompt-shape tests

- **Context:**
  - `internal/shedadapters/bouncer.go`
  - `internal/shedadapters/bouncerfiles.go`
  - `internal/shedadapters/round.go`
  - `internal/shedadapters/singlellm_test.go`
  - `internal/shedadapters/archive_test.go`
  - `internal/shedadapters/bouncer_config_test.go`
  - `contracts/stencils/bouncer/bouncer-template-seed.md`
  - `contracts/stencils/bouncer/bouncer-template-judge.md`
  - `internal/shuttleengine/spec.go`
  - `internal/shedengine/producer.go`
  - `_mill/discussion.md`
- **Edits:** none
- **Creates:**
  - `internal/shedadapters/bouncer_seed_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create `internal/shedadapters/bouncer_seed_test.go`, reusing the existing `fakeShuttle` from `internal/shedadapters/singlellm_test.go` (its `duringRun` hook is what writes the files a real agent would have written) and `fixedClock` from `internal/shedadapters/archive_test.go`.
  Reuse the stencils-fixture helper card 5 added in `internal/shedadapters/bouncer_config_test.go`, and seed it with the real template bytes read from `contracts/stencils/bouncer/bouncer-template-seed.md` and `contracts/stencils/bouncer/bouncer-template-judge.md` so the marker contract is pinned against the shipped templates rather than against a stand-in.

  Cover these scenarios, each asserting on the recorded `shuttleengine.Spec` as well as on the returned outcome, pointer, and error.
  Seed call, happy path: an empty run dir returns `shedengine.Stuck`, an empty pointer, and a nil error; `round-1-focus.md` exists and parses afterwards; exactly one spawn happened, against the seed template; no verdict and no ledger file was written.
  Seed discriminator parses rather than merely stats: `round-1-focus.md` present but unparseable with no report on disk is a seed call — the fake was invoked and the malformed file was archived — not a re-bounce.
  Seed call, spawn produced nothing usable: one case per failure mode — the seed template unreadable, the rubric unreadable, a fill failure arranged by a template declaring a marker the Go side does not supply, a `Run` error, a non-`OutcomeDone` outcome, the agent writing nothing, and the agent writing an unparseable focus file.
  Each returns `shedengine.Stuck` with a nil error, and `round-1-focus.md` afterwards carries `round: 1` with both lists empty.
  Seed-side harvest: the fake writes a valid, non-empty `round-1-focus.md` and *then* reports a `Run` error, and separately a non-`OutcomeDone` outcome; assert the agent's file survives byte-identical and was not replaced by the empty-lists fallback.
  Re-bounce: no report on disk but a present, parsing `round-1-focus.md` returns `shedengine.Stuck` with an empty pointer and a nil error, the fake was never called, and the focus file is left byte-identical, neither archived nor re-seeded.
  Marker completeness, both templates: fill each shipped template through the same values the Go call site supplies and assert `stencil.Fill` returned no error, then assert the filled prompt contains every path in that template's marker map.
  Include the first-round judge case asserting `previous_ledger` renders as the literal `(none)`.
  Stamp-leak regression, both templates: give the rubric fixture a realistic `<!-- lyx-stencil: sha256=... -->` banner and assert the filled prompt contains no `<!-- lyx-stencil:` substring.
  `Spec` identity: `Role` is `"bouncer-seed"` on a seed call and `"bouncer-judge"` on a judge call, and `Round` is the decimal round as a string in each.
  `Spec` passthrough: `Model`, `Effort`, and `Version` arrive verbatim from the config, and every `OutputFiles` entry is absolute.
- **Commit:** `test(shedadapters): cover the Bouncer's seed call, re-bounce, and prompt shape`

### Card 8: package doc

- **Context:**
  - `internal/shedadapters/bouncer.go`
  - `_mill/discussion.md`
- **Edits:**
  - `internal/shedadapters/doc.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Amend `internal/shedadapters/doc.go` in exactly the five places below, leaving its `# The perch run-id scheme` section untouched — that section is `PerchProducer`-specific.

  Opening sentence: "three" becomes four, with `Bouncer` named alongside `SingleLLMProducer`, `PerchProducer`, and `WebsterProducer`.
  Amend the sentence "each is a thin translation layer over an already-shipped engine, never a second implementation of that engine's own loop" rather than leaving it standing: it is false of the Bouncer, which is new logic over `shuttleengine` and composes its own prompt from stencils.
  Name the Bouncer as the one member that is not a wrapper around an already-shipped engine.

  `# Outcome mapping`: add a fourth bullet covering all four `Call` modes (seed, re-bounce, judge, replay), the harvest step that acts on a judgment that provably happened regardless of the shuttle's own classification, and the pointer rule — the ledger path on `Done` and on a `BLOCKING` `Stuck`, empty everywhere else.
  State the deliberate delta versus `PerchProducer`, which reports an empty pointer because a gate producer's verdict is re-derived rather than read back, and why the Bouncer differs: its ledger is a real cross-round artifact a human reads, and hiding it on a `BLOCKING` `Stuck` would hide it exactly when an operator most needs it.
  State the exists-or-empty rule and why it matters: `Shed` never stats a pointer, so a pointer naming an unwritten file is caught nowhere and is simply persisted into the history for a human to read as though the artifact were there.

  `# Told, never derived`: add the Bouncer's told inputs — `RunDir`, `StencilsDir`, the resolved `(Model, Effort, Version)` triple, and the report-name convention as a function.
  Note that this is the one constructor in the package returning an error, and why: the other three either validate nothing or take already-constructed engines that validated themselves, whereas the Bouncer has eleven inputs with real invariants, two of which must be absolute paths, and validating lazily at first `Call` would turn a wiring typo into a mid-run failure in an unattended segment.

  `# Shared cancellation rule`: add the Bouncer, noting that its genuine-success exception covers a *harvested* verdict, not only a `shuttleengine` completion — a parsed verdict is returned as its mapped `Done` or `Stuck` with its pointer regardless of cancellation.

  `# Limitations`: add the two soft spots this design accepts.
  Ledger carry-forward is enforced by the judge prompt alone, so a misbehaving judge can drop an entry with nothing at the Go layer catching it; closing that would require diffing the new ledger's key set against the previous one and deciding what a missing key means, which is a feature rather than a one-line addition.
  The Bouncer installs no mid-run cancellation bridge, the same limitation `SingleLLMProducer` and `WebsterProducer` already record.
- **Commit:** `docs(shedadapters): record the Bouncer in the package documentation`

## Batch Tests

`verify: go test ./internal/shedadapters/...` runs the whole package, which after this batch adds `bouncer_config_test.go` and `bouncer_seed_test.go` to batch 1's two files and the five pre-existing ones (`archive_test.go`, `ctx_test.go`, `perch_test.go`, `singlellm_test.go`, `webster_test.go`).
Package-wide scope is correct rather than over-broad: Go's test unit is the package, and this batch's cards edit one package whose suite is fast, fake-driven, and filesystem-only.
Card 5's constructor table is the guard against a wiring typo surfacing mid-run; card 7's marker-completeness and stamp-leak cases are the guard against the prompt contract drifting between the shipped templates and the Go call site.
The judge, replay, harvest, degradation, pointer-discipline, stale-output, and cancellation scenarios are deliberately left to batch 4, which adds only test files.
No real spawn and no smoke test: the other three adapters have none, and a real spawn would cross the Test Tier Purity Invariant's boundary for no coverage the fake does not already give.
