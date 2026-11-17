// set_test.go covers `lym machine name <name>` over fake hubs.
// It checks the refusals, the machine file written and the sweep requests sent.

package machinecli

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/machinename"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

const (
	hubA = "/hubs/a"
	hubB = "/hubs/b"
)

// fakeHubs stands in for the hub seams of deps, keyed by board directory.
// It records each owner write request and answers what its fields hold, the zero value for a hub it holds nothing for.
type fakeHubs struct {
	mu          sync.Mutex
	registry    []string
	registryErr error
	// cwd is the hub the cwdHub seam answers, none when empty.
	cwd         string
	skip        map[string]fabricengine.BoltSkip
	pullErr     map[string]error
	unpushedErr map[string]error
	ahead       map[string]bool
	owners      map[string]boardengine.Owners
	readErr     map[string]error
	writeErr    map[string]error
	write       map[string]boardengine.OwnerWrite
	requests    map[string][]boardengine.OwnerRequest
}

// newFakeHubs returns a fakeHubs whose machine record lists registry.
func newFakeHubs(registry ...string) *fakeHubs {
	return &fakeHubs{
		registry:    registry,
		skip:        map[string]fabricengine.BoltSkip{},
		pullErr:     map[string]error{},
		unpushedErr: map[string]error{},
		ahead:       map[string]bool{},
		owners:      map[string]boardengine.Owners{},
		readErr:     map[string]error{},
		writeErr:    map[string]error{},
		write:       map[string]boardengine.OwnerWrite{},
		requests:    map[string][]boardengine.OwnerRequest{},
	}
}

// board returns the board directory the seams are keyed by for hub.
func (f *fakeHubs) board(hub string) string {
	return fabricengine.BoardDir(hub)
}

// requestsFor returns the owner write requests the hub received.
func (f *fakeHubs) requestsFor(hub string) []boardengine.OwnerRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.requests[f.board(hub)]
}

// requestTotal returns how many owner write requests every hub received.
func (f *fakeHubs) requestTotal() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	total := 0
	for _, requests := range f.requests {
		total += len(requests)
	}
	return total
}

// attach returns d with its hub seams answered by f.
func (f *fakeHubs) attach(d deps) deps {
	d.hubs = func() ([]string, error) { return f.registry, f.registryErr }
	d.cwdHub = func(string) (string, bool) { return f.cwd, f.cwd != "" }
	d.pull = func(boardDir string) (fabricengine.BoltSkip, string, error) {
		return f.skip[boardDir], "skip detail", f.pullErr[boardDir]
	}
	d.unpushed = func(boardDir string) (bool, error) { return f.ahead[boardDir], f.unpushedErr[boardDir] }
	d.readOwners = func(boardDir string) (boardengine.Owners, error) { return f.owners[boardDir], f.readErr[boardDir] }
	d.setOwners = func(boardDir string, req boardengine.OwnerRequest) (boardengine.OwnerWrite, error) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.requests[boardDir] = append(f.requests[boardDir], req)
		return f.write[boardDir], f.writeErr[boardDir]
	}
	return d
}

// seams is the machine file and hostname side of a deps a test builds.
type seams struct {
	// path is the machine file's path; the locate seam resolves no directory when noDirectory is set.
	path        string
	noDirectory bool
	locateErr   error
	host        string
	hostErr     bool
}

// deps returns the deps over s and f.
func (s seams) deps(f *fakeHubs) deps {
	d := deps{
		locate: func() (string, string, bool, error) {
			if s.locateErr != nil {
				return "", "", false, s.locateErr
			}
			if s.noDirectory {
				return "", "", false, nil
			}
			return s.path, s.path + ".lock", true, nil
		},
		hostName: func() (string, error) {
			if s.hostErr {
				return "", errors.New("hostname unreadable")
			}
			return s.host, nil
		},
	}
	return f.attach(d)
}

// machineFileAt writes content to a new machine file under a temporary directory and returns its path.
// Empty content writes no file, and dirAtPath makes the path a directory.
func machineFileAt(t *testing.T, content string, dirAtPath bool) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "machine.json")
	switch {
	case dirAtPath:
		if err := os.Mkdir(path, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", path, err)
		}
	case content != "":
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	return path
}

