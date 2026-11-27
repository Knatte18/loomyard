// bridgein.go is the relay's receiving of machine mail.
// It reads the hub's bridge channels and hands each message of an approved peer machine to creel.
// It also handles the operator's text in them and finds the bridge channels the bridge map lacks.

package slackengine

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/creelengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/machinename"
)

const (
	// bridgeResyncHint is the name of the hint that asks a hub's relay to look for bridge channels its bridge map lacks.
	bridgeResyncHint = "bridge-resync"
	// unpinnedHoldKind, wrongAppKind, fromClashKind, retiredOrchKind and deliverRefusalKind are notice kinds.
	// A read of a bridge channel posts them in it.
	// Their episodes end when a read moves past a message without that kind's state.
	unpinnedHoldKind   = "unpinned-hold"
	wrongAppKind       = "wrong-app"
	fromClashKind      = "from-clash"
	retiredOrchKind    = "retired-orch"
	deliverRefusalKind = "deliver-refusal"
	// unapprovedBotKind is the kind of the notice about a bridge channel an unapproved app made, one per channel.
	unapprovedBotKind = "unapproved-bot"
	// slackTimestampMicrosDigits is the number of digits after the point of a Slack timestamp.
	slackTimestampMicrosDigits = 6
	// resyncHistoryOldest asks Slack for a channel's whole history.
	resyncHistoryOldest = "0"
)

// receiveEpisodeKinds are the notice kinds a delivered message ends.
var receiveEpisodeKinds = []string{unpinnedHoldKind, wrongAppKind, fromClashKind, retiredOrchKind, deliverRefusalKind}

// isBridgeChannelName reports whether name has the shape of a bridge channel's name:
// the `bridge-` prefix and a `--` between two ends, or for a name cut to Slack's length limit the prefix alone.
func isBridgeChannelName(name string) bool {
	rest, found := strings.CutPrefix(name, bridgeChannelPrefix)
	if !found {
		return false
	}
	return len(name) == maxChannelNameLength || strings.Contains(rest, bridgeEndSeparator)
}

// bridgeReader reads the hub's bridge channels and finds the ones its bridge map lacks.
// One goroutine runs it, the bridge poll's.
type bridgeReader struct {
	geom     Geometry
	mailbox  Mailbox
	slack    SlackClient
	hub      Hub
	liveHubs LiveHubs
	clock    Clock
	keeper   *channelKeeper
	in       *inbound

	// operatorID and botUserID are the setup's operator and this machine's bot, loaded at the start of each read.
	operatorID string
	botUserID  string
	// botApps caches the app each bot id belongs to.
	botApps map[string]string
	// failingKey names the operator message whose command cannot be carried out yet.
	// failingSince is when it first failed.
	failingKey   string
	failingSince time.Time
}

// newBridgeReader returns a reader over the seams.
// in is used only to carry out the operator's peer commands, which read and write no state of its own.
func newBridgeReader(geom Geometry, mailbox Mailbox, slack SlackClient, hub Hub, liveHubs LiveHubs, clock Clock, keeper *channelKeeper, in *inbound) *bridgeReader {
	return &bridgeReader{
		geom: geom, mailbox: mailbox, slack: slack, hub: hub, liveHubs: liveHubs, clock: clock, keeper: keeper, in: in,
		botApps: map[string]string{},
	}
}

// receiveNoticeKey returns the key of a read-side notice of kind in the channel.
// The key holds whether or not the map holds the channel.
func receiveNoticeKey(kind, channelID string) string {
	return kind + ":" + channelID
}

// load reads what a read of the bridge channels needs: the setup's operator and this machine's bot.
func (r *bridgeReader) load(ctx context.Context) error {
	setup, found, err := ReadSetup(r.geom.SetupPath)
	if err != nil {
		return err
	}
	r.operatorID = ""
	if found {
		r.operatorID = setup.OperatorID
	}
	r.botUserID, err = r.keeper.botUser(ctx)
	return err
}

// read reads each open channel of the bridge map and its threads.
// It reports whether any message was delivered or any operator command in a thread was handled.
// A channel whose read fails does not stop the others; the returned error joins every failure.
func (r *bridgeReader) read(ctx context.Context) (bool, error) {
	bridgeMap, err := ReadBridgeMap(r.geom.BridgeMapPath)
	if err != nil {
		return false, err
	}
	ids := bridgeMap.OpenChannelIDs(nil)
	if len(ids) == 0 {
		return false, nil
	}
	if err := r.load(ctx); err != nil {
		return false, err
	}
	progressed := false
	var failures []error
	for _, id := range ids {
		delivered, err := r.readChannel(ctx, id)
		progressed = progressed || delivered
		if err == nil {
			var handled bool
			handled, err = r.readThreads(ctx, id)
			progressed = progressed || handled
		}
		if err == nil {
			continue
		}
		failures = append(failures, err)
		if _, limited := isRateLimit(err); limited {
			break
		}
	}
	return progressed, errors.Join(failures...)
}

