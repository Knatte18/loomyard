//go:build tmux

// revive_integration_test.go proves against a real tmux that a restarted server's sessions are revived in the told spawn order:
// three worktrees on one private socket, a server kill, and the order the new server numbers their sessions in.

package reedengine

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

// newReviveHub builds one engine per worktree name under a single hub, so they share one socket,
// and tells each the spawn order of names with the real Revive of every engine.
// A test replaces a returned entry to script a worktree's Revive.
func newReviveHub(t *testing.T, names ...string) (engines []*Engine, entries []ReviveEntry) {
	t.Helper()
	hubDir := t.TempDir()
	for _, name := range names {
		worktreeDir := filepath.Join(hubDir, name)
		if err := os.MkdirAll(worktreeDir, 0o755); err != nil {
			t.Fatalf("mkdir worktree dir: %v", err)
		}
		seedReedConfig(t, worktreeDir)
		cfg, err := LoadConfig(worktreeDir, "reed")
		if err != nil {
			t.Fatalf("LoadConfig: %v", err)
		}
		if _, err := exec.LookPath(cfg.Tmux); err != nil {
			t.Skipf("configured multiplexer binary %q not found: %v", cfg.Tmux, err)
		}
		cfg.Mouse = "off"
		e := New(cfg, Geometry{
			SocketKey:     ServerName(hubDir),
			SessionName:   SessionName(worktreeDir),
			AnchorPath:    worktreeDir,
			PaneCwd:       worktreeDir,
			WorktreeRoot:  worktreeDir,
			LogsDir:       filepath.Join(hubDir, "logs"),
			HubPath:       hubDir,
			WorktreeName:  name,
			NameShortname: "tc",
			NameSlug:      name,
		})
		engines = append(engines, e)
		entries = append(entries, ReviveEntry{Worktree: name, Revive: e.Revive})
	}
	for _, e := range engines {
		e.geom.SpawnOrder = func() ([]ReviveEntry, error) { return entries, nil }
	}
	tmuxkit.KillOnCleanup(t, engines[0].cfg.Tmux, engines[0].Socket())
	return engines, entries
}

// sessionNamesByID returns the live sessions' names in session-id order.
func sessionNamesByID(t *testing.T, e *Engine) []string {
	t.Helper()
	out, err := e.tmux.output("list-sessions", "-F", "#{session_id} #{session_name}")
	if err != nil {
		return nil
	}
	type session struct {
		id   int
		name string
	}
	var sessions []session
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		id, err := strconv.Atoi(strings.TrimPrefix(fields[0], "$"))
		if err != nil {
			t.Fatalf("session id %q: %v", fields[0], err)
		}
		sessions = append(sessions, session{id, fields[1]})
	}
	slices.SortFunc(sessions, func(a, b session) int { return a.id - b.id })
	names := make([]string, len(sessions))
	for i, s := range sessions {
		names[i] = s.name
	}
	return names
}

// bootAllThenKillServer boots every engine in order, then kills the server and waits until it is gone.
func bootAllThenKillServer(t *testing.T, engines []*Engine) {
	t.Helper()
	for _, e := range engines {
		if _, err := e.Up(); err != nil {
			t.Fatalf("Up(%s): %v", e.SessionName(), err)
		}
	}
	killServerAndWait(t, engines[0])
}

func killServerAndWait(t *testing.T, e *Engine) {
	t.Helper()
	_ = e.tmux.run("kill-server")
	waitUntil(t, 15*time.Second, "the tmux server never went away", func() bool {
		_, err := e.tmux.output("list-sessions")
		return err != nil
	})
}

