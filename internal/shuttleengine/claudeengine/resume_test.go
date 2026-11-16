// resume_test.go covers checkResumable and sessionHolder.
// It uses a temp project directory, a fixture session registry and a fake liveness probe.
// A malformed id, a missing or unstattable transcript and a live holder refuse.
// A live holder whose start time cannot be compared refuses as unproven.
// A dead, non-matching or pid-reused holder proceeds;
// an absent registry or an undecodable entry proceeds with a warning.

package claudeengine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

const resumeTestID = "01234567-89ab-4cde-8f01-23456789abcd"

// resumeFixture builds a project dir holding the transcript (when withTranscript) and a registry dir.
func resumeFixture(t *testing.T, withTranscript bool) (projectDir, registryDir string) {
	t.Helper()
	root := t.TempDir()
	projectDir = filepath.Join(root, "project")
	registryDir = filepath.Join(root, "sessions")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(registryDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if withTranscript {
		if err := os.WriteFile(filepath.Join(projectDir, resumeTestID+".jsonl"), []byte("{}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return projectDir, registryDir
}

func writeRegistryEntry(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCheckResumable(t *testing.T) {
	tiergate.Require(t, 0)
	aliveAll := func(int) bool { return true }
	aliveNone := func(int) bool { return false }

	tests := []struct {
		name        string
		id          string
		transcript  bool
		setup       func(t *testing.T, registryDir string) string // returns the registry dir to use
		alive       func(int) bool
		start       func(int) (string, bool) // nil reads "100" for every pid
		wantErr     string
		wantWarning bool
		// wantNoTranscript expects the refusal to wrap shuttleengine.ErrNoTranscript.
		wantNoTranscript bool
		// wantHolder is the pid a *shuttleengine.SessionHeldError refusal names, zero for any other outcome.
		wantHolder int
	}{
		{
			name:       "malformed id refuses before any file is read",
			id:         "not-a-uuid",
			transcript: true,
			alive:      aliveAll,
			wantErr:    "/status",
		},
		{
			name:             "missing transcript refuses",
			id:               resumeTestID,
			transcript:       false,
			alive:            aliveAll,
			wantErr:          "only a session run from this directory",
			wantNoTranscript: true,
		},
		{
			name:       "live matching pid refuses",
			id:         resumeTestID,
			transcript: true,
			setup: func(t *testing.T, dir string) string {
				writeRegistryEntry(t, dir, "a.json", fmt.Sprintf(`{"pid":4242,"sessionId":%q,"procStart":"100"}`, resumeTestID))
				return dir
			},
			alive:      aliveAll,
			wantErr:    "live process 4242",
			wantHolder: 4242,
		},
		{
			name:       "live pid with a mismatching start time is reused and proceeds",
			id:         resumeTestID,
			transcript: true,
			setup: func(t *testing.T, dir string) string {
				writeRegistryEntry(t, dir, "a.json", fmt.Sprintf(`{"pid":4242,"sessionId":%q,"procStart":"99"}`, resumeTestID))
				return dir
			},
			alive: aliveAll,
		},
		{
			name:       "live pid with an unreadable start time refuses as unproven",
			id:         resumeTestID,
			transcript: true,
			setup: func(t *testing.T, dir string) string {
				writeRegistryEntry(t, dir, "a.json", fmt.Sprintf(`{"pid":4242,"sessionId":%q,"procStart":"99"}`, resumeTestID))
				return dir
			},
			alive:      aliveAll,
			start:      func(int) (string, bool) { return "", false },
			wantErr:    "could not be proven reused",
			wantHolder: 4242,
		},
		{
			name:       "live pid with no procStart refuses as unproven",
			id:         resumeTestID,
			transcript: true,
			setup: func(t *testing.T, dir string) string {
				writeRegistryEntry(t, dir, "a.json", fmt.Sprintf(`{"pid":4242,"sessionId":%q}`, resumeTestID))
				return dir
			},
			alive:      aliveAll,
			wantErr:    "and the pid could not be proven reused",
			wantHolder: 4242,
		},
		{
			name:       "dead matching pid proceeds silently",
			id:         resumeTestID,
			transcript: true,
			setup: func(t *testing.T, dir string) string {
				writeRegistryEntry(t, dir, "a.json", fmt.Sprintf(`{"pid":4242,"sessionId":%q}`, resumeTestID))
				return dir
			},
			alive: aliveNone,
		},
		{
			name:       "non-matching live entry proceeds",
			id:         resumeTestID,
			transcript: true,
			setup: func(t *testing.T, dir string) string {
				writeRegistryEntry(t, dir, "a.json", `{"pid":4242,"sessionId":"ffffffff-ffff-4fff-8fff-ffffffffffff"}`)
				return dir
			},
			alive: aliveAll,
		},
		{
			name:       "absent registry directory proceeds with a warning",
			id:         resumeTestID,
			transcript: true,
			setup: func(t *testing.T, dir string) string {
				return filepath.Join(dir, "missing")
			},
			alive:       aliveAll,
			wantWarning: true,
		},
		{
			name:       "undecodable entry proceeds with a warning",
			id:         resumeTestID,
			transcript: true,
			setup: func(t *testing.T, dir string) string {
				writeRegistryEntry(t, dir, "bad.json", "{not json")
				return dir
			},
			alive:       aliveAll,
			wantWarning: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			projectDir, registryDir := resumeFixture(t, tt.transcript)
			if tt.setup != nil {
				registryDir = tt.setup(t, registryDir)
			}
			start := tt.start
			if start == nil {
				start = func(int) (string, bool) { return "100", true }
			}
			warning, err := checkResumable(tt.id, projectDir, registryDir, tt.alive, start)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want containing %q", err, tt.wantErr)
				}
				if got := errors.Is(err, shuttleengine.ErrNoTranscript); got != tt.wantNoTranscript {
					t.Errorf("errors.Is(err, ErrNoTranscript) = %v; want %v", got, tt.wantNoTranscript)
				}
				holder := 0
				var held *shuttleengine.SessionHeldError
				if errors.As(err, &held) {
					holder = held.PID
				}
				if holder != tt.wantHolder {
					t.Errorf("SessionHeldError pid = %d; want %d", holder, tt.wantHolder)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if (warning != "") != tt.wantWarning {
				t.Fatalf("warning = %q, wantWarning %v", warning, tt.wantWarning)
			}
		})
	}
}

func TestSessionHolder(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name string
		// entry is the one registry file's content; empty leaves the registry empty.
		entry string
		// missingRegistry points the read at a registry directory that does not exist.
		missingRegistry bool
		alive           bool
		startReadable   bool
		wantPID         int
		wantFound       bool
		wantErr         bool
	}{
		{name: "LiveEntryWithAMatchingStartTimeNamesItsPID", entry: `{"pid":4242,"sessionId":%q,"procStart":"100"}`, alive: true, startReadable: true, wantPID: 4242, wantFound: true},
		{name: "LiveEntryWithAnUnreadableStartTimeStillNamesItsPID", entry: `{"pid":4242,"sessionId":%q,"procStart":"100"}`, alive: true, wantPID: 4242, wantFound: true},
		{name: "ReusedPIDHoldsNothing", entry: `{"pid":4242,"sessionId":%q,"procStart":"99"}`, alive: true, startReadable: true},
		{name: "DeadPIDHoldsNothing", entry: `{"pid":4242,"sessionId":%q,"procStart":"100"}`, startReadable: true},
		{name: "AnotherSessionsEntryHoldsNothing", entry: `{"pid":4242,"sessionId":"ffffffff-ffff-4fff-8fff-ffffffffffff%.0s"}`, alive: true, startReadable: true},
		{name: "UnreadableRegistryIsAnError", missingRegistry: true, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, registryDir := resumeFixture(t, false)
			if tt.entry != "" {
				writeRegistryEntry(t, registryDir, "a.json", fmt.Sprintf(tt.entry, resumeTestID))
			}
			if tt.missingRegistry {
				registryDir = filepath.Join(registryDir, "missing")
			}
			alive := func(int) bool { return tt.alive }
			start := func(int) (string, bool) { return "100", tt.startReadable }

			pid, found, err := sessionHolder(resumeTestID, registryDir, alive, start)

			if (err != nil) != tt.wantErr {
				t.Fatalf("sessionHolder() error = %v; want error %v", err, tt.wantErr)
			}
			if pid != tt.wantPID || found != tt.wantFound {
				t.Errorf("sessionHolder() = %d, %v; want %d, %v", pid, found, tt.wantPID, tt.wantFound)
			}
		})
	}
}
