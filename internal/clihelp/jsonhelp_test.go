// jsonhelp_test.go tests InstallJSONHelp and the renderCmdJSON renderer.
// It builds a synthetic cobra command tree with a child command, a local flag, and a hidden flag, then asserts the JSON output matches the expected schema, and that the global --json runs no command except one whose own local --json shadows it.

package clihelp

import (
	"bytes"
	"encoding/json"
	"io"
	"strconv"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// buildJSONHelpRoot constructs a synthetic root command with:
//   - the persistent --json and --help flags InstallJSONHelp declares
//   - a child subcommand with Use="child"
//   - a local string flag --verbose on the root
//   - a hidden local flag --secret on the root
func buildJSONHelpRoot() *cobra.Command {
	root := &cobra.Command{
		Use:   "root",
		Short: "root short",
		Long:  "root long description",
	}

	// A domain flag that should appear in the JSON output.
	root.Flags().String("verbose", "off", "verbosity level")

	// A hidden flag that must be excluded from the JSON output.
	root.Flags().String("secret", "", "hidden internal flag")
	_ = root.Flags().MarkHidden("secret")

	child := &cobra.Command{
		Use:   "child",
		Short: "child short",
		RunE:  WrapRun(func(_ io.Writer, _ []string) int { return 0 }),
	}
	root.AddCommand(child)

	InstallJSONHelp(root)
	return root
}

// TestInstallJSONHelp_RendersSchema renders the synthetic root's help once with the json flag set and asserts the output is a valid JSON document whose fields carry the command's own text, its child and its domain flag, and omit hidden flags, meta flags and cobra's built-in subcommands.
func TestInstallJSONHelp_RendersSchema(t *testing.T) {
	t.Parallel()

	root := buildJSONHelpRoot()
	if err := root.PersistentFlags().Set("json", "true"); err != nil {
		t.Fatalf("set --json: %v", err)
	}

	var buf bytes.Buffer
	root.SetOut(&buf)
	root.Help() //nolint:errcheck // cobra Help() always returns nil

	var parsed map[string]any
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("InstallJSONHelp output is not valid JSON: %v\noutput: %s", err, buf.String())
	}
	var result cmdJSON
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	t.Run("top-level fields", func(t *testing.T) {
		t.Parallel()
		if result.Name == "" {
			t.Error("cmdJSON.Name is empty; want non-empty command path")
		}
		if result.Short != "root short" {
			t.Errorf("cmdJSON.Short = %q; want %q", result.Short, "root short")
		}
		if result.Long != "root long description" {
			t.Errorf("cmdJSON.Long = %q; want %q", result.Long, "root long description")
		}
	})

	t.Run("lists the child subcommand", func(t *testing.T) {
		t.Parallel()
		found := false
		for _, cmd := range result.Commands {
			if cmd.Name == "child" {
				found = true
				if cmd.Short != "child short" {
					t.Errorf("child.Short = %q; want %q", cmd.Short, "child short")
				}
			}
		}
		if !found {
			t.Errorf("Commands does not contain \"child\"; got %v", result.Commands)
		}
	})

	t.Run("includes the local flag", func(t *testing.T) {
		t.Parallel()
		found := false
		for _, f := range result.Flags {
			if f.Name == "--verbose" {
				found = true
			}
		}
		if !found {
			t.Errorf("Flags does not contain \"--verbose\"; got %v", result.Flags)
		}
	})

	t.Run("omits the hidden and meta flags", func(t *testing.T) {
		t.Parallel()
		for _, f := range result.Flags {
			switch f.Name {
			case "--secret":
				t.Errorf("Flags contains hidden flag %q; want it omitted", f.Name)
			case "--json", "--help":
				t.Errorf("Flags contains meta flag %q; want it omitted", f.Name)
			}
		}
	})

	t.Run("omits cobra built-in subcommands", func(t *testing.T) {
		t.Parallel()
		for _, cmd := range result.Commands {
			if cmd.Name == "help" || cmd.Name == "completion" {
				t.Errorf("Commands contains cobra built-in %q; want it omitted", cmd.Name)
			}
		}
	})
}

func TestInstallJSONHelp_FallsThroughToDefaultHelpWhenFlagFalse(t *testing.T) {
	t.Parallel()

	// --json is unset by default; the custom HelpFunc must delegate to cobra's default.
	root := buildJSONHelpRoot()

	var buf bytes.Buffer
	root.SetOut(&buf)
	root.Help() //nolint:errcheck

	output := buf.String()
	// Cobra's default help contains the Usage: header.
	if strings.Contains(output, `"name"`) {
		t.Errorf("InstallJSONHelp with jsonFlag=false produced JSON output; want plain text help")
	}
	if output == "" {
		t.Error("InstallJSONHelp with jsonFlag=false produced empty output; want plain text help")
	}
}

// TestInstallJSONHelp_JSONNeverRunsACommand drives a fresh tree through Execute per row: under the global --json, a leaf whose run, pre-run hook and argument check would each fail prints its JSON help and exits 0, as does a group; a leaf declaring its own local --json runs with that meaning instead.
func TestInstallJSONHelp_JSONNeverRunsACommand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		args     []string
		wantName string
		// wantRan names the leaf whose body must have run; empty asserts that no body ran and the output is JSON help.
		wantRan string
	}{
		{name: "leaf prints help instead of running", args: []string{"group", "leaf", "--json"}, wantName: "root group leaf"},
		{name: "--json before the path still holds", args: []string{"--json", "group", "leaf"}, wantName: "root group leaf"},
		{name: "group prints its help", args: []string{"group", "--json"}, wantName: "root group"},
		{name: "local --json shadows the global flag and runs", args: []string{"group", "own-json", "--json"}, wantRan: "own-json:true"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var ran string
			root := &cobra.Command{Use: "root", Short: "root short"}
			group := &cobra.Command{Use: "group", Short: "group short", RunE: GroupRunE}
			leaf := &cobra.Command{
				Use:   "leaf <arg>",
				Short: "leaf short",
				Args:  cobra.ExactArgs(1),
				PersistentPreRunE: func(*cobra.Command, []string) error {
					t.Error("leaf's PersistentPreRunE ran under --json")
					return nil
				},
				RunE: func(*cobra.Command, []string) error {
					t.Error("leaf's RunE ran under --json")
					return nil
				},
			}
			var ownJSON bool
			own := &cobra.Command{
				Use:   "own-json",
				Short: "own-json short",
				RunE: func(*cobra.Command, []string) error {
					ran = "own-json:" + strconv.FormatBool(ownJSON)
					return nil
				},
			}
			own.Flags().BoolVar(&ownJSON, "json", false, "the command's own json output switch")
			InstallJSONHelp(root)
			group.AddCommand(leaf, own)
			root.AddCommand(group)

			var buf bytes.Buffer
			if code := Execute(root, &buf, tt.args); code != 0 {
				t.Fatalf("Execute(%v) = %d; want 0. output:\n%s", tt.args, code, buf.String())
			}
			if tt.wantRan != "" {
				if ran != tt.wantRan {
					t.Errorf("ran = %q; want %q. output:\n%s", ran, tt.wantRan, buf.String())
				}
				return
			}
			var got cmdJSON
			if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
				t.Fatalf("output is not JSON help: %v\noutput:\n%s", err, buf.String())
			}
			if got.Name != tt.wantName {
				t.Errorf("JSON help name = %q; want %q", got.Name, tt.wantName)
			}
		})
	}
}
