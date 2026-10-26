//go:build tier0

// proctree_linux_test.go pins the Linux wiring of the clock tick rate: the host's real auxiliary vector holds an AT_CLKTCK entry that clockTicksFromAuxv picks up.

package reedengine

import (
	"testing"

	"golang.org/x/sys/unix"
)

func TestClockTicksFromAuxvReadsHostVector(t *testing.T) {
	t.Parallel()
	auxv, err := unix.Auxv()
	if err != nil {
		t.Fatalf("unix.Auxv() error = %v; want nil", err)
	}
	var declared uintptr
	for _, entry := range auxv {
		if entry[0] == 17 {
			declared = entry[1]
		}
	}
	if declared == 0 {
		t.Fatalf("host auxiliary vector holds no positive entry for key 17: %v", auxv)
	}
	if got := clockTicksFromAuxv(auxv); got != int64(declared) {
		t.Errorf("clockTicksFromAuxv(host vector) = %d; want %d", got, declared)
	}
}
