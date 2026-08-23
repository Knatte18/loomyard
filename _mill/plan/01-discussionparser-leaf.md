# Batch: discussionparser leaf package

```yaml
task: 'loom: self-checkable mechanical gates'
batch: 'discussionparser leaf package'
number: 1
cards: 2
verify: go test ./internal/discussionparser/... ./internal/lyxcwd/...
depends-on: []
```

## Batch Scope

This batch creates `internal/discussionparser`, the stdlib-only leaf that becomes the sole reader of `_lyx/discussion/`'s format, and records the invariant that binds it.
It is one batch because the package, its behaviour tests, its leaf-enforcement test, and the `CONSTRAINTS.md` section naming that enforcement test's allowlist are a single self-contained unit: nothing outside the new directory changes except the one new `CONSTRAINTS.md` section, and every other batch in this plan consumes this package rather than contributing to it.

The external interface the next batches consume is exactly two exported names: `discussionparser.Finding` (a `Check`/`Path`/`Detail` struct with an `Error() string` method) and `discussionparser.Validate(decisionRecordPath, supportLogPath string) ([]Finding, error)`.
Batch 2 rewrites `loomshed.discussionValidate.Call` over `Validate`;
batch 3's `validate-discussion` verb calls the same function.

Batch-local decision, differing from nothing in `## Shared Decisions` but worth stating: this package is the cleanest TDD target in the task, so within card 1 the implementer writes `internal/discussionparser/validate_test.go` before `internal/discussionparser/validate.go`.
The two files are one card rather than two so that every commit in this plan compiles — a test file committed ahead of the functions it names would not.

## Cards

### Card 1: discussionparser.Validate and its behaviour tests

- **Context:**
  - `internal/loomshed/discussionvalidate.go`
  - `internal/planparser/validate.go`
  - `internal/planparser/doc.go`
  - `contracts/stencils/loom/loom-template-discussion.md`
- **Edits:** none
- **Creates:**
  - `internal/discussionparser/doc.go`
  - `internal/discussionparser/validate.go`
  - `internal/discussionparser/validate_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create package `discussionparser` under `internal/discussionparser`, importing the standard library and nothing else.

  `internal/discussionparser/doc.go` carries the package doc: `discussionparser` is the sole reader of `_lyx/discussion/`'s format, it takes told absolute paths and declares no on-disk location of its own (`loomengine`'s `DiscussionDecisionRecord` / `DiscussionSupportLog` accessors remain the sole declarers, because they take a `*lyxcwd.Location` a stdlib-only leaf may not import), and it is stdlib-only so a future `Plan-Sweep` consumer can reuse its section parsing without dragging geometry resolution in.
  Follow `internal/planparser/doc.go`'s shape and tone.

  `internal/discussionparser/validate.go` declares, in this order:

  - `Finding`, an exported struct with three exported string fields — `Check`, `Path`, `Detail` — mirroring `planparser.ValidationError`'s `Check`/`Card`/`Detail` shape.
  - `func (f Finding) Error() string`, returning `f.Check + ": " + f.Detail`.
    `Path` carries the absolute path as a structured field for a future caller;
    `Detail` is the human-readable sentence and already names the file or heading concerned, so `Error()` does not repeat the path.
  - Two unexported check-name constants used as `Finding.Check` values: `checkFileMissing = "discussion-file-missing"` and `checkSectionMissing = "discussion-section-missing"`.
  - `requiredDiscussionSections`, unexported, moved verbatim from `internal/loomshed/discussionvalidate.go` — the same seven `## ` headings in the same order (Goal, Scope, Decisions, Constraints, Auto-mode assumptions, Open risks, Acceptance criteria) — carrying its existing pointer comment to `contracts/stencils/loom/loom-template-discussion.md`'s Step 5 verbatim, adjusted only to name `Validate` where it named `discussionValidate`.
    Do not export this list.
  - `func Validate(decisionRecordPath, supportLogPath string) ([]Finding, error)`, whose control flow reproduces `discussionvalidate.go`'s `Call` step for step, per the `short-circuit-order-is-load-bearing` Shared Decision: `os.Stat(supportLogPath)` first — on an error that is not `errors.Is(err, os.ErrNotExist)` return `nil, err`, on a not-exist return exactly one `Finding` with `Check: checkFileMissing`, `Path: supportLogPath`, and a nil error;
    then `os.ReadFile(decisionRecordPath)` with the identical two-way split, its own `checkFileMissing` finding carrying `Path: decisionRecordPath`;
    then the heading check, which is the only place findings accumulate — one `Finding` per missing heading, with `Check: checkSectionMissing`, `Path: decisionRecordPath`, and a `Detail` naming that exact heading.
    A clean run returns a nil (or empty) slice and a nil error.
    `Validate` never returns a non-empty findings slice together with a non-nil error.
  - `missingSections(content string, required []string) []string`, unexported, derived from `discussionvalidate.go`'s `hasAllSections` and preserving its line-scanning semantics exactly: `bufio.NewScanner` over `strings.NewReader(content)`, each line right-trimmed of `" \t\r"`, a heading counting as present only when a whole trimmed line equals it — so a heading nested inside a fenced block or appearing mid-sentence never counts.
    It returns the missing headings in `required`'s own order rather than reporting a bare bool.

  Carry `Call`'s three documented non-checks into `Validate`'s godoc verbatim in substance: `## Notes for the plan writer` is optional and its absence is never a violation, section *order* is not validated, and an extra unexpected `## ` heading is not a violation.

  `internal/discussionparser/validate_test.go` is written first and is package-internal (`package discussionparser`), so the per-heading iteration case can range over the unexported `requiredDiscussionSections`.
  Every case builds its fixture under `t.TempDir()` and tells `Validate` absolute paths;
  no case resolves a cwd or spawns a process.
  Cover: all seven headings present with both files existing (zero findings, nil error);
  each required heading missing individually, ranging over `requiredDiscussionSections` (one finding naming that heading);
  several headings missing at once (one finding per missing heading, not a single aggregate);
  the decision record absent (one finding, not a returned error);
  the support log absent (one finding, not a returned error, and the decision record is never read);
  a required heading present but carrying trailing whitespace (still present);
  a required heading appearing inside a fenced code block and one appearing mid-sentence (neither counts as present);
  `## Notes for the plan writer` absent (not a finding);
  headings present but out of stencil order (not a finding);
  an extra unexpected `## ` heading (not a finding);
  the decision record path being a directory (a returned error with an empty findings slice — the one file actually read);
  and the support log path being a directory, which `os.Stat` accepts, so it counts as present and validation proceeds to the decision record.
  Assert that last case explicitly with a comment saying it preserves today's behaviour deliberately rather than tightening it — "both files exist" is exhaustively what the check is defined to ask about the support log, and an is-regular-file test would be a new check smuggled in under an extraction.
