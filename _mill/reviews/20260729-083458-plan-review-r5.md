MILL_REVIEW_BEGIN
# Review: board: move storage to weft:main — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewed_file: plan/
date: 2026-07-29
```

## Findings

### [BLOCKING] Card 25 breaks the Test Tier Purity guard
**Location:** batch 5 / Card 25
**Issue:** `boardguard_test.go` mirrors `ghguard_test.go`/`gitrepoboundary_test.go`'s `exec.Command("go","env","GOMOD")` root resolution, but both those siblings are individually listed in `tierpurity_test.go`'s `allowedSpawners` map (verified in source), and Card 25 never adds a matching `cmd/lyx/boardguard_test.go` entry. As an untagged file (mirrors its untagged siblings, per the batch's own plain `go test ./cmd/lyx/...` verify), this trips `TestTierPurity_UntaggedTestsSpawnNothing`.
**Fix:** Add `cmd/lyx/tierpurity_test.go` to Card 25's `Edits:` and require a new `allowedSpawners["cmd/lyx/boardguard_test.go"]` entry with a one-line reason.

### [BLOCKING] Batch 2 leaves internal/fabricengine non-compiling between Card 4 and Card 6
**Location:** batch 2 / Cards 4-6
**Issue:** Card 4 changes `suffixWeftPrimaryBranch`'s signature to `(hostBranch string, err error)` but its Requirements never touch `CloneHub`'s own call site (`if err := suffixWeftPrimaryBranch(weftPath); err != nil` — same file, verified in `clone.go`); that fix is explicitly deferred to Card 6 ("Update the call site... step 6b"). `internal/fabricengine` fails to compile at Card 4's and Card 5's commits.
**Fix:** Fold the `CloneHub` call-site update into Card 4, or merge Cards 4 and 6 into one card.

### [BLOCKING] Batch 3 leaves internal/boardengine non-compiling for most of the batch
**Location:** batch 3 / Cards 11-18
**Issue:** Card 11's config-key rename (Home/Sidebar/ProposalPrefix → Readme/DesignPrefix) immediately breaks every pre-existing `boardengine.Config{}`/`Outputs{}` struct literal in `render_test.go`, `board_test.go`, `config_test.go`, `template_test.go`, and `boardtest/*` (verified via grep — dozens of literals), none of which are fixed until Cards 13/17/18. Card 12's `Render`/`RenderToDisk` signature change and `ExtendedTitle` deletion additionally break `render_test.go`/`layer_test.go` (both `package boardengine_test`, compiled together with production code), unresolved until Card 18. The batch's own `verify: go test ./internal/boardengine/...` fails at every intermediate commit from Card 11 through Card 17 — contradicting Card 12's own stated goal of "keeps internal/boardengine compiling continuously," which it only honors for `board.go`, not the test files its own signature change breaks.
**Fix:** Reorder/merge so the config-key rename and each signature change land together with their full test-fixture fallout in the same card, or explicitly document that intermediate cards in this batch are not independently buildable.

### [NIT] Card 1 Context omits internal/gitrepo/gitrepo.go
**Location:** batch 1 / Card 1
**Issue:** Requirements specify calling `gitrepo.New(weftPath).StageAllAndCommit(message)`, but the defining file (`internal/gitrepo/gitrepo.go`) is not in Card 1's `Context:` (only `fabric.go`/`weftgit.go` are). The return shape is fully inlined in the requirement text, so no actual exploration is needed, but this is a literal Context-completeness gap.
**Fix:** Add `internal/gitrepo/gitrepo.go` to Card 1's Context.

### [NIT] Card 17 miscounts configengine/config_test.go fixtures
**Location:** batch 3 / Card 17
**Issue:** Claims "three occurrences" of the `path: _board\nhome: Home.md\n` fixture in `internal/configengine/config_test.go`; grep confirms only two (lines 35, 77) — three further occurrences of `path: _board\n` alone lack the `home:` line entirely.
**Fix:** Correct the count to two (no code impact — the instruction is "do not touch" this file either way).

## Verdict

REQUEST_CHANGES
Three BLOCKING sequencing/guard gaps (batches 2, 3, 5) leave intermediate commits non-compiling or break an enforced guard.
MILL_REVIEW_END
