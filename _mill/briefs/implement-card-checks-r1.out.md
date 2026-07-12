All 3 cards from `## Cards` in the batch file (`03.1`, `03.2`, `03.3`) match commits in the log by their exact `Commit:` subjects. 3 of 3 cards committed — this is a genuine, fully verified "all complete" claim, plus one additional plan-extension commit (`a431316`) required by the STOP-and-extend protocol for the `normalizeCardPath` fix discovered along the way.

Summary of work:

- **Card 10** (`fee2050`): Added `checkCardMissingField` and `checkCardFieldOverlap` to `internal/builderengine/validate.go`; extended `checkScopeMalformed` to also walk every card's five normalized file-op field lists. Discovered and fixed a real parser bug in `internal/builderengine/plan.go`'s `normalizeCardPath` (root `"."` produced an unclean `"./raw"` path instead of `raw`), which the new card-path `scope-malformed` check correctly surfaced against the `plan-valid` fixture and several `runlevel_test.go` cases. Corrected one pre-existing pinned assertion in `internal/builderengine/plan_test.go` that had enshrined the old buggy output, and added a regression test for `root: "."`. Plan file `_mill/plan/03-card-checks.md` was extended first (commit `a431316`) per the STOP-and-extend protocol since `plan.go`/`plan_test.go` weren't originally in card 10's `Edits:`.
- **Card 11** (`6935873`): Added `checkCardNumbering` (batch-prefix match + 1..M sequence) and `checkCardCountMismatch` (`IndexCardCount` vs `len(Cards)`) to `validate.go`, with tests in `validate_test.go`.
- **Card 12** (`cf3fd7a`): Added `checkPathMissing`, `checkCardOutsideScope` (reusing `inScope`/`pathCovers` from `digest.go`, per the batch-local decision), and `checkCommitSubjectMismatch` to `validate.go`; wired all checks into `Validate` in final order and rewrote the file banner comment to enumerate the complete v2 check set; reordered function bodies in the file to match that call order.

Verify (`go test ./internal/builderengine/...`) passes cleanly. Working tree is clean of tracked in-scope modifications (confirmed via `git status --porcelain --untracked-files=no`).

{"status":"success","commit_sha":"cf3fd7a","session_id":"b8931ac3-5a04-4b95-b317-643c70c4ffd9"}
