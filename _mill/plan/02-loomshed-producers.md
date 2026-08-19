# Batch: loomshed-producers

```yaml
task: 'loom: phase-machine scaffolding'
batch: loomshed-producers
number: 2
cards: 8
verify: go test ./internal/loomshed/... ./internal/lyxcwd/... ./cmd/lyx/...
depends-on: [1]
```

## Batch Scope

This batch creates `internal/loomshed`: the package that owns loom's 12-row producer list, the three producers the task builds for real, the two thin wrappers over already-shipped engines, the seven stubs, the `Deps`/`New` constructor that assembles them, and `Seed`, which writes the initial status file nothing in production writes today.
It is one batch because every card lands in one new package with one shared cancellation helper and one shared naming table, and because none of the pieces is independently useful — a producer with no list to sit in, or a list with no seeder, cannot be exercised at all.

The external interface batch 3 consumes: `loomshed.New(Deps) (*shedengine.Shed, error)`, `loomshed.Seed(statusPath, statusLockPath, slug, parent string) error`, and `loomshed.NewPreflightProducer(cwd string) shedengine.ShedProducer`.

Batch-local decisions:

- Card ordering is bottom-up on purpose — the cancellation helper, then the stub, then each real producer, then the list that names them all, then the seeder. Card 9 is the only card that can fail to compile on a naming mistake in an earlier card, which is where the list-shape test lives.
- Cards 8 and 10 carry no untagged test of their own, for stated reasons in each card. Their coverage lands in batch 3.

## Cards

### Card 3: create `internal/loomshed` with its cancellation helper and import guard

- **Context:**
  - `internal/shedadapters/ctx.go`
  - `internal/shedengine/seam_enforcement_test.go`
  - `internal/shedengine/producer.go`
  - `internal/shedengine/doc.go`
- **Edits:**
  - `CONSTRAINTS.md`
  - `docs/overview.md`
