// hubgeom_test.go tests ReedGeometry and BurlerGeometry, guarding the anchor path and worktree root against a swap.
// A swap goes uncaught where the two coincide, so each fixture keeps hub, worktree root and anchor path apart.
// RepoName also differs from every basename, so a field mix-up surfaces instead of passing silently.

package hubgeom

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/creelengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubreconcile"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/machinename"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/slackengine"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestReedGeometry pins every ReedGeometry field against a fixture whose hub, worktree root and anchor path differ.
// The prime gets no slug, and a task worktree gets its own name as the slug.
//
//testtiming:keep pins the field-by-field geometry of a prime and of an anchored task worktree, which the integration tests covering its blocks only see through a real hub
func TestReedGeometry(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name      string
		anchorRel string
		isPrime   bool
		wantSlug  string
	}{
		{"subpath-anchored fixture", filepath.Join("sub", "dir"), false, "some-worktree"},
		{"unanchored fixture", ".", false, "some-worktree"},
		{"prime fixture", ".", true, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			hub := filepath.Join(root, "some-hub-LYMHUB")
			worktreeName := "some-worktree"
			worktreeRoot := filepath.Join(hub, worktreeName)
			anchorPath := filepath.Join(worktreeRoot, tt.anchorRel)

			l := &lymcwd.Location{
				RepoName:     "distinct-repo-name",
				HubPath:      hub,
				WorktreeName: worktreeName,
				AnchorRel:    tt.anchorRel,
			}

			got := reedGeometry(l, tt.isPrime)

			if got.SpawnOrder == nil {
				t.Error("ReedGeometry(l).SpawnOrder = nil, want the hub's spawn-order teller")
			}

			if got.NameSlug != tt.wantSlug {
				t.Errorf("ReedGeometry(l).NameSlug = %q; want %q", got.NameSlug, tt.wantSlug)
			}

			if want := reedengine.ServerName(hub); got.SocketKey != want {
				t.Errorf("ReedGeometry(l).SocketKey = %q; want %q (ServerName(hub))", got.SocketKey, want)
			}
			if want := reedengine.SessionName(worktreeRoot); got.SessionName != want {
				t.Errorf("ReedGeometry(l).SessionName = %q; want %q (SessionName(worktreeRoot))", got.SessionName, want)
			}
			if got.AnchorPath != anchorPath {
				t.Errorf("ReedGeometry(l).AnchorPath = %q; want %q", got.AnchorPath, anchorPath)
			}
			if got.PaneCwd != l.AnchorPath() {
				t.Errorf("ReedGeometry(l).PaneCwd = %q; want %q (l.AnchorPath())", got.PaneCwd, l.AnchorPath())
			}
			if tt.anchorRel != "." && got.PaneCwd == worktreeRoot {
				// Only a subpath-anchored fixture catches a change that points the spawn sites at WorktreeRoot:
				// the two coincide only when AnchorRel is ".".
				t.Errorf("ReedGeometry(l).PaneCwd = %q; want != WorktreeRoot %q", got.PaneCwd, worktreeRoot)
			}
			if got.WorktreeRoot != worktreeRoot {
				t.Errorf("ReedGeometry(l).WorktreeRoot = %q; want %q", got.WorktreeRoot, worktreeRoot)
			}
			if want := fabricengine.HubLogsDir(hub); got.LogsDir != want {
				t.Errorf("ReedGeometry(l).LogsDir = %q; want %q", got.LogsDir, want)
			}
			if want := filepath.Join(fabricengine.HubScratchDir(hub), reedengine.DiscoverSignalFileName); got.DiscoverSignalPath != want {
				t.Errorf("ReedGeometry(l).DiscoverSignalPath = %q; want %q", got.DiscoverSignalPath, want)
			}
			if got.WorktreeName != l.WorktreeName {
				t.Errorf("ReedGeometry(l).WorktreeName = %q; want %q", got.WorktreeName, l.WorktreeName)
			}
			if got.HubPath != hub {
				t.Errorf("ReedGeometry(l).HubPath = %q; want %q", got.HubPath, hub)
			}

			pause := pauseGeometry(l, tt.isPrime)
			if got.PauseRecordPath != pause.RecordPath {
				t.Errorf("ReedGeometry(l).PauseRecordPath = %q; want %q (the pause geometry's record path)", got.PauseRecordPath, pause.RecordPath)
			}
			if want := filepath.Join(fabricengine.HubScratchDir(hub), lymdirs.PauseDirName, pauserecord.RecordFileName); pause.RecordPath != want {
				t.Errorf("pauseGeometry(l).RecordPath = %q; want %q", pause.RecordPath, want)
			}
			if pause.Slug != tt.wantSlug {
				t.Errorf("pauseGeometry(l).Slug = %q; want %q", pause.Slug, tt.wantSlug)
			}

			if got.Buttons == nil {
				t.Fatal("ReedGeometry(l).Buttons = nil, want the button-context teller")
			}
			buttons, err := got.Buttons()
			if err != nil {
				t.Fatalf("ReedGeometry(l).Buttons() error = %v; want nil", err)
			}
			wantPlace := reedengine.ButtonPlaceTask
			if tt.isPrime {
				wantPlace = reedengine.ButtonPlacePrime
			}
			if buttons.Place != wantPlace {
				t.Errorf("Buttons().Place = %q; want %q", buttons.Place, wantPlace)
			}
			// Buttons().Worktree is the anchor path, not the worktree root; only a subpath-anchored fixture tells them apart.
			if buttons.Worktree != anchorPath {
				t.Errorf("Buttons().Worktree = %q; want %q (the anchor path)", buttons.Worktree, anchorPath)
			}
			if buttons.Slug != worktreeName {
				t.Errorf("Buttons().Slug = %q; want %q", buttons.Slug, worktreeName)
			}
			if want := reedengine.SessionName(worktreeRoot); buttons.Session != want {
				t.Errorf("Buttons().Session = %q; want %q", buttons.Session, want)
			}
			if tt.isPrime {
				beam, err := buttons.Beam()
				if err != nil || beam.Anchor != anchorPath {
					t.Errorf("Buttons().Beam() = %+v, %v; want the anchor path %q with no error", beam, err, anchorPath)
				}
			}
		})
	}
}

