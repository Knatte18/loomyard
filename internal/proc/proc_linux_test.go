package proc

import (
	"os/exec"
	"testing"
)

func TestHideWindowIsNoop(t *testing.T) {
	cmd := exec.Command("true")
	HideWindow(cmd)

	if cmd.SysProcAttr != nil {
		t.Errorf("HideWindow(cmd) set SysProcAttr; want nil")
	}
}

func TestDetachSetsSetsid(t *testing.T) {
	cmd := exec.Command("true")
	Detach(cmd)

	if cmd.SysProcAttr == nil {
		t.Fatal("cmd.SysProcAttr is nil")
	}
	if cmd.SysProcAttr.Setsid != true {
		t.Errorf("Detach(cmd) Setsid = %v; want true", cmd.SysProcAttr.Setsid)
	}
}
