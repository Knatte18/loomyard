// Package lyxtest holds the shared git-fixture support machinery for Loomyard's
// test suites across internal/warpengine, internal/warpcli, internal/weftengine,
// internal/weftcli, and internal/hubgeometry.
// It owns the fixture builders and per-test isolation helpers, following the
// template-built-once + per-test filesystem copy pattern to minimize setup overhead
// and maximize parallelism. See MustRun, CopyHostHub, CopyPaired, and CopyWeft.
//
// Leaf Invariant: internal/lyxtest must remain a leaf package importing only the
// standard library and internal/hubgeometry. It must not import internal/configreg or any
// feature package (boardengine/boardcli, warpengine/warpcli, weftengine/weftcli,
// ideengine/idecli, selfreportengine/selfreportcli, muxpoccli). Feature packages'
// internal tests import lyxtest; a configreg or feature import would close a
// test-build cycle. Tests that need real configuration seed it via SeedConfig, which
// takes a configreg-free map[string]string (module name to YAML content), converting
// configreg.Modules() or a feature's ConfigTemplate() at the test site instead of
// inside lyxtest.
//
// Hermetic Git Test Environment: lyxtest also implements the two-layer mechanism
// that keeps git-spawning tests from depending on the operator's global or system
// gitconfig. Layer A (template quiet-config) sets core.fsmonitor=false,
// maintenance.auto=false, and gc.auto=0 on every template repo at build time, so
// fixtures built by initRepo/initBareRemote and their Copy* copies are quiet by
// construction. Layer B (HermeticGitEnv, hermetic.go) is a process-wide override a
// package's TestMain calls before m.Run(), pointing GIT_CONFIG_GLOBAL at a neutral
// config and setting GIT_CONFIG_NOSYSTEM=1, which also covers git spawned by raw
// `git init`/`git clone` inside tests and by any child process the test binary
// launches. See CONSTRAINTS.md's Hermetic Git Test Environment Invariant for the
// machine-enforced half of this contract.
package lyxtest
