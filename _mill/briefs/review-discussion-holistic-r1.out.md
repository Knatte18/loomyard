MILL_REVIEW_BEGIN
# Review: Shed-setup validity checker

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-21
```

## Findings

### [BLOCKING:design] Six kinds miss the motivating perch mis-wirings
**Section:** Problem / Finding kinds / Testing §2
**Issue:** The stated purpose is catching a mis-wired `Bouncer`+`Burler` pair, but neither named mis-wiring is detectable by the six kinds: a Burler handing back with `OnDone: Bouncer` instead of `OnStuck` still makes the gate reachable from its target (no `blind-gate`), and the Bouncer's done edge exits the segment so there is no `done-cycle`; likewise a Bouncer whose `OnDone` points back into the Burler (segment never exits) is clean, since the Burler's `OnDone: ""` satisfies `no-terminal`.
**Fix:** Either state that these classes are out of scope and drop the Testing §2 claim that the loomshed test "will fail when one of the five upcoming tasks mis-wires a `Bouncer`/`Burler` pair", or decide on a seventh kind covering them.

### [BLOCKING:design] `dangling-target` edge-drop vs `no-terminal` undefined
**Section:** Finding kinds — `dangling-target`, `no-terminal`
**Issue:** `dangling-target` says the edge "is then treated as absent for every other check", while `no-terminal` is phrased on the field value (`OnDone == ""`); a reachable row with a dangling `OnDone` is therefore both "has a terminal" and "has no terminal" depending on which sentence the implementer follows, and no test scenario pins it.
**Fix:** State whether a dropped done edge makes its row count as terminal for `no-terminal` (and whether a dropped stuck edge suppresses `blind-gate`), and add the case to the test list.

### [BLOCKING:scope] No test scenario where the return path is a stuck edge
**Section:** Testing §1
**Issue:** The perch shape returns to its gate via the Burler's `OnStuck`, so `blind-gate` non-detection depends entirely on the "reachability is over both edge types" default; every listed clean-graph scenario returns via `OnDone`, so a done-edge-only implementation would pass every listed test and flag every future perch as a blind gate.
**Fix:** Add a scenario where `G`'s gate-reachability from `T` exists only through a stuck edge, asserted clean.

### [NIT:design] `Finding.Message` wording unspecified but literal-asserted
**Section:** Scope / Testing §1
**Issue:** Tests assert "the expected `[]Finding` as a literal ... on the full slice", yet the human-readable message's wording is never specified, forcing the plan writer to invent and pin prose.
**Fix:** Say whether the literal comparison covers `Kind`/producer/target only, with `Message` asserted loosely or not at all.

### [NIT:design] Duplicate-`Name` case has no stated expected output
**Section:** Tolerating malformed input / Testing §1
**Issue:** First-wins resolution is stated, but not what findings the shadowed duplicate row yields (it falls out as `unreachable`, which reads as a misleading diagnosis rather than "duplicate").
**Fix:** State the expected findings for the duplicate case explicitly so the test literal is not invented.

## Verdict

REQUEST_CHANGES
Detection gaps and an internal contradiction around dropped edges must be resolved first.
MILL_REVIEW_END
