//go:build windows

package proc

import (
	"os/exec"
	"testing"
)

func TestHideWindow(t *testing.T) {
	cmd := exec.Command("cmd", "/c", "echo", "test")
	HideWindow(cmd)

	if cmd.SysProcAttr == nil {
		t.Fatal("cmd.SysProcAttr is nil")
	}
	if cmd.SysProcAttr.HideWindow != true {
		t.Errorf("HideWindow(cmd) HideWindow = %v; want true", cmd.SysProcAttr.HideWindow)
	}
	if cmd.SysProcAttr.CreationFlags != createNoWindow {
		t.Errorf("HideWindow(cmd) CreationFlags = %#x; want %#x", cmd.SysProcAttr.CreationFlags, createNoWindow)
	}
}

func TestDetach(t *testing.T) {
	cmd := exec.Command("cmd", "/c", "echo", "test")
	Detach(cmd)

	if cmd.SysProcAttr == nil {
		t.Fatal("cmd.SysProcAttr is nil")
	}
	if cmd.SysProcAttr.HideWindow != true {
		t.Errorf("Detach(cmd) HideWindow = %v; want true", cmd.SysProcAttr.HideWindow)
	}
	if cmd.SysProcAttr.CreationFlags != (createNoWindow | createNewProcessGroup) {
		t.Errorf("Detach(cmd) CreationFlags = %#x; want %#x", cmd.SysProcAttr.CreationFlags, createNoWindow|createNewProcessGroup)
	}
}

func TestDetachBreakaway(t *testing.T) {
	cmd := exec.Command("cmd", "/c", "echo", "test")
	DetachBreakaway(cmd)

	if cmd.SysProcAttr == nil {
		t.Fatal("cmd.SysProcAttr is nil")
	}
	if cmd.SysProcAttr.HideWindow != true {
		t.Errorf("DetachBreakaway(cmd) HideWindow = %v; want true", cmd.SysProcAttr.HideWindow)
	}
	want := createNoWindow | createNewProcessGroup | createBreakawayFromJob
	if cmd.SysProcAttr.CreationFlags != want {
		t.Errorf("DetachBreakaway(cmd) CreationFlags = %#x; want %#x", cmd.SysProcAttr.CreationFlags, want)
	}
}

func TestDetachBreakawayDoesNotAffectDetach(t *testing.T) {
	cmd := exec.Command("cmd", "/c", "echo", "test")
	Detach(cmd)

	if cmd.SysProcAttr == nil {
		t.Fatal("cmd.SysProcAttr is nil")
	}
	if cmd.SysProcAttr.CreationFlags != (createNoWindow | createNewProcessGroup) {
		t.Errorf("Detach(cmd) CreationFlags = %#x; want %#x", cmd.SysProcAttr.CreationFlags, createNoWindow|createNewProcessGroup)
	}
	if cmd.SysProcAttr.CreationFlags&createBreakawayFromJob != 0 {
		t.Errorf("Detach(cmd) set createBreakawayFromJob flag; should not")
	}
}

func TestHideWindowDoesNotSetDetachFlag(t *testing.T) {
	cmd := exec.Command("cmd", "/c", "echo", "test")
	HideWindow(cmd)

	if cmd.SysProcAttr.CreationFlags&createNewProcessGroup != 0 {
		t.Errorf("HideWindow(cmd) set createNewProcessGroup flag; should not")
	}
}

func TestDetachSetsHideWindow(t *testing.T) {
	cmd := exec.Command("cmd", "/c", "echo", "test")
	Detach(cmd)

	if cmd.SysProcAttr.HideWindow != true {
		t.Errorf("Detach(cmd) HideWindow = %v; want true", cmd.SysProcAttr.HideWindow)
	}
}
