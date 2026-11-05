// outbound.go is the relay's read of its own mailbox: it posts the orchs' and programs' mail for `<shortname>:SLACK` to the channel at least once and never twice for one acknowledged read.

package slackengine

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/creelengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/state"
)

const (
	// outboundReadTimeout bounds one read of the mailbox: the wait for mail, the posting and creel's confirmation together.
	outboundReadTimeout = 60 * time.Second
	// postingBudget is how long one read posts before it releases the rest, inside creel's 30-second acknowledgement window.
	postingBudget = 25 * time.Second
	// staleMailAge is the age past which mail is acknowledged without being posted.
	staleMailAge = 24 * time.Hour
	// refusalCap is how many times Slack may refuse one message with an error code before the relay drops it.
	refusalCap = 5
)

// errReadBudget is what a read's handler returns when its posting budget ended, so creel releases the read with the rest unposted.
var errReadBudget = errors.New("slack: the posting budget of one read ended; the rest waits for the next read")

// errOutboxWrite marks a failed write of the outbox file.
var errOutboxWrite = errors.New("slack: write the outbox file")

// outboxRecord is the outbox file's content.
// Handled holds the messages of a read not yet acknowledged that are done.
// Refusals holds how often Slack refused each other message.
// Stale counts the done messages that were stale mail whose count line is not yet posted.
type outboxRecord struct {
	Handled  []int64       `json:"handled,omitempty"`
	Refusals map[int64]int `json:"refusals,omitempty"`
	Stale    int           `json:"stale,omitempty"`
}

// outbound is the relay's outbound read of its mailbox.
type outbound struct {
	geom    Geometry
	mailbox Mailbox
	slack   SlackClient
	clock   Clock
	wait    *loopWait
}

func newOutbound(geom Geometry, mailbox Mailbox, slack SlackClient, clock Clock, floor *sharedFloor) *outbound {
	return &outbound{geom: geom, mailbox: mailbox, slack: slack, clock: clock, wait: newLoopWait(floor)}
}

// read waits for mail in the relay's mailbox and posts it to the channel, and returns how long to wait before the next read.
// The wait is the floor after mail arrived, doubled after a read with none, and a rate limit's retry wait.
// The mail is acknowledged only when creel confirms it, and the outbox record keeps each message already handled until then, so a read creel redelivers posts nothing twice.
// A failed post returns its error with the read released; the record stays until a confirmed acknowledgement clears it.
func (out *outbound) read(ctx context.Context) (time.Duration, error) {
	setup, found, err := ReadSetup(out.geom.SetupPath)
	if err != nil {
		return out.wait.afterIdle(), err
	}
	if !found {
		return out.wait.afterIdle(), fmt.Errorf("slack: the setup file %s is gone; way forward: run `lym slack setup --channel <id> --operator <user-id>` from the operator's terminal", out.geom.SetupPath)
	}
	handlerRan := false
	err = out.mailbox.ReadWait(outboundReadTimeout, func(messages []creelengine.Message) error {
		handlerRan = true
		return out.post(ctx, setup, messages)
	})
	switch {
	case err == nil:
		return out.wait.afterActivity(), writeOutbox(out.geom, outboxRecord{})
	case errors.Is(err, errReadBudget):
		return out.wait.afterActivity(), nil
	case errors.Is(err, creelengine.ErrWaitTimeout) && !handlerRan:
		return out.wait.afterIdle(), nil
	}
	if limit, ok := isRateLimit(err); ok {
		return out.wait.afterRateLimit(limit), err
	}
	if handlerRan {
		return out.wait.afterActivity(), err
	}
	return out.wait.afterIdle(), err
}

// post is the read's handler: it posts each message not yet in the outbox record, oldest first, and records each once it is done.
// Whether the messages all posted or the read ends early, it then posts the one line naming the stale mail the record counts, so that line survives a failed read.
// A line that fails to post stays counted in the record for the next read; its failure is returned only when the messages themselves posted.
// The line waits for the next read when the read ended on a rate limit, which a further call would only prolong, or on a failed write of the record, which would let the line post twice.
func (out *outbound) post(ctx context.Context, setup Setup, messages []creelengine.Message) error {
	record, err := readOutbox(out.geom)
	if err != nil {
		return err
	}
	postErr := out.postMessages(ctx, setup, &record, messages)
	if _, limited := isRateLimit(postErr); limited || errors.Is(postErr, errOutboxWrite) {
		return postErr
	}
	reportErr := out.reportStale(ctx, setup, &record)
	if postErr != nil {
		if reportErr != nil {
			logger.Warn("slackengine: the line about stale mail was not posted", "error", reportErr)
		}
		return postErr
	}
	return reportErr
}

// reportStale posts the line naming the stale mail the record counts, then clears the count in the record and writes it.
// It posts nothing when the record counts none, and on a failed post leaves the count in place.
func (out *outbound) reportStale(ctx context.Context, setup Setup, record *outboxRecord) error {
	if record.Stale == 0 {
		return nil
	}
	line := fmt.Sprintf("%d messages older than 24 hours were not posted; read them with `lym creel log --to %s`", record.Stale, out.relayName())
	if err := out.slack.PostMessage(ctx, setup.ChannelID, line); err != nil {
		return fmt.Errorf("slack: post the line about stale mail: %w", err)
	}
	record.Stale = 0
	return writeOutbox(out.geom, *record)
}

