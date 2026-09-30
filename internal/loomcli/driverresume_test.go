package loomcli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// fakeDriverSender records SendDriver calls and answers from errs in order; once errs is exhausted it
// answers nil, unless repeatErr is set, which it then answers forever.
type fakeDriverSender struct {
	texts     []string
	errs      []error
	repeatErr error
}

func (f *fakeDriverSender) SendDriver(guid, text string) error {
	f.texts = append(f.texts, text)
	n := len(f.texts) - 1
	if n < len(f.errs) {
		return f.errs[n]
	}
	return f.repeatErr
}

// newResumeTestReceiver builds a receiver with a park marker on disk and a counting wait.
func newResumeTestReceiver(t *testing.T, sender *fakeDriverSender) (*loomCLI, *int, string) {
	t.Helper()
	dir := t.TempDir()
	loc := &lyxcwd.Location{HubPath: dir, WorktreeName: "warp", AnchorRel: "."}
	waits := 0
	c := &loomCLI{
		location:         loc,
		runID:            "self",
		driverSender:     sender,
		driverResumeWait: func() { waits++ },
	}
	marker := shedrun.ParkMarker(loc, shedrun.ResolveRunID(loc, c.runID))
	if err := os.MkdirAll(filepath.Dir(marker), 0o755); err != nil {
		t.Fatalf("mkdir marker dir: %v", err)
	}
	if err := os.WriteFile(marker, nil, 0o644); err != nil {
		t.Fatalf("write marker: %v", err)
	}
	return c, &waits, marker
}

func markerExists(marker string) bool {
	_, err := os.Stat(marker)
	return err == nil
}

func notReady() error {
	return fmt.Errorf("pane: %w", shuttleengine.ErrPaneNotReady)
}

func TestResumeParkedDriver_SendsOneLineAndRemovesMarker(t *testing.T) {
	sender := &fakeDriverSender{}
	c, waits, marker := newResumeTestReceiver(t, sender)

	if err := c.resumeParkedDriver("g-drv"); err != nil {
		t.Fatalf("resumeParkedDriver() error = %v; want nil", err)
	}
	if len(sender.texts) != 1 {
		t.Fatalf("SendDriver calls = %d; want 1", len(sender.texts))
	}
	text := sender.texts[0]
	if strings.ContainsAny(text, "\r\n") {
		t.Errorf("resume text %q is not a single line", text)
	}
	reportsDir := shedrun.DriveReportsDir(c.location, shedrun.ResolveRunID(c.location, c.runID))
	if !strings.Contains(text, reportsDir) {
		t.Errorf("resume text %q names no path under %q", text, reportsDir)
	}
	if *waits != 0 {
		t.Errorf("waits = %d; want 0", *waits)
	}
	if markerExists(marker) {
		t.Error("park marker still on disk after a verified send")
	}
}

func TestResumeParkedDriver_WaitsThroughNotReadyPane(t *testing.T) {
	sender := &fakeDriverSender{errs: []error{notReady(), notReady()}}
	c, waits, marker := newResumeTestReceiver(t, sender)

	if err := c.resumeParkedDriver("g-drv"); err != nil {
		t.Fatalf("resumeParkedDriver() error = %v; want nil", err)
	}
	if len(sender.texts) != 3 || *waits != 2 {
		t.Errorf("calls = %d, waits = %d; want 3 and 2", len(sender.texts), *waits)
	}
	if markerExists(marker) {
		t.Error("park marker still on disk after a verified send")
	}
}

func TestResumeParkedDriver_NeverReadyExhaustsAttempts(t *testing.T) {
	sender := &fakeDriverSender{repeatErr: notReady()}
	c, _, marker := newResumeTestReceiver(t, sender)

	err := c.resumeParkedDriver("g-drv")
	if err == nil {
		t.Fatal("resumeParkedDriver() = nil; want a refusal")
	}
	if len(sender.texts) != driverResumeSendAttempts {
		t.Errorf("SendDriver calls = %d; want %d", len(sender.texts), driverResumeSendAttempts)
	}
	if !strings.Contains(err.Error(), "lyx loom start") || !strings.Contains(err.Error(), fmt.Sprint(driverResumeSendAttempts)) {
		t.Errorf("error %q does not name the retry and the attempt count", err)
	}
	if !markerExists(marker) {
		t.Error("park marker was removed despite the failed resume")
	}
}

func TestResumeParkedDriver_OtherErrorIsNeverRetried(t *testing.T) {
	sender := &fakeDriverSender{repeatErr: errors.New("text never appeared")}
	c, waits, marker := newResumeTestReceiver(t, sender)

	if err := c.resumeParkedDriver("g-drv"); err == nil {
		t.Fatal("resumeParkedDriver() = nil; want a refusal")
	}
	if len(sender.texts) != 1 || *waits != 0 {
		t.Errorf("calls = %d, waits = %d; want 1 and 0", len(sender.texts), *waits)
	}
	if !markerExists(marker) {
		t.Error("park marker was removed despite the failed resume")
	}
}
