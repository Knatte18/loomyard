package shedtransient

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/google/go-github/v75/github"
)

func ghError(status int) error {
	return &github.ErrorResponse{Response: &http.Response{StatusCode: status}}
}

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

	tests := []struct {
		name string
		err  error
		want shedengine.TransientClass
	}{
		{"not started wrapped twice", notStarted, shedengine.TransientAgentStart},
		{"git transport", transport, shedengine.TransientGitTransport},
		{"rejected push", rejected, ""},
		{"github 503", ghError(http.StatusServiceUnavailable), shedengine.TransientGitHubAPI},
		{"github 404", ghError(http.StatusNotFound), ""},
		{"rate limit", &github.RateLimitError{}, ""},
		{"existing mark wins", marked, shedengine.TransientGitHubAPI},
		{"plain", plain, ""},
		{"nil", nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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
