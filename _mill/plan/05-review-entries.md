# Batch: Bouncer and BurlerRound entries

```yaml
task: 'Shed recipe: engine registry'
batch: 'Bouncer and BurlerRound entries'
number: 5
cards: 5
verify: go test ./internal/shedrecipe/...
depends-on: [4]
```

## Batch Scope

This batch delivers the last two registry entries, `Bouncer` and `BurlerRound`, which together form one review segment and are the only two entries reading `Env.RunRoot`.
They are one batch because their shared `run_subdir` key is a two-row relationship: the same value in both rows must resolve to the same `RunDir`, and different values in two segments must resolve to different ones, so the two entries cannot be reviewed apart from each other.
Both entries create their joined run directory with `os.MkdirAll` at construction, which no other entry in this package does.
The external interface batch 6's coverage guard consumes is the `"Bouncer"` and `"BurlerRound"` keys in `registry.go`'s map literal, completing the set of twelve.

Batch-local decisions beyond `## Shared Decisions`:

- Directory creation at construction is required, not defensive.
  `Bouncer.Call` reaches `shedadapters.ResolveRound` first, which `os.Stat`s `RunDir` and returns a hard error when it is absent, and the `Bouncer` is its segment's entry point, so it runs before the `BurlerRound` whose own `Call` would otherwise have created the directory.
  The caller cannot pre-create it either, because the joined `RunRoot/<run_subdir>` path exists only inside the entry.
- `BurlerRound`'s `profile.target.paths` and `profile.fasit.paths` are the single documented exception to the join-and-reject-absolute rule, passed through relative and unjoined, and not absolute-checked.

## Cards

### Card 14: The Bouncer entry

- **Context:**
  - `internal/shedrecipe/recipe.go`
  - `internal/shedrecipe/config.go`
  - `internal/shedrecipe/env.go`
  - `internal/shedrecipe/paths.go`
  - `internal/shedadapters/bouncer.go`
  - `internal/shedadapters/round.go`
  - `internal/shedadapters/burler.go`
