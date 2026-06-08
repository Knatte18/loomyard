MILL_REVIEW_BEGIN
# Review: config-layer

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: C:\Code\mhgo\wts\config-layer\_mill\discussion.md
date: 2026-06-08
```

## Findings

### [GAP] board-dir MkdirAll runs too late in writeOp
**Section:** Decisions → board-dir-autocreate
**Issue:** The only `MkdirAll` is inside `AtomicWrite` (git.go:69), but `writeOp` (wiki.go:38) first calls `AcquireWriteLock(filepath.Join(wikiPath, writeLockFile))`, and `flock.New(...).Lock()` opens the lock file inside a board dir that does not yet exist — so a first write to a missing board dir fails before any MkdirAll runs.
**Fix:** Decide and state explicitly that `New`/`writeOp` does `MkdirAll(boardPath)` up front (before acquiring the write lock), not rely on AtomicWrite's late mkdir.

### [NOTE] sync child skips _mhgo check but still locks board dir
**Section:** Decisions → spawn-sync-path / board-dir-autocreate
**Issue:** The detached `sync` child gets `--board-path` and skips LoadConfig, but `Sync`→`commitDirty` still `AcquireWriteLock` inside the board dir; if the board dir was never created (write spawned sync before MkdirAll landed) the child errors silently — acceptable since sync errors are unsurfaced, but undocumented.
**Fix:** Note that sync tolerates a missing board dir (its errors are detached/ignored), tying it to the autocreate ordering decision above.

### [NOTE] init writes .gitignore in arbitrary cwd, not repo root
**Section:** Decisions → gitignore-block
**Issue:** Repo `.gitignore` already has a distinct `# === mill-managed ===` block (no marker collision — good), but the decision doesn't state that `init` writes `<cwd>/.gitignore`, which may not be a git repo root or may not be the file mhgo's own repo uses.
**Fix:** Confirm the mhgo-managed block targets `<cwd>/.gitignore` regardless of git-root status (consistent with the cwd-authoritative model).

## Verdict

GAPS_FOUND
One ordering gap: autocreate must run before the write lock, else first write to a missing board dir fails.
MILL_REVIEW_END
