# `loom` crucible fixer report — glyph-hardening campaign, ROUND 1 (opus5-high)

Companion to [`loom-review-opus5-high-r1.md`](loom-review-opus5-high-r1.md), which carries the findings themselves, the live evidence, and the per-scenario verdicts.

## Summary

22 findings: **20 fixed**, **2 recorded and deliberately not fixed** (both out of loom's module scope, both on a code path loom never takes).
Every fix landed as its own commit, on the current branch, verified green before the next was started. Nothing was pushed.

The round's headline outcome: **a multi-batch plan carrying a `Create`-with-handle card and a declared `Rename` card now runs end to end through Webster for the first time** — three batches, all `terminal: done`, handle bound to its real glyph in both the declaring and the referencing card, and the resulting code builds. Before the fixes that plan wedged permanently at the second batch, twice over.

## What was implemented

Commits, in order, each self-contained and green at the point it landed:

| Commit | Findings | What changed |
|---|---|---|
| `482079234` | F18 | `DetectDrift` excludes every `RenameCandidates` entry from the deleted-symbol sweep. Quarry deliberately leaves an evidence-tier candidate's endpoints in `Deleted`, so the same symbol was reporting a blocking "deleted with no corresponding rename" beside the informational candidate that contradicted it — and the blocking half killed the batch before any reviewer saw the evidence. The function's own no-collision claim, true only for the exact tier, is corrected. |
| `c2b64e001` | F1 | `RecordBatch` splits `DetectDrift`'s mixed severity set instead of blanket-blocking: blocking findings still fail with `ErrCardNotDone`, informational ones ride out on `RecordResult.Warnings` exactly as `ScopeGuard`'s already do. It was the one consumer in the repo with no severity filter. |
| `92cf21c93` | F2 | `renameCardPairs` normalises the pair's `New` side through `resolveKeyFor`. The plan format *requires* a symbol `Rename`'s `New` side to be a `plan:` handle, while quarry's delta reports the new symbol as a bare glyph, so gate one could never match for any plan that passes its own validator. The gate-one fixture was re-pointed at a spec-legal handle pair — its bare-glyph `New` side was a shape no real plan can carry, which is precisely why the test passed while production could not work. |
| `221dad17a` | F9, F20 | `renameSignature` skips a method's receiver clause before searching and matches the identifier as a whole word; `draftHandleIdentifier` takes the member's last dot-separated component. A signature carrying no occurrence of the identifier it declares is now a `rename-old-unresolved` finding rather than a silently wrong declaration. `declSource` carries its card so `handle-name-failed` names one, and its detail quotes the derived declaration that failed. |
| `a82a1ad15` | F8 | `CanonicalizeHandles` reports whether it actually rewrote the plan; `resolvePass` re-reads only when it did, and a failure of *that* re-read is returned wrapped in `ErrQuarryUnavailable` instead of silently degrading to the stale in-memory copy. |
| `ceeaf38ba` | F10 | `resolveContainment` sorts its targets and per-target cards, so findings are reproducible across runs. |
| `56588d84b` | F11 | `ScopeGuard`'s comparison union normalises each target through `resolveKeyFor`, so a handle-shaped Create target covers the bare glyph the delta reports for the symbol that card created. |
| `d967435d8` | F12 | Both bracket verbs refuse a nil `Plan` with a named error instead of panicking inside `planglyph`. `internal/webstercli`'s `TestSmoke_RecordBatchConsumesCrashedSessionReport`, which passed no `Plan` at all and panicked on every run, now supplies one. |
| `5bf342d87` | F4 | `restampFingerprint` re-baselines `State.PlanFingerprint` after each bracket verb's own sanctioned plan rewrite, and `amendments.md` is excluded from plan identity outright. The guard still catches a foreign edit landing between two batches — the whole window it covers. |
| `cb28973b2` | F3, F17 | A `Create` declaration bullet whose handle binds to a real glyph collapses to the plain `` - `<glyph>` `` ref form, rather than being substituted in place into `` `<glyph>` -> `<declaration head>` `` — which is no longer a handle declaration but still carries the arrow, so it parsed as `handle-malformed` plus an empty Create target list. The collapse is narrow by construction: it requires the *left* token to be a handle whose replacement is not one, so canonicalization and `Rename` pairs are untouched. |
| `c6ee9eb16` | F5, F6, F7, F21 | `planglyph.ValidateDispatch` and `planglyph.PendingPlan`: the dispatch boundary and record-batch's drift detection are scoped to cards whose work has not landed. See "The one design-shaped change" below. |
| `129329717` | F14 | `createHandleResults` resolves the glyph behind each `Create` handle in one further batched call, keyed by the handle the card spells, so the Create inversion — and its misspelled-unit protection — reaches the shape the plan format prescribes for creating something new. |
| `9726310b6` | F13, F15, F19 | Documentation: the stencil's unparseable declaration-head example, the spec's illegal `Rename` example, and `loom.md`'s claim that rows 8/10 produce "no artifact". |

## The one design-shaped change, called out explicitly

Every other fix is surgical. `c6ee9eb16` is not, and an orchestrator reading only the table should know it:

`BeginBatch` re-resolved the **whole** plan against the current tree on every batch. A plan describes intended change, so a card already built necessarily contradicts that tree — its `Create` target now exists, its `Delete` target is gone, its `Rename`'s old side no longer resolves — and each of those was reported as a blocking defect. That is the single root cause behind F5, F6, F7 and F21, and no multi-batch plan carrying one of those card types could survive it.

The fix introduces two exported functions in `internal/planglyph` and threads a completed-card set through `resolvePass`. The two halves of the validation are scoped **differently, on purpose**:

- the **resolve-backed** pass runs over the pending cards alone — a completed card's targets are never resolved, and it is never paired against a pending card for containment, because there is no race left to prevent with work that already landed;
- the **pure** pass runs over the whole plan with only its card-scoped findings dropped, because several pure checks are plan-level and would misreport against a filtered plan: `index-file-mismatch` would see every completed card's file as orphaned, `card-numbering` would see gaps, and `path-missing`'s satisfied-by-another-card union would lose the `Create` and `Rename` destinations completed cards contribute to still-pending ones.

`Plan-Validate` and `Plan-Revalidate` run before any card is built and keep the unscoped whole-plan form, so the Gate Self-Check Parity Invariant is untouched: `ValidateFormat`/`Validate` still back both rows and both standalone verbs, unchanged.

## Deliberately NOT fixed

Neither is a judgement call about effort — both are outside `loom`'s module scope per the review prompt's own "Explicitly OUT of scope", and both sit on a code path loom never takes, since loom always runs hub mode.

- **F16 — standalone `lyx webster run` cannot start Master at all** (BLOCKING, confirmed live). `standalonegeom.WebsterGeometry` derives its `AnchorRoot` under `$XDG_STATE_HOME` while `--target-dir` names a separate repository, so the pair is outside-in by construction and `shuttleengine.NewRunner`'s containment assertion refuses it. The mode is dead on the documented entry point `lyx webster --help` gives as its own example. `begin-batch`, `record-batch` and `validate` all work standalone; only `run` does not. **Recommend its own webster/standalonegeom-scoped task** — it is a real shipped blocker, just not loom's.
- **F22 — standalone mode writes `.lyx/logs/` into the target repository unexcluded** (LOW, confirmed live). Hub mode holds `.lyx` out of git via `fabricengine`'s `.git/info/exclude` seeding; standalone has no fabric and seeds nothing, so the operator's own repository shows the trace logs as untracked and `RecordBatch`'s dirty-worktree probe fires on every call because of them. Small, and belongs with F16 in the same task.

Nothing was marked NOT-FIXED-THIS-ROUND for size. Nothing was deferred for needing an operator decision or a second TTY.

## Tests added or extended

Every fix carries a test that fails on the unfixed code, verified by stashing the production change and re-running:

| Test | Package | Covers |
|---|---|---|
| `TestDetectDrift_EvidenceTierCandidateSuppressesTheDeletedSymbolFinding` | `planglyph` (integration) | F18 |
| `TestRecordBatch_EvidenceTierDriftWarnsAndDoesNotBlock` | `websterengine` (integration) | F1 |
| `TestDetectDrift_RenameMatchingCardPairProducesNoFindingNoAmendment` (fixture corrected to a spec-legal handle pair) | `planglyph` (integration) | F2 |
| `TestRenameSignature` (7 cases: free func, receiver-contains-name, receiver-equals-name, type params, interface method, parameter collision, absent identifier) | `planglyph` | F9 |
| `TestDraftHandleIdentifier`, `TestCanonicalizeHandles_RenameMethodDerivesAMethodDeclaration` | `planglyph` | F9 |
| `TestCanonicalizeHandles_ReportsWhetherItRewrote`, `TestValidate_UnparseablePlanDirectoryIsAnInfrastructureError` | `planglyph` | F8 |
| `TestResolveContainment_MultipleOverlapsAreDeterministicallyOrdered` (32 repeats) | `planglyph` | F10 |
| `TestScopeGuard_HandleTargetCoversTheSymbolItStandsFor` | `planglyph` | F11 |
| `TestFingerprint_IgnoresTheAmendmentLog`, `TestRestampFingerprint_RebaselinesTheStalenessGuard` | `websterengine` | F4 |
| `TestBindHandles_MatchedHandleRewritesDeclaringAndReferencingCard` (extended: asserts the collapsed shape and that the bound plan re-parses with no `handle-malformed`/`card-field-empty`/`handle-unreferenced`/`handle-dangling`) | `planglyph` | F3, F17 |
| `TestBeginBatch_AlreadyBuiltCardsAreNotReResolved` (both directions: still-pending blocks, already-built dispatches) | `websterengine` (integration) | F5, F6, F7 |
| `TestRecordBatch_DeleteCardDeletingItsOwnTargetIsNotDrift` | `websterengine` (integration) | F21 |
| `TestCreateFindings_HandleTargetIsInverted` (3 branches), `TestCreateFindings_HandleTargetWithNoAnswerProducesNoFinding` | `planglyph` | F14 |

Two existing drift fixtures were re-pointed at a genuinely pending second card (`addPendingCard`), because the batch being recorded is by definition not "remaining" — putting the reference on its own card was testing a card drifting against itself, which is never drift.

No test sleeps a fixed amount; the live scenarios are driven by real CLI invocations waited on to completion.

## Exact commands run, and results

Recorded in full in the review report's "Gates after the fixes" and "Post-fix live re-verification" sections. In brief: `go build ./...`, `go vet` over the eleven in-scope package sets, `go test -count=5` over those plus `./cmd/lyx/...`, `go test ./...` over the whole repository, `go test -tags integration` over the five in-scope packages, and four named `-tags smoke` tests run one at a time — all green. The four glyph scenarios were re-driven live against a fresh fixture repository with real git and real quarry.

## Changed files

Production:

- `internal/planglyph/handle.go` — `renameSignature`, `draftHandleIdentifier`, `identifierMatcher`; `declSource.card`; `CanonicalizeHandles` returns whether it rewrote
- `internal/planglyph/drift.go` — `renameCardPairs` handle normalisation; candidate exclusion from the deleted sweep
- `internal/planglyph/planglyph.go` — `ValidateDispatch`, `PendingPlan`, `cardIDSet`, `pendingCardsByID`; `resolvePass` scoping and strict reload; the Create-handle resolve
- `internal/planglyph/create.go` — `createHandleResults`; `createFindings` takes a ref-keyed index
- `internal/planglyph/containment.go` — deterministic ordering
- `internal/planglyph/scope.go` — handle normalisation in the comparison union
- `internal/planparser/rewrite.go` — the bound-declaration bullet collapse
- `internal/websterengine/recordbatch.go` — nil-plan precondition; drift severity split; scoped drift; fingerprint re-baseline
- `internal/websterengine/beginbatch.go` — nil-plan precondition; `completedCards`; `ValidateDispatch`; fingerprint re-baseline
- `internal/websterengine/fingerprint.go` — amendment-log exclusion; `restampFingerprint`

Tests: `internal/planglyph/{handle,planglyph,create,containment,scope,drift_integration}_test.go`, `internal/websterengine/{recordbatch,beginbatch,fingerprint}_test.go`, `internal/webstercli/smoke_test.go`.

Docs: `contracts/stencils/loom/loom-template-plan.md`, `contracts/specs/loom-plan-spec.md`, `manifest/designs/loom.md`.

`manifest/roadmap.md` was deliberately not touched — this is a hardening pass, and the roadmap moves only on completing or adding a planned item. No new cross-cutting invariant was introduced, so `CONSTRAINTS.md` is unchanged: the Planparser Sole-Parser Invariant's three write paths are intact (the declaration collapse happens inside `RewriteRefs`, not in a fourth writer), and the Gate Self-Check Parity Invariant is unaffected because `ValidateDispatch` is a third entry point used by neither gate row nor either self-check verb.

## Merge-readiness

**Merge-ready**, with the two out-of-scope findings recorded for their own task.

The glyph surface now does live, end to end, what its design doc says it does. The residual risk worth naming for the next round: no full `lyx loom run` through the seventeen-row recipe with real LLM sessions was possible on this host (three compounding environment gaps, detailed in the review report's "What could NOT be verified"), so the rows *above* Webster — `Plan-Write` actually emitting a legal handle declaration from a real planning session, and `Plan-Review` judging one — remain proven only at the mechanical layer, by the identical functions those rows call.
