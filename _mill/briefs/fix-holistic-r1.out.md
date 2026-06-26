All verify commands pass and git status is clean (untracked mill artefacts are expected, not staged). Fix commit `416cc26` is on top of HEAD.

Summary of what was done:

**Finding processed:** [NIT] Dead-code helper `assertBranch` in `status_test.go`
- VERIFY: Accurate — `assertBranch` was defined at lines 313–322 of `/internal/warp/status_test.go` and confirmed by grep to be called nowhere in the package.
- HARM CHECK: No harm — pure dead-code removal.
- ACTION: FIX — deleted the `assertBranch` function body and removed the now-unused `gitexec` import. The `strings` import was retained because it is referenced by surviving test assertions. `goimports` confirmed the file is clean post-edit.

All 5 batch verify commands passed green.

{"status":"success","commit_sha":"416cc26","session_id":"282cccfe-28f8-4335-8d09-64b97185c8b6"}
