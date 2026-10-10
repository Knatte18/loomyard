// jsonhelp_test.go asserts the --json help schema at multiple levels of the lyx command tree.
// Each row drives the run() seam with --json and validates that the captured output is valid JSON matching the {name, short, commands, flags} schema.
// It also confirms that hidden and meta flags are absent from the flags array.

package main

import (
	"bytes"
	"encoding/json"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// helpJSON mirrors the cmdJSON schema produced by clihelp.renderCmdJSON.
type helpJSON struct {
	Name     string         `json:"name"`
	Short    string         `json:"short"`
	Long     string         `json:"long"`
	Commands []helpJSONCmd  `json:"commands"`
	Flags    []helpJSONFlag `json:"flags"`
}

type helpJSONCmd struct {
	Name  string `json:"name"`
	Short string `json:"short"`
	Usage string `json:"usage"`
}

type helpJSONFlag struct {
	Name      string `json:"name"`
	Shorthand string `json:"shorthand"`
	Usage     string `json:"usage"`
	Default   string `json:"default"`
	Type      string `json:"type"`
}

// decodeHelpJSON parses run() output as helpJSON, fataling on parse errors.
func decodeHelpJSON(t *testing.T, buf *bytes.Buffer) helpJSON {
	t.Helper()
	var h helpJSON
	if err := json.Unmarshal(buf.Bytes(), &h); err != nil {
		t.Fatalf("output is not valid JSON: %v\nraw output:\n%s", err, buf.String())
	}
	return h
}

// flagNames returns the set of flag names present in a helpJSON flags array.
func flagNames(flags []helpJSONFlag) map[string]bool {
	names := make(map[string]bool, len(flags))
	for _, f := range flags {
		names[f.Name] = true
	}
	return names
}

// commandNames returns the set of command names in a helpJSON commands array.
func commandNames(cmds []helpJSONCmd) map[string]bool {
	names := make(map[string]bool, len(cmds))
	for _, c := range cmds {
		names[c.Name] = true
	}
	return names
}

// TestJSONHelp_Schema asserts "--json" help exits 0 and produces valid JSON with the expected schema fields at the root, a verb module, a module with one verb and a leaf verb, with or without --help.
// run() rewrites package-global flag state in newRoot, so neither the test nor its rows run in parallel.
func TestJSONHelp_Schema(t *testing.T) {
	tests := []struct {
		name string
		args []string
		// wantNameContains is a substring of the JSON name; empty asserts only a non-empty name.
		wantNameContains string
		wantCommands     []string
		// wantNoCommands asserts a leaf: the commands array is empty.
		wantNoCommands bool
		wantFlags      []string
		absentFlags    []string
	}{
		{
			name:         "root lists every module",
			args:         []string{"--json"},
			wantCommands: []string{"board", "config", "ide", "reed", "selfreport"},
			absentFlags:  []string{"--json", "--help"},
		},
		{
			name:             "verb module names its subcommands and hides --board-path",
			args:             []string{"board", "--json"},
			wantNameContains: "board",
			wantCommands:     []string{"upsert", "list", "remove", "sync"},
			absentFlags:      []string{"--board-path"},
		},
		{
			name:         "selfreport names its subcommand",
			args:         []string{"selfreport", "--json"},
			wantCommands: []string{"create"},
		},
		{
			name:           "leaf verb lists its flags and no commands",
			args:           []string{"selfreport", "create", "--help", "--json"},
			wantNoCommands: true,
			wantFlags:      []string{"--body", "--label"},
			absentFlags:    []string{"--json", "--help"},
		},
		{
			// Run, selfreport create would refuse its missing title and exit non-zero.
			name:           "leaf verb under --json alone prints its help and never runs",
			args:           []string{"selfreport", "create", "--json"},
			wantNoCommands: true,
			wantFlags:      []string{"--body", "--label"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			code := run(tt.args, &out)
			if code != 0 {
				t.Fatalf("run(%v) = %d; want 0. output:\n%s", tt.args, code, out.String())
			}

			h := decodeHelpJSON(t, &out)

			if h.Short == "" {
				t.Errorf("%v JSON: short is empty", tt.args)
			}
			if tt.wantNameContains == "" && h.Name == "" {
				t.Errorf("%v JSON: name is empty", tt.args)
			}
			if !strings.Contains(h.Name, tt.wantNameContains) {
				t.Errorf("%v JSON name %q does not contain %q", tt.args, h.Name, tt.wantNameContains)
			}

			cmds := commandNames(h.Commands)
			for _, want := range tt.wantCommands {
				if !cmds[want] {
					t.Errorf("%v JSON commands missing %q; commands: %v", tt.args, want, h.Commands)
				}
			}
			if tt.wantNoCommands && len(h.Commands) != 0 {
				t.Errorf("%v JSON commands: want empty, got %v", tt.args, h.Commands)
			}

			flags := flagNames(h.Flags)
			for _, want := range tt.wantFlags {
				if !flags[want] {
					t.Errorf("%v JSON flags missing %q; flags: %v", tt.args, want, h.Flags)
				}
			}
			for _, absent := range tt.absentFlags {
				if flags[absent] {
					t.Errorf("%v JSON flags must not include %q", tt.args, absent)
				}
			}
		})
	}
}

// TestOwnJSONFlags pins the commands below the root that declare a `--json` of their own, local or persistent on a group.
// Such a command shadows the global `--json`, which elsewhere raises help before the command runs.
// The read-only classifier's rule that a trailing `--json` is a help form does not hold for it, so a new entry is a reviewed change against that rule.
// The pinned verbs are the generic shed `status` verb under each recipe mount, which only reads.
func TestOwnJSONFlags(t *testing.T) {
	t.Parallel()

	root := newRoot()
	var got []string
	walkCommands(root, func(cmd *cobra.Command) {
		if cmd != root && cmd.LocalFlags().Lookup("json") != nil {
			got = append(got, strings.TrimPrefix(cmd.CommandPath(), root.Name()+" "))
		}
	})
	sort.Strings(got)

	want := []string{"batten status", "loom status", "shed status"}
	if !slices.Equal(got, want) {
		t.Errorf("commands declaring their own --json = %q; want %q", got, want)
	}
}
