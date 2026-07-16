// helptree_test.go asserts that the lyx help tree is complete: the root help
// output names every module, and each verb-module's help output names every one
// of its subcommands. Tests use superset assertions (pinned set ⊆ output) so
// that cobra's auto-generated help and completion entries do not make them brittle.

package main

import (
	"bytes"
	"strings"
	"testing"
)

// TestHelpTree_RootNamesAllModules asserts that running "lyx --help" (no args, which
// triggers cobra's help for the root) produces output that names every registered
// module. The assertion is a superset check — extra text such as cobra's "help" and
// "completion" entries is tolerated.
func TestHelpTree_RootNamesAllModules(t *testing.T) {
	var out bytes.Buffer
	code := run(nil, &out)
	if code != 0 {
		t.Fatalf("run(nil) = %d; want 0. output:\n%s", code, out.String())
	}

	got := out.String()
	// Every module must appear by name in the root help output.
	requiredModules := []string{
		"init", "board", "config", "ide", "mux", "weft", "warp", "selfreport", "shuttle", "burler", "perch", "builder",
	}
	for _, module := range requiredModules {
		if !strings.Contains(got, module) {
			t.Errorf("root help output missing module %q; got:\n%s", module, got)
		}
	}

	// Assert that the reconcile subcommand is discoverable under config.
	// Invoke "lyx config --help" which short-circuits cobra's help and exits 0.
	var configOut bytes.Buffer
	if code := run([]string{"config", "--help"}, &configOut); code != 0 {
		t.Fatalf("run([config --help]) = %d; want 0. output:\n%s", code, configOut.String())
	}
	if !strings.Contains(configOut.String(), "reconcile") {
		t.Errorf("config --help output missing %q; got:\n%s", "reconcile", configOut.String())
	}
}

// TestHelpTree_VerbModuleSubcommands asserts that each verb-module's help output
// names all of its subcommands. Each module is invoked via the run() seam with
// only the module name so cobra prints the subcommand listing (exit 0). Assertions
// are superset checks so cobra's auto-added "help" entry is tolerated.
func TestHelpTree_VerbModuleSubcommands(t *testing.T) {
	tests := []struct {
		name     string
		module   string
		wantSubs []string
	}{
		{
			name:   "board",
			module: "board",
			wantSubs: []string{
				"upsert", "upsert-batch", "set-status", "remove", "get",
				"list", "list-full", "merge", "set-deps", "rerender", "sync",
			},
		},
		{
			name:   "warp",
			module: "warp",
			wantSubs: []string{
				"clone", "add", "list", "remove", "checkout",
				"pairs", "reconcile", "prune", "cleanup",
			},
		},
		{
			name:     "weft",
			module:   "weft",
			wantSubs: []string{"status", "commit", "push", "pull", "sync"},
		},
		{
			name:     "ide",
			module:   "ide",
			wantSubs: []string{"spawn", "menu"},
		},
		{
			name:     "mux",
			module:   "mux",
			wantSubs: []string{"up", "add", "remove", "status", "attach", "resume", "down", "header"},
		},
		{
			name:     "selfreport",
			module:   "selfreport",
			wantSubs: []string{"create"},
		},
		{
			name:     "shuttle",
			module:   "shuttle",
			wantSubs: []string{"run", "interrupt", "send"},
		},
		{
			name:     "burler",
			module:   "burler",
			wantSubs: []string{"run"},
		},
		{
			name:     "perch",
			module:   "perch",
			wantSubs: []string{"run", "pause"},
		},
		{
			name:     "builder",
			module:   "builder",
			wantSubs: []string{"validate", "status", "spawn-batch", "poll", "run", "pause"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			// Invoking a verb-module with no subcommand prints the subcommand listing and exits 0.
			code := run([]string{tt.module}, &out)
			if code != 0 {
				t.Fatalf("run([%q]) = %d; want 0. output:\n%s", tt.module, code, out.String())
			}

			got := out.String()
			for _, sub := range tt.wantSubs {
				if !strings.Contains(got, sub) {
					t.Errorf("help output for %q missing subcommand %q; got:\n%s", tt.module, sub, got)
				}
			}
		})
	}
}