// openEntry returns the channel's entry from the bridge map; open is false when the map holds no open entry for it.
func (r *bridgeReader) openEntry(channelID string) (BridgeEntry, bool, error) {
	bridgeMap, err := ReadBridgeMap(r.geom.BridgeMapPath)
	if err != nil {
		return BridgeEntry{}, false, err
	}
	entry, held := bridgeMap.Channels[channelID]
	return entry, held && !entry.Archived, nil
}

// readChannel handles the channel's top-level messages after its delivery cursor, oldest first.
// It reports whether any was delivered.
// The cursor moves past each message handled, until the first that is held.
// A message after a held one is never delivered, so the mail of a pair keeps its order;
// the operator's text is still handled.
// An operator command carried out while a message is held ends the pass,
// and the pass runs again from the persisted cursor.
// The held message is then tried before the next operator text is handled.
// The reruns are bounded by the operator's messages in the first read.
// An error leaves the cursor before the message it concerns, so the next read makes it again.
func (r *bridgeReader) readChannel(ctx context.Context, channelID string) (bool, error) {
	delivered := false
	passes := 1
	for pass := 0; pass < passes; pass++ {
		sent, operators, again, err := r.readChannelPass(ctx, channelID)
		delivered = delivered || sent
		if err != nil || !again {
			return delivered, err
		}
		if pass == 0 {
			passes = operators + 1
		}
	}
	return delivered, nil
}

// readChannelPass makes one pass over the channel's messages after its delivery cursor.
// It reports whether any message was delivered and how many of the messages are the operator's.
// It reports again when it ended early, because an operator command was carried out while a message was held.
func (r *bridgeReader) readChannelPass(ctx context.Context, channelID string) (delivered bool, operators int, again bool, err error) {
	entry, open, err := r.openEntry(channelID)
	if err != nil || !open {
		return false, 0, false, err
	}
	messages, err := r.slack.History(ctx, channelID, entry.Cursor)
	if err != nil {
		return false, 0, false, fmt.Errorf("slack: read the bridge channel %s: %w", entry.Name, err)
	}
	for _, message := range messages {
		if r.isOperator(message) {
			operators++
		}
	}
	held := false
	for _, message := range messages {
		operator := r.isOperator(message)
		if held && !operator {
			continue
		}
		commandDue := operator && compareTS(message.TS, entry.OperatorCursor) > 0
		moves, sent, err := r.handle(ctx, channelID, entry, message)
		delivered = delivered || sent
		if err != nil {
			return delivered, operators, false, err
		}
		if !moves {
			held = true
		}
		if held {
			if commandDue {
				return delivered, operators, true, nil
			}
			continue
		}
		if err := r.advanceCursor(channelID, message.TS); err != nil {
			return delivered, operators, false, err
		}
	}
	return delivered, operators, false, nil
}

// advanceCursor moves the channel's delivery cursor to ts when ts is later.
func (r *bridgeReader) advanceCursor(channelID, ts string) error {
	return UpdateBridgeMap(r.geom.BridgeMapPath, func(bridgeMap *BridgeMap) error {
		if entry, held := bridgeMap.Channels[channelID]; held && compareTS(ts, entry.Cursor) > 0 {
			entry.Cursor = ts
			bridgeMap.Channels[channelID] = entry
		}
		return nil
	})
}

// moveOperatorCursor moves the channel's operator cursor to ts when ts is later.
func (r *bridgeReader) moveOperatorCursor(channelID, ts string) error {
	return UpdateBridgeMap(r.geom.BridgeMapPath, func(bridgeMap *BridgeMap) error {
		if entry, held := bridgeMap.Channels[channelID]; held && compareTS(ts, entry.OperatorCursor) > 0 {
			entry.OperatorCursor = ts
			bridgeMap.Channels[channelID] = entry
		}
		return nil
	})
}

// isOperator reports whether message is the setup operator's own text.
func (r *bridgeReader) isOperator(message Message) bool {
	forwardedSubtype := message.Subtype == "" || message.Subtype == fileShareSubtype || message.Subtype == threadBroadcastSubtype
	return r.operatorID != "" && message.User == r.operatorID && message.BotID == "" && forwardedSubtype
}

