{"status":"success","commit_sha":"e9518ad2a0c883eb38f8111bd0cdf5740880a9c0","session_id":"7be0d2b3-292e-418a-8af5-4d8c84f77c5f","cards_done":[1,2,3]}

All 3 of 3 cards committed and verified. Summary of work:

- `contracts/stencils/loom/loom-rubric-discussion-review.md` — new stencil transcribing the Discussion-Review rubric from `manifest/designs/loom.md` (three do-not-flag items, three also-flag items), with a leading HTML comment stripped by `internal/stencil.StripLeadingComment` before interpolation.
- `contracts/stencils/stencils.go` — added `LoomRubricDiscussionReview` embed var and its `entries` registration, placed after `LoomTemplatePlan`.
- `contracts/stencils/rubric_test.go` — new test file with `TestLoomRubricDiscussionReview_NamesEveryRequiredItem` (table-driven substring pins for the six rubric items) and `TestLoomRubricDiscussionReview_CarriesNoStencilMarkers` (guards against `{{.` markers).

One self-caught bug during verify: the stencil's leading comment originally referenced the literal `{{.rubric}}` marker syntax in prose, which tripped the no-markers test I wrote in card 3. Fixed by rephrasing to prose ("rubric marker value") with a follow-up commit `fix(stencils): drop literal marker syntax from rubric's leading comment`.

`verify: go test ./contracts/stencils/...` passes. Working tree has no tracked in-scope modifications (only the untracked, out-of-scope brief file remains, which is expected).

{"status":"success","commit_sha":"e9518ad2a0c883eb38f8111bd0cdf5740880a9c0","session_id":"7be0d2b3-292e-418a-8af5-4d8c84f77c5f","cards_done":[1,2,3]}
