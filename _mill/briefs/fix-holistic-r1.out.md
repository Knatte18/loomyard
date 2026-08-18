HEAD is `ed29dd1b5401bb54c29df4eb8c79a0d8793e1072`, distinct from baseline `fece0e74d14672cd0fb1bca9130c50a674bf6678`, and no uncommitted tracked changes. All verify commands passed.

{"status":"success","commit_sha":"ed29dd1b5401bb54c29df4eb8c79a0d8793e1072","session_id":"f36a1f0b-422a-45c1-89bd-c627f1a0cec5"}

Summary: Fixed the single NIT:consistency finding in the holistic review — `Check`'s doc comment in `/home/knatte/Code/loomyard/wts/orchestrator-preflight/internal/preflight/preflight.go` overstated the nil-Location guarantee on error. Reworded to scope the nil-Location claim to `lyxcwd.Resolve` failures and note that when `Resolve` succeeds but the downstream `CheckResolved(l)` call fails, `Check` returns `(Report{}, l, err)` with a non-nil Location. Committed as `ed29dd1b`. All five batch verify commands (buildinfo/stencilstore/lyxcwd, standalonestate, preflight/loomengine, cmd/lyx/tools-deploy, lyxcwd) passed, including the `-tags integration` variants.

{"status":"success","commit_sha":"ed29dd1b5401bb54c29df4eb8c79a0d8793e1072","session_id":"f36a1f0b-422a-45c1-89bd-c627f1a0cec5"}
