// spawnbusy.go holds the test for a process start refused because the executable was open for writing.

package proc

import (
	"errors"
	"syscall"
)

// IsSpawnBusy reports whether err is a refusal to start a process because its executable was open for writing, ETXTBSY from exec, as while a deploy rewrites the binary in place.
// The refusal comes before the program runs, so a caller may start the same command again without running it twice.
func IsSpawnBusy(err error) bool {
	return errors.Is(err, syscall.ETXTBSY)
}
