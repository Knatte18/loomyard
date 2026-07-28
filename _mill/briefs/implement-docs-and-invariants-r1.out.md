All 7 cards for batch `docs-and-invariants` are complete: 6 commits (cards 40-45) plus card 46's verification-only requirements performed this turn (no commit expected). Working tree is clean.

**Card-count self-check:** 6 of 6 commit-bearing cards (40-45) are present in `git log 1a90b59e..HEAD --oneline`, matching their declared `Commit:` messages exactly. Card 46 (`Commit: none`) had its Requirements verification performed this turn. **All 7 of 7 declared cards are complete.**

Summary of work:

- `/home/knatte/Code/loomyard/wts/loomyard.worktrees/native-clients/CONSTRAINTS.md` — added the GitHub Auth Invariant and gitrepo Client Boundary Invariant entries.
- `/home/knatte/Code/loomyard/wts/loomyard.worktrees/native-clients/internal/gitrepo/doc.go` — rewrote the gitexec-relationship section for the two-backend boundary, rewrote the locale paragraph, added an exhaustive evidence section drawn from `_mill/discussion.md` (both probe reports themselves are gitignored and no longer present on disk, so discussion.md was the durable source for the evidence).
- `/home/knatte/Code/loomyard/wts/loomyard.worktrees/native-clients/internal/githubclient/doc.go` (new) and `githubclient.go` — full package design record; `internal/selfreportengine/selfreport.go` needed no edit (already rewritten in the selfreport-transport batch).
- `/home/knatte/Code/loomyard/wts/loomyard.worktrees/native-clients/docs/overview.md`, `README.md`, `manifest/roadmap.md` — corrected gitrepo/selfreport claims, dropped the dangling `gitnativepoc` entry, added `githubclient`, rewrote Done entries.
- Deleted `/home/knatte/Code/loomyard/wts/loomyard.worktrees/native-clients/manifest/designs/native-clients-migration.md` per the Documentation Lifecycle.
- Card 46 (verification-only): `go build ./...`, `go vet ./...` clean; batch's own `verify:` (`./cmd/lyx/... ./internal/gitrepo/... ./internal/githubclient/...`) passes; all named constraint guards (`tierpurity_test.go`, `hermeticenv_test.go`, `sandbox_coverage_test.go`, `ghguard_test.go`, `gitrepoboundary_test.go`, githubclient's `leaf_enforcement_test.go`) pass explicitly.

**Known pre-existing failures found during the whole-repo Tier-1/Tier-2 run (out of this task's scope):** `internal/reedengine` (`TestTmuxProcessName`), `internal/buildercli` and `internal/webstercli` (`TestWeftCommit_*`), and `internal/builderengine` (`TestImplementerTemplate_StatesBatchDiscipline`) fail. I investigated thoroughly (including a controlled shell/Go reproduction) and confirmed these are caused by `main` having advanced past this branch's fork point with unrelated commits — a `-f`-flag fix to `gitrepo.StageAndCommit`'s pathspec-magic handling, a Windows-path-parsing fix in `internal/reedengine/proctree.go`, and a PATTERN-directive removal in `internal/builderengine` — none of which any batch in this task (1-9) touches (verified via `git diff main -- <pkg>` showing these are main-only changes, and `git log main..HEAD -- <pkg>` showing zero task commits in any of the four affected packages). This is branch staleness relative to `main`, not a regression from this task; fixing it would require merging `main` forward into this branch, which is out of scope for a docs batch and not something I did unilaterally.

commit_sha: b383d07b6bbb75e7fb1a7e18c14bb9d9e13aabf6 (verify by `git -C /home/knatte/Code/loomyard/wts/loomyard.worktrees/native-clients rev-parse HEAD`)

{"status":"success","commit_sha":"b383d07b","session_id":"6405fdce-bb27-411b-afd9-3e98fe3be5e0","cards_done":[40,41,42,43,44,45,46]}
