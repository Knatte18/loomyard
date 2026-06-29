# Batch: warp split

```yaml
task: "Rename Cobra modules to `<module>cli`, extract kernels as `<module>engine`"
batch: "warp split"
number: 3
cards: 4
verify: "go build ./... && go test ./... && go test -tags integration ./..."
depends-on: [2]
```

## Rename mechanic — `git mv`, not rewrite

The cards below list **Creates:** / **Deletes:** as the END STATE, not the procedure.
Almost every "created" file is the matching old-package file **moved**, not authored from
scratch. For each moved file:

1. `git mv <old-path> <new-path>` first — git records a rename, history is preserved, and
   the diff stays a small rename instead of an add+delete.
2. Then apply **surgical edits** only to the lines that actually change: the `package`
   declaration, import paths, identifier/type retargeting, and any `Command()`/`RunCLI`
   seam split.
3. Use full-file creation **only** for a genuinely new file with no predecessor. Never
   write a file from scratch and then delete its old twin.

Note: a file that is **split across two packages** (e.g. `clone.go` → `warpcli/clone.go`
+ `warpengine/clone.go`) is still a move — `git mv` the file to the larger half, then
extract the smaller half into the second file with surgical edits.

## Batch Scope

Split `internal/warp` into `internal/warpengine` (domain kernel) and `internal/warpcli`
(cobra command). The hard part is `clone.go`, which must be **split across two files**:
the handler half (`runClone`, `runCloneWithReset`) → `warpcli/clone.go`; the domain half
(`cloneHub`, `cloneRepo`, `teardownHub`, `deriveHostName`, `deriveBoardURL`, the
`hubSuffix` const, and the `removeAll` test seam) → `warpengine/clone.go`. Because the cli
half calls into the domain half, export the surface it needs:
`cloneHub` → `warpengine.CloneHub` (returns `(hubPath, resolvedBoardURL, err)`),
`deriveHostName` → `warpengine.DeriveHostName`, `hubSuffix` → `warpengine.HubSuffix`, and
the `removeAll` seam → a single exported settable `var RemoveAll = os.RemoveAll` in
`warpengine`, used by **both** `runCloneWithReset` (cli) and `teardownHub` (engine).
`cloneRepo`/`deriveBoardURL` stay engine-internal. The clone test files are **physically
split** by what each test drives. Importers retargeted this batch: `cmd/lyx/main.go`,
`internal/configreg/configreg.go`, `internal/initcli/initcli.go` (production
`warp.WireJunctions`), `internal/initcli/initcli_test.go`, and the `warp.*` usage in
`internal/configcli/configcli_integration_test.go`.

## Cards

### Card 8: Create `internal/warpengine` domain package (incl. clone domain half)

- **Context:**
  - `internal/warp/add.go`
  - `internal/warp/checkout.go`
  - `internal/warp/remove.go`
  - `internal/warp/prune.go`
  - `internal/warp/cleanup.go`
  - `internal/warp/status.go`
  - `internal/warp/list.go`
  - `internal/warp/reconcile.go`
  - `internal/warp/worktreelifecycle.go`
  - `internal/warp/drift.go`
  - `internal/warp/junction.go`
  - `internal/warp/hook.go`
  - `internal/warp/weftwiring.go`
  - `internal/warp/launchers.go`
  - `internal/warp/portals.go`
  - `internal/warp/ancestors.go`
  - `internal/warp/config.go`
  - `internal/warp/template.go`
  - `internal/warp/clone.go`
  - `internal/warp/warp.go`
  - `internal/warp/template.yaml`
  - `internal/warp/post-checkout.sh`
  - `internal/fslink/fslink.go`
