MILL_REVIEW_BEGIN
# Review: fabric: accumulate the result envelope from mutations, not control flow (slice 14)

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opus
reviewer_self_id: Claude (Opus-class, Anthropic)
reviewed_file: _mill/discussion.md
date: 2026-08-11
```

## Findings

### [NIT:consistency] Mutation.Target: absolute vs hub-relative
**Demoted-from:** BLOCKING
**Section:** §Decisions `mutation-entry-shape` vs §Technical context (manifest paragraph)
**Issue:** The decision fixes `Target` as "a `filepath.ToSlash` absolute path", while the harness paragraph says it should match `DiffManifest`'s convention so the cross-check is "a direct comparison"; `CaptureManifest` (`fabrictest/manifest.go:133-137`) keys on `filepath.Rel(hubRoot, path)` slashed — hub-relative, never absolute.
**Fix:** Pick one convention in the decision text and state how a git-ref target and a non-hub-relative path (e.g. the clone's cwd-side paths) are represented under it.

### [BLOCKING:design] Recorder threading does not reach three auto-record sites
**Section:** §Decisions `gate-auto-records`
**Issue:** The recorder is said to be threaded "into the gate's request types (`pathRequest`, `branchRequest`)", but `repointLink(what, container, target string, own pathOwnership)` (`destroy.go:668`), `createExclusiveDir(path string)` (`:703`) and `createGitWorktree(repoDir, addArgs, target)` (`:721`) take neither request type — and the discussion explicitly puts all three in the auto-record set.
**Fix:** State the threading mechanism for the three non-request-shaped executors (extra parameter, request-type adoption, or a gate-level recorder field).

### [BLOCKING:design] The `Kind` enum is never enumerated
**Section:** §Decisions `mutation-entry-shape`, §Testing (`cmd/lyx` guard)
**Issue:** `Kind` is "a fixed enum string" and "precisely what a guard test can pin", but no member is named anywhere in the discussion — the eight gate executors plus the constructive verb sites (worktree/branch/junction/file) give no unambiguous mapping to kinds.
**Fix:** List the initial `Kind` members and the executor/success-site each maps to, plus the rule for adding one.

### [NIT:decision] destructiveCheck vs fabrictest.Check left undecided
**Demoted-from:** BLOCKING
**Section:** §Technical context (refusal type)
**Issue:** The text states that once `refusal.check` is emitted, fabrictest's string-backed `Check` becomes a second encoding of the unexported `destructiveCheck`, and that "whether the two are reconciled ... is a decision to make explicitly, not by default" — the discussion then never decides.
**Fix:** Record a `### Decision:` choosing export-and-consume vs deliberate parallel copies, with the `checkForce` non-membership rule carried into whichever wins.

### [NIT:decision] Relationship to PartialCommitError / PartialPullError unstated
**Demoted-from:** BLOCKING
**Section:** §Technical context ("Existing partial-failure types to align with, not replace")
**Issue:** The discussion says their relationship to `partial: true` "should be stated, not left implicit", then leaves it implicit — unclear whether a `PartialCommitError`/`PartialPullError` return forces `partial: true` independently of record emptiness, and whether `PartialPullError.Stage` surfaces in the envelope.
**Fix:** State whether `partial` derives solely from (error ∧ non-empty record) or also from these types, and whether `Stage` is emitted.

### [BLOCKING:design] Permitted-roots interaction with the truthfulness oracle undecided
**Section:** §Testing (fabrictest oracle, second bullet)
**Issue:** Whether permitted-root changes are excluded from the record↔diff cross-check or asserted positively is deferred to mill-plan as "a real design point to settle", yet it decides whether the `Remove` anomaly cell — the slice's headline case — asserts anything at all.
**Fix:** Decide it here: permitted roots suppress diff noise but the record must still name them positively (or the stated alternative), and say so in the decision list.

### [NIT:scope] `push`/`sync` record composition unspecified
**Section:** §Technical context (verb table), Q&A auto-pick on push/sync
**Issue:** `push` maps to three engine functions (`PushWeft` `weftgit.go:269`, `PushWarpAt`, `CoalescePushBothAt`) and `sync` is composed in `fabriccli/weft_verbs.go` from commit + push, so it is unclear which gain result types and how two records merge into one `mutations` array.
**Fix:** Name the result types introduced and the concatenation rule for the CLI-composed `sync`.

### [NIT:consistency] `ok` semantics depart from the design doc's requirement 2
**Section:** §Decisions `ok-semantics-and-error-path-fields`
**Issue:** `fabric-crucible-followups.md:406` states requirement 2 as "`ok` becomes a statement about that record plus the error, not a synonym for 'no error was returned'"; the decision keeps `ok` = "no error returned" without recording that it supersedes that requirement, and the same commit already edits that doc.
**Fix:** Note the supersession explicitly and amend requirement 2 alongside the `:419` consumer-claim correction.

## Verdict

REQUEST_CHANGES
Four deferred decisions and a path-convention contradiction must be settled before plan writing.
MILL_REVIEW_END
