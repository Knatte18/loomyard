// hermetic.go implements Layer B of the two-layer hermetic git mechanism: a
// process-wide override that stops every git spawned by a test process from
// reading the operator's global or system git config.

package lyxtest

import (
	"os"
	"sync"
)

// hermeticGitEnvOnce guards HermeticGitEnv so repeated calls are no-ops; the
// neutral config file is created and the environment mutated exactly once
// per test binary run.
var hermeticGitEnvOnce sync.Once

// HermeticGitEnv makes every git process spawned by this test binary ignore the
// operator's global and system git config, replacing it with a small neutral
// config. Without this, machine-specific settings such as core.fsmonitor=true
// in the operator's ~/.gitconfig cause every fixture-built or freshly `git
// init`/`git clone`d repo to spawn an fsmonitor--daemon (and auto-maintenance)
// background process, which is what produced hundreds of daemon spawns per
// warpengine test run. HermeticGitEnv covers both direct git spawns and
// indirect ones: os.Setenv mutates the test process's own environment, which
// exec.Command children (and any binaries those children launch, such as
// cmd/lyx's e2e tests invoking the lyx binary, which itself spawns git)
// inherit by default.
//
// Call HermeticGitEnv as the first line of a package's TestMain, before
// m.Run():
//
//	func TestMain(m *testing.M) {
//		lyxtest.HermeticGitEnv()
//		os.Exit(m.Run())
//	}
//
// The neutral config file this writes is a documented accepted leak: it is one
// small file per test-binary run, created under os.TempDir() and never removed.
// TestMain conventionally ends in os.Exit(m.Run()), which skips deferred
// cleanup, and lyxtest's template directories (built via os.MkdirTemp in the
// sync.Once template builders) already leak under exactly the same precedent —
// the OS temp cleaner owns both.
//
// The bare function name HermeticGitEnv is the presence token that cmd/lyx's
// hermetic guard scans test files for (a raw-substring match, matching both the
// qualified lyxtest.HermeticGitEnv() call form used by other packages and the
// unqualified HermeticGitEnv() form used by lyxtest's own tests). Do not rename
// this function without updating the guard.
func HermeticGitEnv() {
	hermeticGitEnvOnce.Do(func() {
		// Neutral config content: fsmonitor/maintenance/gc keys mirror Layer A so
		// raw `git init`/`git clone` calls inside tests are quiet too; identity and
		// init.defaultBranch replace what removing the operator's global config
		// would otherwise silently take away (see discussion.md's
		// neutral-global-config-contents decision).
		const neutralConfig = "[user]\n" +
			"\tname = Test\n" +
			"\temail = test@test.com\n" +
			"[init]\n" +
			"\tdefaultBranch = main\n" +
			"[core]\n" +
			"\tfsmonitor = false\n" +
			"[maintenance]\n" +
			"\tauto = false\n" +
			"[gc]\n" +
			"\tauto = 0\n"

		// Fixture-construction precedent (mustGit): errors here are unrecoverable
		// setup failures, so panic immediately rather than threading an error
		// return through every TestMain in the repo.
		f, err := os.CreateTemp("", "lyxtest-gitconfig-*")
		if err != nil {
			panic(err)
		}
		defer f.Close()

		if _, err := f.WriteString(neutralConfig); err != nil {
			panic(err)
		}

		// GIT_CONFIG_GLOBAL redirects git's "global" config layer to this file
		// instead of the operator's ~/.gitconfig; GIT_CONFIG_NOSYSTEM disables the
		// system-wide layer entirely (Git for Windows ships autocrlf and similar
		// machine-specific settings there). Both env vars are inherited by every
		// child process this test binary spawns, directly or transitively.
		if err := os.Setenv("GIT_CONFIG_GLOBAL", f.Name()); err != nil {
			panic(err)
		}
		if err := os.Setenv("GIT_CONFIG_NOSYSTEM", "1"); err != nil {
			panic(err)
		}
	})
}
