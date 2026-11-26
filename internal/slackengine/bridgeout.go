// bridgeout.go is the relay's sending of machine mail.
// It posts each message creel holds for another machine's orch to the bridge channel of its pair.
// It holds what cannot be posted yet, and settles each message with creel.

package slackengine

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/creelengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/machinename"
)

const (
	// peerUnapprovedReason is the hold reason for mail to a machine the operator has not approved.
	peerUnapprovedReason = "peer-unapproved"
	// buttonOffReason is the reason reported for mail of an orch whose slack button is off.
	buttonOffReason = "button-off"
	// bridgeNameTakenReason is the hold reason for a bridge channel name that someone else's channel holds.
	bridgeNameTakenReason = "bridge-name-taken"
	// bridgeArchivedReason is the hold reason for a bridge channel found archived.
	bridgeArchivedReason = "bridge-archived"
	// peerNotInvitedKind is the kind of the notice about a refused invite of the peer's bot into a bridge channel.
	// The hold itself carries Slack's code as its reason.
	peerNotInvitedKind = "peer-not-invited"
)

// sendHoldKinds are the kinds of the send-side notices whose episode ends with the hold.
var sendHoldKinds = []string{peerUnapprovedReason, bridgeNameTakenReason, bridgeArchivedReason, peerNotInvitedKind}

// bridgeSender posts the machine mail creel holds to the bridge channels.
// One goroutine runs it.
type bridgeSender struct {
	geom    Geometry
	mailbox Mailbox
	slack   SlackClient
	keeper  *channelKeeper
	// held holds the notice keys of the holds the current send met.
	held map[string]bool
}

// newBridgeSender returns a sender over the seams.
func newBridgeSender(geom Geometry, mailbox Mailbox, slack SlackClient, _ Clock, keeper *channelKeeper) *bridgeSender {
	return &bridgeSender{geom: geom, mailbox: mailbox, slack: slack, keeper: keeper, held: map[string]bool{}}
}

// relayProgramSender is the sender of the relay's own lines: its creel program name on the machine, and the program icon.
func relayProgramSender(geom Geometry) Sender {
	name := agentname.FormatMachineAddress(geom.Machine, geom.Shortname+":"+SlackProgram)
	return Sender{Username: name, IconEmoji: programIcon}
}

