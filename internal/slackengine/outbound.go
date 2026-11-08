// outbound.go is the relay's read of its own mailbox: it posts each message to its sender's channel.
// Each message for `<shortname>:SLACK` is posted at least once.
// Once Slack accepted a message, the relay posts it no more within one relay process.

package slackengine

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"maps"
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
	// postingBudget is how long one read posts before it releases the rest;
	// it stays inside creel's 30-second acknowledgement window.
	postingBudget = 25 * time.Second
	// staleMailAge is the age past which mail is acknowledged without being posted.
	staleMailAge = 24 * time.Hour
	// refusalCap is how many times Slack may refuse one message with an error code before the relay drops it.
	refusalCap = 5
	// orchIcon and programIcon are the icons a post shows for mail from an orch and from a program.
	orchIcon    = ":robot_face:"
	programIcon = ":gear:"
	// isArchivedCode and channelNotFoundCode are the codes Slack answers a post to an archived or a gone channel with.
	isArchivedCode      = "is_archived"
	channelNotFoundCode = "channel_not_found"
)

// errReadBudget is what a read's handler returns when its posting budget ends;
// creel then releases the read with the rest unposted.
var errReadBudget = errors.New("slack: the posting budget of one read ended; the rest waits for the next read")

// errNoOpenChannel is what a post answers for a beam with no open channel.
// It applies only while the hub's beam listing cannot be read.
// The message waits unhandled, and the read is released at the batch's end so the mail is delivered again.
var errNoOpenChannel = errors.New("slack: the beam has no open channel while its listing is unread")

// outboxRecord is the outbox file's content.
// Handled holds the IDs of the messages a read has handled and not yet acknowledged.
// Refusals counts how often Slack refused each message that is not yet handled.
// Stale counts the messages skipped as stale mail whose line is not yet posted.
type outboxRecord struct {
	Handled  []int64       `json:"handled,omitempty"`
	Refusals map[int64]int `json:"refusals,omitempty"`
	Stale    int           `json:"stale,omitempty"`
}

// outbound is one read of the relay's mailbox, posting its mail to the orchs' channels.
type outbound struct {
	geom    Geometry
	mailbox Mailbox
	slack   SlackClient
	clock   Clock
	wait    *loopWait
	keeper  *channelKeeper
	hub     Hub
	// unsaved is the outbox record a failed write left unstored;
	// while set it stands in for the file, so a message Slack accepted is not posted again.
	unsaved *outboxRecord
	// saveFault is the first failed write of the outbox record in the current read.
	saveFault error
}

func newOutbound(geom Geometry, mailbox Mailbox, slack SlackClient, clock Clock, floor *sharedFloor, keeper *channelKeeper, hub Hub) *outbound {
	return &outbound{geom: geom, mailbox: mailbox, slack: slack, clock: clock, wait: newLoopWait(floor), keeper: keeper, hub: hub}
}

// read waits for mail in the relay's mailbox and posts it to the channels of its senders.
// It returns how long to wait before the next read.
// The wait is the floor after mail arrived, doubled after a read with none, and a rate limit's retry wait.
// The mail is acknowledged only when creel confirms it.
// Until then the outbox record keeps each handled message, so a redelivered read posts nothing twice.
// A failed post returns its error with the read released; the record stays until a confirmed acknowledgement clears it.
// A post Slack accepted counts as done whatever fails after it.
// A record that fails to write stays in memory in the file's place, and the read returns the write's error once it ends.
func (out *outbound) read(ctx context.Context) (time.Duration, error) {
	_, found, err := ReadSetup(out.geom.SetupPath)
	if err != nil {
		return out.wait.afterIdle(), err
	}
	if !found {
		return out.wait.afterIdle(), fmt.Errorf("slack: the setup file %s is gone; way forward: run `lym slack setup --operator <user-id>` from the operator's terminal", out.geom.SetupPath)
	}
	out.saveFault = nil
	handlerRan := false
	err = out.mailbox.ReadWait(outboundReadTimeout, func(messages []creelengine.Message) error {
		handlerRan = true
		return out.post(ctx, messages)
	})
	switch {
	case err == nil:
		out.save(outboxRecord{})
		return out.wait.afterActivity(), out.saveFault
	case errors.Is(err, errReadBudget):
		return out.wait.afterActivity(), out.saveFault
	case errors.Is(err, creelengine.ErrWaitTimeout) && !handlerRan:
		return out.wait.afterIdle(), nil
	}
	err = errors.Join(err, out.saveFault)
	if limit, ok := isRateLimit(err); ok {
		return out.wait.afterRateLimit(limit), err
	}
	if handlerRan {
		return out.wait.afterActivity(), err
	}
	return out.wait.afterIdle(), err
}

