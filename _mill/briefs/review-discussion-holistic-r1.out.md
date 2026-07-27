MILL_REVIEW_BEGIN
# Review: Crucible review spawn as effort-selectable Agent profiles

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-27
```

## Findings

### [GAP] Effort value enum is unsourced for low/xhigh
**Section:** Decisions → "Effort tiers offered: all five" / Technical context
**Issue:** The cited source confirms only that `effort:` is a supported *key*; the millhouse precedent covers `medium/high/max` only, so `low` and `xhigh` are asserted valid values with no evidence, and the discussion never states what Claude Code does with an unrecognized value (reject file, error at spawn, or silently ignore).
**Fix:** Cite the documented value set for `effort:` and state the observable failure mode of a bad value, or drop unverified tiers.

### [GAP] Smoke check cannot prove the effort key was honored
**Section:** Testing step 2 / Decisions → "Verification: cheap live smoke check"
**Issue:** "Spawns and replies OK to a no-op prompt" passes identically whether `effort:` was applied or silently ignored — exactly the failure the check exists to catch is the one it cannot see, and no observable signal (agent listing output, startup banner, session log) is named as the pass criterion.
**Fix:** Name a concrete observable that distinguishes honored-vs-ignored, or state explicitly that the check only proves frontmatter parses and that effort application is unverifiable.

### [GAP] Tool allowlist silently narrows the round agent
**Section:** Technical context → draft body frontmatter (`tools: Read, Edit, Write, Bash, Grep, Glob, Skill`)
**Issue:** Rounds run as `general-purpose` today with the full tool set; the copied mill-implementer list drops `BashOutput`/`KillShell` (crucible rounds drive long-running live substrate — `crucible/README.md` "Driving the real substrate"), plus `TodoWrite`/`WebFetch`/`WebSearch`, and no Decision records this narrowing or the alternative of omitting `tools:` to inherit.
**Fix:** Add a Decision justifying the exact tool set against crucible round needs, or omit `tools:` so the profile changes effort only.

### [GAP] Stale general-purpose reference outside the edit list
**Section:** Scope → README edits / Testing step 3
**Issue:** `crucible/README.md:22` ("a `general-purpose` Agent, *not* a fork") is a spawn-mechanics reference not in the enumerated edits, and the proposed greps (`subagent_type: general-purpose`, `general-purpose Agent`) both miss it because of the backtick between the words.
**Fix:** Add line 22 to the edit list and change the consistency grep to the bare token `general-purpose` across `crucible/`.

### [GAP] No defined behavior when the operator names no effort
**Section:** Decisions → "No unsuffixed 'inherit' fallback file" / orchestrator-prompt edits
**Issue:** With `general-purpose` removed from rule 2 and no bare `crucible-reviewer.md`, the orchestrator has no defined action if the operator says only "next round, Opus" — ask, default to a named tier, or fall back to `general-purpose` is left to interpretation.
**Fix:** State the orchestrator's required behavior on a missing effort pick in rule 2 / loop step 2.

### [GAP] "No CONSTRAINTS.md entry applies" is overstated
**Section:** Constraints
**Issue:** Two invariants place review obligations on agent prompt text, not on Go packages: the Weft Git Invariant ("agent prompt templates never instruct a weft git op" — the new bodies do instruct `git commit`) and the Review Round Invariant (A-before-B, fix-all-severities, commit-per-fix, never push — precisely what the bodies encode).
**Fix:** Replace the blanket claim with an explicit acknowledgement of those two review obligations and how the file bodies satisfy them (host-repo commits only, never weft).

### [NOTE] Project-level discovery is cwd- and merge-scoped
**Section:** Scope → Out ("auto-discovered")
**Issue:** `.claude/agents/` does not exist anywhere in this repo today; the files resolve only from a worktree whose checkout contains them, so a campaign worktree branched before this merges gets an unknown `subagent_type` mid-round; user-level `~/.claude/agents/` was not considered.
**Fix:** Note the merge/`mill-merge-in` prerequisite for in-flight worktrees and record project-vs-user placement as a decision.

### [NOTE] Five duplicated bodies with no drift guard
**Section:** Scope / Testing
**Issue:** The body duplicates the commit-per-fix and executive-summary contract from `review-prompt-template.md` across five files, with the automated harness explicitly out of scope — five copies can silently drift from the template they restate.
**Fix:** State the intended sync discipline (e.g. bodies byte-identical except name/description/effort; template is authoritative on contract wording).

### [NOTE] Body omits the clean-room constraint
**Section:** Technical context → draft body
**Issue:** The rationale for pre-loading contract text is "know it before opening the review prompt", yet the load-bearing clean-room rule (`review-prompt-template.md` §"Clean-room review constraint": do not open prior `.scratch/` reviews before forming own findings) is not among the pre-loaded points.
**Fix:** Add a one-line clean-room bullet alongside the commit and summary bullets, or state why only those two were selected.

## Verdict

GAPS_FOUND
Effort enum, verification strength, tool set, stale reference, and constraint coverage need resolution.
MILL_REVIEW_END
