// start_test.go covers `lym machine start` through the command built over hand-made seams:
// hubs with owners on a fake board, a fake `lym beam start` child, recorded windows and a temporary desktop.

package machinecli

import (
	"bytes"
	"encoding/json"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/desktoplauncher"
	"github.com/Knatte18/loomyard/internal/dobbyengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubregistry"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/machinename"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/shell"
	"github.com/Knatte18/loomyard/internal/termwindow"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// The machines of the start fixtures: this machine and another one.
const (
	startHere = "here-m"
	startFar  = "far-m"
)

// startHereIdentity is this machine as the start fixtures read it.
var startHereIdentity = machinename.Identity{ID: "id-here", Name: startHere}

// startRegistry is the board registry of every start fixture.
var startRegistry = boardengine.Registry{"id-here": {Name: startHere}, "id-far": {Name: startFar}}

// startPrimeName is the prime worktree's name in every start fixture.
const startPrimeName = "main"

// hubFixture is one hub of a start fixture.
type hubFixture struct {
	shortname  string
	primeOwner string
	// beamOwners maps each local beam's slug to the owner it effectively reads as.
	beamOwners map[string]string
	holds      []boardengine.Hold
	// gone leaves the hub out of the machine record's live hubs.
	gone bool
	// startErr makes every `lym beam start` child in the hub fail as a whole.
	startErr error
	// entryErrors maps a worktree to the error its entry in the child's envelope carries.
	entryErrors map[string]string
	// dobbyOutcomes maps a worktree to the dobby outcome its entry in the child's envelope carries.
	dobbyOutcomes map[string]string
}

// owners returns the hub's board owners.
func (f hubFixture) owners() boardengine.Owners {
	entries := map[string]boardengine.OwnerEntry{}
	for slug, owner := range f.beamOwners {
		entries[slug] = boardengine.OwnerEntry{Owner: owner, IsProject: true, Beam: true}
	}
	return boardengine.Owners{Prime: f.primeOwner, Entries: entries, Registry: startRegistry, Holds: f.holds}
}

// startDeps returns the hub's start deps, its owners judged through the registry.
func (f hubFixture) startDeps() dobbyengine.StartDeps {
	return dobbyengine.StartDeps{
		PrimeName: startPrimeName,
		PrimeOwner: func() (string, bool, error) {
			return f.primeOwner, boardengine.OwnedElsewhere(f.primeOwner, startRegistry, startHereIdentity), nil
		},
		Beams: func() ([]dobbyengine.BeamPair, error) {
			var beams []dobbyengine.BeamPair
			for _, slug := range slices.Sorted(maps.Keys(f.beamOwners)) {
				owner := f.beamOwners[slug]
				beams = append(beams, dobbyengine.BeamPair{Slug: slug, Owner: owner, Elsewhere: boardengine.OwnedElsewhere(owner, startRegistry, startHereIdentity)})
			}
			return beams, nil
		},
	}
}

// terminalCall is one terminal window a start opened.
type terminalCall struct {
	dir   string
	title string
}

// startCalls records what a start did outside the envelope.
type startCalls struct {
	// beamStarts holds `<hub dir name>: <args>` per `lym beam start` child.
	beamStarts []string
	workspaces []string
	terminals  []terminalCall
}

// newStartDeps returns deps over fixtures under root, with a terminal launcher, and a freedesktop desktop in root.
func newStartDeps(t *testing.T, root string, fixtures ...hubFixture) (deps, *startCalls, desktoplauncher.Desktop) {
	t.Helper()
	desktop := desktoplauncher.Desktop{Dir: filepath.Join(root, "desktop"), Form: desktoplauncher.FormFreedesktop}
	if err := os.MkdirAll(desktop.Dir, 0o755); err != nil {
		t.Fatal(err)
	}
	calls := &startCalls{}
	byHub := map[string]hubFixture{}
	byBoard := map[string]hubFixture{}
	live := []hubregistry.Entry{}
	for _, f := range fixtures {
		hub := filepath.Join(root, f.shortname+"-LYMHUB")
		byHub[hub], byBoard[fabricengine.BoardDir(hub)] = f, f
		if !f.gone {
			live = append(live, hubregistry.Entry{HubPath: hub, Shortname: f.shortname})
		}
	}
	d := deps{
		liveHubEntries: func() ([]hubregistry.Entry, error) { return live, nil },
		lookupHub: func(shortname string) (string, error) {
			for _, entry := range live {
				if entry.Shortname == shortname {
					return entry.HubPath, nil
				}
			}
			return "", errors.New("the machine record holds no hub under " + shortname)
		},
		pull:       func(string) (fabricengine.BoltSkip, string, error) { return "", "", nil },
		readOwners: func(boardDir string) (boardengine.Owners, error) { return byBoard[boardDir].owners(), nil },
		identity:   func() (machinename.Identity, error) { return startHereIdentity, nil },
		hubStart: func(hub string) (*lymcwd.Location, dobbyengine.StartDeps, error) {
			prime, err := lymcwd.NamedWorktree(hub, startPrimeName)
			return prime, byHub[hub].startDeps(), err
		},
		lymBinary: func() (string, error) { return "/opt/lym", nil },
		beamStart: func(_, dir string, args []string) ([]beamStartEntry, error) {
			hub := filepath.Dir(dir)
			calls.beamStarts = append(calls.beamStarts, filepath.Base(hub)+": "+strings.Join(args, " "))
			f := byHub[hub]
			if f.startErr != nil {
				return nil, f.startErr
			}
			entries := []beamStartEntry{{Worktree: args[2]}}
			if args[2] == "--all" {
				entries = []beamStartEntry{{Worktree: dobbyengine.PrimeLabel}}
				for _, slug := range slices.Sorted(maps.Keys(f.beamOwners)) {
					if f.beamOwners[slug] == startHere {
						entries = append(entries, beamStartEntry{Worktree: slug})
					}
				}
			}
			for i := range entries {
				worktree := entries[i].Worktree
				entries[i].Error = f.entryErrors[worktree]
				if outcome := f.dobbyOutcomes[worktree]; outcome != "" {
					entries[i].Outcome, entries[i].Detail = outcome, "the dobby could not take the runs up"
					entries[i].Warnings = []string{"the dobby was not started (the dobby could not take the runs up)"}
				}
			}
			return entries, nil
		},
		openWorkspace: func(path string) error { calls.workspaces = append(calls.workspaces, path); return nil },
		terminal: func() (func(dir, title, lymPath string) error, error) {
			return func(dir, title, _ string) error {
				calls.terminals = append(calls.terminals, terminalCall{dir: dir, title: title})
				return nil
			}, nil
		},
		desktop: func() (desktoplauncher.Desktop, error) { return desktop, nil },
	}
	return d, calls, desktop
}

// wantTerminal returns the window a start opens for the worktree named name in the hub of shortname under root.
func wantTerminal(t *testing.T, root, shortname, name string) terminalCall {
	t.Helper()
	hub := filepath.Join(root, shortname+"-LYMHUB")
	location, err := lymcwd.NamedWorktree(hub, name)
	if err != nil {
		t.Fatal(err)
	}
	return terminalCall{dir: location.AnchorPath(), title: boardengine.ConsoleWindowTitle(hub, startPrimeName, name)}
}

// wantWorkspace returns the workspace file a start opens for the hub of shortname under root.
func wantWorkspace(t *testing.T, root, shortname string) string {
	t.Helper()
	prime, err := lymcwd.NamedWorktree(filepath.Join(root, shortname+"-LYMHUB"), startPrimeName)
	if err != nil {
		t.Fatal(err)
	}
	return fabricengine.HubWorkspacePath(prime)
}

// runStartRaw runs `lym machine start` with args over d.
// It returns the exit code and the output.
func runStartRaw(t *testing.T, d deps, args ...string) (int, []byte) {
	t.Helper()
	var out bytes.Buffer
	code := clihelp.ExecuteIn(newCommand(d), t.TempDir(), &out, append([]string{"start"}, args...))
	return code, out.Bytes()
}

// decodeStart decodes the envelope of a start into its result and its error message.
func decodeStart(t *testing.T, raw []byte) (startResult, string) {
	t.Helper()
	var envelope struct {
		Start startResult `json:"start"`
		Error string      `json:"error"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("decode envelope %q: %v", raw, err)
	}
	return envelope.Start, envelope.Error
}

// startSummary renders the output of a start as the readable summary a terminal prints.
func startSummary(t *testing.T, raw []byte) string {
	t.Helper()
	summary, ok := output.Summary(bytes.TrimSpace(raw))
	if !ok {
		t.Fatalf("output.Summary(%q) = not an envelope", raw)
	}
	return summary
}

// runStartVerb runs `lym machine start` with args over d.
// It returns the exit code, the decoded result and the error message.
func runStartVerb(t *testing.T, d deps, args ...string) (int, startResult, string) {
	t.Helper()
	code, raw := runStartRaw(t, d, args...)
	result, message := decodeStart(t, raw)
	return code, result, message
}

// worktreesOf returns the worktrees of the hub of shortname.
func worktreesOf(result startResult, shortname string) []startWorktree {
	var worktrees []startWorktree
	for _, worktree := range result.Worktrees {
		if worktree.Shortname == shortname {
			worktrees = append(worktrees, worktree)
		}
	}
	return worktrees
}

// outcomes maps each of worktrees to its outcome.
func outcomes(worktrees []startWorktree) map[string]string {
	got := map[string]string{}
	for _, worktree := range worktrees {
		got[worktree.Worktree] = worktree.Outcome
	}
	return got
}

// detailOf returns the detail of the worktree named worktree in worktrees.
func detailOf(worktrees []startWorktree, worktree string) string {
	for _, w := range worktrees {
		if w.Worktree == worktree {
			return w.Detail
		}
	}
	return ""
}

func TestStart_StartsOnlyWhatThisMachineOwns(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name           string
		hub            hubFixture
		wantCode       int
		wantBeamStarts []string
		wantOutcomes   map[string]string
		// wantDetails maps a worktree to a substring its detail must carry.
		wantDetails map[string]string
		// wantWarning is a substring one of the hub's warnings must carry.
		wantWarning string
		// wantTerminals names the worktrees a terminal window opens for, in order.
		wantTerminals []string
		wantWorkspace bool
	}{
		{
			name:           "PrimeOwnedHereStartsTheHubWithAll",
			hub:            hubFixture{primeOwner: startHere, beamOwners: map[string]string{"pay": startHere, "ops": startFar}},
			wantBeamStarts: []string{"ly-LYMHUB: beam start --all"},
			wantOutcomes:   map[string]string{"prime": startStarted, "pay": startStarted, "ops": startSkipped},
			wantDetails:    map[string]string{"ops": "owned by far-m"},
			wantTerminals:  []string{startPrimeName, "pay"},
			wantWorkspace:  true,
		},
		{
			name:           "PrimeOwnedElsewhereStartsTheOwnedBeamsBySlugWithoutClaimingThePrime",
			hub:            hubFixture{primeOwner: startFar, beamOwners: map[string]string{"pay": startHere, "web": startHere}},
			wantBeamStarts: []string{"ly-LYMHUB: beam start pay --no-prime-claim", "ly-LYMHUB: beam start web --no-prime-claim"},
			wantOutcomes:   map[string]string{"prime": startSkipped, "pay": startStarted, "web": startStarted},
			wantDetails:    map[string]string{"prime": "owned by far-m; way forward: lym adopt root"},
			wantTerminals:  []string{"pay", "web"},
			wantWorkspace:  true,
		},
		{
			name:           "PrimeWithNoOwnerIsNeitherClaimedNorStartedAndItsBeamsStartWithoutClaimingIt",
			hub:            hubFixture{beamOwners: map[string]string{"pay": startHere, "loose": ""}},
			wantBeamStarts: []string{"ly-LYMHUB: beam start pay --no-prime-claim"},
			wantOutcomes:   map[string]string{"prime": startSkipped, "pay": startStarted, "loose": startSkipped},
			wantDetails:    map[string]string{"prime": "no owner is recorded", "loose": "no owner is recorded"},
			wantTerminals:  []string{"pay"},
			wantWorkspace:  true,
		},
		{
			name:         "NothingOwnedStartsAndOpensNothing",
			hub:          hubFixture{primeOwner: startFar},
			wantOutcomes: map[string]string{"prime": startSkipped},
		},
		{
			name:           "AFailedBeamStartFailsTheVerb",
			hub:            hubFixture{primeOwner: startHere, startErr: errors.New("lym beam start --all: origin is unreachable")},
			wantCode:       1,
			wantBeamStarts: []string{"ly-LYMHUB: beam start --all"},
			wantOutcomes:   map[string]string{"prime": startFailed},
			wantDetails:    map[string]string{"prime": "origin is unreachable"},
		},
		{
			name:           "AnEntryErrorFailsThatWorktreeAlone",
			hub:            hubFixture{primeOwner: startHere, beamOwners: map[string]string{"pay": startHere}, entryErrors: map[string]string{"pay": "the pair is gone"}},
			wantCode:       1,
			wantBeamStarts: []string{"ly-LYMHUB: beam start --all"},
			wantOutcomes:   map[string]string{"prime": startStarted, "pay": startFailed},
			wantDetails:    map[string]string{"pay": "the pair is gone"},
			wantTerminals:  []string{startPrimeName},
			wantWorkspace:  true,
		},
		{
			name:           "AWorktreeWhoseDobbyFailedIsFailedAndItsWarningsAreKept",
			hub:            hubFixture{primeOwner: startHere, beamOwners: map[string]string{"pay": startHere}, dobbyOutcomes: map[string]string{"pay": dobbyengine.OutcomeFailed}},
			wantCode:       1,
			wantBeamStarts: []string{"ly-LYMHUB: beam start --all"},
			wantOutcomes:   map[string]string{"prime": startStarted, "pay": startFailed},
			wantDetails:    map[string]string{"pay": "the dobby did not start (failed): the dobby could not take the runs up"},
			wantWarning:    "pay: the dobby was not started",
			wantTerminals:  []string{startPrimeName},
			wantWorkspace:  true,
		},
		{
			name:           "AWorktreeWhoseDobbyWasRefusedIsFailed",
			hub:            hubFixture{primeOwner: startHere, dobbyOutcomes: map[string]string{"prime": dobbyengine.OutcomeRefused}},
			wantCode:       1,
			wantBeamStarts: []string{"ly-LYMHUB: beam start --all"},
			wantOutcomes:   map[string]string{"prime": startFailed},
			wantDetails:    map[string]string{"prime": "the dobby did not start (refused)"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tt.hub.shortname = "ly"
			root := t.TempDir()
			d, calls, _ := newStartDeps(t, root, tt.hub)

			code, result, _ := runStartVerb(t, d)

			if code != tt.wantCode {
				t.Errorf("start exit = %d; want %d", code, tt.wantCode)
			}
			if !slices.Equal(calls.beamStarts, tt.wantBeamStarts) {
				t.Errorf("beam starts = %q; want %q", calls.beamStarts, tt.wantBeamStarts)
			}
			if len(result.Hubs) != 1 {
				t.Fatalf("hubs = %+v; want one", result.Hubs)
			}
			worktrees := worktreesOf(result, "ly")
			if got := outcomes(worktrees); !maps.Equal(got, tt.wantOutcomes) {
				t.Errorf("outcomes = %v; want %v", got, tt.wantOutcomes)
			}
			for worktree, want := range tt.wantDetails {
				if got := detailOf(worktrees, worktree); !strings.Contains(got, want) {
					t.Errorf("detail of %s = %q; want it to carry %q", worktree, got, want)
				}
			}
			if tt.wantWarning != "" && !slices.ContainsFunc(result.Hubs[0].Warnings, func(w string) bool { return strings.Contains(w, tt.wantWarning) }) {
				t.Errorf("warnings = %q; want one carrying %q", result.Hubs[0].Warnings, tt.wantWarning)
			}
			var wantTerminals []terminalCall
			for _, name := range tt.wantTerminals {
				wantTerminals = append(wantTerminals, wantTerminal(t, root, "ly", name))
			}
			if !slices.Equal(calls.terminals, wantTerminals) {
				t.Errorf("terminals = %+v; want %+v", calls.terminals, wantTerminals)
			}
			var wantWorkspaces []string
			if tt.wantWorkspace {
				wantWorkspaces = []string{wantWorkspace(t, root, "ly")}
			}
			if !slices.Equal(calls.workspaces, wantWorkspaces) {
				t.Errorf("workspaces = %q; want %q", calls.workspaces, wantWorkspaces)
			}
		})
	}
}

func TestStart_WindowAndHubFailuresLeaveTheRestStarted(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	ownedPrime := hubFixture{shortname: "ly", primeOwner: startHere}
	tests := []struct {
		name     string
		hubs     []hubFixture
		mutate   func(d *deps)
		wantCode int
		check    func(t *testing.T, result startResult, calls *startCalls)
	}{
		{
			name: "ATerminalThatDoesNotOpenCarriesTheAttachCommand",
			hubs: []hubFixture{ownedPrime},
			mutate: func(d *deps) {
				d.terminal = func() (func(dir, title, lymPath string) error, error) {
					return func(string, string, string) error { return errors.New("no display") }, nil
				}
			},
			check: func(t *testing.T, result startResult, calls *startCalls) {
				worktrees := worktreesOf(result, "ly")
				if len(worktrees) != 1 || worktrees[0].Outcome != startStarted || !strings.HasSuffix(worktrees[0].Attach, "reed attach") {
					t.Errorf("worktrees = %+v; want the prime started and carrying its attach command", worktrees)
				}
				if warnings := result.Hubs[0].Warnings; len(warnings) != 1 || !strings.Contains(warnings[0], "terminal window of prime did not open: no display") {
					t.Errorf("warnings = %q; want the terminal failure named", warnings)
				}
			},
		},
		{
			name: "AWorkspaceThatDoesNotOpenIsAWarningAndTheTerminalsOpen",
			hubs: []hubFixture{ownedPrime},
			mutate: func(d *deps) {
				d.openWorkspace = func(string) error { return errors.New("code is not installed") }
			},
			check: func(t *testing.T, result startResult, calls *startCalls) {
				hub := result.Hubs[0]
				if hub.Workspace != "" || len(hub.Warnings) != 1 || !strings.Contains(hub.Warnings[0], "code is not installed") {
					t.Errorf("hub = %+v; want no workspace and the failure as its one warning", hub)
				}
				if len(calls.terminals) != 1 {
					t.Errorf("terminals = %+v; want the terminal opened", calls.terminals)
				}
			},
		},
		{
			name:     "AHubWhoseChildFailsDoesNotStopTheNextHub",
			hubs:     []hubFixture{{shortname: "ly", primeOwner: startHere, startErr: errors.New("origin is unreachable")}, {shortname: "mx", primeOwner: startHere}},
			wantCode: 1,
			check: func(t *testing.T, result startResult, calls *startCalls) {
				if got := outcomes(worktreesOf(result, "ly")); !maps.Equal(got, map[string]string{"prime": startFailed}) {
					t.Errorf("outcomes of ly = %v; want the prime failed", got)
				}
				if got := outcomes(worktreesOf(result, "mx")); !maps.Equal(got, map[string]string{"prime": startStarted}) {
					t.Errorf("outcomes of mx = %v; want the prime started", got)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			d, calls, _ := newStartDeps(t, t.TempDir(), tt.hubs...)
			if tt.mutate != nil {
				tt.mutate(&d)
			}

			code, result, _ := runStartVerb(t, d)

			if code != tt.wantCode {
				t.Errorf("start exit = %d; want %d", code, tt.wantCode)
			}
			tt.check(t, result, calls)
		})
	}
}

func TestStart_NamedHubStartsThatHubAlone(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	d, calls, _ := newStartDeps(t, t.TempDir(),
		hubFixture{shortname: "ly", primeOwner: startHere},
		hubFixture{shortname: "mx", primeOwner: startHere},
	)

	code, result, _ := runStartVerb(t, d, "mx")
	if code != 0 || len(result.Hubs) != 1 || result.Hubs[0].Shortname != "mx" || !slices.Equal(calls.beamStarts, []string{"mx-LYMHUB: beam start --all"}) {
		t.Errorf("start mx = exit %d, hubs %+v, beam starts %q; want mx alone started", code, result.Hubs, calls.beamStarts)
	}

	code, _, message := runStartVerb(t, d, "zz")
	if code == 0 || !strings.Contains(message, "lym machine start: no live hub reads the shortname zz") ||
		!strings.Contains(message, "way forward: run lym machine start with no argument") || !strings.Contains(message, "the machine record holds no hub under zz") {
		t.Errorf("start zz = exit %d, error %q; want the verb's refusal naming the shortname, its way forward and the cause", code, message)
	}
}

func TestStart_PrintsAStandingHold(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	subjects := []boardengine.HoldSubject{{Subject: "root"}, {Subject: "pay"}}

	tests := []struct {
		name string
		hold boardengine.Hold
		want startHold
		// wantSummary is the hold as the readable summary prints it.
		wantSummary string
	}{
		{
			name:        "ThisMachinesHoldCarriesTheRerun",
			hold:        boardengine.Hold{Kind: boardengine.HoldKindAdopt, Machine: startHere, Subjects: subjects},
			want:        startHold{Machine: startHere, Subjects: []string{"root", "pay"}, Rerun: "lym adopt root pay"},
			wantSummary: "hold: here-m holds root, pay; re-run: lym adopt root pay",
		},
		{
			name:        "AnotherMachinesHoldCarriesNone",
			hold:        boardengine.Hold{Kind: boardengine.HoldKindAdopt, Machine: startFar, Subjects: subjects},
			want:        startHold{Machine: startFar, Subjects: []string{"root", "pay"}},
			wantSummary: "hold: far-m holds root, pay\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			d, _, _ := newStartDeps(t, t.TempDir(), hubFixture{shortname: "ly", primeOwner: startHere, holds: []boardengine.Hold{tt.hold}})

			_, raw := runStartRaw(t, d)

			result, _ := decodeStart(t, raw)
			if len(result.Hubs) != 1 || result.Hubs[0].Hold == nil {
				t.Fatalf("hubs = %+v; want one hub with its hold", result.Hubs)
			}
			got := *result.Hubs[0].Hold
			if got.Machine != tt.want.Machine || got.Rerun != tt.want.Rerun || !slices.Equal(got.Subjects, tt.want.Subjects) {
				t.Errorf("hold = %+v; want %+v", got, tt.want)
			}
			if summary := startSummary(t, raw); !strings.Contains(summary, tt.wantSummary) {
				t.Errorf("summary = %q; want it to carry %q", summary, tt.wantSummary)
			}
		})
	}
}

func TestStart_PrintsAttachCommandsWhereNoTerminalLauncherResolves(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	d, calls, _ := newStartDeps(t, t.TempDir(), hubFixture{shortname: "ly", primeOwner: startHere, beamOwners: map[string]string{"pay": startHere}})
	d.terminal = func() (func(dir, title, lymPath string) error, error) {
		return nil, termwindow.ErrNoLauncher
	}

	code, raw := runStartRaw(t, d)

	result, _ := decodeStart(t, raw)
	if code != 0 || len(result.Hubs) != 1 || len(result.Worktrees) != 2 {
		t.Fatalf("start = exit %d, hubs %+v, worktrees %+v; want ok with one hub and its two worktrees", code, result.Hubs, result.Worktrees)
	}
	summary := startSummary(t, raw)
	for _, worktree := range result.Worktrees {
		name := worktree.Worktree
		if name == dobbyengine.PrimeLabel {
			name = startPrimeName
		}
		if !strings.Contains(worktree.Attach, name) || !strings.HasSuffix(worktree.Attach, shell.ForGOOS().Invoke("/opt/lym")+" reed attach") {
			t.Errorf("attach of %s = %q; want a cd into its worktree and the lym reed attach", worktree.Worktree, worktree.Attach)
		}
		if !strings.Contains(summary, "ly/"+worktree.Worktree+": started; attach: "+worktree.Attach) {
			t.Errorf("summary = %q; want it to carry the attach command of %s", summary, worktree.Worktree)
		}
	}
	if len(calls.terminals) != 0 || len(result.Warnings) == 0 {
		t.Errorf("terminals %+v, warnings %q; want no window and a warning naming why", calls.terminals, result.Warnings)
	}
}

func TestStart_KeepsTheDesktopLaunchers(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name string
		hubs []hubFixture
		// before are the hubs whose launcher stands before the start.
		before        []string
		wantLaunchers []string
		wantAllHubs   bool
		wantRemoved   []string
	}{
		{
			name: "KeepsAnOwnedHubAndRemovesAnUnownedAndAGoneOne",
			hubs: []hubFixture{
				{shortname: "ly", primeOwner: startFar, beamOwners: map[string]string{"pay": startHere}},
				{shortname: "mx", primeOwner: startFar},
				{shortname: "old", primeOwner: startHere, gone: true},
			},
			before:        []string{"mx", "old"},
			wantLaunchers: []string{"ly"},
			wantAllHubs:   true,
			wantRemoved:   []string{"old"},
		},
		{
			name:   "TheAllHubsLauncherGoesWithTheLastHub",
			hubs:   []hubFixture{{shortname: "mx", primeOwner: startFar}},
			before: []string{"mx"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			d, _, desktop := newStartDeps(t, t.TempDir(), tt.hubs...)
			for _, shortname := range tt.before {
				if err := desktop.Add("/opt/lym", shortname); err != nil {
					t.Fatal(err)
				}
			}

			if code, result, _ := runStartVerb(t, d); code != 0 || !slices.Equal(result.LaunchersRemoved, tt.wantRemoved) {
				t.Errorf("start = exit %d, launchers removed %q; want ok and %q", code, result.LaunchersRemoved, tt.wantRemoved)
			}

			if got, err := desktop.Shortnames(); err != nil || !slices.Equal(got, tt.wantLaunchers) {
				t.Errorf("launchers after the start = %q, %v; want %q", got, err, tt.wantLaunchers)
			}
			_, err := os.Stat(filepath.Join(desktop.Dir, "lym-hubs.desktop"))
			if got := err == nil; got != tt.wantAllHubs {
				t.Errorf("all-hubs launcher stands = %v; want %v", got, tt.wantAllHubs)
			}
		})
	}
}

func TestParseBeamStartOutput_ReadsTheChildsEnvelope(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name    string
		stdout  string
		stderr  string
		runErr  error
		want    []beamStartEntry
		wantErr string
	}{
		{
			name:   "ASuccessEnvelopeGivesItsEntries",
			stdout: `{"ok":true,"result":{"beams":[{"worktree":"prime"},{"worktree":"pay","outcome":"started","warnings":["a note"]}]}}` + "\n",
			want:   []beamStartEntry{{Worktree: "prime"}, {Worktree: "pay", Outcome: "started", Warnings: []string{"a note"}}},
		},
		{
			name:   "APartialFailureEnvelopeGivesItsEntriesAlongsideItsError",
			stdout: "a log line\n" + `{"ok":false,"error":"lym beam start: pay: boom","result":{"beams":[{"worktree":"pay","error":"boom"}]}}` + "\n",
			runErr: errors.New("exit status 1"),
			want:   []beamStartEntry{{Worktree: "pay", Error: "boom"}},
		},
		{
			name:    "ARefusalEnvelopeWithNoResultIsItsError",
			stdout:  `{"ok":false,"error":"lym beam start: no pair here; way forward: lym beam create"}` + "\n",
			runErr:  errors.New("exit status 1"),
			wantErr: "lym beam start: no pair here; way forward: lym beam create",
		},
		{
			name:    "OutputWithNoJSONLineIsNoResult",
			stdout:  "not an envelope\n",
			wantErr: "it printed no result: unexpected end of JSON input",
		},
		{
			name:    "EmptyOutputReadsTheErrorOutput",
			stderr:  "panic: boom\n",
			runErr:  errors.New("exit status 2"),
			wantErr: "panic: boom",
		},
		{
			name:    "EmptyOutputReadsTheRunError",
			runErr:  errors.New("fork/exec /opt/lym: no such file or directory"),
			wantErr: "fork/exec /opt/lym: no such file or directory",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseBeamStartOutput([]byte(tt.stdout), []byte(tt.stderr), tt.runErr)

			if gotErr := errString(err); gotErr != tt.wantErr {
				t.Errorf("parseBeamStartOutput() error = %q; want %q", gotErr, tt.wantErr)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseBeamStartOutput() = %+v; want %+v", got, tt.want)
			}
		})
	}
}

// errString returns the message of err, or the empty string for nil.
func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
