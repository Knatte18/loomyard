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

import "os/exec"

func prodSpawner() { _ = exec.Command("y") }
`

	fixtureTests = `package p

import (
	"os/exec"
	"strings"
	"testing"

	"example.com/m/internal/gitkit"
	"example.com/m/internal/prod"
	"example.com/m/internal/testkit/quietkit"
	"example.com/m/internal/testkit/spawnkit"
	"example.com/m/internal/testkit/tmuxkit"
)

type row struct{ fn func() }

func TestDirect(t *testing.T) { _ = exec.Command("x") }

func TestHelperChain(t *testing.T) { helperA() }
func helperA()                     { helperB() }
func helperB()                     { _ = exec.LookPath("x") }

func TestMainAlone(t *testing.T) {
	tmuxkit.Main(nil)
	_ = strings.ToUpper("x")
}

func TestTmuxSocket(t *testing.T) { tmuxkit.Socket(t) }

func TestHermetic(t *testing.T) { gitkit.HermeticGitEnv() }

func TestGitSpawn(t *testing.T) { gitkit.Run() }

func TestFuncValue(t *testing.T) {
	f := func() {}
	f()
}

func TestFuncField(t *testing.T) {
	r := row{}
	r.fn()
}

func TestProdCall(t *testing.T) { prod.Run() }

func TestSamePackageProd(t *testing.T) { prodSpawner() }

func TestKitSpawn(t *testing.T) { spawnkit.Do() }

func TestKitQuiet(t *testing.T) { quietkit.Do() }

func TestPlain(t *testing.T) { t.Helper() }
`

	fixtureSpawnKit = "package spawnkit\n\nimport \"os/exec\"\n\nfunc Do() { _ = exec.Command(\"z\") }\n"
	fixtureQuietKit = "package quietkit\n\nfunc Do() {}\n"
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
	writeFixture(t, pkgDir, "p_test.go", fixtureTests)
	writeFixture(t, filepath.Join(kitRoot, "spawnkit"), "spawnkit.go", fixtureSpawnKit)
	writeFixture(t, filepath.Join(kitRoot, "quietkit"), "quietkit.go", fixtureQuietKit)

	got, err := scanSpawns(fixtureModule, kitRoot, pkgDir)
	if err != nil {
		t.Fatal(err)
	}

	spawns := spawnVerdict{spawns: true}
	unresolved := spawnVerdict{unresolved: true}
	want := map[string]spawnVerdict{
		"TestDirect":          spawns,
		"TestHelperChain":     spawns,
		"TestMainAlone":       {},
		"TestTmuxSocket":      spawns,
		"TestHermetic":        {},
		"TestGitSpawn":        spawns,
		"TestFuncValue":       unresolved,
		"TestFuncField":       unresolved,
		"TestProdCall":        {},
		"TestSamePackageProd": spawns,
		"TestKitSpawn":        spawns,
		"TestKitQuiet":        {},
		"TestPlain":           {},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("scanSpawns =\n%+v\nwant\n%+v", got, want)
	}
}
