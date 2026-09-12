MILL_REVIEW_BEGIN
# Review: self-report Tier 1: Go-detected structural anomalies

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic) — Opus-class model, per system identification "Opus 5"
reviewed_file: _mill/discussion.md
date: 2026-09-12
```

## Findings

### [BLOCKING:design] `selfreport` knob has no consultation site
**Section:** `### on-by-default-with-a-knob` / `### detection-site` / Testing→Config **Issue:** The knob is decided (`loom.yaml` `selfreport`, default `true`) and a test is specified (`false` → zero client calls), but no section says *who* reads `c.cfg.Selfreport` — and the extracted function's told-input list (entry observation, status/lock paths, marker paths, run error, ledger seam, filing seam) omits it while `drive`'s closure is pinned to call it "unconditionally on one line". **Fix:** State whether the bool is a told input of the extracted function (keeping the call unconditional) or a guard in the closure, and say explicitly whether `false` skips detection too or only filing.

### [BLOCKING:consistency] Step 3b described as unconditional; it is not
**Section:** `### detection-site` rationale (l.73), l.75, Q&A (l.461) **Issue:** Source says step 3b is "deliberately conditional" and fires only when `st.State != StateRunning` (`internal/shedengine/run.go:134`), so on a crash-resume — where the state *is* `running` — it never fires; the crash evidence is destroyed by the next ordinary step-5/6 persist, not by 3b. The Testing section (l.388) correctly calls 3b conditional, contradicting l.73's "unconditionally". **Fix:** Restate the mechanism as "the first persist after the producel call overwrites `running`" and drop the "3b destroys it" claim; the error-path decision itself still holds.

### [BLOCKING:design] Ledger-path failure enumeration misses archiving
**Section:** `### ledger-discovery`, skips 1–3 **Issue:** The enumeration assumes a stale `history[].output` ledger path can only point at *nothing*; `archiveRunDir` (`internal/shedadapters/bouncer.go:246`, clear-and-re-seed) renames the whole run dir aside, recreates it empty and restarts rounds at 1, and `archiveStaleOutputs` (`bouncer.go:620`) stamps a round's own three judge outputs aside on every fresh spawn — so an old `round-N-bouncer-ledger.md` path can resolve to a *different generation's* live file, which discovery would read and attribute to the old history entry. **Fix:** Record this fourth case with a disposition (accepted imprecision, or a generation discriminator in the trigger-5 title), the same way trigger 1's imprecision is recorded.

### [NIT:consistency] Two filing seams named for the same tests
**Section:** Testing→"The filing path" vs `### detection-site` **Issue:** The branch tests stub a told "filing seam" on the extracted function, while the filing-path and config tests swap `selfreportengine.NewGitHubClient`; the discussion never says which layer each assertion binds to. **Fix:** One sentence pinning the told seam as the `loomcli` unit boundary and `NewGitHubClient` as the engine-level boundary.

### [NIT:design] Cancelled/paused run still enters the filing pass
**Section:** `### detection-site`, `### failure-posture` **Issue:** `ErrShedBusy` is the only stated skip, but a Ctrl-C returns `RunPaused` with a nil error, and `selfreportengine.CreateIssue` builds its 30s timeout from `context.Background()`, so an operator stop can be followed by up to 30s of network work per anomaly. **Fix:** State the disposition — file anyway, or skip when `cmd.Context()` is already done.

## Verdict

REQUEST_CHANGES
Knob site undecided, one false mechanism claim, and an incomplete ledger-path failure enumeration.
MILL_REVIEW_END
