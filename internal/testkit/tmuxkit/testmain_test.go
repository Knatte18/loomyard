package tmuxkit

import (
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/lymtemp"
)

func TestMain(m *testing.M) {
	// A package whose TestMain sets up git has made the process run directory before it calls Main.
	// This one makes it directly, so a refused re-exec has a directory to remove.
	if _, err := lymtemp.Dir(); err != nil {
		panic(err)
	}
	os.Exit(Main(m))
}