- **Creates:**
  - `internal/loomshed/doc.go`
  - `internal/loomshed/ctx.go`
  - `internal/loomshed/ctx_test.go`
  - `internal/loomshed/leaf_enforcement_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create package `loomshed` at `internal/loomshed`. Its `doc.go` carries the package comment: `loomshed` owns loom's own ordered producer list and returns a constructed `*shedengine.Shed`; it takes told absolute paths and has no direct production import of `internal/lyxcwd`; it declares its own cancellation helpers rather than reusing `internal/shedadapters`' unexported ones, and states that duplication is deliberate. `ctx.go` declares two unexported helpers with the same contract as their `shedadapters` counterparts but their own message prefix: `entryErr(ctx context.Context, name string) error` returns nil when `ctx.Err()` is nil and otherwise a wrapped error naming the producer and stating the run never started, and `cancelErr(ctx context.Context, name string) error` returns nil when `ctx.Err()` is nil and otherwise a wrapped error naming the producer and stating the context was cancelled during the run. Both wrap `ctx.Err()` with `%w`. Every real producer in this package calls `entryErr` before doing anything and consults `cancelErr` on every non-success exit path, which is what discharges the obligation `Shed` cannot enforce: a `Stuck` returned under a cancelled context is indistinguishable to `Shed` from a genuine verdict and would silently consume bounce budget for what was an operator stop. `ctx_test.go` covers both helpers: nil context error yields nil, a cancelled context yields a non-nil error that `errors.Is`-matches `context.Canceled` and whose text names the producer. `leaf_enforcement_test.go` is the Told-Geometry import guard, modelled on `internal/shedengine/seam_enforcement_test.go`'s `TestProducerSeamInvariant_AllowlistOnly` and named `TestToldGeometryInvariant_AllowlistOnly`: it walks every non-`_test.go` `.go` file in the package with `parser.ParseFile(..., parser.ImportsOnly)` and fails on any non-stdlib import outside an explicit allowlist. The allowlist is `internal/shedengine`, `internal/shedadapters`, `internal/websterengine`, `internal/loomengine`, `internal/planparser`, `internal/batcher`, and `internal/state`. An allowlist rather than a bare `internal/lyxcwd` denylist, matching the in-repo precedent: it catches the excluded import and anything else that would drag geometry resolution in, with no list maintenance beyond a genuine new dependency. Its file comment states that `internal/loomengine` on the allowlist is legal despite `loomengine` itself importing `internal/lyxcwd`, because the invariant's membership predicate is about a direct production import and transitive is explicitly fine. In `CONSTRAINTS.md`, add `internal/loomshed` to the Told-Geometry Invariant's **Machine-enforced** bullet, naming the test exactly as the existing entries name theirs, and add it to the same section's "Enforced by" count. In `docs/overview.md`, add an `internal/loomshed/` row to the internal-package tree, placed immediately after the existing `internal/shedadapters/` row, describing it as loom's own 12-row producer list over `shedengine`.
- **Commit:** `feat(loomshed): create the package with its cancellation helper and import guard`

### Card 4: the stub producer

- **Context:**
  - `internal/shedengine/producer.go`
  - `internal/loomshed/ctx.go`
- **Edits:**
  - `manifest/designs/loom.md`
- **Creates:**
  - `internal/loomshed/stub.go`
  - `internal/loomshed/stub_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/loomshed/stub.go`, declare an unexported `stubProducer` struct holding only its told `name`, a `newStub(name string) *stubProducer` constructor, and a `Call` implementing `shedengine.ShedProducer` that returns `shedengine.Done` with an empty `shedengine.OutputPointer` and a nil error — after consulting `entryErr`, so even a stub honours the cancellation obligation rather than reporting a verdict for a run an operator already stopped. Add a compile-time `var _ shedengine.ShedProducer = (*stubProducer)(nil)` assertion, matching `shedadapters`' own style. Its doc comment states which seven rows it backs — `Discussion-Write`, `Discussion-Review`, `Plan-Sweep`, `Plan-Write`, `Plan-Review`, `Webster-Review`, `Finalize` — and that each is replaced by a real producer in a later task, so the list's sequencing, resume, crash-recovery and pause behaviour is real from the start rather than retrofitted. `stub_test.go` asserts `Call` on a healthy context returns exactly `Done`, an empty pointer, and a nil error, and that `Call` on an already-cancelled context returns a non-nil error rather than `Done`. In `manifest/designs/loom.md`, correct the line reading `This assertion lands with Shed.` in the `**The Plan-never-reads-support-log boundary is not a per-run check.**` paragraph: it is false as of this task. The assertion is over `Plan-Write`'s declared input set, and a stub declares no input set at all, so there is nothing to assert against — writing it now would either assert a vacuous truth or invent a declaration the real producer has not yet made. Say instead that it lands with the real `Plan-Write`.
- **Commit:** `feat(loomshed): add the stub producer backing the seven not-yet-built rows`

### Card 5: build `Discussion-Validate`

- **Context:**
  - `internal/shedengine/producer.go`
  - `internal/loomshed/ctx.go`
  - `internal/loomengine/config.go`
  - `contracts/stencils/loom/loom-template-discussion.md`
- **Edits:** none
- **Creates:**
  - `internal/loomshed/discussionvalidate.go`
  - `internal/loomshed/discussionvalidate_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/loomshed/discussionvalidate.go`, declare an unexported `discussionValidate` struct holding the two told absolute paths — the decision record and the support log — plus its told name, a `newDiscussionValidate(name, decisionRecordPath, supportLogPath string) *discussionValidate` constructor, and a `Call` implementing `shedengine.ShedProducer`. `Call` runs exactly two checks, and nothing beyond them is this producer's to look for: first, both files exist; second, the decision record contains all seven required H2 sections — `## Goal`, `## Scope`, `## Decisions`, `## Constraints`, `## Auto-mode assumptions`, `## Open risks`, `## Acceptance criteria`. Any failure maps to `shedengine.Stuck` with an empty pointer; both checks passing maps to `shedengine.Done`, reporting the decision record's path as the pointer. A read failure that is not a not-exist is a returned error, not `Stuck` — a `Stuck` would bounce back to `Discussion-Write`, which cannot fix an I/O fault. Section matching is per-line on the exact H2 heading text after trimming trailing whitespace, so a heading nested inside a fenced block or appearing mid-sentence never counts. Three things are deliberately NOT checks, and each needs a comment saying so rather than being silently absent: `## Notes for the plan writer` is optional by contract and its absence is never a violation; section *order* is pinned in the stencil but is not validated here; an extra unexpected H2 is not a violation either. `Call` calls `entryErr` first and consults `cancelErr` on every non-`Done` exit. The two paths are told rather than derived because `loomengine`'s own accessors for them take a `*lyxcwd.Location`, which this package may not import. `discussionvalidate_test.go` is table-driven over a `t.TempDir()`: both files present with all seven sections passes; each file missing in turn is `Stuck`; each of the seven sections missing in turn is `Stuck`; `## Notes for the plan writer` present and absent both pass; sections present but out of order passes; an extra unexpected H2 passes; a cancelled context returns an error rather than a verdict.
