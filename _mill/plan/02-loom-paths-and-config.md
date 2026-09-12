# Batch: loom-paths-and-config

```yaml
task: 'self-report Tier 2: per-agent friction notes for unsupervised runs'
batch: 'loom-paths-and-config'
number: 2
cards: 3
verify: go test ./internal/loomengine/... ./cmd/lyx/... ./internal/configreg/...
depends-on: []
```

## Batch Scope

This batch delivers the two things every later batch resolves its values from: `loomengine`'s two new path accessors (`LoomFrictionDir` and `LoomFrictionArchivePrefix`) and `loom.yaml`'s two new keys (`friction` and `friction_timeout_min`), with the `Config` fields and load-time validation that go with them.
It touches no prompt composer and no CLI, so it is independent of batch 1 and can run in parallel with it.

The external interface batches 4 and 7 consume is `loomengine.LoomFrictionDir(l)`, `loomengine.LoomFrictionArchivePrefix(l)`, `Config.Friction`, and `Config.FrictionTimeoutMin`.

Batch-local decision: `LoomFrictionArchivePrefix` is added here rather than in batch 3, even though only `internal/frictionengine`'s caller uses it, because the Cwd Resolution Invariant makes `loomengine` the sole declarer of every path under loom's own subdirectory and both accessors belong in the same file beside `LoomReviewsDir`.

## Cards

### Card 6: `LoomFrictionDir` and `LoomFrictionArchivePrefix`

- **Context:**
  - `internal/lyxdirs/dirs.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/loomengine/config.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add a `frictionDirName = "friction"` constant to `internal/loomengine/config.go`, declared beside the existing `reviewsDirName` constant and documented the same way: loomengine is its sole declarer.

  Add `LoomFrictionDir(l *lyxcwd.Location) string`, returning `filepath.Join(LoomScratchDir(l), frictionDirName)`.
  It must be built **on `LoomScratchDir`**, exactly as `LoomReviewsDir` is (`internal/loomengine/config.go:158-160`) and never by re-joining `l.AnchorPath()`, `lyxdirs.DotLyxDirName`, and `loomDirName` a second time — the Lyxdirs Single-Declarer Invariant forbids naming the `.lyx` literal twice in production path construction, and `LoomScratchDir` is already the accessor that names it once.
  Its doc comment states that friction notes are never-tracked ephemera consumed within the run and discarded, which is why they live under the ephemeral tree, and that per the Cwd Resolution Invariant no other package may construct this path.

  Add `LoomFrictionArchivePrefix(l *lyxcwd.Location) string`, returning `LoomFrictionDir(l) + "-"`.
  This is the absolute path prefix a timestamped archive sibling is composed from — `internal/frictionengine` appends its own compact timestamp to it and renames the friction directory onto the result.
  It exists as a second accessor rather than being derived by the caller so that `frictionengine` stays told rather than deriving, and so the `friction` segment is still named in exactly one place.
- **Commit:** `feat(loomengine): add LoomFrictionDir and LoomFrictionArchivePrefix accessors`

### Card 7: the `friction` and `friction_timeout_min` config keys

- **Context:**
  - `internal/configengine/config.go`
  - `internal/modelspec/modelspec.go`
  - `internal/loomengine/configtemplate.go`
- **Edits:**
  - `internal/loomengine/config.go`
  - `internal/loomengine/template.yaml`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add `Friction string` with yaml tag `friction` and `FrictionTimeoutMin int` with yaml tag `friction_timeout_min` to `loomengine.Config`, placed after the existing `Review`/`ReviewTimeoutMin` pair.

  Add both keys to `internal/loomengine/template.yaml` with a shipped default and the per-key inline comment every other key there carries, so the default is **on**: `friction: opus[effort=high]` and `friction_timeout_min: 30`.
  `ConfigTemplate()` embeds this file, so nothing in `internal/loomengine/configtemplate.go` changes.

  Extend `LoadConfig`'s validation, in the same shape it already uses for `discussion`/`plan`/`review`:

  - `cfg.Friction` is validated through `modelspec.Parse` **only when it is non-empty**.
    A present-but-empty value means Tier 2 is off and must load cleanly, unlike the three existing role keys, which are always required.
    A malformed non-empty value fails with `fmt.Errorf("loom config key %q: %w", "friction", err)`.
  - `friction_timeout_min` joins the existing negative-value loop's slice as a fourth entry, so a negative minute count fails at load time with the loop's existing message rather than hours into a run.
    Zero stays accepted for the same reason the other three accept it: `shuttleengine.Spec.Timeout` treats `0` as "defer to shuttle's own `run_timeout_min`".

  Update `internal/loomengine/config.go`'s file header comment, which today enumerates the three role model-specs and the three timeout knobs as "those six keys", to account for the two new ones.

  Do not reach for `configengine.LoadOrTemplate`: `loomengine` is on the strict `Load` side per the Config Strictness Invariant, so an already-seeded worktree missing the two new keys fails loudly with `configengine`'s own `missing keys: friction, friction_timeout_min; run "lyx config reconcile"` message, which already names its own remedy.
  That is the intended migration path and must not be softened.
- **Commit:** `feat(loomengine): add the friction and friction_timeout_min loom.yaml keys`

### Card 8: tests for the accessors, the keys, and the transient guard

- **Context:**
  - `internal/loomengine/review_test.go`
  - `internal/loomengine/config_test.go`
  - `internal/loomengine/config.go`
  - `internal/lyxdirs/dirs.go`
- **Edits:**
  - `internal/loomengine/config_test.go`
  - `internal/loomcli/wiring_test.go`
  - `cmd/lyx/notransients_test.go`
- **Creates:**
  - `internal/loomengine/friction_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create `internal/loomengine/friction_test.go` with untagged Tier 1 tests for the two new accessors, asserted the way `internal/loomengine/review_test.go` already asserts `LoomReviewsDir`:

  - `LoomFrictionDir(l)` equals `filepath.Join(LoomScratchDir(l), "friction")` and resolves under the anchor path's `.lyx/loom/` subtree.
  - `LoomFrictionArchivePrefix(l)` equals `LoomFrictionDir(l) + "-"`, asserted against `LoomFrictionDir`'s own return value rather than a second hand-built literal, so the two cannot drift.

  Extend `internal/loomengine/config_test.go`:

  - The existing hand-written `loom.yaml` fixtures in that file each gain the two new keys, since `configengine.Load` is strict and would otherwise fail every one of them on `missing keys`.
  - A **present-but-empty** `friction: ""` value loads cleanly and yields the zero value for `Config.Friction`, which is what "Tier 2 off" means.
  - A malformed non-empty `friction` model-spec fails at `LoadConfig` naming the `friction` key.
  - A negative `friction_timeout_min` fails at `LoadConfig` naming the `friction_timeout_min` key.
  - A `loom.yaml` genuinely **lacking** both keys fails `LoadConfig` with the `missing keys` error rather than defaulting.
    Assert that explicitly — it is the migration contract, and asserting the opposite would pin exactly the behaviour the Config Strictness Invariant forbids here.

  In `internal/loomcli/wiring_test.go`, add the two new keys to the hand-written `loom.yaml` fixture the `contents` `fmt.Sprintf` literal builds, so that package's tests keep loading a config the strict `configengine.Load` accepts.
  This one fixture line is updated here rather than in batch 7 so the repo stays green at this batch's own boundary;
  batch 7 edits the same file again, for its own wiring assertions, under the dependency edge that orders the two.

  In `cmd/lyx/notransients_test.go`, add `{"loomengine.LoomFrictionDir", loomengine.LoomFrictionDir(l)}` to `transientSet`, so the new accessor is covered by the machine guard that every never-tracked path resolves under `.lyx` and never under `_lyx`.
  Do not add it to `durableSet`.
