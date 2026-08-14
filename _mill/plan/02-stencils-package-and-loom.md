# Batch: stencils-package-and-loom

```yaml
task: "Relocate producer prompt files into a stencils/ directory"
batch: "stencils-package-and-loom"
number: 2
cards: 5
verify: go build ./... && go test ./stencils/... ./internal/loomengine/... ./internal/lyxcwd/...
depends-on: [1]
```

## Rename mechanic

For each `Moves:` pair the implementer MUST:

1. Run `git mv <old> <new>` FIRST, before making any other change to the moved file.
2. Make ONLY surgical edits — touch only the lines that must change after the move (package or module declaration, imports, identifier retargeting, seam splits).
3. Use a full-file `Creates:` entry only for genuinely new files that have no predecessor.
4. Never write the relocated file from scratch and delete the original — that breaks git rename history and inflates review diffs.

## Batch Scope

This batch creates the top-level `stencils/` Go package that every later move extends, and migrates the first family — loom's two prompts — end to end: `git mv` into `stencils/loom/`, the `//go:embed` directives and registry entries in `stencils/stencils.go`, deletion of `internal/loomengine`'s two embed-declaring files, and `loomengine`'s switch to `stencilstore.Read` at call time.
It also adds the `stencils` root to the Fabric Vocabulary enforcement walk, so the relocated prompts do not silently leave coverage, and the registry-completeness test that keeps the hand-maintained registry and the `.md` tree in lock-step as batches 4, 5, and 6 add the other thirteen.

The external interface the next batches consume is `stencils/stencils.go`'s shape: one typed `[]byte` var per stencil, one `registryEntry` row per stencil, and the `Registry()` accessor returning a `stencilstore.Registry`.
Batches 4, 5, and 6 each add rows to that same file, which is why they form a serial chain rather than running in parallel.

Batch-local decision: `loomengine`'s `PlanSpec` and `DiscussionSpec` gain an explicit `stencilsDir string` parameter rather than deriving one from the `*lyxcwd.Location` they already carry.
`loomengine` has no production caller today — only its own tests reach `PlanSpec`/`DiscussionSpec` — so an explicit parameter costs nothing now and keeps loom on the same told-never-derives footing as the three engines that genuinely need it.

## Cards

### Card 7: Create the top-level `stencils` package with loom's two embedded defaults and the registry

- **Context:**
  - `internal/stencilstore/stencilstore.go`
  - `internal/stencilstore/reconcile.go`
  - `plugins/prowler/main.go`
- **Edits:** none
- **Creates:**
  - `stencils/stencils.go`
- **Deletes:** none
- **Moves:**
  - `internal/loomengine/discussion-template.md` -> `stencils/loom/loom-template-discussion.md`
  - `internal/loomengine/plan-template.md` -> `stencils/loom/loom-template-plan.md`
- **Requirements:**
  Create `stencils/stencils.go` as the single Go file at the root of the new top-level `stencils/` directory, `package stencils`, import path `github.com/Knatte18/loomyard/stencils`.
  A top-level Go package outside `internal/` and `cmd/` is not unprecedented in this repo — `plugins/prowler` is already one.
  `//go:embed` reaches only files at or below the embedding package's own directory, which is why exactly one Go file must sit at `stencils/`'s root and why the family subfolders hold only `.md`.

  The file declares, for each stencil, an exported typed `[]byte` var fed by its own `//go:embed` directive — never an `embed.FS` — so a renamed or missing `.md` is a build error rather than a runtime one:

  ```go
  //go:embed loom/loom-template-discussion.md
  var LoomTemplateDiscussion []byte

  //go:embed loom/loom-template-plan.md
  var LoomTemplatePlan []byte
  ```

  Beside them, declare the name-to-default registry as an ordered slice plus an accessor implementing `stencilstore.Registry`:

  - `type registryEntry struct { name string; def *[]byte }`
  - `var entries = []registryEntry{ {"loom-template-discussion", &LoomTemplateDiscussion}, {"loom-template-plan", &LoomTemplatePlan} }` — kept in the same order stencils are listed, which is the order `lyx stencil list` prints.
  - `type registry struct{}` with `func (registry) Names() []string` returning every entry's `name` in slice order, and `func (registry) Default(name string) ([]byte, bool)` returning the matching entry's bytes.
  - `func Registry() stencilstore.Registry { return registry{} }` — the exported accessor `cmd/lyx`'s root pre-run and `internal/stencilcli` consume.

  This is the only file in the repo that names both a stencil's on-disk path and its Go identifier, so it is the one place a new stencil is registered.
  Do not have any engine import this package — an engine calls `stencilstore.Read(baseDir, name)`, which needs no registry, which is what keeps treadle's import allowlist to a single new entry.

  Both moved `.md` files keep their content byte-for-byte apart from the banner rewrite in card 8.

