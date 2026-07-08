{"status":"success","commit_sha":"d737572a0abeba9d9aafe5bea350b6dc412f4da6","session_id":"9871f133-1af8-47c6-ae82-efa027086e53"}

## Summary

Processed the single NIT finding in `_mill/reviews/20260708-071605-code-review-r1.md` via the mill-receiving-review decision tree:

**Finding:** `burler run` could emit two JSON error envelopes on one invocation because `internal/burlercli/run.go` used cobra's `MarkFlagRequired("profile")`, whose validation error is raised by cobra itself after `PersistentPreRunE` returns but before `RunE`/`ShouldAbort` is consulted — routing through `clihelp.RunRoot`'s generic cobra-error-wrapping path rather than the `output.Err`/`clihelp.SetExit` path every other error in the command uses.

**Verify:** Factually accurate — confirmed by reading `internal/burlercli/run.go`, `internal/burlercli/cli.go`, `internal/clihelp/exec.go`, and comparing against `internal/shuttlecli/run.go` (which validates flag shape manually inside `RunE`, matching the finding's precedent).

**Harm check:** None — fixing this aligns burler's mechanism with the repo's established shuttlecli convention (manual flag-shape validation before touching the receiver, guarded before `ShouldAbort`). Action: **FIX**.

**Changes made** (single commit `d737572`):
- `C:\Code\loomyard\wts\internal-burler\internal\burlercli\run.go` — dropped `cmd.MarkFlagRequired("profile")`; added a manual `profilePath == ""` check at the top of `RunE` that reports via `output.Err`/`clihelp.SetExit`, mirroring `shuttlecli`'s `run.go` pattern exactly. Updated the doc comment to explain why.
- `C:\Code\loomyard\wts\internal-burler\internal\burlercli\cli_test.go` — updated `TestRunCLI_Run_MissingProfile`'s doc comment and assertion (now checks for `"--profile is required"` instead of the bare substring `"profile"`) to reflect the new manual-validation mechanism and the shuttlecli-style double-failure-line precedent.
- `C:\Code\loomyard\wts\internal-burler\_mill\plan\03-burler-cli.md` — updated the batch spec text (was: "required via `MarkFlagRequired`") to describe the corrected manual-validation approach, citing the holistic review round 1.

**Verify:** All batch `verify:` commands ran clean in order (engine-core, engine-round, burler-cli, registration-and-suite, docs-lifecycle), including the full `go build ./... && go test ./...`.

**Housekeeping self-check:** Baseline HEAD was `bd5e30b1f25271443a25e1378dcfe3737212b4d8` ("mill-go: holistic fix round 1"); final HEAD `d737572a0abeba9d9aafe5bea350b6dc412f4da6` differs and contains the real fix commit. `git status --porcelain --untracked-files=no` is clean (no uncommitted tracked changes).