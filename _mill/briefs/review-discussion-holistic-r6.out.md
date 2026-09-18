MILL_REVIEW_BEGIN
# Review: fabric: no remote/GitHub branch deletion

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude Opus 5 (Anthropic)
reviewed_file: _mill/discussion.md
date: 2026-09-18
```

## Findings

### [BLOCKING:consistency] cleanup has no designed-refusal `Error` field
**Section:** remote-failure-is-a-cli-failure → "Consequence for the plan"; Testing → CLI tests (4th bullet)
**Issue:** The prescribed `doc.go` amendment ("cleanup's exemption covers its refusal fields — `Error` on a protected or unmanaged entry") is false against source: `cleanup.go:137-166` sets only `Protected: true` on protected/unmanaged entries and never touches `Error`, and `CleanupBranchEntry.Error` is documented at `cleanup.go:77` as "non-empty when apply is true and branch deletion failed". The designed-refusal `Error` text `doc.go:568-572` quotes ("commit them or re-run with --force", "fabric will not remove it") belongs to **prune** alone (`prune.go:205`, `prune.go:236`).
**Fix:** Restate the amendment in terms cleanup actually has (`Protected`, not `Error`), and replace the prescribed regression test — "an entry whose only non-empty field is a protected/unmanaged `Error` still exits 0" describes a state `Cleanup` cannot produce, so it is unwritable as stated.

### [BLOCKING:design] local-delete failure vs remote failure exit asymmetry unjustified
**Section:** remote-failure-is-a-cli-failure ("A protected or unmanaged entry, and a local-delete failure, keep exactly today's behaviour")
**Issue:** Given the finding above, cleanup's existing `Error` is itself a genuine "the verb failed at its job" outcome, not a designed refusal — so the discussion's own stated test puts it on the reconcile side too, yet the decision leaves it exit 0 while making `RemoteError` exit non-zero. Same verb, same entry struct, two verdicts for two failures of the same class, with no rationale.
**Fix:** State the disposition explicitly — either justify why the local-delete failure stays exit 0 despite the test, or key `errWithRecordFields` on both fields.

### [NIT:design] `attempted` in the synthesised cleanup error has no carrier
**Section:** remote-failure-is-a-cli-failure → the `fmt.Errorf` for `runCleanupWithFlags`
**Issue:** `attempted` is defined as "entries a remote deletion was actually tried for", but no result field carries it; the CLI would have to re-derive it (entries with `Deleted == true`, given `apply && remote && RemoteSkippedReason == ""`), a derivation the discussion never names.
**Fix:** Name the derivation rule, or add a verb-level count field, so the plan writer does not invent one.

### [NIT:consistency] "both stderr spellings" is not two spellings
**Section:** absent-remote-ref-is-success; Testing → `internal/gitrepo` bullet 2
**Issue:** The two quoted forms (`remote ref does not exist` and `error: unable to delete '<branch>': remote ref does not exist`) share the same substring, so a single `strings.Contains` matches both; "cover both stderr spellings git uses" gives the plan writer no actionable second case.
**Fix:** Either name a genuinely distinct wording to match, or say plainly that one substring covers it and the test pins that substring.

### [NIT:consistency] non-refusal gate errors would be reported as remote-push failures
**Section:** gate-runs-unchanged-for-remote
**Issue:** `checkRemoteBranchRequest` reuses `resolveBranchOwnership`, which spawns git (`primaryWeftBranch`, `listWeftBranches`); a spawn failure there returns a non-nil error that lands in `RemoteError` and produces the "remote branch deletion failed for N of M orphan branches" summary — attributing a local git failure to the remote. The discussion states no disposition for a refusal or gate error escaping the remote executor.
**Fix:** Say how a `*destructiveRefusal`/gate error from the remote executor is distinguished from a push failure in `RemoteError`.

### [NIT:scope] `Use:` and usage strings not listed among the doc updates
**Section:** Scope → In (doc updates bullet); Technical context → CLI
**Issue:** Scope names the two `Long` texts but not `cleanupCmd.Use` (`"cleanup [--apply] [--force]"`, `fabric.go:331`), `removeCmd.Use` (`fabric.go:189`), or `runRemoveWithFlag`'s `"usage: lyx fabric remove [--force] <slug>"` (`fabric.go:802`) — all of which the help-tree tests see.
**Fix:** Add them to the doc-update inventory.

## Verdict

REQUEST_CHANGES
The cleanup carve-out rests on a field `cleanup` does not populate; two consequences follow.
MILL_REVIEW_END
