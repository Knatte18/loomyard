MILL_REVIEW_BEGIN
# Review: shed: the LLM driver as a generic stepper and mender

```yaml
duration_s: 56.5
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewed_file: _mill/discussion.md
date: 2026-09-26
```

## Findings

### [BLOCKING:design] Stranded-branch delete names no repo or remote
**Section:** Decisions › repair-scope › Stranded-branch exception
**Issue:** `AppendRef` records a bare ref name with empty `Detail` (`internal/fabricengine/mutation.go` ~177, `add.go` ~157/~235, `weftwiring.go` ~133/~152), so the proposed `fabric: mutation` log line carries neither the repository (warp worktree vs weft repo) nor the remote a branch was created in or pushed to.
The exception prescribes `git branch -D <branch>` and `git push <remote> --delete <branch>` without saying where the driver runs them, and it admits weft branches as well as warp branches, although the coverage list sends stranded weft remotes to `lyx fabric cleanup --apply --remote`.
**Fix:** State how the driver determines the repository and remote for each raw delete: have the log line carry them, or restrict the exception to warp branches and name the source of the repo and remote.
A driver that cannot determine the repository or remote escalates.

### [NIT:design] Trace-id inheritance into reed strands unstated
**Section:** Decisions › trace-dir-and-trace-id
**Issue:** The lookup assumes every child `lyx` writes files carrying the step's `LYX_TRACE_ID`.
The discussion does not say whether that id reaches `lyx` processes started inside tmux strand panes, which inherit the tmux server's environment rather than the caller's.
**Fix:** State whether strand-pane `lyx` processes are expected to share the id, or that their traces are reached only through paths the parent trace names.

## Verdict

REQUEST_CHANGES
The stranded-branch exception's raw git deletes do not say which repository or remote they run against.
MILL_REVIEW_END