- **Edits:** none
- **Creates:**
  - `internal/warpengine/add.go`
  - `internal/warpengine/checkout.go`
  - `internal/warpengine/remove.go`
  - `internal/warpengine/prune.go`
  - `internal/warpengine/cleanup.go`
  - `internal/warpengine/status.go`
  - `internal/warpengine/list.go`
  - `internal/warpengine/reconcile.go`
  - `internal/warpengine/worktreelifecycle.go`
  - `internal/warpengine/drift.go`
  - `internal/warpengine/junction.go`
  - `internal/warpengine/hook.go`
  - `internal/warpengine/weftwiring.go`
  - `internal/warpengine/launchers.go`
  - `internal/warpengine/portals.go`
  - `internal/warpengine/ancestors.go`
  - `internal/warpengine/config.go`
  - `internal/warpengine/template.go`
  - `internal/warpengine/clone.go`
  - `internal/warpengine/template.yaml`
  - `internal/warpengine/post-checkout.sh`
- **Deletes:** none
- **Requirements:** Move all warp domain non-test files into `internal/warpengine` with
  package clause `package warp` → `package warpengine`, content otherwise byte-identical:
  `add.go`, `checkout.go`, `remove.go`, `prune.go`, `cleanup.go`, `status.go`, `list.go`,
  `reconcile.go`, `worktreelifecycle.go` (`Worktree`, `New`), `drift.go` (`PairInSync`),
  `junction.go` (`WireJunctions`), `hook.go` (`InstallPostCheckoutHook`), `weftwiring.go`,
  `launchers.go`, `portals.go`, `ancestors.go`, `config.go`, `template.go`
  (`ConfigTemplate`), plus the `template.yaml` and `post-checkout.sh` assets. **Split
  `clone.go`:** create `internal/warpengine/clone.go` holding only the domain half —
  `cloneHub` renamed to exported `CloneHub` (signature returning
  `(hubPath, resolvedBoardURL, err)` unchanged), `cloneRepo` (unexported), `teardownHub`,
  `deriveHostName` renamed to exported `DeriveHostName`, `deriveBoardURL` (unexported),
  the `hubSuffix` const renamed to exported `HubSuffix`, and a single exported settable
  seam `var RemoveAll = os.RemoveAll` (replacing the old `removeAll`); `teardownHub` calls
  `RemoveAll`. Do NOT move `runClone`/`runCloneWithReset` (card 10) and do not move
  `warp.go` (card 10) — they are read-only Context here so you can see the call sites the
  exports must satisfy. Do not move test files (card 9) and do not delete `internal/warp`
  (card 11). Keep `internal/warpengine` free of any cobra import and of
  `internal/initcli`/`internal/configsync` imports (warp's package-doc dependency
  discipline).
- **Commit:** `refactor(warp): extract warpengine domain package and clone domain half`

### Card 9: Relocate and split warpengine tests

- **Context:**
  - `internal/warp/add_test.go`
  - `internal/warp/checkout_test.go`
  - `internal/warp/cleanup_test.go`
  - `internal/warp/config_test.go`
  - `internal/warp/drift_test.go`
  - `internal/warp/hook_test.go`
  - `internal/warp/launchers_test.go`
  - `internal/warp/list_test.go`
  - `internal/warp/portals_test.go`
  - `internal/warp/prune_test.go`
  - `internal/warp/reconcile_test.go`
  - `internal/warp/remove_test.go`
  - `internal/warp/status_test.go`
  - `internal/warp/template_test.go`
  - `internal/warp/weftwiring_test.go`
  - `internal/warp/ancestors_test.go`
  - `internal/warp/clone_test.go`
  - `internal/warp/clone_integration_test.go`
  - `internal/warpengine/clone.go`
- **Edits:** none
- **Creates:**
  - `internal/warpengine/add_test.go`
  - `internal/warpengine/checkout_test.go`
  - `internal/warpengine/cleanup_test.go`
  - `internal/warpengine/config_test.go`
  - `internal/warpengine/drift_test.go`
  - `internal/warpengine/hook_test.go`
  - `internal/warpengine/launchers_test.go`
  - `internal/warpengine/list_test.go`
  - `internal/warpengine/portals_test.go`
  - `internal/warpengine/prune_test.go`
  - `internal/warpengine/reconcile_test.go`
  - `internal/warpengine/remove_test.go`
  - `internal/warpengine/status_test.go`
  - `internal/warpengine/template_test.go`
  - `internal/warpengine/weftwiring_test.go`
  - `internal/warpengine/ancestors_test.go`
  - `internal/warpengine/clone_test.go`
  - `internal/warpengine/clone_integration_test.go`
