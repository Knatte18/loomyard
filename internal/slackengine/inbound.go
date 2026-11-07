// inbound.go reads the operator's messages from the hub's Slack channel and forwards them to the orchs.
// It also answers the presence commands: away, back and status.

package slackengine

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/creelengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/state"
)

// SlackProgram is the creel program name the relay signs as, formed with the hub's shortname as `<shortname>:SLACK`.
const SlackProgram = "SLACK"

const (
	// forwardPrefix opens the body of every operator message the relay forwards to an orch.
	forwardPrefix = "[slack] "
	// presenceNoticePrefix opens the body of a presence notice, which asks for no reply.
	presenceNoticePrefix = "[slack presence]"
	// relayNoticePrefix opens the body of a notice about the relay itself, which carries no operator authority.
	relayNoticePrefix = "[slack relay]"
	// creelBodyLimit is the largest body creel posts, in bytes.
	creelBodyLimit = 4096
	// retryWindow is how long a message that cannot be handled is retried;
	// after it, the relay answers in Slack and moves past the message.
	retryWindow = 5 * time.Minute
)

// Clock is the relay's source of time.
// Tests supply their own so that no test waits out a production duration.
type Clock interface {
	// Now returns the current time.
	Now() time.Time
	// After returns a channel that delivers once d has passed.
	After(d time.Duration) <-chan time.Time
}

// SystemClock is a Clock that reads the wall clock.
type SystemClock struct{}

// Now returns the wall-clock time.
func (SystemClock) Now() time.Time { return time.Now() }

// After returns a channel that delivers once d of wall-clock time has passed.
func (SystemClock) After(d time.Duration) <-chan time.Time { return time.After(d) }

// Mailbox is the creel mailbox the relay posts to and reads, signing as `<shortname>:SLACK`.
type Mailbox interface {
	// Post leaves body as a message for receiver and returns its id.
	Post(receiver, body string) (int64, error)
	// ReadAll reads every unread message without waiting and hands them to handle, which acknowledges them by returning nil.
	ReadAll(handle func([]creelengine.Message) error) error
	// ReadWait blocks until there is unread mail, hands it to handle and acknowledges it when handle returns nil; timeout bounds the wait, the handler and the confirmation together.
	ReadWait(timeout time.Duration, handle func([]creelengine.Message) error) error
}

// SlackClient is the set of Slack API calls the relay makes.
type SlackClient interface {
	// History reads the channel's messages after oldest, oldest first.
	History(ctx context.Context, channel, oldest string) ([]Message, error)
	// Latest returns the timestamp of the channel's newest message, found false for an empty channel.
	Latest(ctx context.Context, channel string) (string, bool, error)
	// PostMessage posts text to the channel.
	PostMessage(ctx context.Context, channel, text string) error
}

// Hub is the hub state the relay reads that is not in its own files.
type Hub struct {
	// Beams lists the slugs of the hub's beams.
	Beams func() ([]string, error)
	// Paused reports whether a pause covers the orch named receiver.
	Paused func(receiver string) (bool, error)
}

// cursorRecord is the cursor file's content: the newest message of the channel the relay has handled.
type cursorRecord struct {
	Channel string `json:"channel"`
	TS      string `json:"ts"`
}

// inbound polls the hub's Slack channel and holds the state that carries from one poll to the next.
type inbound struct {
	geom    Geometry
	mailbox Mailbox
	slack   SlackClient
	hub     Hub
	clock   Clock

	// lastPresence is the presence the previous poll read, valid while presenceKnown.
	lastPresence  Presence
	presenceKnown bool
	// apiNoticed is set once the current episode of a Slack refusal posted its notice.
	apiNoticed bool
	// failingTS and failingSince name the message that cannot be handled yet and when it first failed.
	failingTS    string
	failingSince time.Time
	episodes     logger.EpisodeSet
}

var (
	// slackLink matches a link Slack wraps in angle brackets, `<url|label>` or `<url>`, whose value is a web or mail address.
	slackLink = regexp.MustCompile(`<((?:https?://|mailto:)[^|>\s]*)(?:\|[^>]*)?>`)
	// slackUnescape reverses the three escapes Slack applies to message text.
	slackUnescape = strings.NewReplacer("&lt;", "<", "&gt;", ">", "&amp;", "&")
	// slackEscape applies the three escapes Slack asks of posted text.
	slackEscape = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
)

