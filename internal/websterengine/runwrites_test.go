// runwrites_test.go covers loadRunWrites: every recorded Master session is audited and its events merged,
// a session id recorded twice is audited once, an audit error surfaces naming the session,
// and writtenWorktreePaths turns the events into worktree-relative paths.
// Untagged, with a shuttlefake.Engine — no git, no subprocess spawns.

package websterengine

import (
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/shuttlefake"
)

func TestLoadRunWrites(t *testing.T) {
	t.Parallel()

	t.Run("merges every session's events", func(t *testing.T) {
		t.Parallel()

		st := &State{
			MasterSessionID: "s1",
			Batches: map[int]*BatchState{
				1: {SessionID: "s1"},
				2: {SessionID: "s2"},
			},
		}
		var audited []string
		engine := &shuttlefake.Engine{AuditForksFn: func(session, _ string) (shuttleengine.ForkAudit, error) {
			audited = append(audited, session)
			return shuttleengine.ForkAudit{
				ParentWriteEvents: []shuttleengine.WriteEvent{{Path: session + "-master", Succeeded: true}},
				Forks: []shuttleengine.ForkReport{
					{WriteEvents: []shuttleengine.WriteEvent{{Path: session + "-fork-a"}}},
					{WriteEvents: []shuttleengine.WriteEvent{{Path: session + "-fork-b"}}},
				},
			}, nil
		}}

		got, err := loadRunWrites(engine, st, "/wt")
		if err != nil {
			t.Fatalf("loadRunWrites: %v", err)
		}
		if want := []string{"s1", "s2"}; !reflect.DeepEqual(audited, want) {
			t.Errorf("audited sessions = %v, want %v", audited, want)
		}
		wantMaster := []shuttleengine.WriteEvent{{Path: "s1-master", Succeeded: true}, {Path: "s2-master", Succeeded: true}}
		if !reflect.DeepEqual(got.Master, wantMaster) {
			t.Errorf("Master = %v, want %v", got.Master, wantMaster)
		}
		wantForks := []shuttleengine.WriteEvent{{Path: "s1-fork-a"}, {Path: "s1-fork-b"}, {Path: "s2-fork-a"}, {Path: "s2-fork-b"}}
		if !reflect.DeepEqual(got.Forks, wantForks) {
			t.Errorf("Forks = %v, want %v", got.Forks, wantForks)
		}
	})

	t.Run("recovery sessions are audited after the Master sessions, once each", func(t *testing.T) {
		t.Parallel()

		st := &State{
			MasterSessionID: "s1",
			Batches: map[int]*BatchState{
				1: {SessionID: "s1", RecoverySessions: []string{"r1", "r2"}},
				2: {SessionID: "s2", RecoverySessions: []string{"r2", "s1"}},
			},
		}
		var audited []string
		engine := &shuttlefake.Engine{AuditForksFn: func(session, _ string) (shuttleengine.ForkAudit, error) {
			audited = append(audited, session)
			return shuttleengine.ForkAudit{
				ParentWriteEvents: []shuttleengine.WriteEvent{{Path: session + "-parent", Succeeded: true}},
				Forks:             []shuttleengine.ForkReport{{WriteEvents: []shuttleengine.WriteEvent{{Path: session + "-fork"}}}},
			}, nil
		}}

		got, err := loadRunWrites(engine, st, "/wt")
		if err != nil {
			t.Fatalf("loadRunWrites: %v", err)
		}
		if want := []string{"s1", "s2", "r1", "r2"}; !reflect.DeepEqual(audited, want) {
			t.Errorf("audited sessions = %v, want %v", audited, want)
		}
		wantRecoveries := []shuttleengine.WriteEvent{{Path: "r1-parent", Succeeded: true}, {Path: "r2-parent", Succeeded: true}}
		if !reflect.DeepEqual(got.Recoveries, wantRecoveries) {
			t.Errorf("Recoveries = %v, want %v", got.Recoveries, wantRecoveries)
		}
		wantForks := []shuttleengine.WriteEvent{{Path: "s1-fork"}, {Path: "s2-fork"}, {Path: "r1-fork"}, {Path: "r2-fork"}}
		if !reflect.DeepEqual(got.Forks, wantForks) {
			t.Errorf("Forks = %v, want %v", got.Forks, wantForks)
		}
	})

	t.Run("a session recorded twice is audited once", func(t *testing.T) {
		t.Parallel()

		st := &State{
			MasterSessionID: "s1",
			Batches: map[int]*BatchState{
				1: {SessionID: "s1"},
				2: {SessionID: "s1"},
				3: {},
			},
		}
		calls := 0
		engine := &shuttlefake.Engine{AuditForksFn: func(string, string) (shuttleengine.ForkAudit, error) {
			calls++
			return shuttleengine.ForkAudit{ParentWriteEvents: []shuttleengine.WriteEvent{{Path: "p"}}}, nil
		}}

		got, err := loadRunWrites(engine, st, "/wt")
		if err != nil {
			t.Fatalf("loadRunWrites: %v", err)
		}
		if calls != 1 {
			t.Errorf("AuditForks calls = %d, want 1", calls)
		}
		if len(got.Master) != 1 {
			t.Errorf("Master = %v, want one event", got.Master)
		}
	})

	t.Run("written worktree paths keep untracked files and drop failed and outside writes", func(t *testing.T) {
		t.Parallel()

		worktree := t.TempDir()
		writes := RunWrites{
			Master: []shuttleengine.WriteEvent{
				{Path: filepath.Join(worktree, "b", "new.go"), Succeeded: true},
				{Path: filepath.Join(worktree, "failed.go")},
				{Path: filepath.Join(t.TempDir(), "outside.go"), Succeeded: true},
			},
			Forks: []shuttleengine.WriteEvent{
				{Path: "a.go", Succeeded: true},
				{Path: filepath.Join(worktree, "b", "new.go"), Succeeded: true},
			},
			Recoveries: []shuttleengine.WriteEvent{
				{Path: "recovered.go", Succeeded: true},
				{Path: "recovery-failed.go"},
			},
		}

		got, err := writtenWorktreePaths(writes, worktree)
		if err != nil {
			t.Fatalf("writtenWorktreePaths: %v", err)
		}
		if want := []string{"a.go", "b/new.go", "recovered.go"}; !reflect.DeepEqual(got, want) {
			t.Errorf("writtenWorktreePaths = %v, want %v", got, want)
		}
	})

	t.Run("an audit error names the session", func(t *testing.T) {
		t.Parallel()

		st := &State{MasterSessionID: "sess-bad"}
		boom := errors.New("transcript unreadable")
		engine := &shuttlefake.Engine{AuditErr: boom}

		_, err := loadRunWrites(engine, st, "/wt")
		if !errors.Is(err, boom) {
			t.Fatalf("err = %v, want it to wrap %v", err, boom)
		}
		if !strings.Contains(err.Error(), "sess-bad") {
			t.Errorf("err = %q, want it to name the session", err)
		}
	})
}
