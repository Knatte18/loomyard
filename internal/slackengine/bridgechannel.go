// bridgechannel.go makes and adopts the bridge channel of a pair of orchs, and holds the pair back while it cannot.

package slackengine

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/logger"
)

// operatorInviteKind is the kind of the notice about a refused invite of the operator into a bridge channel.
const operatorInviteKind = "operator-invite"

// errBridgeNotListed is the error of a create that answered name_taken.
// The channel listing does not show the channel.
var errBridgeNotListed = errors.New("the channel list does not show the taken name")

// bridgeHold is the last outcome a pair met when it tried to make its channel, and when.
// An attempt that ended in errBridgeNotListed holds err and no reason.
type bridgeHold struct {
	reason string
	err    error
	at     time.Time
}

// localBridgeAddress returns the address of this machine's orch, the creel name orch, in the bridge's address form.
func localBridgeAddress(machine, orch string) (agentname.MachineAddress, error) {
	name, err := agentname.Parse(orch)
	if err != nil {
		return agentname.MachineAddress{}, fmt.Errorf("slack: address the orch %q on the bridge: %w", orch, err)
	}
	return agentname.MachineAddress{Machine: machine, Name: name}, nil
}

// bridgeNoticeKey returns the key of a send-side notice of kind for the pair of localOrch and the peer's address.
func bridgeNoticeKey(kind, localOrch, peer string) string {
	return kind + ":" + localOrch + " " + peer
}

// ensureBridge makes or adopts the bridge channel of localOrch and peer, and answers its id.
// It answers a hold reason with no id while the pair has no channel:
// `bridge-name-taken` for a channel of that name that neither this machine's bot nor pin's bot made,
// and `bridge-archived` for an archived one.
// A held pair tries again at most once per bridgePollCeiling; between tries it answers its last hold.
// A pair whose try ended in errBridgeNotListed answers that error between tries, calling Slack no more.
// A new channel enters the bridge map with its cursors at the channel's start and the peer's bot not invited.
// An adopted one is joined when the bot is no member.
// Its delivery cursor is at the start, and its operator cursor and thread floor are at its newest message.
// The setup's operator is then invited and recorded; a refusal mails localOrch one notice per episode.
func (k *channelKeeper) ensureBridge(ctx context.Context, localOrch string, peer agentname.MachineAddress, pin Peer) (string, string, error) {
	peerAddress := formatBridgeAddress(peer)
	pair := localOrch + " " + peerAddress
	lock := k.orchLock("bridge " + pair)
	lock.Lock()
	defer lock.Unlock()

	now := k.clock.Now()
	k.mu.Lock()
	last, held := k.bridgeHolds[pair]
	k.mu.Unlock()
	if held && now.Sub(last.at) < bridgePollCeiling {
		return "", last.reason, last.err
	}
	local, err := localBridgeAddress(k.geom.Machine, localOrch)
	if err != nil {
		return "", "", err
	}
	channelID, reason, err := k.makeBridge(ctx, localOrch, BridgeChannelName(local, peer), peerAddress, pin)
	k.mu.Lock()
	defer k.mu.Unlock()
	switch {
	case errors.Is(err, errBridgeNotListed):
		k.bridgeHolds[pair] = bridgeHold{err: err, at: now}
		return "", "", err
	case err != nil:
		return "", "", err
	case reason == "":
		delete(k.bridgeHolds, pair)
	default:
		k.bridgeHolds[pair] = bridgeHold{reason: reason, at: now}
	}
	return channelID, reason, nil
}

// makeBridge creates the channel called name, and adopts the live one a taken name shows.
func (k *channelKeeper) makeBridge(ctx context.Context, localOrch, name, peerAddress string, pin Peer) (string, string, error) {
	info, err := k.slack.CreateChannel(ctx, name)
	if err != nil {
		if refusal, refused := isAPIError(err); refused && refusal.Code == nameTakenCode {
			return k.adoptBridge(ctx, localOrch, name, peerAddress, pin)
		}
		return "", "", fmt.Errorf("slack: create the bridge channel %s: %w", name, err)
	}
	if info.Name == "" {
		info.Name = name
	}
	entry := BridgeEntry{Name: info.Name, LocalOrch: localOrch, Peer: peerAddress}
	if err := k.recordBridge(info.ID, entry); err != nil {
		return "", "", err
	}
	k.inviteBridgeOperator(ctx, info.ID, entry)
	return info.ID, "", nil
}

