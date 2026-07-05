This is all well-covered. I have now reviewed the implementation comprehensively across all 7 batches. I'm ready to produce the final review.

MILL_REVIEW_BEGIN
# Review: Build internal/shuttle: one LLM agent via a swappable engine — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-07-05
```

## Findings

No findings. This is a round-2 holistic review of a very thorough, plan-compliant implementation across all 7 batches.

Verification performed:
- Batch 1 (mux-extensions): `AddSpec.SessionID` round-trips (verified `strand_test.go:59` test), dead `claude:` config key fully removed from `template.yaml`/`config.go`/`config_test.go`, `configsync_test.go` covers the stale-key reconcile case, `io.go` implements `resolveLivePaneID`/`SendText`/`SendKey`/`CapturePane` exactly per spec with no psmux round trip in tests.
- Batch 2 (shuttle-foundation): config/spec/rundir/posix all mirror muxengine conventions faithfully; `configreg.go` registers shuttle alphabetically between mux/warp.
- Batch 3 (claude-engine): `engine.go` seam types match spec exactly; `command.go` produces the exact launch/resume command shapes (verified no `--continue`, exact quoting); `settings.go` implements the Stop hook + PreToolUse deny/steer matrix correctly with the no-single-quote `init()` guard; `events.go` confirms the previously-flagged non-blocking item (Raw byte round-trip) is now fixed — `Raw: []byte(line)` uses the untrimmed original line, not the trimmed copy; `startup.go` matches the trust/ready heuristics.
- Batch 4 (run-loop): `MuxOps` seam, `Runner`/`Start`/`Wait`/`Interrupt`/`Send` all match the plan's sequencing, cleanup, and classification rules; `wait_test.go` covers all four outcomes, KeepPane, trust-dismiss, multi-Stop offset tracking, and partial-line resilience exactly as specified.
- Batch 5 (cli-and-registration): `shuttlecli` wires `claudeengine.New()` into `NewRunner` at the seam boundary as required; CLI envelope posture followed exactly (all classified outcomes exit 0 via `output.Ok`); root registration, sandbox suite wiring, and `sandbox_coverage_test.go` all correctly reflect the new module (dynamic module-discovery tests in `longlist_test.go`/`registration_test.go` need no literal "shuttle" update, by design).
- Batch 6 (smoke-tests): all three smoke files present, `//go:build smoke`-tagged, reuse `deferHubRelease`/`claudeBinaryPath` conventions, and test exactly the scenarios the plan specifies (full run, guardrail deny-and-steer, guardrail asking, interrupt+send).
- Batch 7 (docs-lifecycle): `docs/modules/mux.md` and `shuttle.md` deleted; grep across the repo confirms zero dangling references outside `_mill/`; `overview.md`, `roadmap.md`, and `CONSTRAINTS.md` all updated per plan, including the repointed `attach` exception reference.
- Provider-seam invariant verified both mechanically (`seam_enforcement_test.go`) and semantically (no Claude-specific markers found in `shuttleengine/run.go` or other provider-invariant files).

## Verdict

APPROVE
Implementation matches the plan precisely across all batches; no blocking issues found in this round.
MILL_REVIEW_END
