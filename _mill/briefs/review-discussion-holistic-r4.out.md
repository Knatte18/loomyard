MILL_REVIEW_BEGIN
# Review: reed: header pane's boot sometimes leaves shell/log noise in its scrollback

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-28
```

## Findings

### [BLOCKING:design] P1 cannot observe the fix in the untagged tier
**Section:** `verification-per-fix-not-per-symptom` (P1) vs `under-go-test-behaviour-unchanged` / Testing §`internal/reedengine`
**Issue:** P1 requires an untagged fake-tmux assertion that the recorded `split-window` argv carries the launch command and that zero `send-keys` fire, but `lifecycle.go:515` calls `headerLaunchLine(shell.ForGOOS(), exe, testing.Testing())` with no injection seam, and the discussion separately decides that under `go test` the header stays a commandless bare-shell split — so post-fix the argv assertion can never be true, and the zero-`send-keys` half is already true on unmodified `main` (the `launchCmd == ""` branch skips both `send-keys` calls today), making the required "red on unmodified main" evidence unobtainable.
**Fix:** Decide the seam that lets an untagged test drive the non-suppressed path (e.g. an engine-level `underTest`/launch-line field or a launch-composer injected at construction), or move P1 to a tier where the real path runs, and restate P1's pre-fix red condition accordingly.

### [BLOCKING:design] P2's "arrange both WARNs" is infeasible on a hubforge hub
**Section:** `verification-per-fix-not-per-symptom` (P2) / Testing §smoke P2
**Issue:** P2 says to trip the dev-refusal and port-back drift warns together by planting a board copy differing from "the worktree's `contracts/stencils` copy", but `seedStencilsAt` sets `sourceDir = ""` when `<worktree>/contracts/stencils` is absent (`cmd/lyx/stencilseed.go:87-92`), and a `hubforge.NewHub` fixture worktree is the synthetic template (README, `backend/`, `nested/`, `wts/some-task/`) with no `contracts/` at all — so `warnPortBackDrift` cannot fire in that fixture, leaving P2 dev-refusal-only, exactly what the discussion forbids.
**Fix:** State how the drift warn is arranged (materialise a `contracts/stencils` tree inside the fixture worktree before running the binary) or drop the both-warns requirement and re-justify P2's pre-fix robustness on the dev-refusal warn alone.

### [NIT:consistency] The "no git rev-parse" gate test is unfalsifiable
**Section:** Testing §`cmd/lyx`
**Issue:** "Assert the gate is evaluated before any geometry resolution, so an opted-out command spawns no `git rev-parse`" cannot fail through `seedStencils`, which returns unconditionally under `testing.Testing()` (`stencilseed.go:36-38`) — the same class of test-that-cannot-go-red the round-2 finding targeted.
**Fix:** Scope that assertion to the extracted predicate's call ordering only, or drop it and rely on the acknowledged Test Tier Purity guard.

### [NIT:decision] Other pane-hosted lyx processes get no disposition
**Section:** `header-opts-out-of-the-stencil-seed-pass` / Scope
**Issue:** The commit-from-a-pane exposure is argued generically ("a git commit from a display pane"), but strand panes also run lyx subcommands through the same root pre-run and are neither annotated nor listed under **Out**.
**Fix:** Add one line stating that non-header pane commands keep the seed pass deliberately, and why.

## Verdict

REQUEST_CHANGES
Two pins rest on false premises about the test harness; verification plan needs rework.
MILL_REVIEW_END
