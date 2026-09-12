# Batch: leaf-surfaces

```yaml
task: 'self-report Tier 1: Go-detected structural anomalies'
batch: 'leaf-surfaces'
number: 1
cards: 4
verify: go test ./internal/selfreportengine/ ./internal/selfreportcli/ ./internal/shedadapters/ ./internal/loomengine/ && go test ./cmd/lyx/ -run 'TestNoTransientsUnderLyx|TestConstructorAnchoring'
depends-on: []
```

## Batch Scope

This batch delivers the four independent leaf surfaces the wiring batch consumes, none of which depends on any other: the engine-owned default label list, the `shedadapters`-owned exported ledger predicate and read accessor, the `loomengine`-owned filed-marker path accessors, and the `selfreport` config key.
Each is a self-contained change to an existing package with no new cross-package edge of its own — the new edges are created by batch 3, which is the only consumer of all four.

The external interface batch 3 consumes is exactly: `selfreportengine.DefaultLabels()`, `shedadapters.IsLedgerPath`/`shedadapters.ReadLedger`/`shedadapters.Ledger`/`shedadapters.LedgerEntry`, `loomengine.LoomSelfreportFiled`/`loomengine.LoomSelfreportFiledLock`, and `loomengine.Config`'s new `Selfreport` field.

Batch-local decision differing from the overview's Shared Decisions: none.

## Cards

### Card 1: move the `bug` label default into `selfreportengine`

- **Context:**
  - `internal/selfreportcli/cli_test.go`
