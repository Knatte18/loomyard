# Batch: focus-file contract

```yaml
task: 'shedadapters: Burler-round producer'
batch: 'focus-file contract'
number: 2
cards: 2
verify: go test ./internal/shedadapters/
depends-on: []
```

## Batch Scope

This batch defines the Go-side contract for the structured next-round focus file: an exported `RoundFocus` struct in `internal/shedadapters` plus a fail-safe reader for it, and the unit suite proving every degraded input path returns "no directive" and no error.
It is one batch because the struct, its reader, and its tests are a single new file pair with no dependency on anything else this plan builds.
The external interface batch 3 consumes is the reader function plus the `RoundFocus` value it returns; the external interface the follow-on `Bouncer` task consumes is the exported `RoundFocus` struct, which it marshals into the same JSON shape this reader parses, so there is exactly one format and one struct declaration.
Batch-local decision differing from `## Shared Decisions`: nothing.

## Cards

### Card 4: `RoundFocus` and its fail-safe reader

- **Context:**
  - `internal/shedadapters/ctx.go`
  - `internal/shedadapters/archive.go`
  - `internal/shedadapters/singlellm.go`
  - `internal/shedadapters/perch.go`
- **Edits:** none
- **Creates:**
  - `internal/shedadapters/focus.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/shedadapters/focus.go` with a file-header comment naming what the file implements, then the following declarations.
  An exported struct `RoundFocus` with exactly two exported fields: `ExcludeLenses []string` tagged `json:"exclude_lenses,omitempty"` and `Hydrate []string` tagged `json:"hydrate,omitempty"`.
  Its godoc must state that it is the structured, mechanically-parseable next-round directive a segment's `Bouncer` writes and a round producer reads, that both fields are optional, and — stated explicitly because getting it wrong fails silently — that the round token in the filename names the round the directives are **for**, not the round that produced them, so a `Bouncer` rejecting round `N` writes the file for round `N+1` and the seed call writes the file for round 1.
  An unexported constant or helper producing the filename shape `round-<round>-focus.json` for a positive decimal `round` with no leading zeros and no attempt suffix, joined onto a told absolute run directory.
  An unexported function `readRoundFocus(name, runDir string, round int) RoundFocus` that is fail-safe end to end and never returns an error: it reads the file at the resolved path, strictly decodes it as JSON with unknown fields rejected, and returns the zero `RoundFocus` after a `logger.Warn` when the file is absent, when reading it fails for any other reason, when the JSON is malformed, or when it carries an unknown field.
  Every warning carries `"producer", name`, `"engine", burlerEngineLabel`, the resolved path, and the reason.
  Declare the package-level constant `burlerEngineLabel = "burler"` in this file, mirroring `singleLLMEngineLabel` in `internal/shedadapters/singlellm.go` and `perchEngineLabel` in `internal/shedadapters/perch.go`; batch 3's producer reuses this one declaration rather than adding a second, so this file must compile and test standalone with no reference to anything batch 3 adds.
  After a successful decode, `readRoundFocus` filters `Hydrate` per entry: an entry that is not absolute, or that does not exist on disk, is dropped from the returned slice with a `logger.Warn` naming it, and the surviving entries keep their original order.
  `ExcludeLenses` is returned verbatim after a successful decode — whether a named lens is usable is decided at application time by the producer and by `burlerengine.Profile.validate`, not here.
  Reuse the `github.com/Knatte18/loomyard/internal/logger` import the package already uses in `internal/shedadapters/singlellm.go`; add no other new module dependency.
- **Commit:** `feat(shedadapters): add RoundFocus and its fail-safe reader`

### Card 5: Focus-file reader tests

- **Context:**
  - `internal/shedadapters/focus.go`
  - `internal/shedadapters/archive_test.go`
  - `internal/shedadapters/singlellm_test.go`
- **Edits:** none
- **Creates:**
  - `internal/shedadapters/focus_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/shedadapters/focus_test.go` covering `readRoundFocus`, using `t.TempDir()` for every fixture and asserting that the returned `RoundFocus` matches expectation in each case.
  Cover: a well-formed file yields both fields with their entries in file order; a file setting only `exclude_lenses` yields that field with an empty `Hydrate`; a file setting only `hydrate` yields that field with an empty `ExcludeLenses`; an absent file yields the zero value; an unreadable file yields the zero value — create a *directory* at the focus file's own path to produce a deterministic read failure rather than relying on file permissions, which do not fail for a privileged test process; malformed JSON yields the zero value; a file carrying a JSON field name the struct does not declare yields the zero value; a `hydrate` entry that is a relative path is dropped while the file's other, valid entries survive; and a `hydrate` entry naming an absolute path that does not exist on disk is dropped while the other entries survive.
  Also assert that the resolved filename uses the round the directives are for — a file written for round 3 is found when reading round 3 and is not found when reading round 4.
  Every case must assert the reader returned without an error value at all, since the function's signature deliberately has none.
  The test file stays untagged and spawns nothing.
- **Commit:** `test(shedadapters): cover the fail-safe focus-file reader`

## Batch Tests

`verify: go test ./internal/shedadapters/` runs the whole `internal/shedadapters` package suite: the new `focus_test.go` plus the existing `archive_test.go`, `ctx_test.go`, `perch_test.go`, `singlellm_test.go`, and `webster_test.go`.
Whole-package scope is correct here because Go's unit of compilation and test invocation is the package — there is no cheaper scoping for a package that gains a new file, and the suite is a pure, spawn-free Tier 1 suite that runs in well under a second.
No new smoke test is added: the reader is pure filesystem and JSON work with no LLM interaction.
