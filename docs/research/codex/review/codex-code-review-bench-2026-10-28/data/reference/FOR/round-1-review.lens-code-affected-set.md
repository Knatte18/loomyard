```yaml
lens: code-affected-set
outcome: reported
```

Lens: code-affected-set. For each change the diff makes, I looked for the code, docs, stencils, fixtures and PATTERN entries that read or describe it.

**Per change, the set found**
- **New stencil `landing-template-fix-review`:**
  - Covered: the `entries` registry, `roleOpeningStencils["fix-reviewer"]` and the `LandingTemplateFixReview` embed in `stencils.go`. The `fixReviewStencilName` const in `fixreview.go` matches.
  - Covered: the new prompt points at the spec by name and renders `parent_directive` and `edit_directive`. This is the same shape as `landing-template-fixer.md:44`.
  - I found no other registry or role list that names the fixer or conflict stencils. The other mentions are `fixer.go`, `mergeresolve/spec.go` and the stencil registry; `configreg` only lists config keys.
- **New spec `landing-fix-review-spec`:** covered. The `specs.go` embed and entry, the `specs_test.go` name list and rel-path row, `ParseFixReviewReport` and `PATTERN-sole-parsers.md` (a new section) all carry it.
- **`FixerAttempts.Reviewed`:** the writer and the same-HEAD read in `judgeFixerChange` are covered. One gap, in F2 below.
- **`keepFixerCommits` signature (new `reviewed` parameter):** covered. `judgeFixerChange` and `endSpent` are the callers I saw, and the `RerouteReviewed` read moved to `reviewFollowsReroute`.
- **`archiveStaleReport` (method turned into a function):** covered. The only call site is `fixerrun.go:117`, plus the new one in `fixreview.go`.
- **Config comments for `Fixer` and `FixerTimeoutMin` and `template.yaml`:** updated.
- **Docs and recipe comments:** the diff updates both recipes' comments, `doc.go` and the `overview.md` Finalize paragraph.
  - `docs/overview.md:576` ("a spent or over-cap fixer") still reads correctly as a description of halts.
  - `llm-model-spec.md:128` was not updated (F1).
- **Declared PATTERN changes:** the directive declares none, so there is nothing to check.
- **Commit message:** it carries no `Pattern-Departure:` trailer, and the diff needs none.

**Findings**

- **F1, LOW, consistency.** `contracts/specs/llm-model-spec.md:128` says `fixer` is the model-spec of "the merge fixer session that Parent-Align and Finalize run" and that `fixer_timeout_min` "bounds one of its turns".
  - The diff reuses both keys for the fix review session (D5) and updates the `config.go` and `template.yaml` comments, but not this spec.
  - The same gap is in `docs/overview.md:585`, which calls these "the merge fixer's `fixer` model-spec, `fixer_timeout_min`".
  - Fix: add the fix review session to both sentences.

- **F2, LOW, design.** `gateAndFix` (`finalize.go:606-616`) resets `Baseline` and `Halt` on an over-cap resume but leaves `Reviewed` in the record.
  - Today this does no harm: the `passed && overCapResume` case returns before `judgeFixerChange`, and a later review overwrites `Reviewed`.
  - `Reviewed` is a stale HEAD that nothing clears, so a later pass at that HEAD would skip review.
  - A comment on the `Reviewed` field (`fixerattempts.go:27`) or an explicit clear at the reset would pin the invariant. I found no test that covers this path.
  - Raise it only if the handler judges it worth the noise.

**Tool calls (3 of 6)**
1. A grep for `landing-template-fixer`, `"fixer"`, `landing-fixer-report-spec` and `LandingFixerReportSpec`. The manifest did not show whether other registries or lists name the fixer stencil or spec.
2. A grep of `PATTERN.md` and the `pattern/` files for landing and fixer mentions, a search for `fixer_timeout_min` and `.Fixer` uses, and a look for a `Pattern-Departure` trailer in the commit message. The manifest did not show these consumers of the keys.
3. A grep for other places that list the conflict stencil or role, to find any list I had missed, plus a read of `llm-model-spec.md:120-132`. The manifest did not include that spec.
