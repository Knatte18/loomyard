MILL_REVIEW_BEGIN
# Review: Crucible review spawn as effort-selectable Agent profiles

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-27
```

## Findings

### [GAP] Round tag drops the new effort axis
**Section:** Technical context → `orchestrator-prompt.md` / `README.md` edits
**Issue:** `orchestrator-prompt.md:30` and `README.md:40` tag a round `<model>-r<N>`, and `review-prompt-template.md:90-91` derives both deliverable filenames from that tag — with effort as a second independent axis, two rounds at the same model but different effort collide in tag and filenames, and the handoff note / campaign table (`README.md:112-118`, "Round | Model") no longer records which effort produced which result.
**Fix:** Decide and state the tag form (e.g. `<model>-<effort>-r<N>`) and add the tag/table/deliverable-naming lines to the edit list, or explicitly decide effort is not recorded per round and say why.

### [GAP] Valid tier names never enumerated in the edited docs
**Section:** Technical context → `orchestrator-prompt.md` edits (rename to "Model + effort selection")
**Issue:** Every edit spec uses the placeholder `crucible-reviewer-<effort>`; nothing says the renamed section (or `README.md`) lists the five concrete `subagent_type` values, so an operator reading the orchestrator prompt cannot know what to pick — and if the smoke check drops `low`/`xhigh` (per the effort-tiers caveat), nothing says the doc enumeration must be trimmed to match.
**Fix:** State that the renamed section enumerates the shipped tier names, and that dropping a tier removes it from that list in the same commit.

### [GAP] Constraints section contradicts the draft body on the weft/host statement
**Section:** `## Constraints` (Weft Git Invariant bullet) vs `## Technical context` draft body
**Issue:** The bullet directs "State this explicitly in the body so a reader doesn't mistake 'commit per fix' for a weft-git instruction", but the draft body contains no host-vs-weft sentence, and the closing paragraph then asserts both invariants are satisfied "by the draft body in Technical context as written; no additional implementation work follows".
**Fix:** Either add the host-repo-only clause to the draft body's "Commit per fix" bullet (and to the sync-discipline list of byte-identical text), or drop the "state this in the body" instruction.

### [GAP] Zero-hit `general-purpose` grep contradicts the required rule-2 text
**Section:** `## Testing` item 3 vs Technical context edit spec (rule 2)
**Issue:** Rule 2 is required to state "never falls back to `general-purpose`", so the prescribed bare-token grep across `crucible/` cannot return zero hits after the edits — the acceptance check fails by construction (verified: the token occurs today only at `README.md:22,40` and `orchestrator-prompt.md:18,30`, all four in the edit list).
**Fix:** Restate the check as an expected-hit inventory (exactly the deliberate never-fall-back mentions) rather than "zero stale hits".

### [NOTE] Five described agents become auto-delegation candidates
**Section:** Decision: Description states the effort level in prose
**Issue:** A `description:` is what Claude Code matches on for automatic delegation; five near-identical crucible descriptions in the repo's first `.claude/agents/` (verified: no `.claude/` exists today) may be picked up for unrelated main-thread work.
**Fix:** Decide whether each `description:` carries an explicit "only when invoked by name / by the crucible orchestrator" qualifier.

### [NOTE] No abort criterion if a core tier's smoke check fails
**Section:** Decision: Effort tiers offered / Testing item 2
**Issue:** "Only `medium`/`high`/`max` are independently confirmed working via the millhouse precedent files" overstates the evidence — those files live on the unmerged `origin/hanf/linux-port-more` (the checked-out millhouse worktree has only untiered `mill-implementer.md`/`mill-reviewer.md`), which proves the key was written, not that a spawn was observed; the drop-a-tier rule covers only `low`/`xhigh`.
**Fix:** State what happens if `medium`/`high`/`max` also fails to spawn (abort the task vs. ship docs-only).

### [NOTE] V0-obsolescence trigger recorded nowhere durable
**Section:** `## Problem` (V0 stopgap framing)
**Issue:** The "delete once Hardener drives the loop via modelspec" intent lives only in this discussion file; neither the five bodies nor `crucible/README.md` records it, so nothing tells a future reader these files are disposable.
**Fix:** Decide whether one line of the shipped text (agent body or README) names the obsolescence trigger.

### [NOTE] Template's "entire instruction set" claim goes stale
**Section:** Scope (files touched)
**Issue:** `review-prompt-template.md:3` says the per-module prompt "is the round agent's *entire* instruction set" — false once the agent-file body also carries clean-room/commit-per-fix/summary contract text, and that file is outside the declared edit set.
**Fix:** Either add the one-line template fix to scope or state that the duplication is intentional and the sentence stands.

## Verdict

GAPS_FOUND
Four gaps: round tagging, tier enumeration, a self-contradicting weft-git instruction, and an unsatisfiable grep check.
MILL_REVIEW_END
