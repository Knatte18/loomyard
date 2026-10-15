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

		got, err := loadRunWrites(shuttlefake.EngineSet(engine), st, "/wt")
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

		got, err := loadRunWrites(shuttlefake.EngineSet(engine), st, "/wt")
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

		got, err := loadRunWrites(shuttlefake.EngineSet(engine), st, "/wt")
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

	t.Run("each session is audited through its recorded engine", func(t *testing.T) {
		t.Parallel()

		var calls []string
		fake := func(name string) *shuttlefake.Engine {
			return &shuttlefake.Engine{AuditForksFn: func(session, _ string) (shuttleengine.ForkAudit, error) {
				calls = append(calls, name+":"+session)
				return shuttleengine.ForkAudit{ParentWriteEvents: []shuttleengine.WriteEvent{{Path: name + ":" + session}}}, nil
			}}
		}
		engines := shuttleengine.NewEngineSet("c", map[string]shuttleengine.Engine{"a": fake("a"), "b": fake("b"), "c": fake("c")})
		st := &State{
			MasterSessionID: "m",
			MasterEngine:    "a",
			Batches: map[int]*BatchState{
				// The Master session recorded again under another engine is audited once, through its first sighting.
				1: {SessionID: "m", Engine: "b"},
				2: {SessionID: "f", Engine: "b", RecoverySessions: []string{"r1", "r2"}, RecoveryEngines: map[string]string{"r1": "a", "r2": "b"}},
				3: {SessionID: "legacy"},
			},
		}

		got, err := loadRunWrites(engines, st, "/wt")
		if err != nil {
			t.Fatalf("loadRunWrites: %v", err)
		}
		if want := []string{"a:m", "b:f", "c:legacy", "a:r1", "b:r2"}; !reflect.DeepEqual(calls, want) {
			t.Errorf("audits = %v, want %v (each session through its own engine, an engine-less one through the default)", calls, want)
		}
		wantMaster := []shuttleengine.WriteEvent{{Path: "a:m"}, {Path: "b:f"}, {Path: "c:legacy"}}
		if !reflect.DeepEqual(got.Master, wantMaster) {
			t.Errorf("Master = %v, want %v", got.Master, wantMaster)
		}
		wantRecoveries := []shuttleengine.WriteEvent{{Path: "a:r1"}, {Path: "b:r2"}}
		if !reflect.DeepEqual(got.Recoveries, wantRecoveries) {
			t.Errorf("Recoveries = %v, want %v", got.Recoveries, wantRecoveries)
		}
	})

	t.Run("a session whose engine cannot be read is refused", func(t *testing.T) {
		t.Parallel()

		fake := &shuttlefake.Engine{}
		withoutForkAudit := struct{ shuttleengine.Engine }{fake}
		tests := []struct {
			name      string
			engines   shuttleengine.EngineSet
			engine    string
			wantIs    error
			wantParts []string
			wantNot   string
		}{
			{
				name:      "UnregisteredEngine",
				engines:   shuttleengine.NewEngineSet("a", map[string]shuttleengine.Engine{"a": fake}),
				engine:    "gone",
				wantIs:    shuttleengine.ErrEngineNotRegistered,
				wantParts: []string{"sess-1", `"gone"`, `with a lyx whose engine set holds "gone"`},
			},
			{
				name:      "EngineLessSessionOnDefaultWithoutForkAudit",
				engines:   shuttleengine.NewEngineSet("a", map[string]shuttleengine.Engine{"a": withoutForkAudit}),
				wantParts: []string{"sess-1", `engine: "a"`, "set engine: in shuttle.yaml back to an engine with fork audit"},
			},
			{
				name:      "HeldEngineWithoutForkAudit",
				engines:   shuttleengine.NewEngineSet("b", map[string]shuttleengine.Engine{"a": withoutForkAudit, "b": fake}),
				engine:    "a",
				wantParts: []string{"internal error", "sess-1", `"a"`},
				wantNot:   "way forward",
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				st := &State{MasterSessionID: "sess-1", MasterEngine: tt.engine}
				_, err := loadRunWrites(tt.engines, st, "/wt")
				if err == nil {
					t.Fatal("loadRunWrites() error = nil; want a refusal")
				}
				if tt.wantIs != nil && !errors.Is(err, tt.wantIs) {
					t.Errorf("loadRunWrites() error = %v; want it to wrap %v", err, tt.wantIs)
				}
				for _, part := range tt.wantParts {
					if !strings.Contains(err.Error(), part) {
						t.Errorf("loadRunWrites() error = %q; want it to contain %q", err, part)
					}
				}
				if tt.wantNot != "" && strings.Contains(err.Error(), tt.wantNot) {
					t.Errorf("loadRunWrites() error = %q; want no %q", err, tt.wantNot)
				}
			})
		}
	})

	t.Run("a nil engines yields an empty history", func(t *testing.T) {
		t.Parallel()

		got, err := loadRunWrites(nil, &State{MasterSessionID: "s1"}, "/wt")
		if err != nil {
			t.Fatalf("loadRunWrites: %v", err)
		}
		if !reflect.DeepEqual(got, RunWrites{}) {
			t.Errorf("loadRunWrites(nil) = %+v, want the empty history", got)
		}
	})

	t.Run("an audit error names the session", func(t *testing.T) {
		t.Parallel()

		st := &State{MasterSessionID: "sess-bad"}
		boom := errors.New("transcript unreadable")
		engine := &shuttlefake.Engine{AuditErr: boom}

		_, err := loadRunWrites(shuttlefake.EngineSet(engine), st, "/wt")
		if !errors.Is(err, boom) {
			t.Fatalf("err = %v, want it to wrap %v", err, boom)
		}
		if !strings.Contains(err.Error(), "sess-bad") {
			t.Errorf("err = %q, want it to name the session", err)
		}
	})
}
