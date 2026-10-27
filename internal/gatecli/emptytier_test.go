//go:build tier0

// emptytier_test.go covers the failure of a go-json tier whose command lacks its tag: a package that holds tests under the tier's tag yet ran none.

package gatecli

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestEmptyTierFailure(t *testing.T) {
	t.Parallel()

	otherOS := "windows"
	if runtime.GOOS == "windows" {
		otherOS = "linux"
	}
	const testFile = "package p\n\nimport \"testing\"\n\nfunc TestA(t *testing.T) {}\n"

	tests := []struct {
		name string
		// files are the package's test files, keyed by name.
		files map[string]string
		// ran is the count of tests that ran in the package.
		ran int
		// args are the call's own words after `--`, and runArgs the run's, which the guard step appends its filter to.
		args, runArgs []string
		wantFailure   bool
	}{
		{
			name:        "a package holding a tier1 test file that ran none fails",
			files:       map[string]string{"p_test.go": "//go:build tier1\n\n" + testFile, "main_test.go": "package p\n\nimport (\n\t\"os\"\n\t\"testing\"\n)\n\nfunc TestMain(m *testing.M) { os.Exit(m.Run()) }\n"},
			wantFailure: true,
		},
		{
			name:  "a tier1 test file constrained to another GOOS does not fail",
			files: map[string]string{"p_test.go": "//go:build tier1 && " + otherOS + "\n\n" + testFile},
		},
		{
			name:  "a test file under another tier's tag does not fail",
			files: map[string]string{"p_test.go": "//go:build tier2\n\n" + testFile},
		},
		{
			name:  "a package with only a TestMain does not fail",
			files: map[string]string{"main_test.go": "//go:build tier1\n\npackage p\n\nimport (\n\t\"os\"\n\t\"testing\"\n)\n\nfunc TestMain(m *testing.M) { os.Exit(m.Run()) }\n"},
		},
		{
			name:  "a -run among the call's args exempts the run",
			files: map[string]string{"p_test.go": "//go:build tier1\n\n" + testFile},
			args:  []string{"-run", "TestB"},
		},
		{
			name:  "a -skip among the call's args exempts the run",
			files: map[string]string{"p_test.go": "//go:build tier1\n\n" + testFile},
			args:  []string{"-skip", "TestB"},
		},
		{
			name:  "a -list among the call's args exempts the run",
			files: map[string]string{"p_test.go": "//go:build tier1\n\n" + testFile},
			args:  []string{"-list", "TestB"},
		},
		{
			name:  "a -bench among the call's args exempts the run",
			files: map[string]string{"p_test.go": "//go:build tier1\n\n" + testFile},
			args:  []string{"-bench", "BenchB"},
		},
		{
			name:  "a -test.run=value among the call's args exempts the run",
			files: map[string]string{"p_test.go": "//go:build tier1\n\n" + testFile},
			args:  []string{"-test.run=TestB"},
		},
		{
			name:  "a double-dash --run among the call's args exempts the run",
			files: map[string]string{"p_test.go": "//go:build tier1\n\n" + testFile},
			args:  []string{"--run", "TestB"},
		},
		{
			name:        "a guard step's -run among the run's args exempts nothing",
			files:       map[string]string{"p_test.go": "//go:build tier1\n\n" + testFile},
			runArgs:     []string{"-run", "^(TestA)$"},
			wantFailure: true,
		},
		{
			name:  "a package that ran a test passes",
			files: map[string]string{"p_test.go": "//go:build tier1\n\n" + testFile},
			ran:   1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			dir := filepath.Join(root, "p")
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/m\n\ngo 1.21\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			for name, content := range tt.files {
				if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
					t.Fatal(err)
				}
			}

			run := tierRun{number: 1, invocations: []invocation{{dir: root, targets: []string{"./..."}}}, args: tt.runArgs}
			got := emptyTierFailure(run, root, map[string]int{"example.com/m/p": tt.ran}, tt.args)
			if !tt.wantFailure {
				if got != "" {
					t.Errorf("emptyTierFailure() = %q; want no failure", got)
				}
				return
			}
			for _, want := range []string{"tier1", "example.com/m/p", "lym config gate"} {
				if !strings.Contains(got, want) {
					t.Errorf("emptyTierFailure() = %q; want it to name %q", got, want)
				}
			}
		})
	}
}
