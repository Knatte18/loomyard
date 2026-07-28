MILL_REVIEW_BEGIN
# Review: native clients: migrate gitrepo to go-git + selfreport gh-CLI to go-github

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-28
```

## Findings

### [GAP] Reactive Reindex cannot fire on error-swallowing reads
**Section:** `linked-worktree-and-interop-evidence` (Reindex policy) + `mixed-backend-call-sites`
**Issue:** The trigger is "an `object not found` for a SHA the handle otherwise reports as present", but three migrated reads never surface that error — `SHAExists` swallows into `false`, `isStrictDescendant` into `false`, `hasUnpushed` into `true` (required by the discussion's own parity case) — so a packfile-only object after a CLI write reads as a silent wrong answer with the remedy structurally unreachable; the concrete path is `SetSnapshotSHA` → `adoptSnapshotRef`'s CLI fetch (named in the discussion's own gc-trigger list) → `isStrictDescendant` (`snapshot.go:197`) returning `false` → `return nil`, silently dropping a strictly-newer snapshot value.
**Fix:** State where the reindex-and-retry wrapper lives and that the bool-swallowing methods must attempt it internally (before folding into their fallback), and add a parity case that repacks between `adoptSnapshotRef` and `isStrictDescendant`.

### [GAP] SetSnapshotSHA's two inline local reads are unclassified
**Section:** Scope "In" / `gogit-boundary-local-vs-remote` / `constraints-gitrepo-boundary-invariant`
**Issue:** `SetSnapshotSHA` performs two pure local reads on the CLI that no decision classifies — the `^{commit}` canonicalization (`snapshot.go:170`) and the adopted-ref read (`snapshot.go:190`) — the latter being byte-identical to `SnapshotSHA`'s ref read (`snapshot.go:101`) which *is* migrating, so the same read ships in two backends inside one file, the exact drift the discussion rejects under `mixed-backend-call-sites`.
**Fix:** Decide explicitly whether the boundary invariant is method-granular or call-granular, and name these two calls (migrate or CLI-bound-with-reason) so the invariant's "justify every `gitexec` call in `gitrepo`" rule has an answer for them.

### [GAP] No concurrency story for the machine-global token cache
**Section:** `token-resolution-and-cache` (location, permissions) + Testing "cache file handling"
**Issue:** The cache is deliberately one file shared by every lyx process on the machine, and lyx's premise is many concurrent worktrees/agents, yet no decision covers concurrent write or 401-invalidation-while-another-process-reads — no atomic temp-file-plus-rename, no lock, and rename interacts with the ACL-stripping step that the discussion itself calls load-bearing.
**Fix:** Record the write discipline (atomic replace with ACLs applied to the temp file before rename, or an explicit "torn write degrades to the corrupt-cache path" statement) and add a concurrent-writer case to the cache tests.

### [GAP] The 401 replay's body rewind has no named test
**Section:** `go-github-version-and-auth-construction` (transport nesting) + Testing "githubclient — token resolution"
**Issue:** The design flags `req.GetBody` rewind as the part that silently misbehaves ("a naive replay would send an empty body and fail confusingly"), but the test list only asserts that a 401 re-resolves exactly once and never loops — nothing asserts the replayed `POST` still carries the JSON body.
**Fix:** Add an explicit case: `httptest` server returns 401 then 201, assert the second request body is byte-identical to the first.

### [NOTE] Contradictory sizing for the pathresolve guard fix
**Section:** Scope "In" (bullet 8) vs `constraints-github-auth-invariant` (latent-hole bullet)
**Issue:** Scope says "size it as a small restructure, not a one-line edit" because the guard matches whole-file substrings (`strings.Contains(content, token)`, `pathresolve_guard_test.go:68`); the decision bullet says "it is a one-line change". Both agree on the end state (line-based match), only the sizing conflicts.
**Fix:** Drop the "one-line change" phrasing so the plan writer sizes it once.

### [NOTE] gitrepo package doc's gitexec section becomes false and is not on the rewrite list
**Section:** `docs-lifecycle` (content decision)
**Issue:** `docs-lifecycle` enumerates the locale paragraph (`doc.go:18-27`) precisely but not `doc.go:1-17`, which states gitrepo is "built on top of internal/gitexec's raw command runner" and "never calls exec.Command itself; every method goes through a single unexported run helper" — after the migration the read surface bypasses `run` entirely.
**Fix:** Add `doc.go:1-17` to the enumerated rewrite sites alongside the locale paragraph.

## Verdict

GAPS_FOUND
Four gaps: Reindex unreachable on swallowing reads, unclassified SetSnapshotSHA reads, cache concurrency, replay-body test.
MILL_REVIEW_END
