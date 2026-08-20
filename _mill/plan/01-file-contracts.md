# Batch: file-contracts

```yaml
task: 'Bouncer: the generic review-gate producer'
batch: 'file-contracts'
number: 1
cards: 3
verify: go test ./internal/shedadapters/...
depends-on: []
```

## Batch Scope

This batch lands everything the Bouncer reads and writes on disk, with no producer and no spawn: the three fail-loud file-contract parsers (verdict, ledger, focus), the focus-file writer, the exported `ResolveRound` scan, and the three round-path helpers.
It is one batch because all six pieces are pure functions over bytes and the filesystem, testable without a `Shuttle`, and because batch 3's `Call` consumes every one of them.
The external interface batch 3 consumes: `parseVerdict`, `parseLedger`, `parseFocus`, `writeFocus`, `ResolveRound`, `verdictPath`, `ledgerPath`, `focusPath`.
No batch-local decisions differ from `## Shared Decisions` in the overview.

## Cards

### Card 1: the three file-contract parsers

- **Context:**
  - `internal/treadleengine/judgeverdict.go`
  - `internal/treadleengine/handoff.go`
  - `internal/burlerengine/verdict.go`
  - `internal/shedadapters/doc.go`
  - `CONSTRAINTS.md`
- **Edits:** none
- **Creates:**
  - `internal/shedadapters/bouncerfiles.go`
  - `internal/shedadapters/bouncerfiles_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create `internal/shedadapters/bouncerfiles.go` in package `shedadapters` with a file header comment stating that it defines the three file contracts the Bouncer owns, that every parser here is fail-loud, and that the fail-safe swallow lives one layer up in `Call`.
  Copy the *posture* of `internal/treadleengine/judgeverdict.go` and `internal/treadleengine/handoff.go`; do not import `internal/treadleengine`.

  Declare `splitFrontmatter(content []byte, kind string) (string, error)`: the file must open with a `---` line, must have a later `---` line, and the header between them must be non-whitespace.
  Each failure returns a distinct `bouncer: %s file ...`-prefixed error naming `kind`.
  Declare `frontmatterProse(content []byte) string` returning everything after the closing `---`, CRLF-normalised and trimmed, and `""` when there is nothing after it.

  Declare `type bouncerVerdict string` with the two constants `verdictApproved bouncerVerdict = "APPROVED"` and `verdictBlocking bouncerVerdict = "BLOCKING"`.
  Declare `parseVerdict(content []byte) (bouncerVerdict, string, error)`: `kind` is `"verdict"`; the frontmatter unmarshals into an unexported header struct with `verdict` and `rationale` yaml tags; the verdict must be exactly one of the two constants, case-sensitively; the rationale must be non-empty after `strings.TrimSpace`.

  Declare `type ledgerEntry struct { Key string; Rounds []int; Status string }` and `type ledgerFile struct { Round int; Entries []ledgerEntry; Prose string }`.
  Declare `parseLedger(content []byte) (ledgerFile, error)`: `kind` is `"ledger"`; the frontmatter carries `round` (must be a positive int) and `ledger` (a list, legally empty, of entries each needing a non-empty `key`, a non-empty `rounds` list whose every member is positive, and a `status` of exactly `open` or `resolved`).
  An empty `ledger` list yields an empty, non-nil `Entries` slice.
  `Prose` is `frontmatterProse(content)`.
  `parseLedger` compares nothing against any previous ledger — carry-forward is stated in the judge prompt and is deliberately not enforced here.

  Declare `type focusFile struct { Round int; ExcludeLenses []string; Focus []string; Prose string }`.
  Declare `parseFocus(content []byte) (focusFile, error)`: `kind` is `"focus"`; the frontmatter carries `round` (positive int), `exclude_lenses` (a list of strings, legally empty), and `focus` (a list of strings, legally empty).
  A scalar where a list is required is a parse error, which `yaml.Unmarshal` into a `[]string` field already produces.
  Both list fields yield empty, non-nil slices when absent or empty.

  Every header struct omits `KnownFields`, so an unknown extra key in any of the three files is tolerated.
  Use `gopkg.in/yaml.v3`, the same dependency `internal/treadleengine/handoff.go` already uses.

  Create `internal/shedadapters/bouncerfiles_test.go` with table-driven tests, one case per rule, each asserting on the returned error's text rather than merely on non-nil.
  Cover for each of the three file types: a missing opening `---`; a missing closing `---`; empty frontmatter; invalid YAML; prose correctly extracted and CRLF-normalised; an unknown extra key tolerated.
  Verdict-specific: both legal spellings accepted; a lowercase `approved` rejected; an unknown verdict rejected; an empty rationale rejected; a whitespace-only rationale rejected.
  Ledger-specific: an empty `ledger` list legal; an empty `key` rejected; an empty `rounds` list rejected; a zero or negative member of `rounds` rejected; a `status` outside `open`/`resolved` rejected; a zero or negative `round` rejected.
  Add one ledger case named for the accepted soft spot: a ledger whose entries drop a key present in a previous ledger parses cleanly, asserting that carry-forward enforcement is absent by design.
  Focus-specific: empty `exclude_lenses` and empty `focus` both legal; a zero or negative `round` rejected; a scalar in place of the `focus` list rejected.
- **Commit:** `feat(shedadapters): add the Bouncer's three fail-loud file-contract parsers`

