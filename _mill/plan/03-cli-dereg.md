# Batch: C -- CLI de-registration + sandbox tags

```yaml
task: 'fabric: cutover -- rewire consumers onto fabric, delete warp/weft'
batch: C -- CLI de-registration + sandbox tags
number: 3
cards: 2
verify: go test ./cmd/lyx/... ./tools/sandbox/...
depends-on: []
```

## Batch Scope

Remove the `lyx warp` / `lyx weft` CLI *registration* from the cobra root and update every
pinned `cmd/lyx` test set plus the sandbox coverage tags in ONE atomic commit (card 13) --
the moment warp/weft leave `newRoot()`, the help-tree, registration, longlist, and
sandbox-coverage guards all fail unless updated together. warpcli/weftcli packages still
exist and compile (they are deleted in batch D1); this batch only de-registers them, so it
is independent of A and B. Card 14 flips the shared-hub sandbox bootstrap's `cloneRun` shell
command from `lyx warp clone` to `lyx fabric clone` (a runtime string, not a `go test`
dependency, hence a separate green commit).

## Cards

### Card 13: de-register warp/weft from cobra root + update pinned cmd/lyx tests + CORE-SUITE tags

- **Context:**
  - `cmd/lyx/registration_test.go`
  - `cmd/lyx/longlist_test.go`
  - `cmd/lyx/drift_test.go`
  - `cmd/lyx/sandbox_coverage_test.go`
  - `internal/fabriccli/fabric.go`
- **Edits:**
  - `cmd/lyx/main.go`
  - `cmd/lyx/helptree_test.go`
  - `cmd/lyx/main_test.go`
  - `cmd/lyx/unknown_subcommand_test.go`
  - `cmd/lyx/jsonhelp_test.go`
  - `cmd/lyx/exitcode_test.go`
  - `tools/sandbox/SANDBOX-CORE-SUITE.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** This is one atomic commit; every edit below lands together so
  `go test ./cmd/lyx/...` stays green.
  - `cmd/lyx/main.go`: remove the `warpcli` and `weftcli` imports; remove the
    `root.AddCommand(weftcli.Command())` and `root.AddCommand(warpcli.Command())`
    registrations from `newRoot()`; remove the `weft` and `warp` names from the `root.Long`
    module list (keep `fabric`); in the group-ordering comment that currently reads
    "board, ide, reed, weft ..." drop the stale `weft` word.
  - `cmd/lyx/helptree_test.go`: remove `"weft"` and `"warp"` from the `requiredModules` set;
    delete the warp and weft subcommand-table cases (the `fabric` case already covers the
    union of their subcommands).
  - `cmd/lyx/main_test.go`: remove the warp/weft behavioural subtests -- the
    `run([]string{"warp","list"})`, `run([]string{"weft","status"})`, and
    `run([]string{"warp","clone"})` cases (and the `["board","warp"]` module loop entry, drop
    `"warp"`) -- since those commands no longer register.
  - `cmd/lyx/unknown_subcommand_test.go`: remove the `{"warp"}`, `{"weft"}`, and
    `{"weft","commit"}` table rows and the whole `TestMountedBareWarp` test; update the
    file-top comment that enumerates guarded/unguarded groups to drop warp/weft.
  - `cmd/lyx/jsonhelp_test.go`: remove `"weft"` and `"warp"` from the module-enumeration list
    and delete the whole `TestJSONHelp_LeafWithFlag` test (it drives `warp remove --help
    --json`); if a replacement leaf-with-flag smoke is wanted, re-point it at an existing
    fabric leaf, otherwise deletion is fine (other tests cover leaf JSON help).
  - `cmd/lyx/exitcode_test.go`: remove the `{"lyx warp (no subcommand)", []string{"warp"}}`
    table row.
  - `tools/sandbox/SANDBOX-CORE-SUITE.md`: delete CORE-SUITE scenario S7 (`Covers: weft`) and
    scenario S8 (`Covers: warp`) so no `Covers:` tag names an unregistered module (fabric
    stays covered by SANDBOX-FABRIC-SUITE.md). This keeps
    `sandbox_coverage_test.go` Assert 2 green.
  `registration_test.go`, `longlist_test.go`, and `drift_test.go` are discovery-driven and
  pass automatically once `main.go` is consistent -- listed as Context to confirm, not edit.
- **Commit:** `refactor(cmd/lyx): de-register warp/weft CLI and update pinned test sets`

### Card 14: flip shared-hub sandbox bootstrap to fabric clone

- **Context:**
  - `internal/fabriccli/fabric.go`
- **Edits:**
  - `tools/sandbox/main.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In the shared-hub bootstrap `cloneRun`, change the shelled command from
  `lyx warp clone ...` to `lyx fabric clone ...` (same arguments; `fabric clone` is the
  drop-in replacement for `warp clone`). Do NOT touch the dedicated fabric-hub plumbing
  (`fabricCloneRun`/`decideFabricClone`/`runFabricSuite`/the `fabric-suite` subcommand) and
  do NOT rewrite the now-stale parallel-build prose comments here -- that prose cleanup is
  card 24 in batch D3 (kept separate so this batch's diff is the single functional flip).
- **Commit:** `refactor(sandbox): clone shared hub via fabric clone`

## Batch Tests

`verify` runs `go test ./cmd/lyx/...` (help-tree, registration, longlist, drift,
sandbox-coverage, tierpurity, hermeticenv guards -- all sensitive to the cobra root and the
SUITE.md coverage tags) and `go test ./tools/sandbox/...` (guards on `tools/sandbox/main.go`,
e.g. pathresolve). No `-tags integration` needed: these are the fast guard tests. The card-13
edits are validated together as one atomic commit; card 14 is a runtime-string change that
does not affect any `go test` assertion.
