All 4 cards are complete and verified. Final commit count: 3 commits committed 4 cards (card 4's scoutcli changes were folded into card 1's commit rather than getting a separate commit as the plan intended — a deviation from the "one commit per card" norm since cards 1 and 4 don't share files, but all edits are present, correct, and covered by the passing verify run).

Summary of work:
- Card 1 (commit `af03abfe`): deleted `internal/builderengine` and `internal/buildercli`, unregistered `builder` from `cmd/lyx/main.go` and `internal/configreg/configreg.go`, repaired `helptree_test.go`, `configreg_test.go`, `configcli_test.go`, `notransients_test.go`, `constructoranchoring_test.go`, `rawgitmutation_test.go` (narrowed to single-package walk), retargeted `internal/webstercli/sync_integration_test.go`'s builder fixture onto perch, retired the builder sandbox suite (`tools/sandbox/suite.go`, `main.go`, `SANDBOX-BUILDER-SUITE.md`, `sandbox/builder-suite.cmd`, S9 scenario in `SANDBOX-CORE-SUITE.md`), and updated `.gitattributes`. Card 4's `internal/scoutcli/cli.go` and `cli_test.go` edits were also folded into this same commit.
- Card 2 (commit `2376679f`): retargeted `internal/fabricengine/weftgit_exclude_test.go` and `trailer_test.go` builder fixtures to webster.
- Card 3 (commit `12a9f7d6`): renamed the loom `validPhases` entry, `preflight_integration_test.go` fixtures, and `perchengine/doc.go`'s `builder-review` gate name to `webster`.

Verify command passed in full: `go build ./...`, the untagged package set, and the integration-tagged `webstercli`/`loomengine` packages all green. Working tree is clean (`git status --porcelain --untracked-files=no` empty).

No live `status.json` with `phase: "builder"` was found outside the mill planning docs under `_mill/` (which are not "live worktree" status files per the Shared Decision's assumption).

{"status":"success","commit_sha":"12a9f7d692b1824d1d3e37e00822a6ec8a898509","session_id":"c9e8d9cc-6675-442c-8033-2ab9af34ce3d","cards_done":[1,2,3,4]}