- **Edits:**
  - `internal/selfreportengine/selfreport.go`
  - `internal/selfreportengine/selfreport_test.go`
  - `internal/selfreportcli/cli.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add an unexported `const defaultLabel = "bug"` and an exported `func DefaultLabels() []string` to `internal/selfreportengine/selfreport.go`, beside the existing `targetRepo` constant and `CreateIssue`.
  `DefaultLabels` must build and return a fresh `[]string{defaultLabel}` on every call rather than returning a shared package-level slice, so a caller that appends to or mutates the result cannot corrupt the default for the next caller.
  Give it a doc comment stating that it is the single owner of the automatic and manual filing paths' shared label default.
  In `internal/selfreportcli/cli.go`, replace `runCreate`'s `labels = []string{"bug"}` fallback with `labels = selfreportengine.DefaultLabels()`;
  change nothing else about `runCreate`, including its `Changed("body")` handling and its envelope shape.
  The `create` subcommand's `Long` help text already names the default `"bug"` label in three places — leave that text byte-identical, since the observable default is unchanged.
  Add a test to `internal/selfreportengine/selfreport_test.go` asserting `DefaultLabels()` equals exactly `[]string{"bug"}`, and a second asserting two successive calls return slices that are not the same backing array (mutating the first must not change the second).
  `internal/selfreportcli/cli_test.go` already covers the manual verb's omit-all-`--label` behaviour;
  it must keep passing unchanged — this card adds no test there and edits none of it.
- **Commit:** `refactor(selfreport): move the bug label default into selfreportengine`

### Card 2: export a ledger-path predicate and a ledger read accessor from `shedadapters`

- **Context:**
  - `internal/shedadapters/bouncerfiles.go`
  - `internal/shedadapters/round.go`
  - `internal/shedadapters/burler.go`
- **Edits:** none
- **Creates:**
  - `internal/shedadapters/ledgeraccess.go`
  - `internal/shedadapters/ledgeraccess_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/shedadapters/ledgeraccess.go` holding the package's two new exported surfaces plus the exported model they carry, and a file doc comment stating that this file is how another package consumes the ledger contract without re-declaring either the filename convention `ledgerPath` owns or the parse grammar `parseLedger` owns.

  Declare `type LedgerEntry struct { Key string; Rounds []int; Status string }` and `type Ledger struct { Round int; Entries []LedgerEntry }` as the exported mirrors of the unexported `ledgerEntry` and `ledgerFile`.
  `Ledger` deliberately carries no `Prose` field: no consumer added by this task reads it, and omitting it keeps the exported surface minimal.

  Declare `func IsLedgerPath(path string) bool`, reporting whether `path`'s base name is the ledger spelling `ledgerPath` formats.
  It must derive its answer from `ledgerPath` itself rather than from a re-typed `round-%d-bouncer-ledger.md` literal — parse the base name against the shape `ledgerPath` produces, and assert that derivation in the test below.
  It must accept a `ledgerPath(dir, n)` result for any positive `n` and reject everything else, including the empty string, a bare directory path, and the four same-directory `round-%d-` siblings `verdictPath`, `focusPath`, `roundReviewPath`, and `roundFixerReportPath` produce.
  Rejecting the two `burler.go` spellings is the predicate's sharpest job, since `BurlerProducer` publishes `roundReviewPath` into a history entry's output pointer on its `Stuck` returns.

  Declare `func ReadLedger(path string) (Ledger, error)`.
  It reads `path`, parses it via the existing `parseLedger`, derives the round from `path`'s own base name through the same helper `IsLedgerPath` uses, and — inheriting the fail-closed check `recordedVerdict` already applies — returns a non-nil error when the parsed frontmatter's `Round` disagrees with the round the filename encodes, rather than returning a `Ledger` claiming the frontmatter's value.
  Give that error message the same shape as `bouncerfiles.go`'s other parse errors, naming both the filename's round and the frontmatter's round.
  Deriving the round inside the accessor rather than requiring the caller to pass it is what keeps round extraction inside the package that owns the filename.
  `ReadLedger` returns a zero `Ledger` and a non-nil error on a read failure, on a parse failure, and on the round disagreement — never a half-filled model.

  Create `internal/shedadapters/ledgeraccess_test.go` as an untagged Tier-1 suite covering the exported surface only;
  `parseLedger`'s own grammar cases are already covered elsewhere in the package and must not be duplicated here.
  Assert the predicate against the real `ledgerPath`, `verdictPath`, `focusPath`, `roundReviewPath`, and `roundFixerReportPath` helpers rather than against hand-typed filenames, so a future filename change cannot pass the test while breaking discovery.
  Cover: accepts `ledgerPath` output for several rounds;
  rejects each of the four siblings for the same round;
  rejects a plain `decision-record.md`;
  rejects a directory-shaped path;
  rejects the empty string.
  For the accessor, use a `t.TempDir()` fixture: a well-formed ledger round-trips into the exported model with its entries intact;
  a malformed one reports an error and a zero model;
  an absent file reports an error without panicking;
  and a file written at round 4's path whose frontmatter says `round: 3` reports an error rather than returning a model claiming round 3.
- **Commit:** `feat(shedadapters): export a ledger-path predicate and read accessor`

### Card 3: add the filed-marker path accessors to `loomengine`

- **Context:**
  - `internal/lyxdirs/dirs.go`
- **Edits:**
  - `internal/loomengine/config.go`
  - `internal/loomengine/loomstatus_test.go`
  - `cmd/lyx/notransients_test.go`
  - `cmd/lyx/constructoranchoring_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add two exported accessors to `internal/loomengine/config.go`, placed beside the existing `LoomDriverLog` and `LoomBootstrapLock` and following their exact shape — `filepath.Join(l.AnchorPath(), lyxdirs.DotLyxDirName, loomDirName, "<file>")`, never a hand-built join naming the `.lyx` literal a second time:

  `func LoomSelfreportFiled(l *lyxcwd.Location) string` returning `.lyx/loom/selfreport-filed.json`, the machine-local marker recording which anomaly titles have already been filed.
  `func LoomSelfreportFiledLock(l *lyxcwd.Location) string` returning `.lyx/loom/selfreport-filed.json.lock`, its sibling advisory lock.

  Both need a doc comment in the style `LoomDriverLog`'s already uses, stating that the file is never-tracked and therefore lives under `lyxdirs.DotLyxDirName` per the Durable-vs-Ephemeral State Invariant, and that it exists as an accessor rather than an inline path because cmd/lyx's transient guard walks constructors, not call sites.
  `LoomSelfreportFiled`'s comment must additionally record that losing the marker — a fresh clone, a fabric re-wire — costs at most one duplicate issue, which is why the marker is deliberately machine-local rather than durable.
  Declare the two filenames as unexported package constants beside the existing `loomStatusFileName`, following its own sole-declarer comment style, rather than as inline string literals.

  Register both accessors in the two guard tests in the same card, or the guard will not cover them.
  In `cmd/lyx/notransients_test.go`, give each accessor its own row in `transientSet` — two rows, not one — beside the existing `loomengine.LoomBootstrapLock` entry, since the guard walks that table element by element and an unlisted accessor is simply never checked.
  In `cmd/lyx/constructoranchoring_test.go`, add the matching `assertPath` lines and map entries for both accessors to all three sites that currently list `loomengine.LoomBootstrapLock`.
  Update `constructoranchoring_test.go`'s file-level comment where it enumerates the loom constructors by name so the list stays accurate.

  Add path assertions to `internal/loomengine/loomstatus_test.go` mirroring the existing `TestLoomRunLock` and `TestLoomRunLock_UnanchoredEqualsWorktreePath` pair: one hand-built `lyxcwd.Location` with a non-`"."` `AnchorRel` proving the accessor follows the anchored subpath, and one unanchored case proving it equals the worktree path.
- **Commit:** `feat(loomengine): add the selfreport filed-marker path accessors`

### Card 4: add the `selfreport` key to loom's config and template

- **Context:**
  - `internal/configengine/config.go`
  - `internal/yamlengine/reconcile.go`
