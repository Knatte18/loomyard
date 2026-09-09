# Batch: exported-surface

```yaml
task: "Centralize glyph ref-shape enumeration"
batch: "exported-surface"
number: 2
cards: 2
verify: go test ./internal/planparser/ ./internal/planglyph/
depends-on: []
```

## Batch Scope

This batch adds planparser's exported cross-package surface: the handle vocabulary (`IsHandleRef`, `HandleBody`, `NewHandle`, `HandleMember`, `HandleIdentifier`, joining the existing `HandlePrefix`/`HandleUnit`) and the two dedup seams (`Plan.GlyphLanguage()`, `Card.ID()`), each with direct unit tests.
It is one batch because it is pure API addition — nothing existing changes behavior, no caller migrates yet — and it is the interface batch 3 (planparser migration, for `rewrite.go`), batch 4 (planglyph migration), and batch 7 (whose `plan:`-op scan assumes these helpers exist) all consume.
Batch-local decision: the new helpers live in `internal/planparser/handle.go` and use raw string operations against `HandlePrefix` per the overview's Decision: exported-surface-placement — deliberately without calling `classifyRef`, since that file stays outside the `refKind` scan's exempt set.

## Cards

### Card 3: Exported handle vocabulary in planparser's handle grammar file

- **Context:**
  - `internal/planglyph/handle.go`
  - `internal/planglyph/handle_test.go`
  - `internal/planparser/classify.go`
- **Edits:**
  - `internal/planparser/handle.go`
  - `internal/planparser/handle_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add five exported functions to `internal/planparser/handle.go`, implemented with raw `strings` operations against `HandlePrefix` (the grammar owner's sanctioned idiom, like the existing `handleUnit`):
  1. `IsHandleRef(raw string) bool` — `strings.HasPrefix(raw, HandlePrefix)`.
     Doc comment states it implements exactly `classifyRef`'s rule 1 (the `plan:` prefix test) and is the one handle-vs-not predicate other packages consume.
  2. `HandleBody(raw string) (string, bool)` — the substring after `HandlePrefix` and `true` when `IsHandleRef(raw)`; `("", false)` otherwise.
     This is the `plan:`-strip that planglyph's `resolveKeyFor` and `BindHandles` open-code today.
  3. `NewHandle(glyphID string) string` — `HandlePrefix + glyphID`, the one sanctioned handle construction.
  4. `HandleMember(handle string) (string, bool)` — port of planglyph's `draftHandleMember` (`internal/planglyph/handle.go`): the substring after the first `#`, `false` when no `#` is present.
     Carry the source's doc-comment substance over: local string work over loomyard's own `plan:` token, never glyph grammar, so the Glyph Conversion Chokepoint is untouched.
  5. `HandleIdentifier(handle string) (string, bool)` — port of planglyph's `draftHandleIdentifier`: `HandleMember`'s last dot-separated component, `false` for an empty member or empty final component; keep the method-owner rationale from the source doc comment.
  Do not delete anything in planglyph in this card — batch 4 card 8 deletes the two `draftHandle*` originals and migrates their callers.
  In `internal/planparser/handle_test.go`, add direct unit tests for all five: table-driven positive and negative cases for `IsHandleRef`/`HandleBody`/`NewHandle` (handle-shaped, glyph-shaped, path-shaped, empty inputs), and for `HandleIdentifier` port the existing table from planglyph's `draftHandleIdentifier` test (`internal/planglyph/handle_test.go`) verbatim as its assertion base, plus a `HandleMember` table covering member, no-`#`, and empty-member cases.
- **Commit:** `planparser: export handle vocabulary (IsHandleRef, HandleBody, NewHandle, HandleMember, HandleIdentifier)`

### Card 4: Export the two dedup seams — Plan.GlyphLanguage and Card.ID

- **Context:**
  - `internal/planparser/validate.go`
  - `internal/planglyph/planglyph.go`
  - `internal/planglyph/resolve.go`
- **Edits:**
  - `internal/planparser/glyphref.go`
  - `internal/planparser/glyphref_test.go`
  - `internal/planparser/plan.go`
  - `internal/planparser/validate_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `internal/planparser/glyphref.go`, add the method `func (p *Plan) GlyphLanguage() (glyph.Language, bool)` delegating to the unexported `planLanguage(p)` — the return type stays `(glyph.Language, bool)` verbatim so every planglyph call site batch 4 migrates compiles unchanged.
  Doc comment: this is the exported form of `planLanguage` and exists so `internal/planglyph` can delete its verbatim `resolveLanguage` duplicate (`internal/planglyph/planglyph.go`).
  In `internal/planparser/plan.go`, add the method `func (c Card) ID() string` returning the same `"N-<slug>"` formatting `cardID` (`internal/planparser/validate.go`) produces — implement the formatting in the method and leave `cardID` in place delegating or byte-identical (implementer's choice, but the two must render identically; the cheapest safe form is `ID()` carrying the `fmt.Sprintf` and `cardID` unchanged, with `cardID`'s doc comment gaining a pointer to `Card.ID`).
  Doc comment on `ID`: exported so `internal/planglyph` can delete its `cardIDOf` duplicate (`internal/planglyph/resolve.go`).
  Tests: in `internal/planparser/glyphref_test.go`, add a `GlyphLanguage` table (empty string and "go" report `glyph.Go` with ok true; "none" and an unrecognized value report ok false) asserting it agrees with `planLanguage` on every case; in `internal/planparser/validate_test.go`, add a `Card.ID` test asserting `c.ID() == cardID(c)` for a representative card.
  Do not change any existing assertion in either test file — both edits are additive test functions only.
- **Commit:** `planparser: export Plan.GlyphLanguage and Card.ID dedup seams`

## Batch Tests

`verify: go test ./internal/planparser/ ./internal/planglyph/` — the new exported-surface unit tests in `internal/planparser/handle_test.go`, `internal/planparser/glyphref_test.go`, and `internal/planparser/validate_test.go` cover every added function directly; the untouched remainder of both suites proves the additions changed nothing.
