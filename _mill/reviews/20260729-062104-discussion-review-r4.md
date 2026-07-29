MILL_REVIEW_BEGIN
# Review: board: move storage to weft:main

```yaml
verdict: GAPS_FOUND
reviewer_model: sonnetmax
reviewed_file: _mill/discussion.md
date: 2026-07-29
```

## Findings

### [GAP] Branch-name reuse for `_board` misattributes suffixWeftPrimaryBranch
**Section:** Decisions — "`_board` becomes a second weft worktree" / "Not a hardcoded main"
**Issue:** The text says `suffixWeftPrimaryBranch` derives `hostBranch` "via `git branch --show-current` on the host worktree," but `internal/fabricengine/clone.go` runs that call with cwd=weftPath (the weft clone, per the function's own doc comment: "reads the branch checked out at weftPath") before renaming it — never the host worktree — and `CloneHub` has no return value from it to reuse afterward.
**Fix:** Since `_board`'s worktree-add runs immediately after the rename, re-deriving via the same call against the weft worktree would wrongly read `<hostBranch>-weft`; state concretely whether `suffixWeftPrimaryBranch`'s signature changes to return `hostBranch` to `CloneHub`, or the value is independently read from the still-unmoved host worktree path.

### [GAP] Testing section contradicts promote-note's crash-safety decision
**Section:** Testing — `internal/boardengine` TDD candidates
**Issue:** The Testing bullet asks for a test asserting "partial-failure leaves both stores unchanged," but the Decisions section explicitly designs `promote-note` as duplicate-on-crash (tasks.json upserted, notes.json removal not yet run survives a crash, "not absent from both") — the two statements describe opposite on-disk outcomes for the same operation.
**Fix:** Split the Testing bullet into two named cases: a validation-failure test (mirrors `MergeTasks` — no write to either store) and a crash-between-saves test (asserts the documented transient-duplicate state plus idempotent-retry recovery, not "unchanged").

### [GAP] New board git-import guard's exec.Command ban risks a false positive
**Section:** Decisions — "Machine-checked guard for board's git-import boundary"
**Issue:** "contains no raw exec.Command/LookPath(\"git\") call" is ambiguous between `ghguard_test.go`'s actual same-line co-occurrence-with-quoted-"git" check and a bare-substring ban like the Test Tier Purity guard's; the bare-substring reading would fail on `spawn.go`'s own legitimate, explicitly-unchanged `exec.Command(exe, "board", ...)` self-relaunch call, which never mentions git, and on nothing in `boardtest/`'s git-based test helpers only because those are test files.
**Fix:** State explicitly that the new guard must match `exec.Command`/`exec.CommandContext` co-occurring with a quoted `"git"` argument on the same line (`ghguard_test.go`'s pattern), not a bare token ban.

### [NOTE] Scope's hubgeometry.go doc-update pointer names the wrong function
**Section:** Scope — doc updates in the same commit
**Issue:** The Scope bullet names "`BoardDir` comment" as needing an update for stale git-backend framing, but `BoardDir`'s own doc comment contains no such language; the actual "board passenger" phrase this task makes stale lives on `IsReservedHubName`'s doc comment instead.
**Fix:** Point the doc-update bullet at `IsReservedHubName`'s comment, or generalize it to "the hubgeometry.go doc comments touching `_board`" as the Technical Context section already does.

## Verdict

GAPS_FOUND
Three gaps in branch-name plumbing, promote-note test semantics, and the new git-import guard need resolution.
MILL_REVIEW_END
