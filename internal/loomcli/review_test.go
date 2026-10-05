package loomcli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/parentreview"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/testkit/envelope"
)

// newReviewStore builds a Store over fresh temp directories.
func newReviewStore(t *testing.T) parentreview.Store {
	t.Helper()
	dir := t.TempDir()
	return parentreview.Store{Root: filepath.Join(dir, "reviews"), LockDir: filepath.Join(dir, "locks")}
}

// openReviewRequest opens a request on s's latest round.
func openReviewRequest(t *testing.T, s parentreview.Store) {
	t.Helper()
	if _, err := s.OpenRequest(parentreview.OpenSpec{Slug: "task", Reviewer: "ab:hub", Brief: "brief"}); err != nil {
		t.Fatalf("OpenRequest: %v", err)
	}
}

// writeReviewFile writes a review file with body and returns its path.
func writeReviewFile(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "review.md")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// assertReviewRefusal asserts a refused envelope whose message holds wantMsg and a way-forward clause.
func assertReviewRefusal(t *testing.T, code int, out *bytes.Buffer, wantMsg string) {
	t.Helper()
	if code == 0 {
		t.Fatalf("exit code = 0; want a refusal (output %q)", out.String())
	}
	msg := envelope.Decode(t, out.String()).Error
	if !strings.Contains(msg, wantMsg) {
		t.Errorf("error = %q; want it to contain %q", msg, wantMsg)
	}
	if !strings.Contains(msg, "way forward:") {
		t.Errorf("error = %q; want a way-forward clause", msg)
	}
}

func TestReviewVerbs_Success(t *testing.T) {
	t.Run("notify", func(t *testing.T) {
		s := newReviewStore(t)
		openReviewRequest(t, s)
		var out bytes.Buffer
		if code := reviewNotifyVerb(&out, s, "task"); code != 0 {
			t.Fatalf("notify exit = %d, output %q", code, out.String())
		}
		r, _, _ := s.Latest()
		if r.Delivery.WaitingNotifys != 1 {
			t.Errorf("WaitingNotifys = %d; want 1", r.Delivery.WaitingNotifys)
		}
	})
	t.Run("delivered", func(t *testing.T) {
		s := newReviewStore(t)
		openReviewRequest(t, s)
		var out bytes.Buffer
		if code := reviewDeliveredVerb(&out, s, "task", ""); code != 0 {
			t.Fatalf("delivered exit = %d, output %q", code, out.String())
		}
		r, _, _ := s.Latest()
		if r.Delivery.DeliveredAt.IsZero() {
			t.Error("DeliveredAt is zero; want it stamped")
		}
	})
	t.Run("delivered failed", func(t *testing.T) {
		s := newReviewStore(t)
		openReviewRequest(t, s)
		var out bytes.Buffer
		if code := reviewDeliveredVerb(&out, s, "task", "no such agent"); code != 0 {
			t.Fatalf("delivered --failed exit = %d, output %q", code, out.String())
		}
		r, _, _ := s.Latest()
		if r.Delivery.FailedReason != "no such agent" || !r.Delivery.DeliveredAt.IsZero() {
			t.Errorf("Delivery = %+v; want the reason recorded and no delivery stamp", r.Delivery)
		}
	})
	t.Run("approve", func(t *testing.T) {
		s := newReviewStore(t)
		openReviewRequest(t, s)
		var out bytes.Buffer
		if code := reviewApproveVerb(&out, s, "task", "", fakeRunStatus(shedengine.StateRunning)); code != 0 {
			t.Fatalf("approve exit = %d, output %q", code, out.String())
		}
		r, _, _ := s.Latest()
		if r.Verdict == nil || r.Verdict.Kind != parentreview.VerdictApprove {
			t.Errorf("Verdict = %+v; want approve", r.Verdict)
		}
	})
	t.Run("approve with review file", func(t *testing.T) {
		s := newReviewStore(t)
		openReviewRequest(t, s)
		var out bytes.Buffer
		if code := reviewApproveVerb(&out, s, "task", writeReviewFile(t, "looks good"), fakeRunStatus(shedengine.StateRunning)); code != 0 {
			t.Fatalf("approve --review exit = %d, output %q", code, out.String())
		}
		r, _, _ := s.Latest()
		if data, err := os.ReadFile(r.ReviewPath()); err != nil || string(data) != "looks good" {
			t.Errorf("review.md = %q, %v; want the copied review", data, err)
		}
	})
	t.Run("reject", func(t *testing.T) {
		s := newReviewStore(t)
		openReviewRequest(t, s)
		var out bytes.Buffer
		if code := reviewRejectVerb(&out, s, "task", writeReviewFile(t, "fix the intro")); code != 0 {
			t.Fatalf("reject exit = %d, output %q", code, out.String())
		}
		r, _, _ := s.Latest()
		if r.Verdict == nil || r.Verdict.Kind != parentreview.VerdictReject {
			t.Errorf("Verdict = %+v; want reject", r.Verdict)
		}
		if data, err := os.ReadFile(r.ReviewPath()); err != nil || string(data) != "fix the intro" {
			t.Errorf("review.md = %q, %v; want the copied findings", data, err)
		}
	})
}

