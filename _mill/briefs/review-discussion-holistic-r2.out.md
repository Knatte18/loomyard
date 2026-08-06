MILL_REVIEW_BEGIN
# Review: .lyx hygiene -- relocate transients out of _lyx, fix .lyx junction geometry (slice 9)

```yaml
verdict: GAPS_FOUND
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-06
```

## Findings

### [GAP] Junction-vs-plain-directory has no Decision block
**Section:** Q&A log line 1 / Decisions
**Issue:** The single most consequential choice — machine-local scratch physically lives inside the weft repo — is recorded only as a Q&A answer ("Junction"), with no rationale and no rejected alternative, even though a plain real `.lyx` directory excluded via the warp's `.git/info/exclude` achieves the stated goal (no tracked artifact in the user's repo) without moving live state into a git worktree.
**Fix:** Add a `### dotlyx-is-a-weft-backed-junction` decision with rationale and the rejected "plain directory + warp exclude" alternative.

### [GAP] Weft-side `.lyx` lifecycle under unwire/Remove undefined
**Section:** Scope / Testing (`internal/fabriccli`)
**Issue:** `internal/fabricengine/unwire.go:87-97` clears the weft-side `_lyx` content (`WeftContent: "cleared"`) on unwire, and paired teardown/checkout also operate on the weft worktree; the discussion never states whether the weft-side `.lyx` target is cleared, preserved, or left to `Remove`'s dirty gate — live reed/scout/shuttle state now sits there.
**Fix:** State the unwire/Remove/checkout contract for the weft-side `.lyx` target and add a test asserting it.

### [GAP] Leaf-invariant allowlists not amended for `internal/lyxdirs`
**Section:** Constraints / Scope
**Issue:** `internal/scoutengine/leaf_enforcement_test.go`'s `allowedImports` is exactly `{configengine, lock, proc, logger, yaml}`, and CONSTRAINTS' Scoutengine Leaf Invariant names the same set — `scoutengine/daemonstate.go` importing `internal/lyxdirs` fails `TestLeafInvariant_AllowlistOnly` on day one; the discussion mentions only that `lyxdirs` itself must stay a leaf.
**Fix:** Enumerate every allowlist/enforcement test and CONSTRAINTS clause that must gain `internal/lyxdirs` (scoutengine at minimum; re-check treadleengine, tokenvocab, pattern, `lyxcwd/enforcement_test.go`, `cmd/lyx/constructoranchoring_test.go`).

### [GAP] No dedup rule; "not read from fabric.yaml any more" is inaccurate
**Section:** Decision `structural-dirs-are-not-config`, "Set composition, exactly"
**Issue:** Deployed `fabric.yaml` keeps `pathspec: _lyx _pattern`, and `Config.Dirs()` (`config.go:28`, `strings.Fields`) still parses it — so `_lyx` lands in both `structuralCommittedDirs` and `filterHubReserved(cfg.Dirs())`; the three set definitions are written as `+` with no dedup, feeding duplicate names to `HostJunctions`, `ScopedPathspec` and status output.
**Fix:** Specify set union with dedup (and order) for all three sets, and correct the rollout claim to "still parsed, but structurally re-added and deduped".

### [GAP] Old repos keep the committed `.gitignore` `.lyx/` block forever
**Section:** Scope / "The committed-`.gitignore` sites to remove"
**Issue:** The task deletes both `fabriccli/clone.go:81` (`gitignore.Ensure`) and `fabricengine/unwire.go:113` (`gitignore.Remove`), so a repo cloned by an older binary keeps the tracked `.lyx/` entry with no code path left to remove it — exactly the tracked artifact the Problem section says must never remain in the user's repo.
**Fix:** Decide and record the cleanup path (one-shot removal at wire/reconcile time, or explicitly out of scope with the manual remedy documented).

### [GAP] Adoption collision rule still undecided
**Section:** Decision `dotlyx-content-adoption-no-other-migration`, implementation note
**Issue:** "The plan should define the collision rule (the weft-side copy wins, or the operation refuses…)" leaves an unresolved alternative in a decision block; the plan writer has to choose.
**Fix:** Pick one (the text already leans to refuse-on-collision) and state it as the decision, with the other as rejected.

### [NOTE] Adoption runs against live daemon state
**Section:** Decision `dotlyx-content-adoption-no-other-migration`
**Issue:** Adoption moves a real `.lyx` whose files reed/scout/shuttle may hold open; on Windows (the platform junctions exist for) a directory move with open handles fails, and POSIX-side writers keep writing to moved inodes.
**Fix:** State the precondition (daemons stopped, or adoption tolerates/reports a busy directory) and cover it in the fabricengine adoption test.

### [NOTE] Downgrade hazard for the `.lyx` junction
**Section:** Decision `structural-dirs-are-not-config`
**Issue:** An older binary's `applyStaleRemoval` (`reconcile.go:391`) removes on-disk junctions absent from its `RepoWiredNames`, so running an old `lyx fabric reconcile` after this change unwires `.lyx` and strands scratch in weft.
**Fix:** Note the one-way-upgrade expectation, or say explicitly that downgrade is unsupported.

## Verdict

GAPS_FOUND
Six unresolved items: junction rationale, weft-side lifecycle, allowlists, dedup, gitignore cleanup, collision rule.
MILL_REVIEW_END
