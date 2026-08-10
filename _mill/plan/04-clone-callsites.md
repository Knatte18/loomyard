# Batch: clone-callsites

```yaml
task: 'fabric: one ownership-and-dirtiness gate for all destruction (slice 12)'
batch: 'clone-callsites'
number: 4
cards: 3
verify: go test ./internal/fabricengine/... && go test -tags integration ./internal/fabricengine/...
depends-on: [2]
```

## Batch Scope

This batch converts the two hub-level destructive sites in `internal/fabricengine/clone.go` — `resetHub` and `teardownHub` — and moves the hub directory's creation onto the gate's exclusive-create minter so `teardownHub` has a transaction identity the gate can verify rather than believe.
It is one batch and one file because the three changes are inseparable: the token `teardownHub` needs does not exist until the creation moves, and the creation cannot move without threading the token through thirteen call sites in the same function.

It runs in parallel with batch 3, which touches no file in this batch.

This is where the second of the three gaps the slice closes lands: `teardownHub` today has no containment check and no ownership check, and fires unconditionally on any clone-or-worktree-add failure from thirteen call sites.

Batch-local decisions beyond `## Shared Decisions`:

- Neither hub-level site has a `*lyxcwd.Location` available.
  `CloneHub` takes only `cwd string`, `resetHub` runs before any resolution, `teardownHub`'s earliest calls are immediately after the warp clone attempt, and the real resolution happens hundreds of lines later.
  This is why the ownership kinds carry their own inputs: both kinds these sites declare need no repo context, so nothing is nil and no synthetic Location is invented.
  Do not construct one, and do not pass the partial `&lyxcwd.Location{HubPath: hubPath, WorktreeName: name}` that already exists in the function for the hook install — it is a hook-install convenience, not geometry the gate may consume.
- `--reset` and `teardownHub` deliberately declare different ownership kinds, and that difference is the point rather than an inconsistency.
  `--reset` acts on a directory that pre-existed the invocation, so `looksLikeHub` is the only rule available to it.
  `teardownHub` acts on a directory this invocation created, so it can assert the strictly stronger "did this invocation create this exact path".

## Cards

### Card 20: mint the hub directory exclusively

- **Context:**
  - `internal/fabricengine/destroy.go`
  - `internal/fabricengine/junctionnames.go`
- **Edits:**
  - `internal/fabricengine/clone.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** in `CloneHub`, replace the `os.MkdirAll(hubPath, 0o755)` that creates the hub directory with a `createExclusiveDir(hubPath)` call, binding the returned token to a local the rest of the function can reach.
  Change nothing else about the creation's placement.
  Both offline "hub already exists at %s" stat guards stay exactly where they are, one in each of `CloneHub`'s two argument forms, and both keep their current messages verbatim.
  This placement is not incidental and both alternatives change behaviour: creating at either stat guard would leak a residual hub whenever the weft-binding probe then fails, because that path returns without teardown;
  folding the stat guards into the creation would defer the offline "hub already exists" refusal until after a network call, breaking the offline-before-network ordering the surrounding comments document as deliberate.
  Leave the immediately following `os.MkdirAll` that creates the hub-level dot-lyx directory as an ordinary `MkdirAll` — it is a child of a directory this call just created exclusively, it is not a gate target, and its creation-failure path deliberately returns directly rather than through teardown.
  Note in a comment that `os.Mkdir`'s `EEXIST` is now the safety property and the stat guards are UX and ordering, and that being later the exclusive create is also strictly more current — it closes a real time-of-check-to-time-of-use window that exists today between the stat and the `MkdirAll`, in which a concurrent process can create the hub and have `os.MkdirAll` silently accept it.
- **Commit:** `fix(fabricengine): create the hub exclusively and mint its teardown token`

### Card 21: gate resetHub

- **Context:**
  - `internal/fabricengine/destroy.go`
- **Edits:**
  - `internal/fabricengine/clone.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** give `resetHub` a `cwd string` parameter — its current signature is `resetHub(hubPath string)` and it carries no parent to contain against, so without one the site cannot declare containment at all, and `--reset` on a derived name is the R4 defect's own path.
  Update both of its call sites in `CloneHub` to pass the same `cwd` the function normalised at its top.
  Convert the `RemoveAll(hubPath)` call to `removePath` on a `pathRequest` with `what` naming the hub, container `cwd`, target `hubPath`, `slug` nil, ownership `ownedFabricHub()`, dirtiness `dirtinessNA("--reset is the operator explicitly asking for this hub to be replaced; ownership is the check that matters here")` and `force` false.
  Keep every existing guard and message in the function: the absent-path silent no-op that keeps `--reset` idempotent, the `reset: inspect %s: %w` stat failure, the not-a-directory refusal, the explicit `looksLikeHub` refusal with its "it has no %s and no %q sibling, so it is not a fabric hub" message, and the `reset: remove hub at %s: %w` wrapper for an operational failure.
  The gate runs `looksLikeHub` too;
  the site's own call is what produces the message the operator reads.
  Containment holds today and asserting it costs nothing, which is `refuseUncontainedPath`'s own stated rationale: the hub path is the operator-named parent joined with a derived name carrying the hub suffix, and the name derivation splits on every separator so none survives — a derived `..` becomes the harmless `..-HUB`.