// reviewVerbCalls runs each of the four verbs against s, keyed by verb name.
func reviewVerbCalls(t *testing.T, s parentreview.Store) map[string]func(out *bytes.Buffer) int {
	t.Helper()
	review := writeReviewFile(t, "findings")
	return map[string]func(out *bytes.Buffer) int{
		"notify":    func(out *bytes.Buffer) int { return reviewNotifyVerb(out, s, "task") },
		"delivered": func(out *bytes.Buffer) int { return reviewDeliveredVerb(out, s, "task", "") },
		"approve": func(out *bytes.Buffer) int {
			return reviewApproveVerb(out, s, "task", "", fakeRunStatus(shedengine.StateRunning))
		},
		"reject": func(out *bytes.Buffer) int { return reviewRejectVerb(out, s, "task", review) },
	}
}

func TestReviewVerbs_NoOpenRequest(t *testing.T) {
	for _, verb := range []string{"notify", "delivered", "approve", "reject"} {
		t.Run(verb, func(t *testing.T) {
			s := newReviewStore(t)
			var out bytes.Buffer
			code := reviewVerbCalls(t, s)[verb](&out)
			assertReviewRefusal(t, code, &out, "no open review request")
			if msg := reviewErrMsg(t, &out); !strings.Contains(msg, "lyx loom status <run>") {
				t.Errorf("error %q: way forward must point at lyx loom status", msg)
			}
		})
	}
}

func TestReviewVerbs_VerdictAlreadyRecorded(t *testing.T) {
	for _, verb := range []string{"notify", "approve", "reject"} {
		t.Run(verb, func(t *testing.T) {
			s := newReviewStore(t)
			openReviewRequest(t, s)
			if err := s.RecordVerdict(parentreview.VerdictApprove, ""); err != nil {
				t.Fatal(err)
			}
			var out bytes.Buffer
			code := reviewVerbCalls(t, s)[verb](&out)
			assertReviewRefusal(t, code, &out, "verdict is already recorded")
			if !strings.Contains(out.String(), "proceeds on its own") {
				t.Errorf("output %q: way forward must say the run proceeds on its own", out.String())
			}
		})
	}
}

func TestReviewVerbs_Expired(t *testing.T) {
	for _, verb := range []string{"notify", "delivered", "approve", "reject"} {
		t.Run(verb, func(t *testing.T) {
			s := newReviewStore(t)
			openReviewRequest(t, s)
			if err := s.MarkExpired(); err != nil {
				t.Fatal(err)
			}
			var out bytes.Buffer
			code := reviewVerbCalls(t, s)[verb](&out)
			assertReviewRefusal(t, code, &out, "expired")
			if !strings.Contains(out.String(), "Discussion-Review still reviews") {
				t.Errorf("output %q: way forward must say Discussion-Review still reviews", out.String())
			}
		})
	}
}

func TestReviewVerbs_ReviewFileMissingOrEmpty(t *testing.T) {
	empty := writeReviewFile(t, "  \n")
	missing := filepath.Join(t.TempDir(), "absent.md")
	tests := []struct {
		name string
		call func(s parentreview.Store, out *bytes.Buffer) int
	}{
		{"reject empty", func(s parentreview.Store, out *bytes.Buffer) int { return reviewRejectVerb(out, s, "task", empty) }},
		{"reject missing", func(s parentreview.Store, out *bytes.Buffer) int { return reviewRejectVerb(out, s, "task", missing) }},
		{"reject none", func(s parentreview.Store, out *bytes.Buffer) int { return reviewRejectVerb(out, s, "task", "") }},
		{"approve missing", func(s parentreview.Store, out *bytes.Buffer) int {
			return reviewApproveVerb(out, s, "task", missing, fakeRunStatus(shedengine.StateRunning))
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := newReviewStore(t)
			openReviewRequest(t, s)
			var out bytes.Buffer
			assertReviewRefusal(t, tc.call(s, &out), &out, "review file is missing or empty")
			if !strings.Contains(out.String(), "re-run") {
				t.Errorf("output %q: way forward must name re-running the verb", out.String())
			}
		})
	}
}

