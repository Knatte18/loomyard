//go:build tier1 || integration

// csharpfixtures_integration_test.go compiles every C# fixture under a `testdata/` directory with `dotnet build`.
// Quarry's tree-sitter parse accepts invalid C#, so it cannot prove a fixture is valid.

package main

import (
	"fmt"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

// csharpFixtureSkipDirs are directories the fixture scan never enters: version control and lyx state hold no fixture.
var csharpFixtureSkipDirs = map[string]bool{
	".git":     true,
	"_lyx":     true,
	".lyx":     true,
	"_mill":    true,
	".scratch": true,
	".wiki":    true,
	"_raddle":  true,
}

// csharpFixtureProject is the throwaway class library one fixture directory compiles in.
// Its compile items are the directory's `.cs` files by absolute path, so the build writes nothing into the repository.
const csharpFixtureProject = `<Project Sdk="Microsoft.NET.Sdk">
  <PropertyGroup>
    <TargetFramework>%s</TargetFramework>
    <OutputType>Library</OutputType>
    <EnableDefaultCompileItems>false</EnableDefaultCompileItems>
  </PropertyGroup>
  <ItemGroup>
    <Compile Include="%s" />
  </ItemGroup>
</Project>
`

// TestCSharpFixtures_CompileWithDotnet builds each directory of `.cs` files under a `testdata/` directory as one C# project and fails on a compiler error.
// It skips when `dotnet` is not on PATH or has no SDK installed, so a machine without .NET stays green.
//
//lyx:guard
func TestCSharpFixtures_CompileWithDotnet(t *testing.T) {
	t.Parallel()

	dotnet, err := exec.LookPath("dotnet")
	if err != nil {
		t.Skip("dotnet is not on PATH")
	}

	fixtureDirs := csharpFixtureDirs(t, scankit.Root(t))
	scankit.RequireFloor(t, len(fixtureDirs), 1, "C# fixture compile")
	targetFramework := dotnetTargetFramework(t, dotnet)

	for _, dir := range slices.Sorted(maps.Keys(fixtureDirs)) {
		t.Run(dir, func(t *testing.T) {
			t.Parallel()
			project := filepath.Join(t.TempDir(), "Fixtures.csproj")
			include := strings.Join(fixtureDirs[dir], ";")
			if err := os.WriteFile(project, fmt.Appendf(nil, csharpFixtureProject, targetFramework, include), 0o644); err != nil {
				t.Fatalf("write %s: %v", project, err)
			}
			output, err := runDotnet(dotnet, filepath.Dir(project), "build", project, "--disable-build-servers", "-nologo", "-v:q")
			if err != nil {
				t.Errorf("dotnet build of the C# fixtures in %s failed: %v\n%s", dir, err, output)
			}
		})
	}
}

// csharpFixtureDirs returns the absolute paths of the `.cs` files under a `testdata/` directory of root, keyed by their slash-separated directory relative to root.
func csharpFixtureDirs(t *testing.T, root string) map[string][]string {
	t.Helper()
	dirs := map[string][]string{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if csharpFixtureSkipDirs[entry.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".cs" {
			return nil
		}
		rel, err := filepath.Rel(root, filepath.Dir(path))
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if slices.Contains(strings.Split(rel, "/"), "testdata") {
			dirs[rel] = append(dirs[rel], path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan %s for C# fixtures: %v", root, err)
	}
	return dirs
}

// dotnetTargetFramework returns the target framework moniker of the installed SDK's major version, such as `net10.0` for SDK 10.0.113.
// It skips the test when `dotnet --version` fails, which a host with the runtime but no SDK does.
func dotnetTargetFramework(t *testing.T, dotnet string) string {
	t.Helper()
	output, err := runDotnet(dotnet, t.TempDir(), "--version")
	if err != nil {
		t.Skipf("dotnet --version failed, so no .NET SDK is installed: %v\n%s", err, output)
	}
	version := strings.TrimSpace(string(output))
	major, _, found := strings.Cut(version, ".")
	if !found || major == "" {
		t.Fatalf("dotnet --version printed %q; want a dotted SDK version", version)
	}
	return "net" + major + ".0"
}

// runDotnet runs dotnet with args in dir, with telemetry, the banner and the first-run setup off, and returns its combined output.
func runDotnet(dotnet, dir string, args ...string) ([]byte, error) {
	cmd := exec.Command(dotnet, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"DOTNET_CLI_TELEMETRY_OPTOUT=1",
		"DOTNET_NOLOGO=1",
		"DOTNET_SKIP_FIRST_TIME_EXPERIENCE=1",
	)
	logger.Info("cmd/lyx test: spawning dotnet for the C# fixture compile", "args", args, "dir", dir)
	output, err := cmd.CombinedOutput()
	logger.Info("cmd/lyx test: dotnet for the C# fixture compile exited", "err", err)
	return output, err
}
