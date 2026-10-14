// verbs_test.go drives submit, cancel and status over fakeWorld.

package hibernateengine

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/tenderengine"
)

func refOf(entry pauserecord.Entry) *tenderengine.EntryRef {
	return &tenderengine.EntryRef{Scope: entry.Scope, Caller: entry.Caller, PausedAt: entry.PausedAt}
}

func readBytes(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func requireNoRequestFile(t *testing.T, w *fakeWorld) {
	t.Helper()
	if _, err := os.Stat(w.requestPath); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("request file stat err = %v; want it absent", err)
	}
}

// requireRefusal fails unless err is non-nil and contains every part.
func requireRefusal(t *testing.T, err error, parts ...string) {
	t.Helper()
	if err == nil {
		t.Fatalf("err = nil; want a refusal containing %q", parts)
	}
	for _, part := range parts {
		if !strings.Contains(err.Error(), part) {
			t.Errorf("err = %q; want it to contain %q", err, part)
		}
	}
}

func agentStrand(t *testing.T) string {
	t.Helper()
	strand, err := agentname.Format("lyx", "p1", "driver")
	if err != nil {
		t.Fatal(err)
	}
	return strand
}

func orchStrand(t *testing.T) string {
	t.Helper()
	strand, err := agentname.Format("lyx", "", agentname.RoleOrch)
	if err != nil {
		t.Fatal(err)
	}
	return strand
}

func running(producer string) RunStatus {
	return RunStatus{State: shedengine.StateRunning, Producer: producer}
}

