MILL_REVIEW_BEGIN
# Review: Shed-generic watchdog for ly-drive and loom's CLI verbs

```yaml
duration_s: 134.8
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: claude-opus-5
reviewed_file: _mill/discussion.md
date: 2026-09-19
```

## Findings

### [BLOCKING:design] Recipe table has no home; import cycle unaddressed
**Section:** `named-recipe-table-arming`, `per-verb-arming-preserves-lightweight-wiring`
**Issue:** The table maps a recipe name to an arming function built over `loomCLI`/`lifecycleCLI` receivers (`wire`/`wireLightweight`, `verbUsesLightweightWiring`), while `loomcli`/`lifecyclecli` must import `shedcli` for `Verbs(spec)` — if `shedcli` owns the table this is an import cycle, and `cmd/lyx/main.go` is today the sole package importing both (lines 28/30).
**Fix:** State where the table lives and how the cycle is avoided (injection from `cmd/lyx`, a fourth package, or a table registered on `shedcli.Command(entries)`), consistent with the "no `init()` self-registration by the back door" constraint the discussion itself sets.

### [NIT:decision] Product-neutral `LoomRun` replacement name never chosen
**Demoted-from:** BLOCKING
**Section:** `inner-run-engine-goes-product-neutral`
**Issue:** The decision enumerates precisely which identifiers change (`registry.go`'s `"LoomRun"` key, `Env.LoomRun`, `LoomRunDeps`, `NewLoomRun`, `loomRunEntry`, log/stuck strings) and which do not, but never names the replacement — yet the recipe YAML `engine:` value, both coverage guards, and the entry's `requireSeam` error texts must all land on one agreed string in one commit.
**Fix:** Pick the name in the discussion (e.g. `InnerRun`) so the plan and its guards cannot diverge.

### [NIT:scope] ly-drive loom-specific enumeration is incomplete
**Demoted-from:** BLOCKING
**Section:** `ly-drive-drives-any-recipe`
**Issue:** The four sections named for gating (reed `$TMUX_PANE` check, `.lyx/loom/friction/`, `selfreport` gate, seventeen-row arithmetic) miss at least four more loom-only claims in `plugins/ly/skills/ly-drive/SKILL.md`: the cwd precondition "must be the task worktree root" (line 15 — lifecycle refuses from non-prime), the baseline and interrupted-invocation reads that invoke `lyx loom status` (lines 37, 92, 100), the assertion that an absent status file "returns an error envelope naming `lyx loom start`" (line 41 — lifecycle returns `found: false` success), and the `interrupt_policy` field the branch reads off *status* (line 96), which for a non-loom recipe is absent entirely rather than empty — the technical-context note only covers `next_interrupt_policy` on the step envelope.
**Fix:** State the enumeration method for the skill (e.g. every `lyx loom *` invocation plus every claim about status shape must be gated or generalized) rather than a hand-listed four.

### [BLOCKING:design] Absent-file status envelope key set left undefined
**Section:** `envelope-contracts-move-with-the-verbs`
**Issue:** `lifecyclecli/status.go` emits exactly two keys when the file is absent (`found`, `status_path`), but the design has the generic core plus a `StatusExtras(st shedengine.Status)` hook; it never says whether the absent branch skips the core and the hook, or passes a zero `Status` through `StatusExtras` — the latter silently adds `history` (and possibly the four core keys) to an envelope that has neither today, contradicting "no existing key... removed or reworded" being the only surface delta.
**Fix:** Say explicitly that the absent-file disposition short-circuits before both the core and `StatusExtras`, and that lifecycle's absent envelope stays exactly two keys.

### [NIT:decision] Told-Geometry bound-packages membership deferred
**Section:** `## Constraints`
**Issue:** The discussion asks the plan to decide whether `shedcli` joins the Told-Geometry bound-packages list, which leaves a CONSTRAINTS.md edit undecided at discussion close.
**Fix:** Decide it here; the new Shed Verb-Set Invariant already carries the no-resolver clause either way.

### [NIT:design] Busy message and discarded PostRun map underspecified
**Section:** `busy-refusal-comes-from-the-arming-spec`, `optional-hooks-for-module-specific-work`
**Issue:** `loomcli/run.go` reports `ErrShedBusy` with a bare `err.Error()`, so the told busy message must have an "unset means passthrough" form the decision does not state; likewise `PostRun`'s returned map has no destination on the error path.
**Fix:** Name the empty-string passthrough and say the returned map is discarded when `runErr != nil`.

## Verdict

REQUEST_CHANGES
Four blocking gaps: table home/import cycle, unnamed rename target, skill enumeration, absent-file envelope.
_Note: 2 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 2._
MILL_REVIEW_END
