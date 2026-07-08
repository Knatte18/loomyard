I have thoroughly reviewed the plan against the discussion, constraints, and source files. The plan is well-grounded, accurate about the codebase, and covers all task requirements. No blocking issues found.

MILL_REVIEW_BEGIN
# Review: Build burler - the review+fix round worker — holistic

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-07-08
```

## Findings

### [NIT] Cards 4 & 5 omit intra-package prior-card files from Context
**Location:** Batch 2, cards 4 (prompt.go) and 5 (engine.go)
**Issue:** Requirements reference identifiers defined in earlier same-batch cards — `FixScopeSource`/`Profile` (profile.go, card 1), `reviewPromptTemplate` (template.go, card 3), `composePrompt`/`ParseReview`/`Verdict`/`Finding`/`ErrClusterUnsupported` — but Context lists only discussion.md plus stencil/shuttle files, not those burlerengine files.
**Fix:** The files are same-package and on disk when these cards run, so this is safe; optionally add `internal/burlerengine/profile.go` / `template.go` / `verdict.go` to Context to protect a cold-start dispatch.

### [NIT] Burler JSON envelope key casing diverges from shuttlecli
**Location:** Batch 3, card 7 (run.go output.Ok fields)
**Issue:** Card prescribes snake_case keys (`session_id`, `strand_guid`, `last_assistant_message`, `review_path`) while shuttlecli's Ok envelope uses camelCase (`sessionId`, `guid`, `lastAssistantMessage`); the card also says "follow shuttlecli's shape file-for-file."
**Fix:** Either match the existing camelCase envelope convention or note the snake_case choice as deliberate in the card.

### [NIT] "clihelp.Abort pattern" is imprecise for RunE errors
**Location:** Batch 3, card 7
**Issue:** Card says RunE errors exit "via the clihelp.Abort pattern," but shuttlecli's run.go RunE uses `clihelp.SetExit(ctx, output.Err(...))`; `Abort` is only for PersistentPreRunE.
**Fix:** State RunE errors go through `output.Err` + `clihelp.SetExit`; PersistentPreRunE failures use `output.Err` + `clihelp.Abort` (as the card already says for pre-run).

## Verdict

APPROVE
Plan is accurate, constraint-compliant, and complete; only cosmetic nits remain.
MILL_REVIEW_END
