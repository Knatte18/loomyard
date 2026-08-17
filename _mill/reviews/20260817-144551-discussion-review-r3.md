MILL_REVIEW_BEGIN
# Review: config degrades to embedded template

```yaml
duration_s: 296.4
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude Opus 4.x-class model (Anthropic); exact build not self-verifiable
reviewed_file: _mill/discussion.md
date: 2026-08-17
```

## Findings

### [BLOCKING:design] Info-always-reaches-durable premise is conditional
**Section:** `### info-level-observability`
**Issue:** "an Info record **always** reaches the durable trace-file sink, at any verbosity" is false as stated — `internal/logger/sink.go:75-103`'s `ensureDurableSink` disarms (`sinkOK = false`) when `testing.Testing() && LYX_TRACE != "1"`, and also when `lyxcwd.Getwd()` or `lyxcwd.Resolve(cwd)` fails, which is exactly the config-less / outside-an-anchor case the fallback exists for; the record then reaches neither stderr (default Warn) nor any file.
**Fix:** Restate the premise with its condition (durable capture requires a resolvable `AnchorPath()`, and `LYX_TRACE=1` under `go test`), and say explicitly whether an unobservable fallback in true standalone mode (T5-T8) is accepted here or recorded as a watch item — plus that no test can assert the log.

### [NIT:scope] Own-loader class missing from the invariant's membership rule
**Demoted-from:** BLOCKING
**Section:** `### membership-rule-is-a-standalone-entry-point` / Scope/Out
**Issue:** A third loader class exists that neither pinned set covers: modules reading a module config via `configengine.ConfigFile` with their own absent-file fallback. The discussion disposes of two (`burlerengine/config.go:63`, `modelspec/load.go:24`) but never mentions `internal/scoutengine/load.go:23` (`servers.yaml`, already degrades on `os.ErrNotExist`), and the invariant's two-set classification plus the T10 `configengine.Load(`/`LoadOrTemplate(` set-equality guard are structurally blind to all three.
**Fix:** Name the own-loader class in the invariant text (with its own rule or an explicit "out of the guard's subject" clause) and give `scoutengine.LoadRegistry` a stated disposition alongside burler and modelspec.

### [NIT:consistency] "Complete" doc-surface table omits a false claim it sits beside
**Section:** Technical context → Doc surface
**Issue:** The table is declared the single authority and complete, but `docs/shared-libs/configengine.md:99-102` asserts `configengine` exports `LyxDirName` and is "the single declarer of this token" — false in source (`config.go` uses `lyxdirs.LyxDirName`; no such export) and contrary to the Lyxdirs Single-Declarer Invariant; lines 124/129 also quote an `in <dir>` suffix `config.go:31` never emits and a `lyx init` remedy no caller emits.
**Fix:** Either add these rows or state that pre-existing staleness outside the five task-caused rows is deliberately out of scope.

### [NIT:design] chmod-0o000 negative test breaks t.TempDir teardown
**Section:** Testing → absence-only discrimination
**Issue:** Chmod'ing the parent to `0o000` leaves it unreadable at `t.TempDir()` cleanup, so `RemoveAll` fails and the test fails even when the assertion passes; the stated escape hatch covers "construction impossible", not "cleanup fails".
**Fix:** Require the test to restore the mode in a `t.Cleanup` registered before the chmod.

## Verdict

REQUEST_CHANGES
One false observability premise and one unclassified loader class need resolution.
_Note: 1 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 1._
MILL_REVIEW_END
