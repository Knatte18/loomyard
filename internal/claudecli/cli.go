// cli.go builds the `lym claude` cobra group with its account verbs and the status line verb.

package claudecli

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/shuttleengines"
)

// notInHubMessage is the refusal of a verb run outside a worktree of a hub.
const notInHubMessage = "Claude accounts belong to a hub; way forward: run the verb from a worktree of a hub"

// Command returns the cobra command for `lym claude`: the bare group, its account group and the status line verb.
func Command() *cobra.Command {
	claude := &cobra.Command{
		Use:   "claude",
		Short: "set or show the Claude account of this worktree, and record usage from the status line",
		Long: `claude manages the Claude accounts logged in on this machine for the worktree it
runs in. account set picks the account the worktree's next Claude session runs
under, and account show prints the resolved account, the pins and each account's
usage. statusline is what Claude Code's status line runs to record an account's
usage.`,
		RunE: clihelp.GroupRunE,
	}
	account := &cobra.Command{
		Use:   "account",
		Short: "set or show the Claude account this worktree runs under",
		RunE:  clihelp.GroupRunE,
	}
	account.AddCommand(accountSetCmd(), accountShowCmd())
	claude.AddCommand(account, statuslineCmd())
	return claude
}

// accountKeeperHere resolves the current worktree and returns the account keeper over it.
// Outside a worktree of a hub it refuses, naming that accounts belong to a hub and the way forward.
func accountKeeperHere() (shuttleengine.AccountKeeper, error) {
	cwd, err := lymcwd.Getwd()
	if err != nil {
		return nil, fmt.Errorf("read the working directory: %w", err)
	}
	location, err := lymcwd.Resolve(cwd)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", notInHubMessage, err)
	}
	dirs := fabricengine.ConfigDirs(location)
	if dirs.Board == "" {
		return nil, errors.New(notInHubMessage)
	}
	keeper, ok := shuttleengines.AccountKeeper(dirs, hubgeom.EngineWorktree(location))
	if !ok {
		return nil, errors.New("no engine keeps accounts; way forward: there is nothing to set or show")
	}
	return keeper, nil
}

// withKeeper runs run over the current worktree's account keeper and sets the exit code it returns.
// A refusal to build the keeper is printed through output.Err.
func withKeeper(cmd *cobra.Command, run func(out io.Writer, keeper shuttleengine.AccountKeeper) int) error {
	if clihelp.ShouldAbort(cmd.Context()) {
		return nil
	}
	out := cmd.OutOrStdout()
	keeper, err := accountKeeperHere()
	if err != nil {
		clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
		return nil
	}
	clihelp.SetExit(cmd.Context(), run(out, keeper))
	return nil
}

// accountSetCmd returns the `claude account set` verb.
func accountSetCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "set <account>",
		Short:       "set the Claude account this worktree's next session runs under",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `set writes the worktree's account entry, which takes effect at the worktree's
next Claude launch; a session already running keeps its account. It refuses an
account the list lacks or has retired, naming the listed ones, and it refuses
under a hub pin, naming the pin. Under a beams pin it writes the entry, which
takes effect while the pin does not apply.

Example:
  lym claude account set work`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withKeeper(cmd, func(out io.Writer, keeper shuttleengine.AccountKeeper) int {
				if err := keeper.SetAccount(args[0]); err != nil {
					return output.Err(out, err.Error())
				}
				return output.Ok(out, map[string]any{"set": map[string]any{
					"account":      args[0],
					"takes_effect": "at the worktree's next Claude launch; running sessions keep their account",
				}})
			})
		},
	}
}

// accountShowCmd returns the `claude account show` verb.
func accountShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "show",
		Short:       "show this worktree's Claude account, the pins and each account's usage",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `show prints the account the worktree resolves to, the hub and beams pins, and for
each listed account its weight, whether it has headroom, the time a usage limit
blocks it until, and each stored usage window with its percentage, threshold and
reset time. It writes nothing.

Example:
  lym claude account show`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return withKeeper(cmd, func(out io.Writer, keeper shuttleengine.AccountKeeper) int {
				text, err := keeper.ShowAccounts()
				if err != nil {
					return output.Err(out, err.Error())
				}
				return output.Ok(out, map[string]any{"lines": strings.Split(text, "\n")})
			})
		},
	}
}

// statuslineCmd returns the `claude statusline` verb Claude Code's status line runs.
func statuslineCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "statusline <account>",
		Short:       "record an account's usage from the status line payload on standard input",
		Hidden:      true,
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceInternal},
		Long: `statusline reads Claude Code's status line payload on standard input and records the
usage windows it reports for the named listed account. It prints one short line:
the account name and each window's percentage. It refuses a name no listed
account has and writes nothing, and it never writes or clears a usage limit's
block.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withKeeper(cmd, func(out io.Writer, keeper shuttleengine.AccountKeeper) int {
				input, err := io.ReadAll(cmd.InOrStdin())
				if err != nil {
					return output.Err(out, fmt.Sprintf("read the status line payload: %v", err))
				}
				line, err := keeper.RecordStatusLine(args[0], input)
				if err != nil {
					return output.Err(out, err.Error())
				}
				fmt.Fprintln(out, line)
				return 0
			})
		},
	}
}
