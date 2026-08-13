MILL_REVIEW_BEGIN
# Review: gitexec: add the checked entry point and migrate the call sites

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-13
```

## Findings

### [BLOCKING:design] Executor exit branch is control flow, not a message
**Section:** `destroy-executors-are-re-signatured` ("The eight call sites merge their two messages under the default merge rule")
**Issue:** At `internal/fabricengine/remove.go:220-245` and `prune.go:283-304` the `exitCode != 0` branch is not a second message — it re-probes `isRegisteredLinkedWorktree*` and, when still registered, performs a **fallback destructive `removePath`** and returns success; the `err != nil` branch bails without destroying anything. Under the default merge rule those two branches collapse, so either the fallback removal is dropped or an exec-level failure / `*destructiveRefusal` now reaches a destructive primitive.
**Fix:** State the rule for this class: at an executor call site the old `exitCode != 0` branch becomes `errors.As(err, &gitErr)` (git ran and refused), with non-`*GitError` and `*destructiveRefusal` keeping the bail path — and say which of the eight sites are plain merges and which are this shape.

### [NIT:consistency] Guard scan scope stated two ways
**Section:** `the-checked-call-invariant` vs Q&A log
**Issue:** The decision says the guard walks `internal/` **and** `cmd/` and explicitly rejects `internal/` alone; the Q&A entry answers "walking all non-test `.go` under `internal/`".
**Fix:** Correct the Q&A answer to match the decision.

### [NIT:design] Prior-call stderr composed into a later call's message
**Section:** `prior-exit-code-exception` / gitrepo table row for `pushWithRebaseRetry`
**Issue:** `internal/gitrepo/push.go:62` and `:65` render the **prior** `pull --rebase` stderr inside messages about the `rebase --abort` outcome. The stated exception covers a prior *exit code* only and names `readBranch` as the one live instance, so the default rule would silently drop `rebaseStderr` here.
**Fix:** Extend the exception to a prior call's stderr and name this site, so the earlier `*GitError` is kept in scope for the abort-branch messages.

### [NIT:consistency] Merge rule assumes `fmt.Errorf`/`%w`; some sites are string fields
**Section:** `default-merge-rule`
**Issue:** `prune.go:288/301` and `cleanup.go:291/295` assign `pe.Error`/`entry.Error` (plain `string`) via `fmt.Sprintf`, where `%w` is not available and the `(git exit %d)` fragment becomes unfillable.
**Fix:** Add one clause: at non-error string sinks the merged form is `%v` of the error, fragment dropped the same way.

## Verdict

REQUEST_CHANGES
One executor call-site class collapses control flow, not just messages; three nits.
MILL_REVIEW_END
