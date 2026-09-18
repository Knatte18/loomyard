MILL_REVIEW_BEGIN
# Review: fabric: no remote/GitHub branch deletion

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-09-18
```

## Findings

### [BLOCKING:consistency] RemoteSkippedReason never reaches either envelope
**Section:** no-origin-is-resolved-once-per-verb + Technical context ("Envelope") + Testing ("The no-origin path")
**Issue:** `okWithRecord`/`errWithRecordFields` emit only the caller's hand-built fields map (`envelope.go:29-33,54-68`); cleanup's map is `{"entries": r.Entries}` and remove's is `{"slug","path","links_removed"}`, so a verb-level field on `CleanupResult`/`RemoveResult` is never marshalled — yet the Testing section requires the reason to appear "exactly once in `remote_skipped_reason`" from both verbs. The Technical-context enumeration of map additions lists only `remote_branch_deleted`/`remote_branch_error` for remove, and claims cleanup's new fields "appear in the JSON automatically" — true only for `CleanupBranchEntry` fields, not for one beside `Entries`.
**Fix:** state that both fields maps gain `"remote_skipped_reason"` explicitly, at both helper exits.

### [BLOCKING:design] No error value named for the CLI failure path
**Section:** remote-failure-is-a-cli-failure
**Issue:** the engine verb returns nil by decision, but `errWithRecordFields(w, rec, err, fields)` dereferences `err.Error()` — the discussion never says what error the CLI synthesises, nor how N entries each carrying `RemoteError` aggregate into one string; a plan writer must invent observable CLI output.
**Fix:** decide the synthesised error's construction and its aggregation rule across failing entries (and note `refusal` is absent on this path, since `RefusalOf` will not match).

### [BLOCKING:consistency] Untagged gate test that must spawn git
**Section:** Testing → "`internal/fabricengine` gate tests — untagged"
**Issue:** the fourth bullet ("a branch still checked out at a worktree is refused with `CheckDirtiness`") reaches `checkBranchDirtiness` → `listWeftBranches` → `gitexec.Run` (`cleanup.go:222`), which Test Tier Purity bans in untagged files; the existing `destroy_test.go:540-563` covers only the two zero-declaration refusals for exactly that reason, and its own comment records why. The third bullet is safe only if the rejection is by naming, since `resolveManagedBranch` spawns git once past the name check (`destroy.go:566-583`).
**Fix:** classify each gate bullet by whether it reaches git, and move the checked-out case (and any non-naming ownership rejection) to an `integration`-tagged file.

### [NIT:design] Dry run vs the no-origin pre-check
**Section:** dry-run-does-not-probe-the-remote + no-origin-is-resolved-once-per-verb
**Issue:** `RemoteURL` is a local read, so it is not barred by the dry-run decision, but the discussion never says whether the pre-check runs — and whether `remote_skipped_reason` is populated — on `--remote` without `--apply`.
**Fix:** state the dry-run disposition of the pre-check explicitly.

### [NIT:scope] Spawn-observability left as an instruction, not a decision
**Section:** Constraints → Live-Substrate Spawn Observability
**Issue:** "Check what the existing `deleteBranch`/push paths do and match them" defers the decision to the plan writer rather than recording what the existing shape is.
**Fix:** name the logging shape the new `gitrepo` method inherits, or state explicitly that `gitexec`'s central logging already satisfies it.

## Verdict

REQUEST_CHANGES
Envelope carrier, CLI error value, and one untagged gate test need resolving before planning.
MILL_REVIEW_END