- **Commit:** `feat(discussionparser): extract Discussion-Validate's checks into a stdlib-only leaf`

### Card 2: leaf enforcement test and the Discussionparser Sole-Parser Invariant

- **Context:**
  - `internal/pattern/leaf_enforcement_test.go`
  - `internal/discussionparser/validate.go`
- **Edits:**
  - `CONSTRAINTS.md`
- **Creates:**
  - `internal/discussionparser/leaf_enforcement_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create `internal/discussionparser/leaf_enforcement_test.go` in `package discussionparser`, modelled on `internal/pattern/leaf_enforcement_test.go`: a file-header comment naming the Discussionparser Sole-Parser Invariant and stating the allowlist is stdlib-only, an `allowedImports` map that is **empty** (no non-stdlib import is permitted), and `TestLeafInvariant_AllowlistOnly`, which uses `runtime.Caller(0)` to find the package directory, `filepath.WalkDir` to visit every non-`_test.go` `.go` file, and `parser.ParseFile(fset, path, nil, parser.ImportsOnly)` so only real import declarations are inspected.
  Classify an import as stdlib when its first path segment contains no `.`, exactly as the model file does, and fail with a message naming the violating file and import path.

  In `CONSTRAINTS.md`, add a `## Discussionparser Sole-Parser Invariant` section immediately after the existing `## Planparser Sole-Parser Invariant` section, mirroring its structure with three bullets: `internal/discussionparser` is the sole reader of `_lyx/discussion/`'s on-disk format, and no other package parses `decision-record.md`'s or `support-log.md`'s section shape;
  it declares no on-disk location of its own — `loomengine`'s `DiscussionDecisionRecord` / `DiscussionSupportLog` remain the sole declarers of where `_lyx/discussion/` is, deliberately unlike `planparser`, because those accessors take a `*lyxcwd.Location` a stdlib-only leaf may not import;
  and it imports the standard library and nothing else.
  Close with an **Enforced by** bullet naming `internal/discussionparser/leaf_enforcement_test.go` (`TestLeafInvariant_AllowlistOnly`), in the same phrasing the neighbouring leaf-invariant sections use.
  Use semantic line breaks throughout, per the `markdown-semantic-line-breaks` Shared Decision.
  Do not add the parity invariant here — batch 4 adds it, in its own section.
- **Commit:** `test(discussionparser): pin the stdlib-only leaf invariant and record it in CONSTRAINTS.md`

## Batch Tests

`verify: go test ./internal/discussionparser/... ./internal/lyxcwd/...` covers `internal/discussionparser/validate_test.go` (every behaviour case above) and `internal/discussionparser/leaf_enforcement_test.go` (the import allowlist).
`./internal/lyxcwd/...` is included because card 2 edits `CONSTRAINTS.md`, and `internal/lyxcwd/docslink_test.go`'s `TestEnforcement_MarkdownLinks` is the Markdown Link Integrity check that resolves links pointing into that file from `manifest/` and `docs/` — a new section heading there changes the anchor set those links resolve against.
Both packages are tier 1: no case spawns a process, and every path handed to `Validate` is a told absolute path under `t.TempDir()`.