- **Commit:** `test(loomengine): cover the friction accessors, the two new keys, and the transient guard`

## Batch Tests

`verify: go test ./internal/loomengine/... ./cmd/lyx/... ./internal/configreg/...` covers the three surfaces this batch changes.

`./internal/loomengine/...` runs the new `friction_test.go` (both accessors) and the extended `config_test.go` (the two new keys' parse, the present-but-empty off case, the two malformed-value failures, and the missing-keys migration contract), plus every existing loomengine test whose hand-written `loom.yaml` fixture this batch had to extend.

`./cmd/lyx/...` runs `TestNoTransientsUnderLyx`, which is the guard the new `LoomFrictionDir` row is added to.

`./internal/configreg/...` is included because `configreg.go:48` registers `loom` with `loomengine.ConfigTemplate`, so a template change that broke the module's registration shape would surface there rather than at first use.

The scope is per-batch, not whole-repo, with one deliberate widening: `internal/loomcli/wiring_test.go`'s hand-written `loom.yaml` fixture is edited here even though `./internal/loomcli/...` is not in this batch's `verify:` scope.
That fixture is the only other hand-written `loom.yaml` in the tree, and leaving it un-extended would make every `internal/loomcli` test fail on `configengine`'s `missing keys` error from this batch's boundary until batch 7 landed.
Extending it here keeps the repo green at every batch boundary;
the repo-wide confirmation of that is `pipeline.done_gate`'s job at task end, and the cheap cross-package compile gate is the overview's module-wide `verify: go build ./...`.
