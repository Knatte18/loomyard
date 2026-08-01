MILL_REVIEW_BEGIN
# Review: fabric: audit and migrate all remaining direct git mutations onto Fabric

```yaml
verdict: APPROVE
reviewer_model: opus
reviewer_self_id: Claude Opus 4.8
reviewed_file: _mill/discussion.md
date: 2026-08-01
```

## Findings

### [NOTE] Guard can't catch re-added mutation in allowlisted files
**Section:** Decisions § regression-guard
**Issue:** The file-level allowlist exempts `gitquery.go`/`gitwrap.go` wholesale, so a future mutating `gitexec.RunGit(` re-added into those two files — exactly where the closed bug (`ResetHard`) lived — would pass the guard silently.
**Fix:** Record this residual coverage limit explicitly (mirrors tierpurity's file-allowlist tradeoff); note the read-only files' stability is what makes the risk acceptable.

### [NOTE] Guard rationale overstates the post-migration token absence
**Section:** Decisions § regression-guard (rationale)
**Issue:** "neither package ever constructs a standalone `gitrepo.Repo` (`gitrepo.New(`) or calls `gitexec.RunGit(` directly for anything, mutating or not" is contradicted by the surviving read-only exemptions (`gitwrap.go:27,40`, `gitquery.go:23,39,62`), which do call both tokens directly.
**Fix:** Reword to "except in the two allowlisted read-only files" so the plan-writer isn't misled about which files still carry the tokens.

## Verdict

APPROVE
Scope, decisions, and testing are sound; round-1 gaps closed; only two non-blocking notes.
MILL_REVIEW_END