- **Commit:** `feat(stencils): add top-level stencils package with loom prompts and registry`

### Card 8: Rewrite the two relocated loom banners for their new filenames

- **Context:**
  - `stencils/stencils.go`
- **Edits:**
  - `stencils/loom/loom-template-discussion.md`
  - `stencils/loom/loom-template-plan.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Each of the two relocated files opens with a leading `<!-- ... -->` banner comment that `stencil.StripLeadingComment` removes before parsing, so it is read by humans only and never reaches the LLM.
  Update each banner so that any sentence naming the file's own old path (`discussion-template.md`, `plan-template.md`) or its old package location names the new path instead (`stencils/loom/loom-template-discussion.md`, `stencils/loom/loom-template-plan.md`).
  Where a banner describes the file as embedded in `internal/loomengine`, restate it as: shipped as an embedded default in the top-level `stencils` package, seeded to `<hub>/_board/_lyx/stencils/loom/` and read from there at call time.
  Do not change any `{{.marker}}` token, any instruction prose below the banner, or the body's line endings — the body's hash is what the stencil mechanism's edit detection keys on, and an accidental body change would make the seeded copy classify as human-edited from its first run.
- **Commit:** `docs(stencils): retarget relocated loom banners at their new paths`

### Card 9: Switch `loomengine` from embedded bytes to a call-time `stencilstore.Read`

- **Context:**
  - `internal/stencilstore/reconcile.go`
  - `stencils/stencils.go`
  - `internal/lyxcwd/lyxcwd.go`
- **Edits:**
  - `internal/loomengine/prompt.go`
  - `internal/loomengine/plan.go`
  - `internal/loomengine/discussion.go`
- **Creates:** none
- **Deletes:**
  - `internal/loomengine/prompttemplate.go`
  - `internal/loomengine/plantemplate.go`
- **Moves:** none
- **Requirements:**
  Delete `prompttemplate.go` and `plantemplate.go` outright — both files exist only to declare a `//go:embed` directive and its package var, and both directives now live in `stencils/stencils.go`.
  This removes the package vars `discussionTemplate` and `planTemplate`.

  In `prompt.go`, change `composePrompt` from
  `func composePrompt(slug, decisionRecordPath, supportLogPath string, autonomous bool) ([]byte, error)`
  to take a leading `stencilsDir string` parameter, and replace the `stencil.Fill(discussionTemplate, values)` call at `prompt.go:23` with a `stencilstore.Read(stencilsDir, "loom-template-discussion")` whose error is returned unwrapped, followed by `stencil.Fill` over the bytes it returned.

  In `plan.go`, change `composePlanPrompt` from
  `func composePlanPrompt(decisionRecordPath, planDir, overviewPath, patternDirective string) ([]byte, error)`
  to take a leading `stencilsDir string` parameter, and replace the `stencil.FillOptional(planTemplate, values, []string{"pattern_directive"})` call at `plan.go:41` with a `stencilstore.Read(stencilsDir, "loom-template-plan")` followed by the same `stencil.FillOptional` call over the bytes it returned, keeping the `pattern_directive` optional-marker list unchanged.

  Add a `stencilsDir string` parameter to both exported factories, threading it into the composer they call:
  - `func PlanSpec(layout *lyxcwd.Location, cfg Config, reg modelspec.Registry) (shuttleengine.Spec, error)` becomes `func PlanSpec(layout *lyxcwd.Location, stencilsDir string, cfg Config, reg modelspec.Registry) (shuttleengine.Spec, error)`.
  - `func DiscussionSpec(layout *lyxcwd.Location, cfg Config, reg modelspec.Registry, slug string, autonomous bool) (shuttleengine.Spec, error)` becomes `func DiscussionSpec(layout *lyxcwd.Location, stencilsDir string, cfg Config, reg modelspec.Registry, slug string, autonomous bool) (shuttleengine.Spec, error)`.

  A read now fails when the board is unavailable, and that failure is returned rather than absorbed: there is no fallback to an embedded default at runtime, by design.
  Do not import the top-level `stencils` package from `internal/loomengine` — the engine reads from disk by name and never needs the registry.
  `internal/webstercli/cli.go`'s `loomengine.PlanDir(layout)` call is unaffected; do not change it.

- **Commit:** `refactor(loomengine): read loom prompts from the stencils directory at call time`

### Card 10: Update `loomengine`'s tests for the new signatures

- **Context:**
  - `internal/loomengine/plan.go`
  - `internal/loomengine/discussion.go`
  - `internal/loomengine/prompt.go`
  - `internal/stencilstore/reconcile.go`
  - `stencils/stencils.go`