- **Commit:** `feat(loomshed): build the Discussion-Validate producer`

### Card 6: build `Plan-Validate`

- **Context:**
  - `internal/shedengine/producer.go`
  - `internal/loomshed/ctx.go`
  - `internal/planparser/parse.go`
  - `internal/planparser/validate.go`
  - `internal/planparser/doc.go`
- **Edits:** none
- **Creates:**
  - `internal/loomshed/planvalidate.go`
  - `internal/loomshed/planvalidate_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/loomshed/planvalidate.go`, declare an unexported `planValidate` struct holding its told name, the told anchor path, and the told worktree root, a `newPlanValidate(name, anchorPath, worktreeRoot string) *planValidate` constructor, and a `Call` implementing `shedengine.ShedProducer`. `Call` is a thin wrap and nothing more: `planparser.ParsePlan(planparser.PlanDir(p.anchorPath))`, then `planparser.Validate(plan, p.worktreeRoot)`. A non-empty `[]planparser.ValidationError` maps to `shedengine.Stuck` with an empty pointer; an empty slice maps to `shedengine.Done`, reporting the plan directory as the pointer. A `ParsePlan` error maps to a returned error, never to `Stuck`: a plan that will not parse is not a plan the `Plan-Write` bounce target can be asked to improve, and the two dispositions differ materially — `Stuck` persists `blocked`, a returned error persists `failed` and aborts the run. The two paths are separate fields because `planparser.PlanDir` takes the anchor path and `planparser.Validate` takes the worktree root, and they are not the same value. The Planparser Sole-Parser Invariant means no plan parsing whatsoever may be written here — this producer calls `planparser` and maps its result, nothing more. `Call` calls `entryErr` first and consults `cancelErr` on every non-`Done` exit. `planvalidate_test.go` builds plan fixtures under a `t.TempDir()` and covers: a plan `planparser.Validate` returns zero findings for maps to `Done`; a plan it returns at least one finding for maps to `Stuck`; an unparseable plan directory maps to a non-nil error rather than `Stuck`; a cancelled context returns an error rather than a verdict.
- **Commit:** `feat(loomshed): build the Plan-Validate producer`

### Card 7: build the `Batchifier` gate and the lazy `Webster` wrapper

- **Context:**
  - `internal/shedengine/producer.go`
  - `internal/loomshed/ctx.go`
  - `internal/batcher/config.go`
  - `internal/batcher/batcher.go`
  - `internal/batcher/template.go`
  - `internal/shedadapters/webster.go`
  - `internal/websterengine/runlevel.go`
  - `internal/configengine/config.go`
- **Edits:**
  - `manifest/designs/loom.md`
