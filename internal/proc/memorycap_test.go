// memorycap_test.go covers a memory-capped run on every platform.
// The child is the test binary itself, which the package's TestMain runs as a memory hog.

package proc_test

import (
	"os"
	"os/exec"
	"strconv"
	"testing"

	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// testMemoryCap is the cap the capped-run tests run a child under, in bytes.
const testMemoryCap = 128 << 20

// The megabytes a child keeps: within testMemoryCap, and over it.
// The larger child stops there by itself, so a cap that does not hold fails its test and never the machine.
const (
	hogWithinCap = 16
	hogOverCap   = 384
)

// memoryHog returns the command of a child that keeps megabytes megabytes and then exits 0.
func memoryHog(t *testing.T, megabytes int) *exec.Cmd {
	t.Helper()
	testBinary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(testBinary)
	cmd.Env = append(os.Environ(), memoryHogEnv+"="+strconv.Itoa(megabytes))
	return cmd
}

func TestRunMemoryCapped(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	tests := []struct {
		name      string
		megabytes int
		limit     int64
		// wantEnded is set for a child the cap ends before it exits 0.
		wantEnded bool
	}{
		{"AChildWithinTheCapEndsOnItsOwn", hogWithinCap, testMemoryCap, false},
		// A cap that is not a whole number of pages is still held, so a child within it runs.
		{"AChildWithinACapThatIsNoPageMultipleEndsOnItsOwn", hogWithinCap, testMemoryCap + 1, false},
		{"AChildOverTheCapIsEnded", hogOverCap, testMemoryCap, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			killed, err := proc.RunMemoryCapped(memoryHog(t, tt.megabytes), tt.limit)
			if ended := err != nil; ended != tt.wantEnded || (killed && !tt.wantEnded) {
				t.Errorf("RunMemoryCapped(a child keeping %d MB, %d) = killed %v, %v; want ended %v", tt.megabytes, tt.limit, killed, err, tt.wantEnded)
			}
		})
	}
}
