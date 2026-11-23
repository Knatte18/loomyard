// remotecontrol_test.go covers the bounded remote-control wait and the checkpoint it counts from.

package shuttleengine

import (
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// remoteControlRecord is a scripted provider record of a session's remote-control connections.
type remoteControlRecord struct {
	mu sync.Mutex
	// checkpoint is what RemoteControlCheckpoint answers until pasted reports true.
	checkpoint int64
	// pasted reports whether the command has been pasted; nil means it never reports true.
	pasted func() bool
	// checkpointAfterPaste is what RemoteControlCheckpoint answers once pasted reports true.
	checkpointAfterPaste int64
	// checkpointErr fails RemoteControlCheckpoint.
	checkpointErr error
	// connectAtRead is the read, counted from one, from which the record shows a connection; zero never shows one.
	connectAtRead int
	// readErr fails every read.
	readErr error
	// readFrom holds the checkpoint each read was given, in order.
	readFrom []int64
}

func (r *remoteControlRecord) RemoteControlCheckpoint(string, string) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.checkpointErr != nil {
		return 0, r.checkpointErr
	}
	if r.pasted != nil && r.pasted() {
		return r.checkpointAfterPaste, nil
	}
	return r.checkpoint, nil
}

func (r *remoteControlRecord) RemoteControlConnected(_, _ string, checkpoint int64) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.readFrom = append(r.readFrom, checkpoint)
	if r.readErr != nil {
		return false, r.readErr
	}
	return r.connectAtRead > 0 && len(r.readFrom) >= r.connectAtRead, nil
}

// remoteControlReadingEngine is a fake engine whose remote-control connections come from a scripted record.
type remoteControlReadingEngine struct {
	*fakeEngine
	*remoteControlRecord
}

// remoteControlPasteEngine is the slash paste engine with a scripted record of remote-control connections.
type remoteControlPasteEngine struct {
	*slashPasteEngine
	*remoteControlRecord
}

// TestRunner_AwaitRemoteControl pins the bounded wait for a session to record its remote control connecting.
// It also pins the checkpoint the wait starts from:
// the engine's answer, and zero for an engine that cannot read remote control or a session with no ID.
func TestRunner_AwaitRemoteControl(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	readFailed := errors.New("transcript unreadable")
	tests := []struct {
		name          string
		connectAtRead int
		readErr       error
		// noReader runs on an engine that cannot read remote control.
		noReader bool
		// emptySessionID passes no session ID instead of "session-1".
		emptySessionID bool
		wantConnected  bool
		wantErr        error
		wantReads      int
		wantWaited     time.Duration
	}{
		{name: "ConnectionRecordedAtOnce", connectAtRead: 1, wantConnected: true, wantReads: 1},
		{name: "ConnectionRecordedAfterTwoTicks", connectAtRead: 3, wantConnected: true, wantReads: 3, wantWaited: time.Second},
		{name: "RefusedConnectionReportsFalseOnceTheWaitPasses", wantReads: 41, wantWaited: 20 * time.Second},
		{name: "UnreadableRecordFails", readErr: readFailed, wantErr: readFailed, wantReads: 1},
		{name: "EngineWithoutTheReaderReportsFalse", noReader: true},
		{name: "EmptySessionIDReadsNothing", connectAtRead: 1, emptySessionID: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			record := &remoteControlRecord{checkpoint: 7, connectAtRead: tt.connectAtRead, readErr: tt.readErr}
			var engine Engine = &remoteControlReadingEngine{fakeEngine: readyAgentEngine(), remoteControlRecord: record}
			wantCheckpoint := int64(7)
			if tt.noReader || tt.emptySessionID {
				wantCheckpoint = 0
			}
			if tt.noReader {
				engine = readyAgentEngine()
			}
			sessionID := "session-1"
			if tt.emptySessionID {
				sessionID = ""
			}
			start := time.Date(2026, 11, 23, 9, 0, 0, 0, time.UTC)
			clock := newFakeClock(start)
			runner := newFixture(t, &fakeReed{}, engine, withClock(clock)).Runner

			checkpoint, err := runner.RemoteControlCheckpoint("", sessionID)
			if err != nil || checkpoint != wantCheckpoint {
				t.Fatalf("RemoteControlCheckpoint() = %d, %v; want %d, nil", checkpoint, err, wantCheckpoint)
			}
			connected, err := runner.AwaitRemoteControl("", sessionID, checkpoint)

			if !errors.Is(err, tt.wantErr) {
				t.Errorf("AwaitRemoteControl() error = %v; want %v", err, tt.wantErr)
			}
			if connected != tt.wantConnected {
				t.Errorf("AwaitRemoteControl() = %v; want %v", connected, tt.wantConnected)
			}
			if got := len(record.readFrom); got != tt.wantReads {
				t.Errorf("record read %d times; want %d", got, tt.wantReads)
			}
			if i := slices.IndexFunc(record.readFrom, func(from int64) bool { return from != wantCheckpoint }); i >= 0 {
				t.Errorf("read %d was given checkpoint %d; want %d", i+1, record.readFrom[i], wantCheckpoint)
			}
			if got := clock.Now().Sub(start); got != tt.wantWaited {
				t.Errorf("waited %v; want %v", got, tt.wantWaited)
			}
		})
	}
}

