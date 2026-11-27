// redfirstdotnet_test.go covers the dotnet path of `lym gate red-first` with its git read and tier runs stood in for.
// It pins the selection of the changed test methods, the base's checkout and the verdict.

package gatecli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/Knatte18/loomyard/internal/dotnetimpact"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// tierSummary returns the summary line a tier run prints to stderr, which a captured output holds.
func tierSummary(discovered, exitCode int) string {
	return gateslot.TierSummaryLine(gateslot.RunRecord{Tier: 0, ScopeForm: "--base main", Discovered: discovered, ExitCode: exitCode}, "/l/tier0.log") + "\n"
}

//testtiming:keep pins parameterized and fully qualified failure-name matching, another test's failure and an out-of-project compile error judging none, and the exact build-error text
func TestJudgeDotnetRedFirst_ReadsTheRunsExitCodeFailureReportAndCompileErrors(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	projectDir := filepath.Join(string(filepath.Separator)+"c", "base", "Calc.Tests")
	addsTwo := dotnetimpact.ChangedTest{File: "Calc.Tests/CalcTests.cs", Class: "Calc.Tests.CalcTests", Method: "AddsTwo"}
	scales := dotnetimpact.ChangedTest{File: "Calc.Tests/CalcTests.cs", Class: "Calc.Tests.CalcTests", Method: "Scales"}
	tests := []dotnetimpact.ChangedTest{addsTwo, scales}
	report := func(failureLines ...string) string {
		lines := append([]string{gateslot.FailureReportHeader, "tier0 (exit 1), log /l/tier0.log:"}, failureLines...)
		return tierSummary(2, 1) + strings.Join(lines, "\n") + "\ngate test: failed runs: tier0 (exit 1)\n"
	}
	compileError := func(file string) string {
		return file + "(3,9): error CS0117: 'Calc' does not contain a definition for 'Two' [" + filepath.Join(projectDir, "Calc.Tests.csproj") + "]\n"
	}

	rows := []struct {
		name     string
		exitCode int
		captured string
		want     dotnetVerdict
	}{
		{name: "ARunThatDiscoveredTestsAndPassedIsGreen", exitCode: 0, captured: tierSummary(2, 0) + "Passed!\n", want: dotnetVerdict{outcome: redFirstGreen}},
		{name: "ARunThatPassedWithNoSummaryLineIsGreen", exitCode: 0, captured: "Passed!\n", want: dotnetVerdict{outcome: redFirstGreen}},
		{
			name: "ARunThatPassedWithNoTestDiscoveredJudgedNoneAsNoTestRan", exitCode: 0, captured: tierSummary(0, 0),
			want: dotnetVerdict{outcome: redFirstUnjudged, noTestRan: true},
		},
		{
			name: "ATestTheReportNamesByItsMethodIsRed", exitCode: 1, captured: report("  AddsTwo:", "    Expected: 2"),
			want: dotnetVerdict{outcome: redFirstRed, red: []dotnetimpact.ChangedTest{addsTwo}},
		},
		{
			name: "AParameterizedTestTheReportNamesWithItsArgumentsIsRed", exitCode: 1, captured: report("  Scales(2):"),
			want: dotnetVerdict{outcome: redFirstRed, red: []dotnetimpact.ChangedTest{scales}},
		},
		{
			name: "ATestTheReportNamesByItsFullyQualifiedNameIsRed", exitCode: 1, captured: report("  Calc.Tests.CalcTests.AddsTwo:"),
			want: dotnetVerdict{outcome: redFirstRed, red: []dotnetimpact.ChangedTest{addsTwo}},
		},
		{name: "AFailureOfAnotherTestJudgedNone", exitCode: 1, captured: report("  AddsThree:"), want: dotnetVerdict{outcome: redFirstUnjudged}},
		{
			name: "ACompileErrorInAFileOfTheTestProjectMakesEveryTestRed", exitCode: 1, captured: compileError(filepath.Join(projectDir, "CalcTests.cs")),
			want: dotnetVerdict{outcome: redFirstRed, red: tests, buildError: "CalcTests.cs: CS0117: 'Calc' does not contain a definition for 'Two'"},
		},
		{
			name: "ACompileErrorOutsideTheTestProjectJudgedNoneAsNoTestRan", exitCode: 1,
			captured: compileError(filepath.Join(string(filepath.Separator)+"c", "base", "Calc", "Calc.cs")),
			want:     dotnetVerdict{outcome: redFirstUnjudged, noTestRan: true},
		},
		{
			name: "ARunWithNoSummaryLineReportAndCompileErrorJudgedNoneAsNoTestRan", exitCode: 1, captured: "restore failed\n",
			want: dotnetVerdict{outcome: redFirstUnjudged, noTestRan: true},
		},
		{
			name: "ARunThatTimedOutWithNoTestDiscoveredRanTests", exitCode: 1,
			captured: tierSummary(0, 1) + gateslot.TierTimeoutLine("tier0", 30*time.Minute) + "\n",
			want:     dotnetVerdict{outcome: redFirstUnjudged},
		},
		{
			name: "ARunThatCrashedItsRunnerWithNoTestDiscoveredRanTests", exitCode: 139,
			captured: tierSummary(0, 139) + gateslot.RunnerCrashLine("tier0 (exit 139, SIGSEGV)") + "\n",
			want:     dotnetVerdict{outcome: redFirstUnjudged},
		},
		{
			name: "ARunStoppedForMemoryPressureWithNoTestDiscoveredRanTests", exitCode: 1,
			captured: tierSummary(0, 1) + gateslot.MemoryPressureLine("tier0 (exit 1)") + "\n",
			want:     dotnetVerdict{outcome: redFirstUnjudged},
		},
		{
			name: "ARunThatDiscoveredTestsAndJudgedNoneRanTests", exitCode: 1, captured: tierSummary(3, 1),
			want: dotnetVerdict{outcome: redFirstUnjudged},
		},
	}
	for _, tt := range rows {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := judgeDotnetRedFirst(tt.exitCode, tt.captured, tests, projectDir)
			if diff := cmp.Diff(tt.want, got, cmp.AllowUnexported(dotnetVerdict{})); diff != "" {
				t.Errorf("judgeDotnetRedFirst(%d, ...) mismatch (-want +got):\n%s", tt.exitCode, diff)
			}
		})
	}
}

