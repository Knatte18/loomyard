# PATTERN-hermetic-git-tests

Every test package whose tests spawn git runs under the hermetic git test environment.

- `TestMain` calls `gitkit.HermeticGitEnv()` before `m.Run()`, or the package is allowlisted (`internal/proc`).
- A package is git-spawning when a test file references a gitkit spawn, by the same definition the test-tier-purity entry uses.
