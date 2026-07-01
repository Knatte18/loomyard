MILL_REVIEW_BEGIN
# Review: Expand the sandbox suite: subfolder init, weft, warp, config reconcile + coverage invariant

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: C:\Code\loomyard\wts\sandbox-suite-expand\_mill\discussion.md
date: 2026-07-01
```

## Findings

### [GAP] "warp already covered by S5/S6" premise is untrue
**Section:** Problem; Scope/In (S8 bullet); Decisions → Scenario numbering
**Issue:** The discussion states current S6 "only tests warp's bad-checkout error path" and S8 goes "beyond the bad-checkout error case already covered by S5", but the actual S6 in `tools/sandbox/SANDBOX-SUITE.md` (lines 187-203) is wrong-directory/error-ergonomics with zero warp/checkout content — and Decisions (line 88) itself says S5 "drives no single module." The referenced warp scenario is likely the deleted old S5, not present anywhere today.
**Fix:** Correct the premise to say warp has *no* existing scenario (S8 is its first) and drop the "already covered by S5" claim, so a plan writer does not add a spurious `Covers: warp` tag to the renamed S5 or under-scope S8's error path.

### [GAP] S6 (subfolder init) has no durability/cleanup note
**Section:** Decisions → Subfolder init scenario (S6)
**Issue:** S6 mutates hub state (scaffolds a nested `_lyx/`, wires junctions via `warpengine.WireJunctions`, and edits `.gitignore` via `gitignore.Ensure` — confirmed in `internal/initcli/initcli.go`), yet unlike S3/S7/S8 it carries no cleanup note; since the hub persists across sessions unless `-reset`, leftover nested `_lyx/`/junctions would alter a later S6 run's observed "created" vs "exists" outcome.
**Fix:** Add an S6 durability note matching S3/S7/S8 (remove the nested `_lyx/`, revert `.gitignore`/junctions at session end), or explicitly state S6 relies on `sandbox-build.cmd -reset` and needs no cleanup.

## Verdict

GAPS_FOUND
Two factual/durability gaps; the coverage-invariant design itself is sound and source-accurate.
MILL_REVIEW_END