// handle handles one message of a bridge channel.
// It answers whether the delivery cursor moves past the message and whether the message was delivered to creel.
// The first of these decides, as the plan's receiving rules list them:
// this bot's own post; the operator's text; any other human; a first line that is no envelope;
// a message past creel's retention; an envelope from this machine's own name;
// an app other than the pinned one; a machine with no pin; an envelope for another orch;
// a retired orch; an orch whose button is off; and then the delivery.
// A message held for an approval or a button keeps the cursor before it.
// An error leaves the cursor before the message too.
func (r *bridgeReader) handle(ctx context.Context, channelID string, entry BridgeEntry, message Message) (bool, bool, error) {
	if message.User == r.botUserID {
		return true, false, nil
	}
	if r.isOperator(message) {
		return r.handleOperator(ctx, channelID, entry, message)
	}
	if message.BotID == "" {
		return true, false, nil
	}
	envelope, body, ok := ParseBridgeMessage(message.Text)
	if !ok {
		return true, false, nil
	}
	return r.handleEnvelope(ctx, channelID, entry, message, envelope, inboundText(body))
}

// handleEnvelope handles a bridge message whose first line is a valid envelope.
// Every outcome but a hold or an error ends the notice episodes of the other kinds.
func (r *bridgeReader) handleEnvelope(ctx context.Context, channelID string, entry BridgeEntry, message Message, envelope BridgeEnvelope, body string) (bool, bool, error) {
	posted := slackMessageTime(message.TS)
	if r.clock.Now().Sub(posted) > creelengine.MachineInboundRetention {
		return r.passed(channelID, ""), false, nil
	}
	names, err := r.machineNames()
	if err != nil {
		return false, false, err
	}
	if holdsName(names, envelope.From.Machine) {
		r.notice(ctx, channelID, fromClashKind, fmt.Sprintf(
			"a message names %s, a name this machine holds or held, as its sender; it was not delivered; "+
				"way forward: ask the operator to give the other machine a name this machine never held",
			envelope.From.Machine))
		return r.passed(channelID, fromClashKind), false, nil
	}
	appID, err := r.appOf(ctx, message.BotID)
	if err != nil {
		return false, false, err
	}
	peers, err := ReadPeers(r.geom.PeersPath)
	if err != nil {
		return false, false, err
	}
	pin, pinned := peers.Machines[envelope.From.Machine]
	if pinned && pin.AppID != appID {
		r.notice(ctx, channelID, wrongAppKind, fmt.Sprintf(
			"a message from the app %s claims the machine %s, which is approved with another app, and was not delivered; "+
				"way forward: compare %s with the other machine's `status`, and if it is the real peer, type approve %s %s here to pin it",
			appID, envelope.From.Machine, appID, envelope.From.Machine, appID))
		return r.passed(channelID, wrongAppKind), false, nil
	}
	if !pinned {
		r.notice(ctx, channelID, unpinnedHoldKind, fmt.Sprintf(
			"mail from the machine %s waits because the machine is not approved; way forward: type approve %s here",
			envelope.From.Machine, envelope.From.Machine))
		return false, false, nil
	}
	if !holdsName(names, envelope.To.Machine) || envelope.To.Name.String() != entry.LocalOrch {
		return r.passed(channelID, ""), false, nil
	}
	live, err := r.orchLive(entry.LocalOrch)
	if err != nil {
		return false, false, err
	}
	if !live {
		r.notice(ctx, channelID, retiredOrchKind, fmt.Sprintf(
			"the orch %s is retired, so mail for it was not delivered", entry.LocalOrch))
		return r.passed(channelID, retiredOrchKind), false, nil
	}
	on, err := ForwardingOn(r.geom.ForwardingPath, entry.LocalOrch)
	if err != nil {
		return false, false, err
	}
	if !on {
		return false, false, nil
	}
	return r.deliver(ctx, channelID, entry, envelope, body, posted)
}