- **Edits:** none
- **Creates:**
  - `internal/shedrecipe/entries_bouncer.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Write one unexported function `bouncerEntry` matching the `Constructor` signature.

  Recognised `Config` keys, and nothing else: required `run_subdir` (via `configString`), `artifact_paths` (via `configStringSlice`), and `rubric_stencil` (via `configString`);
  optional `model`, `effort`, and `version` (each via `configString`).
  End the extraction with `configRejectUnknown(cfg, "run_subdir", "artifact_paths", "rubric_stencil", "model", "effort", "version")`.
  There is deliberately no `report_name` key — see the `ReportName` requirement below.

  `Env` validation: `requireAbsRoot` on `Env.RunRoot`, `Env.WorktreeRoot`, and `Env.StencilsDir`, then `requireSeam` on `Env.Shuttle`.

  Resolve `run_subdir` with `resolveUnderRoot(entry, "run_subdir", env.RunRoot, value)` into the run directory, then create it with `os.MkdirAll(runDir, 0o755)`, wrapping a failure with the path.
  A comment at the `MkdirAll` call states why the entry creates the directory rather than the caller or the round producer, naming both facts: `Bouncer.Call` reaches `shedadapters.ResolveRound`, which hard-errors on an absent `RunDir`, and the `Bouncer` is the segment's entry point so it runs before `BurlerProducer.Call`'s own idempotent `MkdirAll` would have created it.
  Resolve every `artifact_paths` entry with `resolveUnderRoot(entry, "artifact_paths", env.WorktreeRoot, value)`.

  Build a `shedadapters.BouncerConfig` with: `Name` from the `name` argument;
  `RunDir` the joined run directory;
  `ArtifactPaths` the resolved absolute slice;
  `ReportName` a closure rendering exactly `fmt.Sprintf("round-%d-review.md", round)`;
  `StencilsDir` from `Env.StencilsDir`;
  `RubricStencil` from `Config.rubric_stencil`;
  `Model`, `Effort`, `Version` from `Config`;
  `Shuttle` from `Env.Shuttle`;
  `Now` from `Env.Now`.
  A comment at `ReportName` states why it is pinned rather than configurable: `shedadapters.BurlerProducer` writes its report to a hardcoded `round-<n>-review.md` under `RunDir`, and `shedadapters.ResolveRound` finds the current round by statting that same name, so any other value resolves the round to 0 forever and the `Bouncer` re-seeds every call until its bounce budget is spent, with no error anywhere.

  Return `shedadapters.NewBouncer(cfg)`, surfacing its error wrapped rather than discarded — that constructor's own eager rubric-stencil probe is what makes a mistyped `rubric_stencil` fail here.
- **Commit:** `feat(shedrecipe): add the Bouncer registry entry`

### Card 15: The BurlerRound entry

- **Context:**
  - `internal/shedrecipe/recipe.go`
  - `internal/shedrecipe/config.go`
  - `internal/shedrecipe/env.go`
  - `internal/shedrecipe/paths.go`
  - `internal/shedadapters/burler.go`
  - `internal/burlerengine/profile.go`
  - `internal/burlercli/run.go`
- **Edits:** none
- **Creates:**
  - `internal/shedrecipe/entries_burler.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Write one unexported function `burlerRoundEntry` matching the `Constructor` signature.

  Recognised outer `Config` keys, and nothing else: required `run_subdir` (via `configString`) and `profile` (via `configMap`);
  optional `model` and `effort` (via `configString`) and `timeout_s` (via `configInt`).
  End the outer extraction with `configRejectUnknown(cfg, "run_subdir", "profile", "model", "effort", "timeout_s")`.

  The `profile` map recognises exactly six keys, all optional at this level: `target` and `fasit` (each via `configMap`), `rubric` (via `configString`), `fix-scope` (via `configString`), `tool-use` (via `configBool`), and `cluster-fan` (via `configString`), followed by `configRejectUnknown(profile, "target", "fasit", "rubric", "fix-scope", "tool-use", "cluster-fan")`.
  Each of `target` and `fasit` recognises exactly `paths` (via `configStringSlice`) and `instructions` (via `configString`), each followed by its own `configRejectUnknown(inner, "paths", "instructions")` call, so the strict unknown-key rule runs at all three levels.
  A comment states two things: that these six key names are a hand-maintained duplicate of `internal/burlercli`'s `profileYAML` kebab-case shape, kept identical deliberately so a human who has written a burler profile file reads a recipe row without a second vocabulary;
  and that `review-path`, `fixer-report-path`, `prior-reviews`, `prior-fixer-reports`, and `cluster-exclude` are deliberately absent because `shedadapters.NewBurlerProducer`'s own doc states those five `burlerengine.Profile` fields and `burlerengine.RunOpts.Round` are overwritten per round, so a recipe author setting one would be setting a value the producer silently discards.
  The entry does not check `profile`'s inner required-ness: `burlerengine.Profile.validate` already rejects an empty `Rubric` and a `Target`/`Fasit` with neither `Paths` nor `Instructions`, and duplicating that here would drift from it.

  Build a `burlerengine.Profile` literal filling only `Target`, `Fasit`, `Rubric`, `FixScope` (converted with `burlerengine.FixScope(...)`), `ToolUse`, and `ClusterFan`, leaving every other field zero.
  `Target.Paths` and `Fasit.Paths` are passed through **relative and unjoined**, and `resolveUnderRoot` is not called on them and the absolute-rejection check is not applied to them.
  A comment at that site states this is the single exception to the general relative-path rule, with the reason: `burlerengine.Profile.validate` already resolves them against its own told worktree root and stats every resolved entry for existence, so joining here would either double-resolve or hand `validate` an absolute path it did not resolve itself, and an author who writes an absolute path there gets `validate`'s behaviour rather than this package's.

  Build a `burlerengine.RunOpts` filling `Model` and `Effort` from `Config` and `Timeout` as `time.Duration(timeoutS) * time.Second`, leaving `Round` zero.

  `Env` validation: `requireAbsRoot` on `Env.RunRoot`, then `requireSeam` on `Env.Burler`.
  Resolve `run_subdir` with `resolveUnderRoot(entry, "run_subdir", env.RunRoot, value)` and create the joined directory with `os.MkdirAll(runDir, 0o755)`, with a comment stating that the same `run_subdir` value in this row and its segment's `Bouncer` row is what makes both write into one directory, which is what lets `shedadapters`' `roundComplete` find this producer's report where the `Bouncer` looks for it.

  Return `shedadapters.NewBurlerProducer(name, env.Burler, profile, opts, runDir, env.Now)`, surfacing its error wrapped.