// fakeReviewTarget builds reviewTargetDeps over a hub with the given worktrees.
func fakeReviewTarget(prime string, existing ...string) reviewTargetDeps {
	return reviewTargetDeps{
		primeName: func(*lyxcwd.Location) (string, error) { return prime, nil },
		dirExists: func(path string) bool {
			for _, e := range existing {
				if filepath.Base(path) == e {
					return true
				}
			}
			return false
		},
		resolveWorktree: func(root string) (*lyxcwd.Location, error) {
			return &lyxcwd.Location{HubPath: filepath.Dir(root), WorktreeName: filepath.Base(root)}, nil
		},
	}
}

func TestResolveReviewTarget(t *testing.T) {
	hub := filepath.Join(t.TempDir(), "repo-LYXHUB")
	atPrime := &lyxcwd.Location{HubPath: hub, WorktreeName: "repo"}
	atTask := &lyxcwd.Location{HubPath: hub, WorktreeName: "task-a"}
	d := fakeReviewTarget("repo", "task-a", "task-b")

	for _, group := range []string{"review", "circling", "decision"} {
		t.Run(group, func(t *testing.T) {
			t.Run("task worktree addresses itself", func(t *testing.T) {
				got, err := resolveReviewTarget(group, "notify", atTask, "", d)
				if err != nil || got != atTask {
					t.Fatalf("got %v, %v; want the invoking location", got, err)
				}
			})
			t.Run("slug from the prime", func(t *testing.T) {
				got, err := resolveReviewTarget(group, "notify", atPrime, "task-b", d)
				if err != nil || got.WorktreeName != "task-b" {
					t.Fatalf("got %v, %v; want task-b", got, err)
				}
			})
			t.Run("slug required from the prime", func(t *testing.T) {
				_, err := resolveReviewTarget(group, "notify", atPrime, "", d)
				if err == nil || !strings.Contains(err.Error(), "loom: "+group+" notify: slug required from the prime") || !strings.Contains(err.Error(), "way forward:") || !strings.Contains(err.Error(), "lyx board list") || !strings.Contains(err.Error(), "lyx loom "+group+" notify <slug>") {
					t.Fatalf("err = %v; want the slug-required refusal naming %s and lyx board list", err, group)
				}
			})
			t.Run("unknown slug", func(t *testing.T) {
				_, err := resolveReviewTarget(group, "notify", atTask, "nope", d)
				if err == nil || !strings.Contains(err.Error(), "loom: "+group+" notify: unknown slug \"nope\"") || !strings.Contains(err.Error(), "way forward:") || !strings.Contains(err.Error(), "lyx board list") {
					t.Fatalf("err = %v; want the unknown-slug refusal naming %s and lyx board list", err, group)
				}
			})
			t.Run("prime lookup failure", func(t *testing.T) {
				bad := d
				bad.primeName = func(*lyxcwd.Location) (string, error) { return "", errors.New("boom") }
				if _, err := resolveReviewTarget(group, "notify", atTask, "", bad); err == nil {
					t.Fatal("err = nil; want the prime lookup failure")
				}
			})
		})
	}
}

func TestReviewSlugArg(t *testing.T) {
	tests := []struct {
		verb string
		args []string
		want string
	}{
		{"notify", nil, ""},
		{"notify", []string{"s"}, "s"},
		{"reject", []string{"review.md"}, ""},
		{"reject", []string{"s", "review.md"}, "s"},
	}
	for _, tc := range tests {
		if got := reviewSlugArg(tc.verb, tc.args); got != tc.want {
			t.Errorf("reviewSlugArg(%q, %v) = %q; want %q", tc.verb, tc.args, got, tc.want)
		}
	}
}

// reviewErrMsg decodes the error text of a refused review envelope.
func reviewErrMsg(t *testing.T, out *bytes.Buffer) string {
	t.Helper()
	return envelope.Decode(t, out.String()).Error
}

