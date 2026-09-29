// main_test.go covers the production deploy's pieces — the dirty-tree refusal, the marketplace
// parse, the plugin mirror, and the stray-binary report — plus -dev's destination and a
// source-level guard that the -ldflags -X path in run() still names a variable that actually exists
// at internal/buildinfo.
// Tests spawn no git, go build, or go env; the mirror test works in t.TempDir, so goBinDir() and
// the git-driven listing in syncPlugins are intentionally left uncovered here.

package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/tools/internal/devbin"
)

// TestResolveDest_DevUsesDerivedDevBinDir verifies -dev resolves to devbin.Dir().
func TestResolveDest_DevUsesDerivedDevBinDir(t *testing.T) {
	want, err := devbin.Dir()
	if err != nil {
		t.Fatalf("devbin.Dir() error: %v", err)
	}

	got, err := resolveDest(true)
	if err != nil {
		t.Fatalf("resolveDest(true) error: %v", err)
	}
	if got != want {
		t.Errorf("resolveDest(true) = %q; want %q", got, want)
	}
}

// TestDirtyError_RefusesAnyChange verifies production refuses a tree git reports any change in,
// and accepts a clean one.
func TestDirtyError_RefusesAnyChange(t *testing.T) {
	if err := dirtyError(""); err != nil {
		t.Errorf("dirtyError(\"\") = %v; want nil for a clean tree", err)
	}
	for _, porcelain := range []string{" M tools/deploy/main.go", "?? new.go"} {
		if err := dirtyError(porcelain); err == nil {
			t.Errorf("dirtyError(%q) = nil; want a refusal", porcelain)
		}
	}
}

// TestParseMarketplace_ReadsPluginEntries verifies the fields the plugin sync depends on.
func TestParseMarketplace_ReadsPluginEntries(t *testing.T) {
	m, err := parseMarketplace([]byte(`{"name":"loomyard","plugins":[{"name":"ly","version":"1.0.0","source":"./plugins/ly"}]}`))
	if err != nil {
		t.Fatalf("parseMarketplace error: %v", err)
	}
	if m.Name != "loomyard" || len(m.Plugins) != 1 || m.Plugins[0].Source != "./plugins/ly" || m.Plugins[0].Version != "1.0.0" {
		t.Errorf("parseMarketplace = %+v; want loomyard with one ly@1.0.0 entry sourced at ./plugins/ly", m)
	}
	if _, err := parseMarketplace([]byte(`{"plugins":[]}`)); err == nil {
		t.Error("parseMarketplace without a name = nil error; want an error")
	}
}

// TestMirrorFiles_ReplacesTargetWithExactlyTheListedFiles verifies the mirror copies the listed
// files with their mode, drops files outside the plugin, and removes whatever the cache held
// before (a stale built binary included).
func TestMirrorFiles_ReplacesTargetWithExactlyTheListedFiles(t *testing.T) {
	root := t.TempDir()
	write := func(rel string, mode os.FileMode) {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(rel), mode); err != nil {
			t.Fatal(err)
		}
	}
	write("plugins/ly/SKILL.md", 0o644)
	write("plugins/ly/bin/run.sh", 0o755)
	write("plugins/other/x.md", 0o644)

	target := filepath.Join(t.TempDir(), "ly", "1.0.0")
	if err := os.MkdirAll(filepath.Join(target, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "bin", "stale"), []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}

	files := []string{"plugins/ly/SKILL.md", "plugins/ly/bin/run.sh", "plugins/other/x.md"}
	if err := mirrorFiles(root, "plugins/ly", files, target); err != nil {
		t.Fatalf("mirrorFiles error: %v", err)
	}

	if _, err := os.Stat(filepath.Join(target, "bin", "stale")); !os.IsNotExist(err) {
		t.Errorf("stale cache file survived the mirror (stat err %v)", err)
	}
	if _, err := os.Stat(filepath.Join(target, "x.md")); !os.IsNotExist(err) {
		t.Errorf("a file outside the plugin's source was mirrored (stat err %v)", err)
	}
	if got, err := os.ReadFile(filepath.Join(target, "SKILL.md")); err != nil || string(got) != "plugins/ly/SKILL.md" {
		t.Errorf("SKILL.md = %q, %v; want the source content", got, err)
	}
	info, err := os.Stat(filepath.Join(target, "bin", "run.sh"))
	if err != nil {
		t.Fatalf("bin/run.sh not mirrored: %v", err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o100 == 0 {
		t.Errorf("bin/run.sh mode = %v; want the executable bit preserved", info.Mode().Perm())
	}
}

// TestOtherBinaries_ReportsEveryCopyOutsideTheDestDir verifies a second lyx on PATH is reported
// whether it sits before or after the production dir, and the production dir itself never is.
func TestOtherBinaries_ReportsEveryCopyOutsideTheDestDir(t *testing.T) {
	present := map[string]bool{
		filepath.Join("/a", "lyx"):      true,
		filepath.Join("/go/bin", "lyx"): true,
		filepath.Join("/c", "lyx"):      true,
	}
	exists := func(p string) bool { return present[p] }

	got := otherBinaries([]string{"/a", "/go/bin", "/b", "/c", "/go/bin/"}, "/go/bin", "lyx", exists)
	want := []string{filepath.Join("/a", "lyx"), filepath.Join("/c", "lyx")}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("otherBinaries = %v; want %v", got, want)
	}
}

