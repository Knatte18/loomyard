# Batch: loomengine-config-and-paths

```yaml
task: 'loom: Discussion-Review producer'
batch: 'loomengine-config-and-paths'
number: 3
cards: 3
verify: go test ./internal/loomengine/...
depends-on: []
```

## Batch Scope

This batch gives loom the two config knobs the review segment costs money on, and the one path accessor its ephemeral round artifacts live under.
`loom.yaml` gains `review:` and `review_timeout_min:`, validated at load time exactly as `discussion:` and `plan:` already are;
`internal/loomengine` gains a `LoomReviewsDir` scratch accessor and a `ResolveReview` helper that turns the loaded config plus a model registry into the four values batch 4 threads onto `shedrecipe.Env`.
The external interface batch 4 consumes is `LoomReviewsDir`, `ResolveReview`, and the two new `Config` fields.
Nothing here reads or writes a recipe row, so this batch is independent of batches 1 and 2.

## Cards

### Card 9: add review and review_timeout_min to loom's config

- **Context:**
  - `internal/loomengine/configtemplate.go`
  - `internal/loomengine/discussion.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/loomengine/template.yaml`
  - `internal/loomengine/config.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add two keys to `internal/loomengine/template.yaml`, each with an inline comment matching the four existing keys' style: `review: opus[effort=high]` (model-spec for the review-segment producers) and `review_timeout_min: 240` (minutes one review round's shuttle run is allowed to run).
  Place them after the existing `plan_timeout_min` line so the file reads discussion, plan, review.
  Add the matching `Review string` and `ReviewTimeoutMin int` fields to the `Config` struct with `yaml:"review"` and `yaml:"review_timeout_min"` tags, in the same order.
  The Config Strictness Invariant makes an unknown key a load error, so the template and the struct must move together — neither half is optional.
  In `LoadConfig`, add a third `modelspec.Parse` validation for `cfg.Review`, immediately after the existing `cfg.Plan` one, wrapping its error as `loom config key "review": %w` in the same shape as the two beside it, so an ungrammatical spec fails loud at load rather than hours into a run when the Bouncer first spawns.
- **Commit:** `feat(loomengine): add review and review_timeout_min to loom.yaml`

### Card 10: add the reviews scratch accessor

- **Context:**
  - `internal/websterengine/state.go`
  - `internal/lyxdirs/dirs.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/loomengine/config.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add a private `reviewsDirName` constant declared alongside the existing `discussionDirName`, `loomDirName`, and `loomStatusFileName` constants, with the same "loomengine is this segment's sole declarer" doc comment shape, holding the literal `"reviews"`.
  Add a `LoomReviewsDir(l *lyxcwd.Location) string` accessor placed immediately after the existing `LoomScratchDir`, returning `filepath.Join(LoomScratchDir(l), reviewsDirName)`.
  Build it on `LoomScratchDir` rather than re-joining `l.AnchorPath()`, `lyxdirs.DotLyxDirName`, and `loomDirName` a second time: the Lyxdirs Single-Declarer Invariant forbids a hand-built join naming the `.lyx` literal in production path construction, and `LoomScratchDir` is already the accessor that names it once.
  Doc-comment it as the root every review segment's `run_subdir` resolves against — the value `shedrecipe.Env.RunRoot` takes — and state why it is ephemeral: there is no commit seam for a `Bouncer` row, so round reports, verdicts, ledgers, focus files, and their archive siblings would be untracked dirt if they lived under the durable tree.
  Per the Cwd Resolution Invariant, name that no other package may construct this path.
- **Commit:** `feat(loomengine): add the LoomReviewsDir scratch accessor`

### Card 11: add ResolveReview and its tests

- **Context:**
  - `internal/loomengine/config.go`
  - `internal/loomengine/discussion.go`
  - `internal/loomengine/plan.go`
  - `internal/loomengine/discussionpath_test.go`
  - `internal/loomengine/config_test.go`
  - `internal/modelspec/modelspec.go`
  - `internal/websterengine/config_test.go`
- **Edits:**
  - `internal/loomengine/config_test.go`
- **Creates:**
  - `internal/loomengine/review.go`
  - `internal/loomengine/review_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create `internal/loomengine/review.go` declaring a `ReviewSettings` struct with `Model`, `Effort`, `Version` (strings) and `Timeout` (`time.Duration`) fields, and a `ResolveReview(cfg Config, reg modelspec.Registry) (ReviewSettings, error)` function.
  `ResolveReview` parses `cfg.Review` via `modelspec.Parse`, resolves the result via `reg.Resolve`, and returns `Model` from the resolved spec's own model field, `Effort` from its `Params["effort"]`, `Version` from its `Params["version"]`, and `Timeout` as `time.Duration(cfg.ReviewTimeoutMin) * time.Minute`.
  Follow `DiscussionSpec`'s existing parse-then-resolve shape and its error wording exactly — same two-step, same `loom: ResolveReview: review role model-spec: %w` wrapping shape — so the three roles read identically.
  It returns the four values rather than a `shuttleengine.Spec` because there is no prompt to compose: the review segment's prompts are the Bouncer's own stencils, composed inside `internal/shedadapters` at call time, and the caller threads these four onto `shedrecipe.Env` instead.
  Create `internal/loomengine/review_test.go` with two tests: one asserting `ResolveReview` returns the expected triple and timeout for the embedded template's own values, and one asserting an ungrammatical `Review` spec returns an error naming the role.
  Add a third test in the same new file for `LoomReviewsDir`, following `discussionpath_test.go`'s shape: assert the returned path is anchored at `AnchorPath()`, sits under the ephemeral `.lyx` tree rather than the durable one, and mirrors the loom subdirectory that `LoomScratchDir` already names.
  Extend `internal/loomengine/config_test.go` for the two new keys: a valid `review:` spec loads, and an ungrammatical one fails at load with a message naming the key.
  Follow the file's own existing shapes — `TestLoadConfig_WellFormed` for the round-trip and `TestLoadConfig_MalformedPlanSpec` for the error, which is the exact analog of the new `review:` case.
  Add one further test to that file asserting the template and the `Config` struct agree on the full key set.
  This file has no such assertion today (its coverage is per-field literal round-trips only), so this is a new test, not an existing mechanism to reuse: copy the shape of `internal/websterengine/config_test.go`'s `TestConfigTemplate_ContainsEveryConfigYAMLTag`, which is the repo's precedent for a key-set agreement check.
  The check earns its place here specifically because the Config Strictness Invariant makes a struct field with no matching template key a silent hole rather than a load error.
- **Commit:** `feat(loomengine): add ResolveReview and cover the new config and path surface`

## Batch Tests

`verify: go test ./internal/loomengine/...` runs the whole `internal/loomengine` package.
That is the exact scope of this batch — all three cards touch only files in that one package — and the package's existing `config_test.go`, `discussionpath_test.go`, `loomstatus_test.go`, and `seed_test.go` are the regression surface for card 9's `Config` widening (which the Config Strictness Invariant makes an all-or-nothing change) and card 10's new path constant.
Card 11 adds the new coverage;
the rest of the package proves the widening broke nothing.
