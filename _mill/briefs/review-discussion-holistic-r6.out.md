MILL_REVIEW_BEGIN
# Review: reed: AddStrand and attach self-heal a cold worktree

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude Opus 5 (claude-opus-5)
reviewed_file: _mill/discussion.md
date: 2026-09-18
```

## Findings

### [BLOCKING:decision] webster's two standalone reedUp boots undispositioned
**Section:** `redundant-up-sites-kept` / `shuttle-inherits-the-self-heal`
**Issue:** `internal/webstercli/run.go:104` and `internal/webstercli/recoverbatch.go:117-124` both call `c.reedUp()` for exactly the reason the self-heal now removes, and both comments state the invalidated premise verbatim ("pre-fix the spawn died on `no reed session` with an impossible recourse"; "SPAWNS a cold recovery strand through reed.AddStrand, which requires a live session"); `shuttle-inherits-the-self-heal` cites webster's private boot as rationale but never dispositions the sites, and `redundant-up-sites-kept` enumerates only three sites, none of them these.
**Fix:** state keep-or-remove for both webster `reedUp` sites and add their comments to the Scope doc-update inventory, or say explicitly that they are out of scope and why.

### [BLOCKING:consistency] Sandbox disposition is factually wrong and reed-only
**Section:** Constraints → Sandbox Suite Coverage
**Issue:** "That suite is not required to change" is false — `tools/sandbox/SANDBOX-REED-SUITE.md:106-109` (M1) names `lyx reed add` failing with `no reed session; run "lyx reed up"` as the `OK` outcome, which this task deletes (M1's `remove <guid>` half still holds, so the scenario must split, not just be reworded); and the survey stops at the reed suite while `shuttle-inherits-the-self-heal` also falsifies `SANDBOX-WEBSTER-SUITE.md:29-30` ("without it the spawn fails loud with `no reed session`").
**Fix:** state that M1 must be rewritten and split, and extend the sandbox sweep to every suite asserting a pre-`up` refusal, not just the reed one.

### [BLOCKING:design] Boot-attribution test's tier is unstated
**Section:** Testing → "Boot attribution (in `internal/reedengine`, not `reedcli`)"
**Issue:** Asserting `EnsureSession` returns `false` "against a live session with ≥1 pane" needs a real tmux server, but the section's `internal/reedengine` heading declares that package's tests "hermetic (untagged, no tmux contact)" — a plan writer following the heading writes a Test Tier Purity breach.
**Fix:** name the tier explicitly (the package already has `//go:build integration` files, e.g. `contract_integration_test.go`) and split the cold/`true` half from the live-session/`false` half if they land in different tiers.

### [NIT:design] `booted` conflates "took the cold branch" with "spawned"
**Section:** `ensure-session-is-the-seam` / `selfheal-observability`
**Issue:** `ensureSessionLocked` is specified to return `(true, nil)` whenever it delegates to `upLocked`, but `ensureServerAndSessionLocked` can itself return `booted == false` (session came up between probes), so the attribution `Info` line can claim a boot that did not occur.
**Fix:** say whether `booted` means "delegated to upLocked" or "a server/session was actually created", and if the latter, thread `ensureServerAndSessionLocked`'s own flag out through `upLocked`.

## Verdict

REQUEST_CHANGES
Two stale-artifact dispositions missing and one test-tier ambiguity; core seam design is sound.
MILL_REVIEW_END
