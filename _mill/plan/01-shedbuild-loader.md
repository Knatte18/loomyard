# Batch: shedbuild-loader

```yaml
task: 'Shed recipe: loader/builder'
batch: shedbuild-loader
number: 1
cards: 5
verify: go test ./internal/shedbuild/...
depends-on: []
```

## Batch Scope

This batch creates the package `internal/shedbuild` and delivers its file-format half: the two exported types `Recipe` and `Row`, the byte-slice loader `Parse`, its told-path wrapper `Load`, and the machine-enforced told-geometry guard for the whole package.
It is one batch because the types, the decoder, and the decoder's tests are a single closed contract — nothing here needs `internal/shedrecipe`'s registry or `internal/shedengine`'s `ProducerDef`, and nothing outside this batch can be written until the `Recipe` shape exists.
The external interface batch 2 consumes is exactly the two exported types plus the guarantee that a `Recipe` returned by `Parse` has already passed every shape check, so `Build` re-checks only what a hand-built `Recipe` can still get wrong.
No batch-local decisions differ from the overview's `## Shared Decisions`.

## Cards

### Card 1: package doc and the `Recipe`/`Row` types

- **Context:**
  - `internal/shedrecipe/doc.go`
  - `internal/shedrecipe/recipe.go`
  - `internal/shedengine/producer.go`
  - `CONSTRAINTS.md`
