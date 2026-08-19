{"status":"success","commit_sha":"5eeaeb9b2fe681b210f39d97a30f0a7bf70a01e7","session_id":"c468b743-3aa2-4e70-8ea7-31d05052286c","cards_done":[5,6,7,8,9,10]}

Summary of work (all paths absolute):

- `/home/knatte/Code/loomyard/wts/loom-session-bootstrap/internal/fabricengine/add.go` — Card 5: `Topology.Add` now writes and commits the origin record between junction wiring and the warp push, reusing the already-computed `parentBranch` local; `rollbackAdd`'s doc comment states the record needs no separate removal step.
- `/home/knatte/Code/loomyard/wts/loom-session-bootstrap/internal/fabricengine/launchers.go` — Card 6/7: `writeLaunchers` writes a third `run<ext>` script beside `ide`/`fabric-checkout`; `removeLaunchers`' hardcoded script list gains `"run"+ext`.
- `/home/knatte/Code/loomyard/wts/loom-session-bootstrap/internal/fabricengine/launcher_content_test.go` — pins the run launcher's exact command line/mode on both GOOS branches.
- `/home/knatte/Code/loomyard/wts/loom-session-bootstrap/internal/fabricengine/portallauncher_test.go` — extends the foreign-content preservation test to seed/assert the run launcher too.
- `/home/knatte/Code/loomyard/wts/loom-session-bootstrap/internal/fabricengine/origin_integration_test.go` (new) — Card 8: 9 integration tests covering non-default/subpath parent-branch recording, the commit landing on the weft branch's tree, `CommitWeftPaths` lock serialization, rollback on both created and adopted paths, mutation-record entry counts, the `SkipGit` guard, and the run launcher's add/remove lifecycle.
- `/home/knatte/Code/loomyard/wts/loom-session-bootstrap/internal/fabricengine/launchers_containment_integration_test.go`, `/home/knatte/Code/loomyard/wts/loom-session-bootstrap/internal/fabricengine/destroy_containment_toctou_integration_test.go` — added the run launcher's basename to their no-escape/canary lists.
- Card 9: ran `TestCrossProduct` (live-state matrix) at both anchors for the `Add` verb — all cells pass unchanged; confirmed `KindCommitCreated` is classified `false` (git-state, commission-exempt) in `livestate_mutationoracle_test.go`. No code change, no commit, per its `Commit: none` designation.
- `/home/knatte/Code/loomyard/wts/loom-session-bootstrap/manifest/designs/loom.md` — Card 10: rewrote the stale run-launcher paragraph to describe the shipped mechanism, keeping the cwd-authoritative/launcher-geometry links intact.

Verify: `go test ./internal/fabricengine/` and `go test -tags integration ./internal/fabricengine/` both pass. Working tree is clean (confirmed via `git status --porcelain --untracked-files=no`).

{"status":"success","commit_sha":"5eeaeb9b2fe681b210f39d97a30f0a7bf70a01e7","session_id":"c468b743-3aa2-4e70-8ea7-31d05052286c","cards_done":[5,6,7,8,9,10]}