- **Deletes:** none
- **Requirements:** Move every domain `*_test.go` into `internal/warpengine` with package
  clause changed to `warpengine` (or `warpengine_test` if the original used an external
  `warp_test` package — preserve the external/internal distinction). Preserve every
  `//go:build integration` tag verbatim (`add_test`, `checkout_test`, `cleanup_test`,
  `drift_test`, `hook_test`, `launchers_test`, `list_test`, `portals_test`, `prune_test`,
  `reconcile_test`, `remove_test`, `status_test`, `weftwiring_test`, and
  `clone_integration_test` are integration-tagged; `config_test`/`template_test`/
  `ancestors_test` follow their originals). **Split the clone tests by what each test
  drives:** the domain-driving scenarios in `clone_test.go` and the `cloneHub`-driving
  scenarios in `clone_integration_test.go` go to `internal/warpengine/clone_test.go` and
  `internal/warpengine/clone_integration_test.go`, calling `warpengine.CloneHub` (and
  swapping `warpengine.RemoveAll` where they swapped the old `removeAll`). The reset-swap
  test (the `runCloneWithReset` scenario, `clone_integration_test.go` lines ≈309–353) and
  any handler-driving test in `clone_test.go` are **NOT** moved here — they go to
  `warpcli` in card 10. Keep every assertion intact.
- **Commit:** `test(warp): relocate warpengine test suites and split clone tests`

### Card 10: Create `internal/warpcli` command package (incl. clone handler half)

- **Context:**
  - `internal/warp/warp.go`
  - `internal/warp/clone.go`
  - `internal/warp/warp_test.go`
  - `internal/warp/clone_integration_test.go`
  - `internal/warp/clone_test.go`
  - `internal/clihelp/exec.go`
- **Edits:** none
- **Creates:**
  - `internal/warpcli/warp.go`
  - `internal/warpcli/clone.go`
  - `internal/warpcli/warp_test.go`
  - `internal/warpcli/clone_cli_test.go`
- **Deletes:** none
- **Requirements:** Move `warp.go` (the cobra tree + handlers, `Command()`, the `RunCLI`
  seam) into `internal/warpcli/warp.go` with package `warp` → `warpcli`. Create
  `internal/warpcli/clone.go` holding the handler half of the original `clone.go`:
  `runClone` and `runCloneWithReset`. Add the `internal/warpengine` import to warpcli and
  retarget the calls those handlers make into the clone domain — `cloneHub` →
  `warpengine.CloneHub`, `deriveHostName` → `warpengine.DeriveHostName`, `hubSuffix` →
  `warpengine.HubSuffix`, the `removeAll` swap point → `warpengine.RemoveAll` — plus any
  other engine symbols the handlers call (`New`, `Config`, `AddOptions`, etc.) as
  `warpengine.<Symbol>`. The `RunCLI` seam body stays exactly
  `clihelp.Execute(Command(), out, args)`. Move `warp_test.go` to
  `internal/warpcli/warp_test.go`, preserving its `//go:build integration` tag; it is an
  **external** test file (declared `package warp_test`) so its clause becomes
  `package warpcli_test` (NOT `warpcli`). Put the reset-swap test (and any handler-driving
  test from `clone_test.go`) into `internal/warpcli/clone_cli_test.go`, swapping the
  exported `warpengine.RemoveAll` seam cross-package; the reset-swap test comes from the
  internal `package warp` `clone_integration_test.go`, so this file is `package warpcli`
  (internal, which can still swap the exported `warpengine.RemoveAll` cross-package).
  Preserve the original `//go:build integration` tag. If `clone_test.go` contained any
  **untagged** handler test, place it in a separate untagged warpcli clone test file
  rather than mixing build tags in one file.
- **Commit:** `refactor(warp): extract warpcli command package and clone handler half`

### Card 11: Retarget importers and delete `internal/warp`

- **Context:**
  - `internal/warp/warp.go`
  - `internal/warp/clone.go`
  - `internal/warp/junction.go`
