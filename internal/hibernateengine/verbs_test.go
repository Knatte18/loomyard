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
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
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
	strand, err := agentname.Format("lym", "p1", "driver")
	if err != nil {
		t.Fatal(err)
	}
	return strand
}

func orchStrand(t *testing.T) string {
	t.Helper()
	strand, err := agentname.Format("lym", "", agentname.RoleOrch)
	if err != nil {
		t.Fatal(err)
	}
	return strand
}

func beamOrchStrand(t *testing.T) string {
	t.Helper()
	strand, err := agentname.Format("lym", "b", agentname.RoleOrch)
	if err != nil {
		t.Fatal(err)
	}
	return strand
}

func running(producer string) RunStatus {
	return RunStatus{State: shedengine.StateRunning, Producer: producer}
}

func TestSubmit(t *testing.T) {
	tiergate.Require(t, 0)
	tests := []struct {
		name string
		run  func(t *testing.T)
	}{
		{"no trigger writes a waiting now request no daemon carries and reports the prime's dobby live", func(t *testing.T) {
			w := newFakeWorld(t)

			got, err := Submit(w.deps(), SubmitRequest{})

			want := Request{Trigger: TriggerNow, Requester: requesterExternal, RequestedAt: w.wall, Phase: PhaseWaiting}
			if err != nil || !reflect.DeepEqual(got, SubmitReport{Request: want, DobbyLive: true}) || !reflect.DeepEqual(w.request(), want) {
				t.Errorf("returned %+v, %v and written %+v; want %+v with the dobby live", got, err, w.request(), want)
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

				if err != nil || got.Request.Trigger != tc.wantTrigger || got.Request.Slug != tc.wantSlug {
					t.Errorf("%+v: got %+v, %v; want trigger %s with slug %q", tc.req, got, err, tc.wantTrigger, tc.wantSlug)
				}
			}
		}},
		{"the resume-on-wake choice is written into the request", func(t *testing.T) {
			for _, resume := range []bool{true, false} {
				w := newFakeWorld(t)

				got, err := Submit(w.deps(), SubmitRequest{ResumeOnWake: resume})

				if err != nil || got.Request.ResumeOnWake != resume || w.request().ResumeOnWake != resume {
					t.Errorf("Submit(ResumeOnWake %v) = %+v, %v, written %+v; want the choice returned and written", resume, got, err, w.request())
				}
			}
		}},
		{"a prime's dobby that does not run, or whose liveness does not read, is named with lym beam start root and the request is still written", func(t *testing.T) {
			for _, liveErr := range []error{nil, errors.New("lock unreadable")} {
				w := newFakeWorld(t)
				w.dobbyLive, w.dobbyLiveErr = false, liveErr

				got, err := Submit(w.deps(), SubmitRequest{})

				if err != nil || got.DobbyLive || got.WayForward != dobbyDownWayForward || !strings.Contains(got.WayForward, "lym beam start root") {
					t.Errorf("liveness error %v: got %+v, %v; want the dobby reported down with lym beam start root", liveErr, got, err)
				}
				if w.request().Phase != PhaseWaiting {
					t.Errorf("liveness error %v: request = %+v; want a waiting request written", liveErr, w.request())
				}
			}
		}},
		{"an agent caller is refused naming the parent session, the prime's orch, a beam's orch and an external caller are not", func(t *testing.T) {
			for _, tc := range []struct {
				caller  string
				refused bool
			}{{agentStrand(t), true}, {"not-a-parsable-name", true}, {orchStrand(t), false}, {beamOrchStrand(t), false}, {"", false}} {
				w := newFakeWorld(t)
				w.caller = tc.caller

				_, err := Submit(w.deps(), SubmitRequest{})

				if tc.refused {
					requireRefusal(t, err, "ask your parent session")
					requireNoRequestFile(t, w)
				} else if err != nil {
					t.Errorf("caller %q: err = %v; want none", tc.caller, err)
				}
			}
		}},
		{"a refused submit writes no request", func(t *testing.T) {
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
				requireNoRequestFile(t, w)
			}
		}},
		{"a pending request is refused and left as it was, its pause entry live, whether or not a live daemon carries it", func(t *testing.T) {
			for _, live := range []bool{true, false} {
				w := newFakeWorld(t)
				if live {
					w.alive[7] = "s7"
				}
				entry := w.addHubEntry(PauseCaller)
				w.seed(Request{Trigger: TriggerNow, DaemonPID: 7, DaemonStart: "s7", Phase: PhaseConfirming, Owned: refOf(entry)})
				before := readBytes(t, w.requestPath)

				_, err := Submit(w.deps(), SubmitRequest{})

				requireRefusal(t, err, "lym hibernate cancel", "confirming")
				if string(readBytes(t, w.requestPath)) != string(before) || len(w.liveEntries()) != 1 {
					t.Errorf("live %v: the request changed or the entry was lifted (live %+v); want neither", live, w.liveEntries())
				}
			}
		}},
		{"a terminal request is replaced by a waiting request no daemon carries", func(t *testing.T) {
			for _, phase := range []Phase{PhaseDone, PhaseFailed, PhaseCancelled} {
				w := newFakeWorld(t)
				w.alive[7] = "s7"
				w.seed(Request{Trigger: TriggerNow, DaemonPID: 7, DaemonStart: "s7", Phase: phase, Reason: "old"})

				got, err := Submit(w.deps(), SubmitRequest{})

				if err != nil || got.Request.Phase != PhaseWaiting || got.Request.Reason != "" || got.Request.DaemonPID != 0 || w.request().DaemonPID != 0 {
					t.Errorf("replacing %s: got %+v, %v; want a fresh waiting request no daemon carries", phase, got, err)
				}
			}
		}},
		{"a terminal request whose owned entry is still live is refused and left as it was, and one whose entry was lifted is replaced", func(t *testing.T) {
			for _, phase := range []Phase{PhaseDone, PhaseFailed} {
				w := newFakeWorld(t)
				entry := w.addHubEntry(PauseCaller)
				w.seed(Request{Trigger: TriggerNow, DaemonPID: 7, DaemonStart: "s7", Phase: phase, Owned: refOf(entry)})
				before := readBytes(t, w.requestPath)

				_, err := Submit(w.deps(), SubmitRequest{})

				requireRefusal(t, err, "lym hibernate cancel", "lym tender resume", string(phase))
				if string(readBytes(t, w.requestPath)) != string(before) || len(w.liveEntries()) != 1 {
					t.Errorf("%s: the request changed or the entry was lifted; want neither", phase)
				}

				if _, err := w.lift(*refOf(entry)); err != nil {
					t.Fatal(err)
				}
				got, err := Submit(w.deps(), SubmitRequest{})

				if err != nil || got.Request.Phase != PhaseWaiting {
					t.Errorf("%s with the entry lifted: got %+v, %v; want a fresh waiting request", phase, got, err)
				}
			}
		}},
		{"a terminal request that owns an entry is refused with a way forward when the hub cannot be read", func(t *testing.T) {
			w := newFakeWorld(t)
			entry := w.addHubEntry(PauseCaller)
			w.seed(Request{Trigger: TriggerNow, DaemonPID: 7, DaemonStart: "s7", Phase: PhaseDone, Owned: refOf(entry)})
			before := readBytes(t, w.requestPath)
			w.readings = []statusReading{{err: errors.New("pane read failed")}}

			_, err := Submit(w.deps(), SubmitRequest{})

			requireRefusal(t, err, "pane read failed", "way forward", "lym tender status")
			if string(readBytes(t, w.requestPath)) != string(before) {
				t.Errorf("the request changed; want it as it was")
			}
		}},
		{"two submits in parallel write one request and refuse the other", func(t *testing.T) {
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
					requireRefusal(t, err, "lym hibernate cancel")
				}
			}
			if refusals != 1 || w.request().Phase != PhaseWaiting {
				t.Errorf("refusals %d, request %+v; want one refusal and a waiting request", refusals, w.request())
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
	tiergate.Require(t, 0)
	tests := []struct {
		name string
		run  func(t *testing.T)
	}{
		{"no request and a terminal one succeed with nothing cancelled", func(t *testing.T) {
			w := newFakeWorld(t)

			report, err := Cancel(w.deps())

			if err != nil || report.Cancelled {
				t.Errorf("no request: %+v, %v; want nothing cancelled", report, err)
			}
			requireNoRequestFile(t, w)

			w.seed(Request{Trigger: TriggerNow, DaemonPID: 7, DaemonStart: "s7", Phase: PhaseDone})
			before := readBytes(t, w.requestPath)
			report, err = Cancel(w.deps())
			if err != nil || report.Cancelled || report.Lifted != nil || string(readBytes(t, w.requestPath)) != string(before) {
				t.Errorf("terminal request: %+v, %v; want nothing cancelled or lifted and the file as it was", report, err)
			}
		}},
		{"a terminal request whose owned entry is still live has the entry lifted, nothing cancelled and the file left as it was", func(t *testing.T) {
			for _, phase := range []Phase{PhaseDone, PhaseFailed, PhaseCancelled} {
				w := newFakeWorld(t)
				entry := w.addHubEntry(PauseCaller)
				w.seed(Request{Trigger: TriggerNow, DaemonPID: 7, DaemonStart: "s7", Phase: phase, Owned: refOf(entry)})
				before := readBytes(t, w.requestPath)

				report, err := Cancel(w.deps())

				if err != nil || report.Cancelled || report.Lifted == nil || !reflect.DeepEqual(report.Lifted.Resumed, []string{pauserecord.ScopeHub}) {
					t.Errorf("%s: %+v, %v; want nothing cancelled and the hub entry lifted", phase, report, err)
				}
				if live := w.liveEntries(); len(live) != 0 || string(readBytes(t, w.requestPath)) != string(before) {
					t.Errorf("%s: live %+v; want none and the file as it was", phase, live)
				}
			}
		}},
		{"a waiting request a live daemon carries is cancelled with the caller as the reason and nothing lifted", func(t *testing.T) {
			for _, caller := range []string{orchStrand(t), ""} {
				w := newFakeWorld(t)
				w.caller = caller
				w.alive[7] = "s7"
				w.seed(Request{Trigger: TriggerNow, DaemonPID: 7, DaemonStart: "s7", Phase: PhaseWaiting})

				report, err := Cancel(w.deps())

				if err != nil || !report.Cancelled || report.Phase != PhaseWaiting || report.Lifted != nil {
					t.Errorf("caller %q: %+v, %v; want cancelled from waiting with nothing lifted", caller, report, err)
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
		{"a confirming request with an owned entry has the entry lifted, whether or not a live daemon carries it", func(t *testing.T) {
			for _, live := range []bool{true, false} {
				w := newFakeWorld(t)
				if live {
					w.alive[7] = "s7"
				}
				entry := w.addHubEntry(PauseCaller)
				w.seed(Request{Trigger: TriggerNow, DaemonPID: 7, DaemonStart: "s7", Phase: PhaseConfirming, Owned: refOf(entry)})

				report, err := Cancel(w.deps())

				if err != nil || !report.Cancelled || report.Lifted == nil || !reflect.DeepEqual(report.Lifted.Resumed, []string{pauserecord.ScopeHub}) || len(w.liveEntries()) != 0 {
					t.Errorf("live %v: %+v, %v, live entries %+v; want cancelled with the hub entry lifted", live, report, err, w.liveEntries())
				}
			}
		}},
		{"a hibernating request a live daemon carries is refused and nothing is lifted", func(t *testing.T) {
			w := newFakeWorld(t)
			w.alive[7] = "s7"
			entry := w.addHubEntry(PauseCaller)
			w.seed(Request{Trigger: TriggerNow, DaemonPID: 7, DaemonStart: "s7", Phase: PhaseHibernating, Owned: refOf(entry)})
			before := readBytes(t, w.requestPath)

			_, err := Cancel(w.deps())

			requireRefusal(t, err, "lym hibernate status")
			if len(w.liveEntries()) != 1 || string(readBytes(t, w.requestPath)) != string(before) {
				t.Errorf("live %+v; want the entry live and the file as it was", w.liveEntries())
			}
		}},
		{"a hibernating request no live daemon carries is cancelled and its owned entry lifted", func(t *testing.T) {
			w := newFakeWorld(t)
			entry := w.addHubEntry(PauseCaller)
			w.seed(Request{Trigger: TriggerNow, DaemonPID: 7, DaemonStart: "s7", Phase: PhaseHibernating, Owned: refOf(entry)})

			report, err := Cancel(w.deps())

			if err != nil || !report.Cancelled || report.Phase != PhaseHibernating || report.Lifted == nil || len(w.liveEntries()) != 0 {
				t.Errorf("%+v, %v, live %+v; want cancelled with the entry lifted", report, err, w.liveEntries())
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
	tiergate.Require(t, 0)
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
		{"a request is reported whole with its carrier's liveness, and the call writes nothing", func(t *testing.T) {
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
