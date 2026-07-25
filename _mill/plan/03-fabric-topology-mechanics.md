# Batch: fabric-topology-mechanics

```yaml
task: 'fabric: unify warp + weft into one git-coordination module'
batch: fabric-topology-mechanics
number: 3
cards: 5
verify: go test -tags integration ./internal/fabricengine
depends-on: [2]
```

## Batch Scope

Gives fabricengine its self-contained copies of warpengine's unexported filesystem and
weft-wiring mechanics (deliberate duplication per the overview decision) plus the
package-level `CloneHub` with full board parity — everything batch 4's lifecycle verbs
compose. The one behavioral delta threaded through this batch is the uniform branch
scheme: every weft branch this code creates or checks is a `WeftBranchName(...)` value,
and clone leaves the weft primary checked out on `main-weft`. External interface for
batch 4: `hostLayoutFor`, `pruneEmptyAncestors`, `createPortal`/`removePortal`,
`writeLaunchers`/`removeLaunchers`, `WireJunctions`/`UnwireJunctions`,
`InstallPostCheckoutHook`, the weftwiring helpers, and
`CloneHub`/`DeriveHostName`/`RemoveAll`. Batch-local decision: adapted copies keep
warpengine's function names, signatures, and result types wherever behavior is
identical, so a reviewer can diff copy against source; only the deliberate deltas
(branch names, launcher verb, hook sentinel) differ.

## Cards

### Card 8: filesystem mechanics copies

