Affected-set list:

- Quarry declaration inputs: `CanonicalizeHandles` and `renameDeclSource` are covered by cards 1, 3, 9–11.
- Alphabet selection and signatures: production `parseGlyph`/`GlyphLanguage` callers in planparser, planglyph, websterengine, and plandag are covered by cards 2–3; ScopeGuard’s interface, implementation, fake, callers, and tests are covered by card 4.
- Go-toolchain gating: affected validation functions and their test tables are targeted by card 5, but its description overlooks an existing `none` assertion.
- New resolve-status readers: cards 6, 8, 11, and 12 add readers; no card covers the mandatory `allowedStatusConsumers` registration.
- Handle, Rename, and re-sign behavior: existing declaration callers are covered by cards 9–12; batching by card 13.
- Public language documentation: package docs and contracts are covered by cards 14–15, but `Plan.Language`’s field contract is omitted.

Findings:

1. **MEDIUM — design — cards 6, 8, 11, 12: New quarry status consumers omit the enforcement allowlist.**  
   Card 6 introduces `languageMismatchFindings`, which distinguishes found/multipart answers; card 8 introduces `crossAlphabetDetail`, which tests ambiguous status; card 11 introduces `nameResigns` reading the parsed glyph’s `Unit` and `resignCollisionFindings` testing found answers; card 12 introduces `RebindResigns` testing found answers.  
   `internal/planglyph/status_enforcement_test.go:64` includes both `Status` and `Unit` in its selector vocabulary, including non-status `Unit` fields; `:86` declares the closed `allowedStatusConsumers` list; `:135` scans every production file and fails for every unlisted reader.  
   No card names this allowlist or its required registrations. Consequently, these implementation cards fail the package’s existing guard even when their runtime behavior is correct.  
   Plan evidence: `_lyx/plan/06-plan-language-mismatch.md:30`, `08-multi-alphabet-answers.md:22`, `11-csharp-resign-arrows.md:30`, `11-csharp-resign-arrows.md:39`, `12-resign-post-batch.md:27`.  
   This is one recurring affected-set enumeration gap, not five separate findings.

2. **LOW — consistency — card 5: The assertion that `none` already skips the gated checks contradicts an existing test the card changes.**  
   Card 5’s ImpactSummary says “`none` already skipped all of them,” and its test instructions prescribe only adding a C# row to `TestValidate_VerifyNestedModule`.  
   The existing table explicitly contains `"language none still fires"` expecting `verify-nested-module` at `internal/planparser/validate_test.go:1700`; the implementation at `internal/planparser/verifymodule.go:37` has no language gate.  
   The planned `GoToolchain` condition changes this behavior and requires updating that existing row, not merely adding the C# row.  
   Plan evidence: `_lyx/plan/05-go-only-checks.md`, Tests and ImpactSummary.

3. **LOW — consistency — card 2 / card 14: The public `Plan.Language` contract remains Go-only.**  
   `internal/planparser/plan.go:51` documents the legal alphabet as “today `go`” plus `none`.  
   Card 2 adds C# acceptance but does not target this field documentation; card 14 updates `doc.go` files only.  
   The public model therefore retains a contradictory language contract after the planned documentation changes.

Tool-call manifest:

1. Searched changed function names and callers across Go files to check the card target lists against all callers beyond the inherited excerpts.
2. Searched language, kind, and parse readers to find independent consumers and assertions missing from the inherited manifest.
3. Read the existing `none` validation row, `Plan.Language` field documentation, status-consumer enforcement, and index adapters to determine their obligations.
4. Read the enforcement scan and verify implementations and searched documentation and cards to confirm whether the new readers were registered anywhere.
5. Read plandag’s graph logic, canonicalization, drift reload, and related tests to check downstream effects of the alphabet and signature changes.
6. Read handle binding tests and searched test/language references and new helper mentions to finish the affected-set comparison.
