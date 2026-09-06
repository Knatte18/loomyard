# Batch: webster-drift

```yaml
task: "Adopt quarry's glyph alphabet as the plan alphabet"
batch: "webster-drift"
number: 7
cards: 9
verify: go test ./internal/planglyph/ ./internal/websterengine/ ./internal/webstercli/ ./cmd/lyx/ && go test -tags integration ./internal/planglyph/ ./internal/websterengine/
depends-on: [6]
```

## Rename mechanic

_This batch contains no `Moves:` entry; the section is retained for the batch template's own structure and imposes nothing on the implementer._

## Batch Scope

This batch adds the two webster re-resolution boundaries and everything they carry: the `DeltaGit` call site, the done-checks, handle binding at card completion, the glyph scope guard, and drift detection with exact-tier auto-repair and evidence-tier surfacing.
It is one batch because all six consumers read the **same** `record-batch` delta — one `DeltaGit(BatchState.StartSHA, report.HeadSHA, ".")` call serving handle binding, done-checks and the scope guard at once — and splitting them would mean either three delta calls or a half-wired boundary that reports success it did not verify.
The two guard extensions and the two stencil rewrites land here too, because they are only true once the `DeltaGit` call site and the mechanical scope guard exist.

Batch-local decisions, beyond `## Shared Decisions`:

- **Neither boundary is a new `ShedProducer` row.** `internal/shedrecipe`'s registry holds no `BeginBatch` or `RecordBatch` entry — `begin-batch` and `record-batch` are Master's bracket CLI verbs in `internal/webstercli` over `internal/websterengine` functions. Both get their re-resolution as `websterengine` code called by the already-existing verbs, taking the root from `websterengine.Geometry.WorktreeRoot`, which those call sites already carry.
- **`Plan-Revalidate` is a one-shot pre-Webster baseline, not the per-merge boundary.** The recipe places that row once, after the review segment, before `Batchifier` and `Webster`, so it runs strictly before any card has merged. The per-merge `Resolve` folds into `record-batch`, where webster runs cards sequentially and both SHAs are already held and cross-checked.
- **The `"."` whole-repo delta range is right, not wasteful.** The scope guard needs the whole-repo delta anyway, and narrowing it would blind the one consumer that exists to catch out-of-scope changes.

## Cards

### Card 31: add the DeltaGit call site and log its spawn

- **Context:**
  - `internal/planglyph/repo.go`
  - `internal/planglyph/doc.go`
  - `internal/logger/logger.go`
  - `CONSTRAINTS.md`
- **Edits:** none
- **Creates:**
  - `internal/planglyph/delta.go`
  - `internal/planglyph/delta_integration_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add this package's one and only `DeltaGit` call site.
  In `delta.go`, add `Delta(worktreeRoot, fromSHA, toSHA string) (quarry.GitDeltaAnswer, error)` opening the repository through `openRepo` and calling `(*quarry.Repo).DeltaGit(fromSHA, toSHA, ".")`.
  The `"."` target is the whole-repository range and is deliberate: the scope guard in card 35 needs the whole-repo delta, and narrowing the range would blind the one consumer that exists to catch out-of-scope changes.
  Wrap a non-nil error with `ErrQuarryUnavailable` from card 21, so a `DeltaGit` failure is the same infrastructure category as an `Open` or `Resolve` failure and can never be read as an empty delta.
  Log the spawn with `logger.Info` immediately before the `DeltaGit` call, naming the two SHAs and the worktree root: quarry's own `internal/gitsrc` runs `exec.Command("git", ...)`, so this call starts a real OS process on a path reachable from `lyx webster record-batch`, which the Live-Substrate Spawn Observability invariant covers.
  `Info` is the right level because this is a lifecycle spawn rather than a spawn inside a polling probe.
  The invariant's allowlist route is unavailable and must not be used: it permits an exemption only for a site structurally barred from importing `internal/logger`, which this package is not, and an allowlist entry would record "this spawns without logging, and we accepted it" when logging costs one line.
  There is no teardown to log separately — `DeltaGit` waits internally and returns an answer.
  Write the test in `delta_integration_test.go` behind a `//go:build integration` constraint, because it spawns git: build a two-commit fixture repository, call `Delta` across the two SHAs, and assert the answer's `From`/`To` echo and that a symbol added in the second commit appears in `Created`.
  Add a case asserting a bad revision returns an error satisfying `errors.Is(err, ErrQuarryUnavailable)`.
