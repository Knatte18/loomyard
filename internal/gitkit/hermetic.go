// hermetic.go implements Layer B of the two-layer hermetic git mechanism: a process-wide override.
// Every git spawned by a test process ignores the operator's global and system git config.
// It also refuses the https, ssh and git transports and stops git prompting for credentials.

package gitkit

import (
	"os"
	"sync"

	"github.com/Knatte18/loomyard/internal/lymtemp"
)

// hermeticGitEnvOnce guards HermeticGitEnv so repeated calls are no-ops.
// The neutral config file is created and the environment mutated exactly once per test binary run.
var hermeticGitEnvOnce sync.Once

// HermeticGitEnv makes every git process spawned by this test binary ignore the operator's global and system git config.
// It replaces that config with a neutral one.
// It also refuses the https and ssh transports and every credential prompt git or an askpass helper makes,
// so a git that reaches for an https or ssh origin fails at once instead of hanging on a dialog.
// Call it as the first line of TestMain, before m.Run().
// The neutral config file lives in the process run directory of internal/lymtemp.
// tmuxkit.Main removes that directory after the run.
//
// The bare function name HermeticGitEnv is the presence token that cmd/lym's hermetic guard scans test files for;
// do not rename it without updating the guard.
func HermeticGitEnv() {
	hermeticGitEnvOnce.Do(func() {
		// Neutral config content: fsmonitor/maintenance/gc keys mirror Layer A.
		// Raw `git init` and `git clone` calls inside tests are quiet too.
		// Identity and init.defaultBranch replace what removing the operator's global config would otherwise silently take away.
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
			"\tauto = 0\n" +
			"\tautoDetach = false\n"

		// Errors here are unrecoverable setup failures: panic rather than thread an error return through every TestMain.
		f, err := lymtemp.CreateTemp("gitkit-gitconfig-*")
		if err != nil {
			panic(err)
		}
		defer f.Close()

		if _, err := f.WriteString(neutralConfig); err != nil {
			panic(err)
		}

		// GIT_CONFIG_GLOBAL redirects git's "global" config layer to this file instead of the operator's ~/.gitconfig.
		// GIT_CONFIG_NOSYSTEM disables the system-wide layer entirely;
		// Git for Windows ships autocrlf and similar machine-specific settings there.
		// Both env vars are inherited by every child process this test binary spawns, directly or transitively.
		if err := os.Setenv("GIT_CONFIG_GLOBAL", f.Name()); err != nil {
			panic(err)
		}
		if err := os.Setenv("GIT_CONFIG_NOSYSTEM", "1"); err != nil {
			panic(err)
		}

		// A test runs unattended, so git must neither use a hosted origin's https or ssh nor ask anyone for credentials.
		// GIT_ALLOW_PROTOCOL refuses https and ssh, the transports of a hosted origin, before git connects,
		// and it overrides every protocol.<name>.allow setting.
		// It keeps the file transport.
		// It also keeps ext, for tests that script an origin's answer.
		// It keeps http, for tests that point origin at a refused loopback port.
		// GIT_TERMINAL_PROMPT=0 stops git prompting on the terminal.
		// An empty GIT_ASKPASS stops git running any askpass helper, the one SSH_ASKPASS names included;
		// an empty SSH_ASKPASS stops an ssh that an ext transport starts from opening a desktop dialog.
		promptlessTransportEnv := [][2]string{
			{"GIT_ALLOW_PROTOCOL", "file:ext:http"},
			{"GIT_TERMINAL_PROMPT", "0"},
			{"GIT_ASKPASS", ""},
			{"SSH_ASKPASS", ""},
		}
		for _, variable := range promptlessTransportEnv {
			if err := os.Setenv(variable[0], variable[1]); err != nil {
				panic(err)
			}
		}
	})
}
