MILL_REVIEW_BEGIN
# Review: board: move storage to weft:main — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetmax
reviewed_file: plan/
date: 2026-07-29
```

## Findings

### [BLOCKING] Card 11's config-key rename breaks template_test.go in the same package
**Location:** batch 3 / Card 11
**Issue:** `internal/boardengine/template_test.go` (untouched by any card, not in the plan's file manifest) has `TestConfigTemplate_HasRequiredKeys` and `TestConfigTemplate_ResolvesToDefaults` asserting `ConfigTemplate()` contains keys `home`/`sidebar`/`proposal_prefix` and resolves to `Home.md`/`_Sidebar.md`/`proposal-`. Card 11 renames/deletes exactly these keys, so both tests fail on plain `go test ./internal/boardengine/...` — batch 3's own `verify:` command, no `-tags integration` needed.
**Fix:** Add `internal/boardengine/template_test.go` to Card 11's (or Card 17's) `Edits:`, updating the asserted keys/values to `readme`/`design_prefix`.

### [BLOCKING] Config-key rename leaves stale board.yaml fixtures outside boardengine/boardcli
**Location:** batch 3 / Card 11 (root cause; fallout spans batches 5-6 and beyond)
**Issue:** Several test fixtures seed raw `home:`/`sidebar:`/`proposal_prefix:` board.yaml content that no card touches: `cmd/lyx/main_integration_test.go`'s `TestRunDispatchesToBoard` expects `run([]string{"board","rerender"})` to exit 0 but will get exit 1 from `boardengine.LoadConfig`'s "missing keys: readme, design_prefix" error — this breaks batch 5's own `go test -tags integration ./cmd/lyx/...` gate. `internal/ideengine/menu_test.go`'s `TestMenuExcludesMain`/`TestMenuRequiresLyxDir`/`TestMenuNumericSelection` call `Menu()`, which calls `boardengine.LoadConfig` (menu.go:40) and will fail the same way — no `verify:` in this plan covers `internal/ideengine`, so this regression ships uncaught. `internal/boardengine/boardtest/bench_cli_test.go`'s `seedWikiRepo` (reusing `bench_test.go`'s `seedWiki`, which Card 13 edits for two Go-struct literals but not its raw YAML string) drives `boardcli.RunCLI`→`LoadConfig` and breaks the file's own documented `-bench` invocation; also uncaught by any `go test` gate since benchmarks don't run without `-bench`.
**Fix:** Sweep the whole repo (not just `internal/boardengine`/`internal/boardcli`) for `home:`/`sidebar:`/`proposal_prefix:` raw-string board.yaml fixtures and add each hit's file to the appropriate card's `Edits:`.

### [BLOCKING] Card 3's Requirements cite two style-reference files missing from Context
**Location:** batch 1 / Card 3
**Issue:** Card 3's `Requirements:` says "Mirror `CommitWeft`'s existing test shape (see `weftgit_pathspec_integration_test.go`/`weftgit_unborn_warp_test.go` for style)" — both files exist and hold the actual `TestCommitWeft_*` functions this card wants mirrored, but neither is listed in Card 3's `Context:` (only `index_integration_test.go`, `testmain_test.go`, `weftgit.go` are).
**Fix:** Add `internal/fabricengine/weftgit_pathspec_integration_test.go` and `internal/fabricengine/weftgit_unborn_warp_test.go` to Card 3's `Context:`.

### [NIT] store.go's lock-file doc comment goes stale after Card 13's rename
**Location:** batch 3 / Card 13-14
**Issue:** `internal/boardengine/store.go`'s `swapLockSuffix` doc comment references "the coarse tasks.json.lock held across a whole write" by name; Card 13 renames that constant's value to `board.lock`, but `store.go` is not in Card 13's or Card 14's `Edits:`.
**Fix:** Add a bullet to Card 13 (or 14) updating `store.go`'s comment to say `board.lock` instead of `tasks.json.lock`.

### [NIT] Card 25's non-test-file count for internal/boardengine omits template.go
**Location:** batch 5 / Card 25
**Issue:** Card 25 enumerates "8 non-test `.go` files" (`board.go`, `config.go`, `layer.go`, `render.go`, `spawn.go`, `store.go`, `sync.go`, `task.go`) but the package actually has 9, missing `template.go` (the `//go:embed template.yaml` accessor). Harmless to the guard itself (floor is `scanned < 5`), but the rationale text is wrong.
**Fix:** Add `template.go` to the enumerated file list.

### [NIT] Card 32's fold-in text conflates "host repo" with the wiki-hosting repo
**Location:** batch 6 / Card 32
**Issue:** Card 32 summarizes the rejected GitHub-wiki approach as "GitHub wiki rendering requires the host repo to be public" — but `board-weft-storage.md` (lines 7-8) says it's the wiki-*hosting* repo (board's own repo, or the weft companion under the derived-URL default) that must be public, not the host/warp repo. This repo's vocabulary (CONSTRAINTS.md) treats "host repo" as a specific, load-bearing term (=warp), so this would land a factually loose statement in `board.go`'s durable package doc.
**Fix:** Reword to "requires whichever repo hosts the wiki to be public" or similar, not "the host repo."

## Verdict

REQUEST_CHANGES
Two config-rename fallout gaps break existing tests (one in batch 3's own verify gate); Card 3 has a Context gap.
MILL_REVIEW_END
