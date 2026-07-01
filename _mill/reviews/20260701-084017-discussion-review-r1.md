Confirmed. Now composing the final review.

MILL_REVIEW_BEGIN
# Review: Add lyx init --undo / deinit command

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: C:\Code\loomyard\wts\lyx-deinit\_mill\discussion.md
date: 2026-07-01
```

## Findings

### [GAP] Undefined blast radius when a hard-error guard fires
**Section:** Decisions — "Junction removal safety" / "Junction target mismatch"; Testing — `TestRunInit_Undo_RealDirectoryGuard`/`TestRunInit_Undo_TargetMismatch`
**Issue:** When the real-directory or target-mismatch guard hard-errors, it's unspecified whether `--undo` aborts entirely (leaves weft-content/exclude/gitignore untouched) or still runs the other independent steps (per the "no separate pre-gate, each step checks its own target" model), producing a partially-reverted state alongside the reported error.
**Fix:** State explicitly whether a hard error short-circuits the whole `runUndo` before any other step runs, or whether independent steps still execute; the two cited tests only assert the junction/host-dir is untouched and say nothing about gitignore/exclude/weft-content in that scenario.

### [NOTE] Inaccurate premise: ".lyx/ is currently the only entry lyx ever adds"
**Section:** Decisions — "New gitignore helper: gitignore.Remove"
**Issue:** `internal/vscode/config.go` (`WriteConfig`, used by `ideengine`) also calls `gitignore.Ensure(dir, ".vscode/")` into the same shared managed block, so `.lyx/` is not the only entry ever added to it.
**Fix:** Correct the rationale; functionally harmless since `Remove`'s mirror design and the planned "entry present among others → block survives" test already handle the mixed-entry case correctly.

### [NOTE] weftcli commit help text should be re-checked, not just weftengine
**Section:** Constraints — CLI/Cobra Invariant; Technical context — `weftcli/cli.go`
**Issue:** `weftcli`'s `commit` subcommand `Long` text states "The commit message is always the fixed string \"weft sync\" ... cannot be customized with a flag" (`internal/weftcli/cli.go:140-141`); this stays literally true post-change (call sites keep passing `DefaultCommitMessage`, no new flag), but the invariant requires re-reading every affected Short/Long, and this file isn't named as one to re-verify.
**Fix:** Add `weftcli/cli.go`'s commit/push/sync Long text to the explicit list of help text to re-confirm during plan/review, even though no wording change is currently expected.

### [NOTE] docs/overview.md module-table line for `init` not addressed
**Section:** Constraints — Documentation Lifecycle
**Issue:** `docs/overview.md` has an existing "**init** — scaffolds the `_lyx/` directory structure..." bullet (line ~212) describing observable init behavior; the Documentation Lifecycle constraint here only discusses `docs/modules/` entries ("if one exists," and none currently exist for initcli/warpengine/gitignore), leaving unaddressed whether this overview bullet should mention `--undo`.
**Fix:** Confirm during planning whether the `init` bullet in `docs/overview.md` needs a one-line addition for `--undo`, per CLAUDE.md's "update docs/overview.md if the module table... changes."

## Verdict

GAPS_FOUND
One GAP: unspecified partial-failure/abort semantics when a hard-error guard fires during `--undo`.
MILL_REVIEW_END