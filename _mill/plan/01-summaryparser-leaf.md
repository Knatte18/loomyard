# Batch: summaryparser-leaf

```yaml
task: "Producer-agnostic final-summary artifact + wire Finalize"
batch: "summaryparser-leaf"
number: 1
cards: 4
verify: go test ./internal/summaryparser/...
depends-on: []
```

## Batch Scope

This batch stands up `internal/summaryparser` as a self-contained, stdlib-only leaf and records the invariant that governs it, without touching a single existing caller.
Nothing outside the new package directory and `CONSTRAINTS.md` changes here, so the tree compiles and every existing test stays green throughout: `websterengine` still owns its four summary names, and no consumer has been retargeted yet.
The external interface batch 2 consumes is exactly `summaryparser.FileName`, `summaryparser.Path`, `summaryparser.Summary`, `summaryparser.Parse`, and `(*summaryparser.Summary).CommitMessage`.

The parse rules are fully specified by `_mill/discussion.md` before any code is written, so this batch is a TDD candidate: card 2's table can be written against card 1's declared signatures.

## Cards

### Card 1: Create the summaryparser leaf package

- **Context:**
  - `internal/websterengine/summary.go`
  - `internal/discussionparser/doc.go`
  - `internal/discussionparser/validate.go`
- **Edits:** none
- **Creates:**
  - `internal/summaryparser/doc.go`
  - `internal/summaryparser/summary.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create `internal/summaryparser/doc.go` holding only the package doc comment and the `package summaryparser` clause, following `internal/discussionparser/doc.go`'s convention.
  The doc comment states that this package is the sole declarer of the final-summary artifact's filename and the sole parser of its format, that it takes told paths and declares no directory of its own, and that it imports the standard library only so a second last-content producer can satisfy the same contract without depending on any producer.
  Do not name `_lyx` or `.lyx` anywhere in this package — the Lyxdirs Single-Declarer Invariant forbids it.

  Create `internal/summaryparser/summary.go` with a file-header comment in this tree's style, importing `fmt`, `os`, `path/filepath`, and `strings` and nothing else.
  It declares, in this order:

  - `const FileName = "summary.md"` — the artifact's fixed filename.
  - `func Path(dir string) string` returning `filepath.Join(dir, FileName)`, where `dir` is a told directory.
  - `type Summary struct { Title string; Body string }` — `Title` from the `"# <title>"` heading, `Body` the remaining lines verbatim.
  - `func Parse(path string) (*Summary, error)` — the read-and-validate function moved from `websterengine.ParseSummary`, behaviour-identical except for the error prefix.
    It reads the file, rejects a whitespace-only file, scans past leading blank lines to the first non-blank line, rejects a first non-blank line that does not have the `"# "` prefix, rejects an empty title after that prefix, and otherwise returns `&Summary{Title: title, Body: strings.Join(lines[headingIdx+1:], "\n")}`.
    Each of the four rejections is its own distinct wrapped error, and every error string is prefixed `summaryparser: ` in place of the original `webster: `.
    Preserve `Body`'s existing semantics exactly, leading newline included — `Publish` relies on them for the pull-request body.
  - `func (s *Summary) CommitMessage() string` — returns `s.Title` when `strings.TrimSpace(s.Body) == ""`, and otherwise `s.Title + "\n\n" + strings.TrimLeft(s.Body, " \t\r\n")`.
    Do not trim trailing whitespace; git normalizes that itself.
    Its doc comment states the git subject / blank line / body convention it follows and why the trim lives here rather than in `Parse`.

  Do not add an `ArchiveStaleSummary` or an `AppendIntegrationFailure` to this package; both stay in `internal/websterengine`.
- **Commit:** `feat(summaryparser): add the producer-agnostic final-summary read contract`

### Card 2: Parse, Path, and CommitMessage unit tests

- **Context:**
  - `internal/summaryparser/summary.go`
  - `internal/websterengine/summary_test.go`
- **Edits:** none
- **Creates:**
  - `internal/summaryparser/summary_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create `internal/summaryparser/summary_test.go` in the external test package `summaryparser_test`, matching `internal/websterengine/summary_test.go`'s own package choice, and carry over its `writeSummaryFile` helper.
  Every test here is untagged Tier 1: no `exec.Command`, no git, no `time.Sleep`.

  Cover `Parse` with the accept/reject scenarios `internal/websterengine/summary_test.go` exercises today, re-pointed at `summaryparser.Parse` and `summaryparser.FileName`: a well-formed artifact parsing into `Title` and `Body` exactly (with `Body` keeping its leading newline); a heading preceded by blank lines; a missing file; an empty and a whitespace-only file; a first non-blank line that is not a `"# "` heading; and a `"# "` heading whose title is blank or whitespace-only.

  Cover `Path`: it returns the told directory joined with `FileName`.

  Cover `CommitMessage` with each case the **commitmessage-body-trim** Shared Decision names: a `Body` starting with a newline, asserting exactly one blank line between subject and body; an empty `Body` and a whitespace-only `Body`, both yielding the bare `Title` with no trailing blank line; a `Body` with no leading blank line at all, unchanged by the trim; a `Body` whose trailing whitespace survives untouched; and a `Body` carrying an appended `## Integration suite failed` section, which must reach the composed message intact.

  Do not assert on exact error strings beyond the `summaryparser: ` prefix — the four rejections are distinguished by being distinct errors, not by matched text.
