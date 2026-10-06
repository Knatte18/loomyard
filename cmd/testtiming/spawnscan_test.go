package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

const (
	fixtureModule = "example.com/m"

	fixtureProd = `package p

import "os"

func prodExecutable() { _, _ = os.Executable() }
`

	// An untagged file: its tests are judged whatever they reference.
	fixtureUntaggedTests = `package p

import (
	"testing"

	"example.com/m/internal/testkit/lyxbin"
)

func TestUntaggedLyxbin(t *testing.T) { lyxbin.Build(t) }

func TestUntaggedUnresolved(t *testing.T) {
	f := func() {}
	f()
}
`

	// An integration-tier file: the rules decide.
	fixtureIntegrationTests = `//go:build integration

package p

import (
	"context"
	"os"
	"os/exec"
	"testing"

	"example.com/m/internal/gitkit"
	"example.com/m/internal/hubforge"
	"example.com/m/internal/prod"
	"example.com/m/internal/testkit/gitonlykit"
	"example.com/m/internal/testkit/gokit"
	"example.com/m/internal/testkit/quietkit"
	"example.com/m/internal/testkit/lyxbin"
)

type row struct{ fn func() }

func TestLyxbin(t *testing.T) { lyxbin.Build(t) }

func TestExecutable(t *testing.T) { _, _ = os.Executable() }

func TestArgsProgram(t *testing.T) { _ = os.Args[0] }

func TestArgsRest(t *testing.T) { _ = os.Args[1:] }

func TestGoCommand(t *testing.T) { _ = exec.Command("go", "build") }

func TestGoCommandContext(t *testing.T) { _ = exec.CommandContext(context.Background(), "go", "build") }

func TestGitCommand(t *testing.T) { _ = exec.Command("git", "status") }

func TestHelperChain(t *testing.T) { helperA() }
func helperA()                     { helperB() }
func helperB()                     { _, _ = os.Executable() }

func TestHermetic(t *testing.T) { gitkit.HermeticGitEnv() }

func TestGitkitSpawn(t *testing.T) { gitkit.Run() }

func TestHubforge(t *testing.T) { hubforge.NewHub(t) }

func TestFuncValue(t *testing.T) {
	f := func() {}
	f()
}

func TestFuncField(t *testing.T) {
	r := row{}
	r.fn()
}

func TestProdCall(t *testing.T) { prod.Run() }

func TestSamePackageProd(t *testing.T) { prodExecutable() }

func TestKitGitOnly(t *testing.T) { gitonlykit.Do() }

func TestKitRunsGo(t *testing.T) { gokit.Do() }

func TestKitQuiet(t *testing.T) { quietkit.Do() }

func TestPlain(t *testing.T) { t.Helper() }
`

	fixtureTmuxTests = "//go:build tmux\n\npackage p\n\nimport \"testing\"\n\nfunc TestTmuxTier(t *testing.T) { t.Helper() }\n"

	fixtureTmuxLinuxTests = "//go:build tmux && linux\n\npackage p\n\nimport \"testing\"\n\nfunc TestTmuxLinuxTier(t *testing.T) { t.Helper() }\n"

	fixtureGitOnlyKit = "package gitonlykit\n\nimport \"os/exec\"\n\nfunc Do() { _ = exec.Command(\"git\", \"init\") }\n"
	fixtureGoKit      = "package gokit\n\nimport \"os/exec\"\n\nfunc Do() { _ = exec.Command(\"go\", \"build\") }\n"
	fixtureQuietKit   = "package quietkit\n\nfunc Do() {}\n"
)

func writeFixture(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestScanSpawns(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	pkgDir := filepath.Join(root, "p")
	kitRoot := filepath.Join(root, "kits")
	writeFixture(t, pkgDir, "p.go", fixtureProd)
	writeFixture(t, pkgDir, "untagged_test.go", fixtureUntaggedTests)
	writeFixture(t, pkgDir, "integration_test.go", fixtureIntegrationTests)
	writeFixture(t, pkgDir, "tmux_test.go", fixtureTmuxTests)
	writeFixture(t, pkgDir, "tmuxlinux_test.go", fixtureTmuxLinuxTests)
	writeFixture(t, filepath.Join(kitRoot, "gitonlykit"), "gitonlykit.go", fixtureGitOnlyKit)
	writeFixture(t, filepath.Join(kitRoot, "gokit"), "gokit.go", fixtureGoKit)
	writeFixture(t, filepath.Join(kitRoot, "quietkit"), "quietkit.go", fixtureQuietKit)

	got, err := scanSpawns(fixtureModule, kitRoot, pkgDir)
	if err != nil {
		t.Fatal(err)
	}

	outOfProcess := spawnVerdict{outOfProcess: true}
	unresolved := spawnVerdict{unresolved: true}
	want := map[string]spawnVerdict{
		"TestUntaggedLyxbin":     {},
		"TestUntaggedUnresolved": {},
		"TestLyxbin":             outOfProcess,
		"TestExecutable":         outOfProcess,
		"TestArgsProgram":        outOfProcess,
		"TestArgsRest":           {},
		"TestGoCommand":          outOfProcess,
		"TestGoCommandContext":   outOfProcess,
		"TestGitCommand":         {},
		"TestHelperChain":        outOfProcess,
		"TestHermetic":           {},
		"TestGitkitSpawn":        {},
		"TestHubforge":           {},
		"TestFuncValue":          unresolved,
		"TestFuncField":          unresolved,
		"TestProdCall":           {},
		"TestSamePackageProd":    outOfProcess,
		"TestKitGitOnly":         {},
		"TestKitRunsGo":          outOfProcess,
		"TestKitQuiet":           {},
		"TestPlain":              {},
		"TestTmuxTier":           outOfProcess,
		"TestTmuxLinuxTier":      outOfProcess,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("scanSpawns =\n%+v\nwant\n%+v", got, want)
	}
}