// The fixture worktree of the call test: a production project Calc and a test project Calc.Tests.
// The diff changes Calc.cs, adds the test AddsTwo to CalcTests.cs and changes the test project's data.json.
const (
	dotnetBaseTests = "namespace Calc.Tests;\n\npublic class CalcTests\n{\n    [Test]\n    public void Kept() { }\n}\n"
	dotnetHeadTests = "namespace Calc.Tests;\n\npublic class CalcTests\n{\n    [Test]\n    public void Kept() { }\n\n" +
		"    [Test]\n    public void AddsTwo() { Assert.That(Calc.Add(1, 1), Is.EqualTo(2)); }\n}\n"
	dotnetAbstractTests = "namespace Calc.Tests;\n\npublic abstract class CalcTests\n{\n    [Test]\n    public void AddsTwo() { }\n}\n"
	// dotnetMixedTests adds AddsTwo to a concrete class and Shared to an abstract one.
	dotnetMixedTests = dotnetHeadTests + "\npublic abstract class SharedTests\n{\n    [Test]\n    public void Shared() { }\n}\n"
	// dotnetOverloadTests adds Zeta and Alpha out of name order, and two overloads of Mid.
	dotnetOverloadTests = "namespace Calc.Tests;\n\npublic class CalcTests\n{\n    [Test]\n    public void Kept() { }\n\n" +
		"    [Test]\n    public void Zeta() { }\n\n    [Test]\n    public void Alpha() { }\n\n" +
		"    [TestCase(1)]\n    public void Mid(int x) { }\n\n    [TestCase(\"a\")]\n    public void Mid(string x) { }\n}\n"
	// The second test project of the two-project rows.
	otherBaseTests = "namespace Other.Tests;\n\npublic class OtherTests\n{\n    [Test]\n    public void Kept() { }\n}\n"
	otherHeadTests = "namespace Other.Tests;\n\npublic class OtherTests\n{\n    [Test]\n    public void Kept() { }\n\n" +
		"    [Test]\n    public void Mul() { Assert.That(Calc.Mul(2, 3), Is.EqualTo(6)); }\n}\n"
)

// tierRunWant is one tier run the call makes: the test project's path and the filter terms of its tests.
type tierRunWant struct {
	project string
	terms   []string
}

