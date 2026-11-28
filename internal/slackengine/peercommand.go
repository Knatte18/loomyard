// peercommand.go parses and runs the operator's `approve` and `revoke` commands.
// They pin and unpin a peer machine's Slack app.

package slackengine

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/machinename"
)

const (
	// approveVerb and revokeVerb are the two words of a peer command.
	approveVerb = "approve"
	revokeVerb  = "revoke"
	// peerBotNamePrefix precedes the machine name in the name of a machine's bot, `lym-<machine>`.
	peerBotNamePrefix = "lym-"
)

// peerCommand is the operator's `approve` or `revoke` command.
type peerCommand struct {
	// Verb is `approve` or `revoke`.
	Verb string
	// Machine is the machine named, lower-cased.
	Machine string
	// AppID is the Slack app id an `approve` named, as written; empty when the command named none.
	AppID string
}

// parsePeerCommand reads text as `approve <machine>`, `approve <machine> <app id>` or `revoke <machine>`.
// The verb matches case-insensitively and the machine is lower-cased; the app id is kept as written.
// It reports false for any other text.
func parsePeerCommand(text string) (peerCommand, bool) {
	fields := strings.Fields(text)
	if len(fields) < 2 {
		return peerCommand{}, false
	}
	verb, machine := strings.ToLower(fields[0]), strings.ToLower(fields[1])
	switch {
	case verb == approveVerb && len(fields) == 2:
		return peerCommand{Verb: verb, Machine: machine}, true
	case verb == approveVerb && len(fields) == 3:
		return peerCommand{Verb: verb, Machine: machine, AppID: fields[2]}, true
	case verb == revokeVerb && len(fields) == 2:
		return peerCommand{Verb: verb, Machine: machine}, true
	}
	return peerCommand{}, false
}

// handlePeerCommand answers a peer command written in an orch's channel.
// A command that names this machine, by its current or a former name, is refused in the channel.
// A command that cannot be carried out for now is retried as any message is.
func (in *inbound) handlePeerCommand(ctx context.Context, channel string, message Message, command peerCommand) (bool, error) {
	current, former, err := in.hub.MachineNames()
	if err != nil {
		return in.retryOrGiveUp(ctx, channel, message, "this machine's names could not be read", err)
	}
	named := slices.ContainsFunc(append([]string{current}, former...), func(name string) bool {
		return strings.EqualFold(name, command.Machine)
	})
	if named {
		in.settle()
		in.answer(ctx, channel, fmt.Sprintf("approve and revoke name another machine; type approve %s in a channel of that other machine", current))
		return true, nil
	}
	line, err := in.runPeerCommand(ctx, command)
	if err != nil {
		return in.retryOrGiveUp(ctx, channel, message, "the "+command.Verb+" command could not be carried out", err)
	}
	in.settle()
	in.answer(ctx, channel, line)
	return true, nil
}

// runPeerCommand carries out command and returns the line to post in the channel it was written in.
// A refusal of the command is that line; a Slack refusal or a record that fails to write is the error.
// It reads only the fields of inbound set when it is built, its Slack client, geometry and clock, and writes none.
func (in *inbound) runPeerCommand(ctx context.Context, command peerCommand) (string, error) {
	if err := machinename.CheckName(command.Machine); err != nil {
		return err.Error(), nil
	}
	switch {
	case command.Verb == revokeVerb:
		return in.revokePeer(command.Machine)
	case command.AppID != "":
		return in.approvePeerByAppID(ctx, command.Machine, command.AppID)
	default:
		return in.approvePeerByName(ctx, command.Machine)
	}
}

// approvePeerByName pins the one bot named `lym-<machine>`.
// It refuses with the candidates' app ids when there is not exactly one.
func (in *inbound) approvePeerByName(ctx context.Context, machine string) (string, error) {
	bots, err := in.slack.BotUsersNamed(ctx, peerBotNamePrefix+machine)
	if err != nil {
		return "", err
	}
	switch len(bots) {
	case 1:
		return in.pinPeer(machine, bots[0])
	case 0:
		return fmt.Sprintf("found no bot named %s%s; type approve %s <app id> with the app id from `status` typed in a channel of that machine", peerBotNamePrefix, machine, machine), nil
	}
	appIDs := make([]string, 0, len(bots))
	for _, bot := range bots {
		appIDs = append(appIDs, bot.AppID)
	}
	return fmt.Sprintf("found %d bots named %s%s, with app ids %s; type approve %s <app id> to pick one", len(bots), peerBotNamePrefix, machine, strings.Join(appIDs, ", "), machine), nil
}

// approvePeerByAppID pins the bot of the workspace whose app id equals appID exactly, whatever its name.
func (in *inbound) approvePeerByAppID(ctx context.Context, machine, appID string) (string, error) {
	bots, err := in.slack.BotUsersNamed(ctx, "")
	if err != nil {
		return "", err
	}
	for _, bot := range bots {
		if bot.AppID == appID {
			return in.pinPeer(machine, bot)
		}
	}
	return fmt.Sprintf("found no bot of the app id %s; read the app id from `status` typed in a channel of the other machine", appID), nil
}

// pinPeer records bot as the approved app of machine and returns the line that names its app id.
func (in *inbound) pinPeer(machine string, bot BotUser) (string, error) {
	approvedAt := in.clock.Now()
	err := UpdatePeers(in.geom.PeersPath, func(peers *Peers) error {
		peers.Machines[machine] = Peer{AppID: bot.AppID, BotUserID: bot.UserID, ApprovedAt: approvedAt}
		return nil
	})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("approved %s: app id %s", machine, bot.AppID), nil
}

// revokePeer removes the approval of machine, and writes nothing when the machine has none.
func (in *inbound) revokePeer(machine string) (string, error) {
	peers, err := ReadPeers(in.geom.PeersPath)
	if err != nil {
		return "", err
	}
	if _, pinned := peers.Machines[machine]; !pinned {
		return fmt.Sprintf("%s has no approval to revoke", machine), nil
	}
	err = UpdatePeers(in.geom.PeersPath, func(peers *Peers) error {
		delete(peers.Machines, machine)
		return nil
	})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("revoked %s", machine), nil
}