// post is the read's handler.
// It posts each message not yet in the outbox record, oldest first, and records each one once it is done.
// It then posts the line naming the stale mail the record counts, whether or not the read ended early,
// so the line survives a failed read.
// A line that fails to post stays counted in the record for the next read;
// its failure is returned only when the messages themselves posted.
// The line waits for the next read when the read ended on a rate limit, a further call only prolonging it.
func (out *outbound) post(ctx context.Context, messages []creelengine.Message) error {
	record, err := out.loadOutbox()
	if err != nil {
		return err
	}
	postErr := out.postMessages(ctx, &record, messages)
	if _, limited := isRateLimit(postErr); limited {
		return postErr
	}
	reportErr := out.reportStale(ctx, &record)
	if postErr != nil {
		if reportErr != nil {
			logger.Warn("slackengine: the line about stale mail was not posted", "error", reportErr)
		}
		return postErr
	}
	return reportErr
}

// reportStale posts the line naming the stale mail the record counts to the prime's channel.
// It then clears the count in the record and saves it.
// It posts nothing when the record counts none, and on a failed post leaves the count in place.
func (out *outbound) reportStale(ctx context.Context, record *outboxRecord) error {
	if record.Stale == 0 {
		return nil
	}
	prime, err := orchName(out.geom.Shortname, "")
	if err != nil {
		return err
	}
	line := fmt.Sprintf("%d messages older than 24 hours were not posted; read them with `lym creel log --to %s`", record.Stale, out.relayName())
	if err := out.postRecovered(ctx, prime, line, out.relaySender()); err != nil {
		return fmt.Errorf("slack: post the line about stale mail: %w", err)
	}
	record.Stale = 0
	out.save(*record)
	return nil
}

// postMessages posts each message not yet in record, oldest first, and saves the record after each one it handles.
// Mail older than staleMailAge is counted in the record instead of posted.
// A message of a beam with no open channel is left unhandled and the rest of the batch goes on;
// the batch then ends with errNoOpenChannel, so only that beam's own mail waits.
// It returns errReadBudget once the posting budget ends, and the error of a failed post.
func (out *outbound) postMessages(ctx context.Context, record *outboxRecord, messages []creelengine.Message) error {
	messages = slices.Clone(messages)
	slices.SortFunc(messages, func(a, b creelengine.Message) int { return cmp.Compare(a.ID, b.ID) })
	started := out.clock.Now()
	waiting := false
	for _, message := range messages {
		if slices.Contains(record.Handled, message.ID) {
			continue
		}
		if out.clock.Now().Sub(started) >= postingBudget {
			return errReadBudget
		}
		if out.clock.Now().Sub(message.Posted) > staleMailAge {
			record.Stale++
		} else if orch, sender, ok := outboundRoute(out.geom.Shortname, message); !ok {
			logger.Warn("slackengine: dropped mail from a sender the relay does not post", "id", message.ID, "from", message.From)
		} else if err := out.postTo(ctx, record, message, orch, slackEscape.Replace(message.Body), sender); errors.Is(err, errNoOpenChannel) {
			waiting = true
			continue
		} else if err != nil {
			return err
		}
		record.Handled = append(record.Handled, message.ID)
		delete(record.Refusals, message.ID)
		out.save(*record)
	}
	if waiting {
		return errNoOpenChannel
	}
	return nil
}

// outboundRoute names the orch whose channel the message is posted to and the sender it is posted under.
// An orch strand routes to its own orch, a program of the beam `<shortname>:<slug>:<NAME>` to that beam's orch,
// and a program `<shortname>:<NAME>` to the hub's orch.
// The sender carries the creel name as its name and an icon for an orch or for a program.
// It reports ok false for a strand that is no orch, another hub's program and `EXTERNAL`.
func outboundRoute(shortname string, message creelengine.Message) (string, Sender, bool) {
	if creelengine.IsOrchStrand(message.From) {
		orch, err := orchName(shortname, agentname.TaskSlug(message.From))
		return orch, Sender{Username: message.From, IconEmoji: orchIcon}, err == nil
	}
	programShortname, name, err := agentname.ParseProgram(message.From)
	if err != nil || programShortname != shortname || name == agentname.External {
		return "", Sender{}, false
	}
	orch, err := orchName(shortname, programSlug(message.From, programShortname, name))
	return orch, Sender{Username: message.From, IconEmoji: programIcon}, err == nil
}

