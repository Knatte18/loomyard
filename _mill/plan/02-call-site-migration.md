# Batch: call-site-migration

```yaml
task: 'batcher: split out of webster into a standalone configreg module with its own batcher.yaml'
batch: 'call-site-migration'
number: 2
cards: 4
verify: go build ./... && go test ./internal/websterengine/... ./internal/webstercli/... && go test -tags integration ./internal/websterengine/... ./internal/webstercli/...
depends-on: [1]
```

## Batch Scope

This batch moves the batchifier selection off `webster.yaml` and onto `batcher.yaml`: the `Batcher` field leaves `websterengine.Config` and its template, `runlevel.go`'s `batcher.Select` block becomes a nil-guard over a new injected `RunDeps.Batcher`, and `webstercli`'s `PersistentPreRunE` calls `batcher.Active` instead of `batcher.Select`.
It is one batch because it is an atomic *correctness* unit, which is a wider boundary than the compile boundary and the reason the batch does not stop at card 5.
Card 5 alone compiles under both build tags, so a green build after it would be misleading: `newRunFixture`'s `Config{}` literal never set a `Batcher:` key and `verbs_test.go`'s only `Config.Batcher` mention is inside a comment, so nothing in the two `//go:build integration` files breaks the compiler.
What breaks is behaviour — every `TestRun_*` would hit the new `ErrNilBatcher` guard with an unpopulated `RunDeps.Batcher`, and `verbs_test.go`'s `strings.Replace` against the removed `batcher: ""` literal would silently no-op and leave the gate pair asserting nothing.
Cards 6–8 repair exactly that, so the batch is only correct as a whole.

Card 5 is deliberately one large card rather than several: every *production* file it names, plus the untagged `internal/websterengine/config_test.go`, stops compiling the moment `Config.Batcher` is deleted, so splitting it would leave `go build ./...` broken at an intermediate commit.
Cards 6–8 are separate because each targets a distinct `//go:build integration` file (and card 7 adds new evidence rather than repairing existing evidence), and both the untagged build and the untagged test run stay green across all three regardless of order.

The external interface batch 3 documents is the finished state: `batcher.yaml`'s `active:` key, `batcher.Active`, `RunDeps.Batcher`, and the fact that no webster code calls `batcher.Select` any more.

Batch-local decisions beyond `## Shared Decisions`:
`internal/webstercli/verbs_test.go`'s direct `batcher.Select("")` call at the hand-built `*websterCLI` literal stays as-is and only its comment is rewritten — routing that literal through `Active` would force the test to materialize an `_lyx/config/` tree for a test whose whole point is bypassing `PersistentPreRunE`.
The `batcher.Active` call keeps `batcher.Select`'s exact position in `PersistentPreRunE`, after the three `LoadConfig` calls, so the shuttle → reed → webster → batcher not-found error precedence every existing fixture expectation depends on is unchanged.

## Cards

### Card 5: move batchifier selection from webster.yaml to batcher.yaml

- **Context:**
  - `internal/batcher/config.go`
  - `internal/batcher/registry.go`
  - `internal/batcher/batcher.go`
- **Edits:**
  - `internal/websterengine/config.go`
  - `internal/websterengine/template.yaml`
  - `internal/websterengine/template.go`
  - `internal/websterengine/runlevel.go`
  - `internal/webstercli/cli.go`
  - `internal/webstercli/run.go`
  - `internal/websterengine/config_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `internal/websterengine/config.go`, delete the `Config` struct's `Batcher` field (the one carrying the yaml tag `batcher`) together with its whole doc comment — the comment block beginning "Batcher names the active batchifier (see internal/batcher.Select) that", which is deleted with the field and therefore needs no separate doc-batch step.
  Also amend `Config`'s own type doc comment, which currently lists "the batchifier selection" among what the resolved `webster.yaml` carries;
  drop that clause.
  Nothing else in this file changes — `LoadConfig` never referenced `Batcher`.
  In `internal/websterengine/template.yaml`, delete the entire `batcher: ""` line including its trailing comment.
  Do not renumber or reflow the surrounding lines.
  In `internal/websterengine/template.go`, amend `ConfigTemplate`'s doc comment, which today enumerates what the template holds as "role model-specs, batchifier selection, and Master session configuration";
  drop the "batchifier selection" clause.
  This is the exact sibling of the `Config` type-doc amendment above and belongs in the same card rather than in batch 3: a grep for `batcher.Select`/`batcher:`/`batcher.yaml` does not reach this line (it says "batchifier selection"), so leaving it to the documentation batch would let the tree carry a self-falsifying template doc across the whole of batch 2.
  Change no code in that file — the embed directive and the accessor body are untouched.
  In `internal/websterengine/runlevel.go`, declare a new package-level sentinel beside `ErrRunBusy`:

```go
// ErrNilBatcher marks Run's refusal when RunDeps.Batcher was never populated
// by the caller. Population is webstercli's obligation (PersistentPreRunE
// resolves it via batcher.Active); a nil interface here would panic inside
// Batch rather than surface a diagnosable error.
var ErrNilBatcher = errors.New("webster: RunDeps.Batcher not populated")
```

  Add a `Batcher batcher.Batcher` field to `RunDeps` with a doc comment stating it is the CLI-resolved active batchifier (`batcher.Active`) and that `Run` refuses with `ErrNilBatcher` when it is nil, and extend `RunDeps`' own type doc comment (which today ends "Config and Roles carry the loaded configuration and pre-flight-resolved role->model-spec map") to name `Batcher` alongside `Config` and `Roles` as CLI-pre-resolved.
  Replace the three-line `active, err := batcher.Select(deps.Config.Batcher)` block plus its `fmt.Errorf("webster: %w", err)` wrap with a nil guard in exactly that position, and change the following line's `active.Batch(plan.Cards)` to `deps.Batcher.Batch(plan.Cards)`:

```go
	if deps.Batcher == nil {
		return RunResult{}, ErrNilBatcher
	}
	batches := deps.Batcher.Batch(plan.Cards)
