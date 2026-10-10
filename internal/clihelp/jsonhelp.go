// jsonhelp.go implements the global --json flag, its help renderer and the HelpFunc installer for the clihelp package.
// The global --json flag turns every invocation into a help request: lyx --json, lyx <module> --json and lyx <module> <cmd> --json all print structured JSON describing the command's name, short/long description, immediate non-hidden subcommands and local non-meta flags, exit 0, and never run the command.
// A command that declares its own local --json flag (lyx shed status --json) shadows the global one, so there --json keeps the command's own meaning and the command runs.

package clihelp

import (
	"encoding/json"
	"fmt"
	"strconv"

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
// It collects only non-hidden immediate subcommands (skipping cobra's auto "completion" command) and local non-hidden non-meta flags.
func renderCmdJSON(cmd *cobra.Command) cmdJSON {
	result := cmdJSON{
		Name:  cmd.CommandPath(),
		Short: cmd.Short,
		Long:  cmd.Long,
	}

	// Collect non-hidden immediate subcommands, excluding cobra's built-in "completion" command which is infrastructure, not domain.
	for _, child := range cmd.Commands() {
		if child.Hidden {
			continue
		}
		name := child.Name()
		if name == "completion" {
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

// JSONFlagUsage is the help text of the global --json flag InstallJSONHelp declares.
const JSONFlagUsage = "print the command's help as structured JSON and exit 0; the command itself never runs"

// jsonHelpValue is the pflag.Value behind the global --json flag.
// Setting it true also sets the shared --help flag, so cobra takes its help path for the command being executed: help is checked right after flag parsing, before any pre-run hook, argument validation or Run, so no command can run under --json however it is wired.
type jsonHelpValue struct {
	json *bool
	help *bool
}

// Set parses s as a bool into the --json state and, when it is true, raises the --help flag too.
func (v *jsonHelpValue) Set(s string) error {
	b, err := strconv.ParseBool(s)
	if err != nil {
		return err
	}
	*v.json = b
	if b {
		*v.help = true
	}
	return nil
}

// String returns the --json state in pflag's bool notation.
func (v *jsonHelpValue) String() string {
	return strconv.FormatBool(*v.json)
}

// Type reports the flag type pflag shows in usage text.
func (v *jsonHelpValue) Type() string {
	return "bool"
}

// IsBoolFlag lets --json stand without a value, like any bool flag.
func (v *jsonHelpValue) IsBoolFlag() bool {
	return true
}

// InstallJSONHelp declares the global --json flag and a shared --help/-h flag as persistent flags on root, and installs a HelpFunc that renders JSON help when --json is set and delegates to cobra's default renderer otherwise.
// Every descendant inherits both flags and the HelpFunc, so commands added after this call are covered: under --json, every command, leaf or group, prints its JSON help and exits 0 without running.
// A descendant that declares its own local json flag shadows the global one, which then stays unset for that command, and the command runs with its own --json meaning.
// Call InstallJSONHelp once during root command construction.
func InstallJSONHelp(root *cobra.Command) {
	var jsonFlag, helpFlag bool

	// The shared --help replaces cobra's per-command default, which cobra only adds when no help flag is inherited;
	// it is the one the --json value raises.
	root.PersistentFlags().BoolVarP(&helpFlag, "help", "h", false, "help for this command")
	root.PersistentFlags().VarPF(&jsonHelpValue{json: &jsonFlag, help: &helpFlag}, "json", "", JSONFlagUsage).NoOptDefVal = "true"

	// Capture the default help function before overriding it so we can delegate
	// to it on the non-JSON path. root.HelpFunc() returns cobra's built-in renderer.
	defaultHelp := root.HelpFunc()

	root.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		if jsonFlag {
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