- **Creates:**
  - `internal/loomshed/batchifier.go`
  - `internal/loomshed/webster.go`
  - `internal/loomshed/batchifier_test.go`
  - `internal/loomshed/webster_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/loomshed/batchifier.go`, declare an unexported `batchifier` struct holding its told name and told anchor path, a `newBatchifier(name, anchorPath string) *batchifier` constructor, and a `Call` implementing `shedengine.ShedProducer` that calls `batcher.Active(b.anchorPath)` and maps every error to `shedengine.Stuck`, success to `shedengine.Done`, in both cases reporting an empty `shedengine.OutputPointer` — this row is a fail-fast gate and writes no artifact. Nothing is handed across to the `Webster` row: `Call` returns only `(Outcome, OutputPointer, error)`, so there is no channel for a handover in the first place. `batcher.Active` returns a bare error for unknown-name, malformed YAML and I/O failure alike with no sentinel to discriminate on, so all three conflate onto `Stuck`; state that conflation in a comment as accepted, with its reason — `Active` already falls back to the embedded `batcher.ConfigTemplate()` when the config file or its directory is absent, so a remaining error is a genuinely broken config far more often than an infra fault, and `blocked` is the right resting state for an operator-fixable fault. In `internal/loomshed/webster.go`, declare an unexported `websterProducer` struct holding its told name, told anchor path, a `shedadapters.WebsterRunner`, and a `websterengine.RunDeps` whose own `Batcher` field is left nil, plus a `newWebsterProducer(name, anchorPath string, run shedadapters.WebsterRunner, deps websterengine.RunDeps) *websterProducer` constructor and a `Call` that resolves `batcher.Active(w.anchorPath)` itself, inside the call, then fills the resolved value into a copy of its `RunDeps`, constructs `shedadapters.NewWebsterProducer(w.name, w.run, deps)` and delegates to that producer's own `Call`. A `batcher.Active` error here maps to `shedengine.Stuck` identically to row 9's gate, never to a returned error: the two outcomes differ materially in `shedengine.Run`, since `Stuck` under `OnStuck: ""` persists `blocked` and returns `RunBlocked`, which a human resumes after fixing the config, whereas returning the error persists `failed` and aborts the run — the same fault must not end the run one way before Webster and another way at Webster. Resolution is lazy, not injected at construction, and the comment must say why: `shedadapters.NewWebsterProducer` takes `websterengine.RunDeps` by value, so injecting a resolved `Batcher` would require `batcher.Active` to have already succeeded before `Shed.Run` ever starts, which makes the gate's stated value — catching a broken config before Webster spawns LLM sessions — unreachable; and after a crash-restart with `current_producer` naming `Webster`, the gate never re-runs in the new process, so an injected value would have to be re-resolved anyway. State the consequence explicitly as correct behaviour rather than staleness: if the batch config changes between the two rows, the second uses the newer config, because there is no cached value to go stale — the gate's guarantee is precisely "the config was resolvable at row 9", never "the config Webster will use is the one row 9 saw". Both `Call`s call `entryErr` first and consult `cancelErr` on every non-`Done` exit. `batchifier_test.go` covers: a valid config maps to `Done` with an empty pointer; an unknown batchifier name maps to `Stuck`; a malformed config file maps to `Stuck`; an absent config file and an absent `_lyx` directory each map to `Done` rather than `Stuck`, since `batcher.Active` resolves the embedded template — assert this explicitly so the fallback is never mistaken for a gate failure; a cancelled context returns an error. `webster_test.go` drives a fake `shedadapters.WebsterRunner` and covers: a `batcher.Active` error maps to `Stuck` and not to a returned error; a successful resolution reaches the injected runner with a non-nil `Batcher` in the `RunDeps` it receives; and the wrapper holds no value resolved at construction — mutate the batch config between two `Call`s on the same wrapper and assert the second call observes the new config. In `manifest/designs/loom.md`, correct row 9's Output column from `batch grouping handed to Webster` to a gate description, and row 10's Input column from `batch grouping` to what Webster actually resolves for itself. The design table described a mechanism that cannot exist, so the doc is wrong rather than merely unimplemented.
- **Commit:** `feat(loomshed): build the Batchifier gate and the lazy Webster wrapper`

### Card 8: wire in `Preflight` behind an exported wrapper

- **Context:**
  - `internal/shedengine/producer.go`
  - `internal/loomshed/ctx.go`
  - `internal/loomengine/preflight.go`
  - `internal/loomengine/report.go`
  - `internal/preflight/report.go`
- **Edits:** none
- **Creates:**
  - `internal/loomshed/preflight.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/loomshed/preflight.go`, declare an unexported `preflightProducer` struct holding its told name and told cwd, plus the exported constructor `NewPreflightProducer(cwd string) shedengine.ShedProducer`, whose `Call` invokes `loomengine.Preflight(p.cwd)` and maps its result: a `Report` with `OK` true to `shedengine.Done` with an empty pointer, a `Report` with `OK` false to `shedengine.Stuck` with an empty pointer, and a non-nil error to a returned error. That mapping is the whole producer — `Preflight` reports a determined verdict rather than erroring on anything short of an infra failure, so its `OK` false is a verdict to route and its error is an undetermined failure to escalate. `Call` calls `entryErr` first and consults `cancelErr` on every non-`Done` exit. The constructor is exported and owned here, rather than left to the not-yet-built session-bootstrap caller, so this task has a `Preflight` row something can actually construct; `Deps.Preflight` in card 9 stays typed as a bare `shedengine.ShedProducer` so a Tier-1 test injects a fake instead. This card adds the production import of `internal/loomengine` the guard test in card 3 already allowlists — that import does not compromise the package's Told-Geometry position, since the invariant's membership predicate is about a direct production import of `internal/lyxcwd` and transitive is explicitly fine. This card carries no untagged test: `loomengine.Preflight` resolves a real worktree and spawns git, which the Test Tier Purity Invariant forbids an untagged file from doing. Its outcome-mapping coverage is the integration-tagged test in batch 3.
