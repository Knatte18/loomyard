I have verified the discussion's claims against the actual source. The `test-scheme.md` references exist exactly where the discussion says (docs/sandbox-howto.md:117, docs/sandbox-hub.md:101), `suiteFileName = "SANDBOX-SUITE.md"` and `//go:embed SANDBOX-SUITE.md` are confirmed, `runSuite` does no retrieval today, `binaryInfo` holds the four fingerprint fields, `sandbox.cmd` uses `pushd "%~dp0"` then `go run ./tools/sandbox -parent C:\Code %*`, and the gh/selfreport prose in both docs and the SANDBOX-SUITE body is as described.

MILL_REVIEW_BEGIN
# Review: Sandbox suite: emit findings JSON on the shared analysis contract

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-06-30
```

## Findings

### [NOTE] -loomyard flag: placement undecided + %~dp0 quoting
**Section:** Decision: scratch-destination / Technical context (main.go)
**Issue:** The flag is hedged as "top-level/suite", and `sandbox.cmd`'s `-parent C:\Code %*` (where `%*` already contains the `suite` token) forces a *top-level* flag; also `%~dp0` carries a trailing backslash that breaks cmd quoting if passed as `"-loomyard %~dp0"`.
**Fix:** State it is a top-level flag parsed by `fs` (like `-reset`), threaded to `runSuite`, and note the trailing-backslash/quoting handling in `sandbox.cmd`.

### [NOTE] Fetch-helper dest argument is described inconsistently
**Section:** Testing (lines 225/228) vs Decision: scratch-destination (line 86)
**Issue:** The helper param is called the "dest `.scratch` dir" yet the happy-path lands at `<dest>/.scratch/sandbox-report-<sha12>.json` and line 86 says `<loomyard-root>/.scratch/...` — so `dest` is ambiguously the loomyard root or the `.scratch` dir.
**Fix:** Pin one: `dest` = loomyard root, and the helper joins `.scratch` + `MkdirAll`s it.

### [NOTE] Stale prior sandbox-report.json not cleaned before launch
**Section:** Scope / Decision: fetch-only-on-clean-exit
**Issue:** SANDBOX-SUITE.md is overwritten each run, but nothing truncates a prior `sandbox-report.json`; an agent that exits 0 without rewriting it would have a stale findings list fetched and freshly fingerprinted.
**Fix:** Decide whether `runSuite` removes/truncates any pre-existing `sandbox-report.json` before launch so "clean exit, no write" surfaces as missing-report rather than stale data.

### [NOTE] "Present items array" check needs pointer/RawMessage
**Section:** Decision: validation-strictness
**Issue:** With `encoding/json`, an absent `items` and `items: []` both decode to an empty slice unless a `*[]Item`/`RawMessage` is used, so "require a present items array" cannot distinguish missing from empty as stated.
**Fix:** Note the decode shape needed to tell absent from empty, or relax the requirement to "decodes successfully + correct source".

## Verdict

APPROVE
Scope, decisions, and constraint coverage are complete; only minor clarifications remain.
MILL_REVIEW_END
