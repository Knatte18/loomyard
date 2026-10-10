//go:build !linux

// rusage_other.go stands in for the largest resident set where the platform's rusage does not report it in a known unit.

package main

import "os"

// maxRSSBytes returns zero: this platform's max RSS is not read.
func maxRSSBytes(state *os.ProcessState) int64 {
	return 0
}
