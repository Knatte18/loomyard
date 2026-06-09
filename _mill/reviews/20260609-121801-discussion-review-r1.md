I have verified all the key claims. Now I'll compile the review.

Key findings:
1. **board.go line 46** calls `AcquireWriteLock` — the discussion's Technical Context "Key files to read" and the package-layout file map only list store.go and sync.go for lock-call migration. board.go is marked "(unchanged)" in the file map. This is a concrete gap that would break compilation when lock.go is deleted.
2. The init.go template default for `path` is `../_board`, and the discussion's proposed comment line matches — consistent.
3. The two-regex env expansion approach has an ambiguity: how a value containing a literal `?` that is NOT an env-fallback (e.g. a URL query) is handled, and what happens when the optional token is NOT last. The decision says it "must be last" but the parser pseudocode does not enforce/validate it.

Let me write the review.

MILL_REVIEW_BEGIN
# Review: extract-shared-infra

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: C:\Code\mhgo\wts\extract-shared-infra\_mill\discussion.md
date: 2026-06-09
```

## Findings

### [GAP] board.go lock call omitted from migration list
**Section:** Technical context / Package layout + "Key files to read"
**Issue:** `board.go:46` calls `AcquireWriteLock(filepath.Join(b.boardPath, writeLockFile))`, but the file map marks `board.go` "(unchanged)" and the migration call-site list names only `store.go` and `sync.go`; deleting `lock.go` will break board.go compilation.
**Fix:** Add `board.go:46` to the lock-call migration list and mark it as edited (becomes `lock.AcquireWriteLock`).

### [GAP] Malformed `? fallback` placement undefined
**Section:** Decisions / `$env:NAME ? fallback` optional syntax; Technical context / env regex
**Issue:** Decision says the `?`-token "must be last" and "only one per value," but neither the regex (`...\?\s*(.*)$`) nor the expansion-order text defines behaviour when a value has a `?` that is not a fallback (e.g. a URL query string) or has the token mid-value — silent misparse vs error is unspecified.
**Fix:** State whether a non-last/extra `?` is an error or treated literally, and add a test case covering a value with a literal `?` that is not an env fallback.

### [NOTE] `.env` precedence vs setenv side-effect not stated
**Section:** Decisions / `.env` file loading; pseudocode
**Issue:** The pseudocode does `setenv(key,val)` for unset keys, mutating process env globally; in test runs (parallel/`t.Setenv`) this leaks across `Load` calls, but cleanup/scoping is not addressed.
**Fix:** Note whether `.env` vars are applied process-wide (persisting) or scoped to the single `Load` call, and how config tests isolate this.

### [NOTE] init.go template comments lose inline key descriptions
**Section:** Technical context / init.go template comment update
**Issue:** Current `generateCommentedBoardYAML` emits trailing descriptions (e.g. "# board dir ... may contain $env:NAME"); the proposed new lines keep some but the spec block does not confirm the descriptive trailers are preserved.
**Fix:** Confirm the template retains per-key descriptive comments alongside the new `? fallback` examples.

## Verdict

GAPS_FOUND
A lock call in board.go is missing from the migration scope; fallback-syntax validation is underspecified.
MILL_REVIEW_END
