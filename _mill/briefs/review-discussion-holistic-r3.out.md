MILL_REVIEW_BEGIN
# Review: loom: code-writing skills — comments, build, testing

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-24
```

## Findings

### [BLOCKING:decision] golang-testing's per-project section left generic
**Section:** Decisions § "golang-build and golang-testing: minimal drafting" **Issue:** the decision reconciles only `golang-build` with this repo; `plugins/scribe/skills/golang-testing/SKILL.md`'s "Conventions to specify per project" still carries the unfilled placeholder ("Replace this section with the project's actual test strategy") and names only `//go:build integration`, so it omits this repo's `smoke` tag, the Hermetic Git Test Environment Invariant's mandatory `TestMain` calling `gitkit.HermeticGitEnv()`, and the Test Tier Purity Invariant's ban on spawns in untagged files — two machine-enforced invariants an agent following the skill here would violate. **Fix:** state a disposition — either fill `golang-testing`'s per-project section as `golang-build`'s was filled, or record explicitly that it stays generic and why.

### [BLOCKING:consistency] Design doc still cites an invariant the discussion retracted
**Section:** Decisions § "Design doc points to the skill, not the reverse" **Issue:** the decision says a later round found the Producer Pointer-Rule Invariant exempts "design docs restating the rule for a human reader," so it doesn't mandate the outcome — but `manifest/designs/code-comment-conventions.md` line 5 still asserts "This document is design rationale, not a second copy of the rule (Producer Pointer-Rule Invariant, `CONSTRAINTS.md`)", and line 12 then restates the rule anyway, contradicting line 5 in the same file. **Fix:** decide whether the doc keeps the invariant citation or rests on the portability argument alone, and make the doc's header match its own body.

### [NIT:consistency] golang-build's tool mandate not marked overridable
**Section:** Decisions § "golang-build and golang-testing" **Issue:** `golang-build/SKILL.md`'s "Tool installation" says a missing `goimports`/`golangci-lint` means "report which one and its install command, then stop — don't skip the step silently", unqualified, while "This repo's configuration" says skip both here; the top build-commands block likewise lists them unconditionally. **Fix:** note in the generic section that a project configuration may retire either tool, so the two sections don't read as a hard stop versus a skip.

### [NIT:scope] Repo-specific reference inside a portable skill
**Section:** Technical context / `conversation` skill **Issue:** `conversation/SKILL.md`'s File-writing bullet names "lyx's own tooling", a loomyard-specific concept that means nothing in another repo installing `scribe` — the same portability argument the design-doc decision rests on applies here, but no disposition is recorded. **Fix:** state whether repo-specific references are acceptable inside shipped skills, or generalize the bullet.

### [NIT:design] Hook delivery verified only as JSON parse
**Section:** Decisions § "Always-active mechanism" / Testing **Issue:** the structural pass confirmed `hooks/hooks.json` parses; nothing verifies Claude Code actually discovers it — `plugin.json` declares no `hooks` key, and `plugins/prowler/` ships no hook file as precedent, so the always-active mechanism's delivery is untested and the task closes before any install. **Fix:** record who verifies the hook fires after `/plugin install`, or state that unverified delivery is accepted.

## Verdict

REQUEST_CHANGES
Two blockers: golang-testing's unfilled repo section and the design doc's retracted invariant citation.
MILL_REVIEW_END
