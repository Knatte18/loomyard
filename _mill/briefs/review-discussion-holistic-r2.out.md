MILL_REVIEW_BEGIN
# Review: PATTERN wiring: conditional constraint-injection into every agent

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-28
```

## Findings

### [GAP] status/drift junction health still _lyx-only
**Section:** Decisions → reconcile-health-check
**Issue:** The same blind check exists in two more places the discussion never names: `status.go:148-150` (`HostLyxLinkHere()`/`WeftLyxDir()` → `pair.JunctionHealthy`/`InSync`) and `drift.go:74-94` (loom's preflight), so a broken `_pattern` junction still reports healthy/in-sync; `checkJunctionHealth`'s reason strings are also hardcoded `"host _lyx junction missing"`/`"host _lyx is not a junction"`.
**Fix:** Extend the decision to `status.go` and `drift.go` (or state explicitly why they stay `_lyx`-only), and decide the shape of the now-plural `PairStatus.JunctionHealthy`/`JunctionReason` and the health reason wording.

### [GAP] fabric remove leaves a nested _pattern junction
**Section:** Scope (In) / junction-ownership
**Issue:** `remove.go:91` calls `removeHostJunction` (`weftwiring.go:124-129`), which is `HostLyxLink(slug)`-only, and `remove.go:38-39` states this explicit call exists precisely because `fslink.RemoveLinksIn` misses nested junctions at `RelPath != "."` — so `lyx fabric remove` will delete a host worktree with a live `_pattern` junction still inside it.
**Fix:** Add `removeHostJunction`'s generalisation to `HostJunctions(slug)` to the in-scope list, or state why remove is exempt.

### [GAP] Widened pathspec can silently kill every weft commit
**Section:** Decisions → weft-persistence
**Issue:** With `pathspec: _lyx _pattern` on a worktree whose weft `_pattern/` does not exist (any worktree wired before this change, or one where the operator widens the config before re-running init), `git add -- _lyx _pattern` dies with "did not match any files" and `CommitWeft` (`weftgit.go:276-284`) deliberately swallows that into `(nil, committed=false)` — so `_lyx` stops being committed too, silently, with no error.
**Fix:** State the ordering/tolerance contract (materialise weft `_pattern` before the widened pathspec can be used, or make the pathspec entry tolerant) and add a test for widened-pathspec-with-missing-`_pattern`.

### [GAP] Pre-existing host _pattern/ directory has no migration path
**Section:** Decisions → junction-ownership
**Issue:** PATTERN content is described as the host repo's hand-authored invariants, so an operator creating `_pattern/` in the host repo first is the natural mistake — and `seedLyxJunction:113-117` then refuses with "it predates weft — migrate via the hub-creator", hard-failing `lyx init`, `checkout` and `reconcile` for that worktree with no stated remedy.
**Fix:** State the expected operator remedy (and the `Short`/`Long` or error wording that names it) for a real `_pattern` directory already present in a host repo.

### [GAP] UndoResult/InitResult output shape left unstated
**Section:** Decisions → unwire-generalisation / undo-leaves-pattern-content
**Issue:** `UnwireResult.JunctionsRemoved []string` is pinned, but the CLI-observable `UndoResult` only "must name both junctions"; the concrete change sites are unnamed — `initcli.go:120` emits the `lyx_junction` key and `initcli_test.go:74` pins it — and it is undecided whether `InitResult` reports the new `_pattern` directory at all.
**Fix:** Pin the `UndoResult` field/JSON key (rename vs add), name `internal/initcli` and its key test as change sites, and say whether `InitResult` gains a `_pattern` status field.

### [NOTE] hubgeometry's own single-junction tests not listed
**Section:** Testing → internal/fabricengine (last bullet)
**Issue:** The "existing tests that will fail" list names fabric/init tests, but the certain breakages are `hubgeometry/weft_test.go:212-290` (`wantJunctionCount: 1`, indexes `[0]`) and `hubgeometry_test.go:590-602` (`want 1`).
**Fix:** Name those two files in the update list.

### [NOTE] Directive pointer is relative, active-check is Layout-anchored
**Section:** Decisions → directive-shape-and-wording
**Issue:** The checklist hardcodes the relative pointer `_pattern/PATTERN.md` (pinned by a test), while `Directive` stats `WorktreeRoot+RelPath`; the agent pane's cwd is `layout.Cwd`, so the two anchors can disagree in a nested hub even though the Go side was fixed for exactly that case.
**Fix:** State that the pointer is deliberately cwd-relative (matching the existing `_lyx/...` prompt idiom), or interpolate the resolved path.

## Verdict

GAPS_FOUND
Junction generalisation stops short of status/drift/remove; weft-pathspec and result-shape contracts unpinned.
MILL_REVIEW_END
