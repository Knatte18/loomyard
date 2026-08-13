MILL_REVIEW_BEGIN
# Review: gitexec: add the checked entry point and migrate the call sites

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Opus-class, Anthropic)
reviewed_file: _mill/discussion.md
date: 2026-08-13
```

## Findings

### [BLOCKING:consistency] Scope carries pre-r3 gitrepo counts
**Section:** `## Scope` In-bullet 2, `## Testing` Tier 2, `## Q&A log` (last three entries)
**Issue:** Scope says "migrate 18 of 21 `r.run` call sites; leave 3 raw with marker comments", Testing says "the 18 migrated sites", Q&A says "(3 raw, 18 checked)" and "the map pins **4**" — all superseded by r3's re-filing of `HasUnpushed`, which the technical-context table and `the-checked-call-invariant` state as 2 raw / 19 checked and a pinned map of 3. Verified against the code: 21 `r.run(` sites, only `Pull`/`Fetch` (pull.go:19,33) qualify as raw class 2.
**Fix:** Restate Scope and Testing as 19 checked / 2 raw with 3 pinned, and mark the r1/r3 Q&A figures as superseded rather than leaving two live numbers.

### [BLOCKING:scope] `isWeftWorktree` does not exist in the tree
**Section:** `the-raw-vs-checked-discriminator` raw class 1, `bool-returning-predicates-stay-raw`, `### internal/fabricengine — the classes, by shape`
**Issue:** Both decisions name `internal/fabricengine/weftwiring.go`'s `isWeftWorktree`; grep over `internal/fabricengine` returns zero occurrences of that identifier. The two bool-returning predicates in that file are `weftRepoExists` (weftwiring.go:61, `rev-parse --is-inside-work-tree`) and `weftBranchExists` (:83) — the former is never named anywhere in the discussion.
**Fix:** Replace `isWeftWorktree` with `weftRepoExists` in both decisions and in the shape list, so the `internal/fabricengine` pinned count of 2 names the two sites that actually exist.

### [NIT:consistency] prune.go's fallback message deferred to a closed list
**Section:** `merge-rule-at-non-error-string-sinks` (Rejected), `prior-call-diagnostic-exception`
**Issue:** The first defers prune.go's fallback-failure message (`prune.go:301`, `remove weft worktree %q failed (git exit %d); fallback cleanup also failed: %v`) to `prior-call-diagnostic-exception`, but that decision states "**Two live instances**" and lists only `gitrepo/push.go` and `readBranch`; it never says where `%d` is filled from once the shape-(D) migration deletes `exitCode`.
**Fix:** Add prune.go:301 as a third instance and state that its code comes from the shape-(D) `gitErr.ExitCode` already recovered at that site.

### [NIT:design] Pinned-map semantics for an unlisted package
**Section:** `the-checked-call-invariant`
**Issue:** The map lists five packages including explicit zeros, but never says what the guard does when a raw site appears in a package with no map key — fail, or silently pass.
**Fix:** State that a missing key means zero and is a guard failure, so a raw site in a sixth package is caught.

## Verdict

REQUEST_CHANGES
Two blockers: stale pre-r3 gitrepo counts in Scope, and a named predicate that does not exist.
MILL_REVIEW_END
