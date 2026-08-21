MILL_REVIEW_BEGIN
# Review: Shed recipe: engine registry

```yaml
duration_s: 181.0
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-21
```

## Findings

### [BLOCKING:design] Bouncer's Config key set is never enumerated
**Section:** Decisions — `Bouncer`'s `ReportName` closure / relative-path table
**Issue:** `SingleLLM` and `BurlerRound` get complete key lists, but `Bouncer`'s keys appear only piecemeal (`report_name`, `rubric_stencil`, `artifact_paths`, `run_subdir`) and never as a closed set — yet `BouncerConfig.Model/Effort/Version` (`internal/shedadapters/bouncer.go:47-51`) are caller-supplied and recipe-authorable, and the strict unknown-key rule makes the key set the entry's contract.
**Fix:** Enumerate `Bouncer`'s full recognised key set explicitly, stating which `BouncerConfig` fields come from `Config` and which from `Env`.

### [BLOCKING:design] SingleLLM's geometry-derived token set is undefined
**Section:** Decision — The `SingleLLM` entry builds its `SpecSource` from a stencil
**Issue:** The decision says the closure fills the stencil with "static `tokens` merged over a fixed set of geometry-derived tokens supplied from `Env`" but never names that set; `stencil.Fill` hard-errors on a missing token, and testing item 4 asserts behaviour against a set the discussion does not define.
**Fix:** Name the geometry-derived token keys and their `Env` sources, or state explicitly that the set is empty and every token comes from `tokens`.

### [NIT:consistency] Burler profile paths escape the relative-path rule
**Demoted-from:** BLOCKING
**Section:** Decisions — relative `Config` paths / `BurlerRound`'s `Config`
**Issue:** `profile.target.paths` and `profile.fasit.paths` are `Config` path values, but the resolution table omits them, and `burlerengine.Profile.validate` already resolves relative entries against `worktreeRoot` and stats them for existence (`internal/burlerengine/profile.go:64-93`) — so "every entry joins the path and rejects absolute values" is stated as general yet contradicted for this entry.
**Fix:** State the disposition for `target`/`fasit` paths — left relative for `Profile.validate`, or joined and absolute-rejected by the entry — and add them to the table either way.

### [NIT:consistency] "No path is computed inside shedrecipe" contradicts the join decisions
**Demoted-from:** BLOCKING
**Section:** Constraints — Told-Geometry Invariant bullet
**Issue:** The bullet asserts "no path is computed inside `shedrecipe`", while the `run_subdir`, `artifact_paths`, and `output_files` decisions all have registry entries `filepath.Join` a told root with a relative `Config` value; the new CONSTRAINTS.md invariant text is derived from this sentence and would ship false.
**Fix:** Reword to the property actually held — every root is told and none is derived, joins onto told roots permitted — and pin that wording as the invariant text.

### [NIT:scope] Coverage guard's catch condition is unstated
**Section:** Decision — A coverage-guard test pins the registry against loom's current list
**Issue:** The guard is a hand-written engine-name-per-row table; if it only iterates its own rows, a row newly added to `loomshed.New` is not caught, which is the exact failure the rationale claims it prevents.
**Fix:** State that the guard compares its table's key set against the names in `loomshed.New`'s assembled list, failing on either direction of mismatch.

## Verdict

REQUEST_CHANGES
Two key-set gaps and two path-rule contradictions must be settled before plan writing.
_Note: 2 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 2._
MILL_REVIEW_END
