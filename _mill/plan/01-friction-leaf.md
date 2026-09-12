# Batch: friction-leaf

```yaml
task: 'self-report Tier 2: per-agent friction notes for unsupervised runs'
batch: 'friction-leaf'
number: 1
cards: 5
verify: go test ./internal/friction/... ./contracts/stencils/...
depends-on: []
```

## Batch Scope

This batch delivers `internal/friction`, the leaf every prompt-composing engine will import, plus the five new stencil files and their registry rows, plus the new Friction Leaf Invariant in `CONSTRAINTS.md`.
Nothing outside the new package and the stencils tree changes, so this batch is independent of every other batch except as a dependency.

The external interface the later batches consume is exactly five exported identifiers plus one constant: `friction.Role` (four values), `friction.Directive`, `friction.NotePath`, `friction.WarnIfMarkerAbsent`, `friction.EnsureDir`, and `friction.MarkerName` / `friction.ReportFileName`.
Batch 3 consumes `ReportFileName` and `EnsureDir`;
batches 4, 5, and 6 consume `Directive`, `NotePath`, `WarnIfMarkerAbsent`, and `MarkerName`.

Batch-local decision that differs from the overview's Shared Decisions: none.

## Cards

### Card 1: `internal/friction` package — Role, Directive, NotePath, and the two helpers

- **Context:**
  - `internal/pattern/pattern.go`
  - `internal/pattern/doc.go`
  - `internal/stencil/stencil.go`
  - `internal/stencilstore/reconcile.go`
  - `internal/burlerengine/engine.go`
