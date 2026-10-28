//go:build tier0

package shedtransient

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/google/go-github/v75/github"
)

func ghError(status int) error {
	return &github.ErrorResponse{Response: &http.Response{StatusCode: status}}
}

// TestClassAndMark pins the class of each failure shape and that Mark adds exactly that class.
// Its last row notes a spawn refusal in proc's process-wide record, so the test and its rows run in order and never in parallel.
// The died row before it therefore sees no refusal noted.
func TestClassAndMark(t *testing.T) {
	notStarted := fmt.Errorf("outer: %w", fmt.Errorf("inner: %w", shuttleengine.ErrNotStarted))
	transport := fmt.Errorf("publish: %w", &gitexec.GitError{
		Args:     []string{"-c", "push.autoSetupRemote=true", "push"},
		ExitCode: 128,
		Stderr:   "fatal: unable to access: Could not resolve host: github.com",
	})
	rejected := &gitexec.GitError{
		Args:     []string{"push"},
		ExitCode: 1,
		Stderr:   "! [rejected] main -> main (non-fast-forward)",
	}
	marked := shedengine.MarkTransient(shedengine.TransientGitHubAPI, fmt.Errorf("x: %w", shuttleengine.ErrNotStarted))
	plain := errors.New("boom")
	refused := fmt.Errorf("read task patch: %w", &os.PathError{Op: "fork/exec", Path: "/usr/bin/git", Err: syscall.EAGAIN})
	died := fmt.Errorf("seats: shuttle run outcome died: %w", shuttleengine.ErrAgentDied)

	tests := []struct {
		name string
		err  error
		want shedengine.TransientClass
		// before runs ahead of the row, such as noting a spawn refusal.
		before func()
	}{
		{name: "not started wrapped twice", err: notStarted, want: shedengine.TransientAgentStart},
		{name: "git transport", err: transport, want: shedengine.TransientGitTransport},
		{name: "rejected push", err: rejected},
		{name: "github 503", err: ghError(http.StatusServiceUnavailable), want: shedengine.TransientGitHubAPI},
		{name: "github 404", err: ghError(http.StatusNotFound)},
		{name: "rate limit", err: &github.RateLimitError{}},
		{name: "existing mark wins", err: marked, want: shedengine.TransientGitHubAPI},
		{name: "plain", err: plain},
		{name: "nil", err: nil},
		{name: "spawn refused for EAGAIN", err: refused, want: shedengine.TransientHostResources},
		{name: "agent died with no spawn refusal noted", err: died},
		{name: "agent died after a spawn refusal", err: died, want: shedengine.TransientHostResources, before: func() { proc.NoteSpawnAgain(syscall.EAGAIN) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.before != nil {
				tt.before()
			}
			if got := Class(tt.err); got != tt.want {
				t.Errorf("Class = %q, want %q", got, tt.want)
			}
			got := Mark(tt.err)
			if class := shedengine.TransientOf(got); class != tt.want {
				t.Errorf("TransientOf(Mark) = %q, want %q", class, tt.want)
			}
			if (tt.want == "" || tt.err == marked) && got != tt.err {
				t.Errorf("Mark of a non-transient or already marked error must return it unchanged, got %v", got)
			}
		})
	}
}

// TestSpawnRefusedRecently pins the bound of the refusal window: a refusal noted at the edge of it counts, one beyond it does not.
// It notes a spawn refusal in proc's process-wide record, so it runs in order after TestClassAndMark and never in parallel.
func TestSpawnRefusedRecently(t *testing.T) {
	proc.NoteSpawnAgain(syscall.EAGAIN)
	noted, ok := proc.LastSpawnAgain()
	if !ok {
		t.Fatal("proc.LastSpawnAgain() noted nothing after NoteSpawnAgain(EAGAIN)")
	}

	tests := []struct {
		name string
		age  time.Duration
		want bool
	}{
		{"JustNow", 0, true},
		{"JustInsideTheWindow", spawnRefusalWindow - time.Nanosecond, true},
		{"AtTheWindow", spawnRefusalWindow, true},
		{"JustBeyondTheWindow", spawnRefusalWindow + time.Nanosecond, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := spawnRefusedRecently(noted.Add(tt.age)); got != tt.want {
				t.Errorf("spawnRefusedRecently(refusal + %s) = %v; want %v", tt.age, got, tt.want)
			}
		})
	}
}