// channelFor returns the channel entry a post for orch goes to and the orch whose channel it is.
// The hub's orch is ensured through the keeper, and so is a listed beam's, the first routed mail making its channel.
// A beam the hub's beam listing does not name posts to the prime's channel, reopening nothing.
// While the listing cannot be read, a beam whose channel the map holds open posts there.
// A beam with none answers errNoOpenChannel.
// A refused ensure returns an error wrapping errChannelRefused and an empty entry.
func (out *outbound) channelFor(ctx context.Context, orch string) (ChannelEntry, string, error) {
	slug, err := slugOf(orch)
	if err != nil {
		return ChannelEntry{}, "", err
	}
	if slug == "" {
		entry, err := out.keeper.ensure(ctx, orch)
		return entry, orch, err
	}
	beams, err := out.hub.Beams()
	if err != nil {
		channelMap, mapErr := ReadChannelMap(out.geom.ChannelMapPath)
		entry, held := channelMap.Channels[orch]
		if mapErr != nil || !held || entry.Archived {
			return ChannelEntry{}, "", errors.Join(errNoOpenChannel, mapErr)
		}
		return entry, orch, nil
	}
	if slices.Contains(beams, slug) {
		entry, err := out.keeper.ensure(ctx, orch)
		return entry, orch, err
	}
	prime, err := orchName(out.geom.Shortname, "")
	if err != nil {
		return ChannelEntry{}, "", err
	}
	entry, err := out.keeper.ensure(ctx, prime)
	return entry, prime, err
}

// postTo posts a message for orch under the sender's name and icon.
// It also settles what Slack's answer means for the message.
// A post Slack refuses with an error code counts once toward refusalCap through refused.
// So does a channel the keeper refuses.
// It returns nil for a message posted and for one refused for the cap's last time.
// Any other failure, errNoOpenChannel included, is returned as it is.
func (out *outbound) postTo(ctx context.Context, record *outboxRecord, message creelengine.Message, orch, text string, sender Sender) error {
	entry, channelOrch, err := out.channelFor(ctx, orch)
	if err == nil {
		err = out.postRecovered(ctx, channelOrch, text, sender)
	}
	if err == nil {
		return nil
	}
	if errors.Is(err, errNoOpenChannel) {
		return err
	}
	dropped, refuseErr := out.refused(ctx, entry.ID, record, message, err)
	if dropped {
		return nil
	}
	return refuseErr
}

// postRecovered posts text to the orch's channel under the sender's name and icon.
// A post Slack refuses recovers once:
// `is_archived` marks the entry archived, ensures a new channel and posts again;
// `not_in_channel` joins the bot and posts again;
// `channel_not_found` drops the map entry, ensures the channel anew and posts again.
// A recovery that fails returns the refusal joined with the failure, so the refusal is what counts.
func (out *outbound) postRecovered(ctx context.Context, orch, text string, sender Sender) error {
	entry, err := out.keeper.ensure(ctx, orch)
	if err != nil {
		return err
	}
	postErr := out.slack.PostAs(ctx, entry.ID, text, sender)
	refusal, refused := isAPIError(postErr)
	if !refused {
		return postErr
	}
	var recoverErr error
	switch refusal.Code {
	case isArchivedCode:
		recoverErr = out.keeper.updateEntry(orch, entry.ID, func(current *ChannelEntry) { current.Archived = true })
		if recoverErr == nil {
			entry, recoverErr = out.keeper.ensure(ctx, orch)
		}
	case notInChannelCode:
		recoverErr = out.keeper.join(ctx, orch)
	case channelNotFoundCode:
		recoverErr = out.dropEntry(orch, entry.ID)
		if recoverErr == nil {
			entry, recoverErr = out.keeper.ensure(ctx, orch)
		}
	default:
		return postErr
	}
	if recoverErr != nil {
		return errors.Join(postErr, recoverErr)
	}
	return out.slack.PostAs(ctx, entry.ID, text, sender)
}

// dropEntry removes the orch's entry from the channel map when it still holds the channel id.
func (out *outbound) dropEntry(orch, channelID string) error {
	return UpdateChannelMap(out.geom.ChannelMapPath, func(channelMap *ChannelMap) error {
		if entry, held := channelMap.Channels[orch]; held && entry.ID == channelID {
			delete(channelMap.Channels, orch)
		}
		return nil
	})
}

