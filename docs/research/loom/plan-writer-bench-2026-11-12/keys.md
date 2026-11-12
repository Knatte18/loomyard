# Grading keys, fixed before any solo plan was read

Reference: `machine-name-verb`, weft tag `archive/machine-name-verb/fdb791c78e4b`, base commit `0604f2ea`.
The keys are the findings of the landed run's own Plan review: round 1 (verdict `BLOCKING`) and round 2 (verdict `APPROVED`).
Each key names a defect the twill plan shipped to its first review and the review then made the writer fix.
Grades follow the earlier plan benches: R repeated (the plan carries the defect), P partly, A avoided, n/a another route.
A plan that takes another route to the same design has no place to repeat the defect and is graded n/a.

| Key | Round, severity | Defect the landed first draft carried |
|---|---|---|
| L1 | 1, BLOCKING | Card 4 tests the unexported helpers `nextFile` and `hubOfDir` with their own test functions, and half the rows repeat rows of the verb-level test |
| L2 | 1, BLOCKING | Card 1 plans a `boardengine` subtest for a competing origin write on the replace set that `CompetingWriteLosesToOrigins` already covers |
| L3 | 1, MEDIUM | Card 1 leaves three doc sentences (`OwnerRequest.Takeover`, the `SetOwners` doc, `internal/boardengine/doc.go`) false once the replace set writes over an owner that reads as another machine |
| L4 | 1, LOW | The `unpushed` read error has no stated outcome |
| L5 | 1, LOW | The failed-hub way forward branches on whether the write error names its own way forward, which the sweep cannot test |
| L6 | 1, NIT | A test-fake seam is counted in prose, so the count goes stale |
| L7 | 2, LOW | The new hostname-name export states no bound on its callers, and its doc comment is written by comparison with another symbol |
| L8 | 2, LOW | A `TestName_Set` row names no input and repeats the final-names assertion of a sibling row |
| L9 | 2, LOW | A tier 1 subtest builds real hubs for a re-run that the tier 0 rows and `boardengine` already pin |

Granularity: a plan that has no card or test for the thing a key names, because its design leaves the thing out, is graded A when the omission is safe and n/a when the design differs.
