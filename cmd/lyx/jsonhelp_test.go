// jsonhelp_test.go asserts the --json help schema at multiple levels of the lyx
// command tree. Each test drives the run() seam with --json and validates that the
// captured output is valid JSON matching the {name, short, commands, flags} schema.
// It also confirms that hidden and meta flags are absent from the flags array.

package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// helpJSON mirrors the cmdJSON schema produced by clihelp.renderCmdJSON.
// We decode into this struct so individual fields can be asserted without
// being coupled to the exact serialisation order or whitespace.
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

// decodeHelpJSON parses the run() output as a helpJSON struct.
// It fatals the test on any parse error so callers can proceed to field assertions.
func decodeHelpJSON(t *testing.T, buf *bytes.Buffer) helpJSON {
	t.Helper()
	var h helpJSON
	if err := json.Unmarshal(buf.Bytes(), &h); err != nil {
		t.Fatalf("output is not valid JSON: %v\nraw output:\n%s", err, buf.String())
	}
	return h
}

// flagNames returns the set of flag names present in a helpJSON flags array.
// Names are stored as "--flagname" in the schema; this function returns the full
// "--flagname" form so callers can match directly against the schema value.
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

// TestJSONHelp_RootSchema asserts that "lyx --json" produces valid JSON with the
// expected schema fields and lists every module under commands.
func TestJSONHelp_RootSchema(t *testing.T) {
	var out bytes.Buffer
	code := run([]string{"--json"}, &out)
	if code != 0 {
		t.Fatalf("run([--json]) = %d; want 0. output:\n%s", code, out.String())
	}

	h := decodeHelpJSON(t, &out)

	// Schema top-level fields must be present.
	if h.Name == "" {
		t.Error("root JSON: name is empty")
	}
	if h.Short == "" {
		t.Error("root JSON: short is empty")
	}

	// commands must include every domain module; help and completion are excluded
	// by renderCmdJSON so we must NOT assert them here.
	cmds := commandNames(h.Commands)
	requiredModules := []string{
		"init", "board", "config", "ide", "mux", "weft", "warp", "selfreport",
	}
	for _, mod := range requiredModules {
		if !cmds[mod] {
			t.Errorf("root JSON commands missing module %q; commands: %v", mod, h.Commands)
		}
	}

	// Meta flags --json and --help must not appear in the flags array.
	flags := flagNames(h.Flags)
	for _, meta := range []string{"--json", "--help"} {
		if flags[meta] {
			t.Errorf("root JSON flags must not include meta flag %q", meta)
		}
	}
}

// TestJSONHelp_VerbModuleSchema asserts that "lyx board --json" (a verb-module)
// produces valid JSON naming the module's subcommands. This exercises the
// inherited HelpFunc on a non-root node.
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

	// commands must list board subcommands.
	cmds := commandNames(h.Commands)
	for _, sub := range []string{"upsert", "list", "remove", "sync"} {
		if !cmds[sub] {
			t.Errorf("board JSON commands missing %q; commands: %v", sub, h.Commands)
		}
	}

	// Hidden flag --board-path must not appear.
	flags := flagNames(h.Flags)
	if flags["--board-path"] {
		t.Error("board JSON flags must not expose hidden --board-path")
	}
}

// TestJSONHelp_SelfreportSchema asserts that "lyx selfreport --json" produces valid
// JSON with a non-empty short and lists the "create" subcommand under commands.
// This pins the parent module node of the selfreport help tree into the JSON schema test.
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

	// The selfreport module must list its "create" subcommand.
	cmds := commandNames(h.Commands)
	if !cmds["create"] {
		t.Errorf("selfreport JSON commands missing 'create'; commands: %v", h.Commands)
	}
}

// TestJSONHelp_LeafWithFlag asserts that "lyx warp remove --help --json" (a leaf
// command that owns --force) produces valid JSON with a populated flags array
// containing --force and an empty commands array. This prevents the flags assertion
// from being vacuous on a flag-less leaf.
func TestJSONHelp_LeafWithFlag(t *testing.T) {
	var out bytes.Buffer
	// --help triggers the HelpFunc even on a leaf; --json switches it to JSON mode.
	code := run([]string{"warp", "remove", "--help", "--json"}, &out)
	if code != 0 {
		t.Fatalf("run([warp remove --help --json]) = %d; want 0. output:\n%s", code, out.String())
	}

	h := decodeHelpJSON(t, &out)

	if !strings.Contains(h.Name, "remove") {
		t.Errorf("warp remove JSON name %q does not contain 'remove'", h.Name)
	}
	if h.Short == "" {
		t.Error("warp remove JSON: short is empty")
	}

	// A leaf command has no subcommands.
	if len(h.Commands) != 0 {
		t.Errorf("warp remove JSON commands: want empty, got %v", h.Commands)
	}

	// flags must be non-empty and include --force.
	if len(h.Flags) == 0 {
		t.Error("warp remove JSON: flags is empty; expected --force to be present")
	}
	flags := flagNames(h.Flags)
	if !flags["--force"] {
		t.Errorf("warp remove JSON flags missing --force; flags: %v", h.Flags)
	}

	// Meta flags must be absent.
	for _, meta := range []string{"--json", "--help"} {
		if flags[meta] {
			t.Errorf("warp remove JSON flags must not include meta flag %q", meta)
		}
	}
}

// TestJSONHelp_SelfreportCreateLeaf asserts that "lyx selfreport create --help --json"
// (a leaf command that owns --body and --label) produces valid JSON with a non-empty
// short, an empty commands array, and flags that include --body and --label while
// excluding meta flags --json and --help.
func TestJSONHelp_SelfreportCreateLeaf(t *testing.T) {
	var out bytes.Buffer
	// --help triggers the HelpFunc even on a leaf; --json switches it to JSON mode.
	code := run([]string{"selfreport", "create", "--help", "--json"}, &out)
	if code != 0 {
		t.Fatalf("run([selfreport create --help --json]) = %d; want 0. output:\n%s", code, out.String())
	}

	h := decodeHelpJSON(t, &out)

	if h.Short == "" {
		t.Error("selfreport create JSON: short is empty")
	}

	// A leaf command has no subcommands.
	if len(h.Commands) != 0 {
		t.Errorf("selfreport create JSON commands: want empty, got %v", h.Commands)
	}

	// flags must include --body and --label.
	flags := flagNames(h.Flags)
	for _, want := range []string{"--body", "--label"} {
		if !flags[want] {
			t.Errorf("selfreport create JSON flags missing %q; flags: %v", want, h.Flags)
		}
	}

	// Meta flags must be absent.
	for _, meta := range []string{"--json", "--help"} {
		if flags[meta] {
			t.Errorf("selfreport create JSON flags must not include meta flag %q", meta)
		}
	}
}
