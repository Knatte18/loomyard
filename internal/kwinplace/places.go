// places.go keeps a hub's remembered window places and the rules that update them.

package kwinplace

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/state"
)

const (
	placesFileName     = "places.json"
	placesLockFileName = "places.lock"
)

// Places maps a worktree name to the remembered place of its window.
type Places map[string]Entry

// Entry is what is remembered about one worktree's window.
// PID and Title name the window, and SettledBy is the process id of the window whose placement last finished.
// Home is the id of the desktop the window goes back to, "" for none.
// OrderKey is the window's position among that desktop's windows.
// LastDesktop is the desktop it was last placed or seen on.
type Entry struct {
	Worktree    string    `json:"worktree"`
	PID         int       `json:"pid"`
	Title       string    `json:"title"`
	SettledBy   int       `json:"settledBy"`
	Home        string    `json:"home"`
	OrderKey    int       `json:"orderKey"`
	LastDesktop string    `json:"lastDesktop"`
	Output      string    `json:"output"`
	RecordedAt  time.Time `json:"recordedAt"`
}

// Settled reports whether the placement of the entry's current window has finished.
func (e Entry) Settled() bool {
	return e.SettledBy == e.PID
}

// PlacesPath returns the places file of the hub whose Bolt checkout is at boltDir.
func PlacesPath(boltDir string) string {
	return filepath.Join(boltDir, lymdirs.DotLymDirName, lymdirs.WindowPlacesDirName, placesFileName)
}

func placesLockPath(path string) string {
	return filepath.Join(filepath.Dir(path), placesLockFileName)
}

// ReadPlaces returns the places recorded at path.
// An absent file reads as empty, and so does one that cannot be read or decoded, which is logged.
func ReadPlaces(path string) Places {
	places, found, err := state.ReadJSON[Places](path, placesLockPath(path))
	if err != nil {
		logger.Warn("kwinplace: unreadable places file, reading as empty", "path", path, "error", err)
		return Places{}
	}
	if !found || places == nil {
		return Places{}
	}
	return places
}

// RecordOpen records that the worktree's window is now process pid titled title, not yet placed.
// What the entry remembers of the place is kept.
func RecordOpen(path, worktree string, pid int, title string, now time.Time) error {
	return writePlaces(path, func(places Places) Places {
		entry := places[worktree]
		entry.Worktree, entry.PID, entry.Title, entry.RecordedAt = worktree, pid, title, now
		// Unsettled even when pid repeats the previous window's, as it may after a reboot.
		entry.SettledBy = 0
		places[worktree] = entry
		return places
	})
}

// Settle records that the placement of the window pid ended without placing it.
// seenOn is the desktop the window was seen on, "" when it was not seen, which keeps the last known one.
// An entry that names another window is left unchanged.
func Settle(path, worktree string, pid int, seenOn string, now time.Time) error {
	return writePlaces(path, func(places Places) Places {
		entry, found := places[worktree]
		if !found {
			entry = Entry{Worktree: worktree, PID: pid}
		}
		if entry.PID != pid {
			return places
		}
		entry.SettledBy, entry.RecordedAt = pid, now
		if seenOn != "" {
			entry.LastDesktop = seenOn
		}
		places[worktree] = entry
		return places
	})
}

// SeenWindow is one window of the hub as a report showed it.
// Desktop is the one desktop it sits on, "" when it sits on several or on all.
// Order is its position in that desktop's current order.
// A Minimized window stands in no zone, so its Order says nothing of its place.
type SeenWindow struct {
	Worktree  string
	Desktop   string
	Order     int
	Minimized bool
}

// ReportUpdate is what one placement learned about the hub's windows.
type ReportUpdate struct {
	// Worktree and PID name the placed window.
	Worktree string
	PID      int
	// Landed is the desktop the placed window is on now, "" when it is on several or on all.
	Landed string
	// Overflowed reports that the window landed off its home because the home was full.
	Overflowed bool
	// Fallback reports that the window was anchored on the current desktop for want of a parent's place.
	Fallback bool
	// HomeGone reports that the window's remembered home is no longer a desktop.
	HomeGone bool
	// Output is the screen the window is on.
	Output string
	Now    time.Time
	// Worktrees lists every worktree the hub has.
	// It is called while the places are locked for writing, so no entry then present is newer than the list.
	// When it fails, no entry is dropped.
	Worktrees func() ([]string, error)
	// Windows holds every reported window of the hub, the placed one included.
	Windows []SeenWindow
}

// RecordReport updates the places at path with what update reports.
func RecordReport(path string, update ReportUpdate) error {
	return writePlaces(path, func(places Places) Places {
		return updatePlaces(places, update)
	})
}

// writePlaces is one locked read-modify-write of the places file.
// A file that does not decode is removed and written afresh.
func writePlaces(path string, mutate func(Places) Places) error {
	write := func() error {
		return state.UpdateJSON(path, placesLockPath(path), func(places Places, _ bool) (Places, error) {
			if places == nil {
				places = Places{}
			}
			return mutate(places), nil
		})
	}
	err := write()
	if errors.Is(err, state.ErrDecode) {
		if err := removeUndecodablePlaces(path); err != nil {
			return err
		}
		err = write()
	}
	if err != nil {
		return fmt.Errorf("kwinplace: write places: %w", err)
	}
	return nil
}

