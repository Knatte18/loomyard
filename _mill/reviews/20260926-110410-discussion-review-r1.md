# Review: shed: the LLM driver as a generic stepper and mender

```yaml
verdict: APPROVE
reviewer_model: orchestrator
reviewed_file: _mill/discussion.md
date: 2026-09-26
```

## Findings

### [NIT:decision] `run_dir` is filled on one of the three step entry points only
**Demoted-from:** BLOCKING
**Section:** Decisions — `friction-and-run-dir`; Scope ("`internal/shedcli` fills `RunDir`; `internal/loomcli` fills `FrictionDir`").
**Issue:** `RunDir` is "filled by `internal/shedcli` after `Arm` returns".
But the generic step body is armed from three subtrees, not one: CONSTRAINTS.md's Shed Verb-Set Invariant names "the `Verbs(texts, spec)` constructor the three subtrees build from", and `internal/loomcli/cli.go:334` and `internal/battencli/cli.go:172` each call `shedverbs.Verbs` with their own spec.
`lyx loom step` and `lyx batten step` therefore emit the thirteen-key envelope with `run_dir: ""`, while the skill writes its repair records and stop report under `run_dir`.
The Testing section's "`run_dir`/`friction_dir` echo the `Spec` fields" passes on all three paths and never catches it.
**Suggested fix:** decide one of: fill `RunDir` inside `loomcli.ArmAt` and `battencli.ArmAt` (both already hold the `*lyxcwd.Location` and run-id; `shedrun.ScratchDir` is the sanctioned constructor, and `loomcli` already calls it in `driverreport.go`), so every entry point carries it; or state that the skill drives only `lyx shed step` and that `run_dir: ""` on the module-local verbs is intended, and add a test pinning which paths carry it.

### [NIT:decision] The repair verb set does not cover the crash windows this task absorbs, and the discussion never says so
**Demoted-from:** BLOCKING
**Section:** Decisions — `repair-scope` ("Repairs act through `lyx`'s own verbs … When no `lyx` verb can perform the repair, it escalates"); Problem ("This task absorbs the former `fabric-pair-state-after-crash` and `remedy-texts-followed-verbatim` tasks").
**Issue:** The design's promise is that fabric no longer has to resume partial states because the driver repairs them.
Under `repair-scope`, the driver may only use `lyx` verbs plus read-only git.
The fabric verb surface (`internal/fabriccli/fabric.go`: `add`, `remove [--force] [--remote]`, `prune`, `cleanup`, `reconcile`, `unwire`, …) has no verb that deletes a stranded warp branch: `remove`'s help says it deletes "the pair's weft branch", and #269 records that `rollbackAdd`'s warp-branch deletion is refused by the destructive gate with `git branch -D` as the only remedy.
So the headline crash window of the absorbed work escalates by construction, and the discussion does not acknowledge it.
The Fabric Git Invariant (CONSTRAINTS.md) "binds LYX's own code only" and also says "An agent commits its own code to warp only", so whether an LLM driver may run mutating git on warp, for a branch the trace names as created by the failed step, is an open reading, not settled by citing the invariant.
**Suggested fix:** add a short coverage statement: for each absorbed crash window (partial `add`, partial `remove`, stranded warp branch, stranded remote weft branch), name the `lyx` verb that repairs it, or state that it escalates by design.
Then decide explicitly whether the driver may use raw `git branch -D` on warp for a branch its trace shows the failed step created — permit it narrowly, or keep it an escalation and say that this is accepted.

### [NIT:decision] Trace for kind-less pre-run refusals is left to the plan
**Section:** Technical context ("they carry `trace_file` too when the plan finds it cheap, otherwise the skill reads `trace_dir` from `status`").
**Issue:** An unresolved option handed to the plan writer; the skill's branching depends on which one is chosen.
**Suggested fix:** pick one. `trace_dir` from `status` plus the per-step `LYX_TRACE_ID` already covers the case without touching `shedcli`'s pre-run.

### [NIT:design] "The file whose name carries that id" is not unique
**Section:** Decisions — `trace-dir-and-trace-id`.
**Issue:** Child `lyx` processes inherit the same `LYX_TRACE_ID` and write their own files; when they share the anchor, several files in `trace_dir` carry the id (the name is `trace-<UTC>-<traceid>-<pid>.log`), and the step's own pid is unknown after an interrupt.
**Suggested fix:** say "the files whose names carry that id", read all of them, and order by the UTC timestamp in the name.

### [NIT:consistency] Recipe-name tripwire needs word boundaries
**Section:** Decisions — `recipe-blind-skill` ("`SKILL.md` contains none of the shipped recipe names (`shedrun.RecipeNames()`)").
**Issue:** A substring check for `loom` fails on any mention of `loomyard` or `Loomyard`.
**Suggested fix:** match whole words.

### [NIT:consistency] Merge ordering with #23 loom-done-after-friction
**Section:** Decisions — `step-cap-dropped`.
**Issue:** #23 adds a fifteenth loom row (`Friction-Reflect`) and states its `Testing` must update row-count assertions; #28 deletes `AutonomousDriveStepCap`, `drivercap_test.go`, and the cap sentence derived from loom's row count.
The friction handling itself is compatible: #23 (after its round-1 fix) skips reflection when armed for `step` and leaves notes in `loomengine.LoomFrictionDir`, which is exactly what #28's `friction_dir` names, and #28's Out list already excludes reflection under `step`.
**Suggested fix:** note that whichever lands second reconciles the cap files (if #23 lands first and edits them, #28 still deletes them).

## Verdict

APPROVE
The recipe-blind shape, trace-in-envelope, open-append-close sink, and operator-gated self-report all honour the design, but two decisions are missing: where `run_dir` is filled across the three step subtrees, and whether the repair verb set can repair the crash windows this task absorbs.
_Note: 2 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 0._
