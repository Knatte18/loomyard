//go:build integration

// verify_integration_test.go drives the `verify` verb against a real scratch repo and real shell processes, so it carries the integration tag.
// The plan, the verify directory and the webster directories all lie outside the repo, so a verify run never dirties the tree it checks.
// The verb runs the rendered `final` gate call through the `lyx` built from the tree, which each fixture puts first on `PATH`, so every test here is not parallel.

package webstercli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/testkit/envelope"
	"github.com/Knatte18/loomyard/internal/testkit/lyxbin"
	"github.com/Knatte18/loomyard/internal/testkit/plankit"
	"github.com/Knatte18/loomyard/internal/verifytree"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// verifyFixture is a one-commit repo holding a gate.yaml, with a plan under an anchor outside it, and the told geometry over both.
type verifyFixture struct {
	CLI      *websterCLI
	Worktree string
	Anchor   string
}

// verifyGateConfig returns the gate.yaml a fixture worktree commits: tier0 runs command as a shell stand-in, every gate lists it, and the `final` gate lists finalTier instead.
func verifyGateConfig(t *testing.T, command string, finalTier int) []byte {
	t.Helper()
	data, err := yaml.Marshal(map[string]any{
		"slots":       1,
		"go_parallel": 1,
		"scoper":      "none",
		"tiers": map[string]any{
			"tier0": map[string]any{"about": "shell stand-in", "command": command, "timeout_min": 1, "counter": "go-json"},
		},
		"gates": map[string]any{
			"card": []int{0}, "batch": []int{0}, "round": []int{0}, "final": []int{finalTier}, "landing": []int{0}, "publish": []int{0},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// newVerifyFixture builds a fixture whose committed gate.yaml has tier0 run command as a shell stand-in, and whose told GateConfig reads that file.
// The repo excludes its .lyx directory, where a standalone `lyx gate test` writes its run record, so the tree stays clean for the verify.
// It puts the `lyx` built from the tree first on `PATH`, so the calling test is not parallel.
func newVerifyFixture(t *testing.T, command string) *verifyFixture {
	t.Helper()
	lyxbin.PutFirstOnPath(t)

	worktree := t.TempDir()
	gitkit.Git(t, worktree, "init", "-b", "main")
	gitkit.Git(t, worktree, "config", "user.name", "Test User")
	gitkit.Git(t, worktree, "config", "user.email", "test@example.com")
	exclude := filepath.Join(worktree, ".git", "info", "exclude")
	if err := os.MkdirAll(filepath.Dir(exclude), 0o755); err != nil {
		t.Fatalf("create the exclude directory: %v", err)
	}
	if err := os.WriteFile(exclude, []byte(lyxdirs.DotLyxDirName+"/\n"), 0o644); err != nil {
		t.Fatalf("write the exclude file: %v", err)
	}
	configRel, err := filepath.Rel(worktree, configengine.ConfigFile(worktree, "gate"))
	if err != nil {
		t.Fatalf("locate gate.yaml: %v", err)
	}
	gitkit.CommitFile(t, worktree, filepath.ToSlash(configRel), string(verifyGateConfig(t, command, 0)), "gate config")
	gitkit.CommitFile(t, worktree, "base.txt", "base\n", "base commit")

	anchor := t.TempDir()
	plankit.Write(t, planparser.PlanDir(anchor), onlyCreatePlan("", "internal/only/new.go"))

	websterDir := filepath.Join(anchor, "webster")
	geom := websterengine.Geometry{
		AnchorRoot:   anchor,
		WorktreeRoot: worktree,
		WebsterDir:   websterDir,
		ReportsDir:   filepath.Join(websterDir, "reports"),
		PromptsDir:   filepath.Join(websterDir, "prompts"),
		ScratchDir:   filepath.Join(anchor, "scratch"),
		PlanDir:      planparser.PlanDir(anchor),
		VerifyDir:    verifytree.Dir(anchor),
		GateConfig: func() (gateslot.Config, error) {
			return gateslot.LoadConfig(configengine.Dirs{Anchor: worktree})
		},
	}
	return &verifyFixture{CLI: &websterCLI{geom: geom}, Worktree: worktree, Anchor: anchor}
}

// run drives the verb and returns its exit code and decoded envelope.
func (fx *verifyFixture) run(t *testing.T) (int, envelope.Envelope) {
	t.Helper()
	var out bytes.Buffer
	code := clihelp.Execute(fx.CLI.verifyCmd(), &out, nil)
	return code, envelope.Decode(t, out.String())
}

func TestVerifyVerb_PassRecordsTheTree(t *testing.T) {
	fx := newVerifyFixture(t, "exit 0")

	code, env := fx.run(t)

	if code != 0 || !env.OK {
		t.Fatalf("verify on a passing plan = %d, envelope %+v; want 0 and ok", code, env)
	}
	if _, err := os.Stat(verifytree.NewPaths(fx.Worktree, fx.CLI.geom.VerifyDir).Record); err != nil {
		t.Errorf("a passing verify wrote no record: %v", err)
	}
}

func TestVerifyVerb_SecondCallOnTheSameTreeSkips(t *testing.T) {
	fx := newVerifyFixture(t, "exit 0")
	if code, _ := fx.run(t); code != 0 {
		t.Fatalf("first verify = %d; want 0", code)
	}
	logPattern := filepath.Join(fx.CLI.geom.VerifyDir, "verify-*.log")
	logsBefore, err := filepath.Glob(logPattern)
	if err != nil {
		t.Fatal(err)
	}

	code, env := fx.run(t)

	if code != 0 || env.Raw["status"] != string(verifytree.StatusSkipped) {
		t.Errorf("second verify = %d, envelope %+v; want 0 and status %q", code, env, verifytree.StatusSkipped)
	}
	if logs, err := filepath.Glob(logPattern); err != nil || len(logs) != len(logsBefore) {
		t.Errorf("verify logs after the second verify = %q, %v; want %q, the command not to have run", logs, err, logsBefore)
	}
}

func TestVerifyVerb_FailureExitsOneWithFindings(t *testing.T) {
	fx := newVerifyFixture(t, "exit 3")

	code, env := fx.run(t)

	if code != 1 || env.OK {
		t.Fatalf("verify on a failing plan = %d, envelope %+v; want 1 and not ok", code, env)
	}
	findings, ok := env.Raw["findings"].(map[string]any)
	if !ok {
		t.Fatalf("failure envelope has no findings object: %+v", env.Raw)
	}
	if findings["exit_code"] != float64(3) {
		t.Errorf("findings exit_code = %v; want 3", findings["exit_code"])
	}
	if want := filepath.Join(fx.CLI.geom.VerifyDir, "verify-1.log"); findings["log"] != want {
		t.Errorf("findings log = %v; want the run's own log %s", findings["log"], want)
	}
}

func TestVerifyVerb_DirtyTreeExitsOneNamingPaths(t *testing.T) {
	fx := newVerifyFixture(t, "exit 0")
	if err := os.WriteFile(filepath.Join(fx.Worktree, "stray.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write stray file: %v", err)
	}

	code, env := fx.run(t)

	if code != 1 || env.OK {
		t.Fatalf("verify on a dirty tree = %d, envelope %+v; want 1 and not ok", code, env)
	}
	findings, _ := env.Raw["findings"].(map[string]any)
	if got := findings["dirty"]; !strings.Contains(strings.Join(anyStrings(got), ","), "stray.txt") {
		t.Errorf("findings dirty = %v; want it to name stray.txt", got)
	}
}

// TestVerifyVerb_GateConfigRefusalIsTheError pins that a gate.yaml the verb cannot load, or a `final` list naming an undeclared tier, is the verb's error naming `lyx config gate`, never a pass.
func TestVerifyVerb_GateConfigRefusalIsTheError(t *testing.T) {
	tests := []struct {
		name string
		// breakConfig turns the fixture's committed gate.yaml into the refused shape.
		breakConfig func(t *testing.T, configFile string)
	}{
		{"an absent gate.yaml", func(t *testing.T, configFile string) {
			if err := os.Remove(configFile); err != nil {
				t.Fatal(err)
			}
		}},
		{"a final list naming an undeclared tier", func(t *testing.T, configFile string) {
			if err := os.WriteFile(configFile, verifyGateConfig(t, "exit 0", 7), 0o644); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fx := newVerifyFixture(t, "exit 0")
			tt.breakConfig(t, configengine.ConfigFile(fx.Worktree, "gate"))

			code, env := fx.run(t)

			if code != 1 || env.OK || !strings.Contains(env.Error, "lyx config gate") {
				t.Errorf("verify over %s = %d, envelope %+v; want 1 and an error naming lyx config gate", tt.name, code, env)
			}
		})
	}
}

// anyStrings returns v's elements as strings when v is a decoded JSON array of strings.
func anyStrings(v any) []string {
	items, _ := v.([]any)
	var out []string
	for _, item := range items {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}
