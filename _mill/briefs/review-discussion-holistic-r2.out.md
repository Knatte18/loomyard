MILL_REVIEW_BEGIN
# Review: shed: the LLM driver as a generic stepper and mender

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewed_file: _mill/discussion.md
date: 2026-09-26
```

## Findings

### [BLOCKING:design] Repair of busy/ownership kinds is undefined
**Section:** Decision `repair-scope`
**Issue:** The driver "repairs on error envelopes (all five kinds)" and lists `lyx shed pause` as a repair verb.
But `kind: busy` means a live driver or another `step` holds the run lock (`step.go` KindBusy), and `kind: ownership` means a mismatched slug.
Nothing stops a mender from pausing a live loom driver strand or a sibling fork, and nothing says what repairing a slug mismatch would mean.
**Fix:** Give each of the five kinds its own disposition, for example busy and ownership are handed back, and say which kinds the repair path applies to.

### [BLOCKING:consistency] Fork cwd contradicts recipe-blindness, and no mechanism is given
**Section:** Decisions `orchestrator-fork` / `recipe-blind-skill`
**Issue:** "The fork runs from the directory the run's recipe requires (for batten, the hub's prime worktree)" names a recipe, and the `SKILL.md` tripwire bans that.
`recipe-blind-skill` also removes the cwd paragraph.
`lyx shed step` resolves the process cwd (`shedcli/cli.go` `lyxcwd.CwdFrom` → `Resolve`, with no `--target-dir`), and an `Agent` fork inherits the orchestrator's cwd.
The discussion never says who knows the required directory, or how each of the fork's `lyx` calls runs there.
**Fix:** State that the orchestrator's fork prompt names the directory, and say how the fork invokes `lyx` from it without naming a recipe in `SKILL.md`.

### [NIT:design] Trace retention can sweep a trace the driver relies on
**Section:** Decisions `trace-in-envelope`, `trace-dir-and-trace-id`, `repair-scope`
**Issue:** `logger.Sweep` keeps at most 50 non-live trace files per logs directory, and runs on every sink arm.
`TraceFile` now forces a file for every step, and concurrent forks share the prime worktree's logs directory.
The interrupted-step lookup and the stranded-branch evidence both assume the files survive until the driver reads them.
**Fix:** State the assumption, or have the repair record capture the trace lines it acted on.

### [NIT:design] Whether `TraceDir` opens a trace file is unstated
**Section:** Decision `trace-dir-and-trace-id`
**Issue:** "resolved the same way as the sink" does not say whether `TraceDir()` arms the sink, which would create a trace file on every `status` read.
**Fix:** State whether `TraceDir` resolves only, or arms the sink.

### [NIT:consistency] `RunDir` name collides with `shedrun.RunDir`
**Section:** Decision `friction-and-run-dir`
**Issue:** `Spec.RunDir`/`run_dir` is filled from `shedrun.ScratchDir`, while `shedrun.RunDir` is the durable `_lyx` directory.
That invites the wrong constructor, and could put repair records in tracked content.
**Fix:** Call out the mapping as deliberate in the Spec field's contract, or choose a name that does not collide.

### [NIT:design] Kind-less pre-run refusals are listed as repairable but cannot be repaired
**Section:** Decision `repair-scope`
**Issue:** An unseeded run-id and an unsupported verb are listed under the cases the driver repairs.
The driver "never re-seeds", and no `lyx` verb fixes either refusal.
**Fix:** State that these always escalate, and that the driver reads their trace for the report only.

## Verdict

REQUEST_CHANGES
Per-kind repair dispositions and the fork's cwd mechanism need deciding before plan writing.
MILL_REVIEW_END
