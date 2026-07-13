MILL_REVIEW_BEGIN
# Review: Speed up git-fixture tests: bench, analyse, hardlink

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-13
```

## Findings

### [NOTE] New hermetic guard file trips the tierpurity guard
**Section:** Decisions § hermetic-guard-and-constraints-entry / Scope › In
**Issue:** The new `cmd/lyx` guard is a tierpurity-style, untagged `*_test.go` that runs `go env GOMOD` via `exec.Command` and lists `gitexec.RunGit`/`exec.Command`/`lyxtest.Copy` as literal token data — all raw substrings the existing `TestTierPurity_UntaggedTestsSpawnNothing` bans; it is put on the *hermetic* guard's own allowlist but nothing adds it to tierpurity's `allowedSpawners`.
**Fix:** Note that the plan must also add the new guard file to `cmd/lyx/tierpurity_test.go`'s `allowedSpawners` (same commit) or the "both tiers green" gate fails.

### [NOTE] Layer B config-file lifecycle unspecified
**Section:** Decisions § two-layer-hermetic-mechanism / neutral-global-config-contents
**Issue:** The helper "writes one neutral global-config file per test process" from `TestMain`, but `TestMain` has no `t.TempDir` and typically ends in `os.Exit(m.Run())` (skips deferred cleanup) — the file's location and cleanup are undefined, so temp configs leak in `%TEMP%`.
**Fix:** Specify the helper's create path (e.g. `os.CreateTemp`) and either a returned cleanup handle invoked before `os.Exit`, or a documented accepted-leak.

## Verdict

APPROVE
Scope, decisions, and enforcement are sound; two non-blocking allowlist/lifecycle notes for the plan.
MILL_REVIEW_END
