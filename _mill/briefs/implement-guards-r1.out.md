All 4 cards of batch 8 (`guards`) are committed and the batch verify command passes cleanly. Card-count self-check: 4 of 4 declared cards committed (36, 37, 38, 39), confirmed by matching commit subjects against the batch file's `Commit:` messages in the range `13f3c541..HEAD`. No uncommitted tracked changes remain.

Summary of work:

- `/home/knatte/Code/loomyard/wts/loomyard.worktrees/native-clients/tools/sandbox/pathresolve_guard_test.go` (card 36) — replaced the unmatchable `exec.CommandContext("lyx"` literal with a line-based match pairing an exec spawn token with the quoted `"lyx"` argument on the same line; verified it now catches the `exec.CommandContext(ctx, "lyx", ...)` form.
- `/home/knatte/Code/loomyard/wts/loomyard.worktrees/native-clients/cmd/lyx/ghguard_test.go` (card 37, new) — module-root walk guard banning `gh` shell-outs outside `internal/githubclient`, using the same line-based token pairing plus `LookPath("gh")`, floor of 20 scanned files.
- `/home/knatte/Code/loomyard/wts/loomyard.worktrees/native-clients/cmd/lyx/tierpurity_test.go` (cards 38 & 39) — allowlisted `ghguard_test.go` and `gitrepoboundary_test.go` in `allowedSpawners`.
- `/home/knatte/Code/loomyard/wts/loomyard.worktrees/native-clients/internal/githubclient/githubclient_test.go` — reworded a doc comment that incidentally tripped the tier-purity/hermetic-env guards by mentioning `exec.CommandContext` in prose (no behavior change); this file was added to card 38's `Edits:` in the plan before touching it.
- `/home/knatte/Code/loomyard/wts/loomyard.worktrees/native-clients/cmd/lyx/gitrepoboundary_test.go` (card 39, new) — AST-based pinned-set guard for `internal/gitrepo`'s `r.run(`-containing methods plus a comment-stripped exactly-once `gitexec.` assertion scoped to `run`'s body; verified against three injected failures.
- `/home/knatte/Code/loomyard/wts/loomyard.worktrees/native-clients/_mill/plan/08-guards.md` — extended cards 38 and 39's `Edits:` lists (via the STOP-and-record procedure) before touching `internal/githubclient/githubclient_test.go` and `cmd/lyx/tierpurity_test.go` respectively.

{"status":"success","commit_sha":"e41647a0","session_id":"bf1edee4-1194-4de4-a9cd-36ba6155237d","cards_done":[36,37,38,39]}