- **Commit:** `test(summaryparser): cover Parse, Path, and CommitMessage`

### Card 3: Leaf-import enforcement test

- **Context:**
  - `internal/discussionparser/leaf_enforcement_test.go`
- **Edits:** none
- **Creates:**
  - `internal/summaryparser/leaf_enforcement_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Copy `internal/discussionparser/leaf_enforcement_test.go` into `internal/summaryparser/leaf_enforcement_test.go` verbatim in structure — the `go/parser` `ImportsOnly` walk of every non-test `.go` file in the package directory, the stdlib detection by "no `.` in the first path segment", and the intentionally empty `allowedImports` allowlist.
  The file is in the in-package `summaryparser` package, as its source is in `discussionparser`.
  Rename the header comment, the `t.Fatal`/`t.Fatalf` directory-name text, and the final `t.Errorf` message to name the **Summaryparser Sole-Parser Invariant** and this package.
  Keep the test function named `TestLeafInvariant_AllowlistOnly`.
- **Commit:** `test(summaryparser): enforce the stdlib-only leaf allowlist`

### Card 4: Record the Summaryparser Sole-Parser Invariant

- **Context:** none
- **Edits:**
  - `CONSTRAINTS.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add a `## Summaryparser Sole-Parser Invariant` section to `CONSTRAINTS.md`, placed immediately after the existing `## Discussionparser Sole-Parser Invariant` section and kept to that entry's two-line length.
  It states: in production code, `internal/summaryparser` is the sole declarer of the final-summary artifact's filename and the sole parser of its format; it imports the standard library only.
  The "in production code" scope is load-bearing and must appear — bare filename literals in test fixtures stay legal.

  Do not add `internal/summaryparser` to the Told-Geometry Invariant's bound-package list: that list carries `planparser` but not `discussionparser`, and this package follows `discussionparser`, whose stdlib-only rule is strictly stronger than the bound-package rule and is enforced by its own test.
  Do not extend or reword any other invariant in the file.
- **Commit:** `docs(constraints): record the Summaryparser Sole-Parser Invariant`

## Batch Tests

`verify: go test ./internal/summaryparser/...` runs the whole new package: `summary_test.go`'s `Parse`/`Path`/`CommitMessage` table and `leaf_enforcement_test.go`'s import allowlist.
The scope is exactly this batch's `Creates:` set — nothing outside `internal/summaryparser/` has any runnable surface changed here, and `CONSTRAINTS.md` has none at all.
Card 4 is therefore covered by review discipline rather than a test, exactly as the `Discussionparser Sole-Parser Invariant`'s own sole-declarer half is today.