```

  The guard MUST sit before the `Batch` call, not beside the zero-batch refusal that follows it — that refusal runs after the very call that would panic.
  Leave the zero-batch refusal itself and its comment exactly as they are.
  Keep the `internal/batcher` import in `runlevel.go`: it is still needed for the `[]batcher.Batch` parameter types on `mapMasterDone`, `batchIdentity`, `verifyEveryBatchDone`, `runIntegrationStage`, and `accumulatedCardSHAs`.
  In `internal/webstercli/cli.go`, replace `activeBatcher, err := batcher.Select(websterCfg.Batcher)` with `activeBatcher, err := batcher.Active(layout.AnchorPath())`, leaving the surrounding `if err != nil` / `output.Err` / `clihelp.Abort` block, the call's position after the `websterengine.LoadConfig` call, and the `c.batcher = activeBatcher` assignment all untouched.
  Do not move the call earlier in `PersistentPreRunE` even though it no longer depends on `websterCfg`.
  In `internal/webstercli/run.go`, add `Batcher: c.batcher,` to the `websterengine.RunDeps` literal, placed adjacent to the existing `Roles:` and `Config:` fields so the literal's field order still mirrors the struct's.
  In `internal/websterengine/config_test.go`, delete the `Batcher: "",` line from the `websterengine.Config` literal in `TestConfigTemplate_RoundTripsThroughLoadConfig`'s `want`, delete the `batcher: identity` line from the `override` YAML string in `TestLoadConfig_OverridesRoundTrip`, delete the `cfg.Batcher != "identity"` assertion block from that same test, and delete the `batcher: identity` line from the `badRole` YAML string in `TestLoadConfig_BadRoleGrammarNamesTheKey`.
  Leave `TestConfigTemplate_ContainsEveryConfigYAMLTag` and `containsKey` untouched — they walk `Config`'s fields reflectively and stay correct once the field is gone.
- **Commit:** `refactor(batcher): move batchifier selection from webster.yaml to batcher.yaml`

### Card 6: inject the batchifier into websterengine's Run fixture

- **Context:**
  - `internal/websterengine/runlevel.go`
  - `internal/batcher/registry.go`
- **Edits:**
  - `internal/websterengine/runlevel_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add `github.com/Knatte18/loomyard/internal/batcher` to the import block.
  In `newRunFixture`, resolve the identity batchifier once via `batcher.Select("")` (a `t.Fatalf` on its error) and add a `Batcher:` field to the `websterengine.RunDeps` literal carrying it, placed adjacent to `Roles:` and `Config:`.
  `Select` is the right call here rather than `Active`: the fixture's `WorktreeRoot` is a bare scratch git repo with no `_lyx/` tree, and the point of the runlevel-call-site decision is that `Run` needs no config tree.
  Rewrite the file-header comment's parenthetical, which today reads "against the real identity batchifier — Config.Batcher is left empty in every fixture, resolving to internal/batcher's own DefaultName", so it instead states that the real identity batchifier is injected into `RunDeps.Batcher` by `newRunFixture` and that `Run` itself does no config I/O and needs no `_lyx/` tree on `WorktreeRoot`.
  Keep every other clause of that header comment intact.
  Do not seed a config file anywhere in this file — if these tests start needing one, the runlevel-call-site decision has been implemented wrongly.
  All 15 existing `TestRun_*` tests must pass with no other edit.
- **Commit:** `test(webster): inject the identity batchifier into the Run fixture`

### Card 7: pin the nil-batcher refusal

- **Context:**
  - `internal/websterengine/runlevel.go`