// TestRunner_TurnOnRemoteControl_ReportsTheRecordedConnection pins when a typed command reports a connection.
// It reports one only once the session's record shows one past the point it reached before the paste.
// A failed read of the record after the paste is no error, a paste not seen to land still waits for the connection,
// and a failed checkpoint read types nothing.
//
//testtiming:keep pins the connected result TurnOnRemoteControl returns and the checkpoint its read starts from
func TestRunner_TurnOnRemoteControl_ReportsTheRecordedConnection(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	readFailed := errors.New("transcript unreadable")
	checkpointFailed := errors.New("transcript size unreadable")
	unconfirmed := fmt.Errorf("paste unconfirmed: %w", ErrSubmissionNotLanded)
	tests := []struct {
		name          string
		connectAtRead int
		readErr       error
		checkpointErr error
		// pasteErr fails the paste after it was made.
		pasteErr      error
		wantConnected bool
		wantErr       error
		wantPastes    int
	}{
		{name: "RecordedConnectionReportsConnected", connectAtRead: 1, wantConnected: true, wantPastes: 1},
		{name: "RefusedConnectionReportsFalse", wantPastes: 1},
		{name: "UnreadableRecordAfterThePasteReportsFalseWithoutAnError", readErr: readFailed, wantPastes: 1},
		{name: "UnreadableCheckpointFailsBeforeThePaste", checkpointErr: checkpointFailed, wantErr: checkpointFailed},
		{
			name: "UnconfirmedPasteStillReportsTheRecordedConnection", connectAtRead: 1, pasteErr: unconfirmed,
			wantConnected: true, wantErr: ErrSubmissionNotLanded, wantPastes: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			runner, reed, _, _ := newSlashPasteRunnerAppending(t, "", false, false, "", remoteControlReadingShape, nil)
			reed.PasteSubmitErr = tt.pasteErr
			engine, err := runner.engines.Lookup("")
			if err != nil {
				t.Fatalf("look up the runner's engine: %v", err)
			}
			record := engine.(*remoteControlPasteEngine).remoteControlRecord
			record.checkpoint, record.checkpointAfterPaste = 42, 99
			record.pasted = func() bool { return len(reed.PasteSubmitCalls) > 0 }
			record.connectAtRead, record.readErr, record.checkpointErr = tt.connectAtRead, tt.readErr, tt.checkpointErr

			connected, err := runner.TurnOnRemoteControl("strand-1", "neve::tst:orch")

			if !errors.Is(err, tt.wantErr) || (tt.wantErr == nil && err != nil) {
				t.Errorf("TurnOnRemoteControl() error = %v; want %v", err, tt.wantErr)
			}
			if connected != tt.wantConnected {
				t.Errorf("TurnOnRemoteControl() = %v; want %v", connected, tt.wantConnected)
			}
			if len(reed.PasteSubmitCalls) != tt.wantPastes {
				t.Errorf("pastes = %+v; want %d", reed.PasteSubmitCalls, tt.wantPastes)
			}
			if tt.wantPastes > 0 && len(record.readFrom) == 0 {
				t.Errorf("the record was never read after the paste")
			}
			if tt.wantPastes == 0 && len(record.readFrom) != 0 {
				t.Errorf("the record was read %v although nothing was pasted", record.readFrom)
			}
			for i, from := range record.readFrom {
				if from != 42 {
					t.Errorf("read %d was given checkpoint %d; want the pre-paste checkpoint 42", i+1, from)
				}
			}
		})
	}
}