- **Edits:**
  - `cmd/lyx/main.go`
  - `internal/configreg/configreg.go`
  - `internal/initcli/initcli.go`
  - `internal/initcli/initcli_test.go`
  - `internal/configcli/configcli_integration_test.go`
- **Creates:** none
- **Deletes:**
  - `internal/warp/add.go`
  - `internal/warp/checkout.go`
  - `internal/warp/remove.go`
  - `internal/warp/prune.go`
  - `internal/warp/cleanup.go`
  - `internal/warp/status.go`
  - `internal/warp/list.go`
  - `internal/warp/reconcile.go`
  - `internal/warp/worktreelifecycle.go`
  - `internal/warp/drift.go`
  - `internal/warp/junction.go`
  - `internal/warp/hook.go`
  - `internal/warp/weftwiring.go`
  - `internal/warp/launchers.go`
  - `internal/warp/portals.go`
  - `internal/warp/ancestors.go`
  - `internal/warp/config.go`
  - `internal/warp/template.go`
  - `internal/warp/clone.go`
  - `internal/warp/warp.go`
  - `internal/warp/template.yaml`
  - `internal/warp/post-checkout.sh`
  - `internal/warp/add_test.go`
  - `internal/warp/checkout_test.go`
  - `internal/warp/cleanup_test.go`
  - `internal/warp/config_test.go`
  - `internal/warp/drift_test.go`
  - `internal/warp/hook_test.go`
  - `internal/warp/launchers_test.go`
  - `internal/warp/list_test.go`
  - `internal/warp/portals_test.go`
  - `internal/warp/prune_test.go`
  - `internal/warp/reconcile_test.go`
  - `internal/warp/remove_test.go`
  - `internal/warp/status_test.go`
  - `internal/warp/template_test.go`
  - `internal/warp/weftwiring_test.go`
  - `internal/warp/ancestors_test.go`
  - `internal/warp/clone_test.go`
  - `internal/warp/clone_integration_test.go`
  - `internal/warp/warp_test.go`
- **Requirements:** In `cmd/lyx/main.go` replace the `internal/warp` import with
  `internal/warpcli` and change `warp.Command()` to `warpcli.Command()`. In
  `internal/configreg/configreg.go` replace the `internal/warp` import with
  `internal/warpengine` and change `{"warp", warp.ConfigTemplate}` to
  `{"warp", warpengine.ConfigTemplate}` (module name string stays `"warp"`). In
  `internal/initcli/initcli.go` replace the `internal/warp` import with
  `internal/warpengine` and change `warp.WireJunctions` to `warpengine.WireJunctions`. In
  `internal/initcli/initcli_test.go` replace the `internal/warp` import with
  `internal/warpengine` and change `warp.LoadConfig` to `warpengine.LoadConfig`. In
  `internal/configcli/configcli_integration_test.go` replace the `internal/warp` import
  with `internal/warpengine` and change `warp.New`, `warp.Config`, `warp.AddOptions`, and
  `warp.WireJunctions` to `warpengine.<Symbol>` (its weft usage already points at
  `weftcli` from batch 2). Then delete the entire `internal/warp` directory.
- **Commit:** `refactor(warp): retarget importers and remove internal/warp`

## Batch Tests

`verify` is repo-wide (Tier 1 + Tier 2) for the same reasons as the prior batches; warp is
the most integration-heavy module (nearly every warp `*_test.go` is `integration`-tagged),
so the `-tags integration` run is essential to compile-and-run the relocated suites. Moved
coverage: the full warpengine domain suite (add/checkout/cleanup/drift/hook/launchers/list/
portals/prune/reconcile/remove/status/template/weftwiring/ancestors/config and the
`CloneHub` clone scenarios) and the warpcli `warp_test` plus the reset-swap clone test
exercising `runCloneWithReset` against the swapped `warpengine.RemoveAll`. The
`internal/initcli` and `internal/configcli` integration tests re-exercise the retargeted
seams; cmd/lyx guard tests self-derive and re-validate the renamed `warpcli` registration.
