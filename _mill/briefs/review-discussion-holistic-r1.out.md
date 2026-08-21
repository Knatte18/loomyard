MILL_REVIEW_BEGIN
# Review: Shed recipe: engine registry

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-21
```

## Findings

### [BLOCKING:scope] Webster's backing constructor is not on the export list
**Section:** "`loomshed`'s five constructors become exported" / Technical context table
**Issue:** The table says the `Webster` entry wraps loom's lazy wrapper `newWebsterProducer` (`internal/loomshed/webster.go:41`), but that constructor is unexported and is not among the five named for export (`newLoomPreflight`, `newBatchifier`, `newDiscussionValidate`, `newPlanValidate`, `newStub`) — six unexported constructors exist in the package.
**Fix:** State whether `newWebsterProducer` is also exported (making it six) or the `Webster` entry instead calls `shedadapters.NewWebsterProducer` directly, losing lazy `batcher.Active` resolution.

### [BLOCKING:design] `Env`'s flat fields cannot express per-row geometry
**Section:** "Live seams travel in a told `Env`"
**Issue:** `Env` carries one `RunDir` shared by all `Bouncer`/`BurlerRound` rows, yet `RunDir` is per-row: `shedadapters` writes `round-N-bouncer-verdict.md`/`-ledger.md`/`-focus.md` at fixed names inside `RunDir` (`internal/shedadapters/round.go:49-64`), so two Bouncer rows (Discussion-Review and Plan-Review) sharing one `Env.RunDir` overwrite each other's round state. The discussion's own `report_name` rationale ("it is per-row, not per-run") contradicts putting `RunDir` in `Env`.
**Fix:** Decide how per-row geometry is expressed — e.g. `Env` carries a base run dir and `Config` a relative per-row subdir the entry joins — and say which entries take which.

### [BLOCKING:design] No rule for per-row absolute-path inputs
**Section:** "The `SingleLLM` entry builds its `SpecSource`" / "`Bouncer`'s `ReportName` closure"
**Issue:** `NewBouncer` requires a non-empty `ArtifactPaths` whose every entry is absolute (`internal/shedadapters/bouncer.go:91-103`); `Env` has no such field and `manifest/designs/shed-recipe.md:17` bars absolute paths from `Config`. Symmetrically, `SingleLLMProducer.Call` rejects any non-absolute `spec.OutputFiles` entry outright (`internal/shedadapters/singlellm.go:71-75`), so the decided "worktree-relative `output_files`" produces a producer that always errors, and `Env.WorktreeRoot` is annotated for `PlanValidate` only.
**Fix:** Decide the relative-to-absolute resolution rule (which `Env` root each entry joins row-relative `Config` paths against) and name the `Env` fields it requires.

### [BLOCKING:design] `BurlerRound`'s `Config` key set is undecided
**Section:** Scope / Technical context table
**Issue:** `SingleLLM` and `Bouncer` get explicit `Config` key lists, but nothing says how `burlerengine.Profile` (ten fields incl. two `FileSet`s) and `burlerengine.RunOpts` (`Model`/`Effort`/`Timeout`/`Round`) reach the `BurlerRound` entry; `internal/burlercli/run.go:29-70`'s `profileYAML` is an existing serializable shape with no stated disposition (reuse, duplicate, or ignore).
**Fix:** Name `BurlerRound`'s `Config` keys and state whether the burlercli profile shape is reused or deliberately not.

### [BLOCKING:consistency] `name` is not threadable for `Publish`/`Finalize`
**Section:** "Fixed constructor signature" / "`landingshed.Deps` rides `Env`"
**Issue:** The rationale claims `name` is "threaded straight into each producer's own name parameter", but `landingshed.NewPublish`/`NewFinalize` take only `Deps` (`publish.go:60`, `finalize.go:69`) and `landingshed.Deps` has no `Name` field — the producer's identity is the package const `publishName = "Publish"` (`publish.go:31`), so a recipe row's `Name` is silently dropped for two entries.
**Fix:** State the disposition — accept that `name` is ignored for these two (and that a row must therefore be named `Publish`/`Finalize`), or require a `Name` field on `landingshed.Deps`.

### [NIT:consistency] "loomshed's existing tests pass unchanged" is not accurate
**Section:** Testing → "Scenarios that must not be missed"
**Issue:** Seven `internal/loomshed/*_test.go` files call the unexported constructors directly (`stub_test.go`, `batchifier_test.go`, `planvalidate_test.go`, `loompreflight_test.go`, `discussionvalidate_test.go`, `webster_test.go`, `resume_test.go:343-347`), so the rename requires mechanical test edits; only `New`-driven assertions stay untouched.
**Fix:** Reword to "assertions unchanged, call sites renamed" so the behaviour-neutrality claim stays true.

## Verdict

REQUEST_CHANGES
Registry shape is sound, but per-row geometry, path resolution, and two entries' inputs are unresolved.
MILL_REVIEW_END
