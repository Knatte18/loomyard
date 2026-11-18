// resume_test.go covers checkResumable, sessionHolder and CarryTranscript.
// It uses a temp project directory, a fixture session registry and a fake liveness probe.
// The carry runs on fake config directories under a temp hub.
// A malformed id, a missing or unstattable transcript and a live holder refuse.
// A live holder whose start time cannot be compared refuses as unproven.
// A dead, non-matching or pid-reused holder proceeds;
// an absent registry directory proceeds silently;
// an unreadable registry or an undecodable entry proceeds with a warning.

package claudeengine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
		name       string
		id         string
		transcript bool
		setup      func(t *testing.T, registryDir string) string // returns the registry dir to use
		// secondEntry, when set, is the one file of a second registry directory read after the first.
		secondEntry string
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
			name:        "live entry in the second registry refuses",
			id:          resumeTestID,
			transcript:  true,
			secondEntry: `{"pid":4242,"sessionId":%q,"procStart":"100"}`,
			alive:       aliveAll,
			wantErr:     "live process 4242",
			wantHolder:  4242,
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
			name:       "absent registry directory proceeds silently",
			id:         resumeTestID,
			transcript: true,
			setup: func(t *testing.T, dir string) string {
				return filepath.Join(dir, "missing")
			},
			alive: aliveAll,
		},
		{
			name:       "unreadable registry proceeds with a warning",
			id:         resumeTestID,
			transcript: true,
			setup: func(t *testing.T, dir string) string {
				writeRegistryEntry(t, dir, "notadir", "x")
				return filepath.Join(dir, "notadir")
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
			registryDirs := []string{registryDir}
			if tt.secondEntry != "" {
				_, second := resumeFixture(t, false)
				writeRegistryEntry(t, second, "c.json", fmt.Sprintf(tt.secondEntry, resumeTestID))
				registryDirs = append(registryDirs, second)
			}
			warning, err := checkResumable(tt.id, projectDir, registryDirs, tt.alive, start)
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

// TestClaude_CarryTranscript pins which copy of a session's transcript the resolved account's directory ends up holding.
// The account b is the one every row resolves to.
// Each copy's content names the directory it was written under.
func TestClaude_CarryTranscript(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	base := time.Unix(2_000_000, 0)
	const (
		two      = "accounts:\n  - {name: a, config_dir: ROOT/a, weight: 1}\n  - {name: b, config_dir: ROOT/b, weight: 1}\n"
		beamsPin = "account_pins:\n  beams: b\n"
	)
	task := shuttleengine.Worktree{Name: "t", Kind: shuttleengine.WorktreeTask}
	prime := shuttleengine.Worktree{Name: "prime", Kind: shuttleengine.WorktreePrime}
	tests := []struct {
		name     string
		machine  string
		worktree shuttleengine.Worktree
		// entry is the account the worktree's entry names.
		entry string
		// copies maps an account directory to the write time of its copy of the transcript.
		copies map[string]time.Time
		// wantCopied is what CarryTranscript reports.
		wantCopied bool
		// wantContent names the directory whose copy account b's project holds afterwards; empty means it holds none.
		wantContent string
	}{
		{name: "ATranscriptOnlyUnderAnotherDirectoryIsCopiedIntoAProjectThatDoesNotExist", machine: two, worktree: task, entry: "b", copies: map[string]time.Time{"a": base}, wantCopied: true, wantContent: "a"},
		{name: "ANewerCopyReplacesTheResolvedDirectorysOlderOne", machine: two, worktree: task, entry: "b", copies: map[string]time.Time{"a": base.Add(time.Minute), "b": base}, wantCopied: true, wantContent: "a"},
		{name: "TheResolvedDirectorysNewestCopyIsLeftAlone", machine: two, worktree: task, entry: "b", copies: map[string]time.Time{"a": base, "b": base.Add(time.Minute)}, wantContent: "b"},
		{name: "ATranscriptNowhereIsNotCopiedAndTheCheckRefuses", machine: two, worktree: task, entry: "b"},
		{name: "ABeamsPinMovesThePrimesCopyIntoThePinnedDirectory", machine: two + beamsPin, worktree: prime, entry: "a", copies: map[string]time.Time{"a": base}, wantCopied: true, wantContent: "a"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			root, accountDir := t.TempDir(), t.TempDir()
			workdir := filepath.Join(root, "work")
			worktree := tt.worktree
			worktree.AccountDir = accountDir
			engine, _ := engineWithAccounts(t, strings.ReplaceAll(tt.machine, "ROOT", root), worktree)
			entry := accountFile{Worktrees: map[string]string{worktree.Name: tt.entry}}
			if err := updateAccountFile(accountDir, func(accountFile) (accountFile, error) { return entry, nil }); err != nil {
				t.Fatalf("seed the account file: %v", err)
			}
			transcriptOf := func(dir string) string {
				return filepath.Join(projectDirUnder(filepath.Join(root, dir, projectsDirName), workdir), resumeTestID+".jsonl")
			}
			forkOf := func(dir string) string {
				return filepath.Join(projectDirUnder(filepath.Join(root, dir, projectsDirName), workdir), resumeTestID, subagentsDirName, "agent-1.jsonl")
			}
			for dir, writtenAt := range tt.copies {
				for _, path := range []string{transcriptOf(dir), forkOf(dir)} {
					writeClaudeLayer(t, path, "from "+dir)
					if err := os.Chtimes(path, writtenAt, writtenAt); err != nil {
						t.Fatal(err)
					}
				}
			}

			copied, err := engine.CarryTranscript(resumeTestID, workdir)
			if err != nil {
				t.Fatalf("CarryTranscript() error: %v", err)
			}
			if copied != tt.wantCopied {
				t.Errorf("CarryTranscript() = %v; want %v", copied, tt.wantCopied)
			}

			_, checkErr := engine.CheckResume(resumeTestID, workdir)
			if tt.wantContent == "" {
				if !errors.Is(checkErr, shuttleengine.ErrNoTranscript) {
					t.Errorf("CheckResume() error = %v; want ErrNoTranscript", checkErr)
				}
				return
			}
			if checkErr != nil {
				t.Errorf("CheckResume() error = %v; want the carried transcript found", checkErr)
			}
			want := "from " + tt.wantContent
			for _, path := range []string{transcriptOf("b"), forkOf("b")} {
				if got, err := os.ReadFile(path); err != nil || string(got) != want {
					t.Errorf("%s = %q, err %v; want %q", path, got, err, want)
				}
			}
			for dir := range tt.copies {
				if got, err := os.ReadFile(transcriptOf(dir)); dir != "b" && (err != nil || string(got) != "from "+dir) {
					t.Errorf("source %s = %q, err %v; want it left in place", transcriptOf(dir), got, err)
				}
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
		// laterEntry, when set, is a second registry file read after entry.
		laterEntry string
		// secondEntry, when set, is the one file of a second registry directory.
		secondEntry string
		// secondMissing adds a second registry directory that does not exist.
		secondMissing bool
		// missingRegistry points the read at a registry directory that does not exist.
		missingRegistry bool
		// unreadableRegistry points the read at a registry path that is a regular file, so it cannot be read as a directory.
		unreadableRegistry bool
		alive              bool
		startReadable      bool
		wantPID            int
		wantFound          bool
		wantErr            bool
	}{
		{name: "LiveEntryWithAMatchingStartTimeNamesItsPID", entry: `{"pid":4242,"sessionId":%q,"procStart":"100"}`, alive: true, startReadable: true, wantPID: 4242, wantFound: true},
		{name: "LiveEntryWithAnUnreadableStartTimeStillNamesItsPID", entry: `{"pid":4242,"sessionId":%q,"procStart":"100"}`, alive: true, wantPID: 4242, wantFound: true},
		{name: "ReusedPIDHoldsNothing", entry: `{"pid":4242,"sessionId":%q,"procStart":"99"}`, alive: true, startReadable: true},
		{name: "DeadPIDHoldsNothing", entry: `{"pid":4242,"sessionId":%q,"procStart":"100"}`, startReadable: true},
		{name: "AnotherSessionsEntryHoldsNothing", entry: `{"pid":4242,"sessionId":"ffffffff-ffff-4fff-8fff-ffffffffffff%.0s"}`, alive: true, startReadable: true},
		{name: "MissingRegistryHoldsNothing", missingRegistry: true},
		{name: "UnreadableRegistryIsAnError", unreadableRegistry: true, wantErr: true},
		{name: "UndecodableEntryIsAnError", entry: "{not json", alive: true, startReadable: true, wantErr: true},
		{name: "LiveHolderBehindAnUndecodableEntryIsStillFound", entry: "{not json", laterEntry: `{"pid":4242,"sessionId":%q,"procStart":"100"}`, alive: true, startReadable: true, wantPID: 4242, wantFound: true},
		{name: "LiveEntryInASecondRegistryHolds", secondEntry: `{"pid":4242,"sessionId":%q,"procStart":"100"}`, alive: true, startReadable: true, wantPID: 4242, wantFound: true},
		{name: "LiveHolderBesideAMissingSecondRegistryIsFound", entry: `{"pid":4242,"sessionId":%q,"procStart":"100"}`, secondMissing: true, alive: true, startReadable: true, wantPID: 4242, wantFound: true},
		{name: "DeadHolderBesideAMissingSecondRegistryHoldsNothing", entry: `{"pid":4242,"sessionId":%q,"procStart":"100"}`, secondMissing: true, startReadable: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, registryDir := resumeFixture(t, false)
			if tt.entry != "" {
				writeRegistryEntry(t, registryDir, "a.json", fmt.Sprintf(tt.entry, resumeTestID))
			}
			if tt.laterEntry != "" {
				writeRegistryEntry(t, registryDir, "b.json", fmt.Sprintf(tt.laterEntry, resumeTestID))
			}
			if tt.missingRegistry {
				registryDir = filepath.Join(registryDir, "missing")
			}
			if tt.unreadableRegistry {
				writeRegistryEntry(t, registryDir, "notadir", "x")
				registryDir = filepath.Join(registryDir, "notadir")
			}
			registryDirs := []string{registryDir}
			if tt.secondEntry != "" {
				_, second := resumeFixture(t, false)
				writeRegistryEntry(t, second, "c.json", fmt.Sprintf(tt.secondEntry, resumeTestID))
				registryDirs = append(registryDirs, second)
			}
			if tt.secondMissing {
				registryDirs = append(registryDirs, filepath.Join(registryDir, "second-missing"))
			}
			alive := func(int) bool { return tt.alive }
			start := func(int) (string, bool) { return "100", tt.startReadable }

			pid, found, err := sessionHolder(resumeTestID, registryDirs, alive, start)

			if (err != nil) != tt.wantErr {
				t.Fatalf("sessionHolder() error = %v; want error %v", err, tt.wantErr)
			}
			if pid != tt.wantPID || found != tt.wantFound {
				t.Errorf("sessionHolder() = %d, %v; want %d, %v", pid, found, tt.wantPID, tt.wantFound)
			}
		})
	}
}
