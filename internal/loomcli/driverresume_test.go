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

// fakeDriverSender records SendDriver calls and answers from errs in order;
// once errs is exhausted it answers nil, unless repeatErr is set, which it then answers forever.
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
	loc := &lyxcwd.Location{HubPath: dir, WorktreeName: "pair", AnchorRel: "."}
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

// TestResumeParkedDriver asserts the resume sends one single-line message naming the drive-reports directory and removes the park marker once the send is verified,
// waits through a not-ready pane, retries a never-ready pane only up to driverResumeSendAttempts, and never retries any other error;
// a refused resume leaves the marker on disk.
//
//testtiming:keep pins the resume send: one single-line message naming the drive-reports directory and the marker removed on success, waits through a not-ready pane, the attempt bound, and a non-readiness error never retried, with the marker kept on refusal; the covering tests assert the outcome of the resume, not each send
func TestResumeParkedDriver(t *testing.T) {
	tests := []struct {
		name      string
		sender    *fakeDriverSender
		wantTexts int
		wantWaits int
		// wantErr holds substrings of the refusal; empty means the resume succeeds.
		wantErr []string
	}{
		{name: "sends one line and removes the marker", sender: &fakeDriverSender{}, wantTexts: 1},
		{name: "waits through a not-ready pane", sender: &fakeDriverSender{errs: []error{notReady(), notReady()}}, wantTexts: 3, wantWaits: 2},
		{
			name:      "never-ready exhausts the attempts",
			sender:    &fakeDriverSender{repeatErr: notReady()},
			wantTexts: driverResumeSendAttempts,
			wantWaits: driverResumeSendAttempts - 1,
			wantErr:   []string{"lyx loom start", fmt.Sprint(driverResumeSendAttempts)},
		},
		{name: "another error is never retried", sender: &fakeDriverSender{repeatErr: errors.New("text never appeared")}, wantTexts: 1, wantErr: []string{"text never appeared"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, waits, marker := newResumeTestReceiver(t, tt.sender)

			err := c.resumeParkedDriver("g-drv")

			if len(tt.sender.texts) != tt.wantTexts {
				t.Fatalf("SendDriver calls = %d; want %d", len(tt.sender.texts), tt.wantTexts)
			}
			if *waits != tt.wantWaits {
				t.Errorf("waits = %d; want %d", *waits, tt.wantWaits)
			}
			if len(tt.wantErr) > 0 {
				if err == nil {
					t.Fatal("resumeParkedDriver() = nil; want a refusal")
				}
				for _, want := range tt.wantErr {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("error %q lacks %q", err, want)
					}
				}
				if !markerExists(marker) {
					t.Error("park marker was removed despite the failed resume")
				}
				return
			}
			if err != nil {
				t.Fatalf("resumeParkedDriver() error = %v; want nil", err)
			}
			text := tt.sender.texts[len(tt.sender.texts)-1]
			if strings.ContainsAny(text, "\r\n") {
				t.Errorf("resume text %q is not a single line", text)
			}
			reportsDir := shedrun.DriveReportsDir(c.location, shedrun.ResolveRunID(c.location, c.runID))
			if !strings.Contains(text, reportsDir) {
				t.Errorf("resume text %q names no path under %q", text, reportsDir)
			}
			if markerExists(marker) {
				t.Error("park marker still on disk after a verified send")
			}
		})
	}
}
