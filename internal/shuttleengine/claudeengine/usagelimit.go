// usagelimit.go reads when a usage limit resets from the text of Claude Code's usage-limit message.

package claudeengine

import (
	"regexp"
	"strconv"
	"time"
	// The zone database is embedded so a reset clause's zone resolves on a machine with no system zone files, such as Windows.
	_ "time/tzdata"
)

// resetClause matches the reset clause of Claude Code's usage-limit message: `resets 2pm (Europe/Oslo)`, `resets 2:30pm (Europe/Oslo)`, `resets Oct 30, 2pm (Europe/Oslo)` or `reset at 2pm (Europe/Oslo)`.
// Its groups are the month, the day, the hour, the minute, `am` or `pm`, and the IANA zone.
var resetClause = regexp.MustCompile(`resets?(?: at)? (?:([A-Z][a-z]{2}) (\d{1,2}),? (?:at )?)?(\d{1,2})(?::(\d{2}))? ?([ap]m) \(([^)]+)\)`)

// usageLimitResetsAt returns when the usage limit the message text names resets: the first instant after the entry's RFC 3339 timestamp that matches the text's reset clause in the clause's zone.
// A clause with a month and day resolves to that date, in the timestamp's year or the next.
// It returns the zero time when the text has no reset clause, the timestamp does not parse, or the zone, hour, minute, month or day of the month is not valid.
func usageLimitResetsAt(text, timestamp string) time.Time {
	match := resetClause.FindStringSubmatch(text)
	if match == nil {
		return time.Time{}
	}
	written, err := time.Parse(time.RFC3339, timestamp)
	if err != nil {
		return time.Time{}
	}
	zone, err := time.LoadLocation(match[6])
	if err != nil {
		return time.Time{}
	}
	hour, _ := strconv.Atoi(match[3])
	minute := 0
	if match[4] != "" {
		minute, _ = strconv.Atoi(match[4])
	}
	if hour < 1 || hour > 12 || minute > 59 {
		return time.Time{}
	}
	hour %= 12
	if match[5] == "pm" {
		hour += 12
	}
	local := written.In(zone)
	if match[1] == "" {
		reset := time.Date(local.Year(), local.Month(), local.Day(), hour, minute, 0, 0, zone)
		if !reset.After(written) {
			reset = time.Date(local.Year(), local.Month(), local.Day()+1, hour, minute, 0, 0, zone)
		}
		return reset
	}
	month, err := time.Parse("Jan", match[1])
	if err != nil {
		return time.Time{}
	}
	day, _ := strconv.Atoi(match[2])
	reset := time.Date(local.Year(), month.Month(), day, hour, minute, 0, 0, zone)
	if !reset.After(written) {
		reset = time.Date(local.Year()+1, month.Month(), day, hour, minute, 0, 0, zone)
	}
	// time.Date normalises a day the month lacks into the next month.
	if reset.Day() != day || reset.Month() != month.Month() {
		return time.Time{}
	}
	return reset
}
