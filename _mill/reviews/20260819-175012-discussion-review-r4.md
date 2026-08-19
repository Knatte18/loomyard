MILL_REVIEW_BEGIN
# Review: loom: session bootstrap

```yaml
duration_s: 164.0
verdict: REQUEST_CHANGES
reviewer_model: opus
reviewer_self_id: claude-opus-5 (Anthropic)
reviewed_file: _mill/discussion.md
date: 2026-08-19
```

## Findings

### [BLOCKING:design] Bootstrap lock does not close the double-spawn window
**Section:** `reentrancy-ensure-and-attach`
**Issue:** The bootstrap lock is held across steps 1–3 and released before attach, but the run lock it probes is acquired by the *child*, at the top of `shedengine.Run` (`internal/shedengine/run.go:56`) — after `lyx loom drive` has parsed flags, resolved cwd, loaded config and built the full `RunDeps` stack; caller A can therefore release `bootstrap.lock` (spawn returned) while its child has not yet taken `LoomRunLock`, and caller B then acquires the bootstrap lock, probes the run lock free, and spawns a second driver — exactly the `ErrShedBusy`-in-`driver.log` failure the decision claims is "closed, not accepted".
**Fix:** State the handshake that actually closes it — e.g. hold `bootstrap.lock` until the spawner observes `LoomRunLock` held (bounded poll) or the child writes a ready/pid marker — and say what happens if the child dies before signalling.

### [NIT:consistency] Two contradictory rollback contracts for `origin.json`
**Demoted-from:** BLOCKING
**Section:** `origin-record-is-committed-and-is-a-new-class` vs. "The fabric change, concretely"
**Issue:** The decision says rollback is automatic on the created-branch path ("`rollbackAdd` deletes the weft branch the Add created, taking the record commit with it") and deliberately absent on the adopted path; the technical-context paragraph instead says "`rollbackAdd` ... must also unwind the record — through the gated removal path, never a raw `os.Remove`". Verified: `rollbackAdd` (`add.go:238-247`) removes the whole weft worktree via `removeWeftWorktree`, so a separate record removal is a no-op on one path and a history-destroying act on the other. A plan writer cannot tell whether new code lands at a chokepoint-guarded site.
**Fix:** Pick one — state explicitly that `rollbackAdd` gains no new step because the existing weft-worktree/branch removal already covers it, and delete the "must also unwind the record" sentence.

### [NIT:decision] `*Mutations` recorder has no disposition on loom's side
**Section:** `origin-record-ownership-seam` / `missing-record-refusal`
**Issue:** `WriteOrigin` is specified to take a `*Mutations` recorder, but `lyx loom run --parent` is a loom verb whose envelope carries no `mutations`/`partial` keys, and the discussion never says what loom passes or does with the record.
**Fix:** State that `loomcli` passes a throwaway `&Mutations{}` and that loom's envelope is unaffected (the Mutation Record Invariant binds fabric verb outcomes).

### [NIT:consistency] `fabricengine.RefScanner` named as if it were a value
**Section:** `webster-deps-wired-for-real`
**Issue:** `RefScanner` is a type; the constructor is `fabricengine.NewRefScanner(l *lyxcwd.Location)` (`internal/webstercli/wiring.go:130`), so "pinned to `fabricengine.RefScanner`" is not a callable spelling and hides that the matcher needs the resolved Location.
**Fix:** Say `fabricengine.NewRefScanner(loc)`, built eagerly in loom's pre-run as `webstercli` does.

### [NIT:consistency] One module contributing two root children is unstated in the seam rule
**Section:** `run-alias-as-registered-command` / Scope
**Issue:** The CLI/Cobra Invariant reads "Every lyx CLI module is a cobra subtree assembled under one root" and the seam is `Command()`+`RunCLI`; `loomcli` exporting a *second* root-level command is a new seam shape, yet Scope slates only the `RunCLI`/`RunCLIIn` count edits to `CONSTRAINTS.md`.
**Fix:** Name the alias's exported constructor and add a one-line clause to the CLI/Cobra Invariant covering a module that registers an alias command beside its subtree, in the same commit.

## Verdict

REQUEST_CHANGES
Re-entrancy race remains open and the origin-record rollback contract contradicts itself.
_Note: 1 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 1._
MILL_REVIEW_END
