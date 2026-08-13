MILL_REVIEW_BEGIN
# Review: gitexec: add the checked entry point and migrate the call sites

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Opus-class), Anthropic
reviewed_file: _mill/discussion.md
date: 2026-08-13
```

## Findings

### [BLOCKING:consistency] prune.go claimed by two conflicting carve-outs
**Section:** `merge-rule-at-non-error-string-sinks` vs `destroy-executors-are-re-signatured` shape (D) / `prior-call-diagnostic-exception`
**Issue:** The string-sink decision lists "`prune.go` (two assignments to `pe.Error`)" as merge sites that drop `(git exit %d)`, but the only git-derived `pe.Error` assignments in `prune.go` are `:280` (the exec-path bail that shape (D) says must stay a bail, not a merge), `:288` and `:301` (both explicitly assigned to `prior-call-diagnostic-exception`, which says their exit-code fragment is *kept* and filled from `gitErr.ExitCode`) — verified in `internal/fabricengine/prune.go:269-304`. So the "two prune.go sites" either do not exist or contradict the two decisions that own them, at the one call site where a wrong merge silently collapses the destructive-fallback split.
**Fix:** Restrict `merge-rule-at-non-error-string-sinks` to `cleanup.go`'s two `entry.Error` assignments (which do fit: `cleanup.go:291`/`:295`), and state that no `prune.go` sink takes it because shape (D) and the prior-call rule already claim all three.

### [BLOCKING:scope] Prose-correction inventory has no enumeration method
**Section:** `prose-corrections` / Technical context (Regeneration queries)
**Issue:** Every code inventory carries a regeneration grep, but the prose sweep is a hand-listed set (`gitrepo/doc.go`, `CONSTRAINTS.md`, "check `fabricengine/doc.go`") with no query — and it misses at least two live falsified statements: `docs/shared-libs/README.md:29` ("`internal/gitexec` — windowless `RunGit` primitive"), and `internal/fabricengine/destroy.go:679-684, 746-749, 782-793`, whose executor godocs document the `(exitCode, stderr, err)` shape, the `err == nil AND exitCode is zero` recording predicate, and the named-return rationale the re-signature deletes. Prose is not compile-checked, so a miss survives silently.
**Fix:** Add a regeneration query for the prose sweep (e.g. `grep -rn 'RunGit\|exit code\|exitCode' --include='*.md' --include='doc.go' docs/ manifest/ internal/ CONSTRAINTS.md`) and name `docs/shared-libs/README.md`, `destroy.go`'s three executor godocs, and `cmd/lyx/gitrepoboundary_test.go`'s "exactly one `gitexec.RunGit` call site" header comment as in-scope.

### [NIT:design] New guard's gitexec-side raw token is never stated
**Section:** `the-checked-call-invariant` (the paren paragraph)
**Issue:** The paragraph pins the `gitrepo` token as `r.run(` and then contrasts it with "`gitexec.Run`, where the paren is deliberately omitted so the shorter prefix covers both" — true of the three *token* guards, false for `checkedcall_test.go`, whose raw token must be `gitexec.RunGit` or it demands a marker at all ~57 migrated sites. Only the Testing section implies the right spelling, and `/mill-go` is told to write "this reasoning" into the new guard's header.
**Fix:** State the new guard's own raw tokens explicitly (`gitexec.RunGit` and `r.run(`) and scope the shorter-prefix rationale to the three token guards.

### [NIT:design] `pull --rebase` exit branch is control flow, not a message
**Section:** `prior-call-diagnostic-exception` (`gitrepo/push.go`)
**Issue:** At `push.go:55-68` the `rebaseCode != 0` branch gates a `rebase --abort` spawn; the exec path returns bare `err` and destroys nothing. The decision prescribes keeping the `*GitError` in scope for the two messages but never says what the non-`*GitError` branch does, so a mechanical `if err != nil` merge would run `rebase --abort` on an exec-level failure.
**Fix:** Say the site takes the same `errors.As` split as shape (D) — non-`*GitError` returns immediately without attempting the abort.

## Verdict

REQUEST_CHANGES
Two carve-outs claim prune.go's sinks; prose-correction sweep has no enumeration method.
MILL_REVIEW_END
