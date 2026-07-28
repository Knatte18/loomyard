MILL_REVIEW_BEGIN
# Review: native clients: migrate gitrepo to go-git + selfreport gh-CLI to go-github

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-28
```

## Findings

### [GAP] Reindex trigger stated two incompatible ways
**Section:** `linked-worktree-and-interop-evidence` (reactive policy; reindex-and-retry helper)
**Issue:** The policy bullet fires `Reindex()` only "on an `object not found` for a SHA the same handle otherwise reports as present", but the next bullet defines the shared helper as reindexing on *any* `object not found` — and for caller-supplied SHAs (`SHAExists`, `ChangedFilesSince`, `isStrictDescendant`) there is no "otherwise reports present" signal to condition on, so the qualifier is unimplementable there.
**Fix:** Pin one predicate for the helper, and say which lookups it applies to.

### [GAP] Reindex cost claim assumes misses are rare; SHAExists makes them routine
**Section:** `linked-worktree-and-interop-evidence` (Reindex concurrency / cost)
**Issue:** "Bounded by being on the miss path only" holds only if misses are exceptional, but `gitrepo/doc.go:62-74` documents `SHAExists` as the staleness probe callers run *expecting* an absent SHA after a rebase/amend/force-push, and `.scratch/gogit-probe-report.md:264` records that `Reindex()` rescans every `.idx` in the common dir with cost unmeasured on a hub sharing one object store.
**Fix:** State the acceptance criterion (e.g. reindex at most once per `Repo`, or skip it for a genuinely-absent caller-supplied SHA) so a routine absent-SHA check cannot pay a full index rescan under the shared mutex.

### [GAP] Reader locking does not deliver the stated reindex guarantee
**Section:** `lazy-cached-repo-handle` / `linked-worktree-and-interop-evidence` (Reindex concurrency)
**Issue:** The mutex is specified only for handle initialization plus the reindex-and-retry, so "no other reader observes half-reindexed storage" does not follow — a concurrent migrated read on the same `Repo` holds no lock; `internal/gitrepo/push_test.go:305-313` already drives two goroutines through one shared `*Repo`'s `PushCoalesced`→`hasUnpushed`, and go-git's `filesystem.Storage` builds its object index lazily on first read, so even reindex-free concurrent first reads touch mutated shared state.
**Fix:** Specify an RWMutex discipline (RLock spanning each go-git call, Lock for reindex) or record evidence that concurrent go-git reads on one handle are safe, and add `-race` to the verification step.

### [GAP] hasUnpushed's full-history walk has no perf bound on a hot path
**Section:** Technical context (`hasUnpushed` upstream `seenExternal` walk); Testing
**Issue:** The mandated implementation walks the upstream tip's **entire** ancestor set on every call with no early exit, and `PushCoalesced` calls it at each round/sync boundary (`boardengine/sync.go:68`, `fabricengine/weftgit.go:313,332`) — yet no cost is stated, and `.scratch/gogit-probe-report.md:329` says nothing in the probe speaks to repos larger than a handful of commits.
**Fix:** State an expected cost on a real-sized checkout (as the 310 ms `gh auth token` figure does) or a bounded shape (e.g. equal-hash shortcut / walk truncated at the HEAD frontier), plus what result would reverse the migration of this method.

### [NOTE] Q&A log contradicts the guard-allowlist decision
**Section:** Q&A log (review r3, guard allowlist) vs `constraints-github-auth-invariant`
**Issue:** The Q&A entry says the new `gh` guard "needs two allowlist entries, not one — its own scan exclusion plus a `tierpurity_test.go` entry", while the decision says the non-test filter already covers self-exclusion and the `allowedSpawners` entry is the **only** one needed (correct: `pathresolve_guard_test.go:47-50`).
**Fix:** Correct the stale Q&A answer so a plan writer does not add a dead self-exclusion entry.

### [NOTE] Boundary guard cannot see a direct gitexec call
**Section:** `constraints-gitrepo-boundary-invariant`
**Issue:** The invariant is worded as "any new `gitexec` call inside `gitrepo`", but the pinned-set guard keys on `r.run(` only, so a new `gitexec.RunGit` call added directly inside a migrated method satisfies the guard while violating the rule — a second blind spot beyond the one recorded.
**Fix:** Either add an assertion that `gitexec.` appears only in `run`'s definition, or narrow the invariant's wording to `r.run` calls and name this blind spot alongside the other.

## Verdict

GAPS_FOUND
Reindex trigger, locking, and hot-path walk cost need pinning before plan writing.
MILL_REVIEW_END
