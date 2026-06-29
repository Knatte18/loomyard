MILL_REVIEW_BEGIN
# Review: Rename Cobra modules to <module>cli, extract kernels as <module>engine

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-06-29
```

## Findings

### [GAP] ghissues seam split across cli/engine unresolved
**Section:** Technical context (ghissues) / Testing
**Issue:** `internal/ghissues/ghissues.go` mixes a cli-side seam (`var stdin`, read by `runCreate` in cli.go:94) with engine-side seams (`var runGH`, `createIssue`); moving the file wholesale to `ghissuesengine` breaks `runCreate`, and the white-box `cli_test.go` (package `ghissues`) — which the discussion places in `ghissuescli` — swaps `runGH` and so will not compile against an unexported engine seam.
**Fix:** Decide explicitly: place `stdin` in `ghissuescli`, export `runGH` as a settable engine seam (e.g. `ghissuesengine.RunGH`) and/or relocate the pipeline tests to `ghissuesengine`, rather than listing `cli_test.go → ghissuescli` and `ghissues.go → ghissuesengine` as atomic moves.

### [NOTE] Stale comment-ref list is incomplete
**Section:** Scope (comment-only references) / Technical context (testtiming)
**Issue:** `internal/paths/paths.go:344` ("seeders in internal/warp") and `cmd/testtiming/main.go:36,180` (illustrative `internal/board`) reference renamed packages but are absent from the discussion's stale-comment list (lyxtest/doc.go, sandbox, main_test.go).
**Fix:** Add paths.go:344 to the comment-accuracy sweep and note the testtiming illustrative comments will read as nonexistent packages post-rename.

## Verdict
GAPS_FOUND
One unresolved cli/engine seam-placement decision for ghissues blocks plan writing.
MILL_REVIEW_END
