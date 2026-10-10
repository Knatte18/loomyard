//go:build integration

// pool_integration_test.go pins the one behavior that needs a second process: a holder that dies without releasing frees its slot.

package gateslot

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestAcquire_HolderProcessDeathFreesSlot(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	child := exec.Command(os.Args[0])
	child.Env = append(os.Environ(), childSlotDirEnv+"="+dir)
	stdout, err := child.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = child.Process.Kill()
		_ = child.Wait()
	})
	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil || line != childHeldLine+"\n" {
		t.Fatalf("child output = %q, err = %v; want %q once it holds the slot", line, err, childHeldLine)
	}

	pool := &Pool{Dir: dir, Limits: singleSlotLimits, Poll: 10 * time.Millisecond}
	blocked, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := pool.Acquire(blocked, Holder{Site: "parent"}); err == nil {
		t.Fatal("Acquire succeeded while the child held the only slot")
	}

	if err := child.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = child.Wait()

	freed, cancelFreed := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelFreed()
	lease, err := pool.Acquire(freed, Holder{Site: "parent"})
	if err != nil {
		t.Fatalf("Acquire after the holder died = %v; want the slot free", err)
	}
	if err := lease.Release(); err != nil {
		t.Fatal(err)
	}
}
