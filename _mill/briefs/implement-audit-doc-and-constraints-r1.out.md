Both of the batch's 2 declared cards are committed (2 of 2 cards committed — full completion, verified by matching commit subjects against the batch file's `Commit:` messages).

{"status":"success","commit_sha":"6e2e9d36e35b8030200c7a702f32f49bcc724925","session_id":"73c86155-bd6c-45da-b53c-1af14e0a1fb7","cards_done":[1,2]}

Summary of work performed:

- Created `/home/knatte/Code/loomyard/wts/logger-coverage-audit/manifest/designs/logger-coverage.md` — the audit document enumerating the spawn selector, hard-error selector, spawn-sites table, hard-error-return-sites table, the "detached spawns are spawn-only" and "what covered means here" subsections, the enforcement-asymmetry rationale, the untested-log-lines note for `runlevel.go`, and the structural-blocks section for `gitexec`/`gitkit`/`githubclient`. Committed as `8bfdb023`.
- Edited `/home/knatte/Code/loomyard/wts/logger-coverage-audit/CONSTRAINTS.md` — replaced the `## Live-Substrate Spawn Observability` section body with the verbatim sharpened text specified by the plan, keeping the heading unchanged and naming no test file. Committed as `6e2e9d36`.
- Verify command `go test ./internal/lyxcwd/ -run TestEnforcement_MarkdownLinks` passed.
- Both branch pushes to `origin/logger-coverage-audit` succeeded.

2 of 2 cards committed — all complete for this batch.