- **Edits:**
  - `internal/websterengine/runlevel_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add `TestRun_NilBatcherRefuses` to `internal/websterengine/runlevel_test.go`.
  Build a fixture via `newRunFixture`, set `fx.Deps.Batcher = nil`, call `websterengine.Run(fx.Deps, websterengine.RunOptions{})`, and assert `errors.Is(err, websterengine.ErrNilBatcher)`.
  Nothing else exercises this branch — `webstercli` always populates the field — so this test is the sentinel's only evidence.
  The test must reach the guard, which sits after `ParsePlan`, `planparser.Validate`, and the run-lock acquisition, so use the standard fixture rather than a hand-built `RunDeps`.
  Do not assert on the error string beyond what `errors.Is` gives;
  the message text is pinned by the sentinel's own declaration in card 5.
- **Commit:** `test(webster): pin Run's nil-batcher refusal`

### Card 8: relocate webstercli's PersistentPreRunE batcher gate

- **Context:**
  - `internal/batcher/template.go`
  - `internal/batcher/template.yaml`
  - `internal/webstercli/cli.go`
  - `internal/lyxtest/lyxtest.go`
- **Edits:**
  - `internal/webstercli/verbs_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Change `seedPersistentPreRunFixture`'s parameter from `websterConfig string` to `batcherConfig string`, seed `"webster": websterengine.ConfigTemplate()` unconditionally, and add `"batcher": batcherConfig` to the `lyxtest.SeedConfig` map alongside the existing `shuttle`/`reed`/`webster` entries.
  Rewrite its doc comment: the parenthetical "webster.yaml's raw content is caller-supplied, so a test can override its batcher: key" becomes the same rationale for batcher.yaml's `active:` key, and the closing clause naming where load-time batcher selection is wired stays accurate (`PersistentPreRunE`, now via `batcher.Active`).
  In `TestPersistentPreRunE_UnknownBatcherFailsFast`, change the fixture line so the replacement targets batcher's template instead of webster's:

```go
	batcherConfig := strings.Replace(batcher.ConfigTemplate(), `active: ""`, `active: "bogus"`, 1)
	seedPersistentPreRunFixture(t, batcherConfig)
```

  Update its doc comment to name `batcher.Active(baseDir)` and `batcher.yaml`'s `active:` key in place of `batcher.Select(cfg.Batcher)` and `webster.yaml`'s `batcher:` key.
  Every assertion stays byte-identical: exit code 1, `"ok":false`, and both `unknown batcher` and `bogus` present.
  In `TestPersistentPreRunE_DefaultBatcherResolves`, change the fixture argument to `batcher.ConfigTemplate()` and update its doc comment the same way;
  its exit-0 and `"initialized":false` assertions stay byte-identical.
  Also update the two `t.Fatalf` format strings in that pair, which read "status with an unknown batcher: name" and "status with the default batcher: name", so they no longer imply a `batcher:` key.
  Rewrite the comment above the hand-built `*websterCLI` literal (today: "The default (empty) batcher name resolves to the identity batchifier -- exactly what PersistentPreRunE would have resolved and stored on c.batcher, bypassed here along with the rest of PersistentPreRunE") so it names `Active`-resolved `c.batcher` as the thing being stood in for.
  Leave the `batcher.Select("")` call itself and its `t.Fatalf` untouched — `Select` remains exported with unchanged `"" → DefaultName` behaviour, and this test deliberately bypasses `PersistentPreRunE` rather than exercising it.
  Keep the existing `github.com/Knatte18/loomyard/internal/batcher` import;
  it now serves both `Select` and `ConfigTemplate`.
- **Commit:** `test(webstercli): drive the PersistentPreRunE batcher gate off batcher.yaml`

## Batch Tests

`verify:` runs `go build ./...` first, because the field deletion in card 5 is a compile-level change that must be proven across every package before any test runs.

The untagged run (`go test ./internal/websterengine/... ./internal/webstercli/...`) covers card 5's `internal/websterengine/config_test.go` edits — `TestConfigTemplate_RoundTripsThroughLoadConfig`, `TestLoadConfig_OverridesRoundTrip`, `TestLoadConfig_BadRoleGrammarNamesTheKey`, and the reflective `TestConfigTemplate_ContainsEveryConfigYAMLTag`, which is what mechanically proves the struct and the template stayed in sync after the field left both.

The tagged run (`go test -tags integration ./internal/websterengine/... ./internal/webstercli/...`) carries this batch's decisive evidence and is not optional: `internal/websterengine/runlevel_test.go` and `internal/webstercli/verbs_test.go` are both `//go:build integration`, so a plain `go test ./...` neither runs nor even *compiles* them.
A broken `RunDeps` literal in `newRunFixture` (card 6) or a `strings.Replace` still targeting the removed `batcher: ""` literal (card 8) would both land green under the untagged run alone.
Cards 6–8 are covered there by the 15 existing `TestRun_*` tests, the new `TestRun_NilBatcherRefuses`, and the relocated `TestPersistentPreRunE_UnknownBatcherFailsFast`/`TestPersistentPreRunE_DefaultBatcherResolves` pair.

`internal/webstercli`'s four `c.batcher` consumers (`awaitbatch`, `recordbatch`, `beginbatch`, `recoverbatch`) are unaffected by design;
their existing tests run in the same tagged invocation and must not need edits.
