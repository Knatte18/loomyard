MILL_REVIEW_BEGIN
# Review: Shed-generic watchdog for ly-drive and loom's CLI verbs — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: Claude Opus 5 (claude-opus-5)
reviewed_file: plan/
date: 2026-09-19
```

## Findings

### [BLOCKING:scope] Batch 4 breaks test compilation it never declares
**Location:** batch 4, cards 22/23/26/27
**Issue:** Cards 22/23 delete `runCmd`, `stepCmd`, `statusCmd`, `pauseCmd`, `stepEnvelope`, `stepKinds`, the five `stepKind*` constants, `renderStatusLine`, `statusUnavailableLine` and `printStatusLinesOnChange`, but `internal/loomcli/cli_test.go:220-230` uses the method expressions `(*loomCLI).runCmd/pauseCmd/statusCmd`, `internal/loomcli/status_test.go:58,92,112,174` calls `renderStatusLine`/`statusUnavailableLine`/`printStatusLinesOnChange`/`c.statusCmd()`, and `internal/loomcli/step_test.go:38,238,253,312` calls `stepEnvelope`/`stepKinds`/`stepKindBusy`/`c.stepCmd()` — none of those three files appears in any batch-4 card's `Edits:`, and card 27 forbids touching them ("Change no other assertion in either package"). Same gap in lifecyclecli: card 26 deletes `runCmd` while `internal/lifecyclecli/run_test.go:95,117,134,161,196` and `lifecycle_integration_test.go:200` call `c.runCmd()`.
**Fix:** Give cards 22/23/26 explicit `Edits:` entries and requirements for the in-package test call sites they orphan, and carve them out of card 27's no-other-change rule as mechanical retargeting rather than assertion change.

### [BLOCKING:design] Card 25 declares one Arm where card 21 proved two are needed
**Location:** batch 4, card 25
**Issue:** Card 21 states the `c.arm` worker / exported `Arm` wrapper split is "required, not stylistic" because the pre-run must wire its own receiver; card 25 declares only a package-level `func Arm(...)`, tells `resolvePersistentPreRun` to call it, and then demands "the receiver whose closures the returned hooks capture must be the one the pre-run holds" — which a package-level `Arm` cannot deliver, since `wire` is a method (`internal/lifecyclecli/wire.go:46`) and the hooks close over `c.abandonedSession`/`c.env`/`c.shedPaths`. The pre-run's own `c.location`/`c.slug` assignments (`cli.go:115-116`) would also stop happening.
**Fix:** Mirror card 21's split in card 25 — an unexported `(c *lifecycleCLI) arm(...)` worker plus a thin exported `Arm` wrapper that constructs its own receiver — and state which receiver each path uses.

### [BLOCKING:design] Lifecycle's never-run-slug status cannot reach `found: false`
**Location:** batch 3 card 18; batch 4 cards 25/27
**Issue:** `internal/state/state.go`'s `ReadJSONStrict` calls `lock.AcquireReadLock(lockPath)` with no `os.MkdirAll` (its own doc: "does not create missing parent directories"), and `internal/lock/lock.go`'s `AcquireReadLock` is `flock.New(lockPath)` + `RLock()`, which creates the lock file but never its parent. `internal/lifecyclecli/paths.go` puts `StatusFile` and `StatusLock` both under `LifecycleDir`, which nothing creates before a run — only `state.UpdateJSON`'s own `MkdirAll` does. With `EnsureStatusLockDir: false` (card 25), `lyx lifecycle status <never-run-slug>` therefore errors in lock acquisition rather than emitting `found: false`, so card 18's "directory's absence after a status read against a never-run slug" assertion and card 27's `--watch`-exits-immediately-with-`found: false` case are mutually unsatisfiable.
**Fix:** Decide and state the disposition — either lifecycle's told boolean is set, or the absent-file path is defined over an existing per-slug directory — and reword the two test cases to match.

### [BLOCKING:scope] Batch 6 verify never runs the Markdown Link Integrity checker
**Location:** batch 6, cards 38/39 and the batch `verify:`
**Issue:** Card 38 adds links to `docs/overview.md` and `manifest/` docs under the explicit constraint "the Markdown Link Integrity checker must stay green", but that checker is `internal/lyxcwd/docslink_test.go` (its own header names the invariant), and the batch's `verify: go test ./cmd/lyx/... ./tools/...` does not include `./internal/lyxcwd/...`. Card 39 pushes it to a manual grep-and-run step outside the gate.
**Fix:** Add `./internal/lyxcwd/...` to batch 6's `verify:` and name the test file in card 39 instead of telling the implementer to locate it.

### [NIT:consistency] ensureStatusLockDir arity differs between cards 13 and 15
**Location:** batch 3, cards 13 and 15
**Issue:** Card 13 declares `ensureStatusLockDir(prefix, statusLockPath string) error`; card 15 calls `ensureStatusLockDir(spec.StatusLockPath)` with one argument.
**Fix:** Make card 15's call site match card 13's two-argument signature.

### [NIT:consistency] Batch 4 names smoke-tagged suites as its proof but never runs them
**Location:** batch 4, Batch Scope and card 28
**Issue:** `internal/loomcli/smoke_test.go`, `smoke_bootstrapwiring_test.go`, `smoke_attachprobe_test.go` and `smoke_operatorstrand_test.go` all carry `//go:build smoke`, so batch 4's `verify:` (default tier plus `-tags integration`) never compiles them, yet the Batch Scope lists `smoke_test.go` among the suites whose passing "is the proof" and card 28 says to confirm it.
**Fix:** Drop the smoke files from the batch's proof list, or add a scoped `-tags smoke` run over the two packages.

### [NIT:decision] The sixth hook PostStep has no Shared Decision
**Location:** overview `## Shared Decisions`; batch 3 card 10
**Issue:** `_mill/discussion.md`'s `optional-hooks-for-module-specific-work` Decision fixes the hook set at five and says "`loomcli` fills all five"; the plan adds a sixth, `PostStep`, justified only inside card 10, while a comparable discussion deviation (`Result` vs `RunResult`) did get its own Shared Decision.
**Fix:** Record `PostStep` as a Shared Decision in `00-overview.md` with the `recordStepHandoff` placement rationale.

### [NIT:decision] lifecycleVerbTexts' step fields have no stated disposition
**Location:** batch 4, card 26
**Issue:** `VerbTexts` carries `Use`/`Short`/`Long` per verb for all four verbs (card 10), but card 26 declares `lifecycleVerbTexts` for `run`, `status` and `pause` only and discards the returned `step` command without saying what its texts should be.
**Fix:** State that lifecycle leaves the `step` texts empty and never adds the returned `step` command, so the blank `Short` never reaches the live tree `drift_test.go` walks.

## Verdict

REQUEST_CHANGES
Four blocking gaps: orphaned test call sites, lifecyclecli's Arm split, never-run-slug status, and batch 6 verify.
MILL_REVIEW_END
