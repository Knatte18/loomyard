//go:build tier1

// dotnetfixture_integration_test.go drives `lym gate test` over a real .NET solution under the dotnet scoper, with the real `dotnet` for the shared build and the test run.

package gatecli

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/testkit/plankit"
	"github.com/Knatte18/loomyard/internal/verifyrun"
)

// dotnetPackageReferences are the test packages the fixture's test project references, at the versions the offline NuGet global-packages folder is expected to hold.
const dotnetPackageReferences = `    <PackageReference Include="Microsoft.NET.Test.Sdk" Version="17.14.1" />
    <PackageReference Include="NUnit" Version="4.3.2" />
    <PackageReference Include="NUnit3TestAdapter" Version="5.0.0" />
`

// TestGateTest_DotnetFixture runs `lym gate test --base` over a committed two-project .NET solution after an edit to one production type, and asserts the call builds the solution once, runs only the test class that names the type and exits 0.
// A call that waited on an MSBuild node or compiler server holding the build's output pipe would end by its timeout instead.
// It skips when `dotnet` is not on PATH, has no SDK, or when the global-packages folder lacks the test packages, so a machine without .NET stays green.
//
// The build leaves an MSBuild node, and perhaps a compiler server, running until the SDK's idle timeout.
// That is process-global state that outlives the test: `dotnet build-server shutdown` would also end the nodes of other gate runs, so the test leaves them to the idle timeout.
func TestGateTest_DotnetFixture(t *testing.T) {
	t.Parallel()

	dotnet, err := exec.LookPath("dotnet")
	if err != nil {
		t.Skip("dotnet is not on PATH")
	}
	repo, _ := writeDotnetFixture(t, dotnet)
	base := gitkit.RevParse(t, repo, "HEAD")

	circle := filepath.Join(repo, "Circles", "Circle.cs")
	if err := os.WriteFile(circle, []byte("namespace Circles;\n\npublic class Circle\n{\n    public double Radius = 2;\n\n    public double Area() => 3.14 * Radius * Radius;\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	g := startGate(t, gateRun{cwd: repo, args: []string{"test", "--wait", "--base", base}, env: dotnetEnv()})
	if code := g.wait(t); code != 0 {
		t.Fatalf("gate = exit %d, stdout %q; want 0, not a timeout on an MSBuild node holding the output", code, g.stdout.String())
	}

	build, _ := buildRunRecordOf(t, g.stdout.String(), 0)
	if !build.Build || build.ScopeForm != "build" || build.ExitCode != 0 {
		t.Errorf("build run record = %+v; want a passed build", build)
	}
	run, _ := runRecordOf(t, g.stdout.String(), 0)
	if run.ScopeForm != "base" || run.ExitCode != 0 || run.Discovered != 2 || run.Passed != 2 || run.Failed != 0 {
		t.Errorf("tier0 run record = %+v; want the base scope and CircleTests' two passing tests alone", run)
	}
	for _, want := range []string{"Fixture.Tests/Fixture.Tests.csproj", "--filter", "CircleTests"} {
		if !strings.Contains(run.Command, want) {
			t.Errorf("tier0 command = %q; want it to hold %q", run.Command, want)
		}
	}
	if strings.Contains(run.Command, "SquareTests") {
		t.Errorf("tier0 command = %q; want SquareTests left out of the filter", run.Command)
	}
}

// dotnetEnv returns the entries that keep the SDK's telemetry, banner and first-run setup out of a gate run.
func dotnetEnv() []string {
	return []string{"DOTNET_CLI_TELEMETRY_OPTOUT=1", "DOTNET_NOLOGO=1", "DOTNET_SKIP_FIRST_TIME_EXPERIENCE=1"}
}

// writeDotnetFixture commits a .NET solution in a fresh git repository outside every hub and returns its root and the NuGet global-packages folder its restores read.
// The solution holds two production projects, Circles with a Circle type and Squares with a Square type, and one NUnit test project referencing both, with a test class naming each type, so MSBuild builds two projects in parallel and starts an out-of-process node.
// The worktree's own gate.yaml selects the dotnet scoper, a shared build of the solution and a vstest tier that runs the test project without building.
// It skips the test when dotnet has no SDK or the test packages are not in the global-packages folder.
func writeDotnetFixture(t *testing.T, dotnet string) (repo, packages string) {
	t.Helper()

	versionOut, err := runDotnetProbe(dotnet, t.TempDir(), "--version")
	if err != nil {
		t.Skipf("dotnet --version failed, so no .NET SDK is installed: %v\n%s", err, versionOut)
	}
	major, _, found := strings.Cut(strings.TrimSpace(string(versionOut)), ".")
	if !found || major == "" {
		t.Fatalf("dotnet --version printed %q; want a dotted SDK version", versionOut)
	}
	framework := "net" + major + ".0"

	listing, err := runDotnetProbe(dotnet, t.TempDir(), "nuget", "locals", "global-packages", "--list")
	if err != nil {
		t.Skipf("dotnet nuget locals failed: %v\n%s", err, listing)
	}
	_, packages, found = strings.Cut(strings.TrimSpace(string(listing)), "global-packages:")
	if packages = strings.TrimSpace(packages); !found || packages == "" {
		t.Fatalf("dotnet nuget locals printed %q; want the global-packages folder", listing)
	}

	testProject := `<Project Sdk="Microsoft.NET.Sdk">
  <PropertyGroup>
    <TargetFramework>` + framework + `</TargetFramework>
    <IsPackable>false</IsPackable>
  </PropertyGroup>
  <ItemGroup>
` + dotnetPackageReferences + `  </ItemGroup>
  <ItemGroup>
    <ProjectReference Include="..\Circles\Circles.csproj" />
    <ProjectReference Include="..\Squares\Squares.csproj" />
  </ItemGroup>
</Project>
`
	// The probe restores a copy of the test project's package references outside the fixture, so a missing package skips the test and leaves no obj/ in the tree the gate diffs.
	probeDir := t.TempDir()
	probeProject := `<Project Sdk="Microsoft.NET.Sdk">
  <PropertyGroup>
    <TargetFramework>` + framework + `</TargetFramework>
  </PropertyGroup>
  <ItemGroup>
` + dotnetPackageReferences + `  </ItemGroup>
</Project>
`
	if err := os.WriteFile(filepath.Join(probeDir, "Probe.csproj"), []byte(probeProject), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := runDotnetProbe(dotnet, probeDir, "restore", "Probe.csproj", "--source", packages); err != nil {
		t.Skipf("the global-packages folder %s lacks the test packages: %v\n%s", packages, err, out)
	}

	library := func(typeName, member string) string {
		return "namespace " + typeName + "s;\n\npublic class " + typeName + "\n{\n    public double " + member + " = 1;\n\n    public double Area() => 3.14 * " + member + " * " + member + ";\n}\n"
	}
	libraryProject := `<Project Sdk="Microsoft.NET.Sdk">
  <PropertyGroup>
    <TargetFramework>` + framework + `</TargetFramework>
  </PropertyGroup>
</Project>
`
	tests := func(typeName string) string {
		return "using NUnit.Framework;\nusing " + typeName + "s;\n\nnamespace Fixture.Tests;\n\n[TestFixture]\npublic class " + typeName + "Tests\n{\n    [Test]\n    public void AreaIsPositive() { Assert.That(new " + typeName + "().Area(), Is.GreaterThan(0)); }\n\n    [Test]\n    public void AreaGrowsWithSize() { Assert.That(new " + typeName + "().Area(), Is.LessThan(100)); }\n}\n"
	}

	quotedPackages, err := verifyrun.QuoteArg(runtime.GOOS, packages)
	if err != nil {
		t.Fatalf("quote %s for the shell: %v", packages, err)
	}
	gateYAML, err := yaml.Marshal(map[string]any{
		"slots":  1,
		"scoper": "dotnet",
		"build":  map[string]any{"command": "dotnet build Fixture.sln -m --source " + quotedPackages, "timeout_min": 5},
		"tiers": map[string]any{
			"tier0": map[string]any{
				"command":      "dotnet test {targets} --no-build {filter}",
				"all_targets":  "Fixture.Tests/Fixture.Tests.csproj",
				"counter":      "vstest",
				"shared_build": true,
				"timeout_min":  5,
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	repo = plankit.Repo(t, map[string]string{
		".gitignore":                         "bin/\nobj/\n.lym/\n",
		"_lym/config/gate.yaml":              string(gateYAML),
		"Fixture.sln":                        dotnetSolution,
		"Circles/Circles.csproj":             libraryProject,
		"Circles/Circle.cs":                  library("Circle", "Radius"),
		"Squares/Squares.csproj":             libraryProject,
		"Squares/Square.cs":                  library("Square", "Side"),
		"Fixture.Tests/Fixture.Tests.csproj": testProject,
		"Fixture.Tests/CircleTests.cs":       tests("Circle"),
		"Fixture.Tests/SquareTests.cs":       tests("Square"),
	})
	gitkit.Git(t, repo, "init", "-q")
	gitkit.Git(t, repo, "add", "-A")
	gitkit.Git(t, repo, "commit", "-q", "-m", "the .NET fixture")
	return repo, packages
}

// dotnetSolution is the classic solution file of the fixture's three projects, every configuration building every project.
const dotnetSolution = `Microsoft Visual Studio Solution File, Format Version 12.00
# Visual Studio Version 17
Project("{9A19103F-16F7-4668-BE54-9A1E7A4F7556}") = "Circles", "Circles\Circles.csproj", "{11111111-1111-1111-1111-111111111111}"
EndProject
Project("{9A19103F-16F7-4668-BE54-9A1E7A4F7556}") = "Squares", "Squares\Squares.csproj", "{22222222-2222-2222-2222-222222222222}"
EndProject
Project("{9A19103F-16F7-4668-BE54-9A1E7A4F7556}") = "Fixture.Tests", "Fixture.Tests\Fixture.Tests.csproj", "{33333333-3333-3333-3333-333333333333}"
EndProject
Global
	GlobalSection(SolutionConfigurationPlatforms) = preSolution
		Debug|Any CPU = Debug|Any CPU
		Release|Any CPU = Release|Any CPU
	EndGlobalSection
	GlobalSection(ProjectConfigurationPlatforms) = postSolution
		{11111111-1111-1111-1111-111111111111}.Debug|Any CPU.ActiveCfg = Debug|Any CPU
		{11111111-1111-1111-1111-111111111111}.Debug|Any CPU.Build.0 = Debug|Any CPU
		{11111111-1111-1111-1111-111111111111}.Release|Any CPU.ActiveCfg = Release|Any CPU
		{11111111-1111-1111-1111-111111111111}.Release|Any CPU.Build.0 = Release|Any CPU
		{22222222-2222-2222-2222-222222222222}.Debug|Any CPU.ActiveCfg = Debug|Any CPU
		{22222222-2222-2222-2222-222222222222}.Debug|Any CPU.Build.0 = Debug|Any CPU
		{22222222-2222-2222-2222-222222222222}.Release|Any CPU.ActiveCfg = Release|Any CPU
		{22222222-2222-2222-2222-222222222222}.Release|Any CPU.Build.0 = Release|Any CPU
		{33333333-3333-3333-3333-333333333333}.Debug|Any CPU.ActiveCfg = Debug|Any CPU
		{33333333-3333-3333-3333-333333333333}.Debug|Any CPU.Build.0 = Debug|Any CPU
		{33333333-3333-3333-3333-333333333333}.Release|Any CPU.ActiveCfg = Release|Any CPU
		{33333333-3333-3333-3333-333333333333}.Release|Any CPU.Build.0 = Release|Any CPU
	EndGlobalSection
EndGlobal
`

// runDotnetProbe runs dotnet with args in dir, with telemetry, the banner and the first-run setup off, and returns its combined output.
func runDotnetProbe(dotnet, dir string, args ...string) ([]byte, error) {
	cmd := exec.Command(dotnet, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), dotnetEnv()...)
	return cmd.CombinedOutput()
}
