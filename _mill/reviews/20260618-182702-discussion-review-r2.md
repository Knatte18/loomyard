This reveals a feasibility/scope concern. `cmd/lyx/main_test.go` (lines 45, 75) and `internal/board/cli_test.go` (line 26) join `_lyx` but the touchpoint table covers main_test.go (lines 43, 73). Note line numbers differ slightly (45/75 vs 43/73) but that's minor. More importantly, `internal/board/cli_test.go` line 26 creates `_lyx` and the touchpoint table DOES list it (cli_test.go seedCwd). 

There's a notable interaction worth flagging: `PortalTarget` (paths.go line 141) and `menu.go` line 68 join `_lyx` directly (not `_lyx/config`), which is correct since those reference task-artifact `_lyx/`, not the config subdir. No issue.

Let me verify the one remaining concern: whether `config_test.go` has fixtures that use `_lyx/board.yaml` literally so the test migration is well-specified. I've seen enough. The discussion's touchpoint table is thorough and verified accurate. 

Let me compile the review. Key findings:

1. The `.git/info/exclude` vs committed `.gitignore` claim (Decision: Weft model) conflicts with current `lyx init` behavior which writes a committed `.gitignore` managed block — but this is explicitly task 006 scope. NOTE-level.

2. The word "container" appears in prose in CONSTRAINTS.md (lines 5, 18) and the Layout method list in overview.md/CONSTRAINTS.md — the discussion's "single token swap" claim for CONSTRAINTS.md is accurate for identifiers, but prose terminology consistency after `Container→Hub` rename is unaddressed.

MILL_REVIEW_BEGIN
# Review: Weft repo — companion-repo overlay for lyx

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-06-18
```

## Findings

### [NOTE] Prose "container" left after Container→Hub rename
**Section:** Technical context (CONSTRAINTS.md row) / Decisions: Hub/Prime
**Issue:** The rename targets the identifier `Container`, but lowercase prose "container" appears in CONSTRAINTS.md lines 5 ("worktree and container geometry") and 18 ("root, container, relative path"), and the paths.go package doc (line 1). The "single token swap" claim for CONSTRAINTS.md is accurate for identifiers but leaves prose using the old concept word.
**Fix:** State whether prose mentions of "container" become "hub" or stay as generic geometry language, so a plan writer doesn't guess.

### [NOTE] init.go .gitignore vs .git/info/exclude tension unstated
**Section:** Decisions: Weft model
**Issue:** The decision says junctions are listed in `.git/info/exclude`, "never in a committed `.gitignore`," yet current `lyx init` (internal/board/init.go line 103) writes a committed `.gitignore` managed block (`.lyx/`). The discussion does not note this is the existing pattern the weft model diverges from.
**Fix:** Add one line confirming the `.git/info/exclude` mechanism is task 006's concern and that task 005 leaves the existing `.gitignore` block untouched (verified: that block is `.lyx/`, not `_lyx/`, so no config-path conflict).

### [NOTE] init.go production-change scope under-specified vs tests
**Section:** Config path migration touchpoints
**Issue:** The table lists `internal/board/init.go` as creating `_lyx/config/` and writing both YAMLs there, but the row description only says "update file/package comment" prominently; init.go currently writes to `_lyx/board.yaml`/`_lyx/worktree.yaml` at top-level (lines 61, 82) and has no `_lyx/config/` mkdir.
**Fix:** Make the init.go row explicit: add `_lyx/config/` mkdir and retarget both `os.WriteFile` paths, not just the comment.

## Verdict

APPROVE
Scope, decisions, and touchpoints verified against source; only non-blocking terminology and specificity notes.
MILL_REVIEW_END
