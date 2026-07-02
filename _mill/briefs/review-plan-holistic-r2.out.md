All plan claims verify against the source. Summary of what I confirmed:

- `newRoot()` registers exactly 8 modules (init, board, config, ide, muxpoc, weft, warp, selfreport) — matches the coverage design; covered {board,config,init,weft,warp} + excluded {muxpoc,ide,selfreport} = all 8.
- `registration_test.go:71` uses three `filepath.Dir` calls (code) while its comment says "two" — Card 4(b) correctly flags the comment as stale and the code authoritative; three walk-ups from `cmd/lyx/*_test.go` reaches repo root.
- config `--set`/`--print` mutual-exclusivity + module-required, `config reconcile` dry-run default, weft fixed `"weft sync"` message, warp `reconcile` having no `--apply` flag, warp `checkout <branch>`, and `init --undo` JSON step fields (lyx_junction/weft_content/git_exclude/gitignore) all match the code.
- `TestRenderScheme_ContainsHeaderAndBody` only asserts the `"SANDBOX-SUITE"` heading substring; the `"ref":"S6"` strings in `report_test.go` are decoupled JSON round-trip fixtures — the renumber does not break `go test ./tools/sandbox/...`.
- CONSTRAINTS.md has CLI/Cobra Invariant immediately before Documentation Lifecycle, so Card 4's placement instruction is accurate.
- DAG acyclic, both `file:` entries present; all cards have Context/Edits/Creates/Deletes/Moves/Requirements/Commit; all Moves are `none` (no rename mechanic needed); global numbering 1-4 sequential.

MILL_REVIEW_BEGIN
# Review: Expand the sandbox suite: subfolder init, weft, warp, config reconcile + coverage invariant — holistic

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-07-02
```

## Verdict

APPROVE
Plan is accurate, complete, correctly sequenced, and every source claim verifies.
MILL_REVIEW_END