- **Commit:** `feat(loomshed): wire in Preflight behind NewPreflightProducer`

### Card 9: `Deps` and `New` — the 12-row producer list

- **Context:**
  - `internal/shedengine/shed.go`
  - `internal/shedengine/producer.go`
  - `internal/shedengine/validate.go`
  - `internal/shedadapters/webster.go`
  - `internal/websterengine/runlevel.go`
  - `internal/loomshed/stub.go`
  - `internal/loomshed/discussionvalidate.go`
  - `internal/loomshed/planvalidate.go`
  - `internal/loomshed/batchifier.go`
  - `internal/loomshed/webster.go`
  - `manifest/designs/loom.md`
- **Edits:** none
- **Creates:**
  - `internal/loomshed/loomshed.go`
  - `internal/loomshed/loomshed_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/loomshed/loomshed.go`, declare the exported `Deps` struct with exactly these fields, and no others: `StatusPath`, `LockPath`, `StatusLockPath string` and `MaxBounces int`, Shed's own told values; `AnchorPath string`, which feeds both the plan directory and the batch-config lookup; `WorktreeRoot string`, kept separate because `planparser.Validate` genuinely takes a different value from `planparser.PlanDir`; `DecisionRecordPath` and `SupportLogPath string`, told rather than derived because `loomengine`'s accessors for them take a `*lyxcwd.Location` this package may not import; `Preflight shedengine.ShedProducer`, injected pre-constructed because it is the only row that spawns git; and `WebsterRun shedadapters.WebsterRunner` plus `WebsterDeps websterengine.RunDeps`, injected as parts rather than as a `ShedProducer` because the lazy wrapper around them is owned here. There is deliberately no separate base-directory field: the batch-config lookup stats an `_lyx` directory under the directory it is given and returns it unchanged, which is exactly the directory the plan-directory constructor anchors on, so two fields would only invite silent divergence. Declare exported `const`s for the twelve producer names, verbatim per the shared decision, and use them everywhere rather than repeating string literals — the name is the durable on-disk identity in `current_producer`, and a later rename breaks resume for any in-flight task. Declare `New(deps Deps) (*shedengine.Shed, error)`, which builds the list in table order and returns a `*shedengine.Shed` carrying it plus the four told Shed fields. The twelve rows, with their backing and `OnStuck` target: 1 `Preflight`, `deps.Preflight`, `""`; 2 `Discussion-Write`, stub, `""`; 3 `Discussion-Validate`, real, bouncing to `Discussion-Write`; 4 `Discussion-Review`, stub, bouncing to `Discussion-Write`; 5 `Plan-Sweep`, stub, `""`; 6 `Plan-Write`, stub, `""`; 7 `Plan-Validate`, real, bouncing to `Plan-Write`; 8 `Plan-Review`, stub, bouncing to `Plan-Write`; 9 `Batchifier`, real, `""`; 10 `Webster`, the lazy wrapper, `""`; 11 `Webster-Review`, stub, bouncing to `Webster`; 12 `Finalize`, stub, `""`. Document the routing rule the table follows rather than leaving twelve unexplained values: every gate and validator bounces back to the producer whose artifact it guards, and a gate whose guarded artifact is produced by no row in the list escalates instead — `Preflight` gates git and filesystem state and `Batchifier` gates the batch config, neither of which any row writes, so there is nothing to bounce to and a human is the only thing that can fix either. `New` returns an error when `deps.Preflight` is nil, since a nil row would otherwise panic at the call step rather than failing loud; it does not otherwise pre-validate, because `shedengine.Run` validates every field before it touches anything. `loomshed_test.go` asserts, against a `Deps` carrying a fake `Preflight` and a fake `WebsterRunner`: the list is exactly twelve rows, with the names verbatim in table order and the `OnStuck` map above, asserted against a literal table so a reordering or a rename is a test failure, since both break resume; the constructed `Shed`'s four told fields carry the values `Deps` supplied; a nil `deps.Preflight` returns an error; and the constructed list passes `shedengine`'s own pre-run validation — the cheapest guard there is against a typo'd `OnStuck`, a duplicate name, or two lock paths naming one file, and it must be asserted explicitly rather than relied on implicitly. Since `Shed.validate()` is unexported, drive it through a `Run` call against a status file the test seeds, and assert the run does not fail with a validation error rather than asserting the validation result directly.
