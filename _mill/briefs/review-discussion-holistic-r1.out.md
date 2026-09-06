MILL_REVIEW_BEGIN
# Review: Reed and Fabric as standalone modules: public API design

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-09-06
```

## Findings

### [BLOCKING:consistency] Session slice contradicts measured loomcli usage
**Section:** `reed-publishes-named-slices-consumers-may-still-narrow` (vs `reed-preparation-work-is-worth-doing-regardless` (c) and Technical context → Reed → consumer shapes)
**Issue:** The session slice is claimed to be "exactly what `loomcli` reaches for today (`Up`, `Down`, `Resume`, `Status`, `SessionName`, `Socket`, `TmuxPath`, `AttachArgv`)", but grep of `internal/loomcli` shows `Up` (`drive.go:64`, `run.go:145`), `Status`, `RemoveStrand`, `AddStrand`, `TmuxPath`, `AttachArgv` and *no* call to `Down`, `Resume`, `SessionName` or `Socket`; decision (c) separately says "six methods", so the discussion states two different slice memberships.
**Fix:** Pick one membership and state its derivation — either the six methods `loomcli` actually calls (which overlap the transport slice on `AddStrand`/`RemoveStrand`/`Status`) or a deliberately wider lifecycle slice justified by `reedcli` usage — and say whether slice overlap is permitted and in which package the slices are declared.

### [BLOCKING:consistency] CleanClaudeEnv is not an external identifier
**Section:** Technical context → Reed → "External contract footprint: 15 distinct exported identifiers"
**Issue:** `CleanClaudeEnv` is listed among the 15 identifiers "referenced by production code outside the package", yet the same section states (correctly) that "its only production caller is `internal/reedengine/lifecycle.go:299`"; repo-wide grep confirms no production reference outside `reedengine` (only tests and a prose mention in `internal/burlerengine/doc.go`).
**Fix:** Drop it from the external list and restate the headline as 14, or redefine the metric as "exported identifiers referenced anywhere outside the package including tests" and re-derive every count under that definition.

### [NIT:consistency] Engine method count is 17, not 18
**Section:** Technical context → Reed → "Public surface: … `*Engine` with 18 methods"
**Issue:** `internal/reedengine` declares 17 exported `*Engine` methods (`Socket`, `SessionName`, `TmuxPath`, `AddStrand`, `UpdateStrand`, `RemoveStrand`, `AttachArgv`, `SendText`, `SendKey`, `CapturePane`, `Up`, `Resume`, `Down`, `Status`, `Watch`, `HeaderText`, `ValidateHeader`); no value-receiver methods exist.
**Fix:** Correct to 17 and name the producing command, per the discussion's own reproducibility rule.

### [NIT:consistency] Link test does not validate GitHub-issue links
**Section:** Testing → "Link integrity"
**Issue:** The bullet implies `TestEnforcement_MarkdownLinks` checks links "to GitHub issues", but `internal/lyxcwd/docslink_test.go:363` skips every `http://`/`https://`/`mailto:` target, and a dedicated subtest asserts that skip.
**Fix:** Say that external links are unchecked by the test and are a manual-review item, or state that the doc uses no external links.

## Verdict

REQUEST_CHANGES
Two measured claims contradict the discussion's own source-verified numbers; correct before plan writing.
MILL_REVIEW_END
