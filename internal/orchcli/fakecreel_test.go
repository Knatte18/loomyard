// fakecreel_test.go declares the fakes of creel the orch verb tests build their receiver with: a recording orchengine.Creel and a scripted creelClient.

package orchcli

import (
	"time"

	"github.com/Knatte18/loomyard/internal/creelengine"
	"github.com/Knatte18/loomyard/internal/orchengine"
)

// fakeCreel records every call it answers and reaches no daemon.
type fakeCreel struct {
	// calls lists every call in order as "<method>:<receiver>", and "read-mailbox".
	calls []string
	posts []string
}

var _ orchengine.Creel = (*fakeCreel)(nil)

func (f *fakeCreel) Post(receiver, body string) (int64, error) {
	f.calls = append(f.calls, "post:"+receiver)
	f.posts = append(f.posts, body)
	return int64(len(f.posts)), nil
}

func (f *fakeCreel) ReadMailbox(func([]orchengine.CreelMessage) error) error {
	f.calls = append(f.calls, "read-mailbox")
	return nil
}

func (f *fakeCreel) Inject(receiver string, _ orchengine.CreelStep, _ time.Duration) (int64, error) {
	f.calls = append(f.calls, "inject:"+receiver)
	return int64(len(f.calls)), nil
}

func (f *fakeCreel) InjectOutcome(int64) (orchengine.CreelOutcome, error) {
	return orchengine.CreelOutcome{State: orchengine.CreelQueued}, nil
}

func (f *fakeCreel) Sent(string, time.Time) ([]orchengine.CreelSent, error) {
	return nil, nil
}

func (f *fakeCreel) Hold(receiver string, _ time.Duration) error {
	f.calls = append(f.calls, "hold:"+receiver)
	return nil
}

func (f *fakeCreel) Release(receiver string) error {
	f.calls = append(f.calls, "release:"+receiver)
	return nil
}

func (f *fakeCreel) Withdraw(receiver string) ([]orchengine.CreelWithdrawn, error) {
	f.calls = append(f.calls, "withdraw:"+receiver)
	return nil, nil
}

// fakeCreelClient answers the journal entries a test scripts and fails every other creelClient method by its nil embedded interface.
type fakeCreelClient struct {
	creelClient
	shortname string
	entries   []creelengine.LogEntry
	// filters records the filter of every Log call.
	filters []creelengine.LogFilter
}

func (f *fakeCreelClient) Shortname() string { return f.shortname }

func (f *fakeCreelClient) Log(filter creelengine.LogFilter) ([]creelengine.LogEntry, error) {
	f.filters = append(f.filters, filter)
	return f.entries, nil
}
