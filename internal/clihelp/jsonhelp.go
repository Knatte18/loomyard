// jsonhelp.go implements the global --json flag, its help renderer and the installer of the HelpFunc.
// The global --json flag turns every invocation into a help request.
// lym --json, lym <module> --json and lym <module> <cmd> --json each print JSON help and exit 0.
// The JSON describes the command's name and its short and long description.
// It lists the immediate non-hidden subcommands and the local non-meta flags.
// The command itself never runs.
// A command that declares its own local --json flag, such as lym shed status --json, shadows the global one.
// For such a command, --json keeps its own meaning and the command runs.

package clihelp

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// flagJSON describes a single command flag in the --json help output.
// The fields carry lowercase JSON tags, which set the output keys.
type flagJSON struct {
	Name      string `json:"name"`
	Shorthand string `json:"shorthand"`
	Usage     string `json:"usage"`
	Default   string `json:"default"`
	Type      string `json:"type"`
}

// cmdChild describes an immediate subcommand in the --json help output.
// The fields carry lowercase JSON tags, which set the output keys.
type cmdChild struct {
	Name  string `json:"name"`
	Short string `json:"short"`
	Usage string `json:"usage"`
}

// cmdJSON is the top-level schema for the --json help output of a command.
// The fields carry lowercase JSON tags, which set the output keys.
type cmdJSON struct {
	Name     string     `json:"name"`
	Short    string     `json:"short"`
	Long     string     `json:"long"`
	Commands []cmdChild `json:"commands"`
	Flags    []flagJSON `json:"flags"`
}

// metaFlags is the set of flag names left out of the --json flags output.
// They are infrastructure flags, not domain flags that a caller inspects.
var metaFlags = map[string]bool{
	"json": true,
	"help": true,
}

// renderCmdJSON builds the cmdJSON representation of cmd.
// It collects only non-hidden immediate subcommands and local non-hidden flags.
// It does not collect the meta flags.
func renderCmdJSON(cmd *cobra.Command) cmdJSON {
	result := cmdJSON{
		Name:  cmd.CommandPath(),
		Short: cmd.Short,
		Long:  cmd.Long,
	}

	// Cobra's built-in completion command is infrastructure, not domain, so it is left out.
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

	// Local flags only; inherited flags belong to an ancestor command.
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

// JSONFlagUsage is the help text of the global --json flag.
const JSONFlagUsage = "print the command's help as structured JSON and exit 0; the command itself never runs"

// jsonHelpValue is the pflag.Value behind the global --json flag.
// Setting it true also sets the shared --help flag.
// Cobra checks help right after flag parsing, before any pre-run hook, argument validation or Run;
// so no command runs under --json, however it is wired.
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

// InstallJSONHelp declares the global --json flag and a shared --help/-h flag as persistent flags on root.
// It also installs a HelpFunc.
// It renders JSON help when --json is set and delegates to cobra's default renderer otherwise.
// Every descendant inherits both flags and the HelpFunc, so commands added after this call are covered:
// under --json, every command, leaf or group, prints its JSON help and exits 0 without running.
// A descendant that declares its own local json flag shadows the global one, which then stays unset for that command,
// and the command runs with its own --json meaning.
// Call InstallJSONHelp once during root command construction.
func InstallJSONHelp(root *cobra.Command) {
	var jsonFlag, helpFlag bool

	// The shared --help replaces cobra's per-command default, which cobra adds only when no help flag is inherited.
	// The --json value raises this shared flag.
	root.PersistentFlags().BoolVarP(&helpFlag, "help", "h", false, "help for this command")
	root.PersistentFlags().VarPF(&jsonHelpValue{json: &jsonFlag, help: &helpFlag}, "json", "", JSONFlagUsage).NoOptDefVal = "true"

	// Capture cobra's built-in help renderer before overriding it, to delegate to it on the non-JSON path.
	defaultHelp := root.HelpFunc()

	root.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		if jsonFlag {
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