- **Commit:** `31: feat(planglyph): add the DeltaGit call site and log its git spawn`

### Card 32: re-resolve at the dispatch boundary

- **Context:**
  - `internal/planglyph/planglyph.go`
  - `internal/planglyph/repo.go`
  - `internal/websterengine/geometry.go`
  - `internal/planparser/plan.go`
- **Edits:**
  - `internal/websterengine/beginbatch.go`
  - `internal/websterengine/beginbatch_test.go`
  - `internal/webstercli/beginbatch.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Make the dispatch boundary re-resolve the plan before a pack is built, never from a cache.
  In `internal/websterengine/beginbatch.go`, add a re-resolution step inside `BeginBatch`, after the existing pause and fingerprint refusal gates and before the prompt is rendered, calling `planglyph.ValidateFormat(deps.Plan, deps.Geom.WorktreeRoot)`.
  Take the root from `deps.Geom.WorktreeRoot`, which the struct already carries and which `BeginBatch` already reads for its head-SHA capture — this adds no path derivation and no `internal/lyxcwd` import anywhere.
  An infrastructure error satisfying `errors.Is(err, planglyph.ErrQuarryUnavailable)` **blocks**: return it, so no fork is dispatched.
  Dispatching a pack built on a re-resolve that failed is strictly worse than not dispatching.
  A non-empty blocking findings set also blocks, returned as its own sentinel `ErrPlanDrifted` declared beside the file's existing `ErrPaused` and `ErrFingerprintMismatch` sentinels, so a caller distinguishes it with `errors.Is` — webster owns its own domain types.
  Informational findings never block and are carried back on `BeginResult` as a new `Advisories []string` field, so an operator sees them without the run stopping.
  In `internal/webstercli/beginbatch.go`, surface the new sentinel as its own JSON error envelope naming plan drift, distinct from the paused and fingerprint-mismatch envelopes, and emit the advisories in the success envelope.
  This adds no `internal/shedrecipe` registry entry and no new CLI verb: `begin-batch` already exists, and the re-resolution is a `websterengine` function called by it.
  Cover in `beginbatch_test.go`: a clean plan dispatching as today; a plan with one blocking finding returning `ErrPlanDrifted` and writing no prompt file; a quarry-unavailable root returning the wrapped infrastructure error rather than `ErrPlanDrifted`; and an informational-only findings set dispatching normally with the advisory carried on the result.
- **Commit:** `32: feat(websterengine): re-resolve the plan at the begin-batch dispatch boundary`

### Card 33: block the done-checks on the record-batch delta

- **Context:**
  - `internal/planglyph/delta.go`
  - `internal/planglyph/resolve.go`
  - `internal/planglyph/create.go`
  - `internal/planparser/plan.go`
  - `internal/websterengine/geometry.go`
- **Edits:**
  - `internal/websterengine/recordbatch.go`
  - `internal/websterengine/recordbatch_test.go`
  - `internal/webstercli/recordbatch.go`
- **Creates:**
  - `internal/planglyph/donecheck.go`
  - `internal/planglyph/donecheck_integration_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Give a card's completion a mechanical verdict, which the plan format has never had.
  In `donecheck.go`, add `DoneChecks(plan *planparser.Plan, cards []planparser.Card, worktreeRoot string) ([]Finding, error)` issuing one batched resolve over the completed cards' own targets and applying two rules: a `Create` group's target that still does **not** resolve is a blocking finding, check ID `create-not-done`; and a `Delete` group's target that still **does** resolve is a blocking finding, check ID `delete-not-done`.
  Both block, because neither is a judgment call — a `Create` that does not resolve is not an opinion.
  An infrastructure error from the resolve **blocks the done-checks** rather than passing them: a `Create` done-check that could not resolve is indistinguishable from a `Create` that never happened, and passing it would be a false success.
  Return the wrapped error so the caller can tell the two apart.
  Note in the file comment that the `Delete` gate would ideally also want quarry's parked `assert-no-callers`, which is not available and is not in this task's scope, so a `Delete` whose target was in fact renamed is caught by card 36's detector rather than by this check.
  In `internal/websterengine/recordbatch.go`, call `DoneChecks` after the report parse and head-SHA cross-check succeed and before the digest is persisted, using `deps.Geom.WorktreeRoot` for the root and the just-completed batch's own cards for the card set.
  A blocking finding means the batch is not done: return it as a new `ErrCardNotDone` sentinel declared beside the file's existing `ErrNoBeginRecord`, and do not persist a terminal digest.
  In `internal/webstercli/recordbatch.go`, surface that sentinel as its own JSON error envelope naming the failing check IDs, distinct from the existing `no_report` ladder signal, which stays a success envelope and exit 0.
  Write the test in `donecheck_integration_test.go` behind a `//go:build integration` constraint alongside card 31's, since the same fixture machinery builds real commits.
  Cover: a `Create` whose symbol landed passing; one whose symbol did not land producing `create-not-done`; a `Delete` whose symbol is gone passing; one whose symbol survives producing `delete-not-done`; and a quarry-unavailable root producing the infrastructure error rather than a pass.