- **Context:**
  - `internal/warpengine/hostlayout.go`
  - `internal/warpengine/ancestors.go`
  - `internal/warpengine/ancestors_test.go`
  - `internal/warpengine/portals.go`
  - `internal/warpengine/launchers.go`
  - `internal/warpengine/launcher_content.go`
  - `internal/warpengine/launcher_content_test.go`
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/fslink/fslink.go`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/hostlayout.go`
  - `internal/fabricengine/ancestors.go`
  - `internal/fabricengine/ancestors_test.go`
  - `internal/fabricengine/portals.go`
  - `internal/fabricengine/launchers.go`
  - `internal/fabricengine/launcher_content.go`
  - `internal/fabricengine/launcher_content_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Adapted copies in package `fabricengine` of warpengine's
  `hostLayoutFor` (SiblingLayout fast path preserved), `pruneEmptyAncestors`,
  `createPortal`/`removePortal` (via `fslink.CreateDirLink`/`fslink.Remove` and
  `hubgeometry.Layout` portal helpers), `writeLaunchers`/`removeLaunchers`, and the
  pure `launcherExt`/`launcherScript`. Deltas: the checkout launcher file is
  `fabric-checkout<ext>` and its script's lyx args invoke `fabric checkout` instead of
  `warp checkout`; everything else byte-equivalent in behavior. All geometry through
  hubgeometry helpers — no geometry-token literals. Untagged tests: copies of
  `ancestors_test.go` and `launcher_content_test.go` adapted to the package, the latter
  asserting the `fabric-checkout` script content and `.cmd`/`.sh` extension selection.
- **Commit:** `feat(fabricengine): self-contained portal, launcher, and layout mechanics`

### Card 9: junctions and post-checkout hook

- **Context:**
  - `internal/warpengine/junction.go`
  - `internal/warpengine/hook.go`
  - `internal/warpengine/post-checkout.sh`
  - `internal/warpengine/hook_test.go`
  - `internal/warpengine/unjunction_test.go`
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/fslink/fslink.go`
  - `internal/gitexec/gitexec.go`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/junction.go`
  - `internal/fabricengine/hook.go`
  - `internal/fabricengine/post-checkout.sh`
  - `internal/fabricengine/hook_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Adapted copies of `WireJunctions`/`UnwireJunctions` (with
  `UnwireResult` and the seed/unseed git-exclude helpers) and `InstallPostCheckoutHook`
  (embedded `post-checkout.sh`, user-hook chaining via `chainUserHook`). Deltas: the
  sentinel const is `FABRIC_SENTINEL: post-checkout drift warning` (fabric-installed
  hooks must never collide with warp's sentinel detection and vice versa), and the
  script's drift check expects the weft branch to be the host branch plus the `-weft`
  suffix (shell-side derivation in the copied script; the suffix literal in the `.sh`
  is data, not Go path construction). Integration-tagged `hook_test.go` adapted from
  warp's: install idempotency by sentinel, user-hook chaining idempotency, and
  weft-branch resolution for prime and child worktrees under the suffixed scheme.
  Junction wire/unwire behavior is exercised via batch 4's differential lifecycle
  tests; no separate unjunction test copy.
- **Commit:** `feat(fabricengine): junction wiring and suffix-aware post-checkout hook`

### Card 10: weft wiring with uniform branch scheme

- **Context:**
  - `internal/warpengine/weftwiring.go`
  - `internal/warpengine/weftwiring_test.go`
  - `internal/warpengine/add.go`
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/gitexec/gitexec.go`
  - `internal/fabricengine/branchname.go`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/weftwiring.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Adapted copies of `weftRepoExists`, `weftBranchExists`,
  `createWeftWorktree`, `pushWeftBranch`, `removeHostJunction`, `removeWeftWorktree`
  keeping warp's signatures where the argument is already a concrete weft branch name.
  The branch arguments callers pass are ALWAYS `WeftBranchName(...)` values — this
  file never derives names itself and contains no branch-name composition; godoc on
  each helper states the argument is the suffixed weft branch. Push honors the
  `SkipGit`/`SkipPush` options exactly as warp's `pushWeftBranch` does. Covered by
  batch 4's differential lifecycle tests (fork-point isolation, rollback, invalid
  start point) — warp's `weftwiring_test.go` documents the behaviors those tests must
  reproduce differentially.
- **Commit:** `feat(fabricengine): weft worktree wiring helpers`

### Card 11: CloneHub with board parity and main-weft primary

- **Context:**
  - `internal/warpengine/clone.go`
  - `internal/warpengine/clone_test.go`
  - `internal/warpengine/clone_integration_test.go`
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/gitexec/gitexec.go`
  - `internal/fabricengine/branchname.go`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/clone.go`
  - `internal/fabricengine/clone_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Adapted copies of `CloneHub(cwd, hostURL, weftURL, boardURL)
  (hubPath, resolvedBoardURL string, err error)`, `DeriveHostName`, `deriveBoardURL`
  (`<weftURL>.wiki.git` default), `cloneRepo`, `teardownHub`, and the exported
  `var RemoveAll = os.RemoveAll` teardown seam (fabric's OWN seam — the differential
  test tears each side down via its own module's seam, never warp's). Parity scope:
  clones host, weft, AND board into `<name>-HUB` via `hubgeometry.HubPath`, optional
  board URL, strict-abort teardown on any failure. The one delta: after the weft clone
  succeeds, read the weft primary's checked-out branch (`git branch --show-current`),
  create and check out `WeftBranchName(<that branch>)` at its HEAD (e.g. `main` →
  `main-weft`), so `weft:main` is never claimed. `InstallPostCheckoutHook` wiring
  mirrors warp's clone if present there. Untagged `clone_test.go`: `DeriveHostName`
  cases copied, `deriveBoardURL` default + explicit, invalid-URL clone failure.
- **Commit:** `feat(fabricengine): CloneHub with board parity and main-weft primary`

### Card 12: differential clone test

- **Context:**
  - `internal/warpengine/clone.go`
  - `internal/warpengine/clone_integration_test.go`
  - `internal/lyxtest/lyxtest.go`
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/fabricengine/clone.go`
  - `internal/fabricengine/branchname.go`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/clone_differential_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Integration-tagged, package `fabricengine_test` (imports both
  engines). Build one set of local bare fixtures (host, weft, board — same technique as
  `clone_integration_test.go`), then run `warpengine.CloneHub` and
  `fabricengine.CloneHub` into two separate parent dirs and assert equivalent end
  state: same hub directory name and layout (host clone, weft sibling, `_board`),
  same `resolvedBoardURL` contract (derived and explicit variants), geometry
  round-trips through `hubgeometry.Resolve` on both — normalizing the one delta: warp's
  weft primary sits on `main`, fabric's on `main-weft` (assert fabric's weft primary
  branch equals `fabricengine.WeftBranchName` of warp's). Also assert strict-abort
  equivalence: a failing weft URL leaves NO hub dir on either side, each torn down via
  its own module's `RemoveAll` seam.
- **Commit:** `test(fabricengine): differential clone equivalence against warp`

## Batch Tests

`verify: go test -tags integration ./internal/fabricengine` runs the new untagged tests
(ancestors, launcher content, clone derivation) plus the integration-tagged hook and
differential clone tests, alongside batch 2's Tier-1 tests. Junction and weftwiring
copies are deliberately not unit-copied — they are exercised end-to-end by the batch 4
differential lifecycle tests, with warp's own test files listed as the behavioral
reference in this batch's Context.
