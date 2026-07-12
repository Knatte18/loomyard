Both cards from batch `01-fix-red-packages.md` are committed and verified: 2 of 2 cards done. Commit `a0b7039` matches Card 1's `test(initengine): derive TestInit_FirstRun module count from configreg`, and commit `8d9e912` matches Card 2's `fix(ideengine): set board path from hub geometry in Menu`. The verify command (`go test -tags integration ./internal/initengine ./internal/ideengine -count=1`) passed on both packages. Working tree is clean (no uncommitted tracked changes). Both commits were pushed to `origin/test-suite-regression`.

{"status":"success","commit_sha":"8d9e912","session_id":"ea0af731-2170-45fd-9104-0d267d10a0f2"}

Relevant files:
- `C:\Code\loomyard\wts\test-suite-regression\internal\initengine\init_test.go` — Card 1: replaced hardcoded `!= 3` module-count assertion with `want := len(configreg.Modules())`.
- `C:\Code\loomyard\wts\test-suite-regression\internal\ideengine\menu.go` — Card 2: added `cfg.Path = hubgeometry.BoardDir(l.Hub)` after `boardengine.LoadConfig` succeeds in `Menu`.

{"status":"success","commit_sha":"8d9e912","session_id":"ea0af731-2170-45fd-9104-0d267d10a0f2"}