// refused counts Slack's refusal of message in the record and saves it.
// A refusal is a post Slack refused with an error code or a channel the keeper refused;
// a failure that is neither, a transport failure, a server error or a rate limit, counts toward no cap.
// Once the count reaches refusalCap, it posts one line naming the message and reports that it drops the message.
// The line goes to channel, the message's channel, or to the prime's channel when channel is empty.
// Otherwise it returns err so the read is released with the message queued.
func (out *outbound) refused(ctx context.Context, channel string, record *outboxRecord, message creelengine.Message, err error) (bool, error) {
	if _, ok := isAPIError(err); !ok && !errors.Is(err, errChannelRefused) {
		return false, err
	}
	if record.Refusals == nil {
		record.Refusals = map[int64]int{}
	}
	record.Refusals[message.ID]++
	if record.Refusals[message.ID] < refusalCap {
		out.save(*record)
		return false, err
	}
	line := fmt.Sprintf("message %d from %s was refused by Slack %d times and is dropped; read it with `lym creel log --to %s`", message.ID, message.From, refusalCap, out.relayName())
	out.postDroppedLine(ctx, channel, line)
	return true, nil
}

// postDroppedLine posts the line about a dropped message to the channel, or to the prime's when channel is empty.
// When the prime's channel is refused too, the line reaches the hub's orch as a `[slack relay]` notice.
func (out *outbound) postDroppedLine(ctx context.Context, channel, line string) {
	prime, err := orchName(out.geom.Shortname, "")
	if err != nil {
		logger.Warn("slackengine: the line about a dropped message was not posted", "error", err)
		return
	}
	orch := prime
	if channelMap, mapErr := ReadChannelMap(out.geom.ChannelMapPath); mapErr == nil && channel != "" {
		if held, ok := channelMap.OrchOf(channel); ok {
			orch = held
		}
	}
	postErr := out.postRecovered(ctx, orch, line, out.relaySender())
	if postErr == nil {
		return
	}
	logger.Warn("slackengine: the line about a dropped message was not posted", "orch", orch, "error", postErr)
	if errors.Is(postErr, errChannelRefused) && orch == prime {
		if _, mailErr := out.mailbox.Post(prime, relayNoticePrefix+" "+line); mailErr != nil {
			logger.Warn("slackengine: the relay notice for a dropped message was not posted", "receiver", prime, "error", mailErr)
		}
	}
}

// relayName returns the relay's creel program name, `<shortname>:SLACK`.
func (out *outbound) relayName() string {
	return out.geom.Shortname + ":" + SlackProgram
}

// relaySender is the sender of the relay's own lines: its creel program name and the program icon.
func (out *outbound) relaySender() Sender {
	return Sender{Username: out.relayName(), IconEmoji: programIcon}
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

// loadOutbox returns the outbox record: the one a failed write left in memory, otherwise the file's.
func (out *outbound) loadOutbox() (outboxRecord, error) {
	if out.unsaved != nil {
		return cloneOutbox(*out.unsaved), nil
	}
	record, _, err := state.ReadJSON[outboxRecord](out.geom.OutboxPath, recordLockPath(out.geom.OutboxPath))
	if err != nil {
		return outboxRecord{}, fmt.Errorf("slack: read the outbox file %s: %w", out.geom.OutboxPath, err)
	}
	return record, nil
}

// save stores record in the outbox file.
// A failed write keeps a copy of record in memory in the file's place and keeps the error for the read to return,
// so the write never turns a message Slack accepted into one to post again.
func (out *outbound) save(record outboxRecord) {
	if err := state.WriteJSON(out.geom.OutboxPath, recordLockPath(out.geom.OutboxPath), record); err != nil {
		unsaved := cloneOutbox(record)
		out.unsaved = &unsaved
		if out.saveFault == nil {
			out.saveFault = fmt.Errorf("slack: write the outbox file %s; the relay keeps the record in memory: %w", out.geom.OutboxPath, err)
		}
		return
	}
	out.unsaved = nil
}

// cloneOutbox returns a copy of record that shares no slice or map with it.
func cloneOutbox(record outboxRecord) outboxRecord {
	return outboxRecord{Handled: slices.Clone(record.Handled), Refusals: maps.Clone(record.Refusals), Stale: record.Stale}
}
