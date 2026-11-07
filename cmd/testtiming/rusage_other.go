//go:build !linux

// rusage_other.go makes maxRSSBytes return zero on every platform but Linux.

package main

import "os"

// maxRSSBytes returns zero: this platform's max RSS is not read.
func maxRSSBytes(state *os.ProcessState) int64 {
	return 0
}