- **Commit:** `fix(fabricengine): gate resetHub with containment against the operator-named parent`

### Card 22: gate teardownHub

- **Context:**
  - `internal/fabricengine/destroy.go`
- **Edits:**
  - `internal/fabricengine/clone.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** change `teardownHub`'s signature from `teardownHub(hubPath string, cause error) error` to one that also takes the operator-named `cwd` and the `createdToken` card 20 mints, and update all thirteen call sites in `CloneHub` — every one of them is after the creation, so the token is always in scope.
  Convert the `RemoveAll(hubPath)` call to `removePath` on a `pathRequest` with container `cwd`, target `hubPath`, `slug` nil, ownership `ownedFreshlyCreatedPath(tok)`, dirtiness `dirtinessNA("gate-created within this invocation; nothing pre-existing to lose")` and `force` false.
  Keep the function's two return shapes unchanged: `cause` unchanged on success, and the existing "%w; residual hub left at %s; remove it manually before retrying" combination on a removal failure.
  Do **not** declare `ownedFabricHub` here.
  That kind requires a board entry or at least one weft sibling, and this function's earliest call sites run immediately after the warp clone attempt when neither exists yet, so it would refuse at nearly every early failure and leave a residual hub where teardown works today — a gate that blocks cleanup of a half-built hub is worse than the gap it closes.
  Do not invent a trust-me kind either;
  the token is what makes the claim verifiable, and it is strictly stronger than the pattern-match `--reset` uses, because pattern-matching a derived name is exactly what R4's `clone --reset` defect exploited.
  Do not pass `force: true` to make the rollback pass: that would make a rollback indistinguishable from an operator's `--force`, which the gate's force rule explicitly forbids conflating.
- **Commit:** `fix(fabricengine): gate teardownHub with containment and a gate-minted token`

## Batch Tests

`verify:` runs both tiers of `internal/fabricengine`.
The integration tier is the real gate on this batch: `clone_reset_guard_test.go` covers `--reset`'s ownership half directly, and `clone_adopt_test.go`, `warpbinding_clone_integration_test.go` and `clone_test.go` together drive `CloneHub`'s success and failure paths, which is what exercises the thirteen teardown call sites and the new exclusive create.
A teardown that started refusing at an early failure site would show up there as a residual-hub error rather than a clean rollback, which is precisely the regression this batch's ownership-kind choice is designed to avoid.

Scope stays the one package;
the only file this batch edits is `internal/fabricengine/clone.go`, and the module-wide `go build ./...` at the batch boundary covers the two signature changes.

The dedicated test for this batch's gap — that `teardownHub` refuses a hub path outside the operator-named parent and still **succeeds** on a half-built hub with no board and no weft sibling — is written in batch 7 alongside the other two gap tests, so all three share one fixture vocabulary rather than three.
