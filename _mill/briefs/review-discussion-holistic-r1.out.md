MILL_REVIEW_BEGIN
# Review: burlerengine + perchengine told-geometry

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), presented to me as Opus 5; self-assessment uncertain beyond "a Claude Opus-class model"
reviewed_file: _mill/discussion.md
date: 2026-08-18
```

## Findings

### [BLOCKING:consistency] burlerengine smoke files are `smoke`-tagged, not `integration`
**Section:** Testing → `internal/burlerengine` bullet 4, and Testing → Verify
**Issue:** `smoke_round_test.go:1` and `smoke_cluster_test.go:1` both carry `//go:build smoke`, and they are the only tagged files in the package — so `go test -tags integration ./internal/burlerengine/...` (and `go test ./...`) never compiles the two `burlerengine.New(runner, h.Location, …)` lines at `smoke_round_test.go:321` / `smoke_cluster_test.go:140` that this task must edit, leaving a broken build undetected by every command in Verify.
**Fix:** Correct the "integration-tier" wording to `smoke`, and add a `-tags smoke` compile/vet step over `./internal/burlerengine/...` to the Verify list.

### [BLOCKING:consistency] `AnchorRoot` diverges from the cited `reedengine` precedent unexplained
**Section:** Decisions → "told-geometry carrier"; Constraints → Cwd Resolution Invariant
**Issue:** The discussion names `reedengine.Geometry` + `hubgeom.ReedGeometry` as the shipped precedent and `hubgeom` as "the pattern to copy exactly", but `reedengine/geometry.go:22` spells that field `AnchorPath` (matching `lyxcwd`'s `AnchorPath()` accessor) while both new structs use `AnchorRoot`; the perch `GateDir` divergence gets its own Decision and this one gets none, and the value is a worktree *subdirectory*, which sits awkwardly against the invariant's "`root` always means the git worktree/repo root" rule the same discussion quotes.
**Fix:** State a disposition — either adopt `AnchorPath` for both engines, or record why the two told-geometry structs deliberately use different names for the same value that T8's pinned-values table will join on.

### [NIT:scope] `perchengine/run_test.go` call-site count understated ~3x
**Section:** Technical context → Test call sites
**Issue:** The discussion says "~10 `New(fb, qs, Config{}, layout, Options{})` call sites"; the tree shows ~31 `New(fb…` construction sites and 33 `newTestLayout` references in that one file.
**Fix:** Correct the count so the mechanical-edit volume in `run_test.go` is not underestimated.

### [NIT:scope] Stale `*lyxcwd.Location` prose outside `doc.go` not listed
**Section:** Scope → In, final bullet ("`doc.go` updates in …")
**Issue:** `perchengine/engine.go:11` ("the `*lyxcwd.Location` it holds"), `:81` ("the caller that already holds the `*lyxcwd.Location`") and `hubgeom/hubgeom.go:1-2` ("hubgeom.go implements ReedGeometry") all become false, and the scope bullet names only `doc.go`.
**Fix:** Extend the docs bullet to cover file-header/function comments in the edited production files, not just `doc.go`.

## Verdict

REQUEST_CHANGES
Two blocking issues: an unrunnable smoke-tag verify step and an unexplained geometry field-name divergence.
MILL_REVIEW_END
