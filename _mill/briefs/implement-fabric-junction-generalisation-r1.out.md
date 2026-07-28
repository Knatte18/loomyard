All 7 cards (6-12) for batch `fabric-junction-generalisation` are complete and committed, plus two required plan-extension commits (following the "surprise file" escalation protocol for two same-package unit-test files needed to exercise multi-junction loop behavior while `HostJunctions` still returns a single entry). The batch's `verify:` command passed fresh (`-count=1`, no cache) across all five target packages: `internal/fabricengine`, `internal/initengine`, `internal/initcli`, `internal/loomengine`, `cmd/lyx`.

Card-count self-check: comparing `git log 065a86e1..HEAD --oneline` against the batch file's `### Card N:` `Commit:` messages — all 7 declared cards (6, 7, 8, 9, 10, 11, 12) have a matching commit subject in the log. 7 of 7 committed. No uncommitted tracked changes (`git status --porcelain --untracked-files=no` is empty).

Key files touched:
- `internal/fabricengine/junction.go` — materialised weft targets, reworded host-pristine refusal, generalised unwire to `unseedJunctionRecords`
- `internal/fabricengine/weftwiring.go` — generalised `removeHostJunction`/`removeJunctionRecords` (best-effort, opposite contract from unwire)
- `internal/fabricengine/reconcile.go`, `status.go`, `drift.go` — per-junction `checkJunctionHealth`/`PairInSync` with junction-named reasons
- `internal/initengine/undo.go`, `internal/initcli/initcli.go` — `JunctionsRemoved []string` cascade, `junctions_removed` JSON key (breaking change, documented)
- `internal/loomengine/preflight.go` — substring-match junction classification; also fixed a pre-existing Windows-only `t.Cleanup` LIFO-ordering bug in `preflight_integration_test.go` (confirmed pre-existing via identical file content on `main`)
- New test files: `internal/fabricengine/junction_pattern_integration_test.go`, `junction_test.go`, `weftwiring_test.go`, `remove_junctions_integration_test.go`
- `_mill/plan/03-fabric-junction-generalisation.md` — extended twice per the surprise-file protocol

{"status":"success","commit_sha":"53cf9a7b27373578b21bdc1603615c5f8c084c1a","session_id":"a6cf902b-cc03-4423-93c4-abfc627c3801","cards_done":[6,7,8,9,10,11,12]}
