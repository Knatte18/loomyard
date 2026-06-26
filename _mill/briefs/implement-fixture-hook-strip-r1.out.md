Perfect! The batch has been successfully implemented. All tests pass and the commit has been pushed.

## Summary

I've successfully implemented Card 1 of the fixture-hook-strip batch:

**Changes made:**
- Added `stripHookSamples(hooksDir string)` helper function that globs for `*.sample` files and removes them using `os.Remove`, with best-effort error handling
- Called `stripHookSamples` from `initRepo` after git config setup, passing `.git/hooks` directory
- Called `stripHookSamples` from `initBareRemote` after `git init --bare`, passing `hooks` directory (bare repos have hooks directly in the repo root)
- Used only stdlib imports (`os`, `filepath`) to preserve the lyxtest leaf invariant

**Verification:**
- Unit tests pass: `go test ./internal/lyxtest/`
- Integration test passes: `go test -tags integration -run TestList ./internal/warp/`

**Commit:**
- Hash: `ae7395274686dbde1d38fa7cec73e89d7ab6a638`
- Message: `perf(lyxtest): strip inert git hook samples from fixture templates`

{"status":"success","commit_sha":"ae7395274686dbde1d38fa7cec73e89d7ab6a638","session_id":"c16967e3-2e00-4331-9af1-84e159082653"}
