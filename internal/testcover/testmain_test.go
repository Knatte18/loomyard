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
	standInTimed       = "TestStandInTimed"
	standInSplit       = "TestStandInSplit"
	standInFail        = "TestStandInFail"
	standInSkip        = "TestStandInSkip"
	standInSkipProfile = "TestStandInSkipWithProfile"
	standInNoProfile   = "TestStandInNoProfile"
	standInHang        = "TestStandInHangs"
	standInHoldsPipe   = "TestStandInHoldsPipe"
	standInSleeps      = "TestStandInSleeps"
	standInHoards      = "TestStandInHoards"
	standInOutOfMemory = "TestStandInOutOfMemory"
	standInProfileLine = "m/p/x.go:3.21,5.2 1 1"
)

// standInHoardMegabytes is the memory standInHoards keeps before it passes.
// It stops there, so a memory cap that does not hold fails the test and never the machine.
const standInHoardMegabytes = 384

// standInChildPIDSuffix, appended to a stand-in's record path, names the file standInHoldsPipe records its child's pid into.
const standInChildPIDSuffix = ".child-pid"

// standInHangBuildEnv makes the stand-in `go test -c` never end.
const standInHangBuildEnv = "TESTCOVER_STAND_IN_HANG_BUILD"

// standInCompileErrorsEnv makes the stand-in `go test -c` fail with standInCompileErrors on stderr.
const standInCompileErrorsEnv = "TESTCOVER_STAND_IN_COMPILE_ERRORS"

// standInCompileErrors is what the stand-in build prints when the compiler refuses the package:
// three errors in three files, the last with no column.
const standInCompileErrors = "# m/p\n./overlay-1/x.go:4:9: cannot use v (value of type int) as time.Duration value\n/abs/y.go:12:3: mismatched types\n" +
	"/abs/overlay-3/z.go:62: invalid operation: digits[0] == lymMutantInt('0', 275) (mismatched types byte and rune)\n"

// standInChildSleep is how long the child standInHoldsPipe leaves behind holds the output pipe.
// The test's cleanup kills the child long before.
const standInChildSleep = 5 * time.Minute

// TestMain runs a stand-in when standInRecordEnv is set, and the tests under tmuxkit.Main otherwise.
func TestMain(m *testing.M) {
	if record := os.Getenv(standInRecordEnv); record != "" {
		os.Exit(runStandIn(record, os.Args[1:]))
	}
	os.Exit(tmuxkit.Main(m))
}

// standInListOutput is what the stand-in prints as `go list -deps -test`.
// It holds a package of another module, which the template prints as an empty line.
// It then holds m/q twice, m/p compiled for its test, its external test package and the generated test main.
const standInListOutput = "\nm/q\tq\t\nm/p [m/p.test]\tp\tm/p\nm/p_test [m/p.test]\tp_test\tm/p\nm/p.test\tmain\t\nm/q\tq\t\n"

// standInGoEnv is what the stand-in prints as `go env GOVERSION GOOS GOARCH`.
const standInGoEnv = "go1.26.8\nlinux\namd64\n"

// runStandIn writes args, one per line, to record and acts on them, returning the process exit code.
// As `go list`, it prints standInListOutput, and as `go env`, standInGoEnv.
// As `go test -list`, it prints two test names; as `go test -c`, it creates the empty file `-o` names.
// As `go test -c` with standInCompileErrorsEnv set, it prints standInCompileErrors and fails.
// As a coverage binary, it reports the outcome of the one test its `-test.run` names.
// It writes a profile for a passing test, and for a skipped one that standInSkipProfile names, when given a profile flag.
// The test standInTimed prints its `=== RUN` line and its result line in one write, and reports 0.00s.
// The test standInSplit writes its `=== RUN` line and an indented subtest result line.
// It then writes its result line in two writes, and reports 0.00s.
// Run with a pattern matching no test, it writes the profile with its block uncovered.
// The test standInHang never ends on its own.
// The test standInHoldsPipe passes and exits, leaving a child that holds the output pipe for standInChildSleep;
// it records the child's pid in the file standInChildPIDSuffix names.
// The test standInHoards keeps standInHoardMegabytes megabytes, writing each one, and then passes.
// The test standInOutOfMemory prints the Go runtime's out-of-memory line and exits 2 without allocating.
func runStandIn(record string, args []string) int {
	if err := os.WriteFile(record, []byte(strings.Join(args, "\n")), 0o644); err != nil {
		return 1
	}
	switch {
	case len(args) > 0 && args[0] == "list":
		fmt.Print(standInListOutput)
		return 0
	case len(args) > 0 && args[0] == "env":
		fmt.Print(standInGoEnv)
		return 0
	case slices.Contains(args, "-list"):
		fmt.Println("TestAlpha\nExampleBeta\nok  \tm/p\t0.01s")
		return 0
	case slices.Contains(args, "-c"):
		if os.Getenv(standInHangBuildEnv) != "" {
			time.Sleep(time.Hour)
		}
		if os.Getenv(standInCompileErrorsEnv) != "" {
			fmt.Fprint(os.Stderr, standInCompileErrors)
			return 1
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
	// writeProfile writes the profile line with count, unless the binary ran with no profile flag.
	writeProfile := func(count string) error {
		if profile == "" {
			return nil
		}
		return os.WriteFile(profile, []byte("mode: set\n"+strings.TrimSuffix(standInProfileLine, "1")+count+"\n"), 0o644)
	}
	switch name {
	case "":
		if err := writeProfile("0"); err != nil {
			return 1
		}
		fmt.Println("testing: warning: no tests to run\nPASS")
	case standInPass:
		if err := writeProfile("1"); err != nil {
			return 1
		}
		fmt.Printf("--- PASS: %s (0.25s)\n", name)
	case standInTimed:
		if err := writeProfile("1"); err != nil {
			return 1
		}
		fmt.Printf("=== RUN   %s\n--- PASS: %s (0.00s)\n", name, name)
	case standInSplit:
		if err := writeProfile("1"); err != nil {
			return 1
		}
		fmt.Printf("=== RUN   %s\n", name)
		fmt.Printf("    --- PASS: %s/sub (0.00s)\n", name)
		fmt.Print("--- PA")
		fmt.Printf("SS: %s (0.00s)\n", name)
	case standInSkipProfile:
		if err := writeProfile("1"); err != nil {
			return 1
		}
		fmt.Printf("--- SKIP: %s (0.00s)\n", name)
	case standInNoProfile:
		fmt.Printf("--- PASS: %s (0.25s)\n", name)
	case standInSkip:
		fmt.Printf("--- SKIP: %s (0.00s)\n", name)
	case standInHoldsPipe:
		if err := writeProfile("1"); err != nil {
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
	case standInHoards:
		kept := make([][]byte, 0, standInHoardMegabytes)
		for range standInHoardMegabytes {
			block := make([]byte, 1<<20)
			for at := range block {
				block[at] = 1
			}
			kept = append(kept, block)
		}
		if err := writeProfile("1"); err != nil {
			return 1
		}
		fmt.Printf("--- PASS: %s (0.25s)\n", name)
	case standInOutOfMemory:
		fmt.Println("fatal error: runtime: out of memory")
		return 2
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