// deliver hands one message to creel.
// A refusal creel answers skips the message with one notice quoting creel's reason, so no refusal stops the channel.
// Any other error, a lost connection included, holds the cursor for the next poll.
func (r *bridgeReader) deliver(ctx context.Context, channelID string, entry BridgeEntry, envelope BridgeEnvelope, body string, posted time.Time) (bool, bool, error) {
	_, err := r.mailbox.DeliverMachine(creelengine.MachineMail{
		Sender:    formatBridgeAddress(envelope.From),
		Receiver:  entry.LocalOrch,
		SendingID: envelope.ID,
		ReplyTo:   envelope.ReplyTo,
		Body:      body,
		Posted:    posted,
	})
	switch {
	case creelengine.IsDaemonRefusal(err):
		r.notice(ctx, channelID, deliverRefusalKind, fmt.Sprintf(
			"creel refused the message %d from %s and it was not delivered: %v", envelope.ID, formatBridgeAddress(envelope.From), err))
		return r.passed(channelID, deliverRefusalKind), false, nil
	case err != nil:
		return false, false, fmt.Errorf("slack: deliver the message %d from %s to creel: %w", envelope.ID, formatBridgeAddress(envelope.From), err)
	}
	return r.passed(channelID, ""), true, nil
}

// passed ends the notice episodes of the channel that the message's outcome does not belong to, and answers true.
// except is the kind of the outcome, empty for an outcome with no episode.
func (r *bridgeReader) passed(channelID, except string) bool {
	notices, err := readBridgeNotices(r.geom.BridgeNoticesPath)
	if err != nil {
		logger.Warn("slackengine: the bridge-notice record could not be read to end an episode", "channel", channelID, "error", err)
		return true
	}
	for _, kind := range receiveEpisodeKinds {
		key := receiveNoticeKey(kind, channelID)
		if _, marked := notices.Marks[key]; kind == except || !marked {
			continue
		}
		if err := clearBridgeNotice(r.geom.BridgeNoticesPath, key); err != nil {
			logger.Warn("slackengine: the bridge notice could not be unmarked", "key", key, "error", err)
		}
	}
	return true
}

// notice posts line in the channel once per episode of kind.
// The bridge-notice record remembers the episode, so a restarted relay posts no second notice.
// A notice that cannot be posted ends its episode, so the next try posts it.
func (r *bridgeReader) notice(ctx context.Context, channelID, kind, line string) {
	key := receiveNoticeKey(kind, channelID)
	marked, err := markBridgeNotice(r.geom.BridgeNoticesPath, key)
	if err != nil {
		logger.Warn("slackengine: the bridge-notice record could not be written", "key", key, "error", err)
		return
	}
	if !marked {
		return
	}
	err = r.slack.PostAs(ctx, channelID, slackEscape.Replace(relayNoticePrefix+" "+line), relayProgramSender(r.geom))
	if err == nil {
		return
	}
	logger.Warn("slackengine: a bridge notice was not posted in the channel", "channel", channelID, "error", err)
	if clearErr := clearBridgeNotice(r.geom.BridgeNoticesPath, key); clearErr != nil {
		logger.Warn("slackengine: the bridge notice could not be unmarked", "key", key, "error", clearErr)
	}
}

// machineNames reads this machine's current name and its former names.
func (r *bridgeReader) machineNames() ([]string, error) {
	current, former, err := r.hub.MachineNames()
	if err != nil {
		return nil, fmt.Errorf("slack: read this machine's names: %w", err)
	}
	return append([]string{current}, former...), nil
}

// holdsName reports whether names holds machine, compared case-insensitively.
func holdsName(names []string, machine string) bool {
	return slices.ContainsFunc(names, func(name string) bool { return strings.EqualFold(name, machine) })
}

// appOf reads the id of the Slack app the bot belongs to, once per bot id.
func (r *bridgeReader) appOf(ctx context.Context, botID string) (string, error) {
	if appID, known := r.botApps[botID]; known {
		return appID, nil
	}
	appID, err := r.slack.BotAppID(ctx, botID)
	if err != nil {
		return "", fmt.Errorf("slack: read the app of the bot %s: %w", botID, err)
	}
	r.botApps[botID] = appID
	return appID, nil
}

// orchLive reports whether the orch is a live orch of this hub: the prime's, or a beam the hub's beams list.
func (r *bridgeReader) orchLive(orch string) (bool, error) {
	prime, err := orchName(r.geom.Shortname, "")
	if err != nil {
		return false, err
	}
	if orch == prime {
		return true, nil
	}
	slug, err := slugOf(orch)
	if err != nil {
		return false, err
	}
	beams, err := r.hub.Beams()
	if err != nil {
		return false, fmt.Errorf("slack: read the hub's beams to judge the orch %s: %w", orch, err)
	}
	return slices.Contains(beams, slug), nil
}

