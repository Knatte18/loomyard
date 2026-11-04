// record_linux_test.go pins the production zombie reading of the owner record against a real unreaped child.

package runproc

import (
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestListAndEnderReadAZombieLeader(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	dir := t.TempDir()

	truePath, err := exec.LookPath("true")
	if err != nil {
		t.Fatalf("LookPath(true): %v", err)
	}
	child, err := os.StartProcess(truePath, []string{"true"}, &os.ProcAttr{})
	if err != nil {
		t.Fatalf("StartProcess(true): %v", err)
	}
	t.Cleanup(func() { _, _ = child.Wait() })
	pid := child.Pid

	// Nothing waits on the child, so it stays a zombie once it exits.
	deadline := time.Now().Add(10 * time.Second)
	for proc.IsRunning(pid) {
		if time.Now().After(deadline) {
			t.Fatalf("pid %d still running after 10s; want it exited", pid)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !proc.IsAlive(pid) {
		t.Fatalf("IsAlive(%d) = false; want the unreaped child to still answer", pid)
	}

	if err := Register(dir, "slug", ChildRecord(pid, RoleLoop, []string{"true"})); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if listed, err := List(dir, "slug"); err != nil || len(listed) != 0 {
		t.Errorf("List(slug) with a zombie leader = %+v, %v; want nothing", listed, err)
	}
	if owners, err := Owners(dir); err != nil || len(owners) != 0 {
		t.Errorf("Owners with a zombie leader = %+v, %v; want none", owners, err)
	}
	kept, err := productionEnder().recordLister().list(dir, "slug")
	if err != nil || len(kept) != 1 || kept[0].PID != pid {
		t.Errorf("end lister with a zombie leader = %+v, %v; want its record kept, so the group signal reaches the members", kept, err)
	}
}
