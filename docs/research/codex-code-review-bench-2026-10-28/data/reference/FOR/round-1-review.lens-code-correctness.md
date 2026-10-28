```yaml
lens: code-correctness
outcome: reported
```

Lens: code-correctness of the Finalize fix-review diff. I ran every function and branch from the reading context and made two read-only greps.

## Per-function list

- **`ParseFixReviewReport`, empty file:** `lines` is `[""]`, so it fails with "no known verdict". Correct.
- **`ParseFixReviewReport`, CRLF input:** `TrimSpace` handles the first line, the heading and each bullet. Correct.
- **`ParseFixReviewReport`, `### sub` heading:** `CutPrefix("## ")` does not match, so the Findings section stays open. Correct.
- **`ParseFixReviewReport`, `-` or `- ` bullet:** a bare `-` is not a bullet, and a rejection with only that is refused. Correct.
- **`ParseFixReviewReport`, rejected with no bullets and approved with none:** the rejection errors and the approval returns an empty `Findings`. Correct.
- **`judgeFixerChange`, `Baseline == ""`:** it proceeds. Correct.
- **`judgeFixerChange`, `reason == ""` or `Reviewed == head`:** it proceeds with no review. Correct.
- **`judgeFixerChange`, `RerouteReviewed` false, nil, or an error:** it halts over-cap, halts over-cap, or errors, as before the change. Correct.
- **`judgeFixerChange`, approved:** `Reviewed` is written to the record and Finalize proceeds. Correct.
- **`judgeFixerChange`, `Reviewed` across an over-cap resume:** `gateAndFix` rewrites `Baseline` but keeps `Reviewed`, and the over-cap resume path proceeds without judging. Correct.
- **`judgeFixerChange`, rejected and no-verdict:** both reroute with the cause. Correct.
- **`judgeFixerChange`, changed:** it halts with `LandingStopPrefix`. The wording has a flaw, see F1.
- **`endSpent` and `keepFixerCommits` signature:** `endSpent` now reads `RerouteReviewed` and passes `reviewed` in. The only callers are `endSpent` and `judgeFixerChange`, and the spent-fixer path behaves as before. Correct.
- **`runFixReview`, transient start:** `ErrNotStarted` from `Run`, or `Chair.NotStarted`, becomes a transient mark, and other `Run` errors stay plain. Correct.
- **`runFixReview`, timeout:** the dirty and head checks run before the timeout check, so a timeout that also dirties the tree halts. Correct.
- **`runFixReview`, other non-timeout outcomes:** the report is parsed regardless of outcome, so a usable report with a crash-like outcome is honored. Acceptable.
- **`runFixReview`, no report:** `ParseFixReviewReport` returns a wrapped `ErrNotExist`, so it ends no-verdict. Correct.
- **`runFixReview`, deferred log:** the closure reads the final `outcome`. Early error returns after the defer log `end=""`, which is cosmetic and not a finding.
- **`runFixReview`, push by the reviewer:** this is not detected, see F2.
- **`fixReviewTable`:** the required-field loop, the optional `fixer_report_path` and the resolve errors are correct. `Deps.DecisionRecordPath` and `StencilsDir` are populated for the existing describe and fixer paths, so a missing path is a mis-wiring and not an expected state.
- **`archiveStaleReport` refactor:** the rename of a missing file is ignored, the error message keeps its row and label, and the fixer call site is equivalent. Correct.
- **`rerouteCause`:** the rejected case is built from the findings and the report path, and the other cases from the cause and session log. Correct.

## Findings

**F1 — LOW, design.** `internal/landingshed/finalize.go:746` (cause from `verifygate.go:164-178`).
- When the reviewer dirties the tree, the halt text embeds the `clean()` reason, which ends "commit or remove them on the task branch, then resume".
- The same halt then says "way forward: restore the task branch ... with a clean tree".
- That gives two conflicting way-forward instructions, and "commit" would extend the fixer's range past `head`.
- Fix: build the halt cause from the dirty paths only, or strip the "commit or remove them" tail, so one way forward remains.

**F2 — LOW, design.** `internal/landingshed/fixreview.go:578-612`.
- The stencil says "no push", and the fixer run snapshots `TaskRemoteTip` before its session.
- The fix review checks only the local head and the clean tree.
- A reviewer that pushes without committing, or that pushes the fixer's commits, goes undetected.
- Fix: snapshot `deps.TaskRemoteTip()` before `Run` and treat a moved tip as `fixReviewEndChanged`, or accept the gap and say so.

## Tool calls
1. A grep for the callers of `keepFixerCommits` plus a read of `finalize.go:775-815`. I needed to confirm there are no other callers and to see the reroute versus halt body.
2. A grep for the wiring of `DecisionRecordPath` and `StencilsDir` in `landingshed`. I needed to check whether the required inputs of `fixReviewTable` can be empty on a reviewed run.
