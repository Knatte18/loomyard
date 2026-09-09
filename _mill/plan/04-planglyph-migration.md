# Batch: planglyph-migration

```yaml
task: "Centralize glyph ref-shape enumeration"
batch: "planglyph-migration"
number: 4
cards: 2
verify: go test ./internal/planparser/ ./internal/planglyph/
depends-on: [2]
```

## Batch Scope

This batch migrates `internal/planglyph` onto planparser's exported surface: every open-coded `plan:` string operation moves to the batch-2 handle vocabulary, the two `draftHandle*` helpers are deleted in favor of their planparser ports, and the two documented duplicates (`resolveLanguage`, `cardIDOf`) are deleted in favor of `Plan.GlyphLanguage()` and `Card.ID()`.
It is one batch because both cards are the same mechanical shape — retarget call sites onto exported planparser API, delete the local copy — and together they leave planglyph free of `HandlePrefix` string surgery, which is the state batch 7's `plan:`-op scan pins (no planglyph file is exempt).
`resolveKeyFor` is deliberately kept as a thin wrapper (its `func(string) string` pass-through-on-non-handle signature is what its seven call sites are built around).

## Cards

### Card 8: Migrate handle string ops onto the exported vocabulary; delete the draftHandle helpers

- **Context:**
  - `internal/planparser/handle.go`
  - `internal/planglyph/drift.go`
  - `internal/planglyph/scope.go`
- **Edits:**
  - `internal/planglyph/donecheck.go`
  - `internal/planglyph/create.go`
  - `internal/planglyph/handle.go`
  - `internal/planglyph/planglyph.go`
  - `internal/planglyph/handle_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  All edits behavior-identical, proven by the untouched planglyph assertions:
  1. `resolveKeyFor` (`internal/planglyph/donecheck.go`) — reimplement as a thin wrapper over the exported helper: `if body, ok := planparser.HandleBody(ref); ok { return body }` then `return ref`.
     Keep the function, its name, and its signature; its doc comment gains a clause naming `HandleBody` as the grammar owner.
     Read its call sites in `internal/planglyph/create.go`, `internal/planglyph/drift.go`, and `internal/planglyph/scope.go` to confirm the signature contract holds, and leave every one of those call lines byte-identical.
  2. `createHandleResults` (`internal/planglyph/create.go`) — the `strings.HasPrefix(ref, planparser.HandlePrefix)` filter becomes `planparser.IsHandleRef(ref)`.
  3. `CanonicalizeHandles` (`internal/planglyph/handle.go`) — the Rename-pair `p.New` prefix test becomes `planparser.IsHandleRef(p.New)`, and the canonical-handle construction `planparser.HandlePrefix + res.ID` becomes `planparser.NewHandle(res.ID)`.
  4. `cardOwnHandles` (`internal/planglyph/handle.go`) — its `p.New` prefix test becomes `planparser.IsHandleRef(p.New)`.
  5. `BindHandles` (`internal/planglyph/handle.go`) — the `strings.TrimPrefix(h, planparser.HandlePrefix)` expected-glyph strip becomes `resolveKeyFor(h)` (behavior-identical: every `h` from `cardOwnHandles` is handle-shaped, and the pass-through-on-non-handle fallback matches `TrimPrefix`'s no-prefix behavior exactly).
  6. `collectGlyphTargets` (`internal/planglyph/planglyph.go`) — the handle-exclusion guard's `strings.HasPrefix(raw, planparser.HandlePrefix)` becomes `planparser.IsHandleRef(raw)`; the parse-success glyph test on the following lines stays byte-identical per the discussion's Decision: parse-success-glyph-test-kept.
  7. Delete `draftHandleMember` and `draftHandleIdentifier` from `internal/planglyph/handle.go`; retarget `renameDeclSource`'s one `draftHandleIdentifier` call to `planparser.HandleIdentifier`.
  8. In `internal/planglyph/handle_test.go`, delete the `draftHandleIdentifier` table test — its table moved verbatim to planparser's `HandleIdentifier` test in card 3 (the sanctioned rewire from the overview's Decision: behavior-preservation); every other assertion in the file stays untouched.
- **Commit:** `planglyph: migrate handle string ops onto planparser's exported vocabulary`

### Card 9: Delete the resolveLanguage and cardIDOf duplicates

- **Context:**
  - `internal/planparser/glyphref.go`
  - `internal/planparser/plan.go`
- **Edits:**
  - `internal/planglyph/planglyph.go`
  - `internal/planglyph/resolve.go`
  - `internal/planglyph/containment.go`
  - `internal/planglyph/create.go`
  - `internal/planglyph/donecheck.go`
  - `internal/planglyph/drift.go`
  - `internal/planglyph/handle.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Delete `resolveLanguage` from `internal/planglyph/planglyph.go` and retarget every call site to the plan value's own method — `plan.GlyphLanguage()` / `reloaded.GlyphLanguage()` per the local variable in scope — across `internal/planglyph/planglyph.go`, `internal/planglyph/containment.go`, `internal/planglyph/donecheck.go`, `internal/planglyph/drift.go`, and `internal/planglyph/handle.go` (the return type is identical, so each site compiles unchanged beyond the call spelling).
  Delete `cardIDOf` from `internal/planglyph/resolve.go` and retarget every call site to the card value's own `ID()` method (e.g. `cardIDOf(c)` becomes `c.ID()`, `cardIDOf(e.card)` becomes `e.card.ID()`) across all seven files that call it: `internal/planglyph/resolve.go`, `internal/planglyph/planglyph.go`, `internal/planglyph/containment.go`, `internal/planglyph/create.go`, `internal/planglyph/donecheck.go`, `internal/planglyph/drift.go`, and `internal/planglyph/handle.go`.
  Both deletions remove the in-code "mirrors planparser's own unexported" duplication comments along with the functions; no other comment or behavior changes.
- **Commit:** `planglyph: replace resolveLanguage/cardIDOf duplicates with Plan.GlyphLanguage and Card.ID`

## Batch Tests

`verify: go test ./internal/planparser/ ./internal/planglyph/` — planglyph's untouched unit suite (handle, create, donecheck, drift, containment, resolve, scope tests) is the behavior-preservation proof for every retargeted call site; planparser's suite guards the consumed exported surface.