- **Edits:**
  - `internal/loomengine/config.go`
  - `internal/loomengine/template.yaml`
  - `internal/loomengine/config_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add a `Selfreport bool` field with the yaml tag `selfreport` to `loomengine.Config` in `internal/loomengine/config.go`, and a matching `selfreport: true` line to `internal/loomengine/template.yaml` with a trailing comment in the existing style, stating that it switches off automatic filing of Go-detected structural anomalies and that a run against a fork or in CI must set it false so it does not file into the upstream issue tracker.

  The template line is what makes the default true, because `bool`'s zero value is `false` — exactly the trap `discussion_interactive: false` already sits beside.
  But be precise about how strict loading reaches that default, because the obvious assumption is wrong: `configengine.Load` does **not** fall back to the template for a key an existing on-disk file omits.
  Its shared `load` body runs `yamlengine.MissingKeys(template, fileBytes)` and hard-errors with `missing keys: …; run "lyx config reconcile"` whenever that returns anything, before it ever resolves a value.
  The template supplies the default only where the whole file is absent, and that path belongs to `LoadOrTemplate`, which loom deliberately does not use — loom is in the strict set per the Config Strictness Invariant.

  The migration consequence is real and must be stated in the template comment rather than discovered at run time: adding this key makes every already-reconciled `loom.yaml` on disk miss it, so loom's config load hard-errors until the operator runs `lyx config reconcile --apply`, which writes the new key in at the template's value.
  Both edits must still land together — a `Config` field with no template entry is the mirror-image break — but landing them together does not spare an existing worktree the reconcile step.
  Add no validation for this key in `LoadConfig`: unlike the three model-specs and the three timeouts, a bool has no value that can only be a mistake.

  **Repair the four existing fixtures this key breaks, in this same card.** `TestLoadConfig_DiscussionInteractiveTrue`, `TestLoadConfig_MalformedDiscussionSpec`, `TestLoadConfig_MalformedPlanSpec`, and `TestLoadConfig_MalformedReviewSpec` each hand-type a full seven-key YAML block and pass it to `seedLoomConfig`.
  Adding `selfreport` to the template makes all four miss a key, so `MissingKeys` errors before `LoadConfig` ever reaches `yaml.Unmarshal` or `modelspec.Parse` — the interactive test would get a non-nil error where it asserts nil, and the three malformed-spec tests would get a missing-keys error instead of the spec error whose text they match on.
  That breaks `go test ./internal/loomengine/`, which is this batch's own `verify:`, so the repair is not optional and is not a follow-up.
  Switch all four to the package's existing `writeLoomConfigWithKey` helper — one call each, overriding the single key that test actually varies (`discussion_interactive` with `true`, and `discussion`/`plan`/`review` each with the quoted malformed spec literal that test already uses) — rather than hand-adding a `selfreport:` line to each block.
  That helper builds off `ConfigTemplate()` and its own doc comment states it exists so a fixture never drifts from the template's key set, which is exactly the drift that caused this;
  switching makes the four immune to the next template key as well, where hand-adding a line would leave the same trap armed.

  Add two cases to `internal/loomengine/config_test.go` for the new key itself.
  First, a `loom.yaml` seeded from the full `ConfigTemplate()` loads with `Selfreport` true — that, not an omitted key, is what exercises the template's default under strict loading;
  the package's existing well-formed-config test is the shape to follow.
  Second, `writeLoomConfigWithKey` with `selfreport` set to `false` loads with `Selfreport` false.
  Do not write a case asserting that a `loom.yaml` omitting `selfreport` entirely loads with `Selfreport` true;
  that file errors on missing keys, and asserting otherwise would pin behaviour the loader does not have.
  A third case asserting that an omitting file produces the missing-keys error is optional, not required — `configengine`'s own suite already owns that behaviour.
- **Commit:** `feat(loomengine): add the selfreport config key and its template default`

## Batch Tests

`verify:` runs the four packages this batch changes plus the two `cmd/lyx` guard tests card 3 registers into, scoped with `-run` so the package's own cross-compile build test is not dragged into every implementer and fixer round.

Files covered: `internal/selfreportengine/selfreport_test.go` (card 1's `DefaultLabels` cases), `internal/selfreportcli/cli_test.go` (card 1's unchanged-behaviour regression), `internal/shedadapters/ledgeraccess_test.go` (card 2's predicate and accessor cases, plus the package's existing `parseLedger` suite, which must keep passing), `internal/loomengine/loomstatus_test.go` and `internal/loomengine/config_test.go` (cards 3 and 4), and `cmd/lyx/notransients_test.go` plus `cmd/lyx/constructoranchoring_test.go` (card 3's guard registration).

No test here spawns a process, clones a repo, or builds a hub fixture — the only I/O is card 2's `t.TempDir()` ledger fixtures and card 4's `seedLoomConfig` temp-dir config file, both Tier 1.
