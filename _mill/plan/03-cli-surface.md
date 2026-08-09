# Batch: cli surface

```yaml
task: 'fabric: store the warp-URL binding in weft:main; fold bootstrap into clone (slice 10)'
batch: 'cli surface'
number: 3
cards: 3
verify: go build ./... && go test -tags integration ./internal/fabriccli/ && go test ./cmd/lyx/
depends-on: [2]
```

## Batch Scope

This batch completes the observable CLI surface of the flip: the `--force-bootstrap` flag, the rewritten `Use`/`Long` help text and both examples, the two new success-envelope keys, and the CLI-level tests for the new arity and the unbound-weft error.
It is one batch because the CLI/Cobra Invariant makes help text, flag registration, and the envelope one reviewable unit — the help must describe exactly the flags and forms the handler accepts, and `cmd/lyx`'s help-tree tests exercise the `Use` string.

No external interface is produced for later batches;
batch 5 edits `internal/fabriccli/fabric.go` again but in a different function (`runReconcile`), which is why it is sequenced after this batch rather than run alongside it.

## Cards

### Card 8: clone command help, examples, and the --force-bootstrap flag

- **Context:**
  - `internal/fabriccli/clone.go`
  - `internal/fabricengine/clone.go`
  - `internal/fabricengine/warpbinding.go`
  - `internal/weftname/weftname.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/fabriccli/fabric.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Every place the clone subcommand spells its argument order changes together.

  Delete the stale comment line above the command declaration that reads `// clone [--reset] <warp-url> <weft-url> [board-url]` and replace it with one naming the current form.

  Set `Use` to `clone [--reset] [--subpath <rel>] [--force-bootstrap] <weft-url> [<warp-url>]`.
  Leave `Short` as it is — it is non-empty and still accurate.

  Register the new flag beside the existing two: `cloneCmd.Flags().Bool("force-bootstrap", false, "bypass the weft-candidate guard when bootstrapping a brand-new weft remote")`.
  Read it in the `RunE` closure alongside `reset` and `subpath` and pass it through to `runCloneWithReset`.

  Rewrite the `Long` body:
  - Add a paragraph, early, explaining the two forms — `lyx fabric clone <weft-url>` derives the warp URL from the binding recorded on `weft:main`, and `lyx fabric clone <weft-url> <warp-url>` supplies it explicitly, which is required the first time a weft is bound and is a hard error when it disagrees with an existing binding.
  - Explain the binding itself in one short paragraph: a plain single-line record at the board root holding the warp URL only, committed onto `weft:main` beside the recorded lyx-anchor subpath, written the first time a warp URL is supplied for an unbound weft.
    Name the file with the `fabricengine.WarpBindingFileName` constant rather than a hardcoded literal if the surrounding string concatenation style allows it;
    the existing `Long` already concatenates `weftname.Suffix` this way.
  - Update the `<warp-name>` explanation line so it no longer implies the warp URL is always supplied — the hub is still named after the warp repo, but in the one-argument form that name is derived from the recorded binding.
  - Document `--force-bootstrap` as applying to exactly one situation and nothing else: a brand-new weft remote that is neither empty nor already lyx-anchored (for example one created with an auto-generated README), which the weft-candidate guard would otherwise refuse.
    State that it is ignored in the one-argument form and whenever a binding is already recorded.
  - Keep the existing `--reset` and `--subpath` paragraphs, the weft-suffix paragraph, the `_board` paragraph, and the clone-wires-everything paragraph unchanged in substance.
  - Flip both examples to weft-first and make the pair demonstrate both forms, e.g. a two-argument bootstrap with `--subpath backend` and a one-argument bound clone.

  Per the CLI/Cobra Invariant, help accuracy is a review obligation on any change to observable behaviour: re-read the whole `Long` after editing and confirm no sentence still implies two required positionals.
- **Commit:** `feat(fabriccli): document both clone forms and add --force-bootstrap`

### Card 9: thread --force-bootstrap and widen the success envelope

- **Context:**
  - `internal/fabricengine/clone.go`
  - `internal/fabriccli/fabric.go`
