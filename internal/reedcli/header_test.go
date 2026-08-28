// header_test.go covers the `header` verb's pure command construction: Use, Short, and the
// --blocking flag registration, plus the blocking tail's keepalive-survival contract, exercised
// with headerWatch and headerPark both stubbed via card 19's package vars.
// The enveloped default's end-to-end PreRunE -> HeaderText round trip is covered by the reed smoke
// suite (batch 4), not here.

package reedcli

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/reedengine"
)

func TestHeaderCmd_UseAndShort(t *testing.T) {
	c := &reedCLI{}
	cmd := c.headerCmd()

	if cmd.Use != "header" {
		t.Errorf("headerCmd().Use = %q; want %q", cmd.Use, "header")
	}
	if cmd.Short == "" {
		t.Error("headerCmd().Short is empty; want a non-empty short description")
	}
}

func TestHeaderCmd_BlockingFlagRegistered(t *testing.T) {
	c := &reedCLI{}
	cmd := c.headerCmd()

	flag := cmd.Flags().Lookup("blocking")
	if flag == nil {
		t.Fatal("headerCmd() did not register a --blocking flag")
	}
	if flag.Value.Type() != "bool" {
		t.Errorf("--blocking flag type = %q; want %q", flag.Value.Type(), "bool")
	}
	if flag.DefValue != "false" {
		t.Errorf("--blocking flag default = %q; want %q", flag.DefValue, "false")
	}
}

// newBlockingTestCLI builds a reedCLI whose Engine is real enough to run headerCmd's RunE: HeaderText
// dereferences e.cfg unconditionally before the --blocking branch is ever reached, so the bare
// &reedCLI{} shape the two tests above use would panic here. An empty Config.Header.Template falls
// back to the embedded default template, and RepoName/HubPath are the only two Geometry fields
// tokenvocab.Ctx consumes, so this renders cleanly with no filesystem or process I/O.
func newBlockingTestCLI(t *testing.T) *reedCLI {
	t.Helper()
	return &reedCLI{eng: reedengine.New(reedengine.Config{}, reedengine.Geometry{RepoName: "test-repo", HubPath: t.TempDir()})}
}

// stubHeaderTail substitutes headerWatch and headerPark with fakes recording their own call counts,
// restoring both package vars via t.Cleanup, and reports pointers to the two counters.
func stubHeaderTail(t *testing.T, watchErr error) (watchCalls, parkCalls *int) {
	t.Helper()
	watchCalls = new(int)
	parkCalls = new(int)

	origWatch, origPark := headerWatch, headerPark
	headerWatch = func(ctx context.Context, eng *reedengine.Engine) error {
		*watchCalls++
		return watchErr
	}
	headerPark = func() {
		*parkCalls++
	}
	t.Cleanup(func() {
		headerWatch, headerPark = origWatch, origPark
	})

	return watchCalls, parkCalls
}

func TestHeaderCmd_BlockingTailParksAfterNilWatch(t *testing.T) {
	watchCalls, parkCalls := stubHeaderTail(t, nil)
	t.Cleanup(func() { logger.SetOutput(os.Stderr) })

	c := newBlockingTestCLI(t)
	cmd := c.headerCmd()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"--blocking"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("cmd.Execute() = %v; want nil", err)
	}
	if *watchCalls != 1 {
		t.Errorf("headerWatch call count = %d; want 1", *watchCalls)
	}
	if *parkCalls != 1 {
		t.Errorf("headerPark call count = %d; want 1", *parkCalls)
	}
}

func TestHeaderCmd_BlockingTailParksAfterWatchError(t *testing.T) {
	watchCalls, parkCalls := stubHeaderTail(t, errWatchFailedForTest)
	t.Cleanup(func() { logger.SetOutput(os.Stderr) })

	c := newBlockingTestCLI(t)
	cmd := c.headerCmd()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"--blocking"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("cmd.Execute() = %v; want nil (a non-nil headerWatch error must never propagate out of RunE)", err)
	}
	if *watchCalls != 1 {
		t.Errorf("headerWatch call count = %d; want 1", *watchCalls)
	}
	if *parkCalls != 1 {
		t.Errorf("headerPark call count = %d; want 1", *parkCalls)
	}

	// This is the keepalive-survival assertion: an obvious implementation propagates the error and
	// kills the pane, which would show up here as a JSON envelope or error text in buf beyond the
	// rendered header text.
	out := buf.String()
	if strings.Contains(out, `"status"`) || strings.Contains(out, `"error"`) {
		t.Errorf("blocking output = %q; want only the rendered header text, no JSON envelope or error text", out)
	}
}

func TestHeaderCmd_NonBlockingModeUnaffected(t *testing.T) {
	watchCalls, parkCalls := stubHeaderTail(t, nil)

	c := newBlockingTestCLI(t)
	cmd := c.headerCmd()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetArgs([]string{})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("cmd.Execute() = %v; want nil", err)
	}
	if *watchCalls != 0 {
		t.Errorf("headerWatch call count = %d; want 0 in non-blocking mode", *watchCalls)
	}
	if *parkCalls != 0 {
		t.Errorf("headerPark call count = %d; want 0 in non-blocking mode", *parkCalls)
	}
	if !strings.Contains(buf.String(), `"text"`) {
		t.Errorf("non-blocking output = %q; want the JSON envelope with a \"text\" field", buf.String())
	}
}

func TestHeaderCmd_LongMentionsWatchdog(t *testing.T) {
	c := &reedCLI{}
	cmd := c.headerCmd()

	if !strings.Contains(cmd.Long, "watchdog") {
		t.Errorf("headerCmd().Long does not mention %q; want the self-heal watch loop documented there", "watchdog")
	}
}

// errWatchFailedForTest is a sentinel error used to exercise headerWatch's error path without
// depending on any real reedengine failure mode.
var errWatchFailedForTest = errHeaderWatchStub{}

// errHeaderWatchStub is a trivial error type local to this test file.
type errHeaderWatchStub struct{}

// Error implements the error interface with a fixed, test-only message.
func (errHeaderWatchStub) Error() string { return "stub watch failure for test" }
