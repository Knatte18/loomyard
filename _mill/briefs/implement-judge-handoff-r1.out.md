4 of 4 cards committed and verified — all cards in batch `02-judge-handoff.md` (Card 6, Card 7, Card 8, Card 9) are complete, matched against the commit log since the batch-start commit `49b78a69`.

{"status":"success","commit_sha":"f065d0e7","session_id":"f44e6169-28b3-4ebc-bd8e-c21dc2a6cf79","cards_done":[6,7,8,9]}

Summary of work:

- Card 6 (`ce405af4`): Added `internal/treadleengine/handoff.go` and `handoff_test.go` — the `Handoff`/`LedgerEntry` types and fail-loud `ParseHandoff` parser (TDD), mirroring `ParseJudgeVerdict`'s two-layer posture.
- Card 7 (`8349b51c`): Wired the handoff lifecycle into the round loop — `internal/treadleengine/roundfiles.go` (`Handoff` artifact path), `state.go` (additive `roundRecord.HandoffPath`, stale-move inclusion), `handoff.go` (`latestValidHandoff`, `judgeReadSet`), `judge.go` (`judgeInputs.PreviousHandoffPath`/`HandoffPath`, two-file `OutputFiles`), `run.go` (feeding the read-set walk into both judge call sites, `recordHandoffIfValid`). Also fixed mechanical test fallout in `judge_test.go`/`roundfiles_test.go`, after first stopping to record the plan-scope extension in `_mill/plan/02-judge-handoff.md` (commit `4d088a95`) per the brief's surprise-file protocol.
- Card 8 (`9496ef9b`): Extended `internal/treadleengine/judge-circling-template.md` and `judge-milestone-template.md` with the `previous_handoff`/`handoff_path` markers and the handoff-maintenance BLOCKING rules (lossless carry-forward, `covers_rounds` computation, exactly-two-files); updated `template_test.go`'s pins accordingly.
- Card 9 (`f065d0e7`): Added handoff-lifecycle scripted-shuttle tests in `internal/treadleengine/engine_test.go` (four new test functions covering all five required scenarios) and `state_test.go` (additive-field round-trip + legacy-resume); updated `internal/perchengine/run_test.go`'s one judge read-set pin to assert the handoff contract across two judge calls, and `internal/perchengine/doc.go`'s verdict-judge section to document the bounded read-set.

Final verify (`go test ./internal/treadleengine/... ./internal/perchengine/... ./internal/perchcli/... ./cmd/lyx/...`) passes clean with a cold test cache. Working tree has no uncommitted tracked changes (only the pre-existing untracked `_mill/briefs/implement-judge-handoff-r1.md`, out of scope).

{"status":"success","commit_sha":"f065d0e7","session_id":"f44e6169-28b3-4ebc-bd8e-c21dc2a6cf79","cards_done":[6,7,8,9]}
