MILL_REVIEW_BEGIN
# Review: Speed up git-fixture tests: bench, analyse, hardlink

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-13
```

## Findings

### [GAP] Guard token set misses lyxtest.MustRun / SeedConfig git spawns
**Section:** Decisions › hermetic-guard-and-constraints-entry; Technical context
**Issue:** The hermetic guard reuses tierpurity's token set (`gitexec.RunGit`, `exec.Command`, `lyxtest.Copy`), but lyxtest's own public helpers `MustRun` and `SeedConfig` spawn `git` via `exec.Command` *inside* lyxtest — a package whose only git spawn is `lyxtest.MustRun(tb,dir,"git",...)` or `lyxtest.SeedConfig(...)` triggers no token and would pass the guard with no hermetic `TestMain`, reintroducing exactly the silent-daemon regression the guard exists to prevent.
**Fix:** Add `lyxtest.MustRun` and `lyxtest.SeedConfig` to the hermetic guard's git-spawn token set, or state why MustRun/SeedConfig-only spawners need no hermeticity.

### [NOTE] `exec.Command` conflates git with non-git spawners
**Section:** Decisions › hermetic-guard-and-constraints-entry
**Issue:** The `exec.Command` token flags packages that spawn only non-git processes — `cmd/lyx` spawns `go` in `crosscompile_test.go`/`tierpurity_test.go`; mux packages spawn psmux — for which a git-hermetic `TestMain` is meaningless; the discussion names only `internal/proc` + self-references on the allowlist, so the real allowlist is under-enumerated.
**Fix:** Enumerate the non-git spawners that must be allowlisted (go-toolchain, psmux), distinct from packages that get a real `TestMain`.

### [NOTE] Env coverage for git spawned by child binaries unstated
**Section:** Decisions › two-layer-hermetic-mechanism; Testing
**Issue:** `cmd/lyx` e2e tests spawn git only indirectly (via `go run ./cmd/lyx`), not directly; the discussion frames Layer B as each package calling the helper "before m.Run()" but never states that `os.Setenv` in `TestMain` must (and does) propagate to grandchild git for the no-daemon acceptance count to actually drop for those tests.
**Fix:** Note explicitly that Layer B relies on env inheritance to reach git spawned by launched binaries, and confirm cmd/lyx is covered by TestMain rather than allowlisted.

## Verdict
GAPS_FOUND
One token-set hole in the enforcement guard should be resolved before plan writing.
MILL_REVIEW_END