- **Edits:**
  - `internal/fabriccli/clone.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add a `forceBootstrap bool` parameter to `runCloneWithReset` and pass it into `CloneOptions.ForceBootstrap`, replacing the placeholder `false` and its accompanying comment from batch 2.
  Keep the parameter order aligned with the flag order in the caller (`reset`, `subpath`, `forceBootstrap`).

  Extend the success envelope returned by `output.Ok` from `{"hub", "anchor"}` to `{"hub", "anchor", "warp", "warp_binding_recorded"}`:
  `"warp"` carries `res.WarpURL` (the effective URL actually cloned, whether supplied or derived) and `"warp_binding_recorded"` carries `res.WarpBindingRecorded` as a bool.
  Both keys are always present, so a consumer never has to distinguish absent from false.

  Update the function's doc comment to mention the two new keys.
- **Commit:** `feat(fabriccli): report the effective warp URL and binding write in the clone envelope`

### Card 10: CLI-level arity, envelope, and unbound-weft tests

- **Context:**
  - `internal/fabriccli/clone.go`
  - `internal/fabriccli/fabric.go`
  - `internal/fabricengine/warpbinding.go`
  - `internal/fabricengine/clone.go`
- **Edits:**
  - `internal/fabriccli/cli_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Rewrite `TestRunCLI_CloneRequiresExactlyTwoArgs` — its name and its whole table encode the old rule.
  Rename it `TestRunCLI_CloneAcceptsOneOrTwoArgs` and give it a doc comment stating the new arity contract.
  The usage-error subtests become zero positionals and three positionals, both asserting exit 1, `ok == false`, and an error containing `usage: lyx fabric clone`.
  Neither reaches a git spawn, so `t.TempDir()` plus `t.Chdir` remains sufficient with no fixture.

  Add `TestRunCLI_CloneUnboundWeftErrorNamesTwoArgForm`: a one-positional clone against a local bare weft fixture that carries no binding must exit 1 through the `output.Err` envelope with a message containing both `has no recorded warp binding` and `lyx fabric clone <weft-url> <warp-url>`.
  Build the fixture with the file's existing `makeCLICloneWeftBare` helper.
  This test does spawn git (the probe clones the fixture), which is fine — the whole file is already `//go:build integration` and the package already has a `TestMain` calling `lyxtest.HermeticGitEnv()`.

  Flip the two existing end-to-end clone tests to weft-first.
  `TestRunCLI_CloneEndToEnd` passes `--subpath backend` then the weft bare, then the warp bare;
  `TestRunCLI_CloneDefaultSubpathAnchorsAtRoot` passes the weft bare then the warp bare.
  Both keep every existing assertion — the JSON envelope's `hub` and `anchor`, the wired junctions, the tracked anchor marker and repo-wide config on the board worktree, and the per-worktree board config.

  Extend `TestRunCLI_CloneEndToEnd` with three assertions for the new surface:
  the envelope's `warp` key equals the warp bare path the test passed in;
  the envelope's `warp_binding_recorded` key is `true` (this is a first-ever clone of that weft fixture);
  and the binding file named by `fabricengine.WarpBindingFileName` is tracked on the board worktree, checked with the same `git ls-files` shape the existing loop uses for the anchor marker and `fabric.yaml` — add it to that loop's path list rather than writing a second loop.

  Do NOT pass `--force-bootstrap` on the two end-to-end clones.
  `makeCLICloneWeftBare` creates a genuinely empty bare repo with zero commits, which is the unborn-HEAD case the weft-candidate guard admits on its own — the flag would be redundant, and passing it here would leave a future reader doubting the guard's unborn-HEAD carve-out.
  The new unbound-weft test uses the same helper for the same reason.
- **Commit:** `test(fabriccli): cover the new clone arity, envelope keys, and unbound-weft error`

## Batch Tests

`verify:` is `go build ./... && go test -tags integration ./internal/fabriccli/ && go test ./cmd/lyx/`.

`-tags integration` is required for the `internal/fabriccli` half because `internal/fabriccli/cli_test.go` — the only test file this batch edits — carries `//go:build integration` on its first line.

`go test ./cmd/lyx/` is the help-surface gate: `helptree_test.go`, `drift_test.go`, `registration_test.go`, and `longlist_test.go` all read the live cobra tree, so a `Use` string or `Long` body that drifts from the registered flags fails there and nowhere else.
It is run untagged because those four tests are untagged and need no fixtures.

`go build ./...` covers the `runCloneWithReset` signature change reaching its single caller in `internal/fabriccli/fabric.go`.
