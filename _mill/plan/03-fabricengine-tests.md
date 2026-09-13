# Batch: fabricengine-tests

```yaml
task: "Rename hub container suffix from -HUB to -LYXHUB"
batch: "fabricengine-tests"
number: 3
cards: 4
verify: go test ./internal/fabricengine/... && go test -tags integration -run TestOwnership_FabricHubKind ./internal/fabricengine/...
depends-on: [1]
```

## Batch Scope

Every remaining `internal/fabricengine` test file that carries the suffix literal: six untagged files holding synthetic hub-path fixtures, one untagged file that also holds an arbitrary sentinel constant, and four integration-tagged files whose hits are all inside comments.
It also adds the one piece of new coverage this package owes — a subtest proving structural hub discovery is name-blind, so a container still carrying the retired suffix is not refused.

It is one batch because every file belongs to the same package and the same three-class sorting rule decides each hit, so a single reader holds the whole judgement in one head.
`internal/fabricengine/junctionnames_test.go` is deliberately not here — it sits in batch 1 alongside the constant it tests.

Batch-local decision beyond `## Shared Decisions`: the name-blindness subtest extends the existing `TestOwnership_FabricHubKind`, rather than opening a new untagged test, because that test is already the package's home for `looksLikeHub` shape coverage.
Extending it is what makes this batch's verify carry a tagged invocation.

## Cards

### Card 6: Sweep untagged synthetic hub-path fixtures

- **Context:**
  - `internal/fabricengine/junctionnames.go`
- **Edits:**
  - `internal/fabricengine/fabric_test.go`
  - `internal/fabricengine/hubscratch_test.go`
  - `internal/fabricengine/junction_test.go`
  - `internal/fabricengine/origin_test.go`
  - `internal/fabricengine/portallauncher_test.go`
  - `internal/fabricengine/warplayout_fastpath_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Every hit in these six files is a synthetic hub-container path fixture standing for the real suffix, so each becomes the new suffix.
  None of them touches disk, and none may be converted into one that does.

  - `internal/fabricengine/fabric_test.go` — the location literal whose hub field is `filepath.Join("/hub", "myrepo-HUB")` becomes `"myrepo-LYXHUB"`.
  - `internal/fabricengine/hubscratch_test.go` — three `hub := filepath.Join(string(filepath.Separator), "synthetic", "repo-HUB")` fixtures become `"repo-LYXHUB"`.
  - `internal/fabricengine/junction_test.go` — the location literal whose hub field is `filepath.Join("home", "user", "project-HUB")` becomes `"project-LYXHUB"`.
  - `internal/fabricengine/origin_test.go` — three `hub := filepath.Join("repos", "loomyard-HUB")` fixtures become `"loomyard-LYXHUB"`.
  - `internal/fabricengine/portallauncher_test.go` — two `hub := filepath.Join("repos", "loomyard-HUB")` fixtures become `"loomyard-LYXHUB"`.
  - `internal/fabricengine/warplayout_fastpath_test.go` — the `HubPath: filepath.Join(t.TempDir(), "mono-HUB")` field becomes `"mono-LYXHUB"`.

  Change no assertion logic, no test name, and no helper — only the fixture strings.
- **Commit:** `test(fabricengine): sweep synthetic hub-path fixtures to -LYXHUB`

### Card 7: Update `clone_reset_guard_test.go`, leaving its sentinel intact

- **Context:**
  - `internal/fabricengine/clone.go`
- **Edits:**
  - `internal/fabricengine/clone_reset_guard_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In the file-header comment, the sentence describing the R4 defect names the derived container directory shape as the thing that was destroyed.
  Substitute the retired suffix for the new one there, matching the identical sentence in `internal/fabricengine/clone.go`'s `resetHub` doc comment — it is a claim about the path the code derives today, not a dated record.

  Do not edit the `sentinel` const whose value is the string "NOT-A-HUB-USER-DATA".
  That value is an arbitrary recognisable marker written into a directory to prove the guard did not delete it;
  it happens to contain the retired token as a substring but does not denote the hub suffix, and rewriting it would change what the test writes and looks for without changing what it proves.
  Leave it byte for byte as it stands.
