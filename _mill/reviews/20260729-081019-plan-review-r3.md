MILL_REVIEW_BEGIN
# Review: board: move storage to weft:main — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetmax
reviewed_file: plan/
date: 2026-07-29
```

## Findings

### [BLOCKING] `ensureBoardWorktree`'s orphan git invocation likely misbinds its args
**Location:** Batch 2, Card 6 (`internal/fabricengine/boardweft.go`)
**Issue:** For the empty-weft-remote case, Card 6 specifies `git worktree add --orphan <hostBranch> <boardPath>` ("git ≥2.42 syntax — branch name before path"). `git worktree add`'s synopsis is `... [--orphan] [(-b | -B) <new-branch>] <path> [<commit-ish>]` — the only bare positionals are `<path> [<commit-ish>]`; naming the orphan branch requires `-b <branch>` alongside `--orphan` (omitting `-b` defaults the branch name to `basename(<path>)`). As literally written, `<hostBranch>` binds to the `<path>` slot and `<boardPath>` binds to the (disallowed-with-`--orphan`) `<commit-ish>` slot, so the command either errors outright or creates `_board` on a branch named after `basename(boardPath)` (i.e. `"_board"`) rather than the host's actual default branch — breaking the very case Card 9's `TestCloneHub_BoardWorktreeOrphanBranchOnEmptyWeftRemote` exercises, and with it fresh-hub bootstrap against a genuinely empty weft remote.
**Fix:** Change the Requirements text to `git worktree add --orphan -b <hostBranch> <boardPath>`; have the implementer confirm the exact form against `git worktree add --help`/`git-worktree(1)` for the targeted git version before writing the call.

### [NIT] `help_test.go`'s new table entries imply a struct-shape change that isn't stated
**Location:** Batch 4, Card 24 (`internal/boardcli/help_test.go`)
**Issue:** The existing `tests` table's anonymous struct has a `verb string` field, and the loop calls `runHelp(t, tt.verb)`. Card 24's new entries are shaped `{name: "notes upsert", args: []string{"notes","upsert"}, mustContain: [...]}` — an `args` field the current struct type doesn't declare, with no instruction to change the struct or the loop's `runHelp(t, tt.verb)` call to `runHelp(t, tt.args...)`. Taken literally this doesn't compile.
**Fix:** State explicitly that `verb string` becomes `args []string` (existing entries becoming one-element slices) and that the loop's call site changes to `runHelp(t, tt.args...)`.

### [NIT] `renderManifestSection`'s zero-notes output is unspecified
**Location:** Batch 3, Card 12 (`internal/boardengine/render.go`)
**Issue:** Requirements describe the per-note block shape in detail but not what the function returns when `notes` is empty — a bare "# Manifest" heading with nothing under it, or an empty string that omits the section entirely. No card's test coverage asserts on this specific case (existing zero-notes call sites like `TestCLIRerender` only check file existence).
**Fix:** State the empty-notes behavior explicitly (suggest: return `""` and have `Render` skip the blank-line separator, so a notes-free board's README carries no dangling "# Manifest" heading).

## Verdict

REQUEST_CHANGES
Fix Card 6's orphan worktree-add argument binding (BLOCKING); the two NITs are optional polish.
MILL_REVIEW_END
