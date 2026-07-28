MILL_REVIEW_BEGIN
# Review: native clients: migrate gitrepo to go-git + selfreport gh-CLI to go-github

```yaml
verdict: GAPS_FOUND
reviewer_model: opus
reviewed_file: _mill/discussion.md
date: 2026-07-28
```

## Findings

### [GAP] Boundary guard's pinned set ≠ the CLI-bound set
**Section:** `### constraints-gitrepo-boundary-invariant`
**Issue:** The guard collects "every method containing an `r.run(` call" and asserts equality with "the CLI-bound methods", but after this task those two sets differ in both directions: `SnapshotSHA` keeps its CLI fetch (`snapshot.go:98`) while its ref read migrates, so a migrated method lands on the pinned list; and `Push` (`push.go:51`), `PushCoalesced` (`push.go:134-149`) and `SetSnapshotSHA` are CLI-bound by contract yet contain no `r.run(` of their own (they delegate to `pushWithRebaseRetry`/`advanceAndPushSnapshotRef`, or lose their only two `r.run` calls at `snapshot.go:170,190` to go-git).
**Fix:** State the pinned list literally (the method set that will contain `r.run(` post-migration, `SnapshotSHA` included) and record the guard's blind spot — it cannot see a new CLI call added inside an already-listed method, which is exactly `SnapshotSHA` where a migrated read now lives.

### [GAP] Stale doc sites outside the enumerated set
**Section:** `### docs-lifecycle`
**Issue:** The enumeration is line-precise but misses three sites this task falsifies: `docs/overview.md:140` ("typed Repo over one local git checkout, **built on gitexec**" — the same claim the discussion flags as false in `gitrepo/doc.go:1-17`); `manifest/roadmap.md:65`, the Done spike entry, which points readers at `internal/gitnativepoc/doc.go` (deleted here) and states the now-overturned "both commit methods … migrate cleanly" recommendation; and `manifest/roadmap.md:13`, whose Planned text repeats that same superseded scope and links `designs/native-clients-migration.md` (also deleted) — moving it verbatim into Done ships a false record of what landed.
**Fix:** Add the three sites to the docs-lifecycle scope, saying explicitly that the Planned #3 text is rewritten (not moved verbatim) and that the spike's Done entry loses its `gitnativepoc` pointer and its commit-method claim.

### [GAP] Linked-worktree fixture omits the two silent reads
**Section:** `## Testing` — "`internal/gitrepo` — linked-worktree fixtures"
**Issue:** The fixture enumeration runs `CurrentSHA`, `CurrentBranch`, `remoteName`, `SnapshotSHA`'s ref read, `SHAExists` and `ChangedFilesSince`, but not `hasUnpushed` or `isStrictDescendant` — the two migrated reads that swallow failure into `true`/`false` and therefore cannot report a commondir mishandling as an error. `isStrictDescendant` is the named victim of the silent-drop bug (`snapshot.go:197` → `return nil`), and `hasUnpushed` reads branch tracking config plus a `refs/remotes/*` ref, both common-dir state, in the only topology production runs in.
**Fix:** Add both to the linked-worktree fixture's method list (they need the internal `package gitrepo` test file the discussion already mandates), including the strictly-behind `hasUnpushed` case.

### [NOTE] Guard self-exclusion allowlist is redundant
**Section:** `### constraints-github-auth-invariant`
**Issue:** The gh guard scans "non-test `.go` files only", so its own `*_test.go` source is already outside the scan; the cited precedent says exactly that at `tools/sandbox/pathresolve_guard_test.go:47-50` ("excluded by the same filter, with no special-case needed"). The `cmd/lyx/hermeticenv_test.go` analogy does not transfer — that guard scans test files. The `tierpurity_test.go` `allowedSpawners` entry is genuinely required; the self-exclusion one is not.
**Fix:** Drop the self-exclusion allowlist from the spec (or note it as defensive), keeping only the `allowedSpawners` entry as required.

## Verdict

GAPS_FOUND
Guard pinning, three stale doc sites, and linked-worktree coverage need resolution before planning.
MILL_REVIEW_END
