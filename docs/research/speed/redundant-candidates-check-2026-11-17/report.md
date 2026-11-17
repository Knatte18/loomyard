# Check of five `lym gate redundant` candidates, 2026-11-17

A run of `lym gate test --wait --map` and `lym gate redundant` on `internal/boardengine` at `b904a1f8`,
the commit after `lym gate redundant` stopped counting a timed-out or memory-capped mutant run as a kill.
Five of the candidates were read by hand against the tests that cover the same blocks.
The trial changed no code and deleted no test; the defects it found are filed as notes.

## Cost

| | map build | `lym gate redundant` |
| --- | --- | --- |
| wall time | 10.2 s (build 4.9 s, run 4.6 s) | 22 min 0.7 s |
| peak resident size of the `lym` process | 313 MB | 2.1 GB |

`/usr/bin/time -v` measures the `lym` process, not its test binaries.
The map holds 136 tests, and its report says 135 were mapped.
The machine is shared with other lym sessions.

## Lists

`lym gate redundant internal/boardengine` printed 6 candidates, 8 unconfirmed, 29 refuted, 8 kept and 1 not judged (`TestSetOwners`, `unclassifiable call`).
It ran 1284 mutants, 52 unbuildable, in 30989 test runs and 1314 s of test time; 368 runs ended by timeout and 55 by the memory cap.

The trial before the fix listed 14 candidates.
Every one of the 6 candidates and 8 unconfirmed tests of this run is among those 14, and no other test joined them.
The 8 unconfirmed rest on 1 to 12 unjudged mutant runs each:
`TestBoardRefusesUnconfiguredLabel`, `TestListAndFindLabelFilter`, `TestMergeTasksSetStatusRollback`, `TestProjectDependencies`, `TestPromoteMakesNoteATaskAndIsIdempotent`, `TestRenderRecipeCellTakesTheDefaultRecipe`, `TestSetDeps` and `TestUpsertTaskDefaults`.
The 6 candidates are `TestConfigTemplate_ResolvesToDefaults`, `TestLoadConfig_DefaultRecipe`, `TestReadRaddleStore`, `TestRunHolds`, `TestRunLock_BeamKindAndBeamChange` and `TestRunLock_SketchExemptionIsTheWholeNetChange`.
Their seconds now run from 0.000003 to 0.000032.

## The five candidates

`TestLoadConfig_DefaultRecipe`, the sixth candidate, was not read.
Each verdict below is about the assertions, not the kills: a candidate is false when deleting it loses a check no covering test makes.

| candidate | covering tests | verdict | what it adds |
| --- | --- | --- | --- |
| `TestConfigTemplate_ResolvesToDefaults` | `TestFabricReplay` | false | the resolved template's `design_prefix`, both type descriptions and empty `labels`, and that the template resolves with an empty environment; the cover only writes the template and reads `README.md` |
| `TestReadRaddleStore` | `TestSlugRule_RaddleStore` | false, weakly | the decoded `Hash` and `Project`, a tolerated unknown key, and the exact `git -C <dir> checkout -- raddle/broken/store.json` command; the cover asserts the held slugs, the own Sketch and an error naming `store.json` and `checkout --` |
| `TestRunHolds` | `TestComputeLayers` | false | the producers `Worktree-Create`, `Beam-Up` and `Finalize`, a task or a non-beam project at a hold-free name, and a beam with no or a done status; the cover reaches the predicate through layers only |
| `TestRunLock_BeamKindAndBeamChange` | `TestRunLock` | false | a kind change or a beam change alone on a beam project; the cover's beam rows change the project, the recipe, or remove or merge the entry |
| `TestRunLock_SketchExemptionIsTheWholeNetChange` | `TestFabricReplay`, `TestMakeSketch`, `TestCheckRunLocks_FamilyProject` | false | the refusal of a Sketch that also changes the title, changes the body or keeps its run status; the covers pass a plain Sketch |

None of the five is a true redundancy: 0 of 5.
The weakest case is `TestReadRaddleStore`, whose only unique assertions are the decoded fields, the unknown key and the full command text.

## Why the mutation check passed them

Each candidate kills only mutants a covering test also kills, so the verb keeps it listed.
Two gaps explain the five:

- The operators change conditions and operators, never a string, list member or map entry.
  `TestConfigTemplate_ResolvesToDefaults` killed 1 mutant, and no operator touches the producer names in `holdFreeProducers`, which `TestRunHolds` pins.
- Negating a condition breaks every accepted row of a cover, so a clause of a boolean chain looks checked.
  Nothing deletes the `before.Kind != now.Kind` or `before.Beam != now.Beam` clause of `checkRunLocks`, and nothing in `TestRunLock` would fail.

## Defects filed as notes

- `redundant-mutants-skip-literals`: no mutant changes a string, list member or map entry.
- `redundant-clause-inputs-unfed`: no mutant deletes one operand of a boolean chain, so a clause only the candidate exercises goes unnoticed.

The report does not say how many of the 29 refuted tests are right, nor whether the 8 unconfirmed or the sixth candidate are deletable.
