//go:build integration

// prebuild_integration_test.go pins the harness's one lyx build per tagged run.
// It is Tier 2: the building rows spawn a real go build of a throwaway module, so it needs the integration build tag.

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/gateslot"
)

func TestPrebuildLyx(t *testing.T) {
	t.Parallel()

	inherited := gateslot.PrebuiltLyxEnv + "=/stale/lyx"
	module := func(t *testing.T, mainSource string) string {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/prebuild\n\ngo 1.22\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if mainSource == "" {
			return root
		}
		if err := os.MkdirAll(filepath.Join(root, "cmd", "lyx"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "cmd", "lyx", "main.go"), []byte(mainSource), 0o644); err != nil {
			t.Fatal(err)
		}
		return root
	}

	rows := []struct {
		name       string
		tags       string
		mainSource string
		wantBuilt  bool
		wantError  string
	}{
		{name: "an untagged run builds nothing and strips the inherited value", mainSource: "package main\n\nfunc main() {}\n"},
		{name: "a tagged run without cmd/lyx builds nothing and strips the inherited value", tags: "integration"},
		{name: "a tagged run builds cmd/lyx and exports it", tags: "integration", mainSource: "package main\n\nfunc main() {}\n", wantBuilt: true},
		{name: "a tagged run whose cmd/lyx does not build returns the build output", tags: "integration", mainSource: "package main\n\nfunc main() { undefinedCall() }\n", wantError: "undefinedCall"},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()

			root := module(t, row.mainSource)
			env, cleanup, err := prebuildLyx(row.tags, root, append(os.Environ(), inherited), 2)
			if row.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), row.wantError) {
					t.Fatalf("err = %v; want an error holding %q", err, row.wantError)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}

			var exported []string
			for _, entry := range env {
				if value, ok := strings.CutPrefix(entry, gateslot.PrebuiltLyxEnv+"="); ok {
					exported = append(exported, value)
				}
			}
			if !row.wantBuilt {
				cleanup()
				if len(exported) != 0 {
					t.Errorf("env carries %s = %q; want it stripped", gateslot.PrebuiltLyxEnv, exported)
				}
				return
			}
			if len(exported) != 1 {
				t.Fatalf("env carries %s = %q; want the one built binary", gateslot.PrebuiltLyxEnv, exported)
			}
			if info, err := os.Stat(exported[0]); err != nil || !info.Mode().IsRegular() {
				t.Fatalf("the exported binary %s is not a file (err=%v)", exported[0], err)
			}
			cleanup()
			if _, err := os.Stat(filepath.Dir(exported[0])); !os.IsNotExist(err) {
				t.Errorf("the build directory %s survived the cleanup (stat err=%v); want it removed", filepath.Dir(exported[0]), err)
			}
		})
	}
}