// postMessages posts each message of messages not yet in record, oldest first, and writes the record after each one it is done with.
// Mail older than staleMailAge is counted in the record instead of posted.
// It returns errReadBudget once the posting budget ends, and the error of a failed post or write.
func (out *outbound) postMessages(ctx context.Context, setup Setup, record *outboxRecord, messages []creelengine.Message) error {
	messages = slices.Clone(messages)
	slices.SortFunc(messages, func(a, b creelengine.Message) int { return cmp.Compare(a.ID, b.ID) })
	started := out.clock.Now()
	for _, message := range messages {
		if slices.Contains(record.Handled, message.ID) {
			continue
		}
		if out.clock.Now().Sub(started) >= postingBudget {
			return errReadBudget
		}
		if out.clock.Now().Sub(message.Posted) > staleMailAge {
			record.Stale++
		} else if text, ok := outboundText(out.geom.Shortname, message); !ok {
			logger.Warn("slackengine: dropped mail from a sender the relay does not post", "id", message.ID, "from", message.From)
		} else if err := out.slack.PostMessage(ctx, setup.ChannelID, text); err != nil {
			dropped, refuseErr := out.refused(ctx, setup, record, message, err)
			if !dropped {
				return refuseErr
			}
		}
		record.Handled = append(record.Handled, message.ID)
		delete(record.Refusals, message.ID)
		if err := writeOutbox(out.geom, *record); err != nil {
			return err
		}
	}
	return nil
}

// refused counts Slack's refusal of message in the record and writes it.
// It reports dropped true once the refusal count reaches refusalCap, after posting one line that names the message;
// a failure that is no refusal, a transport failure, a server error or a rate limit, counts toward no cap.
// Otherwise it returns err so the read is released with the message queued.
func (out *outbound) refused(ctx context.Context, setup Setup, record *outboxRecord, message creelengine.Message, err error) (bool, error) {
	if _, ok := isAPIError(err); !ok {
		return false, err
	}
	if record.Refusals == nil {
		record.Refusals = map[int64]int{}
	}
	record.Refusals[message.ID]++
	if record.Refusals[message.ID] < refusalCap {
		if writeErr := writeOutbox(out.geom, *record); writeErr != nil {
			return false, errors.Join(err, writeErr)
		}
		return false, err
	}
	line := fmt.Sprintf("message %d from %s was refused by Slack %d times and is dropped; read it with `lym creel log --to %s`", message.ID, message.From, refusalCap, out.relayName())
	if postErr := out.slack.PostMessage(ctx, setup.ChannelID, line); postErr != nil {
		logger.Warn("slackengine: the line about a dropped message was not posted", "id", message.ID, "error", postErr)
	}
	return true, nil
}

// relayName returns the relay's creel program name, `<shortname>:SLACK`.
func (out *outbound) relayName() string {
	return out.geom.Shortname + ":" + SlackProgram
}

// outboundText renders mail for the channel as `[<sender>] <body>`, escaped, and reports false for a sender the relay does not post.
// The relay posts an orch strand and any program of the hub but the one for an unplaced sender.
// Mail from a beam ends with how to answer it, `(answer: @<slug> …)`.
func outboundText(shortname string, message creelengine.Message) (string, bool) {
	slug := ""
	if creelengine.IsOrchStrand(message.From) {
		slug = agentname.TaskSlug(message.From)
	} else {
		programShortname, name, err := agentname.ParseProgram(message.From)
		if err != nil || programShortname != shortname || name == agentname.External {
			return "", false
		}
		slug = programSlug(message.From, programShortname, name)
	}
	text := slackEscape.Replace(fmt.Sprintf("[%s] %s", message.From, message.Body))
	if slug != "" {
		text += fmt.Sprintf(" (answer: @%s …)", slug)
	}
	return text, true
}

// programSlug returns the beam slug of a program sender of the form `<shortname>:<slug>:<NAME>`, empty for the two-segment form.
func programSlug(from, shortname, name string) string {
	middle, found := strings.CutPrefix(from, shortname+":")
	if !found {
		return ""
	}
	slug, found := strings.CutSuffix(middle, ":"+name)
	if !found || agentname.ValidateSlug(slug) != nil {
		return ""
	}
	return slug
}

// readOutbox returns the outbox record, empty when none is written.
func readOutbox(geom Geometry) (outboxRecord, error) {
	record, _, err := state.ReadJSON[outboxRecord](geom.OutboxPath, recordLockPath(geom.OutboxPath))
	if err != nil {
		return outboxRecord{}, fmt.Errorf("slack: read the outbox file %s: %w", geom.OutboxPath, err)
	}
	return record, nil
}

// writeOutbox stores record in the outbox file.
func writeOutbox(geom Geometry, record outboxRecord) error {
	if err := state.WriteJSON(geom.OutboxPath, recordLockPath(geom.OutboxPath), record); err != nil {
		return fmt.Errorf("%w %s: %w", errOutboxWrite, geom.OutboxPath, err)
	}
	return nil
}
