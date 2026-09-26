MILL_REVIEW_BEGIN
# Review: shed: the LLM driver as a generic stepper and mender

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewed_file: _mill/discussion.md
date: 2026-09-26
```

## Findings

### [BLOCKING:design] Stranded-branch rule lets a push authorize deletion
**Section:** Decisions → `repair-scope`, stranded-branch exception.
**Issue:** The first condition admits `branch_pushed` as proof for either delete. But `recordPushIfAdvanced` (`internal/fabricengine/weftgit.go` ~287) records `branch_pushed` whenever the current branch of an existing repo is pushed forward. A step that only pushes commits to a live, pre-existing branch (a weft checkpoint push, for example) therefore authorizes `git branch -D` or a remote delete of that branch. This contradicts the same decision's "never deletes a branch … the trace does not name as created by the failed step".
**Fix:** Require `branch_created` for exactly that branch in every case. Also require `branch_pushed` for a remote delete, and state that a push-only entry never qualifies.

### [BLOCKING:design] Deletion check mixes up local and remote kinds
**Section:** Decisions → `repair-scope`, stranded-branch exception, second condition, and the "Stranded remote warp branch" coverage bullet.
**Issue:** Any later `branch_deleted` or `remote_branch_deleted` entry blocks both deletes. With a non-empty `branch_prefix`, `rollbackAdd` deletes the local warp branch, which records `branch_deleted` (`destroy.go` ~955). It leaves the remote copy on purpose (`add.go` ~277-281). So the #269 remote warp branch this exception exists to repair is refused.
**Fix:** Pair each kind with one side: a local delete is barred by `branch_deleted`, and a remote delete by `remote_branch_deleted`.

## Verdict

REQUEST_CHANGES
The stranded-branch exception's trace conditions allow deleting live branches and block the #269 remote case.
MILL_REVIEW_END
