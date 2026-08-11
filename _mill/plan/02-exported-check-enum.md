# Batch: exported-check-enum

```yaml
task: 'fabric: accumulate the result envelope from mutations, not control flow (slice 14)'
batch: 'exported-check-enum'
number: 2
cards: 3
verify: go test ./internal/fabricengine/
depends-on: []
```

## Batch Scope

This batch turns `internal/fabricengine`'s unexported, int-backed `destructiveCheck` into an exported, string-backed `Check` — the single declarer of the check vocabulary — and adds the `RefusalOf` accessor that lets `internal/fabriccli` read a gate refusal's four fields without naming the unexported `*destructiveRefusal`.
It is one batch because the enum conversion, the `checkForce` deletion, and the accessor all live in `internal/fabricengine/destroy.go` and must land together to compile.
The external interface later batches consume is `Check` with its three constants, `Refusal`, and `RefusalOf`.

It deliberately does **not** touch `internal/fabricengine/fabrictest/refusal.go`'s duplicate copy — that deletion needs `VerbCase` churn and lands in batch 7, which depends on this one.

Batch-local decision: `checkForce` is deleted outright rather than exported.
The whole tree references it in exactly two code locations — its own declaration and the `String()` arm — and nothing anywhere constructs a `*destructiveRefusal` with it, because force is consulted only inside `checkPathDirtiness`, where it makes the dirtiness check *pass* rather than fail.
Moving to a string backing removes `String()` entirely, so the arm goes with it.

## Cards

### Card 4: export the check enum, delete `checkForce`, add `RefusalOf`

- **Context:**
  - `_mill/discussion.md`
  - `internal/fabricengine/fabrictest/refusal.go`
  - `internal/fabricengine/fabrictest/doc.go`
  - `internal/fabricengine/doc.go`
- **Edits:**
  - `internal/fabricengine/destroy.go`
  - `internal/fabricengine/checkout.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  `internal/fabricengine/checkout.go:206` is the tree's only production caller of the enum's `String()` method — `logger.Warn(..., "check", refusal.Check.String())` inside `rollbackSwitch`. Deleting `String()` breaks it, so this card changes that argument to `string(refusal.Check)`, preserving the logged value exactly.
  Grep for `.Check.String()` and `destructiveCheck` across `internal/` and `cmd/` before finishing the card to confirm no other production caller exists.

  In `internal/fabricengine/destroy.go`:

  1. Replace `type destructiveCheck int` and its four `iota` constants with an exported string-backed type carrying exactly three constants:

     ```go
     type Check string

     const (
     	CheckContainment Check = "containment"
     	CheckOwnership   Check = "ownership"
     	CheckDirtiness   Check = "dirtiness"
     )
     ```

     The three string values are exactly the spellings `internal/fabricengine/fabrictest/refusal.go` already uses, so `RefusedByGate`'s `string(check)+" check failed"` composition keeps working verbatim when batch 7 repoints it, and `refusal.check` marshals to JSON with no conversion step.

  2. Delete `checkForce` and delete the `String()` method entirely — a string-backed type renders itself, and `Error()`'s `%s` verb prints the constant's value unchanged.
     Update the two doc comments that name `checkForce` (`destroy.go`'s own file header and the pipeline commentary) so they still describe the pipeline's four-check order accurately while stating that the enum names only the three checks a refusal can be attributed to.

  3. Carry the non-constructibility rule onto `Check`'s own doc comment, in the words `internal/fabricengine/fabrictest/refusal.go` and `internal/fabricengine/fabrictest/doc.go` currently carry it: a `CheckForce` member must never be added, because force is consulted only inside `checkPathDirtiness` where it makes the dirtiness check *pass* rather than fail, so a refusal can never be attributed to it.

  4. Change `destructiveRefusal.Check`'s field type from `destructiveCheck` to `Check`. The struct itself stays unexported.

  5. Add the exported value type and accessor:

     ```go
     type Refusal struct {
     	Check  Check
     	What   string
     	Target string
     	Reason string
     }

     func RefusalOf(err error) (Refusal, bool)
     ```

     `RefusalOf` performs `errors.As(err, new(*destructiveRefusal))` internally — so a refusal wrapped several layers deep is still found — and converts the four fields into a `Refusal` value.
     It returns the zero `Refusal` and `false` for a `nil` error and for an error carrying no refusal.
     Its doc comment states why the accessor exists rather than an exported struct: `*destructiveRefusal` is a mutable pointer type whose identity callers could start switching on, when the envelope only ever needs its four values.

  Every `&destructiveRefusal{Check: check…}` construction site in this file updates to the new constant names.
  No refusal's `What`, `Target`, or `Reason` text changes, and `Error()`'s format string stays byte-identical.
- **Commit:** `refactor(fabricengine): export the gate check enum and add RefusalOf`

### Card 5: repoint the in-package test helpers

- **Context:**
  - `internal/fabricengine/destroy.go`
- **Edits:**
  - `internal/fabricengine/destroy_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `internal/fabricengine/destroy_test.go`, change `assertRefusalCheck(t *testing.T, err error, want destructiveCheck)` to take `want Check`, and update its every call site to the exported constant names (`checkContainment` → `CheckContainment`, `checkOwnership` → `CheckOwnership`, `checkDirtiness` → `CheckDirtiness`).
  Update the helper's failure message if it renders the wanted check through a now-deleted `String()` call — a `%s` verb on the string-backed type is the replacement.
  No assertion's meaning changes;
  this is a mechanical repoint.
- **Commit:** `test(fabricengine): repoint destroy_test to the exported Check enum`

### Card 6: `RefusalOf` unit tests

- **Context:**
  - `internal/fabricengine/destroy.go`
  - `internal/fabricengine/destroy_test.go`
  - `internal/fabricengine/testmain_test.go`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/refusalof_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create `internal/fabricengine/refusalof_test.go` — an **untagged** test file in `package fabricengine`, containing no `gitexec.RunGit`, no `exec.Command`, and no `lyxtest.Copy*` token anywhere including comments.

  Table tests covering `RefusalOf`:

  - A bare `*destructiveRefusal` yields `ok == true` and all four fields carried across unchanged.
  - The same refusal wrapped several layers deep via `fmt.Errorf("...: %w", ...)` is still found — this is the `errors.As`-not-type-assertion case the discussion names explicitly.
  - A `nil` error yields the zero `Refusal` and `false`.
  - An ordinary `errors.New` error yields the zero `Refusal` and `false`.
  - `remove.go`'s own dirty pre-flight shape — a bare `fmt.Errorf("worktree has uncommitted changes; use --force")` — yields `false`, since it is not a gate refusal at all. This is the case batch 6's envelope relies on to omit the `refusal` object on the `Remove` anomaly path.

  Assert the three `Check` constants render as `"containment"`, `"ownership"`, and `"dirtiness"`, and add a compile-time-shaped assertion that there is no fourth member representing force — the simplest honest form is a test that documents the rule in its own name and comment rather than an unenforceable reflection trick.
- **Commit:** `test(fabricengine): cover RefusalOf's errors.As traversal`

## Batch Tests

`verify: go test ./internal/fabricengine/` runs the package's untagged unit tests, which include the two files this batch touches (`internal/fabricengine/destroy_test.go`, the new `internal/fabricengine/refusalof_test.go`) plus every other untagged test in the package — the right scope, because the enum conversion touches a type that `destroy.go`'s whole refusal surface uses and a compile break would surface nowhere else.
The `integration`-tagged tests in this package are not run here;
they are unaffected, since no refusal's rendered message changes.
