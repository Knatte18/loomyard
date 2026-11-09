// testmain_test.go runs the package's tests under tmux isolation.
// It also runs a re-exec'd test binary as a stand-in `go` or a stand-in coverage binary when a runner test asks for one.

package testcover_test

import (
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

// standInRecordEnv names the file a re-exec'd test binary, acting as a stand-in, records its arguments into.
const standInRecordEnv = "TESTCOVER_STAND_IN_RECORD"

// The tests the stand-in coverage binary knows, named for the outcome each reports.
// standInProfileLine is the profile line it writes for a pass.
const (
	standInPass        = "TestStandInPass"
	standInFail        = "TestStandInFail"
	standInSkip        = "TestStandInSkip"
	standInSkipProfile = "TestStandInSkipWithProfile"
	standInNoProfile   = "TestStandInNoProfile"
	standInHang        = "TestStandInHangs"
	standInHoldsPipe   = "TestStandInHoldsPipe"
	standInSleeps      = "TestStandInSleeps"
	standInProfileLine = "m/p/x.go:3.21,5.2 1 1"
)

// standInChildPIDSuffix, appended to a stand-in's record path, names the file standInHoldsPipe records its child's pid into.
const standInChildPIDSuffix = ".child-pid"

// standInHangBuildEnv makes the stand-in `go test -c` never end.
const standInHangBuildEnv = "TESTCOVER_STAND_IN_HANG_BUILD"

// standInChildSleep is how long the child standInHoldsPipe leaves behind holds the output pipe.
const standInChildSleep = 5 * time.Second

// TestMain runs a stand-in when standInRecordEnv is set, and the tests under tmuxkit.Main otherwise.
func TestMain(m *testing.M) {
	if record := os.Getenv(standInRecordEnv); record != "" {
		os.Exit(runStandIn(record, os.Args[1:]))
	}
	os.Exit(tmuxkit.Main(m))
}

// runStandIn writes args, one per line, to record and acts on them, returning the process exit code.
// As `go test -list`, it prints two test names; as `go test -c`, it creates the empty file `-o` names.
// As a coverage binary, it reports the outcome of the one test its `-test.run` names.
// It writes a profile for a passing test, and for a skipped one that standInSkipProfile names.
// The test standInHang never ends on its own.
// The test standInHoldsPipe passes and exits, leaving a child that holds the output pipe for standInChildSleep;
// it records the child's pid in the file standInChildPIDSuffix names.
func runStandIn(record string, args []string) int {
	if err := os.WriteFile(record, []byte(strings.Join(args, "\n")), 0o644); err != nil {
		return 1
	}
	switch {
	case slices.Contains(args, "-list"):
		fmt.Println("TestAlpha\nExampleBeta\nok  \tm/p\t0.01s")
		return 0
	case slices.Contains(args, "-c"):
		if os.Getenv(standInHangBuildEnv) != "" {
			time.Sleep(time.Hour)
		}
		if i := slices.Index(args, "-o"); i >= 0 && i+1 < len(args) {
			if err := os.WriteFile(args[i+1], nil, 0o755); err != nil {
				return 1
			}
		}
		return 0
	}
	var name, profile string
	for _, arg := range args {
		if pattern, ok := strings.CutPrefix(arg, "-test.run="); ok {
			name = strings.TrimSuffix(strings.TrimPrefix(pattern, "^"), "$")
		}
		if path, ok := strings.CutPrefix(arg, "-test.coverprofile="); ok {
			profile = path
		}
	}
	switch name {
	case standInPass:
		if err := os.WriteFile(profile, []byte("mode: set\n"+standInProfileLine+"\n"), 0o644); err != nil {
			return 1
		}
		fmt.Printf("--- PASS: %s (0.25s)\n", name)
	case standInSkipProfile:
		if err := os.WriteFile(profile, []byte("mode: set\n"+standInProfileLine+"\n"), 0o644); err != nil {
			return 1
		}
		fmt.Printf("--- SKIP: %s (0.00s)\n", name)
	case standInNoProfile:
		fmt.Printf("--- PASS: %s (0.25s)\n", name)
	case standInSkip:
		fmt.Printf("--- SKIP: %s (0.00s)\n", name)
	case standInHoldsPipe:
		if err := os.WriteFile(profile, []byte("mode: set\n"+standInProfileLine+"\n"), 0o644); err != nil {
			return 1
		}
		fmt.Printf("--- PASS: %s (0.25s)\n", name)
		child := exec.Command(os.Args[0], "-test.run=^"+standInSleeps+"$")
		child.Env = append(os.Environ(), standInRecordEnv+"="+record+".child")
		child.Stdout, child.Stderr = os.Stdout, os.Stderr
		if err := child.Start(); err != nil {
			return 1
		}
		if err := os.WriteFile(record+standInChildPIDSuffix, []byte(strconv.Itoa(child.Process.Pid)), 0o644); err != nil {
			return 1
		}
	case standInSleeps:
		time.Sleep(standInChildSleep)
	case standInHang:
		time.Sleep(time.Hour)
	default:
		fmt.Printf("--- FAIL: %s (0.10s)\n", name)
		return 1
	}
	return 0
}