- **Edits:**
  - `internal/loomengine/plan_test.go`
  - `internal/loomengine/discussion_test.go`
  - `internal/loomengine/prompt_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Every call to `PlanSpec`, `DiscussionSpec`, `composePrompt`, and `composePlanPrompt` in these three files gains the new `stencilsDir` argument.
  Add a package-local test helper that builds the directory: create a `t.TempDir()`, write `loom/loom-template-discussion.md` and `loom/loom-template-plan.md` into it from `stencils.LoomTemplateDiscussion` and `stencils.LoomTemplatePlan`, and return the temp directory.
  Importing the top-level `stencils` package from a `_test.go` file is correct and carries no allowlist consequence — the allowlist rules in this repo police production imports only.

  Add one new test asserting the runtime-read contract that this change exists to deliver: write a modified body into the temp directory's `loom/loom-template-discussion.md`, call `DiscussionSpec`, and assert the modified text — not the embedded default — reaches the composed prompt.
  Add a second test asserting the hard error: call `DiscussionSpec` with a `stencilsDir` that does not exist and assert it returns an error whose message names the missing stencil, rather than silently falling back to the embedded default.
  Keep every existing assertion in these files intact.
- **Commit:** `test(loomengine): cover call-time stencil reads and the missing-board hard error`

### Card 11: Extend the enforcement walk to `stencils/`, pin registry completeness, and pin the new paths in `.gitattributes`

- **Context:**
  - `stencils/stencils.go`
  - `internal/stencilstore/stencilstore.go`
- **Edits:**
  - `internal/lyxcwd/enforcement_test.go`
  - `.gitattributes`
- **Creates:**
  - `stencils/registry_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `enforcement_test.go`, inside `TestEnforcement_FabricVocabulary`'s `tree-scan` subtest, change the `.md` walk at line 936 from
  `walkEnforcementRoots(t, repoRoot, []string{"internal"}, []string{".md"}, func(relPath string, data []byte) {`
  to walk `[]string{"internal", "stencils"}`.
  Without this the fifteen relocated prompts silently leave Fabric Vocabulary coverage the moment they leave `internal/`.
  In the same subtest, add a visit counter incremented inside that `.md` walk's callback and assert it is non-zero at the end, so a walk that silently visits nothing — a mistyped root, a directory that does not exist yet — fails loudly instead of passing vacuously.
  Update the walk's adjacent comment so it names both roots rather than only `internal/**/*.md`.
  Do not change the Go-file walk's roots: `stencils/stencils.go` is production Go outside `internal/` and `cmd/`, so it falls outside the Go half of this test, and this batch does not widen that half.

  `stencils/registry_test.go` is `package stencils`, untagged, no git spawn.
  It walks the family subfolders under the package's own directory for `*.md` files, derives each one's stencil name from its filename without the extension, and asserts the derived set and `Registry().Names()` name exactly the same stencils **in both directions** — a `.md` present but unregistered fails, and a registered name with no `.md` fails.
  It also asserts each entry's `Default(name)` returns non-empty bytes, and that `stencilstore.RelPath(name)` resolves to the file's actual relative path, which is what pins the family-from-first-token derivation against the on-disk layout.
  Without this test a hand-maintained registry silently omits a file: it would be invisible to `list`, never seeded, and never validated.

  In `.gitattributes`, add two rows pinning the relocated loom prompts to LF:
  ```
  stencils/loom/loom-template-discussion.md text eol=lf
  stencils/loom/loom-template-plan.md text eol=lf
  ```
  Place them with the other `//go:embed` target pins, under the existing leading comment block that explains why embedded assets are LF-pinned.
  Loom's two prompts are unpinned today, so this closes a pre-existing gap rather than only relocating rows.
  Do not remove any `internal/burlerengine` or `internal/treadleengine` row in this batch — those files have not moved yet, and their rows go stale in batches 4 and 5 respectively.
- **Commit:** `test(stencils): extend vocabulary walk to stencils, pin registry completeness and LF`

## Batch Tests

`verify: go build ./... && go test ./stencils/... ./internal/loomengine/... ./internal/lyxcwd/...`

The `go build ./...` half is not optional scope inflation: this batch deletes two `//go:embed`-declaring files and moves their assets across package boundaries, so a compile failure is the most likely way to get it wrong and it can surface in packages the test scope does not name.
The test half covers exactly the three packages with changed behaviour: the new `stencils` package's registry-completeness test, `loomengine`'s rewritten call-time-read tests, and `internal/lyxcwd`'s enforcement walk over the newly added `stencils` root.
`internal/lyxcwd`'s enforcement tests are the guard that the fifteen relocated files stay inside Fabric Vocabulary coverage, so they must run in every batch that moves prompts — batches 4, 5, and 6 repeat this same scope for that reason.