// rejectCappedRound opens a round with a request carrying cap and rejects it.
func rejectCappedRound(t *testing.T, s parentreview.Store, cap int) {
	t.Helper()
	if _, err := s.BeginRound(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.OpenRequest(parentreview.OpenSpec{Slug: "task", Reviewer: "ab:hub", Brief: "brief", Cap: cap}); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordVerdict(parentreview.VerdictReject, writeReviewFile(t, "fix it")); err != nil {
		t.Fatal(err)
	}
}

// fakeRunStatus is a status reader answering with a run in state st.
func fakeRunStatus(st shedengine.State) reviewStatusReader {
	return func() (shedengine.Status, bool, error) {
		return shedengine.Status{State: st}, true, nil
	}
}

func TestReviewApprove_SupersedesCapRejectOnBlockedRun(t *testing.T) {
	s := newReviewStore(t)
	rejectCappedRound(t, s, 2)
	rejectCappedRound(t, s, 2)
	var out bytes.Buffer
	if code := reviewApproveVerb(&out, s, "task", "", fakeRunStatus(shedengine.StateBlocked)); code != 0 {
		t.Fatalf("exit = %d, output %q", code, out.String())
	}
	env := envelope.Decode(t, out.String())
	if env.Raw["action"] != "approve" || env.Raw["superseded"] != true {
		t.Errorf("envelope = %v; want action approve and superseded true", env)
	}
	if msg, _ := env.Raw["message"].(string); !strings.Contains(msg, "lyx loom start") {
		t.Errorf("message = %q; want the way forward lyx loom start", msg)
	}
	r, _, _ := s.Latest()
	if r.Verdict == nil || r.Verdict.Kind != parentreview.VerdictApprove || !r.Verdict.Superseding {
		t.Errorf("verdict = %+v; want a superseding approve", r.Verdict)
	}
	if b, _ := os.ReadFile(r.ReviewPath()); string(b) != "fix it" {
		t.Errorf("review.md = %q; want the cap's review kept", b)
	}
}

func TestReviewApprove_SupersedeRefusals(t *testing.T) {
	t.Run("run not blocked", func(t *testing.T) {
		s := newReviewStore(t)
		rejectCappedRound(t, s, 1)
		var out bytes.Buffer
		code := reviewApproveVerb(&out, s, "task", "", fakeRunStatus(shedengine.StateRunning))
		assertReviewRefusal(t, code, &out, "only on a run halted at the reject cap")
		if msg := reviewErrMsg(t, &out); !strings.Contains(msg, "lyx loom status <slug>") {
			t.Errorf("error %q: way forward must point at lyx loom status", msg)
		}
	})
	t.Run("below the cap", func(t *testing.T) {
		s := newReviewStore(t)
		rejectCappedRound(t, s, 3)
		var out bytes.Buffer
		code := reviewApproveVerb(&out, s, "task", "", fakeRunStatus(shedengine.StateBlocked))
		assertReviewRefusal(t, code, &out, "not the cap's")
	})
	t.Run("review file given", func(t *testing.T) {
		s := newReviewStore(t)
		rejectCappedRound(t, s, 1)
		var out bytes.Buffer
		code := reviewApproveVerb(&out, s, "task", writeReviewFile(t, "other"), fakeRunStatus(shedengine.StateBlocked))
		assertReviewRefusal(t, code, &out, "--review")
		if msg := reviewErrMsg(t, &out); !strings.Contains(msg, "without --review") {
			t.Errorf("error %q: way forward must say to re-run without --review", msg)
		}
	})
}

func TestReviewApprove_OrdinaryApproveIgnoresRunStatus(t *testing.T) {
	s := newReviewStore(t)
	openReviewRequest(t, s)
	var out bytes.Buffer
	if code := reviewApproveVerb(&out, s, "task", "", fakeRunStatus(shedengine.StateRunning)); code != 0 {
		t.Fatalf("exit = %d, output %q", code, out.String())
	}
	if env := envelope.Decode(t, out.String()); env.Raw["superseded"] != nil {
		t.Errorf("envelope = %v; an ordinary approve must not report superseded", env)
	}
	r, _, _ := s.Latest()
	if r.Verdict == nil || r.Verdict.Kind != parentreview.VerdictApprove || r.Verdict.Superseding {
		t.Errorf("verdict = %+v; want a plain approve", r.Verdict)
	}
}
