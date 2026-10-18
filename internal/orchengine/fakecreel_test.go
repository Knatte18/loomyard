// fakecreel_test.go is the recording fake of the Creel seam the watcher tests build their watcher with.

package orchengine

import "time"

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

func (f *fakeCreel) Post(receiver, body string) (int64, error) {
	f.calls = append(f.calls, "post:"+receiver)
	if err := f.fail("post"); err != nil {
		return 0, err
	}
	id := f.newID()
	f.posts = append(f.posts, fakePost{ID: id, Receiver: receiver, Body: body})
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
	id := f.newID()
	f.injects = append(f.injects, fakeInject{ID: id, Receiver: receiver, Step: step, TTL: ttl})
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
