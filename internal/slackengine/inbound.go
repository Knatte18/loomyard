// inbound.go reads the hub's Slack channels on a hint, a resync or the relay's start.
// It forwards the operator's messages to the orch of the channel they were written in, and answers the presence commands.

package slackengine

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/creelengine"
	"github.com/Knatte18/loomyard/internal/logger"
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
	// notInChannelCode is the code Slack answers a read or a post in a channel the bot is no member of with.
	notInChannelCode = "not_in_channel"
	// fileShareSubtype and threadBroadcastSubtype are the subtypes of a message the relay forwards besides a plain one.
	fileShareSubtype       = "file_share"
	threadBroadcastSubtype = "thread_broadcast"
	// hintThreadSeparator joins the channel and the thread timestamp in the name of a thread hint.
	hintThreadSeparator = "_"
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
	// History reads the channel's top-level messages after oldest, oldest first.
	History(ctx context.Context, channel, oldest string) ([]Message, error)
	// Replies reads the messages of the thread started at thread after oldest, oldest first.
	// The thread's first message is included.
	Replies(ctx context.Context, channel, thread, oldest string) ([]Message, error)
	// Latest returns the timestamp of the channel's newest message, found false for an empty channel.
	Latest(ctx context.Context, channel string) (string, bool, error)
	// PostAs posts text to the channel under the sender's name and icon; a Sender field left empty leaves Slack's default.
	PostAs(ctx context.Context, channel, text string, sender Sender) error
	// CreateChannel creates a public channel called name; a name that is taken is an *APIError with the code `name_taken`.
	CreateChannel(ctx context.Context, name string) (ChannelInfo, error)
	// FindChannel finds the public channel called name, archived ones included, and reports found false when there is none.
	FindChannel(ctx context.Context, name string) (ChannelInfo, bool, error)
	// Join makes the bot a member of the public channel.
	Join(ctx context.Context, channel string) error
	// Invite adds the Slack user to the channel; a user who is already a member counts as invited.
	Invite(ctx context.Context, channel, user string) error
	// Rename renames the channel to name; a name that is taken is an *APIError with the code `name_taken`.
	Rename(ctx context.Context, channel, name string) error
	// Archive archives the channel; a channel that is already archived counts as archived.
	Archive(ctx context.Context, channel string) error
	// AuthTest checks the token and tells who it belongs to.
	AuthTest(ctx context.Context) (Identity, error)
}

// Hub is the hub state the relay reads that is not in its own files.
type Hub struct {
	// Beams lists the slugs of the hub's beams.
	Beams func() ([]string, error)
	// Paused reports whether a pause covers the orch named receiver.
	Paused func(receiver string) (bool, error)
}

// owedRead is a read of one orch's channel, or of one thread in it, that is still to be made.
// Reads of the same orch and thread are one.
type owedRead struct {
	// orch is the creel name of the orch whose channel is read.
	orch string
	// thread is the timestamp of the thread to read, empty for the channel's top-level read.
	thread string
	// scan adds the 24-hour scan of the channel's threads to a top-level read.
	scan bool
	// notBefore is the earliest time to make the read, zero for none; a rate limit's retry wait sets it.
	notBefore time.Time
}

// inbound reads the hub's Slack channels and holds the state that carries from one read to the next.
// Its reads run one at a time.
type inbound struct {
	geom    Geometry
	mailbox Mailbox
	slack   SlackClient
	hub     Hub
	clock   Clock
	keeper  *channelKeeper

	mu sync.Mutex
	// owed holds the reads a failure or the start left to make.
	owed []owedRead
	// lastPresence is the presence the previous drain read, valid while presenceKnown.
	lastPresence  Presence
	presenceKnown bool
	// refusedOrchs holds the orchs whose current episode of a Slack refusal posted its notice.
	refusedOrchs map[string]bool
	// failingKey and failingSince name the message that cannot be handled yet and when it first failed.
	failingKey   string
	failingSince time.Time
	episodes     logger.EpisodeSet
}