- **Commit:** `feat(shedrecipe): add the BurlerRound registry entry`

### Card 16: Register Bouncer and BurlerRound

- **Context:**
  - `internal/shedrecipe/entries_bouncer.go`
  - `internal/shedrecipe/entries_burler.go`
- **Edits:**
  - `internal/shedrecipe/registry.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add two entries to the `registry` map literal: `"Bouncer": bouncerEntry` and `"BurlerRound": burlerRoundEntry`, bringing the table to its full twelve keys.
  Update the map's own comment: drop the forward reference to keys later batches still owe, and state instead that the table is complete at twelve and that any thirteenth entry must arrive with a coverage-guard update in the same commit.
  Change `Lookup` and `Names` in no way.
- **Commit:** `feat(shedrecipe): register the Bouncer and BurlerRound engines`

### Card 17: Tests for the Bouncer entry

- **Context:**
  - `internal/shedrecipe/entries_bouncer.go`
  - `internal/shedrecipe/fixture_test.go`
  - `internal/shedrecipe/registry.go`
  - `internal/shedadapters/bouncer.go`
  - `internal/shedadapters/burler.go`
  - `internal/stencilstore/reconcile.go`
  - `internal/stencilstore/stencilstore.go`
- **Edits:** none
- **Creates:**
  - `internal/shedrecipe/entries_bouncer_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Write tests in the `shedrecipe` package, all geometry from `newTestEnv(t)`, writing a real rubric stencil into `env.StencilsDir` for the happy paths because `shedadapters.NewBouncer` probes it eagerly.
  The on-disk shape that stencil must take is the one `stencilstore.Path`/`stencilstore.RelPath` define in `internal/stencilstore/stencilstore.go` — read it rather than guessing.

  Happy path: a minimal valid `Config` plus a filled `Env` constructs a non-nil `shedengine.ShedProducer` with no error.

  Run-directory subtests, the batch's load-bearing group:
  the joined `RunRoot/<run_subdir>` directory exists on disk immediately after construction and before any `Call`, asserted with `os.Stat`;
  two `Bouncer` entries built with different `run_subdir` values resolve to two different, both-existing directories;
  an omitted `run_subdir` fails at construction naming the key;
  a `run_subdir` present with an empty-string value fails at construction naming the key;
  an absolute `run_subdir` fails;
  a `..`-escaping `run_subdir` fails.

  Report-name pinning: assert the pinned filename is byte-identical to what `shedadapters.BurlerProducer` writes for the same round.
  Because `BouncerConfig.ReportName` is not reachable from outside `shedadapters`, assert it behaviourally: construct the entry, write a file named `round-1-review.md` into the joined run directory, and assert the constructed producer's `Call` advances past the seed branch rather than re-seeding — read `internal/shedadapters/bouncer.go`'s `Call` to pick the observable that distinguishes the two branches before writing the assertion.
  Also assert a `report_name` key in `Config` is rejected as unknown.

  Remaining construction failures, each asserting the error names the offending thing: a missing `artifact_paths`, an empty `artifact_paths` list, an absolute `artifact_paths` entry, a `..`-escaping `artifact_paths` entry, a missing `rubric_stencil`, a `rubric_stencil` naming a stencil that does not exist, an unrecognised `Config` key, and one subtest per read `Env` field — blanking `Env.RunRoot`, `Env.WorktreeRoot`, or `Env.StencilsDir`, or nilling `Env.Shuttle`.
  One subtest asserts a nil `Env.Now` constructs successfully.