// TestReedGeometry_ButtonContext pins how the button-context teller reads the place and the machine's buttons.
// The place comes from the pair's origin record, and the buttons come from the desktop machine layer.
func TestReedGeometry_ButtonContext(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name      string
		record    string
		wantPlace reedengine.ButtonPlace
		wantErr   bool
	}{
		{"BeamMarkReadsBeam", `{"parent_branch":"main","beam":true}`, reedengine.ButtonPlaceBeam, false},
		{"NoBeamMarkReadsTask", `{"parent_branch":"main"}`, reedengine.ButtonPlaceTask, false},
		{"UnparseableRecordIsAnError", `{`, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			l := hubWithOrigin(t, tt.record)

			buttons, err := reedGeometry(l, false).Buttons()
			if tt.wantErr {
				if err == nil {
					t.Fatal("Buttons() error = nil; want the origin read error")
				}
				return
			}
			if err != nil {
				t.Fatalf("Buttons() error = %v; want nil", err)
			}
			if buttons.Place != tt.wantPlace {
				t.Errorf("Buttons().Place = %q; want %q", buttons.Place, tt.wantPlace)
			}
		})
	}

	// writeMachineButtons gives l's hub a Bolt checkout whose desktop machine layer holds contents.
	// It also gives l's anchor a _lym directory.
	writeMachineButtons := func(t *testing.T, contents string) *lymcwd.Location {
		t.Helper()
		l := hubWithOrigin(t, `{"parent_branch":"main"}`)
		bolt := fabricengine.BoltDir(l.HubPath)
		for _, dir := range []string{configengine.ScratchDir(bolt), filepath.Join(l.AnchorPath(), lymdirs.LymDirName)} {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatalf("MkdirAll(%q) = %v; want nil", dir, err)
			}
		}
		if err := os.WriteFile(configengine.MachineFile(bolt, "desktop"), []byte(contents), 0o644); err != nil {
			t.Fatalf("write machine file: %v", err)
		}
		return l
	}

	t.Run("MachineButtonsAreTold", func(t *testing.T) {
		t.Parallel()
		l := writeMachineButtons(t, "buttons: {task: [{label: x, run: y}]}\n")

		buttons, err := reedGeometry(l, false).Buttons()
		if err != nil {
			t.Fatalf("Buttons() error = %v; want nil", err)
		}
		var task struct {
			Task []struct {
				Label string `yaml:"label"`
				Run   string `yaml:"run"`
			} `yaml:"task"`
		}
		if err := buttons.MachineButtons.Decode(&task); err != nil {
			t.Fatalf("decode MachineButtons = %v; want nil", err)
		}
		if len(task.Task) != 1 || task.Task[0].Label != "x" || task.Task[0].Run != "y" {
			t.Errorf("MachineButtons task = %+v; want the one entry x/y", task.Task)
		}
	})

	// This subtest reads the process-global logger output, so it does not run in parallel.
	t.Run("RefusedMachineLayerLeavesTheNodeZeroAndLogsAWarn", func(t *testing.T) {
		logs := logcapture.CaptureVerbose(t)
		l := writeMachineButtons(t, "buttons: {task: {}}\n")

		buttons, err := reedGeometry(l, false).Buttons()
		if err != nil {
			t.Fatalf("Buttons() error = %v; want nil", err)
		}
		if buttons.MachineButtons.Kind != 0 {
			t.Errorf("MachineButtons kind = %d; want the zero node", buttons.MachineButtons.Kind)
		}
		if buttons.Place != reedengine.ButtonPlaceTask {
			t.Errorf("Buttons().Place = %q; want %q, intact", buttons.Place, reedengine.ButtonPlaceTask)
		}
		if !strings.Contains(logs.String(), "desktop config") {
			t.Errorf("log = %q; want a Warn naming the desktop config", logs.String())
		}
	})
}

