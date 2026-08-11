MILL_REVIEW_BEGIN
# Review: fabric: live-state integration harness (slice 13) — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnetxhigh
reviewer_self_id: Claude Sonnet 4.5 (Anthropic)
reviewed_file: plan/
date: 2026-08-11
```

## Verification performed

Read the overview and all 8 batch files, then cross-checked the plan's specific claims against
current source: `fabriccli/fabric.go` and `fabriccli/clone.go` (card 1's extraction target),
`destroy.go` (Check constants/String()/Error() format, checkPathRequest/checkPathDirtiness, the
byte-identical dirtiness message at `:564`), `remove.go`/`slug.go` (the `remove ..` pre-flight vs.
gate-containment distinction), `checkout.go`, `prune.go`, `cleanup.go`, `add.go`, `pull.go`,
`reconcile.go`, `junction.go`, `weftwiring.go`, `launchers.go`, `clone.go` (every cited line number
for the dirtiness-scope table, the `CloneHub{Reset}` re-scoping argument, and the
`trackedSymlinkAtWiredPath`/`staleWiredJunction` Add-omission reasoning), `config.go`/`topology.go`/
`junctionnames.go`/`template.yaml` (confirming `fabricengine.Config{}`'s zero value matches the
real default `branch_prefix`/`pathspec` a fresh `CloneAndWire` commits, and that
`NewTopology(fabricengine.Config{...})` with a hand-built `Config` is the pre-existing pattern in
`destructivegaps_integration_test.go`/`add_rollback_adopt_test.go`, not a novel departure),
`internal/lyxcwd/enforcement_test.go` and `cmd/lyx/destructiveguard_test.go` (confirming the exact
maps/lists cards 2-3 edit, and that the walk currently has no directory-skip so card 3's exclusion
is load-bearing), `internal/boardengine/boardtest/doc.go`+`testmain_test.go` (confirming the exact
shape cards 5-6 mirror), `internal/lyxtest/lyxtest.go`+`hermetic.go` (confirming
`buildWarpHub`/`buildWeftPrime`/`initRepo`/`initBareRemote` exist as claimed), `fslink_windows.go`
(confirming `CreateDirLink` writes an `IO_REPARSE_TAG_MOUNT_POINT`, i.e. a junction, backing the
Windows-gap doc claim), and the duplicate `gitStatusPorcelain` in both `weftgit_exclude_test.go` and
`commitweftat_test.go` that card 9 folds/leaves in place.

Every line-number citation, function signature, and error-message substring checked (several dozen,
spanning all nine gate-reaching verbs) matched the current source exactly. The 168-cell arithmetic
(160 ordinary − 30 structural omissions + 34 hostile-input + 4 Reset-column = 168) is internally
consistent. The Batch Index DAG has no cycle, no undeclared batch reference, and every `file:` exists
in the plan directory. Global step numbering (cards 1–22) is sequential with no gaps. `All Files
Touched` is the exact union of every card's `Edits:`/`Creates:` across all 8 batches (21 files,
verified one-by-one). Every card's `Moves:` is `none`, correctly triggering no Rename-mechanic
requirement. `Context:`/`Edits:` lists were spot-checked against the specific functions/constants
each card's `Requirements:` names (including the file-dense card 16) and were complete in every case
examined, including card 1's extraction, whose target functions are already called verbatim inside
`internal/fabriccli/clone.go` — the very file in `Edits:` — so no cold-start exploration is implied
despite those functions being declared elsewhere.

No constraint violation, decision-alignment gap, sequencing error, or context-completeness gap was
found. The plan is unusually precise in its source citations for a document at this level, and I was
unable to find a load-bearing inaccuracy in any of them.

## Verdict

APPROVE — plan verified precise against source on every checked claim; no findings.
MILL_REVIEW_END
