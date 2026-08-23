MILL_REVIEW_BEGIN
# Review: landing: parent-fabric resolution chain

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude Opus 5 (Anthropic)
reviewed_file: _mill/discussion.md
date: 2026-08-23
```

## Findings

### [BLOCKING:consistency] PushBranch closure's handle source contradicts the seam
**Section:** "Where each `Deps` field comes from" table vs `assembly-seam-takes-plain-values` / `two-opens-in-drive-rather-than-a-shared-handle`
**Issue:** The table sources `PushBranch` as a "closure over the new `handle.PushBranch(...)`" — the handle `drive` opened — but the stated `landingDeps` signature takes no handle and Testing says `l` "backs the three closures", which implies a *third* `fabricengine.Open` inside the push closure that the two-opens decision neither names nor costs, and whose `Open`-error path is unstated.
**Fix:** Pick one and state it: pass a prebuilt `pushBranch func() error` (or the handle) into `landingDeps`, or declare the closure opens from `l` and widen the two-opens decision to cover the third open and its error handling.

### [BLOCKING:decision] Config load and Open errors in `drive` have no disposition
**Section:** `Deps` table (`Config` row) + `scalar-read-errors-refuse-or-defer-by-consumer`
**Issue:** `landingshed.LoadConfig` routes through strict `configengine.Load` (`internal/landingshed/config.go:39`), which returns `config file <…>/landing.yaml not found; run "lyx config reconcile"` on an unreconciled hub — a new hard-failure mode for `lyx loom drive` — and `fabricengine.Open(c.location)`'s own error is equally undisposed, while `ParentBranch`, `CurrentBranch()`, and `OriginURL()` each get an explicit, argued disposition.
**Fix:** State both dispositions (envelope refusal, presumably) and whether an absent `landing.yaml` is an acceptable drive-blocking refusal rather than a degrade.

### [NIT:consistency] "performs no I/O" vs the env read inside `landingDeps`
**Section:** `assembly-seam-takes-plain-values` + Testing / `internal/loomcli`
**Issue:** The signature carries no `pushSkipped`, so `fabricengine.EnvSyncOptions()` must be called inside `landingDeps` (consistent with the "two-case assertion driving the env var set and unset"), which contradicts "takes already-resolved plain values… performs no I/O" and makes the tier-1 drift guard depend on process env.
**Fix:** Add a `pushSkipped bool` parameter, or reword the claim to "no filesystem or git I/O".

### [NIT:scope] No test named for the two new `Fabric` methods
**Section:** Testing
**Issue:** `Fabric.OriginURL()` and `Fabric.PushBranch(opts)` are new exported surface on `internal/fabricengine`, but Testing names coverage only for the matcher, `OpenParent`, `LoomScratchDir`, and the loomcli seam.
**Fix:** State that the two delegations are deliberately covered only by `internal/gitrepo`'s existing `RemoteURL`/`PushRebaseFree` tests, or name their own coverage.

### [NIT:consistency] `deps.go` comment count is off by one
**Section:** "Files that will be touched"
**Issue:** Only one comment in `internal/landingshed/deps.go` defers to "the next roadmap item" (line 75); the second deferral lives in `internal/loomcli/wiring.go:111`, which the same list already names separately.
**Fix:** Say "the `OpenFabric`/`OpenParentFabric` field doc's deferral, plus its laziness wording".

## Verdict

REQUEST_CHANGES
Two unresolved wiring/error-disposition questions in `drive`; the rest verifies against source.
MILL_REVIEW_END