// TestBurlerGeometry pins BurlerGeometry's roots against a subpath-anchored fixture.
// The fixture's hub, worktree root and anchor path are all distinct.
//
//testtiming:keep pins that WorktreeRoot is the anchor path while RepoRoot stays the worktree path, which the origin-record test covering its blocks never reads
func TestBurlerGeometry(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	root := t.TempDir()
	hub := filepath.Join(root, "some-hub-LYMHUB")
	worktreeName := "some-worktree"
	worktreeRoot := filepath.Join(hub, worktreeName)
	anchorRel := filepath.Join("sub", "dir")
	anchorPath := filepath.Join(worktreeRoot, anchorRel)

	l := &lymcwd.Location{
		RepoName:     "distinct-repo-name",
		HubPath:      hub,
		WorktreeName: worktreeName,
		AnchorRel:    anchorRel,
	}

	boardDir := fabricengine.BoardDir(hub)
	if err := os.MkdirAll(boardDir, 0o755); err != nil {
		t.Fatalf("MkdirAll board: %v", err)
	}
	if err := os.WriteFile(filepath.Join(boardDir, fabricengine.ShortnameFileName), []byte("tst\n"), 0o644); err != nil {
		t.Fatalf("write shortname: %v", err)
	}

	var got burlerengine.Geometry = burlerGeometry(l, false)

	if got.Shortname != "tst" || got.Slug != worktreeName {
		t.Errorf("burlerGeometry(l, false) Shortname/Slug = %q/%q; want the recorded shortname and the worktree name, %q/%q", got.Shortname, got.Slug, "tst", worktreeName)
	}
	if prime := burlerGeometry(l, true); prime.Shortname != "tst" || prime.Slug != "" {
		t.Errorf("burlerGeometry(l, true) Shortname/Slug = %q/%q; want the recorded shortname and no slug", prime.Shortname, prime.Slug)
	}

	if got.WorktreeRoot != anchorPath {
		t.Errorf("BurlerGeometry(l).WorktreeRoot = %q; want %q (anchorPath)", got.WorktreeRoot, anchorPath)
	}
	if got.AnchorPath != anchorPath {
		t.Errorf("BurlerGeometry(l).AnchorPath = %q; want %q", got.AnchorPath, anchorPath)
	}
	if got.RepoRoot != worktreeRoot {
		t.Errorf("BurlerGeometry(l).RepoRoot = %q; want %q (l.WorktreePath(), not the anchor path)", got.RepoRoot, worktreeRoot)
	}
	if want := fabricengine.SpecsDir(hub); got.SpecsDir != want {
		t.Errorf("BurlerGeometry(l).SpecsDir = %q; want %q", got.SpecsDir, want)
	}
	if got.WorktreeRoot == worktreeRoot {
		// Only a subpath-anchored fixture catches a change that repoints the WorktreeRoot fill at l.WorktreePath():
		// the two coincide only when AnchorRel is ".".
		t.Errorf("BurlerGeometry(l).WorktreeRoot = %q; want != WorktreeRoot %q", got.WorktreeRoot, worktreeRoot)
	}
}

