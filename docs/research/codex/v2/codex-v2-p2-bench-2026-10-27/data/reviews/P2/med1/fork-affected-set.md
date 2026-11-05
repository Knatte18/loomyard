Affected sets reviewed:

- Quarry declarations: `CanonicalizeHandles` and `renameDeclSource`; card 1 covers all production construction sites.
- Alphabet parsing: planparser callers, planglyph callers, plandag, and Webster’s kick-start/post-batch callers; cards 2–4 cover the signature and parsing migrations.
- Scope guard: interface, concrete implementation, fake implementation, production caller, and tests; card 4 covers them.
- Go-only checks: validation, caller coverage, delete-order scanning, and existing tables; card 5 covers these readers.
- Ambiguous answers and Kind values: status/Create/done checks, partitioning, Rename derivation, and kick-start rendering; cards 8 and 10 cover the behavior.
- Naming chokepoint: production naming and the call-site enforcement tests; card 11 covers that chokepoint.
- New status consumers: cards 6, 8, 11, and 12 introduce readers, but no card covers their status-enforcement registration.
- Post-batch re-sign changes: index API, shared record/recovery path, scope, drift, fingerprinting, and batch boundaries; cards 12–13 cover these consumers.
- Documentation and contracts: cards 14–15 cover the package documentation, PATTERN entry, plan spec, planner stencil, and review rubric.

Finding:

**MEDIUM — design — `_lyx/plan/06-plan-language-mismatch.md:6` (also cards 8, 11, 12): New quarry-status consumers omit the existing enforcement allowlist.**

`languageMismatchFindings` must distinguish found/multipart results; `crossAlphabetDetail` distinguishes ambiguous results; `resignCollisionFindings` tests whether minted IDs resolve found; `RebindResigns` distinguishes found from every other result.
`nameResigns` additionally reads the parsed glyph’s `Unit`.
The existing scanner matches all selectors named `Status`, `Known`, `Rejected`, or `Unit`, including glyph units, and refuses every enclosing function absent from `allowedStatusConsumers`.
None of these cards changes that allowlist, so their planned implementations trip the package’s existing test, starting with card 6.
This is one recurring affected-set omission across the new readers, rather than separate findings per helper.

Evidence:
- `internal/planglyph/status_enforcement_test.go:64`: selector vocabulary.
- `internal/planglyph/status_enforcement_test.go:87`: existing allowlist contains none of the new functions.
- `internal/planglyph/status_enforcement_test.go:105`: scan records enclosing functions.
- `internal/planglyph/status_enforcement_test.go:135`: test rejects out-of-allowlist consumers.
- `_lyx/plan/06-plan-language-mismatch.md:30`, `_lyx/plan/08-multi-alphabet-answers.md:22`, `_lyx/plan/11-csharp-resign-arrows.md:30`, `:39`, `_lyx/plan/12-resign-post-batch.md:27`: prescribed new consumers.

Tool-call record:
1. Searched changed-function callers and inspected kick-start/language-test readers missing from the inherited excerpts.
2. Searched Kind consumers and read status-enforcement/completeness tests, which the manifest did not include.
3. Checked related enforcement scans and the existing `none` verification assertion, whose behavior the inherited excerpts did not establish.
4. Read conversion enforcement, normalization, partitioning, and kick-start reader details absent from the manifest.
5. Located exact plan and enforcement evidence lines for the finding.
