// jsonhelp_test.go asserts the --json help schema at multiple levels of the lyx command tree.
// Each test drives the run() seam with --json and validates that the captured output is valid JSON
// matching the {name, short, commands, flags} schema.
// It also confirms that hidden and meta flags are absent from the flags array.

package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
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

// TestJSONHelp_RootSchema asserts that "lyx --json" produces valid JSON with the expected schema
// fields and lists every module under commands.
func TestJSONHelp_RootSchema(t *testing.T) {
	var out bytes.Buffer
	code := run([]string{"--json"}, &out)
	if code != 0 {
		t.Fatalf("run([--json]) = %d; want 0. output:\n%s", code, out.String())
	}

	h := decodeHelpJSON(t, &out)

	if h.Name == "" {
		t.Error("root JSON: name is empty")
	}
	if h.Short == "" {
		t.Error("root JSON: short is empty")
	}

	cmds := commandNames(h.Commands)
	requiredModules := []string{
		"board", "config", "ide", "reed", "selfreport",
	}
	for _, mod := range requiredModules {
		if !cmds[mod] {
			t.Errorf("root JSON commands missing module %q; commands: %v", mod, h.Commands)
		}
	}

	flags := flagNames(h.Flags)
	for _, meta := range []string{"--json", "--help"} {
		if flags[meta] {
			t.Errorf("root JSON flags must not include meta flag %q", meta)
		}
	}
}

// TestJSONHelp_VerbModuleSchema asserts "lyx board --json" produces valid JSON naming subcommands.
func TestJSONHelp_VerbModuleSchema(t *testing.T) {
	var out bytes.Buffer
	code := run([]string{"board", "--json"}, &out)
	if code != 0 {
		t.Fatalf("run([board --json]) = %d; want 0. output:\n%s", code, out.String())
	}

	h := decodeHelpJSON(t, &out)

	if !strings.Contains(h.Name, "board") {
		t.Errorf("board JSON name %q does not contain 'board'", h.Name)
	}
	if h.Short == "" {
		t.Error("board JSON: short is empty")
	}

	cmds := commandNames(h.Commands)
	for _, sub := range []string{"upsert", "list", "remove", "sync"} {
		if !cmds[sub] {
			t.Errorf("board JSON commands missing %q; commands: %v", sub, h.Commands)
		}
	}

	flags := flagNames(h.Flags)
	if flags["--board-path"] {
		t.Error("board JSON flags must not expose hidden --board-path")
	}
}

// TestJSONHelp_SelfreportSchema asserts "lyx selfreport --json" produces valid JSON with
// subcommands.
func TestJSONHelp_SelfreportSchema(t *testing.T) {
	var out bytes.Buffer
	code := run([]string{"selfreport", "--json"}, &out)
	if code != 0 {
		t.Fatalf("run([selfreport --json]) = %d; want 0. output:\n%s", code, out.String())
	}

	h := decodeHelpJSON(t, &out)

	if h.Short == "" {
		t.Error("selfreport JSON: short is empty")
	}

	cmds := commandNames(h.Commands)
	if !cmds["create"] {
		t.Errorf("selfreport JSON commands missing 'create'; commands: %v", h.Commands)
	}
}

// TestJSONHelp_SelfreportCreateLeaf asserts leaf "lyx selfreport create --help --json" produces
// valid JSON.
func TestJSONHelp_SelfreportCreateLeaf(t *testing.T) {
	var out bytes.Buffer
	code := run([]string{"selfreport", "create", "--help", "--json"}, &out)
	if code != 0 {
		t.Fatalf("run([selfreport create --help --json]) = %d; want 0. output:\n%s", code, out.String())
	}

	h := decodeHelpJSON(t, &out)

	if h.Short == "" {
		t.Error("selfreport create JSON: short is empty")
	}

	if len(h.Commands) != 0 {
		t.Errorf("selfreport create JSON commands: want empty, got %v", h.Commands)
	}

	flags := flagNames(h.Flags)
	for _, want := range []string{"--body", "--label"} {
		if !flags[want] {
			t.Errorf("selfreport create JSON flags missing %q; flags: %v", want, h.Flags)
		}
	}

	for _, meta := range []string{"--json", "--help"} {
		if flags[meta] {
			t.Errorf("selfreport create JSON flags must not include meta flag %q", meta)
		}
	}
}
