# Grading keys, fixed before any arm plan was read

Two inputs, each a landed tapestry run whose records are a weft tag.
The keys are findings of the landed run's own plan review, so the landed plan scores clean by construction.
Grades: R repeated (the plan carries the defect), P partly, A avoided, n/a another route to the same design.
A plan with no card or test for what a key names, because its design leaves the thing out, is A when the omission is safe and n/a when the design differs.

## Input S: `machine-name-verb`

Tag `archive/machine-name-verb/fdb791c78e4b`, base commit `0604f2ea`.
The keys are L1 to L9 of `docs/research/loom/plan-writer-bench-2026-11-12/keys.md`, unchanged, so arms A, B and C can be set beside that bench's solo Opus and solo Fable plans.

## Input L: `run-fsm-tracked-records`

Tag `archive/run-fsm-tracked-records/6cbd899d38d2`, base commit `055ef994`.
The keys are the BLOCKING and MEDIUM findings of the landed run's lens review (`_lym/plan/lens-review.md` in the tag) that its plan fixed,
restricted to defects a plan can carry without the landed design's names; the findings the review disputed are left out.
Each key is stated by the defect, not by the landed card number.

| Key | Severity | Defect the first draft carried |
|---|---|---|
| F1 | BLOCKING | A tender resume sets the Bolt hold aside, though only the cancel sets a hold aside |
| F2 | BLOCKING | The cancel and tender resume leave a shed-paused run paused, and the paused refusal names no way forward |
| F3 | BLOCKING | The batten run verb checks the act verdict before whether the run is drivable, so a blocked run with an act verdict gets a pass |
| F4 | BLOCKING | The cleanup probe for a standing pair counts only local worktrees, so a pair standing only on origin is removed |
| F5 | BLOCKING | A new top-level test of the batten pre-run guard repeats rows of the test that already covers the pause refusal |
| F6 | BLOCKING | A new test of the dobby reading is added, repeating assertions of the four existing tests that read the same cells |
| F7 | BLOCKING | A test fake's teardown seam and a nil cancelled-record remover are left unlisted, so the card's tests do not compile |
| F8 | BLOCKING | The PR-record writer is optional in the wiring check, so a deps set without it passes silently |
| F9 | MEDIUM | A test claims "no cache" without removing the cache, so it is vacuous |
| F10 | MEDIUM | A new test of the attempts move duplicates rows of the merge and finalize tests |
| F11 | MEDIUM | The cancel's origin criteria have no test that reads the pushed record from origin |
| F12 | MEDIUM | The state a verb reads from the local parent branch is not read from origin's fetched branch |
| F13 | MEDIUM | The interrupted-cancel refusal does not name the machine that set the intent |
| F14 | MEDIUM | A terminal landed wait with a batten record gets no refusal that names the done slug |
| F15 | MEDIUM | The push-owed aside is not set by the cancel and the tender verbs |
| F16 | MEDIUM | Batten step, first pass and loom start do not ask the act rule, so a driving verb skips it |
| F17 | MEDIUM | The docs and help text that name the moved owner read are left false |

Counted per plan: R and P of F1 to F17, and R and P of L1 to L9 for input S.
