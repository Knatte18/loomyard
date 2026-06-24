MILL_REVIEW_BEGIN
# Review: Extract yamlengine and migrate config via lyx update — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-06-24
```

## Findings

### [BLOCKING] boardtest/sync_test.go calls deleted DefaultConfig()

**Location:** `C:\Code\loomyard\wts\yamlengine\internal\board\boardtest\sync_test.go:166,200`
**Issue:** `TestSkipSeam` calls `board.DefaultConfig()` twice, but `DefaultConfig` was deleted in batch 5 (Card 10). The file carries `//go:build integration`, so standard CI passes silently, but the integration suite fails to compile.
**Fix:** Replace both `board.DefaultConfig()` calls with `board.Config{Path: work, Home: "Home.md", Sidebar: "_Sidebar.md", ProposalPrefix: "proposal-"}` literals, matching the pattern used in the other `sync_test.go` cases that already construct `Config` explicitly.

### [BLOCKING] bench_test.go seedWiki writes incomplete board.yaml

**Location:** `C:\Code\loomyard\wts\yamlengine\internal\board\boardtest\bench_test.go:48`
**Issue:** `seedWiki` writes `path: board\n` as `_lyx/config/board.yaml`. The strict `config.Load` rejects files missing any template key, so the CLI-path benchmarks `BenchmarkUpsert`, `BenchmarkGet`, and `BenchmarkList` will error: missing keys `home`, `sidebar`, `proposal_prefix`. Card 21 fixed `main_test.go`'s fixtures but did not fix `bench_test.go`.
**Fix:** Change the written content to `"path: board\nhome: Home.md\nsidebar: _Sidebar.md\nproposal_prefix: proposal-\n"` (all four template keys present).

### [BLOCKING] testContainsHelper is vacuously true — nested/sequence tests never verify content

**Location:** `C:\Code\loomyard\wts\yamlengine\internal\yamlengine\resolve_test.go:312-314`
**Issue:** `testContainsHelper(s, substr)` returns `len(substr) == 0 || len(s) >= len(substr)` — it checks only relative lengths, not actual substring membership. `TestResolve_NestedMapping` and `TestResolve_Sequence` call this helper and therefore never assert the expanded values appear in output; both tests pass for any non-empty YAML blob. The plan requires verifying that nested leaves at depth >= 2 and sequence leaves are all resolved.
**Fix:** Replace the helper body with `strings.Contains(s, substr)` (add `"strings"` import). The `contains` wrapper and both calling tests then verify actual resolved content.

### [NIT] paths_test.go config-helper unit tests gated behind integration build tag

**Location:** `C:\Code\loomyard\wts\yamlengine\internal\paths\paths_test.go:1` (file-level `//go:build integration`)
**Issue:** `TestConfigHelpers`, `TestLyxDirNameConstant`, and the non-git portions of `TestRefactoredMethods` are pure path-math tests that need no real git repo, but they live in the integration-tagged file and are skipped by the batch 3 verify command (`go test ./internal/paths/...` without `-tags integration`).
**Fix:** Move `TestConfigHelpers` and `TestLyxDirNameConstant` (and any `TestRefactoredMethods` subtests that don't use `lyxtest.CopyHostHub`) into a separate untagged `paths_unit_test.go` so they execute under the standard verify.

### [NIT] configsync double-stat for fileAbsent determination

**Location:** `C:\Code\loomyard\wts\yamlengine\internal\configsync\configsync.go:51-76`
**Issue:** The file is read via `os.ReadFile`, then separately checked via `os.Stat` to determine `fileAbsent`. In the window between the two calls a concurrent `lyx init` or `lyx update` could create the file, causing `fileAbsent` to be false when it was absent at read time.
**Fix:** Use the error from the initial `os.ReadFile` to set `fileAbsent := os.IsNotExist(err)` instead of the separate `os.Stat`.

## Verdict

REQUEST_CHANGES
Three blocking issues: deleted `DefaultConfig` still referenced, incomplete board.yaml seed breaks CLI benchmarks, and the nested-leaf resolver test helper is vacuously true.
MILL_REVIEW_END