- **Commit:** `33: feat(planglyph): add the Create and Delete done-checks and block record-batch on them`

### Card 34: bind handles from the delta at card completion

- **Context:**
  - `internal/planglyph/delta.go`
  - `internal/planglyph/donecheck.go`
  - `internal/planparser/rewrite.go`
  - `internal/planparser/plan.go`
  - `internal/websterengine/geometry.go`
- **Edits:**
  - `internal/planglyph/handle.go`
  - `internal/planglyph/handle_test.go`
  - `internal/websterengine/recordbatch.go`
  - `internal/websterengine/recordbatch_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Turn a handle into the real glyph the card actually created, from the delta rather than from anyone's spelling.
  In `internal/planglyph/handle.go`, beside card 19's canonicalization, add `BindHandles(plan *planparser.Plan, planDir string, delta quarry.GitDeltaAnswer, cards []planparser.Card) ([]Finding, error)`.
  For each completed card's `Create` declarations, match the card's expected glyph against the delta's `Created` symbols by `Symbol.ID`; on a match, build a substitution from the `plan:` handle to the matched `Symbol.ID` and apply the whole batch of substitutions through `planparser.RewriteRefs(planDir, subs)` — the same single write path canonicalization and drift repair use, because binding and rename propagation are one mechanism on different occasions.
  A count mismatch — a card declaring N handles whose delta matches fewer — is a **blocking** finding, check ID `bind-count-mismatch`, and means the card is not done; it suppresses the rewrite for that card so the plan is never half-bound.
  A handle whose expected glyph matches nothing at all degrades to card 37's candidate path rather than binding silently.
  Binding must run **after** card 33's done-checks in `recordbatch.go`, so a card that already failed `create-not-done` is never bound, and the substitution must be applied in one `RewriteRefs` call per record-batch rather than one per card, so the plan bytes are rewritten once.
  Note that handles participate in `websterengine.deriveEdges` identically to any other ref: a card whose `Uses` names `plan:X` cannot precede the card whose `Create` target is `plan:X`, by the same exact string equality, until binding rewrites both sides together — which is why binding rewrites the whole plan rather than only the declaring card.
  Cover in tests: one handle bound from a matching `Created` symbol and rewritten on both the declaring and the referencing card; two handles on one card with only one delta match producing `bind-count-mismatch` and rewriting neither; a zero-handle card producing no finding and no write; and the substitution reaching a card outside the completed batch.
- **Commit:** `34: feat(planglyph): bind plan: handles from the record-batch delta`

### Card 35: add the informational glyph scope guard

- **Context:**
  - `internal/planglyph/delta.go`
  - `internal/planglyph/donecheck.go`
  - `internal/planparser/plan.go`
  - `internal/websterengine/geometry.go`
- **Edits:**
  - `internal/websterengine/recordbatch.go`
  - `internal/websterengine/recordbatch_test.go`
- **Creates:**
  - `internal/planglyph/scope.go`
  - `internal/planglyph/scope_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Replace the informational changed-files union with a glyph-granular one, keeping its informational posture exactly.
  In `scope.go`, add `ScopeGuard(cards []planparser.Card, delta quarry.GitDeltaAnswer) []Finding` comparing the delta's `Created`, `Deleted` and `Modified` symbol IDs against the union of the completed cards' own target glyphs, and emitting one **informational** finding per symbol touched outside that union, check ID `scope-outside-plan`, naming the symbol and the file it lives in.
  It stays informational, never blocking, for two reasons that must both survive: it preserves the existing, deliberate posture that plan-predicted impact is frequently incomplete so deviation alone never fails a fork; and, when only `DeltaGit` failed, the guard is exactly the consumer that is allowed to degrade — an unavailable diff costs visibility, not correctness.
  Implement that degradation explicitly: when `recordbatch.go` holds a `DeltaGit` infrastructure error, skip `ScopeGuard` and record one informational finding stating the guard could not run, while the done-checks in card 33 still block on the same error.
  In `internal/websterengine/recordbatch.go`, collect the guard's findings into the existing `RecordResult.Warnings` slice, which is already documented as carrying non-fatal observations and is already never treated as a failure — reusing it rather than adding a parallel channel keeps one place where a non-blocking observation surfaces.
  Cover in tests: a symbol inside the card's targets producing no finding; one outside producing exactly one informational finding; a delta-unavailable path producing the guard-could-not-run finding and no panic; and a `RecordBatch` run asserting the findings land in `Warnings` and the result stays terminal.
