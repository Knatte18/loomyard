MILL_REVIEW_BEGIN
# Review: Bump quarry to v0.2.0 and adopt Status.Known()/Rejected() — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnetxhigh
reviewer_self_id: Claude Sonnet 5 (claude-sonnet-5)
reviewed_file: plan/
date: 2026-09-09
```

## Findings

None.

Verification performed: read `00-overview.md` and `01-quarry-bump-and-adoption.md` in full, plus every source file in `## Files included` (`go.mod`, `go.sum`, `internal/planglyph/{donecheck,resolve,repo,create,containment,handle}.go`, `internal/planglyph/{donecheck_test,status_enforcement_test,testmain_test}.go`, `internal/quarrycli/resolve.go`, `CLAUDE.md`).

Spot-checks that passed:
- Card 2's described switch (four-case empty body / default appends `glyph-rejected`) matches `donecheck.go` lines 162–172 verbatim, including the doc comment's opening sentence and both crucible-round citations (R9-6, F1).
- Card 3's "second branch still reads `r.Status`" claim is correct: `unreadableStatusDetail` (resolve.go) has two `.Status` reads today — the condition being swapped to `r.Rejected()` and a second, untouched `r.Status` in the trailing `%q` argument — so the AST tripwire (`status_enforcement_test.go`'s `statusHitsIn`, which walks every nested `SelectorExpr`) still fires on the function post-edit, correctly justifying leaving `("resolve.go", "unreadableStatusDetail")` live in `allowedStatusConsumers`.
- Card 3's follow-up citation of `internal/quarrycli/resolve.go` line 80 (`if r.Error != ""`) inside `describeRejectedResolve` is exact; that file is untouched by any card in this batch, so the line number is stable through the whole batch.
- Card 3's claim about `handle.go` (`CanonicalizeHandles` tests `res.Error != ""` on a `quarry.NameResult`; `renameDeclSource` compares `r.Status != quarry.StatusFound` by design) matches source lines 94 and 222.
- Card 4's expectation table (StatusFound/Multipart: no-finding/fires/fires/no-finding; StatusAmbiguous: fires all four; StatusNotFound: fires/no-finding/no-finding/fires) is arithmetically correct against `doneCheckVerdicts`' `resolved`/`stillExists` derivation and matches the existing `TestDoneCheckVerdicts_Rules` table.
- `status_completeness_test.go` does not yet exist (confirmed via directory listing) and `chokepoint_enforcement_test.go` does exist, so both of Card 4's file-placement claims hold.
- `## All Files Touched` is exactly the union of the four cards' `Edits:`/`Creates:` targets, alphabetically sorted, with no Move-source leakage (no card has a non-empty `Moves:`, so no `## Rename mechanic` section is required and none is present — correctly).
- Batch Index DAG is a single node, `depends-on: []`, no cycle, references only the one file present in the plan directory.
- `allowedStatusConsumers`' six pairs as enumerated in `status_enforcement_test.go` match the plan's "six pairs" claim exactly.
- Context completeness holds for all four cards: every named function/constant in each `Requirements:` resolves to a file in that card's own `Context:` or `Edits:` (e.g., Card 4's `SeverityBlocking` reference is covered by `repo.go` in its `Context:`).

One inherent, unavoidable limitation: the plan's core premise (`quarry.Status.Known()`/`quarry.ResolveResult.Rejected()`/`quarry.Statuses` existing in v0.2.0 with the stated additive-only semantics) names an external module's API that lives outside this worktree (per the Quarry CGO Requirement Invariant) and is not Read-able from here — v0.2.0 is not yet in `go.sum`/the module cache. This is not a target-repo mechanism claim, so it falls outside the "mechanism claims must be source-verified" rule's scope, and Card 1 already installs a safety net for exactly this risk ("if `CGO_ENABLED=1 go build ./...` reports an error... stop and report it rather than editing call sites"). Not a finding.

## Verdict

APPROVE
Plan is precisely grounded against source, internally consistent, and correctly scoped as one batch.
MILL_REVIEW_END