// adoptBridge takes over the live channel called name, which a create found taken.
// It answers a hold reason for an archived channel and for one neither this machine's bot nor pin's bot made.
func (k *channelKeeper) adoptBridge(ctx context.Context, localOrch, name, peerAddress string, pin Peer) (string, string, error) {
	info, found, err := k.slack.FindChannel(ctx, name)
	if err != nil {
		return "", "", fmt.Errorf("slack: look up the taken bridge channel %s: %w", name, err)
	}
	if !found {
		return "", "", fmt.Errorf("slack: the bridge channel name %s is taken but the channel list does not show it; the next try looks again: %w", name, errBridgeNotListed)
	}
	if info.IsArchived {
		return "", bridgeArchivedReason, nil
	}
	bot, err := k.botUser(ctx)
	if err != nil {
		return "", "", err
	}
	if info.Creator != bot && info.Creator != pin.BotUserID {
		return "", bridgeNameTakenReason, nil
	}
	if !info.IsMember {
		if err := k.slack.Join(ctx, info.ID); err != nil {
			return "", "", fmt.Errorf("slack: join the bridge channel %s: %w", name, err)
		}
	}
	newest, _, err := k.slack.Latest(ctx, info.ID)
	if err != nil {
		return "", "", fmt.Errorf("slack: read the newest message of the bridge channel %s: %w", name, err)
	}
	entry := BridgeEntry{Name: info.Name, LocalOrch: localOrch, Peer: peerAddress, OperatorCursor: newest, ThreadFloor: newest}
	if err := k.recordBridge(info.ID, entry); err != nil {
		return "", "", err
	}
	k.inviteBridgeOperator(ctx, info.ID, entry)
	return info.ID, "", nil
}

// recordBridge stores entry as the bridge channel channelID in the bridge map.
func (k *channelKeeper) recordBridge(channelID string, entry BridgeEntry) error {
	return UpdateBridgeMap(k.geom.BridgeMapPath, func(bridgeMap *BridgeMap) error {
		bridgeMap.Channels[channelID] = entry
		return nil
	})
}

// inviteBridgeOperator invites the setup's operator into the new bridge channel and records the operator in its entry.
// A failure holds nothing: the channel stands, and a later wake retries the invite.
func (k *channelKeeper) inviteBridgeOperator(ctx context.Context, channelID string, entry BridgeEntry) {
	setup, found, err := ReadSetup(k.geom.SetupPath)
	if err != nil || !found || setup.OperatorID == "" {
		logger.Debug("slackengine: no operator to invite into a new bridge channel", "channel", channelID, "error", err)
		return
	}
	if err := k.inviteBridgeOperatorInto(ctx, channelID, entry, setup.OperatorID); err != nil {
		logger.Debug("slackengine: could not invite the operator into a new bridge channel", "channel", channelID, "error", err)
	}
}

// inviteBridgeOperatorInto invites the Slack user operatorID into the bridge channel and records the user in its entry.
// A refusal Slack gives mails the entry's local orch one notice per episode, naming the code and the way forward.
// The episode ends once an invite stands.
func (k *channelKeeper) inviteBridgeOperatorInto(ctx context.Context, channelID string, entry BridgeEntry, operatorID string) error {
	noticeKey := bridgeNoticeKey(operatorInviteKind, entry.LocalOrch, entry.Peer)
	if err := k.slack.Invite(ctx, channelID, operatorID); err != nil {
		if refusal, refused := isAPIError(err); refused {
			k.mailBridgeNotice(entry.LocalOrch, noticeKey, fmt.Sprintf(
				"Slack refused inviting the operator %s into %s with %s; way forward: run `lym slack setup --operator <user-id>` from the operator's terminal with a user id Slack accepts",
				operatorID, entry.Name, refusal.Code))
		}
		return fmt.Errorf("slack: invite the operator into %s: %w", entry.Name, err)
	}
	if err := clearBridgeNotice(k.geom.BridgeNoticesPath, noticeKey); err != nil {
		logger.Warn("slackengine: the bridge notice could not be unmarked", "key", noticeKey, "error", err)
	}
	return UpdateBridgeMap(k.geom.BridgeMapPath, func(bridgeMap *BridgeMap) error {
		if current, held := bridgeMap.Channels[channelID]; held {
			current.Operator = operatorID
			bridgeMap.Channels[channelID] = current
		}
		return nil
	})
}

// mailBridgeNotice mails orch one `[slack relay]` notice with line for the episode key.
// The bridge-notice record remembers the key, so a restarted relay mails no second notice.
// A notice that cannot be mailed ends its episode, so the next try mails it.
func (k *channelKeeper) mailBridgeNotice(orch, key, line string) {
	marked, err := markBridgeNotice(k.geom.BridgeNoticesPath, key)
	if err != nil {
		logger.Warn("slackengine: the bridge-notice record could not be written", "key", key, "error", err)
		return
	}
	if !marked {
		return
	}
	if _, err := k.mailbox.Post(orch, relayNoticePrefix+" "+line); err != nil {
		logger.Warn("slackengine: the relay notice for a bridge channel was not posted", "receiver", orch, "error", err)
		if clearErr := clearBridgeNotice(k.geom.BridgeNoticesPath, key); clearErr != nil {
			logger.Warn("slackengine: the bridge notice could not be unmarked", "key", key, "error", clearErr)
		}
	}
}
