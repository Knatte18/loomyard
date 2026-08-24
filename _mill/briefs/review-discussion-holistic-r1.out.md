MILL_REVIEW_BEGIN
# Review: loom: Discussion-Write producer

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Opus-class), Anthropic
reviewed_file: _mill/discussion.md
date: 2026-08-24
```

## Findings

### [BLOCKING:design] Validator bounce respawns blind
**Section:** Decisions → `no-on-stuck` / Technical context → Outcome mapping
**Issue:** `loomshed/discussionvalidate.go:51-56` discards `discussionparser.Validate`'s findings and returns bare `Stuck`, and `SingleLLMProducer.Call` archives both files then respawns with an identical prompt — so the one bounce path this task makes live is exactly the context-free retry the `no-on-stuck` rationale rejects, and can burn the whole bounce budget re-producing the same defect.
**Fix:** State a disposition for the `Discussion-Validate` → `Discussion-Write` re-entry: either how findings reach the respawned agent, or an explicit decision that a blind re-write is accepted and why the mechanical heading checks make it converge.

### [BLOCKING:decision] Produced files have no commit disposition
**Section:** Scope / Constraints
**Issue:** `_lyx/discussion/` is junctioned into weft, and no Go path commits the two produced files today — `loomcli/run.go:118` commits only the status file and origin record, and no `loomshed`/`landingshed` row touches them; the Fabric Git Invariant makes committing agent-written `_lyx` content Go's job, and leaving them uncommitted leaves weft dirty for `Finalize`'s merge guard.
**Fix:** State whether this task commits the two artifacts (and where), or name the existing row that already does, or record it as explicitly out of scope with the dirty-weft consequence acknowledged.

### [BLOCKING:design] Step 0 loads an uninstalled plugin
**Section:** Decisions → `skills-load-at-step-0`
**Issue:** `manifest/roadmap.md:366` states installing scribe (`/plugin install scribe@loomyard`) is a manual step "not yet done", and nothing in `shuttleengine`/`claudeengine` installs or verifies plugins — the Step 0 instruction can silently resolve to nothing in the spawned pane, with no stated failure mode.
**Fix:** Decide the plugin-absent behaviour (best-effort wording, a precondition on the loom run, or a prerequisite install step) rather than leaving it implicit.

### [BLOCKING:scope] Doc inventory misses two stale claims
**Section:** Scope → In (docs) / Out → `docs/overview.md`
**Issue:** The enumeration names only `manifest/designs/loom.md` row 3, but `loom.md:270` asserts `DiscussionSpec`/`PlanSpec` are "built but not yet wired into `Shed`", and `internal/loomengine/discussion.go`'s header comment says "the future loom phase machine drives the returned Spec" — both become false when this lands; only `loomshed/stub.go`'s doc is listed.
**Fix:** Extend the doc-edit list to `loom.md:270` and `discussion.go`'s file comment, or state the enumeration method used so a plan writer can re-derive it.

### [NIT:consistency] `modeRules` names a flag that does not exist
**Section:** Decisions → `keep-mode-rules-both-branches`
**Issue:** `prompt.go:40` ships "autonomous (`--auto`) mode" in the prompt the wiring pins on, while the `autonomous-only` decision itself states "no such flag exists today" — the shipped prompt cites a nonexistent CLI flag.
**Fix:** Say whether that phrase is corrected in this task or deliberately left for the interactive follow-up.

## Verdict

REQUEST_CHANGES
Four undecided items: bounce feedback, artifact commit, plugin availability, doc inventory.
MILL_REVIEW_END
