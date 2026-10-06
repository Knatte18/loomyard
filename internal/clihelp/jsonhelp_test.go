// jsonhelp_test.go tests InstallJSONHelp and the renderCmdJSON renderer.
// It builds a synthetic cobra command tree with a child command, a local flag, and a hidden flag,
// then asserts the JSON output matches the expected schema.

package clihelp

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// buildJSONHelpRoot constructs a synthetic root command with:
//   - a persistent --json bool flag (the meta flag InstallJSONHelp is keyed to)
//   - a child subcommand with Use="child"
//   - a local string flag --verbose on the root
//   - a hidden local flag --secret on the root
//
// Returns the root command and a pointer to the jsonFlag bool.
func buildJSONHelpRoot() (*cobra.Command, *bool) {
	root := &cobra.Command{
		Use:   "root",
		Short: "root short",
		Long:  "root long description",
	}

	var jsonFlag bool
	root.PersistentFlags().BoolVar(&jsonFlag, "json", false, "output JSON help")

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

	InstallJSONHelp(root, &jsonFlag)
	return root, &jsonFlag
}

// TestInstallJSONHelp_RendersSchema renders the synthetic root's help once with the json flag set
// and asserts the output is a valid JSON document whose fields carry the command's own text, its
// child and its domain flag, and omit hidden flags, meta flags and cobra's built-in subcommands.
func TestInstallJSONHelp_RendersSchema(t *testing.T) {
	t.Parallel()

	root, jsonFlag := buildJSONHelpRoot()
	*jsonFlag = true

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

	root, jsonFlag := buildJSONHelpRoot()
	// jsonFlag is false by default; the custom HelpFunc must delegate to cobra's default.
	_ = jsonFlag

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
