//go:build !linux

// tie_other.go leaves a verify shell untied where the platform has no parent-death signal.

package verifyrun

import "os/exec"

// tieToParent does nothing: off Linux the caller's own kill-on-close job ends the shell's tree.
func tieToParent(cmd *exec.Cmd) {}
