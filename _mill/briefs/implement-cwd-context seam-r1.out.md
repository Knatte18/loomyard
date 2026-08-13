{"status":"success","commit_sha":"625c33a6378ddf0ccec6edb9e26455fea924bde0","session_id":"98514e89-86b3-4f46-bb1e-6323bc94cf4f","cards_done":[1,2,3,4,5]}

All 5 of 5 cards committed this turn. Card-count self-check: 4 commits cover 5 cards (cards 2 and 3 combined into one commit, `d0943070`, per the batch's explicit "one combined commit" allowance since both touch `internal/clihelp/exec.go`/`exec_test.go`). Verify (`go test ./internal/lyxcwd/... ./internal/clihelp/... ./cmd/lyx/...`) passed on the first attempt with no self-fix needed.

Files touched:
- `/home/knatte/Code/loomyard/wts/hubforge-parallel-chdir/internal/lyxcwd/lyxcwd.go` (doc comment extension)
- `/home/knatte/Code/loomyard/wts/hubforge-parallel-chdir/internal/lyxcwd/cwdcontext.go` (new: `WithCwd`/`CwdFrom`)
- `/home/knatte/Code/loomyard/wts/hubforge-parallel-chdir/internal/lyxcwd/cwdcontext_test.go` (new)
- `/home/knatte/Code/loomyard/wts/hubforge-parallel-chdir/internal/clihelp/exec.go` (`RunRootCtx`, `ExecuteIn`, `WrapRunCtx`)
- `/home/knatte/Code/loomyard/wts/hubforge-parallel-chdir/internal/clihelp/exec_test.go` (new test cases)
- `/home/knatte/Code/loomyard/wts/hubforge-parallel-chdir/cmd/lyx/seamsignature_test.go` (new)
- `/home/knatte/Code/loomyard/wts/hubforge-parallel-chdir/CONSTRAINTS.md` (Cwd Resolution Invariant + CLI/Cobra Invariant amendments)
- `/home/knatte/Code/loomyard/wts/hubforge-parallel-chdir/docs/overview.md` (Module dispatch paragraph)

{"status":"success","commit_sha":"625c33a6378ddf0ccec6edb9e26455fea924bde0","session_id":"98514e89-86b3-4f46-bb1e-6323bc94cf4f","cards_done":[1,2,3,4,5]}
