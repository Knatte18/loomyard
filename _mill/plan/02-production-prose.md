# Batch: production-prose

```yaml
task: "Rename hub container suffix from -HUB to -LYXHUB"
batch: "production-prose"
number: 2
cards: 2
verify: go test ./internal/fabricengine/... ./internal/fabriccli/... ./internal/hubforge/... ./cmd/lyx/...
depends-on: [1]
```

## Batch Scope

The five production-source files that name the hub container suffix without declaring it: three doc comments describing the container directory, and the two lines of `lyx fabric clone` Cobra help text an operator actually reads.
No executable statement changes in this batch — every edit is inside a comment or a help string.

It is one batch because all five sites make the same claim about the same directory name, and because the CLI-help half needs the same reviewer attention as the comment half: it is the only operator-visible surface in the whole task.
It depends on batch 1 because the claims it makes are only true once the constants carry the new value.

Batch-local decision beyond `## Shared Decisions`: the help text gains no migration prose.
See `### Decision: Migration prose placement` in the overview.

## Cards

### Card 4: Update engine and hubforge container doc comments

- **Context:**
  - `internal/fabricengine/junctionnames.go`
- **Edits:**
  - `internal/fabricengine/clone.go`
  - `internal/hubforge/hub.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Three comment substitutions, `<name>-HUB` to `<name>-LYXHUB` in each.

  In `internal/fabricengine/clone.go`:
  - The `HubPath` field comment on the `CloneResult` struct, which reads `HubPath is the created <name>-HUB container directory.`
  - The `resetHub` doc comment sentence describing the R4 defect, whose second line begins "happened to be called" and then names the container directory shape, followed by "user content and all, on a flag whose help promises to".
    This sentence describes the shape of the path `resetHub` derives, which after batch 1 is the new suffix — it is a claim about how the code behaves, not a dated record of a past run, so it is substituted along with the code.

  In `internal/hubforge/hub.go`:
  - The `Path` field comment, which reads `Path is the hub root, the <name>-HUB container directory.`

  Read `internal/fabricengine/junctionnames.go` to confirm `HubSuffix` now carries the new value before making the claims.
  Change nothing executable in either file.
- **Commit:** `docs(fabricengine): update hub container doc comments to -LYXHUB`

### Card 5: Update the `lyx fabric clone` help text

- **Context:**
  - `internal/fabricengine/junctionnames.go`
- **Edits:**
  - `internal/fabriccli/fabric.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Two substitutions inside the `clone` command's `Long` string:
  - The opening line `Clone two repositories into a new hub directory (<parent>/<warp-name>-HUB)` becomes `Clone two repositories into a new hub directory (<parent>/<warp-name>-LYXHUB)`.
  - The later sentence fragment `that merely happens to be named <name>-HUB is reported and left alone.` becomes `that merely happens to be named <name>-LYXHUB is reported and left alone.`

  The `Long` text gains no new prose of any kind.
  The operator procedure for disposing of a container that still carries the retired suffix is recorded in the hub documentation and in the constraints file instead, because help text is read by every future operator forever, long after no such container exists anywhere.

  Leave the command's `Use`, `Short`, `Args`, and `RunE` untouched.
- **Commit:** `docs(fabriccli): update fabric clone help text to -LYXHUB`

## Batch Tests

`verify: go test ./internal/fabricengine/... ./internal/fabriccli/... ./internal/hubforge/... ./cmd/lyx/...` compiles and runs the untagged tests of every package this batch edits, plus `cmd/lyx`.

`cmd/lyx` is included because it is where the CLI help surface is assembled and asserted — `cmd/lyx/helptree_test.go` and `cmd/lyx/jsonhelp_test.go` walk the command tree and render help, so a malformed edit to the `Long` string surfaces there rather than in `internal/fabriccli` alone.
Neither test pins the exact `Long` body, so the substitution is expected to pass unchanged;
the run is a compile-and-render guard, not a golden-text assertion.

No new coverage is owed: every change in this batch is inside a comment or a help string, and no assertion in the repository reads either.
The three edited files carry no build tag, so the untagged run covers all of them.
