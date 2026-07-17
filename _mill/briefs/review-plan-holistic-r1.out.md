MILL_REVIEW_BEGIN
# Review: Extend codeintel lookup to non-Go languages via LSP — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-07-17
```

## Findings

### [BLOCKING] Card 13 edits the engine but declares no engine file
**Location:** Batch 3, Card 13 (also Batch 1, Card 2)
**Issue:** Card 13's Requirements say "add a small exported `BuiltinRegistry()` in the engine if `builtins()` is unexported" — `builtins()` is unexported (Card 2, lowercase), so the engine's `registry.go` must be edited, but Card 13's Edits is `none`, `registry.go` is not in its Context, and `BuiltinRegistry()` is absent from batch 1's stated external interface (Registry / LoadRegistry / DetectLanguage / Entry / sentinels).
**Fix:** Define exported `BuiltinRegistry()` in Batch 1 Card 2's `registry.go` and add it to batch 1's external interface; drop the conditional "add it in the engine" clause from Card 13 so batch 3 only calls the public accessor.

### [MINOR] Card 10 needs a production seam Card 9 does not create
**Location:** Batch 2, Cards 9 & 10
**Issue:** Card 10 (Creates: test file only, Edits: none) requires an injectable transport seam "add it in `lspclient.go` under Card 9's file," but Card 9's Requirements never specify a pipe-injectable constructor (e.g. `newLSPClientFromPipes` / `io.ReadWriter` seam) — the production seam is unowned.
**Fix:** Add the injectable-transport constructor explicitly to Card 9's Requirements so Card 10 only writes the test.

### [MINOR] Cross-links a non-existent doc
**Location:** Batch 3 Card 16, Batch 4 Card 17
**Issue:** Both cards reference/cross-link `docs/modules/websterv2_extension.md` "for the origin reasoning," but that file does not exist in the worktree (docs/modules holds only README/loom/hardener), producing a dangling link.
**Fix:** Confirm the doc exists on `main`, or drop the cross-link / point it at `docs/research/codeintel-spike.md`.

### [MINOR] Integration test unreachable by any verify gate
**Location:** Batch 2 Card 12, Batch 4
**Issue:** Card 12's live-gopls test is `//go:build integration`, but no batch `verify:` runs `-tags integration` (batch 2 verify is untagged, batch 4 verify is null), so the only shipped integration test is never run by an automated gate — batch 4 is exactly where gopls gets installed.
**Fix:** Give Batch 4 a `verify: go test -tags integration ./internal/codeintelengine/...` after the gopls install.

### [NIT] helptree pinned sets not updated
**Location:** Batch 3, Card 15
**Issue:** Card 15 claims no test edits are needed because the guards "auto-derive from the live root," but `helptree_test.go` uses pinned `requiredModules` and per-module `wantSubs` sets; CONSTRAINTS' CLI/Cobra Invariant says update pinned sets when adding a module/subcommand, and `codeintel`/`refs` would go unexercised by that guard (superset checks pass regardless).
**Fix:** Add `codeintel` to `requiredModules` and a `{codeintel, [refs]}` entry to `TestHelpTree_VerbModuleSubcommands` in Card 15's Edits.

## Verdict

REQUEST_CHANGES
One blocking context/edits gap (Card 13 engine export) plus four minor plan defects.
MILL_REVIEW_END