// TestRevive_RestartedServerKeepsSpawnOrder pins the revival against a real server restart.
func TestRevive_RestartedServerKeepsSpawnOrder(t *testing.T) {
	names := []string{"wt-a", "wt-b", "wt-c"}

	t.Run("BootingTheLastBringsTheSessionsBackInListOrder", func(t *testing.T) {
		engines, _ := newReviveHub(t, names...)
		bootAllThenKillServer(t, engines)

		if _, err := engines[2].Up(); err != nil {
			t.Fatalf("Up(wt-c): %v", err)
		}

		if got := sessionNamesByID(t, engines[0]); !slices.Equal(got, names) {
			t.Errorf("sessions in id order = %v, want %v", got, names)
		}
	})

	t.Run("ASecondBooterFillsTheEarlierGapsFirst", func(t *testing.T) {
		engines, _ := newReviveHub(t, names...)
		bootAllThenKillServer(t, engines)
		// Another session brings the server up first; it is not a worktree of the list.
		if err := engines[0].tmux.run("new-session", "-d", "-s", "keepalive", "-c", engines[0].geom.PaneCwd); err != nil {
			t.Fatalf("new-session keepalive: %v", err)
		}

		if _, err := engines[2].Up(); err != nil {
			t.Fatalf("Up(wt-c): %v", err)
		}

		if got, want := sessionNamesByID(t, engines[0]), []string{"keepalive", "wt-a", "wt-b", "wt-c"}; !slices.Equal(got, want) {
			t.Errorf("sessions in id order = %v, want %v", got, want)
		}
	})

	t.Run("AFailingPredecessorIsSkipped", func(t *testing.T) {
		engines, entries := newReviveHub(t, names...)
		bootAllThenKillServer(t, engines)
		entries[0].Revive = func() (bool, error) { return false, errors.New("boom") }

		if _, err := engines[2].Up(); err != nil {
			t.Fatalf("Up(wt-c) = %v, want the boot to complete past the failing predecessor", err)
		}

		if got, want := sessionNamesByID(t, engines[0]), []string{"wt-b", "wt-c"}; !slices.Equal(got, want) {
			t.Errorf("sessions in id order = %v, want %v", got, want)
		}
	})

	t.Run("ALaterLiveSessionIsLeftUntouched", func(t *testing.T) {
		engines, _ := newReviveHub(t, names...)
		bootAllThenKillServer(t, engines)
		// The last worktree comes back alone, ahead of its predecessors.
		if revived, err := engines[2].Revive(); err != nil || !revived {
			t.Fatalf("Revive(wt-c) = (%v, %v), want (true, nil)", revived, err)
		}

		if _, err := engines[1].Up(); err != nil {
			t.Fatalf("Up(wt-b): %v", err)
		}

		if got, want := sessionNamesByID(t, engines[0]), []string{"wt-c", "wt-a", "wt-b"}; !slices.Equal(got, want) {
			t.Errorf("sessions in id order = %v, want %v (the live later session keeps its id, the booter is created after it)", got, want)
		}
	})

	t.Run("AFirstBootAndADownedWorktreeTriggerNoRevival", func(t *testing.T) {
		engines, _ := newReviveHub(t, names...)
		if _, err := engines[2].Up(); err != nil {
			t.Fatalf("Up(wt-c) on a first boot: %v", err)
		}
		if got, want := sessionNamesByID(t, engines[0]), []string{"wt-c"}; !slices.Equal(got, want) {
			t.Fatalf("sessions after a first boot = %v, want only %v", got, want)
		}
		for _, e := range engines[:2] {
			if _, err := e.Up(); err != nil {
				t.Fatalf("Up(%s): %v", e.SessionName(), err)
			}
		}
		if _, err := engines[0].Down(); err != nil {
			t.Fatalf("Down(wt-a): %v", err)
		}
		killServerAndWait(t, engines[0])

		if _, err := engines[2].Up(); err != nil {
			t.Fatalf("Up(wt-c): %v", err)
		}

		if got, want := sessionNamesByID(t, engines[0]), []string{"wt-b", "wt-c"}; !slices.Equal(got, want) {
			t.Errorf("sessions in id order = %v, want %v (the downed worktree is not revived)", got, want)
		}
	})
}