func TestDotnetRedFirstCallRun_RunsTheChangedTestMethodsInACheckoutOfTheBase(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	redReport := gateslot.FailureReportHeader + "\ntier0 (exit 1), log /l/tier0.log:\n  AddsTwo:\n"
	addsTwoRun := []tierRunWant{{"Calc.Tests/Calc.Tests.csproj", []string{"FullyQualifiedName~Calc.Tests.CalcTests.AddsTwo"}}}
	rows := []struct {
		name string
		// command is tier0's command; empty takes `dotnet test {targets} {filter}`.
		command string
		// headTests is CalcTests.cs in the working tree; empty takes dotnetHeadTests.
		headTests string
		// extraFiles are written into the working tree before the call.
		extraFiles map[string]string
		// extraBase are added to the base's tree, and extraChanged to the paths that differ from it.
		extraBase    map[string]string
		extraChanged []string
		// removeTestProject drops Calc.Tests.csproj from the working tree.
		removeTestProject bool
		// exportFails makes the export of the base's tree fail.
		exportFails  bool
		tierExit     int
		tierOutput   string
		wantExit     int
		wantContains []string
		// wantRuns are the tier runs the call makes, in order; none means the call runs no tier.
		wantRuns []tierRunWant
		// wantAbsent are checkout files that the tier runs must find missing.
		wantAbsent []string
	}{
		{
			name: "ATestThatFailsOnTheBaseExitsZeroNamingIt", tierExit: 1, tierOutput: redReport, wantRuns: addsTwoRun,
			wantExit: 0, wantContains: []string{`"ok":true`, `"red_tests":["Calc.Tests.CalcTests.AddsTwo"]`},
		},
		{
			name: "ATestThatPassesOnTheBaseExitsWithTheGreenCode", tierExit: 0, tierOutput: tierSummary(2, 0), wantRuns: addsTwoRun,
			wantExit: gateslot.RedFirstGreenExit, wantContains: []string{"does not show the bug", gateslot.NoBugFixedTitle},
		},
		{
			name: "ATestThatDoesNotBuildOnTheBaseExitsZeroNamingTheCompileError", tierExit: 1, wantRuns: addsTwoRun,
			tierOutput: "CHECKOUT/Calc.Tests/CalcTests.cs(9,55): error CS0117: 'Calc' does not contain a definition for 'Add'\n",
			wantExit:   0, wantContains: []string{`"red_tests":["Calc.Tests.CalcTests.AddsTwo"]`, `"build_errors"`, "CS0117"},
		},
		{
			name: "ARunThatRanTestsAndJudgedNoneExitsOne", tierExit: 1, tierOutput: tierSummary(3, 1), wantRuns: addsTwoRun,
			wantExit: 1, wantContains: []string{"judged none of"},
		},
		{
			name: "ARunThatFailedBeforeAnyTestRanCannotBeJudged", tierExit: 1, tierOutput: "restore failed\n", wantRuns: addsTwoRun,
			wantExit: gateslot.RedFirstUnjudgedExit, wantContains: []string{"before any test ran", "a red-first gate passes this exit"},
		},
		{
			name: "AFilterThatSelectedNoTestCannotBeJudged", tierExit: 0, tierOutput: tierSummary(0, 0), wantRuns: addsTwoRun,
			wantExit:     gateslot.RedFirstUnjudgedExit,
			wantContains: []string{"selected no test", "FullyQualifiedName~Calc.Tests.CalcTests.AddsTwo", "a red-first gate passes this exit"},
		},
		{
			name: "ADiffThatChangesNoTestMethodExitsWithTheNoTestCode", headTests: dotnetBaseTests,
			wantExit: gateslot.RedFirstNoTestExit, wantContains: []string{"adds or changes no test", gateslot.NoTestSeamTitle},
		},
		{
			name: "ATestOfAnAbstractClassCannotBeJudged", headTests: dotnetAbstractTests,
			wantExit: gateslot.RedFirstUnjudgedExit, wantContains: []string{"methods of abstract classes", "a red-first gate passes this exit"},
		},
		{
			name: "AbstractClassTestsMixedWithConcreteOnesRunOnlyTheConcreteOnes", headTests: dotnetMixedTests, tierExit: 1, tierOutput: redReport,
			wantRuns: addsTwoRun, wantExit: 0, wantContains: []string{`"red_tests":["Calc.Tests.CalcTests.AddsTwo"]`},
		},
		{
			name: "AKeyWithNoTierThatTakesTargetsCannotBeJudged", command: "dotnet test Calc.sln {filter}",
			wantExit: gateslot.RedFirstUnjudgedExit, wantContains: []string{"holds {targets}"},
		},
		{
			name: "ATestFileNoProjectHoldsCannotBeJudged", removeTestProject: true,
			wantExit: gateslot.RedFirstUnjudgedExit, wantContains: []string{"no .csproj holds Calc.Tests/CalcTests.cs"},
		},
		{
			name: "ATestFileBesideTwoProjectsCannotBeJudged", extraFiles: map[string]string{"Calc.Tests/Other.Tests.csproj": "<Project />\n"},
			wantExit: gateslot.RedFirstUnjudgedExit, wantContains: []string{"Calc.Tests holds more than one .csproj"},
		},
		{
			name: "TestsOfTwoProjectsRunOncePerProjectAndBothProjectsRedTestsAreNamed", tierExit: 1,
			tierOutput:   gateslot.FailureReportHeader + "\ntier0 (exit 1), log /l/tier0.log:\n  AddsTwo:\n  Mul:\n",
			extraFiles:   map[string]string{"Other.Tests/Other.Tests.csproj": "<Project />\n", "Other.Tests/OtherTests.cs": otherHeadTests},
			extraBase:    map[string]string{"Other.Tests/Other.Tests.csproj": "<Project />\n", "Other.Tests/OtherTests.cs": otherBaseTests},
			extraChanged: []string{"Other.Tests/OtherTests.cs"},
			wantRuns:     append(slices.Clone(addsTwoRun), tierRunWant{"Other.Tests/Other.Tests.csproj", []string{"FullyQualifiedName~Other.Tests.OtherTests.Mul"}}),
			wantExit:     0, wantContains: []string{`"red_tests":["Calc.Tests.CalcTests.AddsTwo","Other.Tests.OtherTests.Mul"]`},
		},
		{
			name: "TheFilterTermsAreSortedAndHoldOverloadsOfOneMethodOnce", headTests: dotnetOverloadTests, tierExit: 0, tierOutput: tierSummary(4, 0),
			wantRuns: []tierRunWant{{"Calc.Tests/Calc.Tests.csproj", []string{
				"FullyQualifiedName~Calc.Tests.CalcTests.Alpha", "FullyQualifiedName~Calc.Tests.CalcTests.Mid", "FullyQualifiedName~Calc.Tests.CalcTests.Zeta",
			}}},
			wantExit: gateslot.RedFirstGreenExit, wantContains: []string{"does not show the bug"},
		},
		{
			name: "AnExportOfTheBaseThatFailsExitsOneWithoutRunningATier", exportFails: true,
			wantExit: 1, wantContains: []string{"cannot check out the base"},
		},
		{
			name: "AChangedTestProjectFileTheWorkingTreeDeletedIsRemovedFromTheCheckout", tierExit: 0, tierOutput: tierSummary(2, 0), wantRuns: addsTwoRun,
			extraBase: map[string]string{"Calc.Tests/old.json": "old\n"}, extraChanged: []string{"Calc.Tests/old.json"},
			wantAbsent: []string{"Calc.Tests/old.json"},
			wantExit:   gateslot.RedFirstGreenExit, wantContains: []string{"does not show the bug"},
		},
	}
	for _, tt := range rows {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			headTests, command := tt.headTests, tt.command
			if headTests == "" {
				headTests = dotnetHeadTests
			}
			if command == "" {
				command = "dotnet test {targets} {filter}"
			}
			root := t.TempDir()
			head := map[string]string{
				"Calc/Calc.csproj":             "<Project />\n",
				"Calc/Calc.cs":                 "head calc\n",
				"Calc.Tests/Calc.Tests.csproj": "<Project />\n",
				"Calc.Tests/CalcTests.cs":      headTests,
				"Calc.Tests/data.json":         "head data\n",
			}
			for rel, text := range tt.extraFiles {
				head[rel] = text
			}
			if tt.removeTestProject {
				delete(head, "Calc.Tests/Calc.Tests.csproj")
			}
			for rel, text := range head {
				writeFixtureFile(t, root, rel, text)
			}
			base := memoryTree{
				"Calc/Calc.csproj":             []byte("<Project />\n"),
				"Calc/Calc.cs":                 []byte("base calc\n"),
				"Calc.Tests/Calc.Tests.csproj": []byte("<Project />\n"),
				"Calc.Tests/CalcTests.cs":      []byte(dotnetBaseTests),
				"Calc.Tests/data.json":         []byte("base data\n"),
			}
			for rel, text := range tt.extraBase {
				base[rel] = []byte(text)
			}
			var gotRuns []tierRunWant
			call := dotnetRedFirstCall{
				request: redFirstRequest{gate: "rf"},
				cfg:     gateslot.Config{Tiers: map[string]gateslot.Tier{"tier0": {Command: command, Dir: "."}}},
				tiers:   []int{0}, root: root, base: "abc123",
				changed:  append([]string{"Calc/Calc.cs", "Calc.Tests/CalcTests.cs", "Calc.Tests/data.json"}, tt.extraChanged...),
				baseTree: base,
				exportBase: func(dest string) error {
					if tt.exportFails {
						return errors.New("the object store is unreadable")
					}
					for rel, data := range base {
						writeFixtureFile(t, dest, rel, string(data))
					}
					return nil
				},
				runTiers: func(_ context.Context, out io.Writer, request testRequest) int {
					checkout := request.checkoutRoot
					if len(request.targets) != 1 {
						t.Errorf("tier run targets = %v; want one test project", request.targets)
						return 1
					}
					project, err := filepath.Rel(checkout, request.targets[0])
					if err != nil {
						t.Errorf("tier run target %s lies outside the checkout %s: %v", request.targets[0], checkout, err)
						return 1
					}
					gotRuns = append(gotRuns, tierRunWant{filepath.ToSlash(project), request.terms})
					if !request.noCarry || !slices.Equal(request.tiers, []int{0}) {
						t.Errorf("tier run request = noCarry %v, tiers %v; want no carry and tier0", request.noCarry, request.tiers)
					}
					for rel, want := range map[string]string{"Calc/Calc.cs": "base calc\n", "Calc.Tests/CalcTests.cs": headTests, "Calc.Tests/data.json": "head data\n"} {
						if got, err := os.ReadFile(filepath.Join(checkout, filepath.FromSlash(rel))); err != nil || string(got) != want {
							t.Errorf("checkout %s = %q, %v; want %q", rel, got, err, want)
						}
					}
					for _, rel := range tt.wantAbsent {
						if _, err := os.Stat(filepath.Join(checkout, filepath.FromSlash(rel))); !errors.Is(err, os.ErrNotExist) {
							t.Errorf("checkout %s: stat error = %v; want it absent, since the working tree deleted it", rel, err)
						}
					}
					_, _ = io.WriteString(out, strings.ReplaceAll(tt.tierOutput, "CHECKOUT", checkout))
					return tt.tierExit
				},
			}
			var out bytes.Buffer
			code := call.run(context.Background(), &out)
			if code != tt.wantExit {
				t.Errorf("run() = exit %d, output %q; want exit %d", code, out.String(), tt.wantExit)
			}
			if diff := cmp.Diff(tt.wantRuns, gotRuns, cmp.AllowUnexported(tierRunWant{})); diff != "" {
				t.Errorf("run() tier runs mismatch (-want +got):\n%s", diff)
			}
			for _, want := range tt.wantContains {
				if !strings.Contains(out.String(), want) {
					t.Errorf("run() output = %q; want it to contain %q", out.String(), want)
				}
			}
		})
	}
}