- **Commit:** `35: feat(planglyph): add the informational glyph scope guard over the record-batch delta`

### Card 36: detect drift and auto-repair the exact tier

- **Context:**
  - `internal/planglyph/delta.go`
  - `internal/planglyph/handle.go`
  - `internal/planparser/rewrite.go`
  - `internal/planparser/amendment.go`
  - `internal/planparser/plan.go`
  - `internal/websterengine/geometry.go`
- **Edits:**
  - `internal/websterengine/recordbatch.go`
  - `internal/websterengine/recordbatch_test.go`
- **Creates:**
  - `internal/planglyph/drift.go`
  - `internal/planglyph/drift_integration_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Notice when the code moved out from under the plan, and repair only what quarry itself asserts.
  In `drift.go`, add `DetectDrift(plan *planparser.Plan, planDir string, delta quarry.GitDeltaAnswer, sha string, now string) ([]Finding, error)` computing the signal deterministically: the delta's deleted symbols intersected with the remaining plan's references.
  Apply two gates before any rewrite.
  Gate one: a rename that matches a `Rename` card's own pair is that card's expected outcome — binding, not drift — and produces no finding and no repair.
  Gate two: a renamed symbol that nothing in the remaining plan references is logged only and never rewritten.
  Past the gates, split by tier.
  An **exact-tier** detection — an entry of the delta's `Renamed` slice, which quarry populates only under its own AST-exact conditions with no threshold — auto-repairs: build the old→new substitution, apply it plan-wide through `planparser.RewriteRefs`, revalidate with one batched resolve, and append one `planparser.Amendment` per repair through `planparser.AppendAmendment`, carrying the timestamp, the card, the old and new glyph, the tier word `exact`, and the triggering SHA.
  Auto-repairing only the tier quarry itself asserts is what keeps loomyard from deciding what quarry deliberately returns as undecided.
  A deleted symbol the plan still references with no rename pair at all is a blocking finding, check ID `plan-references-deleted-symbol`.
  Take `now` and `sha` as parameters rather than reading a clock or a repository inside this function, so the whole detector is deterministic and testable without a fixture.
  In `internal/websterengine/recordbatch.go`, call `DetectDrift` after binding and before the digest is persisted, passing `bs.StartSHA`'s counterpart — the verified head SHA the file already cross-checks against the worktree's actual HEAD — as the triggering SHA.
  A blocking drift finding returns `ErrCardNotDone` from card 33, since a plan naming a symbol that no longer exists is not a plan the next card can be dispatched against.
  Write the test in `drift_integration_test.go` behind a `//go:build integration` constraint.
  Cover: an exact-tier rename rewriting the plan, revalidating clean, and appending exactly one amendment carrying all six fields; a rename matching a `Rename` card producing no drift finding and no amendment; a renamed symbol nothing references producing a log-only outcome with no rewrite; and a deleted-and-still-referenced symbol producing `plan-references-deleted-symbol`.
