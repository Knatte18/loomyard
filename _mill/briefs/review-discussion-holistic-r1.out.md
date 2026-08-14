MILL_REVIEW_BEGIN
# Review: Relocate producer prompt files into a stencils/ directory

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Opus-class, Anthropic)
reviewed_file: _mill/discussion.md
date: 2026-08-14
```

## Findings

### [BLOCKING:design] Promoted file stays "edited" forever
**Section:** `stamp-format-and-edit-detection` + Testing "Port-back guard"
**Issue:** The table keys the untouched branch on `hash(body) == stamp`; after `promote`, the board copy's stamp is still the OLD default's hash, so even once the shipped default equals its body the file stays classified edited — never restamped, skipped by every future refresh, and `diff --all --exit-code` never returns to zero. Testing asserts the opposite ("seeds back to a matching hash on the next run").
**Fix:** State the missing reconciliation rule explicitly (e.g. `hash(body) == hash(newDefault)` ⇒ restamp, or `promote` restamping the board copy) and align the table with it.

### [BLOCKING:design] `diff` has two contradictory base texts
**Section:** `no-automatic-merge` vs `port-back-is-mechanical-not-remembered` / `cli-surface`
**Issue:** One decision says the diff base is the board repo's git history of prior defaults; the other says `diff --all --exit-code` fires "when any board copy has been edited away from its shipped default" (i.e. base = the embedded current default). These are different comparisons with different exit behaviour, and the pre-commit hook's semantics depend on which one ships.
**Fix:** Name one base per diff mode (`diff <name>` drift view vs `diff --all --exit-code` port-back guard) and say where each base text is read from.

### [BLOCKING:design] baseDir never reaches treadle's call sites
**Section:** `stencilstore-ownership`, Scope "all five producer call sites"
**Issue:** `stencilstore.Read(baseDir, …)` requires a hub-anchored `<hub>/_board` path. loom/burler carry `*lyxcwd.Location`, but `internal/treadleengine` is deliberately told only `runDir`/`Profile.GateDir` (neither of which is the hub) and is barred from `lyxcwd`; `judge.go`/`targeting.go` read the embedded vars deep inside `runJudgeCall`. The discussion never says how the stencil base directory is threaded into treadle (or webster's `MasterTemplate()`/`ForkTemplate()` no-arg accessors, which also gain an error channel).
**Fix:** Decide and record the threading seam per engine — a new `Run`/`Profile` field for treadle and the new accessor signatures for webster.

### [BLOCKING:consistency] Bolt board write has no pathspec and no lock
**Section:** Constraints "Fabric Git Invariant" bullet; `seeding-trigger`
**Issue:** The discussion states the board write is "positive-only via `fabricengine.ScopedPathspec`", but `Bolt.Commit` (`internal/fabricengine/bolt.go:23` → `weftgit.go:337`) takes no pathspec and calls `StageAllAndCommit`, and its doc says it "does not acquire the weft write lock; the caller is responsible for synchronization". Seeding on *every* run, from every worktree/session in a hub, is therefore an unsynchronised wildcard-staged commit.
**Fix:** Correct the pathspec claim to what Bolt actually does, and state the concurrency rule for simultaneous seeding writes (lock, `Bolt.Sync`, or seed-only-when-changed).

### [BLOCKING:decision] Pre-commit hook has no installation disposition
**Section:** `port-back-is-mechanical-not-remembered`
**Issue:** The hook is the sole guard against silent permanent drift, yet the discussion never says who installs it, where it lives (`.git/hooks` is untracked and shared repo-wide across all warp worktrees via the common gitdir), or what happens when it runs where `lyx` or the hub is unavailable — a guard nobody installs is no guard.
**Fix:** State the install mechanism (tracked script + `core.hooksPath`, a `lyx` verb, or deploy step) and the behaviour when preconditions are absent.

### [NIT:consistency] CONSTRAINTS seam counts not listed as edits
**Section:** Constraints / CLI-Cobra bullet
**Issue:** The CLI/Cobra Invariant hardcodes "eleven seam modules" and "ten … `RunCLIIn`"; a new `stencil` module makes those twelve/eleven, but the listed CONSTRAINTS edits are only the new invariant plus the treadle bullet.
**Fix:** Add the seam-count update to the same-commit CONSTRAINTS edit list.

### [NIT:scope] "Five producer call sites" is four packages
**Section:** Scope / Technical context
**Issue:** Scope says "all five producer call sites" while the enumerated embed sites are four packages (loom, burler, treadle, webster) across five files.
**Fix:** Say four packages / five files, or name the five sites.

## Verdict

REQUEST_CHANGES
Stamp lifecycle, diff base, baseDir threading, Bolt claims and hook install all unresolved.
MILL_REVIEW_END
