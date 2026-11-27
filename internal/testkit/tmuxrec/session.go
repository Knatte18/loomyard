// session.go builds the session world every recording of the package was made in:
// a real session runner over a real reed engine, whose outside is a recorder or a replay.

package tmuxrec

import (
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/machinename"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/shuttlefake"
)

// The names and launch every recorded session runs under.
const (
	// WorktreeName is the base name of the worktree, and so reed's session name.
	WorktreeName = "pr-session"
	// Shortname starts every strand name.
	Shortname = "tr"
	// EngineName registers the session's engine.
	EngineName = "claude"
	// LaunchCmd is the command the session's pane runs: a process that stays up until it is ended.
	LaunchCmd = "sleep 3600"
	// SessionID is the provider session id every start of the engine reports.
	SessionID = "sess-1"
	// GUIDPrefix starts each strand guid the session's reed mints, followed by the strand's start count.
	GUIDPrefix = "guid-"
)

// recordings holds every recording of the package.
//
//go:embed testdata/*.json
var recordings embed.FS

// Session is a session runner over a real reed engine, built as every recording of the package was made.
type Session struct {
	// Hub is the session's hub directory, and Worktree its worktree under it.
	Hub      string
	Worktree string
	// RunDir is the directory the runner makes each run's directory in.
	RunDir string
	// StateDir is the directory reed keeps its state and operation lock in.
	StateDir string
	// StartupTimeoutS is the runner's startup timeout, in seconds; 0 ends a start whose provider is not up at once.
	StartupTimeoutS int
	// Bindings binds the placeholders of the session's recordings to its directories.
	Bindings Bindings
	Reed     *reedengine.Engine
	// Engine is the provider engine the runner starts, the one stand-in above reed.
	Engine *shuttlefake.Engine
	Runner *shuttleengine.Runner
}

// replaySocket is the -L socket key a replayed session's reed is told; no server listens on it.
const replaySocket = "tmuxrec-replay"

// newSession builds a session in fresh directories whose reed is told the -L socket key socket.
// That reed reaches the outside that outside returns for the session's bindings and guid source.
// Its runner gives a provider startupTimeoutS seconds to come up.
func newSession(t testing.TB, socket string, startupTimeoutS int, outside func(bindings Bindings, newGUID func() (string, error)) reedengine.Outside) *Session {
	t.Helper()
	hub := t.TempDir()
	bindings := Bindings{"{hub}": hub, "{tmp}": os.TempDir(), "{socket}": socket}
	// A pane's title and prompt name the machine and the user, which a recording keeps as placeholders.
	if host, err := machinename.Hostname(); err == nil {
		bindings["{host}"] = host
	}
	if account, err := user.Current(); err == nil {
		bindings["{user}"] = account.Username
	}
	// The uid is bound only where a socket path or a unit name spells it, since a bare number also names panes and sizes.
	// Windows has no uid and answers -1.
	if uid := os.Getuid(); uid >= 0 {
		for _, prefix := range []string{"tmux-", "user-", "user@"} {
			bindings[prefix+"{uid}"] = prefix + strconv.Itoa(uid)
		}
	}
	worktree := filepath.Join(hub, WorktreeName)
	if err := os.MkdirAll(worktree, 0o755); err != nil {
		t.Fatalf("tmuxrec: create the worktree: %v", err)
	}
	cfg, err := reedengine.LoadConfig(configengine.Dirs{Anchor: hub}, "reed")
	if err != nil {
		t.Fatalf("tmuxrec: load the reed config template: %v", err)
	}
	// Reed pins the session's shell by the path the executable search finds, which differs between hosts.
	shellPath, err := reedengine.ResolveShellPath(cfg.Shell)
	if err != nil {
		t.Fatalf("tmuxrec: %v", err)
	}
	bindings["{shell}"] = shellPath
	geom := reedengine.Geometry{
		SocketKey:     socket,
		SessionName:   reedengine.SessionName(worktree),
		AnchorPath:    worktree,
		PaneCwd:       worktree,
		WorktreeRoot:  worktree,
		LogsDir:       filepath.Join(hub, "logs"),
		WorktreeName:  WorktreeName,
		HubPath:       hub,
		NameShortname: Shortname,
	}
	scrub := reedengine.EnvScrub{Names: []string{"CLAUDECODE"}, Prefixes: []string{"CLAUDE_CODE_"}}
	reed := reedengine.NewOver(cfg, geom, scrub, outside(bindings, guidSequence()))

	engine := &shuttlefake.Engine{PrepareLaunch: &shuttleengine.Launch{Cmd: LaunchCmd, SessionID: SessionID}}
	engines := shuttleengine.NewEngineSet(EngineName, map[string]shuttleengine.Engine{EngineName: &shuttlefake.ResumeEngine{Engine: engine}})
	runnerCfg := shuttleengine.Config{
		RunDir:          filepath.Join(hub, "runs"),
		RunTimeoutMin:   60,
		StartupTimeoutS: startupTimeoutS,
		Agents:          shuttlefake.Agents(t),
	}
	return &Session{
		Hub:             hub,
		Worktree:        worktree,
		RunDir:          runnerCfg.RunDir,
		StateDir:        filepath.Join(worktree, lymdirs.DotLymDirName),
		StartupTimeoutS: startupTimeoutS,
		Bindings:        bindings,
		Reed:            reed,
		Engine:          engine,
		Runner:          shuttleengine.NewRunner(reed, engines, worktree, worktree, runnerCfg),
	}
}

// guidSequence returns a guid source that mints GUIDPrefix and the count of guids minted so far, from 1.
func guidSequence() func() (string, error) {
	n := 0
	return func() (string, error) {
		n++
		return GUIDPrefix + strconv.Itoa(n), nil
	}
}

// RecordSession builds a session whose reed runs on the real tmux server rec runs on, through rec.
// Its runner gives a provider startupTimeoutS seconds to come up.
func RecordSession(t testing.TB, rec *Recorder, startupTimeoutS int) *Session {
	t.Helper()
	return newSession(t, rec.socket, startupTimeoutS, func(_ Bindings, newGUID func() (string, error)) reedengine.Outside {
		return rec.Outside(newGUID)
	})
}

// PlaySession builds a session whose reed answers from the package's recording named name.
// The runner's startup timeout is the one the recording was made under.
// It skips the test on an operating system other than the recording's, whose tmux calls differ.
// A call the recording holds no answer to fails in reed; the returned replay lists those calls for the test to check.
func PlaySession(t testing.TB, name string) (*Session, *Replay) {
	t.Helper()
	rec, err := Embedded(name)
	if err != nil {
		t.Fatalf("%v", err)
	}
	if rec.Source.OS != runtime.GOOS {
		t.Skipf("tmuxrec: no recording %s made on %s; the stored one was made on %s", name, runtime.GOOS, rec.Source.OS)
	}
	var replay *Replay
	session := newSession(t, replaySocket, rec.StartupTimeoutS, func(bindings Bindings, newGUID func() (string, error)) reedengine.Outside {
		replay = NewReplay(name, rec, bindings)
		return replay.Outside(newGUID)
	})
	return session, replay
}

// Embedded returns the package's recording named name.
func Embedded(name string) (Recording, error) {
	data, err := recordings.ReadFile("testdata/" + name + ".json")
	if err != nil {
		return Recording{}, fmt.Errorf("tmuxrec: no recording named %s: %w", name, err)
	}
	var rec Recording
	if err := json.Unmarshal(data, &rec); err != nil {
		return Recording{}, fmt.Errorf("tmuxrec: decode recording %s: %w", name, err)
	}
	return rec, nil
}
