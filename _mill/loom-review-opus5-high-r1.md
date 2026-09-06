# `loom` crucible review — glyph-hardening campaign, ROUND 1 (opus5-high)

> Independent review per `_mill/loom-review-prompt.md`. Clean-room: findings below were formed without reading any prior review file in this worktree.
> Status: IN PROGRESS — appended incrementally as evidence lands.

## Executive summary

_(written last)_

## What was tested

### Hermetic baseline (before any source change)

Run at review start, on the clean `crucible-loom-glyph-hardening` tree (HEAD `61265b9bc`, derived from `main`).

| Command | Result |
|---|---|
| `go build ./...` | exit 0, clean |
| `go vet ./internal/loomengine/... ./internal/loomcli/... ./internal/loomrecipe/... ./internal/loomshed/... ./internal/shedengine/... ./internal/shedadapters/... ./internal/shedrecipe/... ./internal/shedbuild/... ./internal/hubgeom/... ./internal/planparser/... ./internal/planglyph/...` | exit 0, clean |
| `go test -count=5 <same 11 pkg sets> ./cmd/lyx/...` | exit 0 — 12 packages `ok`, 0 `FAIL`, 0 "no test files" |

Baseline is green; nothing red before the glyph scenarios started, so no pre-existing-redness finding.

## Findings

### F1 — `RecordBatch` treats an INFORMATIONAL drift finding as blocking (BLOCKING, CONFIRMED-by-trace)

`internal/websterengine/recordbatch.go:252-262`.

`DetectDrift` returns a mixed severity set: `plan-references-deleted-symbol` is `SeverityBlocking`, `rename-candidate` is `SeverityInformational` (`internal/planglyph/drift.go:135-143`).
`RecordBatch` collects **every** finding into `driftBlocking` with no severity filter and fails the batch with `ErrCardNotDone`.
Every other consumer in the repo filters (`loomshed/planvalidate.go:40`, `loomcli/validate.go:76`, `webstercli/validate.go:32`, `websterengine/runlevel.go:178`, `websterengine/beginbatch.go:190`) — this is the one that does not.

Scenario: batch renames a symbol inexactly enough that quarry classifies it a `RenameCandidate` rather than an exact `Renamed`, and some other card still references the old glyph → `DetectDrift` emits the informational `rename-candidate` → `RecordBatch` returns `ErrCardNotDone` and the batch is rejected, though `drift.go`'s own contract says the evidence tier "never auto-repairs" and the decision "is the reviewer's".
This defeats the entire evidence tier: it can never surface to a reviewer because it kills the batch first.

Fix: filter by `Severity`; route `SeverityInformational` drift findings into `RecordResult.Warnings`, exactly as `ScopeGuard`'s output already is (recordbatch.go:243-245).

### F2 — `DetectDrift` gate one can never match a real plan's Rename pair (BLOCKING, CONFIRMED-by-trace)

`internal/planglyph/drift.go:22-35` and `:84`.

`renameCardPairs` indexes `pairs[p.Old] = p.New` verbatim. Gate one then tests `want == newID` where `newID` is `delta.Renamed[i].To.ID` — a bare quarry glyph.
But the plan format **requires** a symbol `Rename` pair's `New` side to be a `plan:` handle (`contracts/specs/loom-plan-spec.md:218`, check `rename-to-not-handle`), and `CanonicalizeHandles` rewrites it to `plan:<canonical-glyph>` — still `plan:`-prefixed (`internal/planglyph/handle.go:145`).
So `want` is `"plan:sub#New"` and `newID` is `"sub#New"`: gate one never fires for any plan that passes its own validator.

Consequence: the card's OWN declared, expected rename is misclassified as drift. Gate two does not save it (the Rename card itself references `p.Old`, so `refCards[oldID]` is non-empty), so `DetectDrift` queues a `driftRepair`, rewrites `oldID`→`newID` plan-wide — clobbering the Rename card's own `Old` side — and appends a spurious `Tier: "exact"` amendment claiming a repair that was really the plan working as designed.

The existing coverage does not catch this because `internal/planglyph/drift_integration_test.go:68` builds the gate-one fixture as ``**Rename:**\n- `sub#Old` -> `sub#New` `` — a pair the plan format itself rejects under `rename-to-not-handle`.

