All findings from the review have been processed:

1. **[BLOCKING:consistency]** raw `"_lyx"` literal in `hub_test.go:124` — FIXED. Replaced with `lyxdirs.LyxDirName` accessor, added the import.
2. **[NIT:consistency]** hardcoded `"_lyx"` porcelain-line literal in `states_test.go:150-152` — FIXED. Built from `lyxdirs.LyxDirName` for consistency.
3. **[NIT:consistency]** sabotage row 1 substitution in `doc.go` vs plan card 20 — the finding's own text states "No code change needed" and only suggests an optional plan-card annotation if the plan is ever amended (conditional, not mandated). No action taken, consistent with the finding's own disposition.

Swept the rest of `internal/fabricengine/fabrictest` for the same geometry-token literal classes (`filepath.Join` args, `+` operands, string consts) — no further occurrences found outside comments/doc text (comments are not covered by the ban).

Verified with `goimports -w` on both changed files, then ran every non-null `verify:` command from batch plan files 01 through 08 in order — all passed.

Files changed:
- `/home/knatte/Code/loomyard/wts/fabric-live-state-harness/internal/fabricengine/fabrictest/hub_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-live-state-harness/internal/fabricengine/fabrictest/states_test.go`

Baseline HEAD was `a37327e2cc3ed160093b5726dfcd0e745db49689`; new commit `58be89233b87a20b99ca8d9056f3d7054c9f4a3e` ("fabrictest: route raw _lyx literals through lyxdirs.LyxDirName in tests") was pushed to `fabric-live-state-harness`. Working tree is clean (no uncommitted tracked changes).

{"status":"success","commit_sha":"58be89233b87a20b99ca8d9056f3d7054c9f4a3e","session_id":"b139f2a6-313a-4c8b-8137-8ef464429fed"}