// slackMessageTime reads a Slack message timestamp, `<seconds>.<micros>`, as a time.
// A timestamp that does not read gives the zero time.
func slackMessageTime(ts string) time.Time {
	seconds, fraction, _ := strings.Cut(ts, ".")
	sec, err := strconv.ParseInt(seconds, 10, 64)
	if err != nil {
		return time.Time{}
	}
	fraction = (fraction + strings.Repeat("0", slackTimestampMicrosDigits))[:slackTimestampMicrosDigits]
	micros, err := strconv.ParseInt(fraction, 10, 64)
	if err != nil {
		micros = 0
	}
	return time.Unix(sec, micros*int64(time.Microsecond)).UTC()
}

// handleOperator handles the operator's top-level text in a bridge channel, once.
// Only a message after the operator cursor is handled, and the cursor then moves to it.
func (r *bridgeReader) handleOperator(ctx context.Context, channelID string, entry BridgeEntry, message Message) (bool, bool, error) {
	if compareTS(message.TS, entry.OperatorCursor) <= 0 {
		return true, false, nil
	}
	if err := r.operator(ctx, channelID, "", entry, message); err != nil && !r.givenUp(ctx, channelID, "", message, err) {
		return false, false, err
	}
	r.failingKey = ""
	if err := r.moveOperatorCursor(channelID, message.TS); err != nil {
		return false, false, err
	}
	return true, false, nil
}

// operator handles one text of the operator in a bridge channel, in thread when thread is set.
// An `approve` or `revoke` naming the channel's peer machine is carried out and answered.
// One naming this machine, by its current or a former name, is left to the other machine's relay.
// Any other text is answered once as not delivered, by the relay of the end that sorts first in the channel's name.
// It returns the error of a command that could not be carried out.
func (r *bridgeReader) operator(ctx context.Context, channelID, thread string, entry BridgeEntry, message Message) error {
	text := inboundText(message.Text)
	peer, err := agentname.ParseMachineAddress(entry.Peer, nil, machinename.CheckName)
	if err != nil {
		return fmt.Errorf("slack: the bridge entry of %s names no valid peer %q: %w", channelID, entry.Peer, err)
	}
	if command, isCommand := parsePeerCommand(text); isCommand {
		names, err := r.machineNames()
		if err != nil {
			return err
		}
		switch {
		case holdsName(names, command.Machine):
			return nil
		case command.Machine == peer.Machine:
			line, err := r.in.runPeerCommand(ctx, command)
			if err != nil {
				return err
			}
			r.answerIn(ctx, channelID, thread, line)
			return nil
		}
	}
	local, err := localBridgeAddress(r.geom.Machine, entry.LocalOrch)
	if err != nil {
		return err
	}
	if bridgeChannelEnd(local) > bridgeChannelEnd(peer) {
		return nil
	}
	own := "its own channel"
	if channelMap, err := ReadChannelMap(r.geom.ChannelMapPath); err == nil {
		if orchEntry, held := channelMap.Channels[entry.LocalOrch]; held && !orchEntry.Archived {
			own = orchEntry.Name
		}
	}
	r.answerIn(ctx, channelID, thread, fmt.Sprintf(
		"a bridge channel carries mail between orchs and nothing typed here reaches %s, so this was not delivered; write in %s instead",
		entry.LocalOrch, own))
	return nil
}

// givenUp reports whether a failing operator message is given up on.
// The message's first failure is recorded.
// Once it is retryWindow old, the failure is answered where the command was written.
// The caller then moves the cursor that held the message past it.
func (r *bridgeReader) givenUp(ctx context.Context, channelID, thread string, message Message, err error) bool {
	now := r.clock.Now()
	key := channelID + " " + message.TS
	if r.failingKey != key {
		r.failingKey, r.failingSince = key, now
	}
	if now.Sub(r.failingSince) < retryWindow {
		logger.Debug("slackengine: an operator command in a bridge channel could not be carried out and is retried", "key", key, "error", err)
		return false
	}
	r.answerIn(ctx, channelID, thread, fmt.Sprintf("this command could not be carried out: %v", err))
	return true
}

// answerIn posts text, escaped, in the channel, or in thread when thread is set.
// An answer Slack refuses is logged and never holds the message it answers.
func (r *bridgeReader) answerIn(ctx context.Context, channelID, thread, text string) {
	escaped := slackEscape.Replace(text)
	var err error
	if thread == "" {
		err = r.slack.PostAs(ctx, channelID, escaped, Sender{})
	} else {
		err = r.slack.PostThreadReply(ctx, channelID, thread, escaped)
	}
	if err != nil {
		logger.Debug("slackengine: Slack refused an answer in a bridge channel", "channel", channelID, "error", err)
	}
}

