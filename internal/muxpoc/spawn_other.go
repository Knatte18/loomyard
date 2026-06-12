//go:build !windows

// spawn_other.go — non-Windows no-ops for the windowless/detached spawn helpers, so the
// package still compiles and the cross-platform logic (env hygiene, state, argv) stays
// testable off Windows. muxpoc itself is only useful on Windows (psmux + ConPTY).
package muxpoc

import "os/exec"

func applyHidden(cmd *exec.Cmd)   {}
func applyDetached(cmd *exec.Cmd) {}