func TestSubmit(t *testing.T) {
	tests := []struct {
		name string
		run  func(t *testing.T)
	}{
		{"no trigger spawns once and writes a waiting now request owning no entry", func(t *testing.T) {
			w := newFakeWorld(t)

			got, err := Submit(w.deps(), SubmitRequest{})

			if err != nil || len(w.spawned) != 1 {
				t.Fatalf("err %v, spawned %v; want one daemon", err, w.spawned)
			}
			want := Request{Trigger: TriggerNow, Requester: requesterExternal, RequestedAt: w.wall, DaemonPID: w.spawned[0], DaemonStart: "start-1000", Phase: PhaseWaiting}
			if !reflect.DeepEqual(got, want) || !reflect.DeepEqual(w.request(), want) {
				t.Errorf("returned %+v and written %+v; want %+v", got, w.request(), want)
			}
		}},
		{"after and after-all write their trigger", func(t *testing.T) {
			for _, tc := range []struct {
				req         SubmitRequest
				wantTrigger Trigger
				wantSlug    string
			}{
				{SubmitRequest{After: "p1"}, TriggerAfter, "p1"},
				{SubmitRequest{AfterAll: true}, TriggerAfterAll, ""},
			} {
				w := newFakeWorld(t)
				w.runs["p1"] = []RunStatus{running("Run-Shed")}

				got, err := Submit(w.deps(), tc.req)

				if err != nil || got.Trigger != tc.wantTrigger || got.Slug != tc.wantSlug {
					t.Errorf("%+v: got %+v, %v; want trigger %s with slug %q", tc.req, got, err, tc.wantTrigger, tc.wantSlug)
				}
			}
		}},
		{"an agent caller is refused naming the parent session, the orch and an external caller are not", func(t *testing.T) {
			for _, tc := range []struct {
				caller  string
				refused bool
			}{{agentStrand(t), true}, {"not-a-parsable-name", true}, {orchStrand(t), false}, {"", false}} {
				w := newFakeWorld(t)
				w.caller = tc.caller

				_, err := Submit(w.deps(), SubmitRequest{})

				if tc.refused {
					requireRefusal(t, err, "ask your parent session")
					if len(w.spawned) != 0 {
						t.Errorf("caller %q spawned %v; want no spawn", tc.caller, w.spawned)
					}
					requireNoRequestFile(t, w)
				} else if err != nil {
					t.Errorf("caller %q: err = %v; want none", tc.caller, err)
				}
			}
		}},
		{"a refused submit spawns nothing and writes no request", func(t *testing.T) {
			tests := []struct {
				name  string
				setup func(w *fakeWorld)
				req   SubmitRequest
				parts []string
			}{
				{"a non-Linux platform", func(w *fakeWorld) { w.goos = "darwin" }, SubmitRequest{}, []string{"Linux only", "darwin"}},
				{"no systemctl", func(w *fakeWorld) { w.systemctl = false }, SubmitRequest{}, []string{"systemctl is not on PATH"}},
				{"both triggers", func(w *fakeWorld) { w.runs["p1"] = []RunStatus{running("Run-Shed")} }, SubmitRequest{After: "p1", AfterAll: true}, []string{"exclusive"}},
				{"a slug with no status beside runs that have one", func(w *fakeWorld) { w.runs["p2"] = []RunStatus{running("Run-Shed")} }, SubmitRequest{After: "p1"}, []string{`"p1" has no batten status`, "p2"}},
				{"a slug with no status and no runs", func(*fakeWorld) {}, SubmitRequest{After: "p1"}, []string{"no batten run has one yet"}},
			}
			for _, tc := range tests {
				w := newFakeWorld(t)
				tc.setup(w)

				_, err := Submit(w.deps(), tc.req)

				requireRefusal(t, err, tc.parts...)
				if len(w.spawned) != 0 {
					t.Errorf("%s: spawned %v; want none", tc.name, w.spawned)
				}
				requireNoRequestFile(t, w)
			}
		}},
		{"a pending request with a live daemon is refused and left as it was", func(t *testing.T) {
			w := newFakeWorld(t)
			w.alive[7] = "s7"
			w.seed(Request{Trigger: TriggerNow, DaemonPID: 7, DaemonStart: "s7", Phase: PhaseConfirming})
			before := readBytes(t, w.requestPath)

			_, err := Submit(w.deps(), SubmitRequest{})

			requireRefusal(t, err, "lyx hibernate cancel", "confirming", "pid 7")
			if string(readBytes(t, w.requestPath)) != string(before) || len(w.spawned) != 0 {
				t.Errorf("request changed or a daemon spawned (%v); want neither", w.spawned)
			}
		}},
		{"a pending request with a dead daemon has its owned entry lifted and is replaced", func(t *testing.T) {
			w := newFakeWorld(t)
			entry := w.addHubEntry(PauseCaller)
			w.seed(Request{Trigger: TriggerNow, DaemonPID: 7, DaemonStart: "s7", Phase: PhaseConfirming, Owned: refOf(entry)})

			got, err := Submit(w.deps(), SubmitRequest{})

			if err != nil || got.Phase != PhaseWaiting || got.DaemonPID != w.spawned[0] || got.Owned != nil {
				t.Fatalf("got %+v, %v; want a fresh waiting request with the new pid", got, err)
			}
			if live := w.liveEntries(); len(live) != 0 {
				t.Errorf("live entries = %+v; want the owned entry lifted", live)
			}
		}},
		{"a terminal request is replaced with one spawn", func(t *testing.T) {
			for _, phase := range []Phase{PhaseDone, PhaseFailed, PhaseCancelled} {
				w := newFakeWorld(t)
				w.seed(Request{Trigger: TriggerNow, DaemonPID: 7, DaemonStart: "s7", Phase: phase, Reason: "old"})

				got, err := Submit(w.deps(), SubmitRequest{})

				if err != nil || len(w.spawned) != 1 || got.Phase != PhaseWaiting || got.Reason != "" {
					t.Errorf("replacing %s: got %+v, %v, spawned %v; want one spawn and a fresh waiting request", phase, got, err, w.spawned)
				}
			}
		}},
		{"two submits in parallel spawn one daemon and refuse the other", func(t *testing.T) {
			w := newFakeWorld(t)
			errs := make([]error, 2)
			var wg sync.WaitGroup
			for i := range errs {
				wg.Add(1)
				go func() {
					defer wg.Done()
					_, errs[i] = Submit(w.deps(), SubmitRequest{})
				}()
			}
			wg.Wait()

			refusals := 0
			for _, err := range errs {
				if err != nil {
					refusals++
					requireRefusal(t, err, "lyx hibernate cancel")
				}
			}
			if refusals != 1 || len(w.spawned) != 1 {
				t.Errorf("refusals %d, spawned %v; want one of each", refusals, w.spawned)
			}
		}},
		{"a spawn error is returned and no request is written", func(t *testing.T) {
			w := newFakeWorld(t)
			w.spawnErr = errors.New("no fork")

			_, err := Submit(w.deps(), SubmitRequest{})

			requireRefusal(t, err, "no fork")
			requireNoRequestFile(t, w)
		}},
		{"a lift error over a dead daemon's request is returned with the request untouched and no spawn", func(t *testing.T) {
			w := newFakeWorld(t)
			entry := w.addHubEntry(PauseCaller)
			w.seed(Request{Trigger: TriggerNow, DaemonPID: 7, DaemonStart: "s7", Phase: PhaseConfirming, Owned: refOf(entry)})
			before := readBytes(t, w.requestPath)
			w.liftErr = errors.New("record locked")

			_, err := Submit(w.deps(), SubmitRequest{})

			requireRefusal(t, err, "record locked")
			if string(readBytes(t, w.requestPath)) != string(before) || len(w.spawned) != 0 {
				t.Errorf("request changed or a daemon spawned (%v); want neither", w.spawned)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tt.run(t)
		})
	}
}

func TestCancel(t *testing.T) {
	tests := []struct {
		name string
		run  func(t *testing.T)
	}{
		{"no request and a terminal one succeed with nothing cancelled", func(t *testing.T) {
			w := newFakeWorld(t)

			report, err := Cancel(w.deps())

			if err != nil || report.Cancelled || len(w.killed) != 0 {
				t.Errorf("no request: %+v, %v, killed %v; want nothing cancelled", report, err, w.killed)
			}
			requireNoRequestFile(t, w)

			w.seed(Request{Trigger: TriggerNow, DaemonPID: 7, DaemonStart: "s7", Phase: PhaseDone})
			before := readBytes(t, w.requestPath)
			report, err = Cancel(w.deps())
			if err != nil || report.Cancelled || len(w.killed) != 0 || string(readBytes(t, w.requestPath)) != string(before) {
				t.Errorf("terminal request: %+v, %v, killed %v; want nothing cancelled and the file as it was", report, err, w.killed)
			}
		}},
		{"a waiting request with a live daemon is cancelled, its daemon killed and nothing lifted", func(t *testing.T) {
			for _, caller := range []string{orchStrand(t), ""} {
				w := newFakeWorld(t)
				w.caller = caller
				w.alive[7] = "s7"
				w.seed(Request{Trigger: TriggerNow, DaemonPID: 7, DaemonStart: "s7", Phase: PhaseWaiting})

				report, err := Cancel(w.deps())

				if err != nil || !report.Cancelled || report.Phase != PhaseWaiting || !report.DaemonStopped || report.Lifted != nil || !reflect.DeepEqual(w.killed, []int{7}) {
					t.Errorf("caller %q: %+v, %v, killed %v; want cancelled from waiting with the daemon stopped and nothing lifted", caller, report, err, w.killed)
				}
				label := caller
				if label == "" {
					label = requesterExternal
				}
				if got := w.request(); got.Phase != PhaseCancelled || got.Reason != "cancelled by "+label {
					t.Errorf("caller %q: request = %+v; want cancelled by %s", caller, got, label)
				}
			}
		}},
		{"a confirming request with a live daemon and an owned entry lifts the entry", func(t *testing.T) {
			w := newFakeWorld(t)
			w.alive[7] = "s7"
			entry := w.addHubEntry(PauseCaller)
			w.seed(Request{Trigger: TriggerNow, DaemonPID: 7, DaemonStart: "s7", Phase: PhaseConfirming, Owned: refOf(entry)})

			report, err := Cancel(w.deps())

			if err != nil || !report.DaemonStopped || report.Lifted == nil || !reflect.DeepEqual(report.Lifted.Resumed, []string{pauserecord.ScopeHub}) {
				t.Fatalf("%+v, %v; want the daemon stopped and the hub entry lifted", report, err)
			}
			if live := w.liveEntries(); len(live) != 0 {
				t.Errorf("live entries = %+v; want none", live)
			}
		}},
		{"an owned entry the daemon records after the hold is still lifted", func(t *testing.T) {
			w := newFakeWorld(t)
			w.alive[7] = "s7"
			w.seed(Request{Trigger: TriggerNow, DaemonPID: 7, DaemonStart: "s7", Phase: PhasePausing})
			entry := w.addHubEntry(PauseCaller)
			w.onKill = func(int) {
				err := updateRequest(w.requestPath, func(cur Request, _ bool) (Request, error) {
					cur.Owned = refOf(entry)
					return cur, nil
				})
				if err != nil {
					t.Error(err)
				}
			}

			report, err := Cancel(w.deps())

			if err != nil || report.Lifted == nil || len(w.liveEntries()) != 0 {
				t.Errorf("%+v, %v, live %+v; want the late entry lifted", report, err, w.liveEntries())
			}
		}},
		{"a hibernating request with a live daemon is refused and nothing is killed or lifted", func(t *testing.T) {
			w := newFakeWorld(t)
			w.alive[7] = "s7"
			entry := w.addHubEntry(PauseCaller)
			w.seed(Request{Trigger: TriggerNow, DaemonPID: 7, DaemonStart: "s7", Phase: PhaseHibernating, Owned: refOf(entry)})
			before := readBytes(t, w.requestPath)

			_, err := Cancel(w.deps())

			requireRefusal(t, err, "lyx hibernate status")
			if len(w.killed) != 0 || len(w.liveEntries()) != 1 || string(readBytes(t, w.requestPath)) != string(before) {
				t.Errorf("killed %v, live %+v; want nothing killed, the entry live and the file as it was", w.killed, w.liveEntries())
			}
		}},
		{"a hibernating request with a dead daemon is cancelled and its owned entry lifted", func(t *testing.T) {
			w := newFakeWorld(t)
			entry := w.addHubEntry(PauseCaller)
			w.seed(Request{Trigger: TriggerNow, DaemonPID: 7, DaemonStart: "s7", Phase: PhaseHibernating, Owned: refOf(entry)})

			report, err := Cancel(w.deps())

			if err != nil || !report.Cancelled || report.Phase != PhaseHibernating || report.DaemonStopped || report.Lifted == nil || len(w.killed) != 0 || len(w.liveEntries()) != 0 {
				t.Errorf("%+v, %v, killed %v, live %+v; want cancelled with the entry lifted and nothing killed", report, err, w.killed, w.liveEntries())
			}
		}},
		{"a request owning no entry lifts nothing and leaves another caller's hub entry live", func(t *testing.T) {
			w := newFakeWorld(t)
			w.alive[7] = "s7"
			w.addHubEntry("EXTERNAL")
			w.seed(Request{Trigger: TriggerNow, DaemonPID: 7, DaemonStart: "s7", Phase: PhaseConfirming})

			report, err := Cancel(w.deps())

			if err != nil || !report.Cancelled || report.Lifted != nil || len(w.liveEntries()) != 1 {
				t.Errorf("%+v, %v, live %+v; want nothing lifted and the entry live", report, err, w.liveEntries())
			}
		}},
		{"an agent caller and a non-Linux platform are refused", func(t *testing.T) {
			w := newFakeWorld(t)
			w.caller = agentStrand(t)
			_, err := Cancel(w.deps())
			requireRefusal(t, err, "ask your parent session")

			w = newFakeWorld(t)
			w.goos = "windows"
			_, err = Cancel(w.deps())
			requireRefusal(t, err, "Linux only")
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tt.run(t)
		})
	}
}

func TestStatus(t *testing.T) {
	tests := []struct {
		name string
		run  func(t *testing.T)
	}{
		{"no request reports a null request", func(t *testing.T) {
			w := newFakeWorld(t)

			report, err := Status(w.deps())

			if err != nil || report.Request != nil || report.DaemonLive {
				t.Errorf("%+v, %v; want no request and no daemon", report, err)
			}
		}},
		{"a request is reported whole with its daemon's liveness, and the call writes nothing", func(t *testing.T) {
			entry := pauserecord.Entry{Scope: pauserecord.ScopeHub, Caller: PauseCaller, PausedAt: newFakeWorld(t).wall}
			for _, live := range []bool{true, false} {
				w := newFakeWorld(t)
				if live {
					w.alive[7] = "s7"
				}
				want := Request{Trigger: TriggerAfter, Slug: "p1", Requester: "EXTERNAL", RequestedAt: w.wall, DaemonPID: 7, DaemonStart: "s7", Owned: refOf(entry), Phase: PhaseWaiting, Reason: "waiting for p1"}
				w.seed(want)
				before := readBytes(t, w.requestPath)
				listing := func() []string {
					infos, err := os.ReadDir(filepath.Dir(w.requestPath))
					if err != nil {
						t.Fatal(err)
					}
					var names []string
					for _, info := range infos {
						names = append(names, info.Name())
					}
					return names
				}
				listed := listing()

				report, err := Status(w.deps())

				if err != nil || report.Request == nil || !reflect.DeepEqual(*report.Request, want) || report.DaemonLive != live {
					t.Errorf("live %v: %+v, %v; want the request whole and daemon_live %v", live, report, err, live)
				}
				if string(readBytes(t, w.requestPath)) != string(before) || !reflect.DeepEqual(listing(), listed) {
					t.Errorf("live %v: the file or the directory listing changed", live)
				}
			}
		}},
		{"a non-Linux platform still reads", func(t *testing.T) {
			w := newFakeWorld(t)
			w.goos = "darwin"
			w.seed(Request{Trigger: TriggerNow, DaemonPID: 7, DaemonStart: "s7", Phase: PhaseDone})

			report, err := Status(w.deps())

			if err != nil || report.Request == nil || report.Request.Phase != PhaseDone {
				t.Errorf("%+v, %v; want the done request", report, err)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tt.run(t)
		})
	}
}
