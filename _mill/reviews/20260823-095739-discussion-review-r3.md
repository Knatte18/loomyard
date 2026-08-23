MILL_REVIEW_BEGIN
# Review: landing: parent-fabric resolution chain

```yaml
duration_s: 302.0
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: claude-opus-5 (Anthropic)
reviewed_file: _mill/discussion.md
date: 2026-08-23
```

## Findings

### [BLOCKING:design] Deps drift-guard test has no stated tier or fixture
**Section:** Testing → `internal/loomcli`
**Issue:** The prescribed assembly function "takes the location, the opened handle, the registry, the runner, and the config", so the drift guard's "every field except `PushSkipped` is non-zero" assertion needs a real `*fabricengine.Fabric` — `newPaired` stat-checks both sides, `CurrentBranch()`/`RemoteURL("origin")` read real git state, and `Config` comes from strict `configengine.Load` (`landingshed/config.go:39`), so a non-zero `TaskBranch`/`OriginURL`/`Config` is unreachable without a wired hub fixture; the section never says whether this test is tagged, while its opening paragraph is about keeping this package's tests tier 1, and neither the Test Tier Purity nor the Hermetic Git Test Environment Invariant appears in the Constraints section.
**Fix:** Decide and state either that the seam takes plain `taskBranch`/`originURL`/`Config` values (keeping the drift guard untagged and fixture-free, with the handle reads staying in `drive.go`) or that the drift guard is an `//go:build integration` hubforge test with a `TestMain` calling `gitkit.HermeticGitEnv`, and add the two invariants to the Constraints section.

### [NIT:decision] No disposition for the two eager scalar reads' errors
**Demoted-from:** BLOCKING
**Section:** Decisions → `two-opens-in-drive-rather-than-a-shared-handle`; Technical context → field table
**Issue:** The table sources `TaskBranch` from `handle.CurrentBranch()` and `OriginURL` from `handle.OriginURL()`, both `(string, error)`, and no decision states what `drive` does with either error — unlike `ReadOrigin`, whose three returns get a whole decision; `gitrepo.RemoteURL` errors outright when no `origin` remote is configured (`remote.go:29-37`), so as written a hub without that remote makes `lyx loom drive` refuse before any producer runs, even though only the `Publish` row consumes the value.
**Fix:** State the disposition explicitly — envelope refusal for both, or an empty-string pass-through for `OriginURL` that lets `Publish` be the layer that refuses — and say which, with the reachability of a remote-less warp checkout named.

### [NIT:consistency] Two cited line numbers are stale
**Section:** Decisions → `push-uses-the-rebase-free-warp-primitive`; Q&A log
**Issue:** `publish.go:120` is cited for the `errors.Is(err, gitrepo.ErrPushRejected)` branch, which is at `publish.go:125`; `finalize.go:120` is cited for the `parentOpener()` error path, which is at `finalize.go:122-124`. Every other citation checked (`run.go:76`, `mergeresolve/deps.go:46`, `configreg.go:47`, `enforcement_test.go:597/654/699`) is exact.
**Fix:** Correct both to the actual lines, or drop the line suffix and cite the symbol only.

### [NIT:scope] Assembly function's parameter list is incomplete
**Section:** Testing → `internal/loomcli`
**Issue:** The named parameters (location, handle, registry, runner, config) do not cover `WebsterDir`/`StencilsDir`, which the field table sources from `c.runDeps.Geom`, nor the already-resolved `parentBranch` the refusal guard produces.
**Fix:** Extend the stated signature to include the geometry value and the resolved parent branch, so the plan writer does not have to re-derive it.

## Verdict

REQUEST_CHANGES
Two gaps: the drift-guard test's tier/fixture, and the two scalar reads' error disposition.
_Note: 1 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 1._
MILL_REVIEW_END