- **Edits:** none
- **Creates:**
  - `internal/friction/friction.go`
  - `internal/friction/doc.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create package `friction` with production imports limited to the standard library, `internal/logger`, `internal/stencil`, and `internal/stencilstore` — no other import, and never `internal/lyxcwd` or a feature package.

  `doc.go` carries the package godoc: why the leaf exists, why "off" is an empty path rather than a boolean, why the four roles are what they are, and why the marker helper logs rather than returning a bool.
  `friction.go` carries the code.

  Declare `type Role int` with four values, `RoleImplementer Role = iota + 1`, `RoleReviewFix`, `RoleOrchestrator`, `RoleInterview`, mirroring `internal/pattern`'s `Role` shape (`internal/pattern/pattern.go:38-49`) and adding `RoleInterview` for the Discussion-Write interview agent, whose prompt is neither editing nor reviewing.

  Declare four unexported stencil-name constants, one per role, each written exactly once: `implementerDirectiveStencil = "friction-directive-implementer"`, `reviewFixDirectiveStencil = "friction-directive-review-fix"`, `orchestratorDirectiveStencil = "friction-directive-orchestrator"`, and `interviewDirectiveStencil = "friction-directive-interview"`.

  Declare `const MarkerName = "friction_directive"` and `const markerLiteral = "{{." + MarkerName + "}}"`.
  `MarkerName` is what the seven composers pass to `stencil.FillOptional`'s optional-names slice;
  `markerLiteral` is what `WarnIfMarkerAbsent` searches for.
  Both are derived from the one string so they cannot drift.

  Declare `const ReportFileName = "reflection-report.md"` — the reflection agent's own mandatory output file, named once here so `internal/frictionengine`'s note scan and its `shuttleengine.Spec.OutputFiles` entry both read the same constant.

  `Directive(notePath, stencilsDir string, role Role) (string, error)` returns `("", nil)` with **no stencil read attempted** for an empty `notePath` and for an unknown or zero `Role`, mirroring `pattern.Directive`'s own empty-`anchorPath` early return and its `default` case (`internal/pattern/pattern.go:79-81` and `:94-98`).
  Otherwise it selects the role's stencil name, reads it with `stencilstore.Read(stencilsDir, name)`, wraps a read failure as `fmt.Errorf("friction: directive stencil: %w", err)`, and returns `stencil.StripLeadingComment(string(content))` with the literal `notePath` substituted into it.
  Substitution is a plain `strings.ReplaceAll` of the literal token `{{.note_path}}` in the stripped stencil text with `notePath`, performed here rather than through `stencil.Fill`, because the returned text is itself injected as another template's marker value and must never be passed through `Fill` a second time — the same reason `pattern.Directive` strips its banner and returns raw text.

  `WarnIfMarkerAbsent(template []byte, stencilName, notePath string)` returns nothing.
  It no-ops when `notePath` is empty (Tier 2 off).
  Otherwise, when `bytes.Contains(template, []byte(markerLiteral))` is false, it calls `logger.Warn` naming the stencil and the marker and pointing the operator at `lyx stencil diff` and `lyx stencil sync`.
  It is the only place in the tree that logs this condition;
  the composers call it and continue.

  `EnsureDir(dir string) ` returns nothing.
  It no-ops on an empty `dir`, otherwise calls `os.MkdirAll(dir, 0o755)` and, on failure, calls `logger.Warn` naming the directory and the error.
  It never returns an error, because a failed create must never fail `lyx loom run` or `lyx loom drive`.
- **Commit:** `feat(friction): add the Tier 2 friction-note leaf package`

### Card 2: `friction.NotePath` — sanitizing, non-clobbering note-path composition

- **Context:**
  - `internal/websterengine/recoverbatch.go`
- **Edits:**
  - `internal/friction/friction.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add `NotePath(frictionDir, id string) string` to `internal/friction/friction.go`.

  It returns `""` when `frictionDir` is empty, so Tier 2's off state composes through `NotePath` and `Directive` alike with no boolean anywhere.

  It returns `""` for an `id` that fails sanitization: an empty `id`, an `id` containing either path separator (`/` or `\`), an `id` equal to `.` or `..` or containing a `..` path element, and an `id` whose `id + ".md"` equals `ReportFileName` — the last so a caller can never name a note over the reflection agent's own output file.

  `id` is treated as a **stem, not a final name**.
  `NotePath` returns `filepath.Join(frictionDir, id+".md")` when that path does not exist on disk, and otherwise the **first free** path in the sequence `id-2.md`, `id-3.md`, ... — "first free", not "highest plus one", so a directory holding `id.md` and `id-3.md` but not `id-2.md` resolves to `id-2.md`.
  Bound the scan at a fixed ceiling (use `1000`) and return the last candidate rather than looping forever;
  exhausting the ceiling is not an error path this package reports.

  A stat error that is not "not exist" is treated as "the path is taken" and the scan advances, so an unreadable entry never causes an overwrite.

  The free-name scan is documented in the function's own doc comment as **best-effort under concurrency**: two spawns racing inside the same directory can both resolve to the same suffix and one note is lost.
  That is accepted — concurrent same-site spawns do not occur today, and a lost note is optional bookkeeping.
  The case this function closes structurally is *sequential* re-invocation of the same site, which is guaranteed on any bounced or crash-resumed run: `Discussion-Write` is re-entered whenever `Discussion-Validate` bounces to it, `Plan-Write` whenever `Plan-Validate` or `Plan-Revalidate` bounces, and `recoverSpawn` is re-runnable for the same batch — `internal/websterengine/recoverbatch.go` timestamp-archives a stale report on each call for exactly that reason.
- **Commit:** `feat(friction): add non-clobbering NotePath composition`

### Card 3: the four directive stencils and the reflection prompt stencil

- **Context:**
  - `contracts/stencils/pattern/pattern-directive-implementer.md`
  - `contracts/stencils/pattern/pattern-directive-orchestrator.md`
  - `contracts/stencils/pattern/pattern-directive-review-fix.md`
  - `contracts/stencils/loom/loom-template-plan.md`
  - `internal/friction/friction.go`
- **Edits:** none
- **Creates:**
  - `contracts/stencils/friction/friction-directive-implementer.md`
  - `contracts/stencils/friction/friction-directive-review-fix.md`
  - `contracts/stencils/friction/friction-directive-orchestrator.md`
  - `contracts/stencils/friction/friction-directive-interview.md`
  - `contracts/stencils/friction/friction-template-reflection.md`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create five stencil files under a new `contracts/stencils/friction/` family directory, each opening with an HTML banner comment in the style of `contracts/stencils/pattern/pattern-directive-implementer.md` naming its consuming call site and stating that `friction.Directive` strips the banner before returning the text.

  The four directive stencils each carry the literal token `{{.note_path}}` exactly once — the token `friction.Directive` replaces with the composed absolute note path — and declare no other marker, because `stencilstore.Validate` parses every registered stencil regardless of whether anything fills it and these files are never passed through `stencil.Fill`.

  Every directive stencil must state, in its own wording for its own role, all four of: that writing the note is **optional**;
  that an **absent note is the normal outcome and never an error**;
  that the note should be written **only when something actually went wrong**;
  and that the note is a title line naming what the agent was doing plus one or two short paragraphs of freeform markdown, written to the one absolute path given, with no schema and no filename of the agent's own choosing.

  Per the Producer Pointer-Rule Invariant, none of the four may restate or paraphrase the discussion, plan, or review format contracts — they point at the note path and describe the note, nothing else.

  The four vary by role: `friction-directive-implementer` is worded for an agent editing code (webster fork, webster recovery strand, webster integration fork, loom Plan-Write);
  `friction-directive-review-fix` for the Burler round, covering both its review and its fix phases;
  `friction-directive-orchestrator` for webster's Master, which forks rather than edits;
  `friction-directive-interview` for the Discussion-Write interview agent, whose job is neither editing nor reviewing.

  `friction-template-reflection.md` is the reflection agent's own prompt, filled through `stencil.Fill` by `internal/frictionengine` in batch 3, and declares exactly three markers: `{{.friction_dir}}`, `{{.report_path}}`, and `{{.note_list}}`.
  Its body instructs the agent to read every note in the friction directory, decide whether anything is worth filing at all and whether it is one issue or several, invoke `lyx selfreport create` itself for each issue it decides to file, and finally write the mandatory report file at `{{.report_path}}` recording the issue URLs it filed or the reason it filed nothing.
- **Commit:** `feat(stencils): add the four friction directive stencils and the reflection prompt`

### Card 4: register the five stencils and assert their optional-note wording

- **Context:**
  - `contracts/stencils/registry_test.go`
  - `contracts/stencils/friction/friction-directive-implementer.md`
  - `contracts/stencils/friction/friction-directive-review-fix.md`
  - `contracts/stencils/friction/friction-directive-orchestrator.md`
  - `contracts/stencils/friction/friction-directive-interview.md`
  - `contracts/stencils/friction/friction-template-reflection.md`
- **Edits:**
  - `contracts/stencils/stencils.go`
  - `contracts/stencils/rubric_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `contracts/stencils/stencils.go`, add one `//go:embed friction/<name>.md` var per new stencil — `FrictionDirectiveImplementer`, `FrictionDirectiveReviewFix`, `FrictionDirectiveOrchestrator`, `FrictionDirectiveInterview`, and `FrictionTemplateReflection` — each with the doc comment shape the existing `PatternDirective*` vars use, and append the five matching `entries` rows (`{"friction-directive-implementer", &FrictionDirectiveImplementer}` and so on) after the three `pattern-directive-*` rows.
  `contracts/stencils/registry_test.go`'s `TestRegistry_MatchesOnDiskTree` then covers registration in both directions with no edit of its own.

  In `contracts/stencils/rubric_test.go`, add content assertions in that file's existing short-distinctive-substring style for the one property that matters across all four directive stencils: each states that writing the note is optional and that an absent note is normal.
  Assert short distinctive substrings, never whole paragraphs.
- **Commit:** `feat(stencils): register the friction stencil family`

### Card 5: `internal/friction` tests and the Friction Leaf Invariant

- **Context:**
  - `internal/pattern/leaf_enforcement_test.go`
  - `internal/pattern/pattern_test.go`
  - `internal/friction/friction.go`
  - `contracts/stencils/friction/friction-directive-implementer.md`
- **Edits:**
  - `CONSTRAINTS.md`
- **Creates:**
  - `internal/friction/friction_test.go`
  - `internal/friction/notepath_test.go`
  - `internal/friction/leaf_enforcement_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add a new `## Friction Leaf Invariant` section to `CONSTRAINTS.md`, placed immediately after the existing `## Pattern Leaf Invariant` section, reading that `internal/friction` imports only stdlib, `internal/logger`, `internal/stencil`, and `internal/stencilstore` — never a feature package — and that the reverse import is never allowed.
  Add a sub-bullet recording why `internal/logger` is admitted: the marker-absent helper logs rather than returning a bool for seven callers to duplicate, and `internal/friction` already pulls `logger` transitively through `internal/stencilstore`, so the admission widens nothing in practice.

  `leaf_enforcement_test.go` is modelled directly on `internal/pattern/leaf_enforcement_test.go`: an **allowlist** walk of every non-test `.go` file in the package directory using `go/parser` with `parser.ImportsOnly`, failing on any import that is neither stdlib nor an allowlist entry, so a future stray dependency is caught with no list maintenance.

  `friction_test.go` covers `Directive` and `WarnIfMarkerAbsent`:
  each of the four roles returns its own stencil's text;
  an empty `notePath` returns `("", nil)` **with no stencil read attempted** — assert the no-read half by pointing `stencilsDir` at a path that does not exist, so any attempted read would surface as an error;
  an unknown and a zero `Role` behave the same way;
  a missing or unreadable stencil surfaces as an error naming the stencil;
  the returned directive text contains the told note path **verbatim**, which is the assertion that catches a composer wiring the wrong path;
  `WarnIfMarkerAbsent` reports absent for template bytes with no `{{.friction_directive}}` literal and present for bytes carrying it, and fires only on the absent-and-enabled combination — never when `notePath` is empty.

  `notepath_test.go` covers `NotePath` against a real `t.TempDir()` with real files, never a stubbed stat, since the guarantee is about the filesystem:
  `NotePath("", id)` returns `""`;
  two calls with the same `id` against a directory where the first note now exists yield `id.md` then `id-2.md`, and a third yields `id-3.md`;
  a gap (`id.md` and `id-3.md` present, `id-2.md` absent) resolves to `id-2.md` rather than skipping ahead;
  an empty `id`, an `id` containing a path separator, an `id` containing `..`, and an `id` whose `id + ".md"` equals `ReportFileName` all return `""`;
  a valid `id` against an empty directory yields the expected join.

  Add one assertion that `ReportFileName` is a single exported constant both a note-scan exclusion and an `OutputFiles` entry can be derived from — assert the two derivations agree with each other rather than asserting two string literals.

  Every test file in this package is untagged Tier 1: offline, no `exec.Command`, no `gitexec`, no `time.Sleep` at or above one second.
- **Commit:** `test(friction): pin the leaf invariant, Directive, and non-clobbering NotePath`

## Batch Tests

`verify: go test ./internal/friction/... ./contracts/stencils/...` covers both halves this batch delivers.

`./internal/friction/...` runs `friction_test.go` (Directive's four roles, the two no-read early returns, the error path, the verbatim-note-path assertion, and the marker-absent helper), `notepath_test.go` (the non-clobbering and sanitization behaviour, against real files in a `t.TempDir()`), and `leaf_enforcement_test.go` (the import allowlist pinning the new Friction Leaf Invariant).

`./contracts/stencils/...` runs the existing `registry_test.go`, which covers all five new stencils' registration in both directions automatically once the files and the `entries` rows exist, plus the new `rubric_test.go` content assertions on the four directive stencils' optional-note wording.

The scope is per-batch, not whole-repo: nothing outside these two package trees changes in this batch.
The `CONSTRAINTS.md` edit has no runnable surface of its own beyond the `leaf_enforcement_test.go` that pins it, which `./internal/friction/...` already runs.
