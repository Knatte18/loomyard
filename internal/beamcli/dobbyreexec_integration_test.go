//go:build tier1

// dobbyreexec_integration_test.go runs a real `lym beam dobby` through a re-exec onto a replaced binary and its stop, over a hubforge hub: the acceptance test of the dobby's lock, log and output file.

package beamcli_test

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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

// reexecDeadline bounds the wait for a re-exec: where the file watch does not open the loop ticks at most once a minute, and the re-exec needs two sightings of the replaced binary.
const reexecDeadline = 3 * time.Minute

// waitUntil polls cond every 50 ms until it holds or the deadline passes, and reports whether it held.
func waitUntil(deadline time.Duration, cond func() bool) bool {
	end := time.Now().Add(deadline)
	for time.Now().Before(end) {
		if cond() {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return cond()
}

// copyExecutable copies the file at src to dst as an executable.
func copyExecutable(t *testing.T, src, dst string) {
	t.Helper()
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("ReadFile(%s) = %v; want nil", src, err)
	}
	if err := os.WriteFile(dst, data, 0o755); err != nil {
		t.Fatalf("WriteFile(%s) = %v; want nil", dst, err)
	}
}

// startRetryingBusy starts the command newCmd builds, and builds and starts a fresh one while the start fails with ETXTBSY, for up to ten seconds.
// A process a parallel test forks can hold the just-written binary open for writing until it execs, which fails this exec with "text file busy".
func startRetryingBusy(t *testing.T, newCmd func() *exec.Cmd) *exec.Cmd {
	t.Helper()
	var cmd *exec.Cmd
	var err error
	waitUntil(10*time.Second, func() bool {
		cmd = newCmd()
		err = cmd.Start()
		return !errors.Is(err, syscall.ETXTBSY)
	})
	if err != nil {
		t.Fatalf("start %s = %v; want nil", cmd.Path, err)
	}
	return cmd
}

// fileText returns the text of the file at path, empty when it does not exist.
func fileText(path string) string {
	data, _ := os.ReadFile(path)
	return string(data)
}

func TestDobby_ReexecsOntoAReplacedBinary(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("selfexec never execs on Windows")
	}
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
