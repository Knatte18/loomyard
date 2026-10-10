//go:build integration

// verify_integration_test.go drives the `verify` verb against a real scratch repo and real shell processes, so it carries the integration tag.
// The plan, the verify directory and the webster directories all lie outside the repo, so a verify run never dirties the tree it checks.

package webstercli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/testkit/envelope"
	"github.com/Knatte18/loomyard/internal/testkit/plankit"
	"github.com/Knatte18/loomyard/internal/verifytree"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// verifyFixture is a one-commit repo with a plan under an anchor outside it, and the told geometry over both.
type verifyFixture struct {
	CLI      *websterCLI
	Worktree string
	Anchor   string
}

// newVerifyFixture builds a fixture whose plan carries command as its `## verify:` section, or no such section when command is empty.
func newVerifyFixture(t *testing.T, command string) *verifyFixture {
	t.Helper()

	worktree := t.TempDir()
	gitkit.Git(t, worktree, "init", "-b", "main")
	gitkit.Git(t, worktree, "config", "user.name", "Test User")
	gitkit.Git(t, worktree, "config", "user.email", "test@example.com")
	gitkit.CommitFile(t, worktree, "base.txt", "base\n", "base commit")

	anchor := t.TempDir()
	plan := onlyCreatePlan("", "internal/only/new.go")
	if command != "" {
		plan.Sections = append(plan.Sections, plankit.Section{Heading: "verify:", Body: "```\n" + command + "\n```"})
	}
	plankit.Write(t, planparser.PlanDir(anchor), plan)

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

func TestVerifyVerb_NoVerifySectionExitsZeroWithWarning(t *testing.T) {
	fx := newVerifyFixture(t, "")

	code, env := fx.run(t)

	if code != 0 || !env.OK {
		t.Fatalf("verify on a plan with no verify section = %d, envelope %+v; want 0 and ok", code, env)
	}
	if _, ok := env.Raw["warning"].(string); !ok {
		t.Errorf("envelope carries no warning: %+v", env.Raw)
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