// repoRoot locates the module root relative to this test file's own source location: tools/deploy
// is two levels below the root, so three filepath.Dir calls from runtime.Caller(0)'s file (which
// removes the filename itself, then "deploy", then "tools") land on the root.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) failed")
	}
	return filepath.Dir(filepath.Dir(filepath.Dir(file)))
}

// TestLdflagsPath_MatchesBuildinfoChannel guards the -X argument tools/deploy/main.go's run() passes
// to the linker against silent drift: Go's linker does not error on an unmatched -X, so a rename of
// either the variable or its package leaves a -dev build behaving as production, with no build
// error, no test failure, and no visible symptom until someone notices stencils refreshing when
// they should not.
func TestLdflagsPath_MatchesBuildinfoChannel(t *testing.T) {
	root := repoRoot(t)

	const wantLdflag = "-X github.com/Knatte18/loomyard/internal/buildinfo.Channel=dev"

	mainSrc, err := os.ReadFile(filepath.Join(root, "tools", "deploy", "main.go"))
	if err != nil {
		t.Fatalf("read tools/deploy/main.go: %v", err)
	}
	if !strings.Contains(string(mainSrc), wantLdflag) {
		t.Errorf("tools/deploy/main.go does not contain %q; the linker silently ignores an "+
			"unmatched -X, so a rename of internal/buildinfo.Channel or its package would leave "+
			"a -dev build behaving as production with no build error, no test failure, and no "+
			"visible symptom until someone notices stencils refreshing when they should not", wantLdflag)
	}

	buildinfoPath := filepath.Join(root, "internal", "buildinfo", "buildinfo.go")
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, buildinfoPath, nil, 0)
	if err != nil {
		t.Fatalf("parse internal/buildinfo/buildinfo.go: %v", err)
	}

	if !declaresExportedVar(file, "Channel") {
		t.Errorf("internal/buildinfo/buildinfo.go declares no exported var Channel; the linker "+
			"silently ignores an unmatched -X, so if this symbol is renamed or moved, tools/deploy's "+
			"%q stamp would stop matching and a -dev build would behave as production with no build "+
			"error, no test failure, and no visible symptom until someone notices stencils "+
			"refreshing when they should not", wantLdflag)
	}
}

// declaresExportedVar reports whether file declares a package-level exported var named name.
func declaresExportedVar(file *ast.File, name string) bool {
	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.VAR {
			continue
		}
		for _, spec := range genDecl.Specs {
			valueSpec, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for _, ident := range valueSpec.Names {
				if ident.Name == name && ident.IsExported() {
					return true
				}
			}
		}
	}
	return false
}
