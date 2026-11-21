// Package gitkit is the below-fabric leaf holding test git plumbing.
// It provides the spawn, query and commit helpers, config seeding, and the hermetic git environment.
// It also provides the primitive repo fixture CopyRepo.
// It never imports fabric.
// Many packages call HermeticGitEnv from TestMain, and some sit inside internal/fabriccli's dependency set,
// so a fabric import in this package would stop those TestMain files compiling.
//
// CopyRepo is pinned to internal/lymcwd alone.
// internal/gitkit/callerset_enforcement_test.go (TestCopyRepoCallerSet_LymcwdOnly) enforces the pin.
// Every other package takes a real hub from internal/hubforge instead.
// Every other export is unpinned, and any package may call it.
//
// Every new git spawn stays in gitkit.go, the file the spawn-observability guard allowlists.
// This package cannot import internal/logger.
//
// Leaf Invariant: internal/gitkit production code imports only the standard library and these packages:
// internal/lymcwd, internal/weftname, internal/configengine, internal/lymdirs, internal/lymtemp and internal/dotgit.
// internal/configreg is excluded by construction.
// So is every feature package: the engine and cli pairs for board, fabric, ide and selfreport.
// Feature packages' own tests import gitkit, so a reverse import would close a test-build cycle.
// internal/gitkit/leaf_enforcement_test.go (TestLeafInvariant_AllowlistOnly) enforces this.
// PATTERN-leaf-packages records it.
// SeedConfig takes a configreg-free map of module name to YAML content;
// a test converts configreg.Modules() or a feature's ConfigTemplate() at the test site.
//
// Fixture marker: the environment variable FixtureGitEnv, value "1", tells fixture-building git from test git.
// Test git here means git the code under test runs.
// The spawn helpers in gitkit.go set it on the child's environment.
// hubforge sets it process-wide for the duration of a hub build.
//
// Hermetic Git Test Environment: gitkit implements a two-layer mechanism.
// The mechanism keeps git-spawning tests from depending on the operator's global or system gitconfig.
// Layer A (template quiet-config) applies at build time to every template repo;
// so fixtures built by initRepo/initBareRemote and their Copy* copies are quiet by construction.
// Layer B (HermeticGitEnv, hermetic.go) is a process-wide override a package's TestMain calls before m.Run().
// It covers git spawned by raw `git init`/`git clone` inside tests, and any child process the test binary launches.
// The variables reach every `lym` child a test runs through lymbin.Run.
// See PATTERN-test-isolation for the machine-enforced half of this contract.
package gitkit
