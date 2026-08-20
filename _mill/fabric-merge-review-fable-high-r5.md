# fabric merge surface — independent review (fable-high-r5)

Round 5 of the fabric-merge crucible campaign (second instalment).
Clean-room review by a fresh agent; no prior-round review/fixer material read before this findings list was complete.

## Scope reviewed

- `internal/fabricengine`: merge.go, mergelifecycle.go, mergeerrors.go, mergeguards.go, mergestate.go, mergestage.go, mergepaths.go + tests
- `internal/gitrepo/merge.go` + tests
- `internal/fabriccli/merge_verbs.go`, `cmd/lyx` wiring
- Docs: `internal/fabricengine/doc.go` "# The merge surface", `docs/overview.md`, `CONSTRAINTS.md`, `README.md`
- SPEC: `git show 3b800bc8:_mill/discussion.md` (+ plan batch headings)

## What was tested

(appended incrementally as each command/scenario returns)

### Code reading pass

- Read the full SPEC discussion (decisions: recorded merge, two verbs, no-new-commit-until-clean, unified conflict paths, SHA-not-branch merges, aggregated side-free guards, conclude-never-rolls-back, weft-gated reset, correspondence, CLI mirror).
- Read all seven fabricengine merge production files, gitrepo/merge.go, fabriccli/merge_verbs.go, destroy.go's resetMergeSides, commit.go's sibling guard, doc.go lines 846–1046, mergevocab_test.go.

## Provisional findings (to be confirmed)

(jotted as spotted; graded and confirmed below before Job 2)

## Findings

(final graded list)
