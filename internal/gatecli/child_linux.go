//go:build linux

// child_linux.go gives the go test child a parent-death SIGKILL, so a SIGKILL of `lyx gate test` ends go itself.

package gatecli

import "syscall"

func init() {
	armParentDeath = func(attr *syscall.SysProcAttr) {
		attr.Pdeathsig = syscall.SIGKILL
	}
}
