# PATTERN-completion-signal

Every code path in `internal/shuttleengine` that finalizes a NEGATIVE answer to "did this run finish" consults `allOutputFilesExist` over the run's `OutputFiles` first.

- The negative answers are `OutcomeDied`, `OutcomeTimeout`, a mechanism-failure `error`, and `verdictRespawnEligible`.
- The check is reached directly or through the three helpers that own it: `classifyDeadlineExpiry` and `finishedDespiteMechanismFailure` (`wait.go`), and `soleFinishedCandidate` (`attach.go`).
- A clock expiring, `reed` losing a strand, `reed.Status` erroring, the events file staying unreadable, `reed.json` being absent or undecodable each answer "has something gone wrong", never "did this run finish".
  The run's output files are its return value.
- Stated at length in `wait.go`'s own file doc comment, under the same heading.
  Six instances were fixed across crucible rounds `opus5-high-r4` (both deadlines), `fable5-high-r5` (both retry-exhausted caps), `fable5-xhigh-r6` (`Attach`'s `dispositionCandidate`) and `opus5-high-r7` (`Attach`'s three reed-state gates).

## Enforcement

A **tripwire, not a completeness proof** (`internal/shuttleengine/completionsignal_enforcement_test.go`).
Two AST scans pin the audited negative-verdict return sites and the audited `allOutputFilesExist` call sites in `wait.go` and `attach.go`, so adding an exit or deleting a guard fails loudly and forces a human to confirm.
It is deliberately not a `shape.go`-style ledger, because that mechanism needs a closed value enum and these outcomes span three types in two files.