// newInbound returns an inbound over the seams.
func newInbound(geom Geometry, mailbox Mailbox, slack SlackClient, hub Hub, clock Clock, keeper *channelKeeper) *inbound {
	return &inbound{
		geom: geom, mailbox: mailbox, slack: slack, hub: hub, clock: clock, keeper: keeper,
		refusedOrchs: map[string]bool{},
	}
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

// fileLines renders one `(file not relayed: <name>)` line per file, each preceded by a newline.
// The relay never downloads a file, so the line is all the orch learns of it.
func fileLines(files []File) string {
	var lines strings.Builder
	for _, file := range files {
		fmt.Fprintf(&lines, "\n(file not relayed: %s)", file.Name)
	}
	return lines.String()
}

// orchName forms the name of the orch of the beam slug, the hub's own orch for an empty slug.
func orchName(shortname, slug string) (string, error) {
	return agentname.Format(shortname, slug, agentname.RoleOrch)
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

// slackTimestamp renders t as a Slack message timestamp, `<seconds>.<micros>`.
func slackTimestamp(t time.Time) string {
	return fmt.Sprintf("%d.%06d", t.Unix(), t.Nanosecond()/1000)
}

// parseHint reads the name of a hint: `<channel>` for a channel read and `<channel>_<thread ts>` for a thread read.
// ok is false for a name of any other shape.
func parseHint(name string) (channel, thread string, ok bool) {
	channel, thread, hasThread := strings.Cut(name, hintThreadSeparator)
	if channel == "" || (hasThread && (thread == "" || strings.Contains(thread, hintThreadSeparator))) {
		return "", "", false
	}
	return channel, thread, true
}

// addRead adds read to reads, merging it into the entry of the same orch and thread.
// A merged entry scans when either did and takes the later read's earliest time.
func addRead(reads []owedRead, read owedRead) []owedRead {
	for i, held := range reads {
		if held.orch == read.orch && held.thread == read.thread {
			reads[i].scan = held.scan || read.scan
			reads[i].notBefore = read.notBefore
			return reads
		}
	}
	return append(reads, read)
}

// openOrchs lists the orchs whose channel the map holds open, sorted.
func openOrchs(channelMap ChannelMap) []string {
	orchs := make([]string, 0, len(channelMap.Channels))
	for orch, entry := range channelMap.Channels {
		if !entry.Archived {
			orchs = append(orchs, orch)
		}
	}
	sort.Strings(orchs)
	return orchs
}

// start owes every open channel of the hub's map a read with its thread scan.
// retryOwed makes the reads.
func (in *inbound) start() {
	in.mu.Lock()
	defer in.mu.Unlock()
	channelMap, err := ReadChannelMap(in.geom.ChannelMapPath)
	if err != nil {
		logger.Warn("slackengine: the start read of the hub's channels needs the channel map", "error", err)
		return
	}
	for _, orch := range openOrchs(channelMap) {
		in.owed = addRead(in.owed, owedRead{orch: orch, scan: true})
	}
}

// drain makes the reads the inbox's hints ask for, once per wake.
// It reports whether any read handled something or the presence changed.
// A hint is removed before the read it triggers, so a hint written during the read is read at the next drain.
// A hint whose name does not parse, or whose channel the hub's map does not hold open, is removed unread.
// A read that fails on something other than a Slack refusal stays owed for retryOwed.
// The reads of the other hints go on past a failed one, and the returned error joins every failure.
// It returns an error when the hub's setup file is gone.
func (in *inbound) drain(ctx context.Context) (bool, error) {
	in.mu.Lock()
	defer in.mu.Unlock()
	setup, found, err := ReadSetup(in.geom.SetupPath)
	if err != nil {
		return false, err
	}
	if !found {
		return false, fmt.Errorf("slack: the setup file %s is gone; way forward: run `lym slack setup --operator <user-id>` from the operator's terminal", in.geom.SetupPath)
	}
	activity := in.presenceChanged()
	reads, err := in.takeHints()
	if err != nil {
		return activity, err
	}
	var failures []error
	for i, read := range reads {
		handled, err := in.attempt(ctx, setup, read)
		activity = activity || handled
		if err == nil {
			continue
		}
		failures = append(failures, err)
		if limit, limited := isRateLimit(err); limited {
			for _, rest := range reads[i+1:] {
				rest.notBefore = in.clock.Now().Add(limit.RetryAfter)
				in.owed = addRead(in.owed, rest)
			}
			break
		}
	}
	return activity, errors.Join(failures...)
}

// retryOwed makes each owed read whose time has come and reports whether any handled something.
// A read that fails again stays owed.
// The owed reads live in memory and die with the relay, whose next start reads every open channel.
func (in *inbound) retryOwed(ctx context.Context) bool {
	in.mu.Lock()
	defer in.mu.Unlock()
	if len(in.owed) == 0 {
		return false
	}
	setup, found, err := ReadSetup(in.geom.SetupPath)
	if err != nil || !found {
		logger.Debug("slackengine: owed reads wait for the setup file", "path", in.geom.SetupPath, "error", err)
		return false
	}
	now := in.clock.Now()
	var due []owedRead
	for _, read := range in.owed {
		if !now.Before(read.notBefore) {
			due = append(due, read)
		}
	}
	activity := false
	for _, read := range due {
		key := "owed:" + read.orch + " " + read.thread
		handled, err := in.attempt(ctx, setup, read)
		activity = activity || handled
		if err == nil {
			in.episodes.Recover(key, "slackengine: an owed Slack read succeeds again", "orch", read.orch, "thread", read.thread)
			continue
		}
		in.episodes.Fail(key, "slackengine: an owed Slack read failed again", "orch", read.orch, "thread", read.thread, "error", err)
		if _, limited := isRateLimit(err); limited {
			break
		}
	}
	return activity
}

// takeHints removes each hint in the inbox and returns the reads they ask for, in the hints' name order.
// A name that starts with a dot is a hint being written and is left alone.
func (in *inbound) takeHints() ([]owedRead, error) {
	entries, err := os.ReadDir(in.geom.InboxDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("slack: list the inbox %s: %w", in.geom.InboxDir, err)
	}
	var names []string
	for _, entry := range entries {
		if !entry.IsDir() && !strings.HasPrefix(entry.Name(), ".") {
			names = append(names, entry.Name())
		}
	}
	if len(names) == 0 {
		return nil, nil
	}
	channelMap, err := ReadChannelMap(in.geom.ChannelMapPath)
	if err != nil {
		return nil, err
	}
	var reads []owedRead
	for _, name := range names {
		if err := os.Remove(filepath.Join(in.geom.InboxDir, name)); err != nil && !errors.Is(err, os.ErrNotExist) {
			logger.Debug("slackengine: could not remove a hint", "hint", name, "error", err)
		}
		for _, read := range readsForHint(channelMap, name) {
			reads = addRead(reads, read)
		}
	}
	return reads, nil
}

// readsForHint returns the reads the hint called name asks for.
// A resync asks for every open channel's read; any other hint asks for one channel's or thread's.
func readsForHint(channelMap ChannelMap, name string) []owedRead {
	if name == resyncHint {
		var reads []owedRead
		for _, orch := range openOrchs(channelMap) {
			reads = append(reads, owedRead{orch: orch, scan: true})
		}
		return reads
	}
	channel, thread, ok := parseHint(name)
	if !ok {
		logger.Debug("slackengine: removed a hint whose name does not parse", "hint", name)
		return nil
	}
	orch, held := channelMap.OrchOf(channel)
	if !held || channelMap.Channels[orch].Archived {
		logger.Debug("slackengine: removed a hint that names no open channel of the hub", "hint", name)
		return nil
	}
	return []owedRead{{orch: orch, thread: thread}}
}

// attempt makes one read.
// A read that succeeds is no longer owed.
// A read that fails stays owed.
// After a rate limit it is owed from the end of the retry wait.
// A Slack refusal or a refused channel is not owed, since retrying does not clear it.
func (in *inbound) attempt(ctx context.Context, setup Setup, read owedRead) (bool, error) {
	var handled bool
	var err error
	if read.thread == "" {
		handled, err = in.readChannel(ctx, setup, read.orch, read.scan)
	} else {
		handled, err = in.readThread(ctx, setup, read.orch, read.thread)
	}
	if err == nil {
		in.clearOwed(read)
		return handled, nil
	}
	if _, refused := isAPIError(err); refused || errors.Is(err, errChannelRefused) {
		in.clearOwed(read)
		return handled, err
	}
	read.notBefore = time.Time{}
	if limit, limited := isRateLimit(err); limited {
		read.notBefore = in.clock.Now().Add(limit.RetryAfter)
	}
	in.owed = addRead(in.owed, read)
	return handled, err
}

// clearOwed drops the owed entry of the read's orch and thread, unless the entry scans and the read did not.
func (in *inbound) clearOwed(read owedRead) {
	for i, held := range in.owed {
		if held.orch == read.orch && held.thread == read.thread && (read.scan || !held.scan) {
			in.owed = append(in.owed[:i], in.owed[i+1:]...)
			return
		}
	}
}

// presenceChanged reads the presence and reports whether it differs from the one the previous drain read.
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

// pruneEntry drops the orch's thread cursors older than 24 hours and returns the orch's entry.
// open is false when the map holds no open entry for the orch.
func (in *inbound) pruneEntry(orch string) (ChannelEntry, bool, error) {
	var pruned ChannelEntry
	open := false
	err := UpdateChannelMap(in.geom.ChannelMapPath, func(channelMap *ChannelMap) error {
		entry, held := channelMap.Channels[orch]
		if !held || entry.Archived {
			return nil
		}
		entry.PruneThreads(in.clock.Now())
		channelMap.Channels[orch] = entry
		pruned, open = entry, true
		return nil
	})
	return pruned, open, err
}

// advance applies change to the orch's entry when it still holds the channel id.
func (in *inbound) advance(orch, channelID string, change func(*ChannelEntry)) error {
	return UpdateChannelMap(in.geom.ChannelMapPath, func(channelMap *ChannelMap) error {
		if entry, held := channelMap.Channels[orch]; held && entry.ID == channelID {
			change(&entry)
			channelMap.Channels[orch] = entry
		}
		return nil
	})
}

// readWithJoin runs read.
// A start or resync read (scan set) that Slack refuses with `not_in_channel` first joins the bot to the channel.
// It then runs read once more.
func (in *inbound) readWithJoin(ctx context.Context, orch string, scan bool, read func() ([]Message, error)) ([]Message, error) {
	messages, err := read()
	if !scan {
		return messages, err
	}
	if refusal, refused := isAPIError(err); !refused || refusal.Code != notInChannelCode {
		return messages, err
	}
	if joinErr := in.keeper.join(ctx, orch); joinErr != nil {
		return nil, joinErr
	}
	return read()
}

// readChannel handles the orch's top-level messages after the channel's cursor, oldest first.
// It moves the cursor past each.
// With scan it then reads the replies of the threads the last 24 hours' messages started.
// It reports whether anything was forwarded or answered.
// An error leaves the cursor before the message it concerns, so the read can be made again.
func (in *inbound) readChannel(ctx context.Context, setup Setup, orch string, scan bool) (bool, error) {
	entry, open, err := in.pruneEntry(orch)
	if err != nil || !open {
		return false, err
	}
	messages, err := in.readWithJoin(ctx, orch, scan, func() ([]Message, error) {
		return in.slack.History(ctx, entry.ID, entry.Cursor)
	})
	if err != nil {
		return false, in.readFailed(orch, err)
	}
	delete(in.refusedOrchs, orch)
	activity := false
	for _, message := range messages {
		handled, err := in.handle(ctx, setup, orch, entry.ID, message)
		activity = activity || handled
		if err != nil {
			return activity, err
		}
		if err := in.advance(orch, entry.ID, func(current *ChannelEntry) { current.Advance(message.TS) }); err != nil {
			return activity, err
		}
	}
	if !scan {
		return activity, nil
	}
	scanned, err := in.scanThreads(ctx, setup, orch, entry.ID)
	return activity || scanned, err
}

// scanThreads reads the replies of each thread started within the last 24 hours that has a reply past its cursor.
func (in *inbound) scanThreads(ctx context.Context, setup Setup, orch, channelID string) (bool, error) {
	oldest := slackTimestamp(in.clock.Now().Add(-threadRetention))
	messages, err := in.readWithJoin(ctx, orch, true, func() ([]Message, error) {
		return in.slack.History(ctx, channelID, oldest)
	})
	if err != nil {
		return false, in.readFailed(orch, err)
	}
	entry, open, err := in.pruneEntry(orch)
	if err != nil || !open {
		return false, err
	}
	activity := false
	for _, message := range messages {
		if message.LatestReply == "" || compareTS(message.LatestReply, entry.ThreadStart(message.TS)) <= 0 {
			continue
		}
		handled, err := in.readThread(ctx, setup, orch, message.TS)
		activity = activity || handled
		if err != nil {
			return activity, err
		}
	}
	return activity, nil
}

// readThread handles the replies of the thread after its cursor, oldest first, and moves the thread's cursor past each.
// It skips the thread's own first message and every `thread_broadcast`, which the channel's read forwards.
// It reports whether anything was forwarded or answered.
func (in *inbound) readThread(ctx context.Context, setup Setup, orch, thread string) (bool, error) {
	entry, open, err := in.pruneEntry(orch)
	if err != nil || !open {
		return false, err
	}
	replies, err := in.slack.Replies(ctx, entry.ID, thread, entry.ThreadStart(thread))
	if err != nil {
		return false, in.readFailed(orch, err)
	}
	delete(in.refusedOrchs, orch)
	activity := false
	for _, reply := range replies {
		if reply.TS == thread {
			continue
		}
		if reply.Subtype != threadBroadcastSubtype {
			handled, err := in.handle(ctx, setup, orch, entry.ID, reply)
			activity = activity || handled
			if err != nil {
				return activity, err
			}
		}
		if err := in.advance(orch, entry.ID, func(current *ChannelEntry) { current.AdvanceThread(thread, reply.TS) }); err != nil {
			return activity, err
		}
	}
	return activity, nil
}

// readFailed returns err.
// It first mails the hub's orch one `[slack relay]` notice for the orch's first Slack refusal of an episode.
// The episode of each orch ends when a read of its channel stands.
// An error that is no refusal, a rate limit or a transport failure, mails nothing.
func (in *inbound) readFailed(orch string, err error) error {
	refusal, ok := isAPIError(err)
	if !ok || in.refusedOrchs[orch] {
		return err
	}
	receiver, nameErr := orchName(in.geom.Shortname, "")
	if nameErr != nil {
		return errors.Join(err, nameErr)
	}
	body := fmt.Sprintf("%s Slack refused the relay's read of the channel of %s with %s; way forward: run `lym slack login` from the operator's terminal for a refused token, and check the app against the manifest in docs/slack-app.md",
		relayNoticePrefix, orch, refusal.Code)
	if _, postErr := in.mailbox.Post(receiver, body); postErr != nil {
		logger.Warn("slackengine: the relay notice for a Slack refusal was not posted", "receiver", receiver, "error", postErr)
		return err
	}
	in.refusedOrchs[orch] = true
	return err
}

// handle handles one message of the orch's channel and reports whether it forwarded or answered anything.
// It forwards the setup's operator's message of no subtype, `file_share` or `thread_broadcast`;
// any other subtype, a bot's message and anyone else's are skipped.
// An error means the message is not handled yet and the cursor stays before it.
func (in *inbound) handle(ctx context.Context, setup Setup, orch, channel string, message Message) (bool, error) {
	forwardedSubtype := message.Subtype == "" || message.Subtype == fileShareSubtype || message.Subtype == threadBroadcastSubtype
	if !forwardedSubtype || message.BotID != "" || message.User != setup.OperatorID {
		logger.Debug("slackengine: skipped a message that is not the operator's text", "ts", message.TS)
		return false, nil
	}
	text := inboundText(message.Text)
	switch strings.ToLower(strings.TrimSpace(text)) {
	case "away":
		return in.setPresence(ctx, channel, message, PresenceSlack)
	case "back":
		return in.setPresence(ctx, channel, message, PresenceTerminal)
	case "status":
		return in.status(ctx, channel, message)
	}
	return in.forward(ctx, orch, channel, message, text)
}

// setPresence sets the presence for an `away` or `back` command and answers in the channel with the resulting state.
// A presence that was not written is retried;
// a write that stood with a notice or beams failure is logged once per episode and counts as handled.
func (in *inbound) setPresence(ctx context.Context, channel string, message Message, presence Presence) (bool, error) {
	changed, err := SetPresence(in.geom, in.mailbox, in.hub.Beams, presence, in.clock.Now())
	switch {
	case err != nil && !changed:
		return in.retryOrGiveUp(ctx, channel, message, "the presence was not set", err)
	case err != nil:
		in.episodes.Fail("presence-notice", "slackengine: the presence was written but a notice failed", "presence", presence, "error", err)
	default:
		in.episodes.Recover("presence-notice", "slackengine: presence notices succeed again")
	}
	in.lastPresence, in.presenceKnown = presence, true
	in.settle()
	in.answer(ctx, channel, fmt.Sprintf("presence is now %s", presence))
	return true, nil
}

// status answers a `status` command in the channel with the presence.
func (in *inbound) status(ctx context.Context, channel string, message Message) (bool, error) {
	presence, err := ReadPresence(in.geom.PresencePath)
	if err != nil {
		return in.retryOrGiveUp(ctx, channel, message, "the presence could not be read", err)
	}
	in.lastPresence, in.presenceKnown = presence, true
	in.settle()
	in.answer(ctx, channel, fmt.Sprintf("presence is %s", presence))
	return true, nil
}

// forward posts a message of the operator to orch, the orch of the channel it was written in.
// The text goes as it stands, an `@<slug>` prefix included, followed by a line per attached file.
func (in *inbound) forward(ctx context.Context, orch, channel string, message Message, text string) (bool, error) {
	posted := forwardPrefix + strings.TrimPrefix(text+fileLines(message.Files), "\n")
	if len(posted) > creelBodyLimit {
		in.answer(ctx, channel, fmt.Sprintf("that message is %d bytes, over the %d the hub's mail takes; send a shorter one", len(posted), creelBodyLimit))
		return true, nil
	}
	if _, err := in.mailbox.Post(orch, posted); err != nil {
		return in.retryOrGiveUp(ctx, channel, message, "your message was not delivered; send it again", err)
	}
	in.settle()
	paused, err := in.hub.Paused(orch)
	switch {
	case err != nil:
		in.episodes.Fail("paused", "slackengine: could not read whether the hub is paused", "receiver", orch, "error", err)
	case paused:
		in.episodes.Recover("paused", "slackengine: the pause read succeeds again")
		in.answer(ctx, channel, "the hub is paused; your message waits for the resume")
	default:
		in.episodes.Recover("paused", "slackengine: the pause read succeeds again")
	}
	return true, nil
}

// retryOrGiveUp handles a message that could not be handled:
// it returns err, leaving the cursor before the message, until the message's first failure is retryWindow old;
// then it answers in the channel with reason and err and reports the message handled,
// so it holds the channel's later messages no longer than that.
func (in *inbound) retryOrGiveUp(ctx context.Context, channel string, message Message, reason string, err error) (bool, error) {
	now := in.clock.Now()
	key := channel + " " + message.TS
	if in.failingKey != key {
		in.failingKey, in.failingSince = key, now
	}
	if now.Sub(in.failingSince) >= retryWindow {
		in.settle()
		in.answer(ctx, channel, fmt.Sprintf("%s: %v", reason, err))
		return true, nil
	}
	in.episodes.Fail("message", "slackengine: a message could not be handled and is retried", "ts", message.TS, "error", err)
	return false, err
}

// settle ends the episode of a message that could not be handled.
func (in *inbound) settle() {
	in.failingKey = ""
	in.episodes.Recover("message", "slackengine: the message was handled")
}

// answer posts text to the channel, escaped.
// An answer Slack refuses with `not_in_channel` joins the bot to the channel's orch and posts once more.
// An answer Slack refuses otherwise is logged once per episode and never holds the message it answers.
func (in *inbound) answer(ctx context.Context, channel, text string) {
	escaped := slackEscape.Replace(text)
	err := in.slack.PostAs(ctx, channel, escaped, Sender{})
	if refusal, refused := isAPIError(err); refused && refusal.Code == notInChannelCode {
		err = in.postAfterJoin(ctx, channel, escaped, err)
	}
	if err != nil {
		in.episodes.Fail("answer", "slackengine: Slack refused an answer", "error", err)
		return
	}
	in.episodes.Recover("answer", "slackengine: answers post again")
}

// postAfterJoin joins the bot to the orch the map gives for the channel and posts text once more.
// It returns refused, the refusal that asked for the join, when the map names no orch for the channel.
func (in *inbound) postAfterJoin(ctx context.Context, channel, text string, refused error) error {
	channelMap, err := ReadChannelMap(in.geom.ChannelMapPath)
	if err != nil {
		return err
	}
	orch, held := channelMap.OrchOf(channel)
	if !held {
		return refused
	}
	if err := in.keeper.join(ctx, orch); err != nil {
		return err
	}
	return in.slack.PostAs(ctx, channel, text, Sender{})
}
