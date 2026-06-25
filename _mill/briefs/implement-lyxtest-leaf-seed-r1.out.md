Perfect! All cards have been implemented and the verify command passes. Let me provide the final status report:

{"status":"success","commit_sha":"8d446bfd65a6b1516c7759bf125f37c8891c7358","session_id":"bcad531b-54d7-4124-a78e-03fb4ca505f0"}

## Summary

I have successfully implemented all 6 cards of the `lyxtest-leaf-seed` batch:

**Card 1:** Removed the `configreg` import from `internal/lyxtest/lyxtest.go` and replaced the config seeding loop in `buildWeftPrime` with a neutral fixture containing only a placeholder file.

**Card 2:** Added the `SeedConfig` function to `internal/lyxtest/lyxtest.go`, which takes a configreg-free `map[string]string` and seeds configuration files into a git repository, maintaining the leaf invariant.

**Card 3:** Updated `internal/configcli/configcli_integration_test.go` to seed real config into the weft-prime fixture via `SeedConfig` before running `worktree.Add()`.

**Card 4:** Added `SeedConfig` calls to two failing integration tests:
- `TestRunCLI_EnvMapToOption` in `internal/weft/weft_integration_test.go`
- `TestRunCLI_StatusWithMinimalFixture` in `internal/weft/cli_test.go`

**Card 5:** Added comprehensive tests to `internal/lyxtest/lyxtest_test.go`:
- `TestSeedConfig`: Verifies that config files are written and committed
- `TestCopyPaired_NeutralFixture`: Verifies the neutral fixture contains placeholder but no real config

**Card 6:** Recorded and enforced the leaf invariant:
- Added invariant documentation to `internal/lyxtest/doc.go`
- Added "lyxtest Leaf Invariant" section to `CONSTRAINTS.md`
- Created `internal/lyxtest/leaf_enforcement_test.go` using `go/parser` to detect banned imports

**Verification:** All verify steps pass:
- `go build ./...` - Production code compiles
- `go vet -tags integration ./...` - No import cycles
- `go test -tags integration ./...` - All 26 test packages pass
