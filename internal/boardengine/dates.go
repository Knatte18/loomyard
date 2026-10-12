// dates.go — an entry's created and revised times: stamping them on a write and showing them as a compact age.

package boardengine

import (
	"fmt"
	"time"
)

// entryTime is t in UTC and whole seconds, the precision board.json stores.
func entryTime(t time.Time) time.Time {
	if t.IsZero() {
		return t
	}
	return t.UTC().Truncate(time.Second)
}

// stampDates sets the dates on after, the entries a write leaves, judged against loaded, the entries it started from.
// A new entry gets now for whichever of Created and Revised it does not carry.
// An entry whose scope changed gets now as Revised unless the write set Revised itself;
// a status change is no revision, and neither is promotedSlug, the entry a promote made a task, so a promote keeps the note's dates.
// A dependency on a done entry that the same write removes is no revision either, so prune leaves the survivors' dates.
// An existing entry without a Created keeps none, since its creation is unknown.
func stampDates(loaded, after []Task, now time.Time, promotedSlug string) {
	now = entryTime(now)
	loadedBySlug := make(map[string]Task, len(loaded))
	for _, task := range loaded {
		loadedBySlug[task.Slug] = task
	}
	removedDone := removedDoneSlugs(loaded, after)

	for i := range after {
		before, existed := loadedBySlug[after[i].Slug]
		if !existed {
			if after[i].Created.IsZero() {
				after[i].Created = now
			}
			if after[i].Revised.IsZero() {
				after[i].Revised = now
			}
			continue
		}
		judged := after[i]
		if judged.Slug == promotedSlug {
			judged.Kind = before.Kind
		}
		if scopeChanged(before, judged, removedDone) && after[i].Revised.Equal(before.Revised) {
			after[i].Revised = now
		}
	}
}

// FormatAge is the time from since to now in one compact unit: minutes, hours, days, weeks, months or years, such as `3d` or `5w`.
// A zero since, an unknown date, is the empty string, and a since after now is `0m`.
func FormatAge(since, now time.Time) string {
	if since.IsZero() {
		return ""
	}
	age := max(now.Sub(since), 0)
	day := 24 * time.Hour
	switch {
	case age < time.Hour:
		return fmt.Sprintf("%dm", int(age/time.Minute))
	case age < day:
		return fmt.Sprintf("%dh", int(age/time.Hour))
	case age < 14*day:
		return fmt.Sprintf("%dd", int(age/day))
	case age < 91*day:
		return fmt.Sprintf("%dw", int(age/(7*day)))
	case age < 365*day:
		return fmt.Sprintf("%dmo", int(age/(30*day)))
	default:
		return fmt.Sprintf("%dy", int(age/(365*day)))
	}
}
