MILL_REVIEW_BEGIN
# Review: dev/test lyx.exe separated from production deploy

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-25
```

## Findings

### [GAP] Guard misses exec.Command("lyx") bare-PATH form
**Section:** Testing → "Guard test for the new invariant"
**Issue:** The guard is specified to scan only `lookPath("lyx")`, but `main.go:34` resolves the binary today via `exec.Command("lyx", "warp", "clone", …)` — Go's `exec.Command` LookPath's a separator-free name, so this is an equivalent bare-PATH footgun the guard would not catch on regression.
**Fix:** Widen the guard (and invariant statement) to also forbid `exec.Command("lyx"`/`exec.CommandContext("lyx"` outside `resolveLyx`, not just `lookPath("lyx")`.

### [NOTE] Source marker must reach the report JSON struct, not just binaryInfo
**Section:** Decisions → fingerprint-source-marker
**Issue:** The decision promises `Source` "appears in sandbox-report.json's meta.fingerprint" but names only `binaryInfo` + `header()`; the JSON is serialized through a distinct `reportFingerprint` struct (`report.go:43`) stamped in `fetchReport` (`report.go:150`), which the decision does not mention — a literal reading would add the field only to the header and leave the JSON without it.
**Fix:** State that `reportFingerprint` gains a `Source` field and `fetchReport` stamps it from the resolved `source`.

## Verdict

GAPS_FOUND
One guard-coverage gap leaves the new invariant's enforcement blind to a bare-PATH form present today.
MILL_REVIEW_END
