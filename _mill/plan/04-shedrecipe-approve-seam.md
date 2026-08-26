# Batch: shedrecipe-approve-seam

```yaml
task: 'loom: Plan-Write/Plan-Validate approval deadlock (F7)'
batch: 'shedrecipe-approve-seam'
number: 4
cards: 3
verify: go test ./internal/shedrecipe/...
depends-on: [2]
```

## Batch Scope

This batch makes batch 2's `Approve` seam recipe-selectable: `Env` gains an `ApprovePlan func() error` field, and the `Bouncer` registry entry gains an `approve_seam` config key resolving to it.
It depends on batch 2 alone — `shedadapters.BouncerConfig` must already carry the field this entry assigns — and is independent of batch 3, which touches a different entry file in the same package.

The external interface batch 5 consumes is `shedrecipe.Env.ApprovePlan`, which `internal/loomcli`'s `wire()` fills.
The external interface batch 6 consumes is the `approve_seam` config key itself, which the shipped recipe's `Plan-Bouncer` row then sets.
Nothing in this batch changes the shipped recipe, so the tree keeps building and every existing `Bouncer` row keeps constructing with a nil `Approve`.

Batch-local decision: `approve_seam` accepts the single value `"plan"` rather than mirroring `commit_seam`'s two-value set.
There is no discussion-side or webster-side approval flag, so a second accepted value would be a hypothetical;
an unknown value is a construction error whose message names the one value that is accepted.

## Cards

### Card 14: Add Env.ApprovePlan

- **Context:**
  - `internal/shedrecipe/entries_bouncer.go`
- **Edits:**
  - `internal/shedrecipe/recipe.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add an `ApprovePlan func() error` field to `Env`, placed immediately after the existing `CommitPlan` field so the plan-side seams stay together.
  Match the surrounding field-doc convention, which names which producer reads each closure: state that `ApprovePlan` is the injected closure marking the reviewed plan approved, that the `Bouncer` entry is what reads it, and that it is invoked on the approved branch of that producer's settle before the commit seam.
  Add no import — the field is a plain `func() error` built entirely by the caller — so this package's import allowlist in `seam_enforcement_test.go` needs no change.
- **Commit:** `14: shedrecipe: add Env.ApprovePlan`

### Card 15: Resolve the approve_seam config key in the Bouncer entry

- **Context:**
  - `internal/shedrecipe/recipe.go`
  - `internal/shedrecipe/env.go`
  - `internal/shedadapters/bouncer.go`
- **Edits:**
  - `internal/shedrecipe/entries_bouncer.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Read a new optional string config key named `approve_seam` with the package's existing `configString(cfg, key, required)` helper and `required` false, beside the existing `commit_seam` read.
  Add `"approve_seam"` to this entry's `configRejectUnknown` allowlist call, which currently ends at `"commit_seam"`.
  Resolve the key with a `switch` mirroring the shipped `commit_seam` switch one-for-one: the empty string leaves the resolved closure nil, which means "approve nothing" and keeps every existing `Bouncer` row valid unchanged;
  `"plan"` passes through the existing `requireSeam` guard against `env.ApprovePlan` before assigning it, so a nil `Env` closure is a construction error rather than a silent reproduction of the no-seam condition the key exists to eliminate;
  and any other value is an error naming the single accepted value.
  Assign the resolved closure to the `Approve` field of the `shedadapters.BouncerConfig` this entry builds, placed beside the existing `Commit` assignment.
  Extend the entry's own doc comment so it names the new key alongside `commit_seam` and states that `approve_seam` resolves to `env.ApprovePlan` and nothing else.
- **Commit:** `15: shedrecipe: resolve approve_seam on the Bouncer row`

### Card 16: Cover the approve_seam key

- **Context:**
  - `internal/shedrecipe/entries_bouncer.go`
  - `internal/shedrecipe/recipe.go`
- **Edits:**
  - `internal/shedrecipe/entries_bouncer_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add cases following the shape this file's existing `commit_seam` cases already use: `approve_seam: plan` with a non-nil `Env.ApprovePlan` builds successfully;
  `approve_seam: plan` with a nil `Env.ApprovePlan` is a construction error whose message names the `Env` field;
  an unknown value such as `approve_seam: discussion` is a construction error whose message names `"plan"` as the accepted value;
  the key absent builds successfully with the seam left nil;
  and a hyphenated `approve-seam` typo is still rejected as an unrecognised key, proving the allowlist widened by exactly one name.
  Where a case needs to observe which closure was resolved, follow whatever the file's existing `commit_seam` cases already do to observe `Commit` rather than inventing a second mechanism.
- **Commit:** `16: shedrecipe: cover the approve_seam key`

## Batch Tests

`verify: go test ./internal/shedrecipe/...` runs the whole package suite, which is the batch's exact edit surface.
The package-wide scope is right rather than narrower: card 14 edits `recipe.go`, whose `Env` struct every entry test in the package constructs, and card 15 edits the `Bouncer` entry, whose existing `commit_seam` and seam-validation cases are the regression surface for the absent-key default path.
`seam_enforcement_test.go`, which pins this package's import allowlist, runs in the same command and is what proves card 14 added no import.
