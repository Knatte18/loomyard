//go:build integration

// prebuild_integration_test.go pins the harness's one lyx build per tagged run.
// It is Tier 2: the building rows spawn a real go build of a throwaway module, or the test binary as a stand-in go, so it needs the integration build tag.

package main

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
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

	template, err := gateslot.TemplateConfig()
	if err != nil {
		t.Fatal(err)
	}

	rows := []struct {
		name          string
		tags          string
		mainSource    string
		standInGo     bool
		wantBuilt     bool
		wantBuildArgs func(root, bin string) []string
		wantError     string
	}{
		{name: "an untagged run builds nothing and strips the inherited value", mainSource: "package main\n\nfunc main() {}\n"},
		{name: "a tagged run without cmd/lyx builds nothing and strips the inherited value", tags: "integration"},
		{name: "a tagged run builds cmd/lyx and exports it", tags: "integration", mainSource: "package main\n\nfunc main() {}\n", wantBuilt: true},
		{
			name: "a tagged run outside a hub caps its build at the gate template's -p", tags: "integration", mainSource: "package main\n\nfunc main() {}\n", standInGo: true, wantBuilt: true,
			wantBuildArgs: func(root, bin string) []string {
				return []string{"build", "-C", root, "-p", strconv.Itoa(template.GoParallel), "-o", bin, "./cmd/lyx"}
			},
		},
		{name: "a tagged run whose cmd/lyx does not build returns the build output", tags: "integration", mainSource: "package main\n\nfunc main() { undefinedCall() }\n", wantError: "undefinedCall"},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()

			root := module(t, row.mainSource)
			goBinary, base := "go", append(os.Environ(), inherited)
			record := filepath.Join(t.TempDir(), "go-args")
			if row.standInGo {
				testBinary, err := os.Executable()
				if err != nil {
					t.Fatal(err)
				}
				goBinary, base = testBinary, append(base, standInGoRecordEnv+"="+record)
			}
			env, cleanup, err := prebuildLyx(goBinary, row.tags, root, base)
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
			if row.wantBuildArgs != nil {
				recorded, err := os.ReadFile(record)
				if err != nil {
					t.Fatal(err)
				}
				if got, want := strings.Split(string(recorded), "\n"), row.wantBuildArgs(root, exported[0]); !slices.Equal(got, want) {
					t.Errorf("build args = %q; want %q", got, want)
				}
			}
			cleanup()
			if _, err := os.Stat(filepath.Dir(exported[0])); !os.IsNotExist(err) {
				t.Errorf("the build directory %s survived the cleanup (stat err=%v); want it removed", filepath.Dir(exported[0]), err)
			}
		})
	}
}