Fix: strip `planparser.HandlePrefix` from the New side when indexing, reusing the same normalisation `donecheck.go:34` (`resolveKeyFor`) and `handle.go:222` (`BindHandles`' `expected`) already apply. Re-point the integration test at a spec-legal handle pair.

### F3 — `BindHandles` corrupts the declaring `Create` card's own declaration grammar, wedging every later batch (BLOCKING, CONFIRMED-by-trace)

`internal/planglyph/handle.go:239-250` → `internal/planparser/rewrite.go:122-136` → `internal/planparser/parse.go:678-685` → `internal/planparser/validate.go:580-590`.

`BindHandles` substitutes `plan:X` → `X` plan-wide through `RewriteRefs`. `rewriteBulletLine` matches the `` `old` -> `new` `` shape, which is *also* the `Create` declaration shape, so the declaring card's own sub-bullet is rewritten from
``- `plan:internal/foo#NewThing` -> `func NewThing() *Thing` `` to ``- `internal/foo#NewThing` -> `func NewThing() *Thing` ``.

On the next parse that payload no longer satisfies `splitHandleDeclaration` (no `plan:` prefix) but still contains `" -> "`, so `parseCreateField` drops it into `CreateRaw` — and `checkHandleMalformed` emits a blocking `handle-malformed` for every `CreateRaw` entry.

Each `lyx webster begin-batch` re-parses the plan from disk (`internal/webstercli/beginbatch.go:58`) and runs `planglyph.ValidateFormat`, refusing on any blocking finding with `ErrPlanDrifted` (`beginbatch.go:196-198`).
So: batch 1 lands a `Create` card carrying a handle → `RecordBatch` binds it → **batch 2's `begin-batch` refuses forever**. The plan cannot be un-wedged without a hand edit.

### F4 — Webster's plan-fingerprint gate is incompatible with the glyph surface's own in-flight plan rewrites (BLOCKING, CONFIRMED-by-trace)

`internal/websterengine/beginbatch.go:170-176`, `internal/websterengine/fingerprint.go:22-51`.

`fingerprint` hashes the name+contents of **every** `*.md` in `planDir`. Three glyph-surface code paths legitimately rewrite that directory *while the run is in flight*:

- `CanonicalizeHandles` → `RewriteRefs` (from `resolvePass`, reached by both `BeginBatch`'s own `ValidateFormat` and `Plan-Validate`/`Plan-Revalidate`),
- `BindHandles` → `RewriteRefs` (from `RecordBatch`),
- `DetectDrift` → `RewriteRefs` **and** `AppendAmendment`, which creates `amendments.md` — a brand-new `.md` file in `planDir`, hashed by `fingerprint` (and explicitly a "known non-card file" per `loom-plan-spec.md:222`).

None of them re-stamps `State.PlanFingerprint`. So the very first batch whose `RecordBatch` binds a handle or appends an amendment makes the next `BeginBatch` fail with `ErrFingerprintMismatch`, whose message tells the operator to `lyx webster run --fresh` — which archives state and starts over, hitting the same wall again.

This is independent of F3: it fires for a `Rename` card (drift amendment) and for a `Create` card (bind rewrite) alike, and F3's fix does not touch it.

Fix: re-stamp `State.PlanFingerprint` after webster's own sanctioned rewrites (end of `BeginBatch` and end of `RecordBatch`), so the guard keeps catching *foreign* edits between the two while tolerating the pipeline's own. Also exclude `amendments.md` from the fingerprint, since it is an append-only log, not plan identity.

### F5 / F6 / F7 — the dispatch-boundary re-resolution validates already-executed cards against the post-change tree (BLOCKING, CONFIRMED-by-trace)

`internal/websterengine/beginbatch.go:183` calls `planglyph.ValidateFormat(deps.Plan, …)` over the **whole** plan on every batch dispatch.
A plan describes intended change, so a card whose work already landed necessarily contradicts the current tree:

- **F5 (`Delete` card):** the deleted glyph is still in the card's `Targets`, so `statusFindings` reports blocking `glyph-not-found` (`internal/planglyph/resolve.go:102-116`) from the next batch onward.
- **F6 (`Rename` card):** the `Old` glyph is projected into `Targets` (`loom-plan-spec.md:177`) and no longer resolves → blocking `glyph-not-found`; and `CanonicalizeHandles`' `renameDeclSource` cannot resolve `Old` any more → blocking `rename-old-unresolved` (`internal/planglyph/handle.go:47-56`).
- **F7 (`Create` card with a bare-glyph target rather than a handle):** the created symbol now resolves `found`, so the Create inversion reports blocking `create-already-exists` (`internal/planglyph/create.go:70-77`).

Each of these wedges `begin-batch` with `ErrPlanDrifted` for the remainder of the run.
Together with F3 this means **no multi-batch plan containing a `Create`, `Delete` or `Rename` card can complete through Webster** — which is every non-trivial plan.
Round 2's successful full-pipeline pass does not contradict this: it ran a minimal, single-batch, non-glyph task.

Root cause is one gap, not three: nothing scopes the re-resolution to cards that have **not yet executed**. `deps.State.Batches[n].Terminal` plus `deps.Batches`' card lists already carry exactly the information needed.

### F8 — `resolvePass` silently swallows a post-rewrite plan re-parse failure (MEDIUM, CONFIRMED-by-trace)

`internal/planglyph/planglyph.go:87-90`.

```go
current := plan
if reloaded, rerr := planparser.ParsePlan(plan.Dir); rerr == nil {
    current = reloaded
}
```

`rerr` is discarded. If `CanonicalizeHandles`' own `RewriteRefs` produced a plan directory that no longer parses, the three resolve-backed passes below silently run against the **stale in-memory pre-rewrite** plan and the gate reports a clean-looking verdict over bytes that are no longer on disk.
That is precisely the "a plan looks validated and was not" failure mode `repo.go:28-31` says was rejected by design.
Fix: return the error, wrapped, rather than degrading.

### F9 — `renameDeclSource` derives the new declaration with a first-occurrence string replace (MEDIUM, PLAUSIBLE)

`internal/planglyph/handle.go:67`: `decl := strings.Replace(sym.Signature, sym.Glyph.Name, member, 1)`.

`strings.Replace(..., 1)` hits the *first substring* occurrence, not the declared identifier. For a method whose receiver type name contains the method name as a substring the first hit is in the receiver, so the wrong token is renamed — e.g. `func (c *Canonicalizer) Canonical() error` with `Glyph.Name == "Canonical"` yields `func (c *NewNameizer) Canonical() error`. Same for `func (r *Resolve) Resolve()`.
The derived `Decl` then goes to `quarry.Name`, so the canonical glyph computed for the handle is wrong — and it is wrong *silently*, because `Name` will happily name whatever declaration it is handed.
Marked PLAUSIBLE pending a live check of what `Symbol.Signature` actually contains for a Go method.

### F10 — `resolveContainment` emits findings in nondeterministic order (LOW, CONFIRMED-by-trace)

`internal/planglyph/containment.go:47`: `for target, cards := range byTarget` — a Go map range, so `members` and `selves` are built in random order and the resulting `containment-file-overlap` findings come out in a different order on each run whenever more than one overlap exists.
Every sibling in this package sorts deliberately (`sortedCards`, `sort.Strings(canonicals)`, `sort.Strings(targets)`); this one does not.
Consequence: a gate's rendered findings list is not reproducible, so two runs over an identical plan produce different operator output and any test asserting more than one overlap is inherently flaky.

### F11 — `ScopeGuard` reports every handle-created symbol as out-of-plan (LOW, CONFIRMED-by-trace)

`internal/planglyph/scope.go:18-26` builds the comparison union from `Card.Targets` verbatim.
A `Create` card declaring `plan:internal/foo#NewThing` has exactly that handle-shaped string in `Targets` (`internal/planparser/parse.go:680`), while `delta.Created[i].ID` is the bare glyph `internal/foo#NewThing`.
`union[id]` therefore misses, and `ScopeGuard` emits an informational `scope-outside-plan` finding for **every symbol the plan explicitly asked to be created** — the exact opposite of the check's purpose.
`RecordBatch` runs `ScopeGuard` on `batch.Cards`, which are parsed at CLI entry and so still carry the handle even after `BindHandles`' on-disk rewrite (recordbatch.go:243).
Fix: normalise the union through the same handle-stripping `donecheck.go:34`'s `resolveKeyFor` performs.

### F12 — `planglyph`'s entry points nil-deref on a nil plan, and the glyph landing left a smoke fixture panicking (MEDIUM, CONFIRMED-by-trace)

`internal/planglyph/planglyph.go:115-116`: `resolveLanguage` dereferences `plan.Language` with no nil guard, and `DoneChecks`, `CanonicalizeHandles`, `BindHandles`, `DetectDrift`, `ValidateFormat` and `Validate` all call it first.

`internal/webstercli/smoke_test.go:333-344` constructs `websterengine.RecordDeps` with **no `Plan` field** — nil — and calls `RecordBatch`, which reaches `planglyph.DoneChecks(deps.Plan, …)` at `recordbatch.go:195`. That is an unconditional nil-pointer panic.
The fixture predates PR #230 and the `Plan` field it added to `RecordDeps`; nothing updated it, and the smoke tier is not part of any routine gate, so it went unnoticed.
`internal/websterengine/recordbatch_test.go`'s only `RecordDeps` constructor does pass a plan, which is why the hermetic tier stays green.

Strictly `webstercli`'s test is outside loom's module scope, but the defect is a direct consequence of the surface under review and the nil-guard gap is in `planglyph` itself, so both halves are recorded here and fixed this round.

## Scope assessment

_(written last)_
