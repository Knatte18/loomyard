MILL_REVIEW_BEGIN
# Review: fabric: cutover -- rewire consumers onto fabric, delete warp/weft

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-26
```

## Findings

### [GAP] fabric.md inbound-link count is incomplete
**Section:** Decisions §fabric.md deletion / Technical context §fabric.md inbound links
**Issue:** The discussion enumerates exactly "seven other docs" that link to `fabric.md`, but `crucible/gitrepo-review-prompt.md` (`:72,:152`) and `crucible/fabric-review-prompt.md` (`:58,:288,:417`) also link to it and are not in the list, so they become dangling refs after deletion.
**Fix:** Either add the two `crucible/*.md` files to the repoint list or explicitly scope `crucible/` out (as ephemeral review scaffolding) with a stated reason.

### [NOTE] Stale "weft" module-group comment in main.go not swept
**Section:** batch DAG §D / Testing §stale comment refs
**Issue:** `cmd/lyx/main.go:92` comment names "board, ide, reed, weft" as groups installing their own `PersistentPreRunE`; once `weftcli` is deleted this is stale, but it is a bare word (not the `internal/weftcli` full path), so it escapes both the scheduled comment-sweep list and the Tier-2 grep pattern.
**Fix:** Add `cmd/lyx/main.go:92` to Batch D's comment-sweep list so the "weft" mention is dropped when registration is removed.

## Verdict

GAPS_FOUND
Discussion is otherwise thorough and source-accurate; one incomplete link enumeration must be resolved.
MILL_REVIEW_END