MILL_REVIEW_BEGIN
# Review: Rename the fabric host vocabulary to warp, and name the composite repo Fabric

```yaml
verdict: GAPS_FOUND
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class
reviewed_file: _mill/discussion.md
date: 2026-08-09
```

## Findings

### [GAP] Owner dirs weftname/boardengine omitted from scope
**Section:** Scope "In" / "Enforcement guard is tightened"
**Issue:** Removing the host-half owner skip makes the guard scan all six owner dirs, but `internal/weftname/weftname.go:10` ("host-worktree slug") and `internal/boardengine/board.go:15,17,25` ("host branch", "host's own default branch", "host/warp repo", "the host repo's") carry policed fabric-sense phrases in production comments — and neither package is in scope.
**Fix:** Add `internal/weftname` and `internal/boardengine` to the in-scope set (board.go:23/26 "hosts the wiki"/"wiki-hosting" are verb-sense and must be preserved by hand), or state why the tightening excludes them.

### [GAP] CONSTRAINTS.md:26 renames but is declared unaffected
**Section:** Constraints / "The ban list is not a rename target"
**Issue:** `CONSTRAINTS.md:26` (Cwd Resolution Invariant) names `HostLyxLink`/`HostJunctions`, which this task retires, yet the discussion lists only lines 175/180/188/200/214 as renaming and asserts "Cwd Resolution Invariant — unaffected".
**Fix:** Add line 26 to the hand-edited CONSTRAINTS list and correct the "unaffected" claim to "prose/identifier citation renames, semantics unchanged".

### [GAP] Token-boundary rule contradicts the `hostclean.go` case
**Section:** "Rename mechanism" (line 55) vs Testing (line 294)
**Issue:** The stated boundary rule requires `hostname`/`localhost`/`conhost` not to match, i.e. `host` followed by a lowercase letter is not a token — but that same rule cannot match the all-lowercase compounds `hostclean`/`hostlayout`, which appear in the swept content at `internal/fabricengine/hostclean.go:1` and `hostlayout.go:1` (each file's own doc comment names itself).
**Fix:** State the disambiguation explicitly — e.g. a `.go`-suffix-aware case, or hand-edit those two doc-comment lines in commit (b) alongside the `Moves:` pairs — and drop the claim that the one pass handles `hostclean.go`→`warpclean.go`.

### [GAP] Tightened guard does not encode "zero host"
**Section:** Testing, "Repo-wide completeness check"
**Issue:** The claim that the tightened test "encodes permanently" the zero-hits assertion overstates the mechanism read at `internal/lyxcwd/enforcement_test.go`: `hostGeometryIdentifiers` is five exact lowercased names (so `HostJunctions`, `hostPath`, `hostBare`, `CopyHostHub`, `HostFixture` are not caught), `*_test.go` is excluded from all three rules (line 868), and the tree scan covers only `internal/`+`cmd/` `.go` plus `internal/**/*.md` — never `docs/`, `README.md`, `manifest/`, `tools/sandbox/*.md`, or `post-checkout.sh`.
**Fix:** Either state the guard's actual reach honestly (and that docs/tests/shell stay review-obligation), or decide in-discussion to broaden `hostGeometryIdentifiers`, lift the `_test.go` exclusion for the host half, and extend the walk roots.

### [GAP] `tools/sandbox/*.go` in or out is unstated
**Section:** Scope / Documentation surface
**Issue:** The eight `SANDBOX-*-SUITE.md` templates are in scope, but the Go beside them is not mentioned, and `tools/sandbox/main.go` holds `hostURL`, `fabricHostURL`, `fabricHostDir`, `hostRepoDir` and user-facing "fabric hub host repo not found" (line 139) — while `CONSTRAINTS.md`'s owner set names `tools/` and `sandbox/`.
**Fix:** Decide explicitly, noting the exclude list is not empty there (`main.go:32` "the dedicated hub hosts fabric's stricter …" is verb-sense).

### [NOTE] Historical benchmark doc omitted from the doc list
**Section:** Documentation surface
**Issue:** `docs/benchmarks/test-suite-timing.md` contains `CopyHostHub` (line 936) and `TestRemoveHostJunctionRemoved`/`TestWeftRollbackOnPostHostCreateFailure` (748, 754, 854) — a historical record where renaming would falsify past run names.
**Fix:** Name it as a deliberate exclusion with that reason, rather than leaving it unlisted.

### [NOTE] Agent config files carry the phrase
**Section:** Documentation surface
**Issue:** `.claude/agents/crucible-reviewer-{low,medium,high,max,xhigh}.md:16` each read "This is a **host-repo** commit on the crucible worktree, never a weft-repo operation."
**Fix:** State whether `.claude/agents/` is in or out of the doc sweep.

### [NOTE] Compile is not full proof for merged names
**Section:** Testing, "verification is mechanical"
**Issue:** `go build` catches package-level redeclaration but not a local shadowing a same-named package-level symbol; the survey verified only the `hostPath`/`warpPath` pair, while `hostBare` (29) also merges into an existing `warpBare` local in `internal/fabricengine/coalesce_integration_test.go:95`.
**Fix:** Extend the "zero files contain both" check to every merged name, not just `hostPath`.

## Verdict

GAPS_FOUND
Scope misses two owner dirs; boundary rule and guard-strength claims are contradicted by source.
MILL_REVIEW_END