func TestNearestProject_ClimbsPastAMissingDirectoryAndRefusesAnUnreadableOne(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	rows := []struct {
		name string
		rel  string
		// windowsSkips is whether a Windows read of the path reports it missing, so the row cannot show an unreadable directory.
		windowsSkips bool
		want         string
		wantReason   string
	}{
		{name: "AMissingDirectoryIsClimbedPast", rel: "Calc.Tests/gone/Deep.cs", want: "Calc.Tests/Calc.Tests.csproj"},
		{name: "ADirectoryThatIsAFileNamesTheReadError", rel: "Calc.Tests/data.json/Deep.cs", windowsSkips: true, wantReason: "cannot read Calc.Tests/data.json"},
	}
	for _, tt := range rows {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if tt.windowsSkips && runtime.GOOS == "windows" {
				t.Skip("a Windows read of a path below a file reports it missing")
			}
			root := t.TempDir()
			writeFixtureFile(t, root, "Calc.Tests/Calc.Tests.csproj", "<Project />\n")
			writeFixtureFile(t, root, "Calc.Tests/data.json", "{}\n")
			got, reason := nearestProject(root, tt.rel)
			if got != tt.want || (tt.wantReason == "") != (reason == "") || !strings.Contains(reason, tt.wantReason) {
				t.Errorf("nearestProject(%q) = %q, %q; want project %q and a reason holding %q", tt.rel, got, reason, tt.want, tt.wantReason)
			}
		})
	}
}

// writeFixtureFile writes text to the slash path rel under root, creating its directory.
func writeFixtureFile(t *testing.T, root, rel, text string) {
	t.Helper()
	target := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}