// readThreads handles the operator's replies in the threads of the last 24 hours that have a reply past their cursor.
// It reports whether any reply was handled.
// Every other reply is skipped, and a thread's cursor moves past each reply read.
func (r *bridgeReader) readThreads(ctx context.Context, channelID string) (bool, error) {
	entry, open, err := r.prunedEntry(channelID)
	if err != nil || !open {
		return false, err
	}
	oldest := slackTimestamp(r.clock.Now().Add(-threadRetention))
	messages, err := r.slack.History(ctx, channelID, oldest)
	if err != nil {
		return false, fmt.Errorf("slack: read the threads of the bridge channel %s: %w", entry.Name, err)
	}
	cursors := ChannelEntry{Threads: entry.Threads, ThreadFloor: entry.ThreadFloor}
	handled := false
	for _, message := range messages {
		if message.LatestReply == "" || compareTS(message.LatestReply, cursors.ThreadStart(message.TS)) <= 0 {
			continue
		}
		threadHandled, err := r.readThread(ctx, channelID, entry, message.TS)
		handled = handled || threadHandled
		if err != nil {
			return handled, err
		}
		if entry, open, err = r.openEntry(channelID); err != nil || !open {
			return handled, err
		}
		cursors = ChannelEntry{Threads: entry.Threads, ThreadFloor: entry.ThreadFloor}
	}
	return handled, nil
}

// prunedEntry drops the channel's thread cursors older than 24 hours and returns its entry.
// open is false when the map holds no open entry for the channel.
func (r *bridgeReader) prunedEntry(channelID string) (BridgeEntry, bool, error) {
	var pruned BridgeEntry
	open := false
	err := UpdateBridgeMap(r.geom.BridgeMapPath, func(bridgeMap *BridgeMap) error {
		entry, held := bridgeMap.Channels[channelID]
		if !held || entry.Archived {
			return nil
		}
		cursors := ChannelEntry{Threads: entry.Threads, ThreadFloor: entry.ThreadFloor}
		cursors.PruneThreads(r.clock.Now())
		entry.Threads, entry.ThreadFloor = cursors.Threads, cursors.ThreadFloor
		bridgeMap.Channels[channelID] = entry
		pruned, open = entry, true
		return nil
	})
	return pruned, open, err
}

// readThread handles the replies of the thread after its cursor, oldest first, and reports whether any was handled.
// It skips the thread's own first message and every `thread_broadcast`, which the channel's read handles.
func (r *bridgeReader) readThread(ctx context.Context, channelID string, entry BridgeEntry, thread string) (bool, error) {
	cursors := ChannelEntry{Threads: entry.Threads, ThreadFloor: entry.ThreadFloor}
	replies, err := r.slack.Replies(ctx, channelID, thread, cursors.ThreadStart(thread))
	if err != nil {
		return false, fmt.Errorf("slack: read a thread of the bridge channel %s: %w", entry.Name, err)
	}
	handled := false
	for _, reply := range replies {
		if reply.TS == thread {
			continue
		}
		if reply.Subtype != threadBroadcastSubtype && r.isOperator(reply) {
			if err := r.operator(ctx, channelID, thread, entry, reply); err != nil && !r.givenUp(ctx, channelID, thread, reply, err) {
				return handled, err
			}
			r.failingKey = ""
			handled = true
		}
		err := UpdateBridgeMap(r.geom.BridgeMapPath, func(bridgeMap *BridgeMap) error {
			current, held := bridgeMap.Channels[channelID]
			if !held {
				return nil
			}
			moved := ChannelEntry{Threads: current.Threads}
			moved.AdvanceThread(thread, reply.TS)
			current.Threads = moved.Threads
			bridgeMap.Channels[channelID] = current
			return nil
		})
		if err != nil {
			return handled, err
		}
	}
	return handled, nil
}