- **Commit:** `test(shedrecipe): cover the Bouncer entry, its run dir, and its pinned report name`

### Card 18: Tests for the BurlerRound entry

- **Context:**
  - `internal/shedrecipe/entries_burler.go`
  - `internal/shedrecipe/entries_bouncer.go`
  - `internal/shedrecipe/fixture_test.go`
  - `internal/shedrecipe/registry.go`
  - `internal/burlerengine/profile.go`
  - `internal/shedadapters/burler.go`
- **Edits:** none
- **Creates:**
  - `internal/shedrecipe/entries_burler_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Write tests in the `shedrecipe` package, all geometry from `newTestEnv(t)`.

  Happy path: a minimal valid `Config` — a `run_subdir` plus a `profile` map — constructs a non-nil `shedengine.ShedProducer` with no error.

  Profile-mapping subtests: the six recipe-authorable `profile` keys land in the right `burlerengine.Profile` fields, and `model`, `effort`, and `timeout_s` land in `burlerengine.RunOpts`.
  The constructed `burlerengine.Profile` is captured inside `shedadapters.BurlerProducer` and unreachable from outside, so assert it through the `fakeBurlerRunner` from `fixture_test.go`, which records the `Profile` and `RunOpts` it is handed;
  drive the producer's `Call` to get there, and read `internal/shedadapters/burler.go`'s `Call` first to see what the run directory must contain for the call to reach the runner.
  Assert `timeout_s: 30` becomes a `RunOpts.Timeout` of `30 * time.Second`, and that a fractional `timeout_s` is rejected at construction naming the key.

  The relative-path exception needs its own subtest and is the assertion that pins the decision: a `profile.target.paths` entry and a `profile.fasit.paths` entry both reach `burlerengine.Profile` **still relative and unmodified**, and an absolute value in either is **not** rejected by the entry.

  Strict-unknown-key subtests at all three levels: an unrecognised outer `Config` key is rejected;
  an unrecognised `profile` key is rejected;
  each of `review-path`, `fixer-report-path`, `prior-reviews`, `prior-fixer-reports`, and `cluster-exclude` in `profile` is rejected as unknown rather than silently discarded;
  an unrecognised key inside `profile.target` and inside `profile.fasit` is rejected.

  Run-directory subtests: the joined directory exists on disk immediately after construction;
  a `BurlerRound` and a `Bouncer` built with the *same* `run_subdir` against the same `Env` resolve to the same directory, which is the regression guard for the cross-segment overwrite a flat run directory would have caused;
  an omitted `run_subdir`, an empty-string `run_subdir`, an absolute `run_subdir`, and a `..`-escaping `run_subdir` each fail at construction.

  Remaining construction failures: a missing `profile` key fails naming the key;
  blanking `Env.RunRoot` fails naming the field;
  nilling `Env.Burler` fails naming the field;
  a nil `Env.Now` constructs successfully.
- **Commit:** `test(shedrecipe): cover the BurlerRound entry and its profile mapping`

## Batch Tests

`verify: go test ./internal/shedrecipe/...` is correctly scoped: this batch adds two production files and two map keys inside the package and touches nothing outside it.
The two new test files carry the batch's two load-bearing assertions.
The first is the run-directory group: same `run_subdir` in a `Bouncer` and its `BurlerRound` resolves to one directory, different values resolve to different ones, and the directory exists before any `Call` — without the last, every fresh segment would fail on its first call, and no other test in the repo would catch it.
The second is the pinned report name: a drift there makes `shedadapters.ResolveRound` return 0 forever and the `Bouncer` re-seed silently, with no error anywhere to observe.
