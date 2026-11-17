// bridgemessage.go formats and parses the envelope of a bridge message and names the bridge channel of a pair of orchs.

package slackengine

import (
	"strconv"
	"strings"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/creelengine"
	"github.com/Knatte18/loomyard/internal/machinename"
)

const (
	// bridgeEnvelopePrefix opens the first line of every bridge message.
	bridgeEnvelopePrefix = creelengine.MachineEnvelopeLead
	// bridgeFromKey, bridgeToKey, bridgeIDKey and bridgeReplyToKey open the fields of the envelope line, in that order.
	bridgeFromKey    = "from="
	bridgeToKey      = "to="
	bridgeIDKey      = "id="
	bridgeReplyToKey = "reply-to="
	// bridgeChannelPrefix opens the name of every bridge channel.
	bridgeChannelPrefix = "bridge-"
	// bridgeEndSeparator joins the two ends of a bridge channel's name.
	bridgeEndSeparator = "--"
)

// BridgeEnvelope is the first line of a bridge message: who wrote it, to whom, and under which ids.
type BridgeEnvelope struct {
	// From is the orch that wrote the message and To the orch it is for.
	From, To agentname.MachineAddress
	// ID is the id the sending machine gave the message.
	ID int64
	// ReplyTo is the id the receiving machine gave the message this one answers, 0 for mail.
	ReplyTo int64
}

// FormatBridgeMessage returns the text of a bridge message: the envelope line, a newline and body as given.
// The line reads `[lym-bridge] from=<machine>::<name> to=<machine>::<name> id=<id>`.
// When ReplyTo is set, ` reply-to=<id>` follows the id.
// The caller escapes body for Slack.
func FormatBridgeMessage(envelope BridgeEnvelope, body string) string {
	line := strings.Join([]string{
		bridgeEnvelopePrefix,
		bridgeFromKey + formatBridgeAddress(envelope.From),
		bridgeToKey + formatBridgeAddress(envelope.To),
		bridgeIDKey + strconv.FormatInt(envelope.ID, 10),
	}, " ")
	if envelope.ReplyTo != 0 {
		line += " " + bridgeReplyToKey + strconv.FormatInt(envelope.ReplyTo, 10)
	}
	return line + "\n" + body
}

// formatBridgeAddress returns address in the `<machine>::<name>` form.
func formatBridgeAddress(address agentname.MachineAddress) string {
	return agentname.FormatMachineAddress(address.Machine, address.Name.String())
}

// ParseBridgeMessage reads text, a message as Slack delivers it.
// It answers the envelope, the body after the first line and true.
// It answers false for a first line that is not exactly the envelope FormatBridgeMessage writes:
// the prefix, then `from=`, `to=` and `id=` in that order, and an optional `reply-to=` last.
// Each id is a positive integer, and each address parses as a cross-machine address under the machine name rule.
// It judges no machine as this one's.
func ParseBridgeMessage(text string) (BridgeEnvelope, string, bool) {
	line, body, _ := strings.Cut(text, "\n")
	fields := strings.Split(line, " ")
	if len(fields) < 4 || len(fields) > 5 || fields[0] != bridgeEnvelopePrefix {
		return BridgeEnvelope{}, "", false
	}
	from, fromOK := parseBridgeAddress(fields[1], bridgeFromKey)
	to, toOK := parseBridgeAddress(fields[2], bridgeToKey)
	id, idOK := parseBridgeID(fields[3], bridgeIDKey)
	if !fromOK || !toOK || !idOK {
		return BridgeEnvelope{}, "", false
	}
	envelope := BridgeEnvelope{From: from, To: to, ID: id}
	if len(fields) == 5 {
		replyTo, replyToOK := parseBridgeID(fields[4], bridgeReplyToKey)
		if !replyToOK {
			return BridgeEnvelope{}, "", false
		}
		envelope.ReplyTo = replyTo
	}
	return envelope, body, true
}

// parseBridgeAddress reads a `<key><machine>::<name>` field.
func parseBridgeAddress(field, key string) (agentname.MachineAddress, bool) {
	value, found := strings.CutPrefix(field, key)
	if !found {
		return agentname.MachineAddress{}, false
	}
	address, err := agentname.ParseMachineAddress(value, nil, machinename.CheckName)
	return address, err == nil
}

// parseBridgeID reads a `<key><id>` field whose id is a positive integer written without sign or leading zero.
func parseBridgeID(field, key string) (int64, bool) {
	value, found := strings.CutPrefix(field, key)
	if !found {
		return 0, false
	}
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id <= 0 || strconv.FormatInt(id, 10) != value {
		return 0, false
	}
	return id, true
}

// BridgeChannelName returns the name of the bridge channel of the pair of orchs one and other, lower-cased.
// The name reads `bridge-<end>--<end>`, each end being the machine, `-` and the creel name with `:` turned to `-`.
// The two ends are sorted, so both machines compute one name for a pair.
// A name over Slack's length limit is cut and ends in a hash of the whole name.
func BridgeChannelName(one, other agentname.MachineAddress) string {
	ends := []string{bridgeChannelEnd(one), bridgeChannelEnd(other)}
	if ends[1] < ends[0] {
		ends[0], ends[1] = ends[1], ends[0]
	}
	return fitName(bridgeChannelPrefix + ends[0] + bridgeEndSeparator + ends[1])
}

// bridgeChannelEnd returns one end of a bridge channel's name.
func bridgeChannelEnd(address agentname.MachineAddress) string {
	return slackNamePart(address.Machine) + channelNameSeparator + slackNamePart(address.Name.String())
}
