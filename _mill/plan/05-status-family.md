# Batch: status-family

```yaml
task: "Centralize glyph ref-shape enumeration"
batch: "status-family"
number: 5
cards: 2
verify: go test ./internal/planparser/ ./internal/planglyph/
depends-on: [4]
```

## Batch Scope

This batch is the family-1 hardening from `_mill/discussion.md`'s Decision: status-family-disposition: fix the one confirmed fail-open `quarry.ResolveResult.Status` consumer (`resolveContainment`'s bare allow-list), add the AST-based `.Status` tripwire test so a future unguarded consumer is caught, and document the two intentional asymmetries in place.
It is one batch because all three actions are the same family's disposition and share the same small planglyph reading set.
This is one of the plan's two sanctioned behavior changes — the guard lands with its own regression tests, and the absent-target skip is pinned as legal by a second test.

## Cards

### Card 10: Fail-closed Status guard in resolveContainment, with both regression tests

- **Context:**
  - `internal/planglyph/resolve.go`
  - `internal/planglyph/planglyph.go`
  - `internal/planglyph/donecheck.go`
- **Edits:**
  - `internal/planglyph/containment.go`
  - `internal/planglyph/containment_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `resolveContainment` (`internal/planglyph/containment.go`), replace the member-glyph branch's bare 2-of-4 allow-list (`!resolved || (r.Status != quarry.StatusFound && r.Status != quarry.StatusMultipart)` with silent `continue`) with a two-part disposition scoped to the `Status` half only:
  1. The `!resolved` half (target absent from the results index) keeps its silent `continue` deliberately — absence is structurally legitimate here because canonicalization can rewrite handles into glyph refs that were never in the resolve batch; carry that rationale into the comment.
  2. The `Status` half becomes a fail-closed vocabulary guard in the same shape as `doneCheckVerdicts`' (`internal/planglyph/donecheck.go`): the four readable statuses (`StatusFound`/`StatusMultipart` proceed to read `Symbols`; `StatusNotFound`/`StatusAmbiguous` keep today's silent skip of the member entry), and anything outside that vocabulary — the zero-value `Status` included — raises the blocking finding `glyph-rejected` rendered via the existing `unreadableStatusDetail` (`internal/planglyph/resolve.go`) with a containment-appropriate noun, one finding per referencing card, instead of silently dropping the member from the containment index.
     This requires the function's findings slice to be reachable from the target loop — restructure minimally (e.g. declare `findings` before the loop); the containment-conflict reporting below it stays byte-identical.
     Double-reporting alongside `statusFindings`' own `default` arm for the same anomalous target is accepted by design — say so in the comment.
  Regression tests in `internal/planglyph/containment_test.go` (new test functions; existing assertions untouched):
  one drives `resolveContainment` with a synthetic `quarry.ResolveResult` carrying an out-of-vocabulary `Status` (and one with the zero value plus `Error`/`Reason`) and asserts the blocking `glyph-rejected` finding surfaces rather than a silent drop;
  the other drives it with a member target absent from the results index entirely and asserts no finding and no containment entry — the canonicalization-introduced-target case stays legal.
- **Commit:** `planglyph: fail closed on unreadable Status in resolveContainment`

### Card 11: .Status tripwire test and the two documented asymmetries

- **Context:**
  - `internal/planglyph/resolve.go`
  - `internal/planglyph/donecheck.go`
  - `internal/planglyph/containment.go`
  - `internal/cliwire/bannedecl_enforcement_test.go`
- **Edits:**
  - `internal/planglyph/handle.go`
  - `internal/planglyph/create.go`
- **Creates:**
  - `internal/planglyph/status_enforcement_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create `internal/planglyph/status_enforcement_test.go` per the overview's Decision: ast-enforcement-idiom: parse every production `.go` file in `internal/planglyph` and flag any `ast.SelectorExpr` whose `Sel` is exactly `Status` that sits outside an allowlisted enclosing function.
  The allowlist names the consumers verified fail-closed today, as (file, function) pairs: `statusFindings` and `unreadableStatusDetail` (resolve.go), `createFindings` (create.go), `doneCheckVerdicts` (donecheck.go), `renameDeclSource` (handle.go), `resolveContainment` (containment.go).
  The failure message instructs: a new `.Status` consumer must handle the vocabulary fail-closed (switch-with-default, or a boolean derived after a vocabulary guard) and then be added to the allowlist — the test is a tripwire forcing that review, not a style rule.
  Doc-comment the deliberate file scope: `internal/quarrycli` also consumes `Status` but renders quarry's own output verbatim and is outside the validation surface this invariant hardens.
  Include the seeded self-test (synthetic source string with an out-of-allowlist `.Status` read; assert the matcher fires).
  Document the two intentional asymmetries in place, comment-only edits:
  in `renameDeclSource` (`internal/planglyph/handle.go`), extend the doc comment to state the Found-only rule is intentional — accepting a multipart answer would arbitrarily derive the declaration from the first symbol in the answer's list;
  in `createFindings` (`internal/planglyph/create.go`), extend the switch's `default`-arm comment to state that routing `StatusAmbiguous` to the `default`/`glyph-rejected` arm is deliberate for a Create target.
- **Commit:** `planglyph: add .Status tripwire test and document the two intentional Status asymmetries`

## Batch Tests

`verify: go test ./internal/planparser/ ./internal/planglyph/` — card 10's two new containment regression tests carry the behavior change (fail-closed on unreadable Status, absent-target skip pinned legal); the new tripwire test asserts zero out-of-allowlist `.Status` consumers on the real tree; every existing containment/resolve/donecheck assertion stays untouched as the no-other-behavior-change proof.
