# Batch: coordinated-checkout

```yaml
task: 'Introduce warp: the host↔weft-coordinated git module'
batch: coordinated-checkout
number: 5
cards: 3
verify: go build ./... && go test ./internal/warp/
depends-on: [4]
```

## Batch Scope

Deliver the priority correctness gap: `lyx warp checkout <branch>` switches the host worktree and its weft sibling together and re-points junctions, as an all-or-nothing operation. Preconditions are checked first (refuse on a dirty weft worktree; surface git's own refusal when the host has conflicting changes); on weft-side failure the host switch is rolled back so the pair is never half-switched. Switching onto a host branch with no weft sibling **forks the weft branch via the same fork-point path as `warp add`** (adopt-or-create), producing a managed pair.

External interface batch 8 consumes: `warp.Checkout(...)` and the `lyx warp checkout` CLI verb (the launcher shortcut and hook reference it).

Batch-local decision: checkout reuses card-12's adopt-or-create fork-point logic and card-11's junction primitive; it does not duplicate branch-fork math. The drift primitive (card 13) is the precondition source of truth for "is the pair currently consistent."

## Cards

### Card 15: warp checkout — coordinated switch with rollback

- **Context:**
  - `internal/warp/add.go`
  - `internal/warp/weftwiring.go`
  - `internal/warp/junction.go`
  - `internal/warp/drift.go`
  - `internal/paths/paths.go`
  - `internal/gitexec/gitexec.go`
- **Edits:** none
- **Creates:**
  - `internal/warp/checkout.go`
- **Requirements:** Create `internal/warp/checkout.go` with `func (w *Worktree) Checkout(l *paths.Layout, branch string) (CheckoutResult, error)` (define a small `CheckoutResult` with the switched branch + weft path, JSON-tagged). Steps, all-or-nothing: (1) precondition — refuse if the weft worktree is dirty (`gitexec` `status --porcelain`), and let git's own refusal propagate if the host switch would clobber uncommitted host changes; (2) switch the host worktree to `branch` (`git checkout`/`switch`); (3) resolve the weft sibling branch: if it exists, switch the weft worktree to it; if it does not (unmanaged target), fork it from the parent's weft branch using the same adopt-or-create/fork-point helper as `Add`; (4) re-point the junction(s) via the junction primitive; (5) on any failure at step 3–4, **roll back** the host switch to the original branch and return the original error untouched. Never leave a half-switched pair.
- **Commit:** `feat(warp): coordinated host+weft checkout with rollback`

### Card 16: Route checkout through RunCLI

- **Context:**
  - `internal/warp/checkout.go`
  - `internal/warp/worktreelifecycle.go`
  - `internal/paths/paths.go`
  - `internal/output/output.go`
- **Edits:**
  - `internal/warp/warp.go`
- **Creates:** none
- **Requirements:** In `internal/warp/warp.go` `RunCLI`, add `case "checkout"`: parse `<branch>` (usage `usage: lyx warp checkout <branch>`), resolve layout, `LoadConfig(cwd, "warp")`, `New(cfg)`, call `Checkout`, emit JSON `{branch, weft_worktree}` on success via `output.Ok`.
- **Commit:** `feat(warp): route lyx warp checkout`

### Card 17: Coordinated-checkout tests

- **Context:**
  - `internal/warp/checkout.go`
  - `internal/warp/add_test.go`
  - `internal/lyxtest/lyxtest.go`
  - `internal/paths/paths.go`
- **Edits:** none
- **Creates:**
  - `internal/warp/checkout_test.go`
- **Requirements:** Create `internal/warp/checkout_test.go` (integration-tagged where it drives real git, mirroring `clone_integration_test.go`'s style) covering: (1) happy path — host+weft both move to the target branch and the junction re-points; (2) dirty-weft precondition refusal — no switch occurs; (3) host rollback — force a weft-side failure and assert the host worktree is back on its original branch and the pair is untouched; (4) checkout onto an unmanaged branch — the weft branch is forked from the parent's weft (managed pair results), matching `warp add`'s fork-point. Seed config via `warp.ConfigTemplate()` at the call site (lyxtest leaf invariant).
- **Commit:** `test(warp): coordinated checkout happy/refusal/rollback/fork paths`

## Batch Tests

`verify: go build ./... && go test ./internal/warp/`. `checkout_test.go` is the new TDD surface (write the four scenarios first). The full `internal/warp` suite must stay green — checkout adds a verb and a file without changing existing behaviour. Integration-tagged cases drive real git; run them under the same tag the existing clone integration test uses.
