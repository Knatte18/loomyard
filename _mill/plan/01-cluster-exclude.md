# Batch: burlerengine ClusterExclude

```yaml
task: 'shedadapters: Burler-round producer'
batch: 'burlerengine ClusterExclude'
number: 1
cards: 3
verify: go test ./internal/burlerengine/
depends-on: []
```

## Batch Scope

This batch adds the one new exported knob `internal/burlerengine` needs so a caller can trim a resolved cluster fan per call: `Profile.ClusterExclude []string`, applied inside `Profile.validate` after `ResolveFan` resolves the fan, storing the survivors in the existing unexported `clusterLenses`.
It is one batch because the field, its filtering helper, its table-driven tests, and its package-doc paragraph are a single self-contained change to one package, and nothing outside `internal/burlerengine` is touched.
The external interface batch 3 consumes is exactly the new `ClusterExclude` field: `BurlerProducer` sets it per round from the focus file's `exclude_lenses` list.
Batch-local decision differing from `## Shared Decisions`: this package's Go comments use em dashes, matching `profile.go`/`doc.go`'s existing style rather than `internal/shedadapters`' `--` convention.

## Cards

### Card 1: Add `Profile.ClusterExclude` and apply it in `validate`

- **Context:**
  - `internal/burlerengine/config.go`
  - `internal/burlerengine/cluster.go`
  - `internal/burlerengine/engine.go`
- **Edits:**
  - `internal/burlerengine/profile.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add an exported field `ClusterExclude []string` to the `Profile` struct in `internal/burlerengine/profile.go`, declared immediately after the existing `ClusterFan string` field and before the unexported `clusterLenses []Lens` field, with a godoc comment stating it names lenses to drop from the resolved `ClusterFan` and that it is a per-call advisory filter, not config.
  Replace the existing `ClusterFan` block inside `(*Profile).validate` — the `if p.ClusterFan != ""` block that calls `ResolveFan(cfg, p.ClusterFan)` and assigns `p.clusterLenses = lenses` — with a two-branch form.
  When `p.ClusterFan == ""` and `len(p.ClusterExclude) > 0`, return a fail-loud error stating that `profile.ClusterExclude` is set but `profile.ClusterFan` is empty, so there is no fan to trim.
  When `p.ClusterFan != ""`, call `ResolveFan(cfg, p.ClusterFan)` exactly as today, return its error unchanged, then assign `p.clusterLenses = applyClusterExclude(p.ClusterFan, lenses, p.ClusterExclude)`.
  Add a new unexported helper `applyClusterExclude(fan string, lenses []Lens, exclude []string) []Lens` in the same file, placed after `validate` and before `resolvePath`, with these rules: an empty or nil `exclude` returns `lenses` unchanged; otherwise build a set of the excluded names, walk `lenses` in order keeping every lens whose `Name` is not in the set, and preserve fan order among the survivors; every entry of `exclude` that matches no lens `Name` in `lenses` emits a `logger.Warn` naming the fan and the unmatched name and is otherwise a no-op; if the surviving slice is empty, emit a `logger.Warn` naming the fan and stating the exclusion was dropped whole to keep the fan intact, and return `lenses` unchanged.
  A duplicate name in `exclude` must be harmless — the set collapses it.
  Add the `github.com/Knatte18/loomyard/internal/logger` import to `internal/burlerengine/profile.go`; the package already depends on it from `internal/burlerengine/engine.go`.
  Do not change `ResolveFan` in `internal/burlerengine/config.go`, which stays fail-loud, and do not change `auditClusterRound` in `internal/burlerengine/cluster.go`, whose exact-N contract is satisfied by the post-filter `len(p.clusterLenses)`.
- **Commit:** `feat(burlerengine): add Profile.ClusterExclude fan trimming`

### Card 2: Table-drive `ClusterExclude` in `profile_test.go`

- **Context:**
  - `internal/burlerengine/profile.go`
  - `internal/burlerengine/config.go`
  - `internal/burlerengine/testmain_test.go`
- **Edits:**
  - `internal/burlerengine/profile_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add a new table-driven test function `TestProfileValidate_ClusterExclude` to `internal/burlerengine/profile_test.go`, reusing the file's existing `testClusterFanConfig` and `newValidProfileFixture` helpers rather than adding new fixtures.
  Each case sets `ClusterFan` and `ClusterExclude` on a copy of the fixture profile, calls `validate`, and asserts both the returned error and the resulting `clusterLenses` names in order.
  Cover: no exclusion behaves exactly as today (both lenses of the `standard` fan survive, in fan order); an exclusion naming one lens drops exactly that lens and preserves the order of the rest; an exclusion naming a lens absent from the resolved fan is a no-op for that name and not an error, leaving the fan intact; a duplicate name in the exclusion list is harmless and yields the same survivors as the single-name case; an exclusion covering every lens in the fan leaves `clusterLenses` equal to the full fan rather than emptying it or returning an error; and `ClusterExclude` set with an empty `ClusterFan` returns an error whose text names `ClusterExclude`.
  For that error case, also assert the same two prefix invariants the existing `TestProfile_Validate` table already guards: the message is `burler: `-prefixed, and it carries exactly one `burler: ` occurrence — the regression guard that exists because a wrapped `ResolveFan` error once double-prefixed.
  Assert on lens `Name` values only, never on `Text`.
  Do not add a `TestMain` to this package — `internal/burlerengine/testmain_test.go` already provides one — and do not spawn git, `exec.Command`, or any fixture helper from the new test.