// resync finds the bridge channels the bridge map lacks and adds or reports them.
// A channel is a candidate when the bot is a member, its name has a bridge channel's shape,
// and no orch channel map of a hub of this machine holds it.
func (r *bridgeReader) resync(ctx context.Context) error {
	channels, err := r.slack.MemberChannels(ctx)
	if err != nil {
		return fmt.Errorf("slack: list the bot's channels to find bridge channels: %w", err)
	}
	var candidates []ChannelInfo
	for _, info := range channels {
		if !info.IsArchived && isBridgeChannelName(info.Name) {
			candidates = append(candidates, info)
		}
	}
	if len(candidates) == 0 {
		return nil
	}
	bridgeMap, err := ReadBridgeMap(r.geom.BridgeMapPath)
	if err != nil {
		return err
	}
	orchChannels, err := r.orchChannelIDs()
	if err != nil {
		return err
	}
	bot, err := r.keeper.botUser(ctx)
	if err != nil {
		return err
	}
	var failures []error
	for _, info := range candidates {
		if _, held := bridgeMap.Channels[info.ID]; held || orchChannels[info.ID] {
			continue
		}
		if err := r.resyncChannel(ctx, info, bot); err != nil {
			failures = append(failures, err)
			if _, limited := isRateLimit(err); limited {
				break
			}
		}
	}
	return errors.Join(failures...)
}

// orchChannelIDs returns the ids of every channel an orch channel map of a hub of this machine holds.
// A hub whose map cannot be read fails the read, since a channel it holds would be taken for a bridge channel.
func (r *bridgeReader) orchChannelIDs() (map[string]bool, error) {
	live, err := r.liveHubs()
	if err != nil {
		return nil, fmt.Errorf("slack: read the live hubs to find bridge channels: %w", err)
	}
	ids := map[string]bool{}
	for _, geom := range append([]Geometry{r.geom}, live...) {
		channelMap, err := ReadChannelMap(geom.ChannelMapPath)
		if err != nil {
			return nil, err
		}
		for _, entry := range channelMap.Channels {
			ids[entry.ID] = true
		}
	}
	return ids, nil
}

// resyncChannel judges one candidate channel.
// A channel made by this bot or a pinned peer's bot is added when its newest authored envelope names an orch of this hub.
// An envelope is authored when this bot, or the app pinned for its `from` machine, posted it.
// A channel made by an app no pin names is reported when its newest envelope, whoever posted it, names this hub.
// Every other channel is left alone.
func (r *bridgeReader) resyncChannel(ctx context.Context, info ChannelInfo, bot string) error {
	peers, err := ReadPeers(r.geom.PeersPath)
	if err != nil {
		return err
	}
	creatorTrusted := info.Creator == bot
	for _, pin := range peers.Machines {
		creatorTrusted = creatorTrusted || pin.BotUserID == info.Creator
	}
	messages, err := r.slack.History(ctx, info.ID, resyncHistoryOldest)
	if err != nil {
		return fmt.Errorf("slack: read the channel %s to judge it as a bridge channel: %w", info.Name, err)
	}
	var newest, newestAuthored *BridgeEnvelope
	for _, message := range messages {
		envelope, _, ok := ParseBridgeMessage(message.Text)
		if !ok {
			continue
		}
		newest = &envelope
		authored, err := r.authored(ctx, message, envelope, bot, peers)
		if err != nil {
			return err
		}
		if authored {
			newestAuthored = &envelope
		}
	}
	if creatorTrusted {
		if newestAuthored == nil {
			return nil
		}
		return r.adoptByEnvelope(ctx, info, *newestAuthored)
	}
	if newest == nil || (newest.From.Name.Shortname != r.geom.Shortname && newest.To.Name.Shortname != r.geom.Shortname) {
		return nil
	}
	appID, err := r.slack.UserAppID(ctx, info.Creator)
	if err != nil {
		return fmt.Errorf("slack: read the app of the creator of %s: %w", info.Name, err)
	}
	if appID == "" {
		return nil
	}
	return r.reportUnapprovedBot(ctx, info, appID, newest.From.Machine)
}

// authored reports whether this bot posted the message, or the app pinned for the envelope's `from` machine did.
func (r *bridgeReader) authored(ctx context.Context, message Message, envelope BridgeEnvelope, bot string, peers Peers) (bool, error) {
	if message.User == bot {
		return true, nil
	}
	pin, pinned := peers.Machines[envelope.From.Machine]
	if message.BotID == "" || !pinned {
		return false, nil
	}
	appID, err := r.appOf(ctx, message.BotID)
	return err == nil && appID == pin.AppID, err
}