// inboundText turns a message's text, as Slack delivers it, into the text the orch reads.
// A link whose value starts with http://, https:// or mailto: becomes the bare value;
// the escapes `&amp;`, `&lt;` and `&gt;` are unescaped.
// A channel, a `<!here>` and a user mention stay as Slack delivers them.
func inboundText(text string) string {
	return slackUnescape.Replace(slackLink.ReplaceAllString(text, "$1"))
}

// orchName forms the name of the orch of the beam slug, the hub's own orch for an empty slug.
func orchName(shortname, slug string) (string, error) {
	return agentname.Format(shortname, slug, agentname.RoleOrch)
}

// mentionedSlug reports the beam slug a text opens with, in the form `@<slug> <text>`.
func mentionedSlug(text string) (string, bool) {
	rest, found := strings.CutPrefix(text, "@")
	if !found {
		return "", false
	}
	slug, after, hasSpace := strings.Cut(rest, " ")
	if !hasSpace || strings.TrimSpace(after) == "" || agentname.ValidateSlug(slug) != nil {
		return "", false
	}
	return slug, true
}

// route picks the orch a text is for and the body to send it.
// A text opening `@<slug> ` goes to that beam's orch without the prefix when slug is one of beams;
// otherwise ok is false;
// any other text goes to the hub's own orch unchanged.
func route(shortname, text string, beams []string) (string, string, bool) {
	slug, mentioned := mentionedSlug(text)
	if !mentioned {
		receiver, err := orchName(shortname, "")
		return receiver, text, err == nil
	}
	if !slices.Contains(beams, slug) {
		return "", "", false
	}
	receiver, err := orchName(shortname, slug)
	if err != nil {
		return "", "", false
	}
	_, after, _ := strings.Cut(text, " ")
	return receiver, strings.TrimSpace(after), true
}

// SetPresence writes presence and, only when it changed, mails a `[slack presence]` notice to the hub's orch;
// the notice goes to the orch of every beam `beams` lists.
// The notice asks for no reply.
// It returns changed false with an error when the presence was not written;
// it returns changed true with an error when the write stood and the beams read or a notice failed;
// the error joins every failed notice.
func SetPresence(geom Geometry, mailbox Mailbox, beams func() ([]string, error), presence Presence, now time.Time) (bool, error) {
	changed, err := WritePresence(geom.PresencePath, presence, now)
	if err != nil || !changed {
		return false, err
	}
	var failures []error
	slugs, err := beams()
	if err != nil {
		failures = append(failures, fmt.Errorf("slack: read the hub's beams for the presence notice: %w", err))
	}
	notice := fmt.Sprintf("%s presence is now %s", presenceNoticePrefix, presence)
	for _, slug := range append([]string{""}, slugs...) {
		receiver, err := orchName(geom.Shortname, slug)
		if err != nil {
			failures = append(failures, err)
			continue
		}
		if _, err := mailbox.Post(receiver, notice); err != nil {
			failures = append(failures, fmt.Errorf("slack: post the presence notice to %s: %w", receiver, err))
		}
	}
	return true, errors.Join(failures...)
}

// readCursor returns the cursor file's content, found false when none is written.
func readCursor(geom Geometry) (cursorRecord, bool, error) {
	cursor, found, err := state.ReadJSON[cursorRecord](geom.CursorPath, recordLockPath(geom.CursorPath))
	if err != nil {
		return cursorRecord{}, false, fmt.Errorf("slack: read the cursor file %s: %w", geom.CursorPath, err)
	}
	return cursor, found, nil
}

// writeCursor stores cursor in the cursor file.
func writeCursor(geom Geometry, cursor cursorRecord) error {
	if err := state.WriteJSON(geom.CursorPath, recordLockPath(geom.CursorPath), cursor); err != nil {
		return fmt.Errorf("slack: write the cursor file %s: %w", geom.CursorPath, err)
	}
	return nil
}

// slackTimestamp renders t as a Slack message timestamp, `<seconds>.<micros>`.
func slackTimestamp(t time.Time) string {
	return fmt.Sprintf("%d.%06d", t.Unix(), t.Nanosecond()/1000)
}