- **Commit:** `test(fabricengine): update clone reset guard header to -LYXHUB`

### Card 8: Sweep integration-tagged comment references

- **Context:**
  - `internal/fabricengine/clone.go`
- **Edits:**
  - `internal/fabricengine/livestate_doc_test.go`
  - `internal/fabricengine/livestate_verbs_test.go`
  - `internal/fabricengine/warpbinding_clone_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  All four hits across these three files are comment references to the derived container directory shape, spelled either as the `<derived>-HUB` form or the `<name>-HUB` form.
  Each becomes the corresponding `-LYXHUB` form.

  - `internal/fabricengine/livestate_doc_test.go` — the comment line describing a non-hub directory at the derived path, in the entry that also names `resetHub`'s own pre-flight.
  - `internal/fabricengine/livestate_verbs_test.go` — two comment lines, one describing a directory that is not a hub being refused at the pre-flight, and one describing a directory whose name happens to match the derived path being refused at `resetHub`.
  - `internal/fabricengine/warpbinding_clone_integration_test.go` — the comment line asserting that neither the derived container nor any throwaway-clone-prefixed directory is left behind.

  These are comments in tagged files: nothing executable changes, but the files must still compile under the integration tag.
  Read `internal/fabricengine/clone.go` to confirm the derivation each comment describes before rewording.
- **Commit:** `test(fabricengine): sweep integration-tagged comments to -LYXHUB`

### Card 9: Pin name-blind hub discovery, leaving the outside-parent sentinel intact

- **Context:**
  - `internal/fabricengine/clone.go`
  - `internal/fabricengine/export_test.go`
  - `internal/fabricengine/junctionnames.go`
- **Edits:**
  - `internal/fabricengine/destructivegaps_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add one subtest to the existing `TestOwnership_FabricHubKind`, placed immediately after its `AcceptsWeftSiblingOnly` subtest and written in the same shape as its siblings (a `t.Run` with `t.Parallel()`).
  Name it `AcceptsLegacySuffixedHubDirectory`.

  It must build a directory under `t.TempDir()` whose basename carries the retired suffix — `loomyard-HUB` — create the board entry inside it using `fabricengine.BoardDirName`, and assert that `fabricengine.LooksLikeHubForTest` returns true for it.
  Give it a comment recording what it pins: `looksLikeHub` is purely structural, so a container created before the suffix rename still resolves as a hub and is never refused on the strength of its name.
  This is the behaviour the clean break depends on, and without this case nothing in the repository asserts it.

  Use only `os.MkdirAll` and `filepath.Join` — the subtest must not clone, spawn git, or build a fixture hub, exactly like the `AcceptsBoardEntryOnly` and `AcceptsWeftSiblingOnly` subtests beside it.

  Do not edit the `sentinel` const whose value is the string "OUTSIDE-PARENT-HUB-CONTENT".
  It is an arbitrary recognisable marker, not the hub suffix, and it stays byte for byte as it stands.
  The two lines in this file that build a container path by appending `fabricengine.HubSuffix` need no edit at all — they read the constant and updated themselves in batch 1.
- **Commit:** `test(fabricengine): pin that hub discovery ignores the container suffix`

## Batch Tests

`verify` is two chained invocations.

`go test ./internal/fabricengine/...` covers the seven untagged files this batch edits — the six fixture files in card 6 and `internal/fabricengine/clone_reset_guard_test.go` in card 7.
Their changes are fixture strings and comments, so the expectation is that every existing assertion passes unchanged;
the run's job is to catch a mis-edited fixture that breaks a path comparison.

`go test -tags integration -run TestOwnership_FabricHubKind ./internal/fabricengine/...` covers the four integration-tagged files this batch edits.
Building the tagged test binary compiles all of them, which is the whole risk for cards 8's comment-only edits, and the `-run` filter then executes exactly the test card 9 extends — including its new `AcceptsLegacySuffixedHubDirectory` subtest.
The filter is deliberate: the rest of the tagged suite clones real repositories and is several minutes of work that this batch does not change, and it is already covered task-wide by `pipeline.done_gate`, configured as `go test ./... && go test -tags integration ./...`.