// adoptByEnvelope adds the channel to the bridge map for the orch of this hub the envelope names, or leaves it unmapped.
// The orch is the envelope's end with this hub's shortname that names this machine, by its current or a former name.
// A live orch's entry starts its delivery cursor at the channel's start, so messages within retention are delivered once,
// and its operator cursor and thread floor at the channel's newest message, so no operator text already there is handled.
// A retired orch leaves the channel unmapped, with one notice in it.
func (r *bridgeReader) adoptByEnvelope(ctx context.Context, info ChannelInfo, envelope BridgeEnvelope) error {
	names, err := r.machineNames()
	if err != nil {
		return err
	}
	isLocal := func(address agentname.MachineAddress) bool {
		return holdsName(names, address.Machine) && address.Name.Shortname == r.geom.Shortname
	}
	var local, peer agentname.MachineAddress
	switch {
	case isLocal(envelope.From) && !isLocal(envelope.To):
		local, peer = envelope.From, envelope.To
	case isLocal(envelope.To) && !isLocal(envelope.From):
		local, peer = envelope.To, envelope.From
	default:
		return nil
	}
	localOrch := local.Name.String()
	live, err := r.orchLive(localOrch)
	if err != nil {
		return err
	}
	if !live {
		r.notice(ctx, info.ID, retiredOrchKind, fmt.Sprintf(
			"the orch %s is retired, so this bridge channel is not read", localOrch))
		return nil
	}
	newest, _, err := r.slack.Latest(ctx, info.ID)
	if err != nil {
		return fmt.Errorf("slack: read the newest message of the bridge channel %s: %w", info.Name, err)
	}
	entry := BridgeEntry{Name: info.Name, LocalOrch: localOrch, Peer: formatBridgeAddress(peer), OperatorCursor: newest, ThreadFloor: newest}
	err = UpdateBridgeMap(r.geom.BridgeMapPath, func(bridgeMap *BridgeMap) error {
		if _, held := bridgeMap.Channels[info.ID]; !held {
			bridgeMap.Channels[info.ID] = entry
		}
		return nil
	})
	if err != nil {
		return err
	}
	for _, kind := range []string{retiredOrchKind, unapprovedBotKind} {
		if err := clearBridgeNotice(r.geom.BridgeNoticesPath, receiveNoticeKey(kind, info.ID)); err != nil {
			logger.Warn("slackengine: the bridge notice could not be unmarked", "channel", info.ID, "error", err)
		}
	}
	return nil
}

// reportUnapprovedBot posts, once per channel, the notice about a bridge channel an unapproved app made.
// It goes to the prime orch's channel, or with that channel not open to the open channel of each orch whose button is on.
// It is also mailed to the prime orch.
// A notice that reached nowhere ends its episode, so the next resync tries again.
func (r *bridgeReader) reportUnapprovedBot(ctx context.Context, info ChannelInfo, appID, machine string) error {
	key := receiveNoticeKey(unapprovedBotKind, info.ID)
	marked, err := markBridgeNotice(r.geom.BridgeNoticesPath, key)
	if err != nil || !marked {
		return err
	}
	line := fmt.Sprintf(
		"the channel %s (%s) was made by the app %s, which no approved machine owns, and it carries mail from the machine %s; "+
			"way forward: if that app is the machine %s, type approve %s, or approve %s %s to pin exactly this app, in the orch channel of this hub",
		info.Name, info.ID, appID, machine, machine, machine, machine, appID)
	text := relayNoticePrefix + " " + line

	prime, err := orchName(r.geom.Shortname, "")
	if err != nil {
		return err
	}
	reached := false
	for _, channel := range r.noticeChannels(prime) {
		if err := r.slack.PostAs(ctx, channel, slackEscape.Replace(text), relayProgramSender(r.geom)); err != nil {
			logger.Warn("slackengine: the notice about an unapproved bridge channel was not posted", "channel", channel, "error", err)
			continue
		}
		reached = true
	}
	if _, err := r.mailbox.Post(prime, text); err != nil {
		logger.Warn("slackengine: the notice about an unapproved bridge channel was not mailed", "receiver", prime, "error", err)
	} else {
		reached = true
	}
	if reached {
		return nil
	}
	return clearBridgeNotice(r.geom.BridgeNoticesPath, key)
}

// noticeChannels returns the channels a notice for the hub goes to.
// These are the prime orch's open channel, or with none the open channel of each orch whose button is on.
func (r *bridgeReader) noticeChannels(prime string) []string {
	channelMap, err := ReadChannelMap(r.geom.ChannelMapPath)
	if err != nil {
		return nil
	}
	if entry, held := channelMap.Channels[prime]; held && !entry.Archived {
		return []string{entry.ID}
	}
	forwarded, err := ReadForwarding(r.geom.ForwardingPath)
	if err != nil {
		return nil
	}
	var channels []string
	for _, orch := range forwarded {
		if entry, held := channelMap.Channels[orch]; held && !entry.Archived {
			channels = append(channels, entry.ID)
		}
	}
	return channels
}
