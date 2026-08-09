MILL_REVIEW_BEGIN
# Review: builder: delete internal/builderengine and internal/buildercli, retire builder-contract.md as a reference — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: claude-sonnet-5
reviewed_file: plan/
date: 2026-08-09
```

## Findings

### [BLOCKING:consistency] Batch 2 `verify:` diverges from the overview's DAG
**Location:** `00-overview.md` batch index entry `number: 2` vs `02-comment-sweep.md`'s own frontmatter.
**Issue:** `02-comment-sweep.md`'s `verify:` line appends `&& go test -tags integration ./internal/websterengine/... ./internal/webstercli/...`, which `00-overview.md`'s `batches:` block (stated as "the authoritative DAG mill-go reads to schedule batches", and "mirrored" from each batch file) entirely omits; the batch's own "Batch Tests" prose section also never mentions an integration run, describing only `go build ./...`, `go vet ./...`, and a scoped untagged `go test`.
**Fix:** Make the two `verify:` strings byte-identical. Given the batch is comment-only, touches no integration-tagged test file, and the "no-new-tests" Shared Decision, the integration clause in the batch file's own frontmatter looks like a stray copy-paste from batch 1's verify line and should be dropped from there.

### [BLOCKING:design] Card 4's scoutcli fixture rename creates an unaddressed path collision
**Location:** Batch 1, Card 4 (`internal/scoutcli/cli_test.go`, `TestFilterWithin`).
**Issue:** Current fixtures: `inScope1`/`inScope2` = `/repo/internal/builderengine/{poll,spawn}.go`, `crossPackage` = `/repo/internal/websterengine/poll.go` (the negative control proving cross-package exclusion), `prefixCollision` = `/repo/internal/buildercli/cli.go`. Requirements rename `inScope1`/`inScope2` to `/repo/internal/websterengine/poll.go` and `/repo/internal/websterengine/state.go`, and change `within:` to `/repo/internal/websterengine`, but never mention `crossPackage`. After the prescribed rename, `inScope1`'s File field becomes byte-identical to the untouched `crossPackage`'s File field, both now genuinely under the new `within` scope — `filterWithin`/`isWithinDir` compare only the File path, so `crossPackage` would now classify as in-scope too, contradicting every subtest's `wantRefs` (which still excludes it, per unmodified `wantRefs: []scoutengine.Reference{inScope1, inScope2}`). The card's closing claim — "the property under test... is preserved exactly; only the synthetic package names change" — rests on a false premise once `crossPackage` collides with the renamed `inScope1`.
**Fix:** Add `crossPackage` to Card 4's Requirements, renaming its File path to something that stays genuinely outside the new `internal/websterengine` scope (e.g. a different real sibling package such as `internal/perchengine`), so the negative control is still out of scope after the rename.

## Verdict

REQUEST_CHANGES
Two concrete defects — a verify-field mismatch and a broken test fixture rename — block otherwise well-grounded, precisely-sourced batches.
MILL_REVIEW_END
