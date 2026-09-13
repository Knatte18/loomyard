# Batch: sandbox-fixture

```yaml
task: "Rename hub container suffix from -HUB to -LYXHUB"
batch: "sandbox-fixture"
number: 5
cards: 2
verify: go test ./tools/sandbox/...
depends-on: [1]
```

## Batch Scope

The sandbox fixture hub is a real directory produced by the real clone path, so its name must follow the constant.
This batch renames it in the one Go constant that declares it, fixes the one comment that quotes that constant's value, and updates the seven operator-facing suite documents that name the fixture container in a path.

It is one batch because the Go constant and the seven documents assert the same fixture name, and letting them drift is precisely the fidelity loss the sandbox exists to prevent.
No test in the package carries the literal — every sandbox test builds its paths from the constant — so the Go half is a one-line change with a package-wide blast radius that the package's own suite covers.

Batch-local decision beyond `## Shared Decisions`: the manual-removal procedure for a fixture container still carrying the retired suffix is recorded in the sandbox documentation, not in these suite files.
See `### Decision: Migration prose placement` in the overview.

## Cards

### Card 16: Rename the sandbox fixture container constant

- **Context:**
  - `internal/fabricengine/junctionnames.go`
  - `tools/sandbox/main_test.go`
- **Edits:**
  - `tools/sandbox/main.go`
  - `tools/sandbox/suite.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `tools/sandbox/main.go`, change the `hubName` const from `"lyx-test-HUB"` to `"lyx-test-LYXHUB"`.
  This is the name the sandbox resolves its fixture container through, and after batch 1 it is also the name the real clone path produces for the `lyx-test` warp — the two must agree for the suite to exercise the real layout.

  In `tools/sandbox/suite.go`, the `warpDirName` doc comment describes the subdirectory under the hub and names the fixture container in passing.
  Substitute the new name there.
  The comment is the only hit in the file;
  `warpDirName`'s own value is unrelated to the suffix and does not change.

  Read `tools/sandbox/main_test.go` first to confirm the claim that the package's tests derive every hub path from `hubName` rather than from a literal, so this one-line change is sufficient for the whole package.
- **Commit:** `refactor(sandbox): rename the fixture hub container to lyx-test-LYXHUB`

### Card 17: Update the sandbox suite documents

- **Context:**
  - `tools/sandbox/main.go`
- **Edits:**
  - `tools/sandbox/SANDBOX-BURLER-SUITE.md`
  - `tools/sandbox/SANDBOX-CORE-SUITE.md`
  - `tools/sandbox/SANDBOX-FABRIC-SUITE.md`
  - `tools/sandbox/SANDBOX-REED-SUITE.md`
  - `tools/sandbox/SANDBOX-REED-WATCH-SUITE.md`
  - `tools/sandbox/SANDBOX-SHUTTLE-SUITE.md`
  - `tools/sandbox/SANDBOX-WEBSTER-SUITE.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Nine substitutions across the seven files, each replacing the retired suffix with `-LYXHUB`.

  All seven files carry one hit of the same shape: the bolded sentence stating that the agent under test works exclusively inside the Hub's repository, which names the path `lyx-test-HUB/lyx-test`.
  Substitute the container name in each.

  Two files carry a second hit:
  - `tools/sandbox/SANDBOX-CORE-SUITE.md` — the setup step describing the build launcher cloning the warp and weft into a fresh fixture container names that container by name.
  - `tools/sandbox/SANDBOX-FABRIC-SUITE.md` — the reset scenario instructs the operator to create a directory named after the derived container plus a nested file inside it, then run the clone verb with the reset flag there.
    The whole point of that scenario is that the derived path must match what the code derives, so the container name in it moves with the constant.

  Follow the repository's semantic-line-break markdown rule: where a substitution lengthens a line, do not re-wrap the paragraph at a fixed column.
  Break only at a sentence end or an internal independent-clause boundary, and only if the line genuinely needs it.
  Add no migration prose to these files.
- **Commit:** `docs(sandbox): update suite documents to the -LYXHUB fixture container`

## Batch Tests

`verify: go test ./tools/sandbox/...` runs the package's five test files.
None of them carries the suffix literal — every one derives its hub paths from `hubName` — so this run is exactly the check card 16 needs: it proves the constant's new value flows through `main_test.go`, `suite_test.go`, `report_test.go`, `resolve_test.go`, and `pathresolve_guard_test.go` without any of them having pinned the old value behind the planner's back.

The seven suite documents have no runnable surface;
they are operator scripts read by a human agent, not executed.
Their correctness is the substitution itself, reviewed rather than tested.

The sandbox suite is not run here and is not part of `pipeline.done_gate` either: it is manual, operator-driven, and needs a real cloned fixture container on disk.
Re-cloning that container is the operator's own step after this task lands, and per `### Decision: No physical rename` in the overview it is a re-clone plus a manual removal of the old container, never a rename.
