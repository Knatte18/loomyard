---
verdict: BLOCKING
findings:
  - id: F1
    severity: BLOCKING
    class: consistency
    location: _lyx/plan/09-csharp-create-handles.md:24
    summary: The fallback declared-name scan selects a generic interface qualifier instead of the member
    fix: >-
      Correct card 9's fallback declaredNameIndex rule so it skips generic type and
      explicit-interface qualifier components and finds the declaration's member
      token when the draft name is absent. In particular, the already planned
      draft-to-Count row must select Count in int IProvider<int>.Count { get; },
      remove IProvider<int> from Owner, and bind with Owner Renderer. Keep D7's
      specified matching-name rule and generic-return-type bound unchanged.
---

### [BLOCKING] Generic qualifiers defeat draft-name canonicalization

**Location:** _lyx/plan/09-csharp-create-handles.md:24

**Issue:** The fallback selects the first identifier followed by any listed token,
including `<`. For the card's own `Renderer.IProvider<int>.draft` example, the
head has no `draft`, so this selects `IProvider`, before `Count`. Consequently
`csharpOwner` cannot find the qualifier immediately before the selected token
and retains `Renderer.IProvider<int>`. Quarry rejects that generic owner, as
the card itself explains. This contradicts both the expected clean row at line
35 and D7's required interface-qualifier removal.

## Rejected

- Cards 9–13 otherwise carry D7's three collision variants, parameter-list
  presence bound, shared record/recover completion check, plan-wide rewrite,
  fingerprint restamp, refreshed drift input, scope union and batching boundary.
- Card 6 names both language-mismatch halves and reaching tests. Card 7 reads
  disk existence rather than extensions and preserves completed-card filtering.
  The Go-parsable property used by card 6's resolve test is a member, so
  `diskPathForRef` does not subject it to `path-missing`.
- The specified ParseRef, GoToolchain, plan-aware ScopeGuard and nameDeclarations
  introductions precede their planned consumers. The focused Edit targets use
  member glyphs and state their specified signature changes with arrows.
- The focused cards leave no quarry fact as a research step. Cards 14–15 carry
  the required documentation and chokepoint changes, add no static-prose tests,
  and prescribe no raw Verify command.
