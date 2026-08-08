# Batch: docs-alignment

```yaml
task: 'Scoutengine: rewrite CONSTRAINTS.md as a seam rule, convert leaf test to banned-list, add LSP guard'
batch: docs-alignment
number: 2
cards: 3
verify: go vet ./internal/scoutengine/ && go test ./internal/scoutengine/
depends-on: [1]
```

## Batch Scope

This batch brings the three documentation surfaces into line with the rule batch 1 now enforces: the package doc comment that still restates the deleted allowlist, the one word in the module table that still calls scout a leaf, and a verification pass over the already-committed `CONSTRAINTS.md` section.
It is one batch because all three are the same edit viewed from three altitudes, and none of them compiles or runs anything the others do not.
It depends on batch 1 because card 6 verifies the committed "Enforced by" line against the test paths and function names batch 1 actually lands — that check is meaningless before those files exist.

Batch-local decisions beyond `## Shared Decisions`:

- The `internal/modelspec` cross-reference at the tail of the paragraph card 4 rewrites goes with the paragraph, not just the import enumeration.
  It calls modelspec "the shape this package mirrors most directly" and says scout is cycle-free "the same way `internal/modelspec` already is". `modelspec` remains an allowlisted leaf with its own untouched `CONSTRAINTS.md` section, so keeping those sentences would re-import the leaf framing through the back door.
- `docs/overview.md` gets the one-word swap named in the discussion's Scope and nothing else.
  Its surrounding prose is out of scope.

## Cards

### Card 4: Rewrite the engine/CLI split paragraph in the package doc

- **Context:**
  - `CONSTRAINTS.md`
  - `docs/overview.md`
- **Edits:**
  - `internal/scoutengine/doc.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Rewrite the whole paragraph under the `# The engine/CLI split` heading — the block that currently opens "scoutengine is a leaf package" and closes with the sentence about being importable "the same way internal/modelspec already is".
  Nothing above that heading and nothing from the `# The generalized LSP client` heading onward changes.

  The rewritten paragraph must:

  - Drop the phrase "leaf package" and every other use of the word "leaf".
  - Drop the enumerated import list ("imports nothing beyond stdlib, internal/configengine, internal/lock, internal/proc, and gopkg.in/yaml.v3"), which is also already factually wrong — it omits `internal/logger`, which two production files in the package import.
  - Drop the `internal/modelspec` cross-reference in both places it appears: the parenthetical naming it as the shape this package mirrors most directly, and the closing "the same way internal/modelspec already is".
  - State the seam positively and negatively: typed `(T, error)` results out, never `internal/output`, cobra, or any `internal/*cli` package in, with `internal/scoutcli` as the sole consumer mapping results and errors onto the JSON envelope.
  - State that there is no import allowlist.
  - Keep the existing reference to the CLI/Cobra Invariant's "engine returns (T, error), cli emits the envelope" formulation, which is unchanged and still correct.

  A suitable replacement, for reference — match its substance, not necessarily its wording:

  ```go
  // # The engine/CLI split
  //
  // scoutengine is the engine half of an engine/CLI seam: it returns typed
  // Go results and typed errors and never imports internal/output, cobra, or
  // any internal/*cli package — no io.Writer, no exit codes, no output
  // envelope. internal/scoutcli is the sole consumer that maps engine
  // results/errors onto the internal/output JSON envelope (output.Ok/output.Err),
  // exactly the CLI/Cobra Invariant's "engine returns (T, error), cli emits
  // the envelope" split every other lyx module follows. Beyond that negative
  // rule there is no import allowlist: scoutengine draws on the shared
  // infrastructure layer as freely as any other engine module, which keeps it
  // cycle-free and importable by any future consumer (e.g. builder or webster)
  // without charging rent on each new dependency. CONSTRAINTS.md's "Scout
  // Engine-Seam Invariant" records the rule; the package's seam enforcement
  // test enforces it.
  ```

  Keep the file's existing comment style: `//`-prefixed package doc lines wrapped at roughly the same width as the surrounding paragraphs.
  Do not reflow paragraphs the card does not touch.

  This card is why the whole task is docs-and-tests: `docs/overview.md` designates this package doc as scout's module doc, its design doc having been deleted on landing, so the repo's "docs land in the same commit" rule binds it.
- **Commit:** `docs(scoutengine): restate the engine/CLI split as a seam, not an allowlist`

### Card 5: Stop calling scoutengine a leaf in the module table

- **Context:**
  - `CONSTRAINTS.md`
- **Edits:**
  - `docs/overview.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In the **scout** bullet of the module table, change the phrase `cycle-free leaf` to `cycle-free engine`.
  It appears in the clause reading "is a cycle-free leaf (typed results/errors, no `internal/output`)".

  That is the entire edit: one word, one occurrence.
  Do not reword the surrounding sentence, do not touch the parenthetical, and do not touch any other module's bullet.
  Confirm afterwards that no other occurrence of the word "leaf" in this file refers to scout.
  The file's other occurrences belong to `internal/reedengine/render`, `internal/modelspec`, `internal/tokenvocab`, `internal/pattern`, and `internal/shell` — all correct, all staying.
- **Commit:** `docs(overview): scoutengine is a cycle-free engine, not a leaf`

### Card 6: Verify the pre-staged invariant section matches what landed

- **Context:**
  - `CONSTRAINTS.md`
  - `internal/scoutengine/seam_enforcement_test.go`
  - `internal/scoutengine/lspclient_guard_test.go`
- **Edits:** none
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  The `## Scout Engine-Seam Invariant` section was written and committed during mill-start and is already on this branch.
  No card re-applies it.
  This card is a zero-diff gate confirming the pre-staged text still describes what batch 1 actually landed.

  Confirm, by reading the section:

  - Its "Enforced by" line names the two test file paths and the two function names that now exist, with no typo and no stale name: the seam enforcement test with `TestEngineSeamInvariant_BannedImports`, and the LSP client guard with `TestLSPClientGuard_StdlibAndLoggerOnly`.
  - Its narrower-guard bullet's allowed set — stdlib plus `internal/logger` — matches what the guard actually asserts, and its denial that the file is stdlib-only or hermetic is still accurate.
  - Its banned-list bullet still names `internal/clihelp` explicitly and still states that the policy covers direct imports only, never the transitive closure, matching the four predicates that landed.
  - Nothing in the section forbids `internal/scoutengine` from importing `internal/lyxcwd`, since the follow-up task `scout-lyxcwd-accessors` depends on exactly that being legal.

  If every check passes, this card is complete and produces no commit.
  If any check fails, stop and report the mismatch rather than editing — the section is authoritative and a divergence means batch 1 landed something the plan did not describe.

  Do not touch any other section of the file.
  The seven other leaf and seam invariants belong to the parallel `leaf-invariant-audit` task, and editing them would collide with work in flight.
- **Commit:** none

## Batch Tests

`verify:` runs `go vet ./internal/scoutengine/` followed by `go test ./internal/scoutengine/`.

Card 4 edits a Go file, so the package must still compile and vet clean — a malformed comment block or an accidentally-deleted closing line would show up there.
Re-running the package's tests is a two-second re-confirmation that the batch-1 guards are untouched, which matters because card 6 asserts things about their names.

Card 5 edits markdown, which has no runnable surface;
its verification is card 5's own closing check that no other scout-referring occurrence of the word survives.

Card 6 is a read-only gate with no diff to verify.
Its assertions are prose comparisons a test cannot express — nothing in the repo machine-reads `CONSTRAINTS.md` section titles or "Enforced by" lines.

Repo-wide verification is the configured done gate's job, not this batch's.
