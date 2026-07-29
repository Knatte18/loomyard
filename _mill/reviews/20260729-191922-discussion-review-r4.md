MILL_REVIEW_BEGIN
# Review: prowler: installable Claude Code plugin (Go), hosted in LoomYard

```yaml
verdict: GAPS_FOUND
reviewer_model: opus
reviewer_self_id: Claude Opus 4.8 (claude-opus-4-8)
reviewed_file: /home/knatte/Code/loomyard/wts/prowler/_mill/discussion.md
date: 2026-07-29
```

## Findings

### [GAP] `${CLAUDE_PLUGIN_ROOT}` unverified; weblens parity claim is false
**Section:** skill-contract / build-on-first-run
**Issue:** The pinned invocation `bash "${CLAUDE_PLUGIN_ROOT}/scripts/run.sh"` (and the binary path `${CLAUDE_PLUGIN_ROOT}/bin/prowler`) is claimed to "match how weblens already invokes its own run.sh", but weblens' SKILL.md actually uses `${CLAUDE_SKILL_DIR}/../../scripts/run.sh` and its run.sh self-locates via `$0` — `CLAUDE_PLUGIN_ROOT` is never used, so parity is asserted for an unproven variable and an unset value would make the invocation resolve to `/scripts/run.sh` and fail.
**Fix:** Either add a "plan must confirm `${CLAUDE_PLUGIN_ROOT}` resolves in the skill's bash context at install" obligation (symmetric to the marketplace-`source` confirmation already required), or adopt the weblens-proven pattern (`${CLAUDE_SKILL_DIR}`-relative invocation + run.sh deriving `bin/` from `$0`), which removes the env-var dependency entirely.

### [NOTE] settings.json `go`-build permission not stated
**Section:** skill-contract
**Issue:** Build-on-first-run runs `go build` inside run.sh, but the permissions block is only described as allowing `Skill(prowler:*)` and the bash/binary invocation; weblens' analog lists `Bash(bash *)` explicitly, and whether the child `go` process is covered by the top-level `Bash(bash *)` grant is left implicit.
**Fix:** State that `settings.json` permits `Bash(bash *)` (covering the child `go build`), mirroring weblens' `permissions.allow` shape, so the plan pins the exact allow-list.

## Verdict

GAPS_FOUND
One load-bearing invocation depends on an unverified env var justified by a false weblens-parity claim.
MILL_REVIEW_END