// poll reads the operator's top-level messages after the cursor and handles each once, oldest first.
// It returns whether anything was forwarded, answered or the presence changed;
// the returned error leaves the cursor before the message it concerns, so the relay retries it.
// A first poll, and the first after the setup names another channel, moves the cursor to the channel's newest message;
// it forwards nothing from before.
func (in *inbound) poll(ctx context.Context) (bool, error) {
	setup, found, err := ReadSetup(in.geom.SetupPath)
	if err != nil {
		return false, err
	}
	if !found {
		return false, fmt.Errorf("slack: the setup file %s is gone; way forward: run `lym slack setup --channel <id> --operator <user-id>` from the operator's terminal", in.geom.SetupPath)
	}
	activity := in.presenceChanged()
	cursor, err := in.cursorFor(ctx, setup)
	if err != nil {
		return activity, err
	}
	messages, err := in.slack.History(ctx, setup.ChannelID, cursor.TS)
	if err != nil {
		return activity, in.readFailed(err)
	}
	in.apiNoticed = false
	for _, message := range messages {
		handled, err := in.handle(ctx, setup, message)
		activity = activity || handled
		if err != nil {
			return activity, err
		}
		if err := writeCursor(in.geom, cursorRecord{Channel: setup.ChannelID, TS: message.TS}); err != nil {
			return activity, err
		}
	}
	return activity, nil
}

// presenceChanged reads the presence and reports whether it differs from the one the previous poll read.
// A presence that cannot be read reports no change.
func (in *inbound) presenceChanged() bool {
	presence, err := ReadPresence(in.geom.PresencePath)
	if err != nil {
		return false
	}
	changed := in.presenceKnown && presence != in.lastPresence
	in.lastPresence, in.presenceKnown = presence, true
	return changed
}

// cursorFor returns the cursor of the configured channel;
// when none is written or it is for another channel, it first writes the cursor at the channel's newest message.
func (in *inbound) cursorFor(ctx context.Context, setup Setup) (cursorRecord, error) {
	cursor, found, err := readCursor(in.geom)
	if err != nil {
		return cursorRecord{}, err
	}
	if found && cursor.Channel == setup.ChannelID {
		return cursor, nil
	}
	ts, latestFound, err := in.slack.Latest(ctx, setup.ChannelID)
	if err != nil {
		return cursorRecord{}, in.readFailed(err)
	}
	if !latestFound {
		ts = slackTimestamp(in.clock.Now())
	}
	cursor = cursorRecord{Channel: setup.ChannelID, TS: ts}
	return cursor, writeCursor(in.geom, cursor)
}

// readFailed mails the hub's orch one `[slack relay]` notice for the first Slack refusal of an episode and returns err.
// An error that is no refusal, a rate limit or a transport failure, mails nothing.
func (in *inbound) readFailed(err error) error {
	refusal, ok := isAPIError(err)
	if !ok || in.apiNoticed {
		return err
	}
	receiver, nameErr := orchName(in.geom.Shortname, "")
	if nameErr != nil {
		return errors.Join(err, nameErr)
	}
	body := fmt.Sprintf("%s Slack refused the relay's read with %s; way forward: run `lym slack login` for a refused token or `lym slack setup` for a channel the bot cannot read, from the operator's terminal", relayNoticePrefix, refusal.Code)
	if _, postErr := in.mailbox.Post(receiver, body); postErr != nil {
		logger.Warn("slackengine: the relay notice for a Slack refusal was not posted", "receiver", receiver, "error", postErr)
		return err
	}
	in.apiNoticed = true
	return err
}

// handle handles one message of the channel and reports whether it forwarded or answered anything.
// An error means the message is not handled yet and the cursor stays before it.
func (in *inbound) handle(ctx context.Context, setup Setup, message Message) (bool, error) {
	if message.Subtype != "" || message.BotID != "" || message.User != setup.OperatorID || (message.ThreadTS != "" && message.ThreadTS != message.TS) {
		logger.Debug("slackengine: skipped a message that is not the operator's top-level text", "ts", message.TS)
		return false, nil
	}
	text := inboundText(message.Text)
	switch strings.ToLower(strings.TrimSpace(text)) {
	case "away":
		return in.setPresence(ctx, setup, message, PresenceSlack)
	case "back":
		return in.setPresence(ctx, setup, message, PresenceTerminal)
	case "status":
		return in.status(ctx, setup, message)
	}
	return in.forward(ctx, setup, message, text)
}

