MILL_REVIEW_BEGIN
# Review: loom: code-writing skills — comments, build, testing

```yaml
duration_s: 133.0
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-24
```

## Findings

### [BLOCKING:design] "Always-active" has no mechanism, and ships a TBD
**Section:** Decisions § prose and conversation split; Scope Out (stencil wiring)
**Issue:** `prose`/`conversation` are declared always-active, but stencil wiring is explicitly out of scope, so nothing makes them load; `plugins/scribe/skills/INDEX.md:13` ships the gap as an open item ("not yet decided — see `discussion.md` for this task"), pointing at a mill-transient file that does not travel with the plugin.
**Fix:** State what "always-active" means in the shipped artefact today (skill-description convention only, or a per-skill "load `prose` first" line), and decide whether INDEX.md's undecided sentence and its `discussion.md` pointer stay, move to the roadmap, or are cut.

### [NIT:consistency] Design doc contradicts itself after the status-line edit
**Demoted-from:** BLOCKING
**Section:** Scope In (status line updated); Technical context
**Issue:** `manifest/designs/code-comment-conventions.md:4` says "No producer-stencil wiring yet", while line 44 of the same file states wiring is "composed into every code-writing producer's stencil via a 'Load these skills: ...' section ... not left to model discretion" as present fact.
**Fix:** Say in Scope that the Enforcement section's closing wiring sentence is reconciled (future tense or a roadmap pointer) in the same edit as the status line.

### [NIT:consistency] Constraints section dismisses an invariant that binds skills
**Demoted-from:** BLOCKING
**Section:** Constraints
**Issue:** The blanket "CONSTRAINTS.md's invariants don't apply — touches nothing under `internal/` or `cmd/`" is false for the Producer Pointer-Rule Invariant, which binds instruction files (skills) and forbids paraphrasing another file's rule content; `code-quality/SKILL.md:87-98` paraphrases `code-comment-conventions.md:9-29` (self-containment, the two exceptions, the information-triage list) rather than pointing at it, and no authority between the two is stated.
**Fix:** Name the Producer Pointer-Rule Invariant explicitly and decide which file is the single source of the self-containment rule — design doc or skill — with the other pointing at it.

### [NIT:decision] Roadmap item's disposition unstated
**Demoted-from:** BLOCKING
**Section:** Scope In / Out
**Issue:** This completes `manifest/roadmap.md`'s Wave 1 item, whose text still names a separate `code-comments` skill and a millhouse-mirroring shape this discussion rejected; neither the roadmap edit nor the reconciliation of that stale text is in scope, though CLAUDE.md requires roadmap movement on completing a planned item.
**Fix:** Add the roadmap entry to Scope In with its disposition (mark complete, and reword or drop the `code-comments`/two-plugin phrasing).

### [NIT:consistency] `update-plugins.sh` claim overstates what was verified
**Demoted-from:** BLOCKING
**Section:** Technical context; Testing
**Issue:** `update-plugins.sh:21-24` returns on the not-installed branch before `source_dir` is ever used, so the quoted "Skipped (not installed)" line proves only that marketplace.json parses and names `scribe` — it validates nothing under `plugins/scribe/`, contrary to "confirmed the wiring is correct."
**Fix:** Restate the claim to what the script actually proves, and say what (if anything) checks the plugin's own shape — `plugin.json`, per-skill frontmatter `name` matching its directory — since Testing names no mechanical check at all.

### [NIT:scope] Scope reads as already-done, leaving no work inventory
**Section:** Scope In; Technical context
**Issue:** Items are written in the perfect tense ("All seven are drafted", the status line "now points at"), and all seven `SKILL.md` files plus the marketplace entry already exist on disk, so a plan writer cannot tell what remains to be built versus what only needs review.
**Fix:** Split Scope into what already exists in the worktree and what the plan must still produce.

## Verdict

REQUEST_CHANGES
Five blocking gaps: unimplemented always-active, contradictory design doc, missed invariant, roadmap, overstated verification.
_Note: 4 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 1._
MILL_REVIEW_END
