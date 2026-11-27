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
// Git is the one spawning primitive for queries.
// query.go builds the rev, branch, ancestry, ls-files and exclude queries and the commit helpers on it.
// RevParse reads HEAD from the repository's files through internal/dotgit when they answer, and asks git otherwise.
// Every new spawn stays in gitkit.go, the file the spawn-observability guard allowlists.
// This package cannot import internal/logger.
//
// Leaf Invariant: internal/gitkit production code imports only the standard library and these packages.
// They are internal/lymcwd, internal/weftname and internal/configengine.
// They also include internal/lymdirs, internal/lymtemp and internal/dotgit.
// internal/configreg is excluded by construction.
// So is every feature package: the engine and cli pairs for board, fabric, ide and selfreport.
// Feature packages' own tests import gitkit, so a reverse import would close a test-build cycle.
// internal/gitkit/leaf_enforcement_test.go (TestLeafInvariant_AllowlistOnly) enforces this.
// PATTERN-leaf-packages records it.
// Tests that need real configuration seed it via SeedConfig, which takes a configreg-free map[string]string.
// The map holds module name to YAML content.
// A test converts configreg.Modules() or a feature's ConfigTemplate() at the test site.
//
// Fixture marker: the environment variable FixtureGitEnv, value "1", tells fixture-building git from test git.
// Test git here means git the code under test runs.
// The spawn helpers in gitkit.go set it on the child's environment.
// hubforge sets it process-wide for the duration of a hub build.
// A census of git processes reads it through git's trace2 env-var events.
//
// Hermetic Git Test Environment: gitkit also implements a two-layer mechanism.
// It keeps git-spawning tests from depending on the operator's global or system gitconfig.
// Layer A (template quiet-config) applies at build time to every template repo:
// it sets core.fsmonitor=false, maintenance.auto=false, and gc.auto=0.
// WriteFixtureGitConfig appends those keys, and a non-bare repository's identity, to the config file from Go.
// A fixture initialiser calls it once after git init, so no git config process runs per key.
// Fixtures built by initRepo/initBareRemote and their Copy* copies are therefore quiet by construction.
// Layer B (HermeticGitEnv, hermetic.go) is a process-wide override a package's TestMain calls before m.Run().
// It points GIT_CONFIG_GLOBAL at a neutral config and sets GIT_CONFIG_NOSYSTEM=1.
// It covers git spawned by raw `git init`/`git clone` inside tests, and any child process the test binary launches.
// The neutral config also sets gc.autoDetach=false, so any gc that still runs is foreground and waited for.
// The neutral config file lives in the process run directory of internal/lymtemp,
// so it goes with that directory when tmuxkit.Main removes it after the run.
// The variables reach every `lym` child a test runs through lymbin.Run, which inherits the process environment.
// Reed's clean environment already passes them through.
// See PATTERN-test-isolation for the machine-enforced half of this contract.
package gitkit
