MILL_REVIEW_BEGIN
# Review: gitexec: decide whether RunGit should return a typed error carrying stderr — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: claude-sonnet-5 (Sonnet 5)
reviewed_file: plan/
date: 2026-08-11
```

## Findings

### [BLOCKING:scope] Card 3's wiki-task body omits the Client Boundary Invariant guard fix
**Location:** Batch `verdict-record`, Card 3.
**Issue:** Card 3's brief correctly says "the two known guard-test collisions that must be fixed in the same commit," but the body's explicit checklist ("the three things the implementation commit must do beyond the migration itself") only names one collision class — "update the guards that key on the old literal token" (`tierpurity_test.go`, `hermeticenv_test.go`, `rawgitmutation_test.go`).
It never names the second, structurally distinct collision: `cmd/lyx/gitrepoboundary_test.go`'s Client Boundary Invariant, whose three assertions (`gitexecTotal != 1`, the run-body requirement, and the `r.run`-keyed pinned-method set) all break once `gitrepo` gains `runChecked` — confirmed against the guard's actual source (`gitexecTotal` counts any `gitexec.` occurrence and `runBoundMethods` keys on `bodyCallsMethodOnReceiver(..., "run")`, neither of which is "the literal token `gitexec.RunGit`"). Discussion.md states plainly these three assertions "must change in the implementation commit" and that each invariant's `CONSTRAINTS.md` entry needs a cross-reference to the other — Card 1's design-doc spec (item 9) captures this fully, but Card 3's wiki-task summary does not, so a reader working from the filed task alone could miss it.
**Fix:** Add a fourth bullet to Card 3's "three things" list (or fold it into item 1) explicitly naming the Client Boundary Invariant's three assertions in `cmd/lyx/gitrepoboundary_test.go` as a required same-commit fix, matching the "two known guard-test collisions" already promised in the brief.

## Verdict

REQUEST_CHANGES
Card 3's filed wiki-task body under-enumerates required guard fixes vs. its own brief and the design doc.
MILL_REVIEW_END