// TestBurlerGeometry_ParentNameFromOriginRecord pins ParentName to the parent named by the worktree's origin record.
// ParentName is empty when the record names no parent worktree.
//
//testtiming:keep pins ParentName from a hand-written origin record, which the roots test covering its blocks never builds
func TestBurlerGeometry_ParentNameFromOriginRecord(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	l := hubWithOrigin(t, `{"parent_branch":"main","parent_worktree":"gone-task"}`)
	if got, want := burlerGeometry(l, false).ParentName, "tst:gone-task:orch"; got != want {
		t.Errorf("BurlerGeometry(l).ParentName = %q; want %q", got, want)
	}

	l = hubWithOrigin(t, `{"parent_branch":"main"}`)
	if got := burlerGeometry(l, false).ParentName; got != "" {
		t.Errorf("BurlerGeometry(l).ParentName without a parent worktree = %q; want empty", got)
	}
}

// TestIsPrimeWorktree pins the rule that tells the prime worktree from a task worktree by its .git entry.
// A .git directory, or a gitdir file pointing straight at a git directory, is the main worktree.
// A gitdir file pointing into worktrees/ is a linked one, and anything else is an error.
func TestIsPrimeWorktree(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name      string
		setup     func(t *testing.T, root string)
		wantPrime bool
		wantErr   bool
	}{
		{"git directory is the prime", func(t *testing.T, root string) {
			mkdir(t, filepath.Join(root, ".git"))
		}, true, false},
		{"gitdir file into worktrees is a task worktree", func(t *testing.T, root string) {
			writeFile(t, filepath.Join(root, ".git"), "gitdir: /hub/prime/.git/worktrees/some-task\n")
		}, false, false},
		{"gitdir file to a separate git directory is the prime", func(t *testing.T, root string) {
			writeFile(t, filepath.Join(root, ".git"), "gitdir: /elsewhere/repo.git\n")
		}, true, false},
		{"missing entry is an error", func(t *testing.T, root string) {}, false, true},
		{"file without gitdir is an error", func(t *testing.T, root string) {
			writeFile(t, filepath.Join(root, ".git"), "not a pointer\n")
		}, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			tt.setup(t, root)

			prime, err := isPrimeWorktree(root)
			if (err != nil) != tt.wantErr {
				t.Fatalf("isPrimeWorktree() error = %v; want error %v", err, tt.wantErr)
			}
			if err == nil && prime != tt.wantPrime {
				t.Errorf("isPrimeWorktree() = %v; want %v", prime, tt.wantPrime)
			}
		})
	}
}

