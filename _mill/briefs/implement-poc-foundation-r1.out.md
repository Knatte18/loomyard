All 3 cards declared in the batch file are committed: 3 of 3 cards committed, matching the batch file's `## Cards` section exactly (Card 1: `feat(gitnativepoc): add go-git dep and package skeleton`, Card 2: `test(gitnativepoc): integration-tagged hermetic TestMain`, Card 3: `test(gitnativepoc): differential parity-harness helpers`).

{"status":"success","commit_sha":"90587d33","session_id":"54967404-8962-4b00-8100-e78e8f401a14","cards_done":[1,2,3]}

## Summary

All 3 of 3 cards in batch `01-poc-foundation` committed and verified.

**Card 1** — `/home/knatte/Code/loomyard/wts/git-native-library/internal/gitnativepoc/gitnativepoc.go` (new), `/home/knatte/Code/loomyard/wts/git-native-library/go.mod`, `/home/knatte/Code/loomyard/wts/git-native-library/go.sum`: added `github.com/go-git/go-git/v5` as a direct dependency via `go get` + `go mod tidy`, and created the `gitnativepoc` package skeleton with `Repo` (path + `*git.Repository`) and `OpenRepo(path string) (*Repo, error)`.

**Card 2** — `/home/knatte/Code/loomyard/wts/git-native-library/internal/gitnativepoc/testmain_test.go` (new): `//go:build integration`-tagged `TestMain` calling `lyxtest.HermeticGitEnv()`, mirroring `internal/gitrepo/testmain_test.go`.

**Card 3** — `/home/knatte/Code/loomyard/wts/git-native-library/internal/gitnativepoc/harness_test.go` (new): six fixture builders (`newRepoFixture`, `newEmptyRepoFixture`, `newNonASCIIFixture`, `newRenameFixture`, `newSnapshotRefFixture`, `newBareRemoteFixture`) and four assert helpers (`assertParitySHA`, `assertParityFileList`, `assertParityBool`, `assertParityErrClass`), plus a `runGit` passthrough. I temporarily added a throwaway sanity test (outside any card's `Creates:`/`Edits:` scope, never committed) to exercise all six fixtures and four helpers locally, confirmed they pass, then deleted it before the Card 3 commit.

Verify command passed: `go test -tags integration ./internal/gitnativepoc/` (compiles, hermetic `TestMain` runs, no tests yet) and `go test ./cmd/lyx/ -run 'TestTierPurity_UntaggedTestsSpawnNothing|TestHermeticGitEnv_GitSpawningPackagesHaveTestMain'` (both guard tests pass, confirming the new files are correctly integration-tagged and the package carries the hermetic `TestMain`).

Commits (pushed to `git-native-library` branch):
- `a4eb2f57` feat(gitnativepoc): add go-git dep and package skeleton
- `0e53a628` test(gitnativepoc): integration-tagged hermetic TestMain
- `90587d33` test(gitnativepoc): differential parity-harness helpers

No files outside the batch's declared `Context:`/`Edits:`/`Creates:` scope were touched; no plan-edit commit was needed.
