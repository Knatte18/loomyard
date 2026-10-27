Affected sets checked:

- Quarry upgrade and `Declaration.Lang`: all construction sites are in `CanonicalizeHandles` and `renameDeclSource`; card 1 covers them.
- `parseGlyph` replacement: parser validation, redundancy, review exemption, target mappings and vocabulary readers are covered by card 2.
- `GlyphLanguage` and language-bearing helper signatures: production callers in planglyph, websterengine and plandag, plus the direct collector test, are covered by card 3.
- Plan-aware `ScopeGuard`: interface, both implementations, production caller and direct tests are covered by card 4.
- Go-only checks: caller/import scans, handle rule, verify-package/module checks and external-test mappings are covered by card 5.
- New status and glyph-unit readers: cards 6, 8, 11 and 12 introduce them, but no card covers their enforcement registry.
- C# handles and Rename derivation: canonicalization and declared-name readers are covered by cards 2, 9 and 10.
- Re-sign completion: the real index implementation, shared record/recovery pass, fingerprint and scope/drift consumers are covered by card 12; existing test indexes either embed the real index or are constructor/validation-only placeholders.
- Re-sign batch boundaries: the shared boundary rule and cost-batchifier table are covered by card 13.
- Package and planner contracts: cards 14–15 cover the stated documentation surfaces.

Finding:

**MEDIUM — design — `_lyx/plan/08-multi-alphabet-answers.md:22`, with cards 6, 11 and 12 — The plan omits the status-consumer registry required by its new readers.**

`crossAlphabetDetail` must recognize the ambiguous status, `nameResigns` reads the parsed glyph’s `Unit`, and the collision and completion passes inspect resolve statuses.
`TestStatusEnforcement_NoOutOfAllowlistConsumer` scans every production function in planglyph and rejects selectors named `Status`, `Known`, `Rejected` or `Unit` outside `allowedStatusConsumers`.
The rule deliberately includes `glyph.Glyph.Unit`, even though that field is not a status.
No card targets or instructs an update to this registry.
Consequently these new functions hit the existing enforcement test as their cards land.
This is one recurring affected-set enumeration gap, rather than separate findings per function.

Evidence: `internal/planglyph/status_enforcement_test.go:57–68`, `:87–99`, `:105–123`, `:135–160`; `_lyx/plan/08-multi-alphabet-answers.md:22`; `_lyx/plan/11-csharp-resign-arrows.md:31–43`; `_lyx/plan/12-resign-post-batch.md:27–30`.
Card 11 covers the distinct naming-call chokepoint registry, but that does not cover this status-consumer scan.

Tool-call manifest:

1. Searched changed helper signatures and language readers across the tree to verify callers omitted from the inherited excerpts.
2. Searched Index/Delta implementations, external glyph readers and plan coverage to identify additional consumers.
3. Read index fake implementations and searched CLI/constraint guards and declaration construction sites, which were not fully inherited.
4. Read module requirements and searched canonicalization tests and new-reader locations to verify upgrade and extraction effects.
5. Searched finding-ID consumers and additional enforcement registries for readers beyond the inherited paths.
6. Searched existing C# fixtures and re-sign enforcement references to check collateral test expectations.
7. Read query-boundary wrappers and searched tree-sitter guards to check remaining dependency and naming consumers.

Departure: call 2’s broad plan-file search inadvertently returned two lines from `_lyx/plan/lens-review.md`; neither was used as evidence or grounds for this finding.
