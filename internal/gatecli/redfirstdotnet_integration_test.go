// redfirstdotnet_integration_test.go drives `lym gate red-first`.
// The hub is a hubforge hub whose prime holds a small .NET solution.
// The shared build and the test runs use the real `dotnet`.
// It is Tier 1: it builds a real hub, spawns git and runs dotnet.

package gatecli

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/testkit/plankit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestGateRedFirst_Dotnet runs `lym gate red-first` over the .NET fixture in a hub whose scoper is dotnet.
// The fix makes Circle's area exact, and each row adds tests of its own to CircleTests.
// It skips when `dotnet` is not on PATH, has no SDK, or the global-packages folder lacks the test packages.
//
// Each call builds the solution cold in a fresh checkout of the base, and each row is slow.
// So every row has its own hub and runs in parallel.
// For the same reason, one row holds both a test that fails on the base and one that passes there.
//
// The build leaves an MSBuild node running until the SDK's idle timeout, as TestGateTest_DotnetFixture says.
//
//lym:only upkeep
func TestGateRedFirst_Dotnet(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	dotnet, err := exec.LookPath("dotnet")
	if err != nil {
		t.Skip("dotnet is not on PATH")
	}
	files, _ := dotnetFixtureFiles(t, dotnet)
	gateYAML := files["_lym/config/gate.yaml"] + "gates:\n  rf: {tiers: [0], scope: diff}\n"
	delete(files, "_lym/config/gate.yaml")
	delete(files, ".gitignore")
	baseCircle, baseTests := files["Circles/Circle.cs"], files["Fixture.Tests/CircleTests.cs"]

	tests := []struct {
		name string
		// extraMember is appended to Circle beside the fix.
		extraMember string
		// testMethods are added to CircleTests, each as a [Test] method.
		testMethods  []string
		wantContains []string
		// wantCounts, when set, are the test counts the tier run reports in the verb's output.
		wantCounts string
	}{
		{
			name: "OnlyTheTestThatFailsOnTheBaseIsNamedRed",
			testMethods: []string{
				"public void AreaIsPi() { Assert.That(new Circle().Area(), Is.EqualTo(System.Math.PI)); }",
				"public void RadiusIsOne() { Assert.That(new Circle().Radius, Is.EqualTo(1)); }",
			},
			wantContains: []string{
				`"red_tests":["Fixture.Tests.CircleTests.AreaIsPi"]`,
				`"tests":["Fixture.Tests.CircleTests.AreaIsPi","Fixture.Tests.CircleTests.RadiusIsOne"]`,
			},
			wantCounts: "discovered 2 passed 1 failed 1",
		},
		{
			name:         "ATestThatReadsAMemberTheChangeAddsIsRedSinceItDoesNotBuildOnTheBase",
			extraMember:  "\n    public double Perimeter() => 2 * System.Math.PI * Radius;\n",
			testMethods:  []string{"public void PerimeterIsTwoPi() { Assert.That(new Circle().Perimeter(), Is.EqualTo(2 * System.Math.PI)); }"},
			wantContains: []string{`"red_tests":["Fixture.Tests.CircleTests.PerimeterIsTwoPi"]`, `"build_errors"`, "Perimeter"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
			if err := os.WriteFile(configengine.ConfigFile(h.BoardDir(), "gate"), []byte(gateYAML), 0o644); err != nil {
				t.Fatal(err)
			}
			prime := h.PrimeWorktree()
			plankit.WriteTree(t, prime, files)
			gitkit.Git(t, prime, "add", "Fixture.sln", "Circles", "Squares", "Fixture.Tests")
			gitkit.Git(t, prime, "commit", "-q", "-m", "the .NET fixture")
			base := gitkit.RevParse(t, prime, "HEAD")

			circle := strings.Replace(baseCircle, "3.14 * Radius", "System.Math.PI * Radius", 1)
			circle = strings.TrimSuffix(circle, "}\n") + tt.extraMember + "}\n"
			circleTests := strings.TrimSuffix(baseTests, "}\n")
			for _, method := range tt.testMethods {
				circleTests += "\n    [Test]\n    " + method + "\n"
			}
			plankit.WriteTree(t, prime, map[string]string{"Circles/Circle.cs": circle, "Fixture.Tests/CircleTests.cs": circleTests + "}\n"})

			var out bytes.Buffer
			code := clihelp.ExecuteIn(Command(), prime, &out, []string{"red-first", "--gate", "rf", "--base", base})
			lines := strings.Split(strings.TrimSpace(out.String()), "\n")
			envelope := lines[len(lines)-1]
			if !json.Valid([]byte(envelope)) {
				t.Fatalf("red-first printed %q; want the envelope on its last line", out.String())
			}
			if code != 0 {
				t.Errorf("red-first = exit %d, envelope %q; want exit 0", code, envelope)
			}
			if !strings.Contains(out.String(), tt.wantCounts) {
				t.Errorf("red-first printed %q; want the tier run to report %q", out.String(), tt.wantCounts)
			}
			for _, want := range tt.wantContains {
				if !strings.Contains(envelope, want) {
					t.Errorf("red-first envelope = %q; want it to hold %q", envelope, want)
				}
			}
		})
	}
}
