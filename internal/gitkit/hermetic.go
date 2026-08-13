// hermetic.go implements Layer B of the two-layer hermetic git mechanism: a process-wide override
// that stops every git spawned by a test process from reading the operator's global or system git
// config.

package gitkit

import (
	"os"
	"sync"
)

// hermeticGitEnvOnce guards HermeticGitEnv so repeated calls are no-ops; the
// neutral config file is created and the environment mutated exactly once
// per test binary run.
var hermeticGitEnvOnce sync.Once

// HermeticGitEnv makes every git process spawned by this test binary ignore the operator's global
// and system git config, replacing it with a neutral config.
// Call it as the first line of TestMain, before m.Run().
// It also runs the CLI-reexec refusal guard before anything else.
//
// The bare function name HermeticGitEnv is the presence token that cmd/lyx's hermetic guard scans
// test files for;
// do not rename without updating the guard.
func HermeticGitEnv() {
	refuseCLIReexec()
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
		f, err := os.CreateTemp("", "gitkit-gitconfig-*")
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
