MILL_REVIEW_BEGIN
# Review: Facilitate Linux support (Win11-side prep) — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-07-10
```

## Findings

### [BLOCKING] Required-subcommand set omits has-session/kill-session
**Location:** Batch 2 card 6; Batch 3 cards 9 & 10
**Issue:** The probe's `requiredSubcommands`, the doc.go contract list, and the integration test all enumerate the same 12 commands but omit `has-session` and `kill-session`, both of which the engine provably calls (`overlay.go:91` has-session; `lifecycle.go:142`/`:415` kill-session) — so the set does not "cover the engine's dependency set" as card 6 claims and the godoc is inaccurate, defeating the fail-loud/drift-canary purpose.
**Fix:** Add `has-session` and `kill-session` to card 6's `requiredSubcommands`, card 9's subcommand list, and card 10's exercised set.

### [NIT] Newly-unused `strings` import not removed (cards 12, 18)
**Location:** Batch 4 card 12; Batch 5 card 18
**Issue:** Card 12 removes `pwshSingleQuote` (the sole `strings` user in `command.go`, line 66) and card 18 replaces the inline `strings.ReplaceAll` calls (`launchers.go:43,76`); each leaves `strings` imported-but-unused, which fails `go build`/`go test`. Card 2 explicitly called out import cleanup; these two do not.
**Fix:** Instruct removing the now-unused `strings` import in `command.go` and `launchers.go`.

### [NIT] Card 12 commit leaves package non-compiling
**Location:** Batch 4 cards 12 & 13
**Issue:** Card 12 changes `buildLaunchCmd`/`buildResumeCmd` to take a leading `sh shell.Shell` arg but defers updating the sole production caller (`Prepare` in `claudeengine.go`) and the tests to card 13, so card 12's per-card commit does not build.
**Fix:** Fold the `Prepare` call-site update into card 12 (or merge cards 12+13) so each commit compiles.

## Verdict

REQUEST_CHANGES
Probe/contract under-enumerate the engine's psmux subcommand dependency set; two minor import/atomicity nits.
MILL_REVIEW_END
