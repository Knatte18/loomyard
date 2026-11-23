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
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/testkit/plankit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestGateRedFirst_Dotnet runs `lym gate red-first` over the .NET fixture in a hub whose scoper is dotnet.
// The fix makes Circle's area exact, and each row adds a test of its own to CircleTests.
// It skips when `dotnet` is not on PATH, has no SDK, or the global-packages folder lacks the test packages.
//
// The build leaves an MSBuild node running until the SDK's idle timeout, as TestGateTest_DotnetFixture says.
func TestGateRedFirst_Dotnet(t *testing.T) {
	tiergate.Require(t, 1)
	// The rows run one after another: each rewrites the one working tree that the next row's diff starts from.
	t.Parallel()

	dotnet, err := exec.LookPath("dotnet")
	if err != nil {
		t.Skip("dotnet is not on PATH")
	}
	files, _ := dotnetFixtureFiles(t, dotnet)
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	gateYAML := files["_lym/config/gate.yaml"] + "gates:\n  rf: {tiers: [0], scope: diff}\n"
	if err := os.WriteFile(configengine.ConfigFile(h.BoardDir(), "gate"), []byte(gateYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	delete(files, "_lym/config/gate.yaml")
	delete(files, ".gitignore")
	prime := h.PrimeWorktree()
	plankit.WriteTree(t, prime, files)
	gitkit.Git(t, prime, "add", "Fixture.sln", "Circles", "Squares", "Fixture.Tests")
	gitkit.Git(t, prime, "commit", "-q", "-m", "the .NET fixture")
	base := gitkit.RevParse(t, prime, "HEAD")

	baseCircle, baseTests := files["Circles/Circle.cs"], files["Fixture.Tests/CircleTests.cs"]
	writeCircle := func(extraMember string) {
		t.Helper()
		source := strings.Replace(baseCircle, "3.14 * Radius", "System.Math.PI * Radius", 1)
		source = strings.TrimSuffix(source, "}\n") + extraMember + "}\n"
		if err := os.WriteFile(filepath.Join(prime, "Circles", "Circle.cs"), []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeTest := func(method string) {
		t.Helper()
		source := strings.TrimSuffix(baseTests, "}\n") + "\n    [Test]\n    " + method + "\n}\n"
		if err := os.WriteFile(filepath.Join(prime, "Fixture.Tests", "CircleTests.cs"), []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	run := func() (int, string) {
		t.Helper()
		var out bytes.Buffer
		code := clihelp.ExecuteIn(Command(), prime, &out, []string{"red-first", "--gate", "rf", "--base", base})
		lines := strings.Split(strings.TrimSpace(out.String()), "\n")
		last := lines[len(lines)-1]
		if !json.Valid([]byte(last)) {
			t.Fatalf("red-first printed %q; want the envelope on its last line", out.String())
		}
		return code, last
	}

	t.Run("ATestThatFailsOnTheBaseExitsZeroNamingIt", func(t *testing.T) {
		writeCircle("")
		writeTest("public void AreaIsPi() { Assert.That(new Circle().Area(), Is.EqualTo(System.Math.PI)); }")
		code, envelope := run()
		if code != 0 || !strings.Contains(envelope, `"red_tests":["Fixture.Tests.CircleTests.AreaIsPi"]`) {
			t.Errorf("red-first = exit %d, envelope %q; want exit 0 naming AreaIsPi as red", code, envelope)
		}
	})
	t.Run("ATestThatReadsAMemberTheChangeAddsIsRedSinceItDoesNotBuildOnTheBase", func(t *testing.T) {
		writeCircle("\n    public double Perimeter() => 2 * System.Math.PI * Radius;\n")
		writeTest("public void PerimeterIsTwoPi() { Assert.That(new Circle().Perimeter(), Is.EqualTo(2 * System.Math.PI)); }")
		code, envelope := run()
		if code != 0 || !strings.Contains(envelope, `"red_tests":["Fixture.Tests.CircleTests.PerimeterIsTwoPi"]`) || !strings.Contains(envelope, "Perimeter") || !strings.Contains(envelope, `"build_errors"`) {
			t.Errorf("red-first = exit %d, envelope %q; want exit 0 naming PerimeterIsTwoPi as red and the compile error that reads Perimeter", code, envelope)
		}
	})
	t.Run("ATestThatPassesOnTheBaseExitsWithTheGreenCode", func(t *testing.T) {
		writeCircle("")
		writeTest("public void RadiusIsOne() { Assert.That(new Circle().Radius, Is.EqualTo(1)); }")
		code, envelope := run()
		if code != gateslot.RedFirstGreenExit || !strings.Contains(envelope, "does not show the bug") {
			t.Errorf("red-first = exit %d, envelope %q; want exit %d saying the test does not show the bug", code, envelope, gateslot.RedFirstGreenExit)
		}
	})
}
