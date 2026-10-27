Affected-set inventory:

- Declaration `Lang`: `CanonicalizeHandles` and `renameDeclSource` contain all production declaration constructions; cards 1, 3, 9–11 cover them.
- Alphabet selection: `parseGlyph` readers in planparser, direct `glyph.Parse` readers in planglyph and plandag, and `GlyphLanguage` readers in websterengine are covered by cards 2–3.
- Scope signature: both `planindex.Delta` implementations, `postBatchChecks`, and all direct scope tests are covered by card 4.
- Go-only checks: caller coverage, delete/import scanning, verify checks, handle validation, and external-test unit mapping are covered by card 5.
- New resolve-status consumers: cards 6, 8, 11, and 12 introduce consumers, but no card covers their enforcement allowlist.
- Rename alphabet parsing: card 10 requires the plan in `renameDeclSource`, but does not carry the changed signature or its caller.
- Ambiguity rendering: `statusFindings`, `createFindings`, and `doneCheckVerdicts` are covered by card 8.
- Re-sign naming chokepoint: its call-site enforcement is covered by card 11; its separate status-selector enforcement is not.
- Post-batch rewriting: the shared record/recover path, index interface, implementation, and integration tests are covered by card 12.
- Batching: shared `renameBoundaries` and its cost table are covered by card 13.
- Package and contract documentation: cards 14–15 cover the principal changed contracts.

Findings:

1. **MEDIUM — design — cards 6, 8, 11, 12: the affected-set enumeration omits the status-consumer enforcement registry.**  
   `languageMismatchFindings` must inspect found/multipart status; `crossAlphabetDetail` inspects ambiguous status; `nameResigns` reads `g.Unit`; `resignCollisionFindings` and `RebindResigns` inspect found status.
   `internal/planglyph/status_enforcement_test.go:65–68` scans both `Status` and `Unit`, including `Unit` selectors on glyphs; `allowedStatusConsumers` at line 87 names only existing consumers.
   `TestStatusEnforcement_NoOutOfAllowlistConsumer` at line 135 rejects every new enclosing function absent from that registry.
   No plan card targets or discusses `allowedStatusConsumers`, so implementing the prescribed helpers leaves their card gates failing.
   Evidence: `_lyx/plan/06-plan-language-mismatch.md:30`, `08-multi-alphabet-answers.md:22`, `11-csharp-resign-arrows.md:31` and `:39`, `12-resign-post-batch.md:27–30`.

2. **MEDIUM — design — card 10: the new plan-aware Rename parse has no plan argument or covered caller change.**  
   `_lyx/plan/10-csharp-rename-derivation.md:23` requires `renameDeclSource` to call `planparser.ParseRef` on the new handle.
   The existing function at `internal/planglyph/handle.go:98` receives only card/ref/root strings and resolve results, not the plan.
   Its sole caller at `internal/planglyph/handle.go:220` is inside `CanonicalizeHandles`, which has the plan.
   Card 10 neither re-signs `renameDeclSource` nor includes `CanonicalizeHandles` among its targets; earlier cards do not introduce that argument either.
   The change therefore needs an unplanned signature/caller migration, or would have to invent a language-bearing plan locally, contrary to D3’s plan-owned alphabet choice.

Tool-call manifest:

1. Searched whole-tree callers of changed signatures; the inherited manifest lacked complete caller sets.
2. Read status-enforcement implementation and searched naming/Kind readers; the manifest did not establish all enforcement consumers.
3. Searched cross-package readers, verdict rendering, index wiring, and message assertions; these consumers were not fully enumerated.
4. Searched scope/ref helpers and read kick-start and begin-batch call sites; these callers were beyond the inherited excerpts.
5. Located prospective status readers and inspected verify/index tests; needed to distinguish uncovered readers from tests already targeted.
6. Searched other enforcement scans and read ambiguity verdict branches; needed to establish whether another registry or renderer was affected.
7. Located exact plan and source lines for the two findings; needed precise evidence locations.