- **Commit:** `36: feat(planglyph): detect plan drift and auto-repair exact-tier renames with an amendment`

### Card 37: surface evidence-tier candidates without rewriting

- **Context:**
  - `internal/planglyph/delta.go`
  - `internal/planparser/rewrite.go`
  - `internal/planparser/amendment.go`
- **Edits:**
  - `internal/planglyph/drift.go`
  - `internal/planglyph/drift_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Give the undecided half of quarry's rename answer a visible disposition and no automatic one.
  Extend `DetectDrift` to read the delta's `RenameCandidates` entries and emit one **informational** finding per deleted symbol the remaining plan still references, check ID `rename-candidate`, listing each candidate's `ID`, `File` and its `Signals` — the signature-identical-modulo-name flag, the body token similarity, the two token counts, and the doc-identical flag — so a reviewer sees the mechanical evidence rather than a verdict.
  State in the finding's detail that the decision is rename-versus-genuine-delete and that it is the reviewer's, never the pipeline's.
  This tier **never** calls `planparser.RewriteRefs` and **never** calls `planparser.AppendAmendment`: an amendment records a repair, and no repair happened.
  Candidate ordering is quarry's own deterministic ordering and is explicitly not a ranking, a recommendation or a verdict — carry it through unchanged and do not re-sort, re-rank or truncate it, and do not cap a large candidate block, because a cap is a threshold under another name and the whole point of the tier split is that this side carries no threshold.
  Evidence-tier findings stay informational and never block, per the blocking-policy Shared Decision: gating on them would make loomyard decide what quarry deliberately returns as undecided.
  Cover in tests: a candidate block producing informational findings whose details carry every signal field; the plan bytes being byte-identical before and after, asserted directly rather than inferred; no amendment entry appended; and a candidate for a symbol nothing in the remaining plan references producing no finding at all, since gate two applies to this tier too.
- **Commit:** `37: feat(planglyph): surface evidence-tier rename candidates as informational findings`

### Card 38: make quarry's in-quarry git spawn visible to both guards

- **Context:**
  - `internal/planglyph/delta.go`
  - `internal/logger/logger.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `cmd/lyx/tierpurity_test.go`
  - `cmd/lyx/spawnobservability_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Close the gap that leaves both mechanical guards blind to a real OS-process spawn.
  Both guards scan **loomyard's own source** — `bannedTokens` is a raw-substring list and the observability guard AST-matches `exec.Command` call expressions — so a `DeltaGit` call, whose `exec.Command` lives inside quarry's `internal/gitsrc`, trips neither today.
  Without this card, "anything reaching `DeltaGit` is `integration`-tagged" is an assertion enforced by nothing, and a real spawn reachable from `lyx webster record-batch` sits outside the Live-Substrate Spawn Observability invariant entirely.
  In `cmd/lyx/tierpurity_test.go`, add the token `DeltaGit` to `bannedTokens`, deliberately narrow and deliberately **not** `quarry.Open`: only `DeltaGit` spawns a process, while `Resolve`, `TOC`, `Glyphs` and `Expand` read files and `Name` performs no I/O at all, so banning the constructor would force `integration` tags onto tests that spawn nothing.
  Raw-substring is the right shape here, matching that guard's own documented design.
  State the guard's inherited limit in the comment beside the new token rather than overstating it: `bannedTokens` scans untagged `*_test.go` files, so it catches a **direct textual** `DeltaGit` call site only — a test calling a `planglyph` wrapper that reaches `DeltaGit` transitively contains no such token and still passes.
  The existing tokens accept exactly this limit and this one inherits it; the guard narrows the gap, it does not close it, and a later change must not restate this as full coverage.
  In `cmd/lyx/spawnobservability_test.go`, extend `fileHasUnloggedSpawn` to count a `DeltaGit` call expression as a spawning call alongside `exec.Command`/`exec.CommandContext`, keeping the AST match rather than a substring one so a doc-comment mention is still not a call — matching that guard's own documented design, which is deliberately different from the tier-purity half's.
  Do **not** add an entry to `spawnObservabilityAllowedSpawners`: the invariant permits an exemption only for a site structurally barred from importing `internal/logger`, which `internal/planglyph` is not.
  Extend each guard's own table-driven sub-tests with a case proving the new detection fires: an untagged test file containing the `DeltaGit` token tripping the tier-purity guard, and a `DeltaGit` call site that does not import `internal/logger` tripping the observability guard.
- **Commit:** `38: test(guards): make quarry's DeltaGit spawn visible to the tier-purity and observability guards`