// machineOf returns the `machine` object of an envelope.
func machineOf(t *testing.T, envelope map[string]any) map[string]any {
	t.Helper()
	machine, ok := envelope["machine"].(map[string]any)
	if !ok {
		t.Fatalf("envelope %v carries no machine object", envelope)
	}
	return machine
}

// hubOutcomes returns the per-hub outcomes of an envelope's machine object.
func hubOutcomes(t *testing.T, envelope map[string]any) []map[string]any {
	t.Helper()
	var outcomes []map[string]any
	for _, raw := range machineOf(t, envelope)["hubs"].([]any) {
		outcomes = append(outcomes, raw.(map[string]any))
	}
	return outcomes
}

// requireSweepRequest fails the test unless hub received exactly one owner write that sweeps replace for name.
func requireSweepRequest(t *testing.T, f *fakeHubs, hub, name string, replace []string) {
	t.Helper()
	requests := f.requestsFor(hub)
	if len(requests) != 1 {
		t.Fatalf("hub %s received %d owner writes; want 1", hub, len(requests))
	}
	req := requests[0]
	if req.Machine != name || !slices.Equal(req.Replace, replace) {
		t.Errorf("hub %s request = Machine %q, Replace %v; want %q, %v", hub, req.Machine, req.Replace, name, replace)
	}
	if req.Takeover || req.Prime || req.Claim != "" || len(req.Slugs) != 0 {
		t.Errorf("hub %s request = %+v; want no takeover, prime, claim or slugs", hub, req)
	}
}

// requireRecoveries fails the test unless the envelope's machine object lists recoveries.
func requireRecoveries(t *testing.T, envelope map[string]any) {
	t.Helper()
	if recoveries, _ := machineOf(t, envelope)["recoveries"].([]any); len(recoveries) == 0 {
		t.Errorf("machine.recoveries = %v; want a non-empty list", machineOf(t, envelope)["recoveries"])
	}
}

// wantNoDirectoryVariable is the variable the refusal of a missing directory names on this platform.
func wantNoDirectoryVariable() string {
	if runtime.GOOS == "windows" {
		return "LOCALAPPDATA"
	}
	return "XDG_CONFIG_HOME"
}

