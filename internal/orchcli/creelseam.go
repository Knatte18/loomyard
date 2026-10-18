// creelseam.go declares creelSeam, the production adapter that satisfies orchengine.Creel over a creelcli Go client, and the folds of creel's journal entries into the watcher's plain values.

package orchcli

import (
	"time"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/creelengine"
	"github.com/Knatte18/loomyard/internal/orchengine"
)

// orchWatchProgram is the program name the orch watcher signs creel calls with, whatever strand name its environment holds.
const orchWatchProgram = "ORCHWATCH"

// The journal events the folds read.
const (
	eventPosted      = "posted"
	eventSending     = "sending"
	eventInjected    = "injected"
	eventUnconfirmed = "unconfirmed"
	eventExpired     = "expired"
)

// kindInject is the kind a journal entry carries for an inject, as against mail and a reply.
const kindInject = "inject"

// creelClient is the slice of the creelcli Go client the seam calls.
type creelClient interface {
	Shortname() string
	Post(receiver, body string) (int64, error)
	ReadAll(handle func([]creelengine.Message) error) error
	Inject(receiver, text string, ttl time.Duration) (int64, error)
	InjectCommand(receiver string, command creelengine.Command, ttl time.Duration) (int64, error)
	Log(filter creelengine.LogFilter) ([]creelengine.LogEntry, error)
	Hold(receiver string, duration time.Duration) error
	Release(receiver string) (bool, error)
	Withdraw(receiver string) (creelengine.WithdrawResult, error)
}

// creelSeam adapts a creelClient to orchengine.Creel.
type creelSeam struct {
	client creelClient
}

var _ orchengine.Creel = creelSeam{}

// Post leaves body as mail for receiver.
func (s creelSeam) Post(receiver, body string) (int64, error) {
	return s.client.Post(receiver, body)
}

// ReadMailbox hands the watcher's unread messages to handle.
func (s creelSeam) ReadMailbox(handle func([]orchengine.CreelMessage) error) error {
	return s.client.ReadAll(func(messages []creelengine.Message) error {
		converted := make([]orchengine.CreelMessage, 0, len(messages))
		for _, message := range messages {
			converted = append(converted, orchengine.CreelMessage{ID: message.ID, From: message.From, ReplyTo: message.ReplyTo, Body: message.Body})
		}
		return handle(converted)
	})
}

// Inject queues a command inject when step names a command and a text inject otherwise.
func (s creelSeam) Inject(receiver string, step orchengine.CreelStep, ttl time.Duration) (int64, error) {
	if step.Command != "" {
		return s.client.InjectCommand(receiver, creelengine.Command{Name: creelengine.CommandName(step.Command), Argument: step.Argument}, ttl)
	}
	return s.client.Inject(receiver, step.Text, ttl)
}

// InjectOutcome folds the journal entries of inject id into where it stands.
func (s creelSeam) InjectOutcome(id int64) (orchengine.CreelOutcome, error) {
	entries, err := s.client.Log(creelengine.LogFilter{ID: id})
	if err != nil {
		return orchengine.CreelOutcome{}, err
	}
	return injectOutcomeOf(id, entries), nil
}

// Sent folds the journal entries of what the watcher posted to receiver since the given time into the posts and injects it sent.
func (s creelSeam) Sent(receiver string, since time.Time) ([]orchengine.CreelSent, error) {
	from, err := agentname.FormatProgram(s.client.Shortname(), orchWatchProgram)
	if err != nil {
		return nil, err
	}
	entries, err := s.client.Log(creelengine.LogFilter{From: from, To: receiver, Since: since})
	if err != nil {
		return nil, err
	}
	return sentOf(entries), nil
}

// Hold holds receiver's deliveries to the watcher's own for duration.
func (s creelSeam) Hold(receiver string, duration time.Duration) error {
	return s.client.Hold(receiver, duration)
}

// Release ends the watcher's hold on receiver.
func (s creelSeam) Release(receiver string) error {
	_, err := s.client.Release(receiver)
	return err
}

// Withdraw removes the watcher's queued injects and unread mail to receiver and answers the mail it removed.
func (s creelSeam) Withdraw(receiver string) ([]orchengine.CreelWithdrawn, error) {
	result, err := s.client.Withdraw(receiver)
	if err != nil {
		return nil, err
	}
	withdrawn := make([]orchengine.CreelWithdrawn, 0, len(result.Messages))
	for _, message := range result.Messages {
		withdrawn = append(withdrawn, orchengine.CreelWithdrawn{ID: message.ID, Subject: message.Subject})
	}
	return withdrawn, nil
}

// injectOutcomeOf folds the journal entries entries holds for inject id, oldest first, into its outcome.
// The newest sending, injected, unconfirmed or expired entry decides the state, an expired entry with the withdrawn reason reading as withdrawn, and an inject with none is queued.
// SendingAt is the time of the newest sending entry, since every typing attempt journals one and a busy attempt requeues the inject.
// Stall is the reason of an expired entry other than a withdrawal.
func injectOutcomeOf(id int64, entries []creelengine.LogEntry) orchengine.CreelOutcome {
	var outcome orchengine.CreelOutcome
	for _, entry := range entries {
		if entry.ID != id {
			continue
		}
		switch entry.Event {
		case eventPosted:
			outcome.State = orchengine.CreelQueued
		case eventSending:
			outcome.State, outcome.SendingAt, outcome.Stall = orchengine.CreelSending, entry.At, ""
		case eventInjected:
			outcome.State, outcome.Stall = orchengine.CreelInjected, ""
		case eventUnconfirmed:
			outcome.State, outcome.Stall = orchengine.CreelUnconfirmed, ""
		case eventExpired:
			if entry.Reason == creelengine.WithdrawnReason {
				outcome.State, outcome.Stall = orchengine.CreelWithdrawnState, ""
			} else {
				outcome.State, outcome.Stall = orchengine.CreelExpired, entry.Reason
			}
		}
	}
	return outcome
}

// sentOf folds the journal entries entries holds into the posts and injects they record, oldest first.
func sentOf(entries []creelengine.LogEntry) []orchengine.CreelSent {
	var sent []orchengine.CreelSent
	for _, entry := range entries {
		if entry.Event != eventPosted {
			continue
		}
		sent = append(sent, orchengine.CreelSent{
			ID:      entry.ID,
			At:      entry.At,
			Mail:    entry.Kind != kindInject && entry.Command == "",
			Step:    orchengine.CreelStep{Command: string(entry.Command), Argument: entry.Argument},
			Subject: entry.Subject,
		})
	}
	return sent
}
