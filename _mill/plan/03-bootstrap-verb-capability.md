# Batch: bootstrap-verb-capability

```yaml
task: 'Seeded driver choice: ly-drive strand as the child''s driver'
batch: bootstrap-verb-capability
number: 3
cards: 3
verify: go build ./... && go test ./internal/loomcli/... ./internal/battencli/... ./internal/shedcli/...
depends-on: []
```

## Batch Scope

This batch declares, once per recipe, whether that recipe has a bootstrap verb — which is exactly the question "may this recipe be seeded `llm`", asked in a form that stays true as recipes are added.
Each CLI package that owns a verb exports its own constant beside the command it names, and `internal/shedcli`'s recipe-table entry gains a field populated from those constants.
It is one batch because the two constants and the table field are one fact recorded in three places, and the sync meta-test that keeps them from drifting cannot be written until all three exist.

This batch adds **no behaviour**: nothing reads the new field yet, and `shedrun.ValidateDriver` still refuses `DriverLLM` exactly as it does today.
Batch 5 is where both validators start consulting it.
The external interface batches 4 and 5 consume is `loomcli.BootstrapVerb`, `battencli.BootstrapVerb`, and the `BootstrapVerb` field on `shedcli`'s table entry.

Batch-local decision beyond the overview's: the capability lives in **each module** rather than in one central map, and that placement is forced rather than chosen.
`internal/shedcli` already imports every module's cli package, which is its documented package-naming deviation under the CLI/Cobra Invariant — so the table can read the constants.
`internal/battencli` cannot import `shedcli` without a cycle, and batten's own `--driver` validator needs the same fact, so a single accessor on either side of that boundary would force one.
Two constants plus a synced field is the cheapest shape that keeps both sides compiling.

## Cards

### Card 5: loomcli.BootstrapVerb

- **Context:**
  - `internal/loomcli/start.go`
  - `internal/loomcli/cli.go`
- **Edits:** none
- **Creates:**
  - `internal/loomcli/bootstrapverb.go`
  - `internal/loomcli/bootstrapverb_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/loomcli/bootstrapverb.go` declaring one exported constant, `BootstrapVerb = "start"`, naming loom's own bootstrap verb — the command that reads this worktree's run seed and decides who drives.
  Its doc comment must state the contract the constant carries rather than merely restating its value: a recipe whose `BootstrapVerb` is non-empty has a command that reads its own run's seed at startup and can therefore honour `driver: llm`; a recipe whose `BootstrapVerb` is empty has no such site and cannot.
  Say that the value must equal the `Use` string of the cobra command `startCmd` builds in `start.go`, and that card 7's table field is populated from this constant so the fact is recorded once.
  Put it in its own file rather than in `cli.go` or `bootstrap.go`: `bootstrap.go` is the pure-decisions file whose doc comment scopes it to predicates and composers, and a bare capability declaration is neither.
  In `bootstrapverb_test.go` assert `BootstrapVerb` is exactly `"start"` and, separately, that it equals the `Use` field of the command `startCmd` returns — the second assertion is what catches a rename of the verb that leaves the constant behind.
- **Commit:** `feat(loomcli): declare loom's bootstrap verb as an exported constant`

### Card 6: battencli.BootstrapVerb

- **Context:**
  - `internal/battencli/cli.go`
  - `internal/loomcli/bootstrapverb.go`
- **Edits:** none
- **Creates:**
  - `internal/battencli/bootstrapverb.go`
  - `internal/battencli/bootstrapverb_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/battencli/bootstrapverb.go` declaring one exported constant, `BootstrapVerb = ""`, recording that batten has no bootstrap verb.
  Its doc comment must say why the empty value is a deliberate declaration rather than an unfilled placeholder: batten has no `lyx batten start`, so `lyx batten run <slug>`'s driver **is** the process the operator typed, and there is no spawn seam to branch on a seed's `driver`.
  It must also say that the constant is read by batten's own `--driver` validator directly, because `battencli` cannot import `shedcli` without an import cycle, and that the value changes only if batten grows a bootstrap verb — at which point `driver: llm` becomes supportable for batten with no edit to either validator.
  In `bootstrapverb_test.go` assert `BootstrapVerb` is the empty string.
  Write that assertion as an explicit equality against `""` with a comment naming it as the capability declaration under test, not as an `if len(...) == 0` liveness check — the point is that a future edit filling the constant in must break this test and force a human to confirm that batten really did grow a bootstrap verb.
- **Commit:** `feat(battencli): declare batten as having no bootstrap verb`

### Card 7: the recipe table's BootstrapVerb field

- **Context:**
  - `internal/loomcli/bootstrapverb.go`
  - `internal/battencli/bootstrapverb.go`
  - `internal/shedrun/seed.go`
  - `internal/shedcli/cli.go`
- **Edits:**
  - `internal/shedcli/table.go`
  - `internal/shedcli/table_test.go`
  - `internal/loomcli/wiring_test.go` (added mid-batch: batch loom-driver-config's own commits added
    a required `driver` key to `internal/loomengine/template.yaml` without updating this file's two
    hand-written nine-key-now literals, `seedLoomConfigWithInteractive` and
    `seedLoomConfigWithFriction`, which still wrote eight keys -- breaking this batch's own `verify:`
    scope, which runs `internal/loomcli`'s suite. Fixed here rather than left for a later batch since
    this batch's `verify:` is what surfaces it.)
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add a `BootstrapVerb string` field to the `entry` struct in `internal/shedcli/table.go`, and populate it in the `recipes` map literal from each module's own constant: the `loom` entry takes `loomcli.BootstrapVerb`, the `batten` entry takes `battencli.BootstrapVerb`.
  Name the constants — never spell `"start"` or `""` as literals in the map, which is the copy this card's own meta-test exists to catch going stale.
  The field's doc comment on the struct must say it answers "may a run seeded for this recipe be driven by an LLM", that it is populated from the owning module's exported constant, and that batch 5's `lyx shed seed --driver llm` validator consults it by emptiness rather than by comparing the recipe name against the literal `"loom"`.
  Nothing reads the field in this batch; that is deliberate and stated in the doc comment so a reader does not take the field for dead code.
  In `table_test.go` add the sync meta-test: iterate the `recipes` map and assert each entry's `BootstrapVerb` equals its own module's exported constant, in the shape of the existing key-set sync meta-test in that file.
  Add a second assertion that at least one entry has a non-empty `BootstrapVerb` and at least one has an empty one, so the table cannot degenerate to all-empty — which would make batch 5's validator refuse every recipe while every per-entry equality assertion still passed.
- **Commit:** `feat(shedcli): record each recipe's bootstrap verb on its table entry`

## Batch Tests

`verify: go build ./... && go test ./internal/loomcli/... ./internal/battencli/... ./internal/shedcli/...` runs the untagged suites of the three packages this batch touches plus a whole-module build.
All three are in scope because the fact this batch records is split across them by design; `shedcli` is where the drift shows up, and the two module packages are where the authoritative values live.

`table_test.go`'s sync meta-test is the mechanism this batch's whole shape rests on.
Without it the constants and the table are two hand-maintained records of one fact, and the failure mode is silent in the worst direction: a stale `""` copied into the `loom` entry would make batch 5 refuse `--driver llm` for the one recipe that supports it, with a message saying loom has no bootstrap verb — a refusal that reads as authoritative and is simply wrong.

The both-arms-present assertion is the one that is easy to leave out and expensive to omit.
Every per-entry equality check passes against a table where both entries are empty, and that table is exactly what a bad merge or an over-eager "initialise the new field" edit produces.
