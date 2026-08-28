3 of 3 cards committed — all cards (3, 4, 5) are complete, verify passed, and the working tree is clean.

Summary of changes:
- `/home/knatte/Code/loomyard/wts/logger-coverage-audit/internal/shedadapters/singlellm.go` — added `logger.Warn` to `mapOutcome`'s Died/Timeout and default branches.
- `/home/knatte/Code/loomyard/wts/logger-coverage-audit/internal/shedadapters/singlellm_test.go` — extended `TestSingleLLMProducer_OutcomeDiedAndTimeout` with an unrecognized-outcome case and log-capture assertions; added `TestSingleLLMProducer_CancelledDuringRun_DiedOutcomeEmitsNoWarn`.
- `/home/knatte/Code/loomyard/wts/logger-coverage-audit/internal/mergeresolve/mergeresolve.go` — added `logger` import and a `logger.Warn` call in `resolveConflicts` before `abortAndStuck`.
- `/home/knatte/Code/loomyard/wts/logger-coverage-audit/internal/mergeresolve/seam_enforcement_test.go` — added `internal/logger` to `mergeresolveAllowedImports` with doc-comment rationale.
- `/home/knatte/Code/loomyard/wts/logger-coverage-audit/internal/mergeresolve/mergeresolve_test.go` — extended `TestResolve_ShuttleOutcomes_MapToStuckNoConclude` with log-capture assertions; added `TestResolve_ShuttleOutcomes_WarnSurvivesAbortFailure`.
- `/home/knatte/Code/loomyard/wts/logger-coverage-audit/internal/websterengine/runlevel.go` — added `logger` import and `logger.Warn` calls to the four non-Done Master outcome branches (untested per plan's batch-local decision).

Commits: `564f9209`, `a4753724`, `566d9739`.

{"status":"success","commit_sha":"566d9739b05de04ce63acde5c9fdab9f8209cc86","session_id":"ef70df03-7a4f-4985-8bc8-93967ebd6408","cards_done":[3,4,5]}
