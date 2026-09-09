# Batch: boundary-enforcement

```yaml
task: "Centralize glyph ref-shape enumeration"
batch: "boundary-enforcement"
number: 7
cards: 1
verify: go test ./internal/planparser/ ./internal/planglyph/
depends-on: [3, 4]
```

## Batch Scope

This batch lands the two AST-based boundary-enforcement scans — the `refKind` scan and the `plan:`-op scan — over both packages' production files.
It must run after both migrations (batches 3 and 4) because the scans assert the migrated end state: zero classifier dispatch outside the registry boundary and zero open-coded `plan:` string surgery outside the grammar owner.
It is the requirement-4 capstone: after this batch, adding a fifth `refKind` fails the batch-1 meta-tests, and adding a new hand-rolled shape test anywhere in either package fails these scans.

## Cards

### Card 14: The refKind scan and the plan:-op scan

- **Context:**
  - `internal/planparser/classify.go`
  - `internal/planparser/shape.go`
  - `internal/planparser/handle.go`
  - `internal/planparser/validate.go`
  - `internal/planglyph/handle.go`
  - `internal/cliwire/bannedecl_enforcement_test.go`
- **Edits:** none
- **Creates:**
  - `internal/planparser/shape_enforcement_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create `internal/planparser/shape_enforcement_test.go` per the overview's Decision: ast-enforcement-idiom, scanning every production `.go` file in both `internal/planparser` and `internal/planglyph` (resolve both directories from `runtime.Caller(0)`), with two scans whose exempt sets are package-qualified relative paths — never bare basenames — compared against each parsed file's path:
  1. The `refKind` scan — exempt set exactly `internal/planparser/classify.go` and `internal/planparser/shape.go`.
     Build the banned identifier set dynamically: parse `internal/planparser/classify.go`'s `refKind` const block and take its declared constant identifiers (so a fifth kind is banned automatically), plus the fixed names `refKind` and `classifyRef`.
     Flag any `ast.Ident` in a non-exempt production file whose name is in that set.
     `refKindName` and `lookup` are deliberately outside the set — calls into shape.go's API are always legal; the ban is on comparing/switching over kinds and on classifying directly.
  2. The `plan:`-op scan — exempt set exactly `internal/planparser/classify.go`, `internal/planparser/shape.go`, and `internal/planparser/handle.go`; the test's comment states explicitly that no planglyph file is exempt, `internal/planglyph/handle.go` included, because that shared basename is where the invariant bites.
     Flag, in non-exempt production files: any `ast.CallExpr` to `strings.HasPrefix`/`strings.TrimPrefix` whose second argument is the identifier or selector `HandlePrefix` or the string literal `"plan:"`; any `ast.BinaryExpr` string concatenation with `HandlePrefix` or a `"plan:"` literal as an operand; and any other `"plan:"` basic literal in operand position of one of those shape operations.
     A `"plan:"` literal outside operand position of a shape operation (and every comment) never trips — the match is on the AST, mirroring the cliwire precedent's own rule.
  Both scans skip `_test.go` files by design (test fixtures legitimately spell raw `plan:` refs and call `classifyRef`); say so in the file comment.
  Include the seeded self-tests: one synthetic source string per scan carrying a deliberate violation (a `classifyRef` call plus a `refKindPath` comparison; a `strings.HasPrefix(x, "plan:")` call), asserting each matcher fires; then assert zero hits on the real tree.
- **Commit:** `planparser: enforce the ref-shape registry boundary with AST scans over both packages`

## Batch Tests

`verify: go test ./internal/planparser/ ./internal/planglyph/` — the new enforcement test is self-verifying (seeded violations prove the matchers fire; the real-tree assertion proves both packages are clean after the migrations); the rest of both suites re-proves nothing regressed.