- **Commit:** `test(burlerengine): cover Profile.ClusterExclude in validate`

### Card 3: Document `ClusterExclude` in the package doc

- **Context:**
  - `internal/burlerengine/profile.go`
  - `internal/burlerengine/config.go`
- **Edits:**
  - `internal/burlerengine/doc.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Extend the `# Cluster fan-out (fork subagents)` section of `internal/burlerengine/doc.go` with a paragraph documenting `Profile.ClusterExclude`, placed after the existing paragraph that ends with the sentence about there being deliberately no fan named "default" and before the paragraph beginning "A cluster round still runs as ONE shuttle session".
  The paragraph must state: `ClusterExclude` names lenses to drop from the fan `ClusterFan` resolves to, applied inside `validate` after `ResolveFan`, with the survivors stored in `clusterLenses` — the single value both prompt composition and `auditClusterRound`'s exact-N fork check read, so a trimmed round demands exactly the forks it named and `ErrClusterForksMissing`'s fail-loud posture is untouched.
  It must also state the three edge cases and why they split on who authored the input: `ClusterExclude` set with an empty `ClusterFan` is a `validate` error because that is a Go caller's mistake; a name not present in the resolved fan is a no-op for that name with a warning, because an exclusion list is an advisory, per-call directive over a config-owned fan an operator may edit between rounds, so a stale name is stale rather than wrong; and an exclusion that would empty the fan drops the whole exclusion and keeps the fan intact, because dropping to zero lenses is never what "these found nothing last round" meant and re-running the full fan costs tokens, never correctness.
  Keep the section's existing em-dash comment style.
- **Commit:** `docs(burlerengine): document Profile.ClusterExclude in the package doc`

## Batch Tests

`verify: go test ./internal/burlerengine/` runs the whole `internal/burlerengine` package suite — `profile_test.go` (which card 2 extends), plus `config_test.go`, `cluster_test.go`, `engine_test.go`, `prompt_test.go`, `template_test.go`, and `verdict_test.go`.
Whole-package scope is correct rather than over-broad here: card 1 changes `Profile.validate`, which every one of those files' fixtures runs through `Engine.Run` or calls directly, so a regression would surface outside `profile_test.go`.
The opt-in smoke files (`smoke_round_test.go`, `smoke_cluster_test.go`) carry build tags and do not run under this command.
No new smoke test is added: this batch's change is pure, deterministic Go with no LLM interaction.
