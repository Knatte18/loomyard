// jsonhelp.go implements the --json help renderer and the HelpFunc installer
// for the clihelp package. When the --json flag is set, any cobra help invocation
// (lyx --json, lyx <module> --json, lyx <module> <cmd> --help --json) emits
// structured JSON describing the command's name, short/long description, immediate
// non-hidden subcommands, and local non-meta flags.

package clihelp

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// flagJSON describes a single command flag in the --json help output.
// Fields use lowercase JSON tags matching the discussion schema.
type flagJSON struct {
	Name      string `json:"name"`
	Shorthand string `json:"shorthand"`
	Usage     string `json:"usage"`
	Default   string `json:"default"`
	Type      string `json:"type"`
}

// cmdChild describes an immediate subcommand in the --json help output.
// Fields use lowercase JSON tags matching the discussion schema.
type cmdChild struct {
	Name  string `json:"name"`
	Short string `json:"short"`
	Usage string `json:"usage"`
}

// cmdJSON is the top-level schema for the --json help output of a command.
// Fields use lowercase JSON tags matching the discussion schema.
type cmdJSON struct {
	Name     string     `json:"name"`
	Short    string     `json:"short"`
	Long     string     `json:"long"`
	Commands []cmdChild `json:"commands"`
	Flags    []flagJSON `json:"flags"`
}

// metaFlags is the set of flag names that are excluded from the --json flags output.
// These are meta/infrastructure flags that are not meaningful to callers inspecting
// a command's domain-level flags.
var metaFlags = map[string]bool{
	"json": true,
	"help": true,
}

// renderCmdJSON builds the cmdJSON representation of cmd.
// It collects only non-hidden immediate subcommands (skipping cobra's auto
// "help" and "completion" commands) and local non-hidden non-meta flags.
func renderCmdJSON(cmd *cobra.Command) cmdJSON {
	result := cmdJSON{
		Name:  cmd.CommandPath(),
		Short: cmd.Short,
		Long:  cmd.Long,
	}

	// Collect non-hidden immediate subcommands, excluding cobra's built-in
	// "help" and "completion" commands which are infrastructure, not domain.
	for _, child := range cmd.Commands() {
		if child.Hidden {
			continue
		}
		name := child.Name()
		if name == "help" || name == "completion" {
			continue
		}
		result.Commands = append(result.Commands, cmdChild{
			Name:  name,
			Short: child.Short,
			Usage: child.UseLine(),
		})
	}

	// Collect local (non-inherited) flags, excluding hidden flags and the
	// --json / --help meta flags that are infrastructure rather than domain flags.
	cmd.LocalFlags().VisitAll(func(f *pflag.Flag) {
		if f.Hidden {
			return
		}
		if metaFlags[f.Name] {
			return
		}
		result.Flags = append(result.Flags, flagJSON{
			Name:      "--" + f.Name,
			Shorthand: f.Shorthand,
			Usage:     f.Usage,
			Default:   f.DefValue,
			Type:      f.Value.Type(),
		})
	})

	return result
}

// InstallJSONHelp installs a custom HelpFunc on root that renders JSON help when
// *jsonFlag is true, and delegates to the previously-captured default HelpFunc
// otherwise. Because cobra's HelpFunc is inherited by all descendants, this single
// call covers lyx --json, lyx <module> --json, and lyx <module> <cmd> --help --json.
// Call InstallJSONHelp once during root command construction, before adding subcommands.
func InstallJSONHelp(root *cobra.Command, jsonFlag *bool) {
	// Capture the default help function before overriding it so we can delegate
	// to it on the non-JSON path. root.HelpFunc() returns cobra's built-in renderer.
	defaultHelp := root.HelpFunc()

	root.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		if *jsonFlag {
			// Render the command as JSON and write it to the command's output writer.
			data, err := json.MarshalIndent(renderCmdJSON(cmd), "", "  ")
			if err != nil {
				// Marshal of our own structs should never fail; fall through to
				// default help if it somehow does.
				defaultHelp(cmd, args)
				return
			}
			fmt.Fprintln(cmd.OutOrStdout(), string(data))
			return
		}
		defaultHelp(cmd, args)
	})
}