// removeUndecodablePlaces removes the file at path if it still does not decode once the lock is held,
// so a file another writer has since written is kept.
func removeUndecodablePlaces(path string) error {
	held, err := lock.AcquireWriteLock(placesLockPath(path))
	if err != nil {
		return fmt.Errorf("kwinplace: lock places: %w", err)
	}
	defer held.Release()

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("kwinplace: read places: %w", err)
	}
	var places Places
	if json.Unmarshal(data, &places) == nil {
		return nil
	}
	logger.Warn("kwinplace: removing undecodable places file", "path", path)
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("kwinplace: remove undecodable places: %w", err)
	}
	return nil
}

// updatePlaces returns cur as a report leaves it.
// The placed window is settled where it landed, and windows moved by hand are rehomed.
// An entry that names a newer window of the placed one's worktree is left unchanged.
// The order keys of disturbed desktops are renumbered, and the entries of removed worktrees are dropped.
func updatePlaces(cur Places, update ReportUpdate) Places {
	places := maps.Clone(cur)
	if places == nil {
		places = Places{}
	}
	if worktrees, err := update.Worktrees(); err != nil {
		logger.Warn("kwinplace: worktrees not listed, dropping no entry", "error", err)
	} else {
		maps.DeleteFunc(places, func(worktree string, _ Entry) bool {
			return !slices.Contains(worktrees, worktree)
		})
	}
	// The desktops whose order keys are to be renumbered.
	disturbed := map[string]bool{}

	windows := slices.Clone(update.Windows)
	placed := places[update.Worktree]
	if placed.PID == 0 {
		placed.Worktree, placed.PID = update.Worktree, update.PID
	}
	if placed.PID == update.PID {
		placed.SettledBy, placed.Output, placed.RecordedAt = update.PID, update.Output, update.Now
		if update.Landed != "" {
			placed.LastDesktop = update.Landed
			if (placed.Home == "" || update.HomeGone) && !update.Fallback && !update.Overflowed {
				rehome(places, &placed, update.Landed)
			}
		}
		places[update.Worktree] = placed
	} else {
		// Where this older window stands says nothing about the newer one the entry names.
		windows = slices.DeleteFunc(windows, func(w SeenWindow) bool { return w.Worktree == update.Worktree })
	}

	seen := slices.SortedStableFunc(slices.Values(windows), func(a, b SeenWindow) int {
		return cmp.Or(cmp.Compare(a.Desktop, b.Desktop), cmp.Compare(a.Order, b.Order))
	})
	for _, window := range seen {
		entry, found := places[window.Worktree]
		if window.Worktree == update.Worktree || !found || !entry.Settled() {
			continue
		}
		if window.Desktop == "" || window.Desktop == entry.LastDesktop {
			continue
		}
		entry.LastDesktop = window.Desktop
		if entry.Home != window.Desktop {
			rehome(places, &entry, window.Desktop)
		}
		places[window.Worktree] = entry
		disturbed[window.Desktop] = true
	}

	// sitting lists, per desktop, the settled windows that sit on it and are homed on it, in current order.
	// A minimized window is left out, so it keeps its position as an absent one does.
	sitting := map[string][]string{}
	for _, window := range seen {
		entry, found := places[window.Worktree]
		if found && entry.Settled() && !window.Minimized && window.Desktop != "" && entry.Home == window.Desktop {
			sitting[window.Desktop] = append(sitting[window.Desktop], window.Worktree)
		}
	}
	for desktop, worktrees := range sitting {
		if !slices.IsSortedFunc(worktrees, func(a, b string) int { return cmp.Compare(places[a].OrderKey, places[b].OrderKey) }) {
			disturbed[desktop] = true
		}
	}
	for desktop := range disturbed {
		renumber(places, desktop, sitting[desktop])
	}
	return places
}

// rehome makes desktop the entry's home, ordered after every window already homed there.
func rehome(places Places, entry *Entry, desktop string) {
	next := 0
	for worktree, other := range places {
		if worktree != entry.Worktree && other.Home == desktop {
			next = max(next, other.OrderKey+1)
		}
	}
	entry.Home, entry.OrderKey = desktop, next
}

// renumber numbers the entries homed on desktop from 0.
// The windows named by sitting, in current order, take the positions in key order that they held between them;
// every other entry keeps its position.
func renumber(places Places, desktop string, sitting []string) {
	var homed []string
	for worktree, entry := range places {
		if entry.Home == desktop {
			homed = append(homed, worktree)
		}
	}
	slices.SortFunc(homed, func(a, b string) int {
		return cmp.Or(cmp.Compare(places[a].OrderKey, places[b].OrderKey), cmp.Compare(a, b))
	})
	next := 0
	for i, worktree := range homed {
		if slices.Contains(sitting, worktree) {
			worktree = sitting[next]
			next++
		}
		entry := places[worktree]
		entry.OrderKey = i
		places[worktree] = entry
	}
}