- **Commit:** `feat(loomshed): assemble loom's 12-row producer list behind Deps and New`

### Card 10: `Seed` — write the initial status file

- **Context:**
  - `internal/shedengine/status.go`
  - `internal/shedengine/run.go`
  - `internal/state/state.go`
  - `internal/loomengine/status.go`
  - `internal/loomengine/preflight.go`
- **Edits:** none
- **Creates:**
  - `internal/loomshed/seed.go`
  - `internal/loomshed/seed_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/loomshed/seed.go`, declare `Seed(statusPath, statusLockPath, slug, parent string) error`, which writes the initial status file: a `shedengine.Status` with `CurrentProducer` naming `Preflight`, `State: shedengine.StateRunning`, an empty non-nil `History`, `PauseRequested: false`, and a `Product` carrying a marshalled `loomengine.Status` with the given slug and parent and a nil `StartSha`. `Seed` takes bare told paths rather than a `Deps` because seeding happens before any `Shed` exists, so a `Deps` would couple this seam to a struct whose producer fields are irrelevant to it. The state value is pinned rather than left open: `shedengine`'s five-member state enum hard-rejects the empty string at its read gate, so an unpinned seed is a hard error at Shed's first read, and `StateRunning` is the only member that means "a run may proceed from here" — `paused`, `done`, `blocked` and `failed` all describe a run that has already happened. `Seed` refuses when the file already exists, returning an error rather than overwriting, because overwriting silently destroys an in-flight run's history and the whole resume contract rests on that history; a deliberate re-seed is then an explicit operator act, never an accident. Two write mechanics are load-bearing and both are silent failures if missed. First, the refuse-if-exists decision is made under the held lock, via `state.UpdateJSON`'s own `found` argument — return an error when `found` is true — and never as a stat followed by `state.WriteJSON`, which is a TOCTOU window between the check and the write. Second, `Seed` creates the lock file's parent directory before acquiring, because `internal/state` creates the *status* file's parent but not the *lock* file's — the same gap `loomengine`'s own check-4 read already works around at its lock acquisition. `seed_test.go` covers, over a `t.TempDir()`: the exact status written, asserting `current_producer`, `state`, an empty history, a false pause flag, and the product payload round-tripping back out of `json.RawMessage` with the slug and parent it was given and a null start sha; a second `Seed` against an existing file returns an error and leaves the file byte-identical; and `Seed` succeeds when the lock file's parent directory does not yet exist.
- **Commit:** `feat(loomshed): add Seed, the production writer for the initial status file`

## Batch Tests

`verify:` runs `go test ./internal/loomshed/... ./internal/lyxcwd/... ./cmd/lyx/...`.

- `./internal/loomshed/...` is the batch's own new untagged suite: `ctx_test.go`, `stub_test.go`, `discussionvalidate_test.go`, `planvalidate_test.go`, `batchifier_test.go`, `webster_test.go`, `loomshed_test.go`, `seed_test.go`, and the `leaf_enforcement_test.go` import guard. Every one of these is offline — temp dirs and fakes only, no git spawn — so the whole package stays Tier 1.
- `./cmd/lyx/...` covers `tierpurity_test.go`, which is what actually enforces that claim across the new files, and `hermeticenv_test.go`.
- `./internal/lyxcwd/...` covers `docslink_test.go`, the Markdown Link Integrity guard over the `CONSTRAINTS.md`, `docs/overview.md` and `manifest/designs/loom.md` edits cards 3, 4 and 7 make.
- No integration-tagged pass here: this batch creates no tagged file. Card 8's wrapper is covered by the tagged test batch 3 adds.
