MILL_REVIEW_BEGIN
# Review: gitexec: decide whether RunGit should return a typed error carrying stderr

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-11
```

## Findings

### [BLOCKING:consistency] Constraints section contradicts the decided guard fact
**Section:** `## Constraints` (gitrepo Client Boundary bullet) vs `guard-test-with-justification-comments`
**Issue:** The Constraints bullet still says the implementation "changes the *shape* of calls inside already-pinned methods, which the set-equality check tolerates — but it must confirm that, not assume it", while the decision section states as settled fact that `gitrepoboundary_test.go:167/174/177` all break and change in the implementation commit; I verified `gitexecTotal != 1`, the run-body assertion, and that `bodyCallsMethodOnReceiver(..., "run")` will not match `r.runChecked(`, so a migrated method silently drops out of the pinned set.
**Fix:** Delete or rewrite the superseded Constraints bullet to match the decided outcome (both assertions change; the pinned method set changes as methods migrate).

### [BLOCKING:design] Merge rule has no clause for helper-constructed errors
**Section:** `the-migration-is-a-two-message-merge-not-a-substitution` / `The migration recipe to record in the verdict`
**Issue:** The default rule assumes both branches are `fmt.Errorf(...: %s, stderr)`; `internal/fabricengine/warpprobe.go` builds all four of its exit paths through `wrapProbeError(weftURL, op, stderr, cause)` — a helper whose signature takes stderr and cause as separate parameters, so the merge collapses two calls into one and the helper itself must change shape. The document cites `wrapProbeError` three times, but only as a classifier trap, never as a merge case.
**Fix:** Add a clause covering error-constructing helpers that take stderr and cause separately, stating whether the helper is re-signatured to take a single `error` or kept and fed `err.Error()`.

### [BLOCKING:consistency] Marker template's mandated justification excludes a decided raw class
**Section:** `guard-test-with-justification-comments` vs round-3 `pull.go` disposition
**Issue:** The invariant requires `//gitexec:raw — <why a non-zero exit is not a failure here>`, but `gitrepo.Pull` (`pull.go:19`) and `Fetch` (`:33`) are decided **raw** precisely because a non-zero exit *is* a failure whose stderr is deliberately withheld (`pull_test.go:87/119` fail on `"fatal:"`). The mandated justification form cannot be truthfully filled at those two sites, and that wording is what lands in CONSTRAINTS.md.
**Fix:** Broaden the marker's stated justification to "why the raw form is correct here" and name the two raw classes (pure predicate; pinned deliberate-suppression contract).

### [NIT:consistency] `gitrepo.go:193` is not the same tri-state mapping as `:140`
**Section:** `predicate-sites-are-real-and-must-stay-expressible`
**Issue:** The doc says both `diff --cached --quiet` sites map exit 1 to `ErrIndexNotEmpty`; at `:193` (`StageAllAndCommit`) exit 0 returns `("", false, nil)` "nothing to commit" and exit 1 falls through to the commit — the answer codes are inverted relative to `:140`.
**Fix:** State the two mappings separately so the `errors.As` recovery is not transcribed with the wrong exit code.

### [NIT:decision] Roadmap link disposition at doc-deletion time unstated
**Section:** `verdict-doc-lifecycle` / `## Constraints` (Markdown Link Integrity)
**Issue:** `manifest/roadmap.md:75` links to `designs/gitexec-error-shape.md`; the doc says the link must survive *this* task but never says the implementation task must drop it when it deletes the doc, which would otherwise trip `TestEnforcement_MarkdownLinks`.
**Fix:** Add a one-line handoff note that the implementation task removes the roadmap link in the same commit as the deletion.

## Verdict

REQUEST_CHANGES
Two superseded/contradictory statements and one uncovered merge class must be resolved first.
MILL_REVIEW_END