// settleAccepted settles delivered every message the post record holds as accepted by Slack.
// An entry goes once creel answers, a refusal included, since creel refuses an id it no longer holds.
// An entry stays on a lost connection, for the next try.
func (s *bridgeSender) settleAccepted() error {
	posts, err := readBridgePosts(s.geom.BridgePostsPath)
	if err != nil {
		return err
	}
	ids := make([]int64, 0, len(posts.Accepted))
	for id := range posts.Accepted {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	var failures []error
	for _, id := range ids {
		if err := s.settleDelivered(&posts, id, posts.Accepted[id]); err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

// settleDelivered settles message id delivered with Slack's timestamp ts, and drops it from the accepted posts.
// It keeps the entry and returns the error when creel did not answer.
func (s *bridgeSender) settleDelivered(posts *bridgePosts, id int64, ts string) error {
	err := s.mailbox.SettleMachineDelivered(id, ts)
	if err != nil && !creelengine.IsDaemonRefusal(err) {
		return fmt.Errorf("slack: settle message %d delivered: %w", id, err)
	}
	delete(posts.Accepted, id)
	return writeBridgePosts(s.geom.BridgePostsPath, *posts)
}

// send settles the accepted posts of the record, takes the machine mail creel holds and posts each message, oldest first.
// It reports whether it posted any.
// A message the post record still holds as accepted, because its settle failed, is never posted again.
// The next send's settle takes it up.
// A held message creel returned at its hour is no longer taken, so it is never posted.
// The refusal counts of messages creel no longer holds are dropped.
// So is the notice mark of each hold no taken message meets any more.
// A rate limit ends the send; any other failure of one message leaves the rest to go on.
func (s *bridgeSender) send(ctx context.Context) (bool, error) {
	settleErr := s.settleAccepted()
	messages, err := s.mailbox.TakeMachine()
	if err != nil {
		return false, errors.Join(settleErr, fmt.Errorf("slack: take the machine mail: %w", err))
	}
	posts, err := readBridgePosts(s.geom.BridgePostsPath)
	if err != nil {
		return false, errors.Join(settleErr, err)
	}
	clear(s.held)
	var failures []error
	if settleErr != nil {
		failures = append(failures, settleErr)
	}
	failures = append(failures, s.dropStaleRefusals(&posts, messages)...)
	posted := false
	for _, message := range messages {
		if _, accepted := posts.Accepted[message.ID]; accepted {
			continue
		}
		sent, err := s.sendOne(ctx, message, &posts)
		posted = posted || sent
		if err == nil {
			continue
		}
		failures = append(failures, err)
		if _, limited := isRateLimit(err); limited {
			break
		}
	}
	if len(failures) == 0 {
		failures = append(failures, s.clearEndedNotices())
	}
	return posted, errors.Join(failures...)
}

// dropStaleRefusals drops the refusal counts of ids the messages do not hold, and returns the failure of saving that.
func (s *bridgeSender) dropStaleRefusals(posts *bridgePosts, messages []creelengine.MachineOutbound) []error {
	taken := map[int64]bool{}
	for _, message := range messages {
		taken[message.ID] = true
	}
	changed := false
	for id := range posts.Refusals {
		if !taken[id] {
			delete(posts.Refusals, id)
			changed = true
		}
	}
	if !changed {
		return nil
	}
	if err := writeBridgePosts(s.geom.BridgePostsPath, *posts); err != nil {
		return []error{err}
	}
	return nil
}

// clearEndedNotices clears the mark of each send-side hold notice that no taken message met in this send.
func (s *bridgeSender) clearEndedNotices() error {
	notices, err := readBridgeNotices(s.geom.BridgeNoticesPath)
	if err != nil {
		return err
	}
	keys := make([]string, 0, len(notices.Marks))
	for key := range notices.Marks {
		kind, _, _ := strings.Cut(key, ":")
		if slices.Contains(sendHoldKinds, kind) && !s.held[key] {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	var failures []error
	for _, key := range keys {
		if err := clearBridgeNotice(s.geom.BridgeNoticesPath, key); err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

// sendOne posts one message from the local orch to its peer, or reports or holds it, and reports whether it posted.
// The first of these decides:
// the orch's slack button is off, which reports `button-off`;
// the peer's machine has no approval, which holds `peer-unapproved`;
// the pair has no open bridge channel and ensureBridge holds the pair back;
// the peer's bot is not invited and Slack refuses the invite, which holds with Slack's code.
// A post Slack accepted is written to the post record before creel is told, so a restart never posts it again.
// A post Slack refuses with a code counts toward refusalCap, where the message is settled returned with the code.
// A transport failure or a rate limit counts toward nothing and is returned.
func (s *bridgeSender) sendOne(ctx context.Context, message creelengine.MachineOutbound, posts *bridgePosts) (bool, error) {
	peer, err := agentname.ParseMachineAddress(message.To, nil, machinename.CheckName)
	if err != nil {
		return false, fmt.Errorf("slack: the machine mail %d is addressed to no orch of another machine: %w", message.ID, err)
	}
	local, err := localBridgeAddress(s.geom.Machine, message.From)
	if err != nil {
		return false, err
	}
	peerAddress := formatBridgeAddress(peer)

	on, err := ForwardingOn(s.geom.ForwardingPath, message.From)
	if err != nil {
		return false, err
	}
	if !on {
		return false, s.report(message.ID, buttonOffReason)
	}
	peers, err := ReadPeers(s.geom.PeersPath)
	if err != nil {
		return false, err
	}
	pin, approved := peers.Machines[peer.Machine]
	if !approved {
		line := fmt.Sprintf("mail to %s waits because the machine %s is not approved; way forward: type lym approve %s in a channel of an orch of this machine",
			peerAddress, peer.Machine, peer.Machine)
		return false, s.hold(ctx, message, peerUnapprovedReason, bridgeNoticeKey(peerUnapprovedReason, message.From, peerAddress), line)
	}

	channelID, held, err := s.channelOf(ctx, message, local, peer, pin)
	if err != nil || held {
		return false, err
	}
	bridgeMap, err := ReadBridgeMap(s.geom.BridgeMapPath)
	if err != nil {
		return false, err
	}
	if !bridgeMap.Channels[channelID].PeerInvited {
		invited, err := s.invitePeerBot(ctx, message, channelID, peer, pin)
		if err != nil || !invited {
			return false, err
		}
	}

	envelope := BridgeEnvelope{From: local, To: peer, ID: message.ID, ReplyTo: message.ReplyTo}
	text := FormatBridgeMessage(envelope, slackEscape.Replace(message.Body))
	timestamp, err := s.slack.PostTimestamped(ctx, channelID, text, Sender{Username: formatBridgeAddress(local), IconEmoji: orchIcon})
	if err != nil {
		return false, s.postRefused(message, channelID, posts, err)
	}
	if posts.Accepted == nil {
		posts.Accepted = map[int64]string{}
	}
	posts.Accepted[message.ID] = timestamp
	delete(posts.Refusals, message.ID)
	saveErr := writeBridgePosts(s.geom.BridgePostsPath, *posts)
	return true, errors.Join(saveErr, s.settleDelivered(posts, message.ID, timestamp))
}

// channelOf returns the id of the open bridge channel of the message's pair.
// It makes or adopts the channel when the map holds none.
// It reports held true, with the hold made, when the pair cannot have a channel yet.
func (s *bridgeSender) channelOf(ctx context.Context, message creelengine.MachineOutbound, local, peer agentname.MachineAddress, pin Peer) (string, bool, error) {
	peerAddress := formatBridgeAddress(peer)
	bridgeMap, err := ReadBridgeMap(s.geom.BridgeMapPath)
	if err != nil {
		return "", false, err
	}
	if channelID, found := bridgeMap.ChannelOf(message.From, peerAddress); found {
		return channelID, false, nil
	}
	channelID, reason, err := s.keeper.ensureBridge(ctx, message.From, peer, pin)
	if err != nil {
		return "", false, err
	}
	if reason == "" {
		return channelID, false, nil
	}
	name := BridgeChannelName(local, peer)
	return "", true, s.hold(ctx, message, reason, bridgeNoticeKey(reason, message.From, peerAddress), bridgeHoldLine(reason, name))
}

// bridgeHoldLine is the notice of a hold on a pair's channel name: what holds it and the way forward.
func bridgeHoldLine(reason, name string) string {
	if reason == bridgeArchivedReason {
		return fmt.Sprintf("the bridge channel %s is archived and the relay never unarchives it; "+
			"way forward: unarchive it in Slack, and the next try adopts it when this machine's bot or the approved peer's bot made it; "+
			"for a channel anyone else made, also rename it to free the name, then archive it again if wanted", name)
	}
	return fmt.Sprintf("the channel %s was made by someone other than this machine's bot or the approved peer's bot; "+
		"way forward: rename that channel in Slack, which frees the name, and the next try creates the bridge channel", name)
}

// invitePeerBot invites the peer's bot into the channel and records the invite in the bridge map.
// A refusal Slack gives holds the message with Slack's code as its reason, and invited is false.
func (s *bridgeSender) invitePeerBot(ctx context.Context, message creelengine.MachineOutbound, channelID string, peer agentname.MachineAddress, pin Peer) (bool, error) {
	if err := s.slack.Invite(ctx, channelID, pin.BotUserID); err != nil {
		refusal, refused := isAPIError(err)
		if !refused {
			return false, err
		}
		line := fmt.Sprintf("Slack refused inviting the bot of %s into %s with %s; way forward: invite @%s%s into that channel by hand in Slack",
			peer.Machine, channelID, refusal.Code, peerBotNamePrefix, peer.Machine)
		return false, s.hold(ctx, message, refusal.Code, bridgeNoticeKey(peerNotInvitedKind, message.From, formatBridgeAddress(peer)), line)
	}
	err := UpdateBridgeMap(s.geom.BridgeMapPath, func(bridgeMap *BridgeMap) error {
		if entry, held := bridgeMap.Channels[channelID]; held {
			entry.PeerInvited = true
			bridgeMap.Channels[channelID] = entry
		}
		return nil
	})
	return err == nil, err
}

// postRefused handles the error of a post to a bridge channel and returns the error the send reports.
// A post Slack refused with a code counts in the post record.
// `is_archived` and `channel_not_found` also drop the channel from the bridge map.
// The next send then makes or finds it again.
// At refusalCap the message is settled returned with the code.
// Any other error counts toward nothing.
func (s *bridgeSender) postRefused(message creelengine.MachineOutbound, channelID string, posts *bridgePosts, postErr error) error {
	refusal, refused := isAPIError(postErr)
	if !refused {
		return postErr
	}
	var failures []error
	if refusal.Code == isArchivedCode || refusal.Code == channelNotFoundCode {
		err := UpdateBridgeMap(s.geom.BridgeMapPath, func(bridgeMap *BridgeMap) error {
			delete(bridgeMap.Channels, channelID)
			return nil
		})
		if err != nil {
			failures = append(failures, err)
		}
	}
	if posts.Refusals == nil {
		posts.Refusals = map[int64]int{}
	}
	posts.Refusals[message.ID]++
	if err := writeBridgePosts(s.geom.BridgePostsPath, *posts); err != nil {
		failures = append(failures, err)
	}
	if posts.Refusals[message.ID] < refusalCap {
		return errors.Join(append(failures, postErr)...)
	}
	err := s.mailbox.SettleMachineReturned(message.ID, refusal.Code)
	if err != nil && !creelengine.IsDaemonRefusal(err) {
		failures = append(failures, fmt.Errorf("slack: settle message %d returned: %w", message.ID, err))
	}
	return errors.Join(failures...)
}

// report tells creel the relay holds message id for reason.
// A report creel refuses, for an id it no longer holds, is logged and skipped; the next take decides.
func (s *bridgeSender) report(id int64, reason string) error {
	err := s.mailbox.ReportMachine(id, reason)
	if creelengine.IsDaemonRefusal(err) {
		logger.Debug("slackengine: creel refused the report of a held machine message", "id", id, "reason", reason, "error", err)
		return nil
	}
	if err != nil {
		return fmt.Errorf("slack: report message %d held for %s: %w", id, reason, err)
	}
	return nil
}

// hold reports the message held for reason and, once per episode of noticeKey, tells the local orch why.
// The notice is posted in the orch's channel and mailed to the orch as a `[slack relay]` notice.
// A hold counts toward no refusal cap: it lasts until it clears or creel returns the message at its hour.
func (s *bridgeSender) hold(ctx context.Context, message creelengine.MachineOutbound, reason, noticeKey, line string) error {
	s.held[noticeKey] = true
	if err := s.report(message.ID, reason); err != nil {
		return err
	}
	marked, err := markBridgeNotice(s.geom.BridgeNoticesPath, noticeKey)
	if err != nil || !marked {
		return err
	}
	text := relayNoticePrefix + " " + line
	entry, postErr := s.keeper.ensure(ctx, message.From)
	if postErr == nil {
		postErr = s.slack.PostAs(ctx, entry.ID, slackEscape.Replace(text), relayProgramSender(s.geom))
	}
	_, mailErr := s.mailbox.Post(message.From, text)
	if postErr != nil {
		logger.Warn("slackengine: a bridge notice was not posted in the orch's channel", "orch", message.From, "error", postErr)
	}
	if mailErr != nil {
		logger.Warn("slackengine: a bridge notice was not mailed to the orch", "orch", message.From, "error", mailErr)
	}
	if postErr != nil && mailErr != nil {
		return clearBridgeNotice(s.geom.BridgeNoticesPath, noticeKey)
	}
	return nil
}
