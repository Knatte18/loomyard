//go:build tier0

// main_test.go covers the production deploy's pieces — the dirty-tree refusal, the marketplace
// parse, the plugin mirror, and the stray-binary report — plus -dev's destination and a
// source-level guard that the -ldflags -X path in run() still names a variable that actually exists
// at internal/buildinfo, and a go.mod guard that lymMainPackage still names lym's main package.
// Tests spawn no git, go build, or go env;
// the mirror and stray-binary tests work in t.TempDir, so goBinDir(), the pushed-to-main gate and the git-driven listing in syncPlugins are intentionally left uncovered here.

package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fslink"
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

// TestOtherLymBinaries_ReportsEachLymCopyOutsideTheDestDirOnce verifies a second lym on PATH is reported whether it sits before or after the production dir, while the production binary, a foreign program of the same name and a second PATH entry reaching an already seen file through a directory link never are.
// The test binary stands in for lym, recognised by its own main package path.
func TestOtherLymBinaries_ReportsEachLymCopyOutsideTheDestDirOnce(t *testing.T) {
	t.Parallel()
	self, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable() error: %v", err)
	}
	selfInfo, ok := debug.ReadBuildInfo()
	if !ok {
		t.Fatal("debug.ReadBuildInfo() reports no build info for the test binary")
	}
	content, err := os.ReadFile(self)
	if err != nil {
		t.Fatalf("read test binary: %v", err)
	}

	name := binaryName()
	base := t.TempDir()
	dir := func(rel string) string { return filepath.Join(base, rel) }
	write := func(rel string, data []byte) {
		if err := os.MkdirAll(dir(rel), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir(rel), name), data, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	link := func(rel, target string) {
		if err := fslink.CreateDirLink(dir(rel), dir(target)); err != nil {
			t.Fatal(err)
		}
	}
	write("go/bin", content)
	write("a", content)
	write("c", content)
	write("foreign", []byte("#!/bin/sh\necho LyX document processor\n"))
	link("alias-a", "a")
	link("alias-dest", "go/bin")
	pathDirs := []string{dir("a"), dir("alias-dest"), dir("go/bin"), dir("foreign"), dir("alias-a"), "", dir("b"), dir("c"), dir("go/bin") + "/"}

	tests := []struct {
		name        string
		mainPackage string
		want        []string
	}{
		{"LymCopies", selfInfo.Path, []string{filepath.Join(dir("a"), name), filepath.Join(dir("c"), name)}},
		{"OtherMainPackage", "example.com/other/cmd/lym", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := otherLymBinaries(pathDirs, dir("go/bin"), name, tt.mainPackage)
			if strings.Join(got, "|") != strings.Join(tt.want, "|") {
				t.Errorf("otherLymBinaries(%q) = %v; want %v", tt.mainPackage, got, tt.want)
			}
		})
	}
}

// repoRoot locates the module root the way run() does, through devbin.RepoRoot.
func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := devbin.RepoRoot()
	if err != nil {
		t.Fatalf("devbin.RepoRoot() error: %v", err)
	}
	return root
}

// TestLdflagsPath_MatchesBuildinfoChannel guards the -X argument tools/deploy/main.go's run() passes
// to the linker against silent drift: Go's linker does not error on an unmatched -X, so a rename of
// either the variable or its package leaves a -dev build behaving as production, with no build
// error, no test failure, and no visible symptom until someone notices stencils refreshing when
// they should not.
func TestLdflagsPath_MatchesBuildinfoChannel(t *testing.T) {
	root := repoRoot(t)

	wantLdflags := []string{
		"-X github.com/Knatte18/loomyard/internal/buildinfo.Channel=dev",
		"-X github.com/Knatte18/loomyard/internal/buildinfo.Channel=production",
	}

	mainSrc, err := os.ReadFile(filepath.Join(root, "tools", "deploy", "main.go"))
	if err != nil {
		t.Fatalf("read tools/deploy/main.go: %v", err)
	}
	for _, wantLdflag := range wantLdflags {
		if !strings.Contains(string(mainSrc), wantLdflag) {
			t.Errorf("tools/deploy/main.go does not contain %q; the linker silently ignores an "+
				"unmatched -X, so a rename of internal/buildinfo.Channel or its package would leave "+
				"a build unstamped with no build error, no test failure, and no "+
				"visible symptom until someone notices stencils refreshing when they should not", wantLdflag)
		}
	}
	wantLdflag := wantLdflags[0]

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

// TestLymMainPackage_MatchesModulePath guards lymMainPackage against drift from go.mod's module path:
// a stale constant matches no installed lym, so the foreign-lym warning would never fire with every other test green.
func TestLymMainPackage_MatchesModulePath(t *testing.T) {
	t.Parallel()
	goMod, err := os.ReadFile(filepath.Join(repoRoot(t), "go.mod"))
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}
	var modulePath string
	for _, line := range strings.Split(string(goMod), "\n") {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(line), "module "); ok {
			modulePath = strings.TrimSpace(rest)
			break
		}
	}
	if modulePath == "" {
		t.Fatal("go.mod declares no module path")
	}
	if want := modulePath + "/cmd/lym"; lymMainPackage != want {
		t.Errorf("lymMainPackage = %q; want %q", lymMainPackage, want)
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