### Card 2: the focus-file writer

- **Context:**
  - `internal/treadleengine/handoff.go`
- **Edits:**
  - `internal/shedadapters/bouncerfiles.go`
  - `internal/shedadapters/bouncerfiles_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add `renderFocus(f focusFile) ([]byte, error)` to `internal/shedadapters/bouncerfiles.go`: it marshals an unexported header struct carrying `round`, `exclude_lenses`, and `focus` through `yaml.Marshal`, wraps the result between an opening and a closing `---` line, and appends `f.Prose` below the closing delimiter when `f.Prose` is non-empty.
  Marshalling rather than hand-formatting the header is what makes a lens name containing a YAML metacharacter round-trip instead of corrupting the file.
  Both list fields must render as an explicit empty list rather than as `null` when the slice is empty, so the emitted file satisfies `parseFocus`'s always-present-list shape — initialise a nil slice to an empty, non-nil slice before marshalling.

  Add `writeFocus(path string, f focusFile) error`: renders via `renderFocus` and writes the bytes with `os.WriteFile` at mode `0o644`, wrapping any failure in a `bouncer: `-prefixed error naming the path.

  Add round-trip tests to `internal/shedadapters/bouncerfiles_test.go`: for each of several `focusFile` values, assert that `parseFocus(renderFocus(f))` yields back a value equal to `f`.
  Include explicitly the seed-fallback shape — `focusFile{Round: 1}` with both lists empty — since that is what the seed call's fallback emits and what the round producer reads as its round-1 input.
  Include a case with a lens name containing a colon and a space, and a case carrying prose.
  Add one `writeFocus` test writing into a `t.TempDir()` and reading the file back through `parseFocus`.
- **Commit:** `feat(shedadapters): add the Bouncer's focus-file writer`

### Card 3: `ResolveRound` and the round-path helpers

- **Context:**
  - `internal/treadleengine/roundfiles.go`
  - `_mill/discussion.md`
- **Edits:** none
- **Creates:**
  - `internal/shedadapters/round.go`
  - `internal/shedadapters/round_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create `internal/shedadapters/round.go` in package `shedadapters` with a file header comment stating that it is the single place a round number becomes a concrete path, mirroring `internal/treadleengine/roundfiles.go`'s discipline, and that the Bouncer has no attempt concept because a retry here is a `Shed` bounce rather than an in-producer retry.

  Declare the exported `ResolveRound(runDir string, reportName func(round int) string) (int, error)` with a doc comment pinning its contract for both halves of a segment: it returns the highest round `N` for which `reportName(N)` names an existing file inside `runDir`, scanning upward from 1 and stopping at the first absent round; it returns `0` when `reportName(1)` does not exist, which is a legal non-error answer meaning no round has been run yet; the Bouncer's "round to judge" is the return value and the round producer's "round to write" is the return value plus one.
  Implementation: `os.Stat(runDir)` first, returning a `shedadapters: `-prefixed error when it fails or when the result is not a directory — a mistyped or not-yet-created run dir must surface as an error rather than as a fresh-segment `0`.
  Then loop `n` from 1, stat `filepath.Join(runDir, reportName(n))`, and classify with `errors.Is(err, fs.ErrNotExist)`: only that error means absent and returns `n-1`; every other stat error is returned wrapped.
  Do not test `err != nil` and assume absence — a permission or I/O failure read as absence would truncate the scan mid-sequence or re-seed a segment that has already judged rounds.

  Declare three unexported helpers taking `(runDir string, round int) string` and joining onto `runDir`: `verdictPath` returning `round-<N>-bouncer-verdict.md`, `ledgerPath` returning `round-<N>-bouncer-ledger.md`, and `focusPath` returning `round-<N>-focus.md`.
  Document on `focusPath` that its round is the round the file *targets*, not the round that wrote it, so the round producer reads its own round's focus file with no off-by-one reasoning.

  Create `internal/shedadapters/round_test.go` covering: an empty run dir returns `0` with a nil error; only round 1's report present returns `1`; reports for rounds 1 through 3 present returns `3`; reports for rounds 1 and 3 present with round 2 absent returns `1`, asserting explicitly that `3` is not returned; a `runDir` that does not exist returns an error rather than `0`; and a report whose stat fails for a reason other than not-exist returns an error rather than `0` or a truncated scan, arranged by chmod-ing the run dir to a non-searchable mode and skipped when the test runs as root or on a platform where the mode cannot be arranged.
  Add one test asserting both derived readings against a single disk state: with reports for rounds 1 and 2 on disk, the round to judge is `2` and the round to write is `3`.
  Add a small table test over the three path helpers pinning their exact filename spellings.
- **Commit:** `feat(shedadapters): add ResolveRound and the Bouncer's round-path helpers`

## Batch Tests

`verify: go test ./internal/shedadapters/...` runs the whole package's tests, which after this batch covers `bouncerfiles_test.go` and `round_test.go` alongside the existing `archive_test.go`, `ctx_test.go`, `perch_test.go`, `singlellm_test.go`, and `webster_test.go`.
Package-wide scope is correct here rather than over-broad: Go's test unit is the package, this batch adds new files to an existing package, and the package's existing suite is fast and filesystem-only.
Every parser test is table-driven and asserts a specific error message; the writer test asserts a parser round-trip rather than a byte-for-byte rendering, so the emitted YAML's incidental formatting stays free to change.
