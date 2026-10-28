---
verdict: APPROVED
findings:
  - id: F1
    severity: MEDIUM
    class: design
    origin: lens:code-tests
    location: internal/landingshed/finalize_gate_test.go (TestFinalize_GateAndFixer), internal/landingshed/finalize.go:722,741
    summary: D6 has no test for the approval writing Reviewed to the attempt record, nor for a HEAD that moved since the approval being reviewed again
    fix: In the "the fix review's end decides" table, after the "an approval lands" row read the record with ReadFixerAttempts(FixerAttemptsPath(fx.deps.ScratchDir, finalizeName), 1) and assert Reviewed equals f1. Add a row to the "pass at the approved HEAD" subtest that seeds Reviewed with a different HEAD, scripts a rejecting review, and asserts a fix review session runs and the run reroutes.
  - id: F2
    severity: MEDIUM
    class: design
    origin: lens:code-tests
    location: internal/landingshed/finalize_gate_test.go (TestFinalize_GateAndFixer), internal/landingshed/fixreview.go:571-585,609
    summary: D3 routes for a review session timeout, a failed session start (transient) and the single re-prompt over an unparseable report have no test row
    fix: Add rows to "the fix review's end decides" table. One uses the fake's sessionTimeout and expects a reroute naming fixer_timeout_min. One sets fx.seats.startErr and asserts the error is marked shedengine transient and nothing lands. One scripts reviewTurns as an unparseable report then approvedReview and expects a landing.
  - id: F3
    severity: MEDIUM
    class: design
    origin: lens:code-tests
    location: internal/landingshed/finalize_gate_test.go:~297-306
    summary: The D2 criterion that a spent fixer reroutes with no review session is not pinned, because the check only rejects sessions whose role is not the fix review
    fix: In the resumed-entry test, assert no table has RolePrefix fixReviewRole for the spent rows, and len(fx.seats.tables) == 0 for the rows that spawn no fixer. Replace the current loop, which a spawned fix review would also pass.
  - id: F4
    severity: MEDIUM
    class: design
    origin: lens:code-tests
    location: internal/landingshed/finalize_gate_test.go (over-cap table), internal/landingshed/fixreview.go:491-494
    summary: The D5 criterion that the fix review runs under the fixer model-spec and a fixer_timeout_min turn bound is asserted nowhere
    fix: In the over-cap table row that inspects the review table, assert review.Timeout equals time.Duration(cfg.FixerTimeoutMin) * time.Minute and that the seat's ModelChoice equals the one the fixer's table carries.
  - id: F5
    severity: LOW
    class: design
    origin: lens:code-correctness
    location: internal/landingshed/finalize.go:746
    summary: The halt for a reviewer that dirtied the tree embeds the clean() reason, whose tail says to commit or remove the changes then resume, beside the halt's own way forward to restore the branch to the fixer's range
    fix: Build the halt cause from the dirty paths alone, or drop the "commit or remove them on the task branch, then resume" tail from review.cause, so the halt names only the way forward to restore the task branch to Baseline..head with a clean tree.
  - id: F6
    severity: LOW
    class: consistency
    origin: lens:code-affected-set
    location: contracts/specs/llm-model-spec.md:128, docs/overview.md:585
    summary: Both docs still describe the fixer model-spec and fixer_timeout_min as the merge fixer's alone, although the diff reuses both keys for the fix review session
    fix: In llm-model-spec.md line 128 and docs/overview.md line 585, add the fix review session Finalize runs over an over-cap fixer change to the sessions the two keys govern, matching the updated config.go and template.yaml comments.
---

### [MEDIUM] D6 approval record and moved-HEAD bound untested

**Location:** `internal/landingshed/finalize_gate_test.go`, `internal/landingshed/finalize.go:722,741`
**Issue:** The only `Reviewed` test seeds the field by hand. Dropping the write in `judgeFixerChange`, or loosening `rec.Reviewed == head` to a non-empty check, breaks no test.

### [MEDIUM] Timeout, failed start and re-prompt routes untested

**Location:** `internal/landingshed/finalize_gate_test.go`, `internal/landingshed/fixreview.go:571-585,609`
**Issue:** The decision record names timeout and a transient failed start among D3's routes, and the design adds one re-prompt. The fake has `sessionTimeout` and `startErr`, but no review row uses them, and no row scripts a bad then a good report.

### [MEDIUM] D2 spent-fixer no-review criterion weakly pinned

**Location:** `internal/landingshed/finalize_gate_test.go:~297-306`
**Issue:** The loop errors only on a session whose role is not the fix review. A spent fixer that wrongly spawned a fix review would pass.

### [MEDIUM] D5 model-spec and timeout unasserted

**Location:** `internal/landingshed/finalize_gate_test.go`, `internal/landingshed/fixreview.go:491-494`
**Issue:** An acceptance criterion with no assertion on the table's `Timeout` or `ModelChoice`.

### [LOW] Contradictory way forward on a dirtied tree

**Location:** `internal/landingshed/finalize.go:746`, `internal/landingshed/verifygate.go:164-178`
**Issue:** The cause from `clean()` ends "commit or remove them on the task branch, then resume", and the same halt then says to restore the task branch to the fixer's range. A commit would move the head past the range the reviewer was asked about.

### [LOW] Fixer-key docs not widened to the fix review

**Location:** `contracts/specs/llm-model-spec.md:128`, `docs/overview.md:585`
**Issue:** Both still say the keys serve the merge fixer alone. The code comments and `template.yaml` were updated; these two were not.

## Rejected

- Reviewer push not detected (lens:code-correctness, LOW): D3 defines the changed end as a dirty tree or a moved task head, and the stencil forbids a push; adding a remote-tip check goes beyond the settled scope.
- `Reviewed` kept across an over-cap resume (lens:code-affected-set, LOW): the value records an approval of a specific HEAD and stays true at that HEAD; the resume path returns before `judgeFixerChange`, so no harm is reachable.
- `fixReviewTable` required-field errors and the `RerouteReviewed` error path untested (lens:code-tests, LOW): the first is internal misuse the trust-the-caller rule excludes, and the second is a one-line propagated error.

My own holistic pass found no deviation from the board entry or the decision record. D1 through D7 are implemented as decided. The stencil renders both directives and is listed in `roleOpeningStencils`, `ParseFixReviewReport` is the sole parser, and the docs, recipe comments and `PATTERN-sole-parsers.md` land in the same commit. The directive declares no PATTERN change, and the commit needs no `Pattern-Departure` trailer.