- **Edits:** none
- **Creates:**
  - `internal/shedbuild/doc.go`
  - `internal/shedbuild/recipe.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create `internal/shedbuild/doc.go` carrying the `package shedbuild` clause and the package doc comment only, no declarations.
  The package doc states: this package owns the recipe file format, decoding a recipe document into a `Recipe` and assembling a `Recipe` plus a caller-supplied `shedrecipe.Env` into the `[]shedengine.ProducerDef` that `shedengine.Shed` already consumes unchanged;
  it holds the Told-Geometry Invariant in the form `CONSTRAINTS.md` pins, with `Load` reading exactly the absolute path it is told and the package deriving no path of its own;
  `Parse` is filesystem-free but `Build` is not, because three registry constructors reach disk at construction time between them, producing four distinct effects — the bouncer constructor does both, creating its resolved run directory and eagerly probing its rubric stencil, while the burler-round constructor only creates its run directory and the single-LLM constructor only probes its stencil — and this package neither suppresses nor wraps those effects;
  and this package defines no on-disk location for recipe files — no directory constant, no filename convention, no embedded default.
  Create `internal/shedbuild/recipe.go` declaring exactly two exported types and no functions.
  `Recipe` has fields `Version int`, `Entry string`, `Terminals []string`, and `Producers []Row`, with yaml tags `version`, `entry`, `terminals`, `producers`.
  `Row` has fields `Name string`, `Engine string`, `Config map[string]any`, `OnDone string`, `OnStuck string`, `Segment string`, and `MaxBounces int`, with yaml tags `name`, `engine`, `config`, `on_done`, `on_stuck`, `segment`, `max_bounces`.
  Every tag is written explicitly, including the ones yaml's default lowercasing would otherwise get right, because `on_done`, `on_stuck`, and `max_bounces` would not round-trip from `OnDone`, `OnStuck`, and `MaxBounces` without them.
  `Row.Config` is typed `map[string]any` rather than `shedrecipe.Config` so this file binds the decoded shape to no other package;
  `recipe.go` imports nothing at all.
  Document on `Recipe` that `Entry` and `Terminals` are graph metadata `Build` never consumes, surfaced because `shedcheck.Check` takes both as told arguments and refuses to infer either, and document on `Row.Config` that the decoded sub-map is passed to the row's engine untouched — never normalised, lowercased, or key-walked — because each registry entry owns its own key validation.
- **Commit:** `feat(shedbuild): add package doc and the Recipe/Row types`

### Card 2: `Parse` and `Load`

- **Context:**
  - `internal/modelspec/load.go`
  - `internal/shedrecipe/paths.go`
  - `internal/shedbuild/recipe.go`
- **Edits:** none
- **Creates:**
  - `internal/shedbuild/parse.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create `internal/shedbuild/parse.go` declaring an unexported `supportedVersion = 1` constant, the exported `Parse(data []byte) (Recipe, error)`, the exported `Load(path string) (Recipe, error)`, and whatever unexported shape-check helpers `Parse` factors out.
  `Parse` builds `dec := yaml.NewDecoder(bytes.NewReader(data))`, calls `dec.KnownFields(true)`, and decodes into a `Recipe` value — the same decoder shape `internal/modelspec/load.go` already uses, which is the repo's established strict-decode pattern and the only strict mechanism `gopkg.in/yaml.v3` offers.
  A `Decode` error that `errors.Is(err, io.EOF)` — what an empty, whitespace-only, or comments-only document yields — becomes the distinct error `shedbuild: recipe is empty`;
  every other `Decode` error is returned as `fmt.Errorf("shedbuild: %w", err)`, passing yaml's own text and its `line N:` position through verbatim.
  After a successful decode, `Parse` runs its own shape checks in this fixed order, returning the first failure:
  `Version != supportedVersion` yields `shedbuild: unsupported recipe version %d (supported: 1)`;
  an empty `Entry` yields `shedbuild: entry must not be empty`;
  an empty `Terminals` yields `shedbuild: terminals must not be empty`;
  an empty `Producers` yields `shedbuild: producers must not be empty`.
  Then, walking `Producers` in list order with the zero-based index `i`, per row:
  an empty `Name` yields `shedbuild: producer %d: name must not be empty`;
  an empty `Engine` yields `shedbuild: producer %d %q: engine must not be empty`;
  a negative `MaxBounces` yields `shedbuild: producer %d %q: max_bounces must not be negative, got %d`;
  and a `Name` already seen at an earlier index yields `shedbuild: producer %d %q: duplicate name, already defined by producer %d`, naming the earlier index last.
  `Version` is a plain `int`, not a `*int`, so an absent `version` key and a literal `version: 0` both decode to `0` and share the one `unsupported recipe version 0` message;
  neither is ever a valid recipe, so they are deliberately not told apart.
  `Parse` performs no check of its own that `config` is a mapping and no duplicate-mapping-key scan: `Row.Config`'s declared type makes a scalar or a list in that position fail inside `Decode`, and `gopkg.in/yaml.v3`'s own duplicate-key detection already rejects a repeated key at either level, so both are the decoder's job and a hand-rolled equivalent would be dead code.
  That duplicate-key rejection is not a consequence of `KnownFields(true)`: it is governed by the decoder's own unique-keys flag, which that library sets unconditionally when a decoder is constructed, so it holds independently of the strictness setting.
  `Parse` validates nothing about routing — it never checks that `OnDone` or `OnStuck` names an existing row, that segments agree, that the graph is acyclic, or that any row is reachable, because `shedengine`'s own validation and `internal/shedcheck` already own those and a third copy is what drifts.
  `Load` rejects a `path` that fails `filepath.IsAbs` before any read, with `shedbuild: recipe path %q must be absolute`, rejecting rather than resolving exactly as `internal/shedrecipe/paths.go` does;
  it then calls `os.ReadFile(path)`, returns `fmt.Errorf("shedbuild: read recipe %q: %w", path, err)` on failure, and otherwise delegates to `Parse`.
  `Load` is the only function in the package whose own code touches the filesystem.
- **Commit:** `feat(shedbuild): add Parse and Load`

### Card 3: `Parse` tests

- **Context:**
  - `internal/shedbuild/recipe.go`
  - `internal/shedbuild/parse.go`
