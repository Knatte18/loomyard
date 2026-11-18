// accountenv.go resolves the Claude account a launch runs under and renders what the launch does with CLAUDE_CONFIG_DIR.
// It also records the account a run launched under in the run directory.

package claudeengine

import (
	"fmt"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/envconfig"
	"github.com/Knatte18/loomyard/internal/shell"
	"github.com/Knatte18/loomyard/internal/state"
)

// launchedAccountFileName is the record, in a run's or an opened session's directory, of the account it launched under.
const launchedAccountFileName = "account.json"

// accountEnv is what a launch line does with CLAUDE_CONFIG_DIR.
// The zero value does nothing.
type accountEnv struct {
	// dir is the config directory the line exports; empty exports nothing.
	dir string
	// unset makes the line remove the variable, for an account on Claude Code's default directory.
	// Setting it there would move Claude Code's `.claude.json`.
	unset bool
}

// statement returns the standalone statement that applies the environment, empty when it does nothing.
func (e accountEnv) statement(sh shell.Shell) string {
	switch {
	case e.dir != "":
		return sh.ExportEnv(claudeConfigDirEnv, e.dir)
	case e.unset:
		return sh.UnsetEnv(claudeConfigDirEnv)
	}
	return ""
}

// wrap returns cmd with the environment applied to it.
// An exported directory wraps the line outermost, and the unset runs as a statement ahead of it.
func (e accountEnv) wrap(sh shell.Shell, cmd string) string {
	switch {
	case e.dir != "":
		return sh.WithEnv(claudeConfigDirEnv, e.dir, cmd)
	case e.unset:
		return sh.Chain(e.statement(sh), cmd)
	}
	return cmd
}

// launchedAccount is the account a launch ran under, as written to the launch's directory.
type launchedAccount struct {
	// ConfigDir is the config directory the launch's session reads.
	ConfigDir string `json:"config_dir"`
	// Account is the name of the listed account whose directory ConfigDir is; empty when none is.
	Account string `json:"account"`
}

// writeLaunchedAccount records launched in dir.
func writeLaunchedAccount(dir string, launched launchedAccount) error {
	path := filepath.Join(dir, launchedAccountFileName)
	return state.WriteJSON(path, lockPathOf(path), launched)
}

// readLaunchedAccount reads the account recorded in dir, reporting false when dir holds no record.
func readLaunchedAccount(dir string) (launchedAccount, bool, error) {
	path := filepath.Join(dir, launchedAccountFileName)
	return state.ReadJSON[launchedAccount](path, lockPathOf(path))
}

// resolveLaunchAccount resolves the account a launch in the told worktree runs under.
// It returns what the launch does with CLAUDE_CONFIG_DIR and the account to record.
// An account whose directory is not Claude Code's default is exported.
// One on the default directory unsets the variable.
// With no account the launch touches nothing, and the record names the directory the session reads:
// the env module's CLAUDE_CONFIG_DIR, else the default.
// The record names the listed account whose directory that is, if any.
// A failure to read the env module refuses the launch.
func (c *Claude) resolveLaunchAccount(claude Config) (accountEnv, launchedAccount, error) {
	home, err := claudeHomeDir()
	if err != nil {
		return accountEnv{}, launchedAccount{}, fmt.Errorf("resolve the home directory for the Claude config directory: %w", err)
	}
	defaultDir := filepath.Join(home, claudeConfigDirName)

	var file accountFile
	var usage map[string]usageRecord
	if accountDir := c.worktree.AccountDir; accountDir != "" {
		if file, err = readAccountFile(accountDir); err != nil {
			return accountEnv{}, launchedAccount{}, fmt.Errorf("read the Claude account file: %w", err)
		}
		if usage, err = readUsage(accountDir, claude.Accounts); err != nil {
			return accountEnv{}, launchedAccount{}, err
		}
	}
	if account, ok := resolveAccount(claude, c.worktree, file, usage, c.now()); ok {
		if canonicalConfigDir(account.ConfigDir) == canonicalConfigDir(defaultDir) {
			return accountEnv{unset: true}, launchedAccount{ConfigDir: defaultDir, Account: account.Name}, nil
		}
		return accountEnv{dir: account.ConfigDir}, launchedAccount{ConfigDir: account.ConfigDir, Account: account.Name}, nil
	}

	env, err := envconfig.LoadConfig(c.dirs)
	if err != nil {
		return accountEnv{}, launchedAccount{}, fmt.Errorf("read the env module for the Claude config directory: %w", err)
	}
	launched := launchedAccount{ConfigDir: defaultDir}
	if dir := env.Vars[claudeConfigDirEnv]; dir != "" {
		launched.ConfigDir = dir
	}
	for _, listed := range claude.Accounts {
		if canonicalConfigDir(listed.ConfigDir) == canonicalConfigDir(launched.ConfigDir) {
			launched.Account = listed.Name
			break
		}
	}
	return accountEnv{}, launched, nil
}