func TestName_Set(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	collidingOwners := func(f *fakeHubs) {
		f.owners[f.board(hubA)] = boardengine.Owners{Entries: map[string]boardengine.OwnerEntry{"x": {Owner: "osl46", IsProject: true, Beam: true}}}
	}
	tests := []struct {
		name        string
		content     string
		dirAtPath   bool
		noDirectory bool
		locateErr   error
		host        string
		hostErr     bool
		arg         string
		setup       func(f *fakeHubs)
		// wantRefusal lists substrings of the refusal's message; empty means the set succeeds.
		wantRefusal []string
		wantNames   []string
		wantWritten bool
		// wantDropped is how many empty entries the output says were dropped.
		wantDropped        int
		wantInvalidDropped bool
		wantNamesLost      bool
	}{
		{name: "InvalidNameIsRefusedAndWritesNothing", arg: "Bad_Name", wantRefusal: []string{"is invalid"}},
		{name: "NoDirectoryIsRefusedNamingTheVariables", noDirectory: true, wantRefusal: []string{wantNoDirectoryVariable(), "absolute path"}},
		{name: "RelativeVariableIsRefused", locateErr: errors.New("XDG_CONFIG_HOME is relative"), wantRefusal: []string{"XDG_CONFIG_HOME is relative"}},
		{name: "UnreadableFileIsRefusedNamingItsPath", dirAtPath: true, wantRefusal: []string{"machine.json", "remove the file or fix its permissions"}},
		{
			name:        "RegistryReadFailureIsRefusedBeforeAnyWrite",
			setup:       func(f *fakeHubs) { f.registryErr = errors.New("registry boom") },
			wantRefusal: []string{"registry boom"},
		},
		{
			name: "UnreadableBoardIsRefusedBeforeAnyWrite",
			setup: func(f *fakeHubs) {
				f.registry = []string{hubA, hubB}
				f.readErr[f.board(hubB)] = errors.New("board boom")
			},
			wantRefusal: []string{hubB, "board boom", "lym machine name osl46"},
		},
		{
			name: "CollisionIsRefusedNamingHubAndOwner", setup: collidingOwners,
			wantRefusal: []string{hubA, "x", `"osl46"`, "by hand", "lym adopt"},
		},
		{
			name: "PrimeCollisionIsRefusedNamingHubAndPrime",
			setup: func(f *fakeHubs) {
				f.owners[f.board(hubA)] = boardengine.Owners{Prime: "osl46"}
			},
			wantRefusal: []string{hubA, boardengine.OwnerWritePrime, `"osl46"`, "lym adopt"},
		},
		{
			name: "NoFileTakesTheHostnameNameAsOld", host: "osl-1046",
			wantNames: []string{"osl-1046", "osl46"}, wantWritten: true,
		},
		{
			name: "AlreadySetWritesNothing", content: `{"name":"osl46","names":["box","osl46"]}`,
			wantNames: []string{"box", "osl46"},
		},
		{
			name: "NoHostnameAddsNone", content: `{"name":"neve","names":["neve"]}`, hostErr: true,
			wantNames: []string{"neve", "osl46"}, wantWritten: true,
		},
		{
			name: "SetNameOutOfOrderMovesToTheEnd", content: `{"name":"osl46","names":["osl46","neve"]}`, host: "osl46",
			wantNames: []string{"neve", "osl46"}, wantWritten: true,
		},
		{
			name: "OwnFormerNamePassesTheCollisionCheck", content: `{"name":"BAD","names":["neve","osl46"]}`, setup: collidingOwners,
			wantNames: []string{"neve", "box", "osl46"}, wantWritten: true, wantInvalidDropped: true,
		},
		{
			name: "UndecodableFileLosesItsNamesAndSaysSo", content: "not json",
			wantNames: []string{"box", "osl46"}, wantWritten: true, wantNamesLost: true,
		},
		{
			name: "HandWrittenNameSweepsTheHostnameName", content: `{"name":"osl46"}`,
			wantNames: []string{"box", "osl46"}, wantWritten: true,
		},
		{
			name: "HostnameNamePassesTheCollisionCheck", arg: "box", host: "box",
			setup: func(f *fakeHubs) {
				f.owners[f.board(hubA)] = boardengine.Owners{Prime: "box"}
			},
			wantNames: []string{"box"}, wantWritten: true,
		},
		{
			name: "EmptyEntryDropIsReported", content: `{"name":"box","names":["","box"]}`,
			wantNames: []string{"box", "osl46"}, wantWritten: true, wantDropped: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			arg, host := tt.arg, tt.host
			if arg == "" {
				arg = "osl46"
			}
			if host == "" {
				host = "box"
			}
			path := machineFileAt(t, tt.content, tt.dirAtPath)
			f := newFakeHubs(hubA)
			if tt.setup != nil {
				tt.setup(f)
			}
			d := seams{path: path, noDirectory: tt.noDirectory, locateErr: tt.locateErr, host: host, hostErr: tt.hostErr}.deps(f)
			before := fileState(t, path)

			code, envelope := runName(t, d, t.TempDir(), arg)

			after := fileState(t, path)
			if len(tt.wantRefusal) > 0 {
				if code == 0 || envelope["ok"] != false {
					t.Fatalf("set exit %d, envelope %v; want a refusal", code, envelope)
				}
				message, _ := envelope["error"].(string)
				for _, want := range tt.wantRefusal {
					if !strings.Contains(message, want) {
						t.Errorf("refusal %q; want it to name %q", message, want)
					}
				}
				if after != before {
					t.Errorf("machine file after a refusal = %q; want it unchanged from %q", after, before)
				}
				if total := f.requestTotal(); total != 0 {
					t.Errorf("a refused set sent %d owner writes; want none", total)
				}
				return
			}
			if code != 0 || envelope["ok"] != true {
				t.Fatalf("set exit %d, envelope %v; want ok", code, envelope)
			}
			machine := machineOf(t, envelope)
			if written, _ := machine["written"].(bool); written != tt.wantWritten {
				t.Errorf("machine.written = %v; want %v", machine["written"], tt.wantWritten)
			}
			if tt.wantWritten {
				var file machinename.File
				if err := json.Unmarshal([]byte(strings.TrimPrefix(after, "file:")), &file); err != nil {
					t.Fatalf("decode the written file %q: %v", after, err)
				}
				if file.Name != arg || !slices.Equal(file.Names, tt.wantNames) {
					t.Errorf("written file = name %q, names %v; want %q, %v", file.Name, file.Names, arg, tt.wantNames)
				}
			} else if after != before {
				t.Errorf("machine file = %q; want it unchanged from %q", after, before)
			}
			if dropped, _ := machine["dropped_empty_entries"].(float64); int(dropped) != tt.wantDropped {
				t.Errorf("dropped empty entries = %v; want %d", machine["dropped_empty_entries"], tt.wantDropped)
			}
			if invalid, _ := machine["invalid_name_dropped"].(bool); invalid != tt.wantInvalidDropped {
				t.Errorf("invalid name dropped = %v; want %v", invalid, tt.wantInvalidDropped)
			}
			if lost, _ := machine["names_lost"].(bool); lost != tt.wantNamesLost {
				t.Errorf("names lost = %v; want %v", lost, tt.wantNamesLost)
			}
			requireSweepRequest(t, f, hubA, arg, tt.wantNames[:len(tt.wantNames)-1])
			requireRecoveries(t, envelope)
		})
	}

	t.Run("PullSkipOrErrorIsCheckedLocally", func(t *testing.T) {
		t.Parallel()
		f := newFakeHubs(hubA, hubB)
		f.skip[f.board(hubA)] = fabricengine.BoltSkip("dirty")
		f.pullErr[f.board(hubB)] = errors.New("pull boom")
		path := machineFileAt(t, "", false)

		code, envelope := runName(t, seams{path: path, host: "box"}.deps(f), t.TempDir(), "osl46")

		if code != 0 {
			t.Fatalf("set exit %d, envelope %v; want ok", code, envelope)
		}
		outcomes := hubOutcomes(t, envelope)
		if len(outcomes) != 2 || !strings.Contains(outcomes[0]["checked_locally"].(string), "skip detail") || outcomes[1]["checked_locally"] != "pull boom" {
			t.Errorf("hub outcomes = %v; want both hubs marked checked locally with their detail", outcomes)
		}
		for _, hub := range []string{hubA, hubB} {
			requireSweepRequest(t, f, hub, "osl46", []string{"box"})
		}
	})

	t.Run("UnpushedReadErrorIsNotedAndSwept", func(t *testing.T) {
		t.Parallel()
		f := newFakeHubs(hubA)
		f.unpushedErr[f.board(hubA)] = errors.New("unpushed boom")
		path := machineFileAt(t, "", false)

		code, envelope := runName(t, seams{path: path, host: "box"}.deps(f), t.TempDir(), "osl46")

		if code != 0 {
			t.Fatalf("set exit %d, envelope %v; want ok", code, envelope)
		}
		outcome := hubOutcomes(t, envelope)[0]
		if !strings.Contains(outcome["unpushed_read_note"].(string), "unpushed boom") || outcome["checked_locally"] != nil || outcome["outcome"] != outcomeNothing {
			t.Errorf("hub outcome = %v; want the unpushed error noted, not checked locally, and nothing to sweep", outcome)
		}
		requireSweepRequest(t, f, hubA, "osl46", []string{"box"})
	})

	t.Run("FailedHubIsReportedAndOthersSweep", func(t *testing.T) {
		t.Parallel()
		f := newFakeHubs(hubA, hubB)
		f.writeErr[f.board(hubA)] = errors.New("push boom; way forward: run lym board sync")
		f.write[f.board(hubB)] = boardengine.OwnerWrite{Recorded: []string{"x"}}
		path := machineFileAt(t, "", false)
		d := seams{path: path, host: "box"}.deps(f)

		code, envelope := runName(t, d, t.TempDir(), "osl46")

		if code == 0 || envelope["ok"] != false || !strings.Contains(envelope["error"].(string), "lym machine name osl46") {
			t.Fatalf("set exit %d, envelope %v; want a non-zero exit naming the re-run", code, envelope)
		}
		outcomes := hubOutcomes(t, envelope)
		if outcomes[0]["outcome"] != outcomeFailed || !strings.Contains(outcomes[0]["error"].(string), "push boom") ||
			!strings.Contains(outcomes[0]["way_forward"].(string), "lym machine name osl46") {
			t.Errorf("failed hub outcome = %v; want the write error and the re-run", outcomes[0])
		}
		if outcomes[1]["outcome"] != outcomeSwept {
			t.Errorf("second hub outcome = %v; want swept despite the first hub's failure", outcomes[1])
		}
		afterFirst := fileState(t, path)

		delete(f.writeErr, f.board(hubA))
		code, envelope = runName(t, d, t.TempDir(), "osl46")

		if code != 0 {
			t.Fatalf("re-run exit %d, envelope %v; want ok", code, envelope)
		}
		requests := f.requestsFor(hubA)
		if len(requests) != 2 || requests[1].Machine != "osl46" || !slices.Equal(requests[1].Replace, requests[0].Replace) {
			t.Errorf("hub requests = %+v; want the re-run to send the same Machine and Replace", requests)
		}
		if after := fileState(t, path); after != afterFirst {
			t.Errorf("machine file after the re-run = %q; want it unchanged from %q", after, afterFirst)
		}
	})

	t.Run("LostEntryIsReportedPerEntry", func(t *testing.T) {
		t.Parallel()
		f := newFakeHubs(hubA)
		f.write[f.board(hubA)] = boardengine.OwnerWrite{Recorded: []string{"s1"}, Lost: map[string]string{"s2": "far", "s3": "other"}}
		path := machineFileAt(t, "", false)

		code, envelope := runName(t, seams{path: path, host: "box"}.deps(f), t.TempDir(), "osl46")

		if code == 0 {
			t.Fatalf("set exit 0, envelope %v; want a non-zero exit for a lost entry", envelope)
		}
		lost := hubOutcomes(t, envelope)[0]["lost"].([]any)
		if len(lost) != 2 {
			t.Fatalf("lost entries = %v; want one per entry", lost)
		}
		for i, want := range []struct{ slug, owner string }{{"s2", "far"}, {"s3", "other"}} {
			entry := lost[i].(map[string]any)
			forward := entry["way_forward"].(string)
			if entry["slug"] != want.slug || entry["owner"] != want.owner || !strings.Contains(forward, "lym machine name osl46") || !strings.Contains(forward, "lym adopt") {
				t.Errorf("lost entry %d = %v; want %s under %s with the re-run and lym adopt", i, entry, want.slug, want.owner)
			}
		}
	})

	t.Run("UnpushedBeforeReportsPushed", func(t *testing.T) {
		t.Parallel()
		f := newFakeHubs(hubA)
		f.ahead[f.board(hubA)] = true
		path := machineFileAt(t, "", false)

		_, envelope := runName(t, seams{path: path, host: "box"}.deps(f), t.TempDir(), "osl46")

		if outcome := hubOutcomes(t, envelope)[0]["outcome"]; outcome != outcomePushed {
			t.Errorf("hub outcome = %v; want %q for a board that was ahead of origin", outcome, outcomePushed)
		}
	})

	t.Run("LostOnlyHubReportsKeptNotNothingToSweep", func(t *testing.T) {
		t.Parallel()
		f := newFakeHubs(hubA)
		f.ahead[f.board(hubA)] = true
		f.write[f.board(hubA)] = boardengine.OwnerWrite{Lost: map[string]string{"s2": "far"}}
		path := machineFileAt(t, "", false)

		code, envelope := runName(t, seams{path: path, host: "box"}.deps(f), t.TempDir(), "osl46")

		if code == 0 {
			t.Fatalf("set exit 0, envelope %v; want a non-zero exit for a lost entry", envelope)
		}
		if outcome := hubOutcomes(t, envelope)[0]["outcome"]; outcome != outcomeKept {
			t.Errorf("hub outcome = %v; want %q for a hub whose every selected entry stayed with another owner", outcome, outcomeKept)
		}
	})

	t.Run("MachineFileWriteFailureIsRefusedNamingTheFileAndTheRerun", func(t *testing.T) {
		t.Parallel()
		f := newFakeHubs(hubA)
		if runtime.GOOS == "windows" || os.Geteuid() == 0 {
			t.Skip("a read-only directory does not stop a write on this platform or user")
		}
		dir := t.TempDir()
		path := filepath.Join(dir, "machine.json")
		// The lock file exists already, so the read finds no machine file and the write is what fails.
		if err := os.WriteFile(path+".lock", nil, 0o644); err != nil {
			t.Fatalf("write the lock file: %v", err)
		}
		if err := os.Chmod(dir, 0o555); err != nil {
			t.Fatalf("make %s read-only: %v", dir, err)
		}
		t.Cleanup(func() { os.Chmod(dir, 0o755) })

		code, envelope := runName(t, seams{path: path, host: "box"}.deps(f), t.TempDir(), "osl46")

		if code == 0 || envelope["ok"] != false {
			t.Fatalf("set exit %d, envelope %v; want a refusal", code, envelope)
		}
		message, _ := envelope["error"].(string)
		for _, want := range []string{"write the machine file " + path, "lym machine name osl46"} {
			if !strings.Contains(message, want) {
				t.Errorf("refusal %q; want it to name %q", message, want)
			}
		}
		if total := f.requestTotal(); total != 0 {
			t.Errorf("a refused set sent %d owner writes; want none", total)
		}
	})

	t.Run("NothingRecordedReportsNothingToSweep", func(t *testing.T) {
		t.Parallel()
		f := newFakeHubs(hubA)
		path := machineFileAt(t, "", false)

		_, envelope := runName(t, seams{path: path, host: "box"}.deps(f), t.TempDir(), "osl46")

		if outcome := hubOutcomes(t, envelope)[0]["outcome"]; outcome != outcomeNothing {
			t.Errorf("hub outcome = %v; want %q", outcome, outcomeNothing)
		}
	})

	t.Run("CwdHubInTheRegistryIsSweptOnce", func(t *testing.T) {
		t.Parallel()
		f := newFakeHubs(hubA, hubB)
		f.cwd = hubB
		path := machineFileAt(t, "", false)

		_, envelope := runName(t, seams{path: path, host: "box"}.deps(f), t.TempDir(), "osl46")

		if len(hubOutcomes(t, envelope)) != 2 {
			t.Errorf("hub outcomes = %v; want each hub once", hubOutcomes(t, envelope))
		}
		requireSweepRequest(t, f, hubB, "osl46", []string{"box"})
	})

	t.Run("CwdHubOutsideTheRegistryIsSweptLast", func(t *testing.T) {
		t.Parallel()
		f := newFakeHubs(hubA)
		f.cwd = "/hubs/c"
		path := machineFileAt(t, "", false)

		_, envelope := runName(t, seams{path: path, host: "box"}.deps(f), t.TempDir(), "osl46")

		outcomes := hubOutcomes(t, envelope)
		if len(outcomes) != 2 || outcomes[0]["hub"] != hubA || outcomes[1]["hub"] != "/hubs/c" {
			t.Errorf("hub outcomes = %v; want the registry's hub, then the working directory's", outcomes)
		}
	})

	t.Run("CwdContainerDirectoryIsSwept", func(t *testing.T) {
		t.Parallel()
		f := newFakeHubs()
		container := t.TempDir()
		if err := os.MkdirAll(fabricengine.BoltDir(container), 0o755); err != nil {
			t.Fatalf("make the container's Bolt directory: %v", err)
		}
		if err := fabricengine.WriteShortname(fabricengine.BoltDir(container), "tst"); err != nil {
			t.Fatalf("record a shortname: %v", err)
		}
		d := seams{path: machineFileAt(t, "", false), host: "box"}.deps(f)
		d.cwdHub = hubOfDir

		code, envelope := runName(t, d, container, "osl46")

		if code != 0 {
			t.Fatalf("set exit %d, envelope %v; want ok", code, envelope)
		}
		requireSweepRequest(t, f, container, "osl46", []string{"box"})
	})

	t.Run("PlainCwdAddsNoHub", func(t *testing.T) {
		t.Parallel()
		f := newFakeHubs()
		d := seams{path: machineFileAt(t, "", false), host: "box"}.deps(f)
		d.cwdHub = hubOfDir

		code, envelope := runName(t, d, t.TempDir(), "osl46")

		if code != 0 || len(hubOutcomes(t, envelope)) != 0 {
			t.Errorf("set exit %d, hub outcomes %v; want ok with no hub swept", code, envelope["machine"])
		}
	})
}