- **Edits:** none
- **Creates:**
  - `internal/shedbuild/parse_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create `internal/shedbuild/parse_test.go` in `package shedbuild`, table-driven over inline YAML byte-slice literals with no filesystem access anywhere in the file.
  Each case asserts either the decoded `Recipe` value or a substring of the returned error.
  Cover, as separate cases: a minimal well-formed document with one row setting every field, round-tripping to the expected `Recipe`;
  a row with no `config:` block yielding a nil `Config` rather than an empty non-nil map;
  a row whose `config:` survives untouched, including a nested mapping and a list value, with key names and value types unaltered;
  row order preserved exactly in `Recipe.Producers`;
  malformed YAML (a tab-indented line and an unclosed quote) erroring under the `shedbuild: ` prefix;
  a missing `version` key and a literal `version: 0` as two inputs sharing the one `unsupported recipe version 0` message;
  `version: 2` and `version: -1` each naming the offending value;
  a duplicate mapping key at document level and a duplicate key inside one row, each asserted on the repeated key's name plus the `shedbuild: ` prefix and never on yaml's own wording;
  empty input, whitespace-only input, and comments-only input all yielding `shedbuild: recipe is empty`;
  an unknown document-level key and an unknown row-level key, each asserted on the offending key name plus the prefix only;
  a `config:` holding a scalar and a `config:` holding a list, each asserted on the prefix plus the presence of a line number and never on yaml's wording;
  empty `entry`, empty `terminals`, and empty `producers` each erroring distinctly;
  and per row, an empty `name`, an empty `engine`, and a negative `max_bounces`, each asserting the row's index appears in the message and, for the latter two, the row's name as well.
  Cover a duplicate `name` across two rows, asserting the name and both indices appear.
  Add one determinism test outside the table: parse the same unknown-key input twice and assert both calls report the same key, guarding against map-iteration-order nondeterminism in the rejection message.
- **Commit:** `test(shedbuild): cover Parse shape and decode errors`

### Card 4: `Load` tests

- **Context:**
  - `internal/shedbuild/parse.go`
  - `internal/shedbuild/recipe.go`
- **Edits:** none
- **Creates:**
  - `internal/shedbuild/load_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create `internal/shedbuild/load_test.go` in `package shedbuild`, small and deliberately not a second copy of the `Parse` table.
  Cover four things only: a well-formed recipe written into a `t.TempDir()` file parses to exactly the same `Recipe` value `Parse` returns for the same bytes;
  an absolute path naming no file errors;
  a relative path errors with the must-be-absolute message, asserted to fail before any read is attempted by using a relative path that does resolve to a real file from the test's working directory;
  and a directory path errors.
  Every path in this file derives from `t.TempDir()` except the relative-path case, which is what makes the reject-before-read assertion meaningful.
- **Commit:** `test(shedbuild): cover Load path handling`

### Card 5: told-geometry seam enforcement

- **Context:**
  - `internal/shedrecipe/seam_enforcement_test.go`
  - `CONSTRAINTS.md`
- **Edits:** none
- **Creates:**
  - `internal/shedbuild/seam_enforcement_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create `internal/shedbuild/seam_enforcement_test.go` in `package shedbuild`, copying the whole shape of `internal/shedrecipe/seam_enforcement_test.go`: a `runtime.Caller(0)`-derived package directory, a `filepath.WalkDir` over every non-`_test.go` `.go` file, `go/parser.ParseFile` with `parser.ImportsOnly`, a stdlib test on the import path's first segment, and a membership test against a package-level allowlist map.
  Name the test function `TestToldGeometryInvariant_AllowlistOnly`, matching the sibling's name exactly, since `CONSTRAINTS.md` names that function in its machine-enforced list.
  Name the allowlist map `shedbuildAllowedImports`, holding exactly four entries: `gopkg.in/yaml.v3`, `github.com/Knatte18/loomyard/internal/shedrecipe`, `github.com/Knatte18/loomyard/internal/shedengine`, and `github.com/Knatte18/loomyard/internal/shedcheck`.
  Keep the sibling's separate named-denylist half too: a `shedbuildDeniedLyxcwdImport` constant holding `github.com/Knatte18/loomyard/internal/lyxcwd`, collected and reported by name, so a violation of that specific rule is named rather than only implied by its absence from the allowlist.
  The file's header comment states that the allowlist is a membership list rather than a bare denylist for the same reason the sibling's is, and that this package's allowlist is deliberately short: it reaches the registry, the producer-definition type, and the checker, and nothing else.
- **Commit:** `test(shedbuild): enforce the told-geometry import allowlist`

## Batch Tests

`verify: go test ./internal/shedbuild/...` runs the whole new package, which after this batch is `parse_test.go`, `load_test.go`, and `seam_enforcement_test.go`.
The scope is exactly the batch's own `Creates:` set — no file outside `internal/shedbuild/` is touched, so no wider test target is warranted.
This is a Go project, so the command uses the native runner directly with no `PYTHONPATH=` prefix.
The batch is complete when `Parse` and `Load` are fully covered and the import allowlist is machine-enforced;
`Build` and `Check` do not exist yet, and no test in this batch references either.
