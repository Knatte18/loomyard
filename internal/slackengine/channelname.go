// channelname.go names the Slack channel of each orch, and the dated name a retired channel takes before its archive.

package slackengine

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

const (
	// maxChannelNameLength is the longest channel name Slack accepts.
	maxChannelNameLength = 80
	// channelNameHashLength is how many hex digits of the name's SHA-256 a cut name ends in.
	channelNameHashLength = 6
	// channelNameSeparator joins the parts of a channel name; Slack allows no colon in one.
	channelNameSeparator = "-"
	// archivedDateLayout is the layout of the date a retired channel's name carries.
	archivedDateLayout = "2006-01-02"
)

// ChannelName returns the name of an orch's channel, lower-cased.
// The name reads `<machine>-<shortname>-<project handle>-<project slug>`.
// The project is the one the orch runs: the root project for the hub's own orch, and the beam's project for a beam's.
// Putting the machine first keeps an orch's channel on one machine.
// It also lets Slack's order group a machine's channels, then a hub's.
// A character Slack does not allow, anything but lowercase letters, digits, `-` and `_`, becomes `-`.
// An empty part is left out.
// A name over Slack's length limit is cut and ends in a hash of the whole name.
func ChannelName(machine, shortname, projectHandle, projectSlug string) string {
	var parts []string
	for _, part := range []string{machine, shortname, projectHandle, projectSlug} {
		if part != "" {
			parts = append(parts, slackNamePart(part))
		}
	}
	return fitName(strings.Join(parts, channelNameSeparator))
}

// slackNamePart lower-cases part and turns each character a Slack channel name does not allow into `-`.
func slackNamePart(part string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			return r
		}
		return '-'
	}, strings.ToLower(part))
}

// ArchivedName returns the name a retired channel takes before it is archived.
// It is name with the UTC date of day, and with `-<n>` after the date for n of 2 and on.
// A rename refused as taken can then try the next n.
// The result is cut as ChannelName cuts a name.
func ArchivedName(name string, day time.Time, n int) string {
	dated := name + "-" + day.UTC().Format(archivedDateLayout)
	if n >= 2 {
		dated = fmt.Sprintf("%s-%d", dated, n)
	}
	return fitName(dated)
}

// fitName returns name unchanged when it fits Slack's length limit.
// Otherwise it keeps the leading characters, then `-` and the first hex digits of the SHA-256 of the whole name.
// Two long names that share a prefix stay apart.
func fitName(name string) string {
	if len(name) <= maxChannelNameLength {
		return name
	}
	sum := sha256.Sum256([]byte(name))
	hash := hex.EncodeToString(sum[:])[:channelNameHashLength]
	return name[:maxChannelNameLength-channelNameHashLength-1] + "-" + hash
}
