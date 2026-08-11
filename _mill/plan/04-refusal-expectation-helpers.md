# Batch: refusal-expectation-helpers

```yaml
task: 'fabric: live-state integration harness (slice 13)'
batch: 'refusal-expectation-helpers'
number: 4
cards: 2
verify: go build ./... && go test -tags integration ./internal/fabricengine/fabrictest/ && go test ./internal/lyxcwd/ -run TestEnforcement
depends-on: [2]
```

## Batch Scope

This batch delivers the two refusal-expectation helpers and the three exported `Check` constants that let a cell assert **which layer refused** rather than merely that something refused.
It is one batch because the two helpers are defined against each other — `RefusedBefore`'s correctness depends on excluding exactly the string `RefusedByGate` matches on — and because the negative assertions that prove that relationship are the batch's real deliverable.

The external interface batches 6 and 7 consume is `Check`, `RefusedByGate` and `RefusedBefore`.

Batch-local decision: no production refusal type is exported.
`*destructiveRefusal` is unexported, so `errors.As` is unavailable from `fabrictest`, and exporting it now would be a production API change in a slice scoped as additive — one that slice 14's result-envelope rewrite would replace a slice later.

## Cards

### Card 12: `Check` constants and the two refusal-expectation helpers

- **Context:**
  - `internal/fabricengine/destroy.go`
  - `internal/fabricengine/remove.go`
  - `internal/fabricengine/checkout.go`
  - `internal/fabricengine/clone.go`
  - `internal/fabricengine/fabrictest/doc.go`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/fabrictest/refusal.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** `//go:build integration` on the first line.
  Define an exported string-backed `Check` type with **exactly three** constants — `CheckContainment` (`"containment"`), `CheckOwnership` (`"ownership"`), `CheckDirtiness` (`"dirtiness"`) — whose values match `destructiveCheck.String()`'s output at `destroy.go:43-56`.
  Add no `CheckForce`: `checkForce` is declared at `destroy.go:39` and rendered by `String()` at `destroy.go:51` but is never constructed into a `destructiveRefusal` anywhere in the tree, because force is consulted only inside `checkPathDirtiness`, where it makes the dirtiness check *pass* — so a refusal is never attributed to it and a `CheckForce` constant could never match.
  Record that in a comment beside the constant block so a future reader does not add it back believing the gate simply forgot to emit it.
  `RefusedByGate(err error, check Check) bool` reports whether `err` is non-nil and its `Error()` string contains `string(check) + " check failed"`.
  The gate renders `refusing to <what>: <check> check failed for <target>: <reason>` (`destroy.go:70-72`), so this substring is unambiguous and survives call-site wrapping because it searches the full error string.
  Assert-by-substring rather than `errors.As` because `*destructiveRefusal` is unexported and therefore unreachable from this package;
  the message *is* slice 12's honest-reporting contract, so pinning it tests something real rather than a proxy.
  `RefusedBefore(err error, substring string) bool` reports whether `err` is non-nil, its `Error()` string contains `substring`, **and** its `Error()` string does **not** contain the literal `"check failed"`.
  The exclusion is mandatory, not defensive: the gate's dirtiness reason at `destroy.go:564` is byte-identical to `Remove`'s own pre-flight message at `remove.go:74` — both are exactly `worktree has uncommitted changes; use --force` — so a gate refusal renders as `refusing to remove worktree: dirtiness check failed for <target>: worktree has uncommitted changes; use --force`, which *contains* the pre-flight string.
  Without the exclusion the helper reports a pre-flight refusal when the gate refused, and the layer-pinning property the whole two-kind scheme rests on is false in the pre-flight-to-gate direction.
  Document that in the function's doc comment, citing both line references.
  Both helpers return `false` for a nil error rather than panicking, so a cell that expected a refusal and got a success fails on the expectation rather than crashing the test binary.
- **Commit:** `fabrictest: add the Check constants and the two refusal-expectation helpers`

### Card 13: TDD suite for the refusal helpers

- **Context:**
  - `internal/fabricengine/fabrictest/refusal.go`
  - `internal/fabricengine/fabrictest/hub.go`
  - `internal/fabricengine/remove.go`
  - `internal/fabricengine/prune.go`
  - `internal/fabricengine/unwire.go`
  - `internal/fabricengine/destroy.go`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/fabrictest/refusal_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** `//go:build integration`, `package fabrictest`, every test `t.Parallel()` on its own hub.
  Assertions are made against **real errors produced by driving a verb**, never against hand-written error strings — a helper proved only against synthetic strings proves nothing about the message format it is pinning.
  **`RefusedByGate`, one case per reachable `Check`.**
  Drive a real gate refusal for each of `CheckContainment`, `CheckOwnership` and `CheckDirtiness` and assert the helper matches the right one.
  Reach containment through `fabricengine.UnwireJunctions` with a junction name escaping its worktree (`../x`) — `unwire.go:143-152` is the link executor's containment surface, and `TestUnwireJunctions_RefusesLinkOutsideItsWorktree` already proves it is a live refusal rather than a hypothetical one.
  Reach ownership through `Topology.Prune(apply=true)` against a hub child whose name ends in the weft suffix but which fabric never created.
  Reach dirtiness through the gate's own `dirtyScopeAll` request at `remove.go:196`/`:230`, planting the dirt so `Remove`'s earlier pre-flight does not claim it first — if that proves unreachable in practice, drive `Prune`'s gate request at `prune.go:269`/`:292` instead and say so in a comment.
  **Two negatives, both required.**
  A non-refusal error (an ordinary `fmt.Errorf`, or a real error from a verb failing for an unrelated reason) must match **no** `Check`;
  and a refusal by one check must not match either of the other two.
  **`RefusedBefore` against a real pre-flight refusal.**
  `Remove`'s own dirty message is the sharpest case and must be present: plant uncommitted changes in a pair's warp worktree, call `Remove` without force, and assert `RefusedBefore(err, "worktree has uncommitted changes; use --force")` is true.
  Then assert the discriminating half explicitly: construct or drive a **gate** dirtiness refusal whose message contains the same reason text, and assert `RefusedBefore` reports **false** for it while `RefusedByGate(err, CheckDirtiness)` reports true.
  That pair is the whole point of the `"check failed"` exclusion, and without it sabotage row 3 in batch 8 would stay green on its refusal half for the wrong reason.
  Also assert `RefusedBefore` matches `Remove`'s slug-validation refusal (`remove.go:45`, message shape `invalid slug ".."`) and `resetHub`'s pre-flight refusal text `is not a fabric hub` (`clone.go:573-577`).
- **Commit:** `fabrictest: prove the refusal helpers pin the layer that refused`

## Batch Tests

`verify:` runs card 13's suite via `go test -tags integration ./internal/fabricengine/fabrictest/`, which is the batch's substantive gate;
it re-runs batches 2 and 3's suites alongside, which is correct because the refusal cases drive real verbs against factory-built hubs.
`go build ./...` catches a compile break in the untagged default build.
`go test ./internal/lyxcwd/ -run TestEnforcement` covers vocabulary and geometry in `refusal.go`, which names warp and weft in its doc comments.
This batch adds no `_test.go` file outside the integration tag, so the tier-purity and hermetic-env guards cannot regress here and are deliberately not re-run.