// TestReconcileGeometry verifies a hub with a board-level lym dir yields exactly the board dir and worktree root,
// and a layout without one yields the zero value and false.
func TestReconcileGeometry(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name         string
		boardLymDir  bool
		wantGeometry bool
	}{
		{"hub with a board lym dir", true, true},
		{"layout without a board lym dir", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			hub := filepath.Join(t.TempDir(), "some-hub-LYMHUB")
			l := &lymcwd.Location{HubPath: hub, WorktreeName: "some-worktree", AnchorRel: "."}
			mkdir(t, l.WorktreePath())
			if tt.boardLymDir {
				mkdir(t, filepath.Join(fabricengine.BoardDir(hub), lymdirs.LymDirName))
			} else {
				mkdir(t, fabricengine.BoardDir(hub))
			}

			got, ok := ReconcileGeometry(l)

			want := hubreconcile.Geometry{}
			if tt.wantGeometry {
				want = hubreconcile.Geometry{BoardDir: fabricengine.BoardDir(hub), WorktreePath: l.WorktreePath()}
			}
			if ok != tt.wantGeometry || got != want {
				t.Errorf("ReconcileGeometry() = (%+v, %v); want (%+v, %v)", got, ok, want, tt.wantGeometry)
			}
		})
	}
}

// TestCreelGeometry pins every creel geometry field.
// One hub gives one socket path, and two hubs of one shortname get different ones.
// A hub with no shortname record is refused, naming it.
func TestCreelGeometry(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	newHub := func(t *testing.T, name, shortname string) string {
		t.Helper()
		hub := filepath.Join(t.TempDir(), name)
		mkdir(t, fabricengine.BoardDir(hub))
		if shortname != "" {
			writeFile(t, filepath.Join(fabricengine.BoardDir(hub), fabricengine.ShortnameFileName), shortname+"\n")
		}
		return hub
	}
	tempRoot := filepath.Join(t.TempDir(), "root")

	hub := newHub(t, "a-LYMHUB", "abc")
	got, err := CreelGeometry(hub, tempRoot)
	if err != nil {
		t.Fatalf("CreelGeometry(%q) = %v", hub, err)
	}
	stateDir := filepath.Join(fabricengine.HubScratchDir(hub), lymdirs.CreelDirName)
	want := creelengine.Geometry{
		HubPath:       hub,
		Shortname:     "abc",
		StateDir:      stateDir,
		JournalDir:    filepath.Join(stateDir, "journal"),
		LockPath:      filepath.Join(stateDir, "daemon.lock"),
		SocketPath:    filepath.Join(tempRoot, creelengine.SocketName("abc", hub)),
		DaemonLogPath: filepath.Join(stateDir, "daemon.log"),
		FlagDir:       filepath.Join(stateDir, lymdirs.CreelFlagDirName),
	}
	if got != want {
		t.Errorf("CreelGeometry(%q) = %+v; want %+v", hub, got, want)
	}
	// A Claude strand's mail hook tests flags in the directory its engine derives from the board directory alone.
	if hookDir := shuttleengine.CreelFlagDir(fabricengine.BoardDir(hub)); got.FlagDir != hookDir {
		t.Errorf("CreelGeometry(%q).FlagDir = %q; want the Claude hook's %q", hub, got.FlagDir, hookDir)
	}

	again, err := CreelGeometry(hub, tempRoot)
	if err != nil || again.SocketPath != got.SocketPath {
		t.Errorf("CreelGeometry(%q) again = (%q, %v); want socket %q", hub, again.SocketPath, err, got.SocketPath)
	}

	other := newHub(t, "b-LYMHUB", "abc")
	otherGeometry, err := CreelGeometry(other, tempRoot)
	if err != nil {
		t.Fatalf("CreelGeometry(%q) = %v", other, err)
	}
	if otherGeometry.SocketPath == got.SocketPath {
		t.Errorf("two hubs of one shortname share socket path %q", got.SocketPath)
	}

	bare := newHub(t, "c-LYMHUB", "")
	_, err = CreelGeometry(bare, tempRoot)
	if err == nil || !strings.Contains(err.Error(), fabricengine.ShortnameFileName) || !strings.Contains(err.Error(), bare) {
		t.Errorf("CreelGeometry(no shortname) error = %v; want one naming %s and the hub", err, fabricengine.ShortnameFileName)
	}
}