// setPresence sets the presence for an `away` or `back` command and answers with the resulting state.
// A presence that was not written is retried;
// a write that stood with a notice or beams failure is logged once per episode and counts as handled.
func (in *inbound) setPresence(ctx context.Context, setup Setup, message Message, presence Presence) (bool, error) {
	changed, err := SetPresence(in.geom, in.mailbox, in.hub.Beams, presence, in.clock.Now())
	switch {
	case err != nil && !changed:
		return in.retryOrGiveUp(ctx, setup, message, "the presence was not set", err)
	case err != nil:
		in.episodes.Fail("presence-notice", "slackengine: the presence was written but a notice failed", "presence", presence, "error", err)
	default:
		in.episodes.Recover("presence-notice", "slackengine: presence notices succeed again")
	}
	in.lastPresence, in.presenceKnown = presence, true
	in.settle()
	in.answer(ctx, setup, fmt.Sprintf("presence is now %s", presence))
	return true, nil
}

// status answers a `status` command with the presence.
func (in *inbound) status(ctx context.Context, setup Setup, message Message) (bool, error) {
	presence, err := ReadPresence(in.geom.PresencePath)
	if err != nil {
		return in.retryOrGiveUp(ctx, setup, message, "the presence could not be read", err)
	}
	in.lastPresence, in.presenceKnown = presence, true
	in.settle()
	in.answer(ctx, setup, fmt.Sprintf("presence is %s", presence))
	return true, nil
}

// forward posts a message of the operator to the orch it is for.
func (in *inbound) forward(ctx context.Context, setup Setup, message Message, text string) (bool, error) {
	slug, mentioned := mentionedSlug(text)
	var beams []string
	if mentioned {
		var err error
		if beams, err = in.hub.Beams(); err != nil {
			return false, fmt.Errorf("slack: read the hub's beams to route a message for %q: %w", slug, err)
		}
	}
	receiver, body, ok := route(in.geom.Shortname, text, beams)
	if !ok {
		listing := "none"
		if len(beams) > 0 {
			listing = strings.Join(beams, ", ")
		}
		in.answer(ctx, setup, fmt.Sprintf("no beam %q; the hub's beams are: %s", slug, listing))
		return true, nil
	}
	posted := forwardPrefix + body
	if len(posted) > creelBodyLimit {
		in.answer(ctx, setup, fmt.Sprintf("that message is %d bytes, over the %d the hub's mail takes; send a shorter one", len(posted), creelBodyLimit))
		return true, nil
	}
	if _, err := in.mailbox.Post(receiver, posted); err != nil {
		return in.retryOrGiveUp(ctx, setup, message, "your message was not delivered; send it again", err)
	}
	in.settle()
	paused, err := in.hub.Paused(receiver)
	switch {
	case err != nil:
		in.episodes.Fail("paused", "slackengine: could not read whether the hub is paused", "receiver", receiver, "error", err)
	case paused:
		in.episodes.Recover("paused", "slackengine: the pause read succeeds again")
		in.answer(ctx, setup, "the hub is paused; your message waits for the resume")
	default:
		in.episodes.Recover("paused", "slackengine: the pause read succeeds again")
	}
	return true, nil
}

// retryOrGiveUp handles a message that could not be handled:
// it returns err, leaving the cursor before the message, until the message's first failure is retryWindow old;
// then it answers in Slack with reason and err and reports the message handled,
// so it holds the later ones no longer than that.
func (in *inbound) retryOrGiveUp(ctx context.Context, setup Setup, message Message, reason string, err error) (bool, error) {
	now := in.clock.Now()
	if in.failingTS != message.TS {
		in.failingTS, in.failingSince = message.TS, now
	}
	if now.Sub(in.failingSince) >= retryWindow {
		in.settle()
		in.answer(ctx, setup, fmt.Sprintf("%s: %v", reason, err))
		return true, nil
	}
	in.episodes.Fail("message", "slackengine: a message could not be handled and is retried", "ts", message.TS, "error", err)
	return false, err
}

// settle ends the episode of a message that could not be handled.
func (in *inbound) settle() {
	in.failingTS = ""
	in.episodes.Recover("message", "slackengine: the message was handled")
}

// answer posts text to the channel, escaped.
// An answer Slack refuses is logged once per episode and never holds the message it answers.
func (in *inbound) answer(ctx context.Context, setup Setup, text string) {
	if err := in.slack.PostMessage(ctx, setup.ChannelID, slackEscape.Replace(text)); err != nil {
		in.episodes.Fail("answer", "slackengine: Slack refused an answer", "error", err)
		return
	}
	in.episodes.Recover("answer", "slackengine: answers post again")
}