### Card 39: update the two remaining stencils and close the roadmap item

- **Context:**
  - `internal/planglyph/scope.go`
  - `internal/planparser/validate.go`
  - `contracts/stencils/loom/loom-template-plan.md`
  - `manifest/designs/quarry-glyph-plan-alphabet.md`
- **Edits:**
  - `contracts/stencils/webster/webster-body-implementer.md`
  - `contracts/stencils/loom/loom-rubric-plan-review.md`
  - `manifest/roadmap.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Retire the two pieces of stencil text the glyph alphabet invalidates, and move the roadmap item.
  In `contracts/stencils/webster/webster-body-implementer.md`, rewrite the `deviations` paragraph's deviation-union sentence, which today tells the implementer that the union is every path-shaped target entry plus the files holding every symbol-shaped target entry, "which you resolve yourself since it is already in your worktree and resolving a package-qualified symbol to its file is one read".
  That instruction is invalidated twice over: a symbol-shaped target is now a glyph rather than a package-qualified name, and the scope comparison is now performed mechanically by card 35's guard over the record-batch delta rather than by the implementer's own estimate.
  Replace it with a statement that the union is the batch's own target glyphs, that the implementer reports the paths it changed outside them, and that the mechanical guard is what actually compares — keeping the surrounding contract unchanged, including that `deviations` is always informational and that a non-empty list never makes `status` `FAILED` on its own.
  In `contracts/stencils/loom/loom-rubric-plan-review.md`, update the two places the alphabet change reaches: the line naming `prosa-symbol-target`, which now means a `Prosa` group may target file and unit self glyphs and a member glyph is the finding; and the per-symbol card-granularity guidance, which keeps its judgment — one card per independently reviewable unit, not one per literal symbol — but must now be phrased over glyphs.
  Keep the `Custom` caveat about escaping `path-missing` and `prosa-symbol-target` and extend it to the two new classification checks, since a mistyped `Custom` card escapes those too.
  In `manifest/roadmap.md`, move the Planned item **Adopt quarry's glyph alphabet as the plan alphabet** into the `## Done` section with a link to its design doc, per that file's own rule that an item moves on shipping with no renumbering needed anywhere.
  Leave the Someday **webster: worktree-per-card parallel execution** item where it is and edit only its clause stating it is blocked on this Planned item, since this task unblocks it but does not deliver it.
- **Commit:** `39: docs(stencils): retire the manual symbol-resolution guidance and close the roadmap item`

## Batch Tests

The batch's `verify:` is two commands chained, because this batch is the first to add `integration`-tagged tests and an untagged run alone would not execute the cards that matter most.
The untagged half, `go test ./internal/planglyph/ ./internal/websterengine/ ./internal/webstercli/ ./cmd/lyx/`, covers the pure logic — `scope_test.go`, `handle_test.go`, and the `beginbatch_test.go`/`recordbatch_test.go` wiring assertions, all driven through fake deltas rather than real ones — plus the two extended guards in `cmd/lyx`, whose own sub-tests prove the new detections fire.
The tagged half, `go test -tags integration ./internal/planglyph/ ./internal/websterengine/`, runs `delta_integration_test.go`, `donecheck_integration_test.go` and `drift_integration_test.go`, which build real fixture repositories and spawn git through `DeltaGit`; they are tagged for exactly that reason and run under `internal/planglyph`'s `TestMain` hermetic git environment from card 16.
`./cmd/lyx/` is in scope for a second reason beyond card 38: after card 38 lands, any untagged test file in the module containing the literal `DeltaGit` fails the tier-purity guard, so this run is what proves the new tagged files are tagged correctly.
The scope is four named packages rather than the module because those are the four this batch edits, and the hub's own `pipeline.done_gate` — `go test ./... && go test -tags integration ./...` — runs the whole repository in both modes before the task is marked done, which is what catches a regression in a package no batch verify covers.
</content>
