//go:build tier1 && !windows

// dobbyreexec_unix_integration_test.go holds the dobby re-exec test, since selfexec never execs on Windows.

package beamcli_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/dobbyengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/testkit/lymbin"
)

func TestDobby_ReexecsOntoAReplacedBinary(t *testing.T) {
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	prime := h.Location
	geom, err := hubgeom.DobbyGeometry(prime, prime.WorktreeName)
	if err != nil {
		t.Fatalf("DobbyGeometry() = %v; want nil", err)
	}
	for _, dir := range []string{filepath.Dir(geom.LogPath), geom.PrimeRunsDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("MkdirAll(%s) = %v; want nil", dir, err)
		}
	}
	seeded := bytes.Repeat([]byte("seeded raw output\n"), 70_000)
	if err := os.WriteFile(geom.OutPath, seeded, 0o644); err != nil {
		t.Fatalf("WriteFile(dobby.out) = %v; want nil", err)
	}

	binary := filepath.Join(t.TempDir(), "lym")
	built := lymbin.Build(t)
	copyExecutable(t, built, binary)

	// An older spawner left the dobby's standard output and error on dobby.log.
	oldOut, err := os.OpenFile(geom.LogPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("OpenFile(dobby.log) = %v; want nil", err)
	}
	defer func() { _ = oldOut.Close() }()
	cmd := startRetryingBusy(t, func() *exec.Cmd {
		cmd := exec.Command(binary, "beam", "dobby", h.PrimeWorktree())
		cmd.Dir = h.Path
		cmd.Stdout, cmd.Stderr = oldOut, oldOut
		proc.Detach(cmd)
		return cmd
	})
	exited := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(exited)
	}()
	t.Cleanup(func() {
		select {
		case <-exited:
		default:
			_ = cmd.Process.Kill()
			<-exited
		}
	})

	live := func() bool {
		alive, err := dobbyengine.Live(geom)
		return err == nil && alive
	}
	if !waitUntil(time.Minute, func() bool { _, err := os.Stat(geom.OutPath + ".1"); return live() && err == nil }) {
		t.Fatalf("the dobby did not come up and rotate its output; dobby.log:\n%s", fileText(geom.LogPath))
	}
	if got, _ := os.ReadFile(geom.OutPath + ".1"); !bytes.Equal(got, seeded) {
		t.Errorf("dobby.out.1 holds %d bytes; want the %d seeded bytes", len(got), len(seeded))
	}
	if info, err := os.Stat(geom.OutPath); err != nil || info.Size() >= 1<<20 {
		t.Errorf("Stat(dobby.out) = %v, %v; want a file under the bound", info, err)
	}

	tidy := &dobbyengine.TidyState{StrandGUID: "strand-kept", ReportPath: filepath.Join(geom.TidyDir, "report.md"), Deadline: time.Now().Add(24 * time.Hour)}
	if err := dobbyengine.UpdateState(geom.StatePath, func(s *dobbyengine.State) error { s.Tidy = tidy; return nil }); err != nil {
		t.Fatalf("UpdateState() = %v; want nil", err)
	}

	// Liveness probes take the lock for an instant, which the re-exec's wait for the lock must outlast.
	stopProbing := make(chan struct{})
	probed := make(chan struct{})
	go func() {
		defer close(probed)
		for {
			select {
			case <-stopProbing:
				return
			case <-time.After(20 * time.Millisecond):
				_ = live()
			}
		}
	}()
	defer func() {
		close(stopProbing)
		<-probed
	}()

	replacement := binary + ".new"
	copyExecutable(t, built, replacement)
	if err := os.Rename(replacement, binary); err != nil {
		t.Fatalf("Rename() = %v; want nil", err)
	}

	tick := 0
	reexeced := waitUntil(reexecDeadline, func() bool {
		tick++
		if err := os.WriteFile(filepath.Join(geom.PrimeRunsDir, "wake"), []byte{byte(tick)}, 0o644); err != nil {
			t.Fatalf("WriteFile(wake) = %v; want nil", err)
		}
		text := fileText(geom.LogPath)
		time.Sleep(time.Second)
		return strings.Contains(text, "re-exec onto the changed lym binary") && strings.Contains(text, "start re-exec")
	})
	if !reexeced {
		t.Fatalf("the dobby did not re-exec within %v; dobby.log:\n%s", reexecDeadline, fileText(geom.LogPath))
	}

	record, found, err := dobbyengine.ReadState(geom.StatePath)
	if err != nil || !found || record.PID != cmd.Process.Pid || record.Tidy == nil || record.Tidy.StrandGUID != tidy.StrandGUID {
		t.Errorf("ReadState() = %+v, %v, %v; want the process's pid %d and the tidy kept", record, found, err, cmd.Process.Pid)
	}
	if !live() {
		t.Errorf("Live() = false; want the re-exec'd dobby live")
	}

	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("Signal(SIGTERM) = %v; want nil", err)
	}
	select {
	case <-exited:
	case <-time.After(30 * time.Second):
		t.Fatalf("the dobby did not stop on SIGTERM")
	}
	if got := fileText(geom.OutPath); !strings.Contains(got, `"stopped":true`) {
		t.Errorf("dobby.out = %q; want the stopped envelope", got)
	}
	for _, line := range strings.Split(fileText(geom.LogPath), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "{") {
			t.Errorf("dobby.log holds the JSON envelope %q; want none", line)
		}
	}
}
