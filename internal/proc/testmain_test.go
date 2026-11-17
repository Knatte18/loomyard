// testmain_test.go runs the package's tests under tmuxkit.Main.
// Their temp files live in the process run directory and go with it.
// It is an external test package because tmuxkit imports proc.
// It also runs the test binary as a memory hog when a capped-run test asks for one.

package proc_test

import (
	"fmt"
	"os"
	"strconv"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

// memoryHogEnv makes the test binary a child that keeps the megabytes the variable names and then exits 0.
const memoryHogEnv = "PROC_TEST_MEMORY_HOG_MB"

// TestMain runs a memory hog when memoryHogEnv is set, and the tests under tmuxkit.Main otherwise.
func TestMain(m *testing.M) {
	if megabytes := os.Getenv(memoryHogEnv); megabytes != "" {
		os.Exit(hoardMemory(megabytes))
	}
	os.Exit(tmuxkit.Main(m))
}

// hoardMemory keeps megabytes megabytes, writing each one so the host counts it, and returns the process exit code.
// It stops there, so a child no cap ends exits 0.
func hoardMemory(megabytes string) int {
	count, err := strconv.Atoi(megabytes)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s=%q names no megabyte count\n", memoryHogEnv, megabytes)
		return 3
	}
	kept := make([][]byte, 0, count)
	for range count {
		block := make([]byte, 1<<20)
		for at := range block {
			block[at] = 1
		}
		kept = append(kept, block)
	}
	fmt.Println("kept", len(kept), "megabytes")
	return 0
}
