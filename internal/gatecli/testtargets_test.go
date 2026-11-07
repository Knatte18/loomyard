// testtargets_test.go covers how a tier's {test_targets} narrows its targets to the packages holding a test of the tier.

package gatecli

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// writeTierModule writes a module with package a, whose test requires tier 1, and package b, whose test requires tier 0.
func writeTierModule(t *testing.T) string {
	t.Helper()
	moduleDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(moduleDir, "go.mod"), []byte("module example.com/m\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	const header = "package p\n\nimport (\n\t\"testing\"\n\n\t\"github.com/Knatte18/loomyard/internal/testkit/tiergate\"\n)\n\n"
	for dir, tier := range map[string]string{"a": "1", "b": "0"} {
		if err := os.MkdirAll(filepath.Join(moduleDir, dir), 0o755); err != nil {
			t.Fatal(err)
		}
		content := header + "func TestP(t *testing.T) {\n\ttiergate.Require(t, " + tier + ")\n}\n"
		if err := os.WriteFile(filepath.Join(moduleDir, dir, "p_test.go"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return moduleDir
}

func TestTierTestTargets(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	moduleDir := writeTierModule(t)
	// Enough import paths, each kept, to push the narrowed list past the command budget.
	pastBudget := []string{"./b"}
	for len(strings.Join(pastBudget, " ")) <= greenCommandBudget {
		pastBudget = append(pastBudget, fmt.Sprintf("example.com/m/package%04d", len(pastBudget)))
	}

	tests := []struct {
		name string
		// goBinary is the go command that lists a pattern's packages; empty means the default.
		goBinary string
		covers   []int
		targets  []string
		want     []string
	}{
		{name: "a package without a test of the tier is left out", targets: []string{"./a", "./b"}, want: []string{"./a"}},
		{name: "a package with a test of a covered tier is kept", covers: []int{0}, targets: []string{"./b", "./a"}, want: []string{"./a", "./b"}},
		{name: "no package with a test of the tier leaves none", targets: []string{"./b"}},
		{name: "an import path is kept", targets: []string{"example.com/m/c", "./b"}, want: []string{"example.com/m/c"}},
		{name: "a narrowed list past the command budget runs every target", targets: pastBudget, want: pastBudget},
		{name: "a pattern is listed and narrowed", targets: []string{"./..."}, want: []string{"./a"}},
		{name: "a pattern whose packages cannot be listed runs every target", goBinary: "no-such-go-binary", targets: []string{"./..."}, want: []string{"./..."}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			goBinary := tt.goBinary
			if goBinary == "" {
				goBinary = defaultGoBinary
			}
			run := tierRun{number: 1, tier: gateslot.Tier{Covers: tt.covers}}
			got := tierTestTargets(context.Background(), goBinary, moduleDir, run, tt.targets, nil)
			if !slices.Equal(got, tt.want) {
				t.Errorf("tierTestTargets(%q) = %q; want %q", tt.targets, got, tt.want)
			}
		})
	}
}

func TestRunInvocationsTestTargets(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	moduleDir := writeTierModule(t)
	tests := []struct {
		name        string
		invocations [][]string
		wantOutput  string
		wantStderr  string
	}{
		{
			name:        "the command gets the kept packages in {test_targets} and every target in {targets}",
			invocations: [][]string{{"./a", "./b"}},
			wantOutput:  "narrowed ./a\nwhole ./a ./b\n",
		},
		{
			name:        "an invocation left with no package runs no command and says so",
			invocations: [][]string{{"./b"}},
			wantStderr:  "gate test: no package of ./b in " + moduleDir + " holds a tier1 test, so the command of tier1 does not run there\n",
		},
		{
			name:        "an invocation left with no package does not stop the next one",
			invocations: [][]string{{"./b"}, {"./a"}},
			wantOutput:  "narrowed ./a\nwhole ./a\n",
			wantStderr:  "gate test: no package of ./b in " + moduleDir + " holds a tier1 test, so the command of tier1 does not run there\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var output, stderr bytes.Buffer
			counter, err := newCounter(counterGoJSON, &output)
			if err != nil {
				t.Fatal(err)
			}
			run := tierRun{number: 1, tier: gateslot.Tier{Command: "echo narrowed {test_targets} && echo whole {targets}"}}
			for _, targets := range tt.invocations {
				run.invocations = append(run.invocations, invocation{dir: moduleDir, targets: targets})
			}
			request := testRequest{goBinary: defaultGoBinary, root: moduleDir, stderr: &stderr, pipeWaitDelay: defaultPipeWaitDelay}

			exit, _, refusal := runInvocations(context.Background(), run, counter, nil, request)
			if err := counter.Flush(); err != nil {
				t.Fatal(err)
			}
			if exit != 0 || refusal != "" {
				t.Errorf("runInvocations() = exit %d, refusal %q; want exit 0 and no refusal", exit, refusal)
			}
			if output.String() != tt.wantOutput {
				t.Errorf("command output = %q; want %q", output.String(), tt.wantOutput)
			}
			if stderr.String() != tt.wantStderr {
				t.Errorf("stderr = %q; want %q", stderr.String(), tt.wantStderr)
			}
		})
	}
}