// TestSlackGeometry pins every Slack geometry field.
// The token path lies in the credential directory, while every other path lies under the hub's Slack directory.
// A hub with no shortname record is refused.
// A machine with no credential directory is refused, naming the environment variables that decide it.
// The test redirects the credential directory through t.Setenv, which is process-global state,
// so it does not run in parallel.
func TestSlackGeometry(t *testing.T) {
	tiergate.Require(t, 0)
	credentialRoot := t.TempDir()
	for _, name := range []string{"HOME", "XDG_CONFIG_HOME", "LOCALAPPDATA"} {
		t.Setenv(name, credentialRoot)
	}
	newHub := func(name, shortname string) string {
		hub := filepath.Join(t.TempDir(), name)
		mkdir(t, fabricengine.BoardDir(hub))
		if shortname != "" {
			writeFile(t, filepath.Join(fabricengine.BoardDir(hub), fabricengine.ShortnameFileName), shortname+"\n")
		}
		return hub
	}

	hub := newHub("a-LYMHUB", "abc")
	got, err := SlackGeometry(hub)
	if err != nil {
		t.Fatalf("SlackGeometry(%q) = %v", hub, err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable() = %v", err)
	}
	machine, err := machinename.Name()
	if err != nil {
		t.Fatalf("machinename.Name() = %v", err)
	}
	dir := filepath.Join(fabricengine.HubScratchDir(hub), lymdirs.SlackDirName)
	want := slackengine.Geometry{
		HubPath:        hub,
		Shortname:      "abc",
		Machine:        machine,
		Dir:            dir,
		TokenPath:      filepath.Join(credentialRoot, "lym", slackengine.TokenFileName),
		ChannelMapPath: filepath.Join(dir, slackengine.ChannelMapFileName),
		InboxDir:       filepath.Join(dir, slackengine.InboxDirName),
		SocketLockPath: filepath.Join(credentialRoot, "lym", slackengine.SocketLockFileName),
		SetupPath:      filepath.Join(dir, slackengine.SetupFileName),
		PresencePath:   filepath.Join(dir, slackengine.PresenceFileName),
		CursorPath:     filepath.Join(dir, slackengine.CursorFileName),
		OutboxPath:     filepath.Join(dir, slackengine.OutboxFileName),
		RelayStatePath: filepath.Join(dir, slackengine.RelayStateFileName),
		LockPath:       filepath.Join(dir, slackengine.RelayLockFileName),
		LogPath:        filepath.Join(dir, slackengine.RelayLogFileName),
		RunProcDir:     fabricengine.RunProcDir(hub),
		LymExecutable:  executable,
	}
	if got != want {
		t.Errorf("SlackGeometry(%q) = %+v; want %+v", hub, got, want)
	}

	bare := newHub("b-LYMHUB", "")
	_, wantErr := CreelGeometry(bare, "")
	if _, err := SlackGeometry(bare); err == nil || wantErr == nil || err.Error() != wantErr.Error() {
		t.Errorf("SlackGeometry(no shortname) error = %v; want CreelGeometry's %v", err, wantErr)
	}

	for _, name := range []string{"HOME", "XDG_CONFIG_HOME", "LOCALAPPDATA"} {
		t.Setenv(name, "")
	}
	_, err = SlackGeometry(hub)
	if err == nil {
		t.Fatal("SlackGeometry with no credential directory = nil error; want a refusal")
	}
	for _, name := range []string{"HOME", "XDG_CONFIG_HOME", "LOCALAPPDATA"} {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("SlackGeometry with no credential directory error = %v; want it to name %s", err, name)
		}
	}
}

func mkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) = %v", path, err)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) = %v", path, err)
	}
}
