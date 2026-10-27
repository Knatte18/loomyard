Affected-set inventory:

- Quarry upgrade and declaration `Lang`: all production `quarry.Declaration` construction sites are in `CanonicalizeHandles` and `renameDeclSource`; cards 1, 3, 9–11 cover them.
- Alphabet migration: `parseGlyph` consumers in validation, redundancy, review classification, target derivation, and edit-test-file checks are covered by card 2; external `GlyphLanguage`/`glyph.Parse` consumers and their changed helper callers are covered by card 3.
- Scope guard signature: interface, real implementation, fake, production caller, and direct tests are covered by card 4.
- Go-only guards: implementation consumers and associated test tables are covered by card 5.
- New resolve-status readers: cards 6, 8, 11, 12 introduce consumers of selectors policed by `allowedStatusConsumers`; no card covers that registry.
- C# Rename parsing: card 10 requires `ParseRef(plan, body)` inside `renameDeclSource`, but does not supply the plan or cover its caller.
- Re-sign naming chokepoint: card 11 covers `chokepointGuardedFuncs` and its tests, but not the separate status-reader guard.
- Post-batch/index and batch boundary changes: cards 12–13 cover their interfaces, implementations, execution callers, and tests.
- Package and contract documentation: cards 14–15 cover the named docs and format consumers.

Findings:

1. **MEDIUM — design — cards 6, 8, 11, 12: new quarry readers omit the existing status-consumer registry.**
   The plan adds `languageMismatchFindings` (found/multipart), `crossAlphabetDetail` (ambiguous), `resignCollisionFindings` (found), and `RebindResigns` (found), plus `nameResigns` reading the glyph’s `.Unit`.
   `internal/planglyph/status_enforcement_test.go:64–69` explicitly scans `Status`, `Known`, `Rejected`, and `Unit`; the fixed registry at lines 88–99 names none of these functions.
   `TestStatusEnforcement_NoOutOfAllowlistConsumer`, lines 134–157, fails every new unregistered reader.
   No card targets or mentions `allowedStatusConsumers`.
   This is one recurring enumeration omission: the cards account for the `Name` chokepoint guard but omit the independent reader registry, so their own package gates cannot pass without unplanned edits.
   Locations: `_lyx/plan/06-plan-language-mismatch.md:30`, `08-multi-alphabet-answers.md` (`crossAlphabetDetail`), `11-csharp-resign-arrows.md:31`, `:39`, and `12-resign-post-batch.md` (`RebindResigns`).

2. **MEDIUM — design — card 10: `renameDeclSource` cannot use the plan-aware parser with its planned signature.**
   `_lyx/plan/10-csharp-rename-derivation.md:23` requires parsing the new handle through `planparser.ParseRef`.
   Card 2 defines that helper as `ParseRef(plan *Plan, ref string)`.
   The existing `renameDeclSource` receives only four strings and a result map (`internal/planglyph/handle.go:98`), and its sole caller passes only those arguments (`handle.go:220`).
   Card 10 lists `renameDeclSource` without a re-sign arrow at line 8 and omits `CanonicalizeHandles` from its targets.
   No earlier card supplies a plan parameter.
   The required alphabet decision therefore forces an unplanned signature and caller change; deriving a substitute plan from the resolved symbol would evade D3’s requirement that the actual plan choose the alphabet.

Tool-call manifest:

1. Whole-tree searches for parsing, handle, scope, and changed-helper readers; inherited searches had narrower scope.
2. Existing test assertions, redundancy implementation, vocabulary reader, target derivation, and normalization; their bodies were absent from inherited context.
3. Nested-module test and implementation, status registry, Kind/declaration readers, and canonicalization call; these exposed behavior assertions and the independent reader guard.
4. Guard searches, parser canonicalization, index tests, batch boundary implementation, and handle-test inventory; these checked additional consumers beyond the supplied manifest.
5. Numbered card locations and whole-tree ref/scope callers; these confirmed finding locations and the remaining reader inventory.
