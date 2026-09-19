MILL_REVIEW_BEGIN
# Review: Seeded driver choice: ly-drive strand as the child's driver

```yaml
duration_s: 172.0
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: claude-opus-4-class (self-assessment; brief dictates "opusmedium")
reviewed_file: _mill/discussion.md
date: 2026-09-19
```

## Findings

### [BLOCKING:design] Report dir "already exists" rests on an unverified premise
**Section:** `driver-report-is-the-run-s-output-file`, 4th bullet
**Issue:** The claim that `start.go`'s step-4 `MkdirAll` already creates `.lyx/shed/<run-id>/` holds only if `seeded-shed-core` also relocates the *bootstrap lock* there; on disk `loomengine.LoomBootstrapLock` (and `LoomDriverLog`, `LoomRunLock`) are `.lyx/loom/…`, and the quoted start.go comment is about that directory, not about `.lyx/shed/<run-id>/`.
**Fix:** State the disposition explicitly — either pin "seeded-shed-core moves the bootstrap lock under `.lyx/shed/<run-id>/`" as part of the inherited contract to re-verify after rebase, or decide the driver launch ensures the report's parent directory itself.

### [BLOCKING:design] No disposition for a driver that never starts
**Section:** `handshake-is-driver-specific` / `dead-driver-is-an-operator-case`
**Issue:** `Runner.Start` returning proves only that the pane was created and the launch string sent; it does not prove `claude` booted (shuttle's own `Startup`/`requireReadyAgentPane` probe is never run on this path). A missing or failing provider binary yields a successful `--no-attach` bootstrap and a run that never moves, while the `go` path's handshake catches exactly that case — an asymmetry the discussion never records.
**Fix:** Say which it is: fold "driver never started" into the accepted residual alongside "driver died mid-run", or decide a startup probe is part of the `llm` readiness signal.

### [NIT:consistency] Orphan sweep is not an independent reclaimer
**Section:** `llm-driver-launches-through-shuttle`, "What that costs"
**Issue:** `sweepOrphansOpportunistic` removes only run dirs whose strand GUID is absent from reed state; a finished driver's strand stays registered (no `Wait`, no `RemoveStrand`), so the sweep cannot reclaim it until corpse removal has already run — the two are one chain, not two mechanisms.
**Fix:** Restate as a single chain (corpse removal, then the next `Start`'s sweep, subject to its `2*StartupTimeoutS` age guard).

### [NIT:scope] Driver-launch seam shape left undecided
**Section:** Constraints (Test Tier Purity) and Testing (`internal/loomcli` bootstrap decisions)
**Issue:** The discussion requires "the driver-launch seam must be injectable so the branch is testable at Tier 1" and asks for a Tier 1 test of the branch selecting a strand launch, but `loomCLI.runner` is a concrete `*shuttleengine.Runner` and no decision names the seam's shape (interface field, à la `runnerMasterStarter`, vs. function injection).
**Fix:** Name the seam shape in one line, or say explicitly that its shape is a plan-level choice.

## Verdict

REQUEST_CHANGES
Two premises need explicit disposition: the report directory's existence and a never-started driver.
MILL_REVIEW_END
