# Batch: loom-driver-config

```yaml
task: 'Seeded driver choice: ly-drive strand as the child''s driver'
batch: loom-driver-config
number: 2
cards: 2
verify: go build ./... && go test ./internal/loomengine/...
depends-on: []
```

## Batch Scope

This batch gives the driver the same "which model runs this role" declaration every other agent-spawning role in the stack already has: one new `driver` key on `internal/loomengine`'s `Config`, validated at load time, plus the resolver that turns it into the provider model id, effort and version triple a `shuttleengine.Spec` carries.
It is one batch because the key and its resolver are one decision split across two files by the package's existing layout — `friction` and `review` are shaped exactly this way — and because a key with no resolver is an unread config key, which the discussion rejects by name.

The external interface batch 4 consumes is `loomengine.DriverSettings` and `loomengine.ResolveDriver(cfg, reg)`.

Batch-local decision beyond the overview's: **no `driver_timeout_min` key**, and no negative-timeout guard beside the new key.
`shuttleengine.Spec.Timeout` feeds `Run.deadline`, which only `Wait` reads, and the driver path deliberately never calls `Wait` — so a timeout key would configure nothing and a test asserting it would pass against a dead field.
`DriverSettings` therefore carries three fields, not the four `ReviewSettings` carries: the `Timeout` member has no analogue here and must not be added for symmetry.

## Cards

### Card 3: the driver config key

- **Context:**
  - `internal/modelspec/modelspec.go`
  - `internal/configreg/configreg.go`
- **Edits:**
  - `internal/loomengine/config.go`
  - `internal/loomengine/config_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add `Driver string ` with the yaml tag `driver` to `loomengine.Config` in `internal/loomengine/config.go`, placed beside the existing `Friction` field.
  Validate it in `LoadConfig` with `modelspec.Parse` **only when non-empty**, following the `Friction` arm line for line, including the error shape `fmt.Errorf("loom config key %q: %w", "driver", err)`.
  An empty or absent value is legal and means "defer to the engine default" — the same meaning `shuttleengine.Spec.Model`'s empty value already carries — so it must not be defaulted to a literal model alias here.
  Add the key to `ConfigTemplate` with a comment saying it names the model that runs an `llm`-driven run's ly-drive session, and that an empty value defers to the provider default.
  `ConfigTemplate` is registered in `internal/configreg`'s module list as `loom`, so existing worktrees pick the key up through `configengine.Load`'s template merge with no migration step.
  Amend the file's own package doc comment, whose opening sentence enumerates the role model-specs this file validates as "discussion, plan, review, and friction" — add driver to that list and to the "validated only when non-empty" sentence that today names friction alone.
  Do **not** add a `driver_timeout_min` key and do not copy the negative-timeout guards that sit beside the other role keys.
  In `config_test.go` cover: a valid `driver` spec loading cleanly; a malformed one failing at load time with the error naming the key `driver`; an absent key loading cleanly with `Driver` empty; a present-but-empty key loading cleanly rather than erroring; and the key being present in `ConfigTemplate`.
- **Commit:** `feat(loomengine): add the driver role model-spec config key`

### Card 4: ResolveDriver

- **Context:**
  - `internal/loomengine/review.go`
  - `internal/modelspec/modelspec.go`
  - `internal/loomengine/config.go`
- **Edits:** none
- **Creates:**
  - `internal/loomengine/driver.go`
  - `internal/loomengine/driver_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/loomengine/driver.go` declaring `DriverSettings` and `func ResolveDriver(cfg Config, reg modelspec.Registry) (DriverSettings, error)`, modelled on `ResolveReview` in `internal/loomengine/review.go`.
  `DriverSettings` carries exactly three fields — `Model string`, `Effort string`, `Version string` — and no `Timeout`, per this batch's scope note.
  `ResolveDriver` calls `modelspec.Parse(cfg.Driver)` then `reg.Resolve(spec)`, wrapping each error as `fmt.Errorf("loom: ResolveDriver: driver role model-spec: %w", err)`, and fills the triple from the **resolved** value exactly as `ResolveReview` does: `Model: resolved.Model`, `Effort: resolved.Params["effort"]`, `Version: resolved.Params["version"]`.
  Resolving through the registry rather than copying `cfg.Driver` is the whole point of the function: `modelspec.Parse` checks grammar only, and `reg.Resolve` is what turns an alias into a provider model id and lifts its bracket params out.
  An empty `cfg.Driver` must resolve to an empty `DriverSettings` with a nil error rather than failing — the "off/default" arm `Friction` already has — so the empty case is handled before `Parse` is reached.
  In `driver_test.go` cover: an alias resolving to its provider model id with `effort` and `version` lifted out of `resolved.Params`; an unknown alias erroring with the key named; a malformed spec erroring; and an empty `cfg.Driver` returning a zero `DriverSettings` and a nil error.
  The alias-resolution case is the load-bearing one — a test asserting only that the returned `Model` is non-empty would pass against a raw copy of the config string, which is the exact bug this function exists to prevent.
- **Commit:** `feat(loomengine): resolve the driver role's model spec`

## Batch Tests

`verify: go build ./... && go test ./internal/loomengine/...` runs the package's untagged suite plus a whole-module build.
The module build is in scope because `Config` is loaded by `internal/loomcli`'s wiring and by `internal/configreg`'s template registration, neither of which this verify's test list names.

The two load-bearing assertions are the empty-value cases and the alias-resolution case.
An empty `driver` must be legal at both layers — `LoadConfig` must not reject it and `ResolveDriver` must not error on it — because every existing worktree picks the key up through the template merge with no value set, so a strict arm at either layer would break every loom worktree in the hub on the first `lyx loom start` after this lands, on a key nothing had set yet.
The alias-resolution case is what distinguishes a real `reg.Resolve` call from a raw copy, which is the difference between an operator's `driver: opus[high]` reaching the provider as a model id plus `high` and reaching it as the literal string `opus[high]`.
