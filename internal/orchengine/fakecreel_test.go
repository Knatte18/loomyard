// fakecreel_test.go is the recording fake of the Creel seam the watcher tests build their watcher with.

package orchengine

import (
	"strings"
	"time"
)

// fakePost is one post the fake recorded.
type fakePost struct {
	ID       int64
	Receiver string
	Body     string
}

// fakeInject is one inject the fake recorded.
type fakeInject struct {
	ID       int64
	Receiver string
	Step     CreelStep
	TTL      time.Duration
}

// fakeCreel records every call and answers the outcomes, mailbox messages, sent list and withdrawn messages a test scripts.
// A scripted error in errs, keyed by the method name in lower case, fails that method's next call and is then consumed.
type fakeCreel struct {
	posts   []fakePost
	injects []fakeInject
	// calls lists every call in order as "<method>:<receiver>" for the receiver-addressed methods and "read-mailbox".
	calls []string

	// mailbox holds the unread messages ReadMailbox hands to its handler; a handler that returns nil marks them read.
	mailbox      []CreelMessage
	mailboxReads int
	// outcomes is the outcome InjectOutcome answers by inject id; an id with none answers a queued inject.
	outcomes map[int64]CreelOutcome
	// sent is the list Sent answers for every receiver and time.
	sent []CreelSent
	// withdrawn is what Withdraw answers.
	withdrawn []CreelWithdrawn

	// clock stamps the sent entries a journaled-but-failed call leaves, and the time an auto-landed inject was sent.
	clock func() time.Time
	// autoLand settles every inject as injected, sent at the clock's time, so a test that does not script outcomes sees every step typed at once.
	autoLand bool
	// journalFails scripts the next calls to fail after journaling their request: the entry reaches the sent list and the call answers the error.
	journalFails []error
	// onSend runs on every post and inject the fake accepts, with the post's body or the inject's step.
	onSend func(mail bool, step CreelStep, body string)
	// onReadMailbox runs before ReadMailbox hands its messages to the handler.
	onReadMailbox func()

	errs   map[string][]error
	nextID int64
}

var _ Creel = (*fakeCreel)(nil)

// fail consumes and returns the next scripted error of method, nil when none is scripted.
func (f *fakeCreel) fail(method string) error {
	queue := f.errs[method]
	if len(queue) == 0 {
		return nil
	}
	f.errs[method] = queue[1:]
	return queue[0]
}

// failNext scripts err as the failure of method's next call.
func (f *fakeCreel) failNext(method string, err error) {
	if f.errs == nil {
		f.errs = map[string][]error{}
	}
	f.errs[method] = append(f.errs[method], err)
}

func (f *fakeCreel) newID() int64 {
	f.nextID++
	return f.nextID
}

// journalFail consumes the next scripted journaled failure and, when there is one, records the request in the sent list and returns its error.
func (f *fakeCreel) journalFail(entry CreelSent) error {
	if len(f.journalFails) == 0 {
		return nil
	}
	err := f.journalFails[0]
	f.journalFails = f.journalFails[1:]
	entry.ID, entry.At = f.newID(), f.clock()
	f.sent = append(f.sent, entry)
	return err
}

func (f *fakeCreel) Post(receiver, body string) (int64, error) {
	f.calls = append(f.calls, "post:"+receiver)
	if err := f.fail("post"); err != nil {
		return 0, err
	}
	firstLine, _, _ := strings.Cut(body, "\n")
	if err := f.journalFail(CreelSent{Mail: true, Subject: firstLine}); err != nil {
		return 0, err
	}
	id := f.newID()
	f.posts = append(f.posts, fakePost{ID: id, Receiver: receiver, Body: body})
	if f.onSend != nil {
		f.onSend(true, CreelStep{}, body)
	}
	return id, nil
}

func (f *fakeCreel) ReadMailbox(handle func([]CreelMessage) error) error {
	f.calls = append(f.calls, "read-mailbox")
	f.mailboxReads++
	if err := f.fail("readmailbox"); err != nil {
		return err
	}
	if len(f.mailbox) == 0 {
		return nil
	}
	if f.onReadMailbox != nil {
		f.onReadMailbox()
	}
	if err := handle(append([]CreelMessage(nil), f.mailbox...)); err != nil {
		return err
	}
	f.mailbox = nil
	return nil
}

func (f *fakeCreel) Inject(receiver string, step CreelStep, ttl time.Duration) (int64, error) {
	f.calls = append(f.calls, "inject:"+receiver)
	if err := f.fail("inject"); err != nil {
		return 0, err
	}
	if err := f.journalFail(CreelSent{Step: step, Subject: step.Text}); err != nil {
		return 0, err
	}
	id := f.newID()
	f.injects = append(f.injects, fakeInject{ID: id, Receiver: receiver, Step: step, TTL: ttl})
	if f.autoLand {
		if f.outcomes == nil {
			f.outcomes = map[int64]CreelOutcome{}
		}
		f.outcomes[id] = CreelOutcome{State: CreelInjected, SendingAt: f.clock()}
	}
	if f.onSend != nil {
		f.onSend(false, step, "")
	}
	return id, nil
}

func (f *fakeCreel) InjectOutcome(id int64) (CreelOutcome, error) {
	if err := f.fail("injectoutcome"); err != nil {
		return CreelOutcome{}, err
	}
	if outcome, ok := f.outcomes[id]; ok {
		return outcome, nil
	}
	return CreelOutcome{State: CreelQueued}, nil
}

func (f *fakeCreel) Sent(receiver string, since time.Time) ([]CreelSent, error) {
	if err := f.fail("sent"); err != nil {
		return nil, err
	}
	return append([]CreelSent(nil), f.sent...), nil
}

func (f *fakeCreel) Hold(receiver string, duration time.Duration) error {
	f.calls = append(f.calls, "hold:"+receiver)
	return f.fail("hold")
}

func (f *fakeCreel) Release(receiver string) error {
	f.calls = append(f.calls, "release:"+receiver)
	return f.fail("release")
}

func (f *fakeCreel) Withdraw(receiver string) ([]CreelWithdrawn, error) {
	f.calls = append(f.calls, "withdraw:"+receiver)
	if err := f.fail("withdraw"); err != nil {
		return nil, err
	}
	return append([]CreelWithdrawn(nil), f.withdrawn...), nil
}

// count returns how many recorded calls start with prefix.
func (f *fakeCreel) count(prefix string) int {
	n := 0
	for _, call := range f.calls {
		if strings.HasPrefix(call, prefix) {
			n++
		}
	}
	return n
}

// postBodies lists the bodies of the recorded posts to receiver in order.
func (f *fakeCreel) postBodies(receiver string) []string {
	var bodies []string
	for _, post := range f.posts {
		if post.Receiver == receiver {
			bodies = append(bodies, post.Body)
		}
	}
	return bodies
}
