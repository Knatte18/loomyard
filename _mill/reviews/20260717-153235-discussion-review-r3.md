MILL_REVIEW_BEGIN
# Review: loom: Preflight phase (precondition validation)

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-17
```

## Findings

### [GAP] Presence-checklist conflicts with pinned schema doc
**Section:** field-presence-and-nullability vs Scope→In / status-schema.md
**Issue:** Scope says the validator implements "the schema doc's validation checklist," whose item 1 requires all nine fields (incl. `history`/`start_sha`/`pause_requested`/`next_action`) be *present*, but the decision deliberately does NOT presence-enforce those four (absent ≡ valid null/false/empty) — a silent deviation from the pinned contract a plan writer could implement either way.
**Fix:** State explicitly that Preflight treats absent `history`/`start_sha`/`pause_requested`/`next_action` as satisfying the checklist's "present" (via zero/null semantics), and reconcile the schema doc's checklist wording.

### [GAP] Seed-read path is Cwd-anchored, not worktree-root
**Section:** seed-read-path / Technical context (hubgeometry)
**Issue:** The decision reads `l.LyxDir()/status.json`, but `LyxDir()` is `filepath.Join(l.Cwd, LyxDirName)` (verified hubgeometry.go:318) — invoked from a subdirectory, check 4 reads a nonexistent `_lyx` and reports a false `seed-missing`, while checks 2/3 correctly use `WorktreeRoot`, an inconsistency Preflight (which accepts any `Getwd` cwd) never addresses.
**Fix:** Pin whether the new `hubgeometry` status.json accessor anchors at `WorktreeRoot` (recommended) rather than `Cwd`, so all five checks agree on the worktree.

### [NOTE] "Never mutates filesystem" vs ReadJSONStrict side effects
**Section:** strict-read-mechanism / Problem (line 22)
**Issue:** Preflight is described as never mutating filesystem state, yet `ReadJSONStrict` is defined "identical to `ReadJSON`," which does `os.MkdirAll` (state.go:52) and `AcquireReadLock` creates a `.status.json.lock` file (lock.go:57) — a write into the weft-synced `_lyx/`.
**Fix:** Note the benign side effect (dir already exists post-stat; `*.lock` gitignored/excluded so it trips no check) or have the strict variant skip `MkdirAll` on read.

### [NOTE] Lock file lands in weft-synced overlay
**Section:** strict-read-mechanism (open note) 
**Issue:** The suggested `.status.json.lock` under `_lyx/` sits in the weft overlay; builder's own convention is `state.json.lock` under `_lyx/builder/` (state.go:120) — confirm the loom lock path is excluded from weft commit pathspec like builder's `*.lock`.
**Fix:** Have the plan pin the lock path and confirm it inherits builder's `*.lock` weft-exclusion.

## Verdict

GAPS_FOUND
Two gaps: presence-checklist vs pinned schema, and Cwd-anchored seed read.
MILL_REVIEW_END
