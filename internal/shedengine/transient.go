// transient.go declares the transient mark: a class a producer boundary attaches to an error so a caller can tell an infrastructure failure a single re-step may clear from every other stop.
// The mark is stdlib-only on purpose (Shed Producer-Seam Invariant);
// the classifier that decides which failure gets which class is told to Shed, never imported here.

package shedengine

import "errors"

// TransientClass names one kind of transient infrastructure failure.
// The set is closed at the three constants below, and the empty class means "not transient".
// A failure qualifies only when the same step, run again unchanged, can plausibly succeed;
// a rejected credential, a merge conflict, a missing branch or any verdict never does.
type TransientClass string

const (
	// TransientGitTransport is a git network or transport failure: the remote was unreachable or the connection dropped mid-operation.
	// A non-fast-forward rejection or an auth failure never qualifies.
	TransientGitTransport TransientClass = "git-transport"
	// TransientGitHubAPI is a GitHub API failure that a retry can clear: a network error, a 5xx, or a timeout.
	// A 4xx that names a bad request, a missing resource or bad credentials never qualifies, and neither does a rate limit, since an immediate retry hits the same limit.
	TransientGitHubAPI TransientClass = "github-api"
	// TransientAgentStart is an agent session that never became ready or never took its first input.
	// An agent that started and then died or timed out never qualifies.
	TransientAgentStart TransientClass = "agent-start"
)

// transientError carries a class beside the error it wraps.
// Error and Unwrap pass the wrapped error through untouched, so the status file's error text and every errors.Is match stay as before.
type transientError struct {
	class TransientClass
	err   error
}

func (e *transientError) Error() string { return e.err.Error() }

func (e *transientError) Unwrap() error { return e.err }

// MarkTransient returns err marked with class.
// An empty class or a nil err returns err unchanged.
func MarkTransient(class TransientClass, err error) error {
	if class == "" || err == nil {
		return err
	}
	return &transientError{class: class, err: err}
}

// TransientOf returns the class of the mark found anywhere in err's chain, or the empty class when there is none.
func TransientOf(err error) TransientClass {
	var te *transientError
	if errors.As(err, &te) {
		return te.class
	}
	return ""
}
