I have read the overview, all three batch files, the discussion, CONSTRAINTS, and every referenced source and test file, and verified the rename/cleanup blast radius via grep. My analysis is complete.

Key verifications:
- Batch DAG is acyclic (2←1, 3 independent), all `file:` entries (01/02/03) exist, names declared, no forward deps; cards 1–9 are sequentially numbered with no gaps.
- All board-source references to `SetPhase`/`id_or_slug`/`set-phase`/`set_phase` (store.go, store_test.go, cli.go, cli_test.go, board.go, boardtest/bench_test.go, cmd/lyx/main_test.go, helptree_test.go) are each assigned to an editing card. The remaining matches are in `_mill/`, `CLAUDE.md` (the unrelated wiki-daemon `set_phase`), and `docs/benchmarks/test-suite-timing.md` (`TestCLISetPhase`, which contains no `set-phase` token, so Card 1's docs-grep claim holds).
- `removeOrphanProposals` lives only in render.go; render_test.go does not call it directly (it exercises it via `RenderToDisk`), so Card 9's ghost-test restructure is the real work and is addressed.
- Constraints (Path, CLI/Cobra, lyxtest leaf, docs lifecycle) are respected; manifest path is `filepath.Join(boardPath, …)` per the documented exception, no new `os.Getwd`/`git rev-parse`.
- writeOp discards in-memory mutations on error (no Save), so merge's end-of-sequence `SetStatus` error still yields disk-level atomicity.

MILL_REVIEW_BEGIN
# Review: Board fixes from sandbox run — payload keys, help, rerender — holistic

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-06-28
```

## Findings

### [NIT] Card 2 Context omits store.go
**Location:** Batch 1 / Card 2
**Issue:** Card 2's requirement relies on the `Store.GetTask`/`RemoveTask`/`SetStatus` type-switch resolvers (string/int/float64, `id:0` via `nextID()`) but lists only discussion.md in Context; store.go is neither Context nor Edits.
**Fix:** Add `internal/board/store.go` to Card 2 Context (the int/float64 boundary is already described in discussion.md, so this is documentation hygiene, not a cold-start blocker).

### [NIT] No explicit atomic-rollback test for merge set_status missing target
**Location:** Batch 1 / Card 5
**Issue:** The discussion lists "merge when set_status target absent → whole merge fails atomically and leaves store unchanged" as a TDD candidate, but Card 5's added cases cover only the inner-validation/unknown-key paths, not the missing-target rollback.
**Fix:** Add a `store_test.go`/`cli_test.go` case asserting a merge whose `set_status` target does not exist returns an error and leaves the store unchanged.

### [NIT] Card 2 set-status status extraction underspecified
**Location:** Batch 1 / Card 2
**Issue:** Card 2 converts the renamed set-status test to `{"slug":"foo","status":"active"}` and must apply the status, but the requirement text focuses on the lookup selector and leaves how `status` is read into the `*string` for `SetStatus` implicit (Card 3 owns the required/null logic).
**Fix:** State in Card 2 that the handler reads `status` from the helper's returned map and passes it to `SetStatus` (full presence/null semantics deferred to Card 3).

## Verdict

APPROVE
Plan is complete, constraint-clean, and correctly sequenced; only minor context/test polish remains.
MILL_REVIEW_END
