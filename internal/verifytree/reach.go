// reach.go declares the reach turn.
// It is a decorator over a verify gate that hands a writer the reach note through its own gate.

package verifytree

import (
	"maps"
	"path/filepath"
	"strings"
	"sync"

	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/state"
)

// The name of the file a site's turns record their listed functions in is the prefix, the site slug and the suffix.
const (
	reachListedPrefix = "reach-listed-"
	reachListedSuffix = ".json"
)

// reachWayForward closes the findings of a reach turn.
const reachWayForward = "Add the missing tests and commit them, or end your turn unchanged."

// ReachPending is what a reach report that matches the tree asks of the writer.
type ReachPending struct {
	// Note is the reach note's lines joined by newlines.
	Note string
	// Functions maps the id of each unreached changed function to the text hash of its declaration.
	Functions map[string]string
}

// ReachTurn tells WithReachTurn where its pending report comes from and how often it may spend a writer turn.
type ReachTurn struct {
	// Pending returns the pending report.
	// It returns false when no report of the gate's key matches HEAD or the report lists no unreached function.
	Pending func() (ReachPending, bool, error)
	// Attempts is the gate entry's attempt budget.
	Attempts int
	// Rerun is the entry's mark of a re-run after a cold fixer; nil where nothing re-runs the gate.
	Rerun *shuttleengine.RerunMark
	// ListedPath is the file recording the functions the turns of this gate listed, with their hashes.
	ListedPath string
}

// reachListed is the record a turn leaves in its listed file.
type reachListed struct {
	Functions map[string]string `json:"functions"`
}

// ReachListedPath names the file in verifyDir that records the functions each turn of site listed, with their hashes.
func ReachListedPath(verifyDir, site string) string {
	slug := strings.ReplaceAll(strings.ToLower(site), " ", "-")
	return filepath.Join(verifyDir, reachListedPrefix+slug+reachListedSuffix)
}

// WithReachTurn returns a gate that runs inner and then, once inner passes, hands the writer the pending reach note.
// A failing result of inner is returned as it is and counts one failing arrival.
// A passing result reads turn.Pending.
// The turn runs when the report holds a function whose id and hash the listed file lacks.
// The failing count plus one must also be below turn.Attempts.
// The turn then adds the pending functions to those the listed file holds.
// It returns one failing result marked WriterOnly.
// Its findings are the note followed by the way forward.
// Any other pass is returned and resets the count.
// A call inside turn.Rerun adds nothing to the count, since its arrival's first, failing call counted already.
// An error of turn.Pending is the gate's error.
// A listed file that cannot be read or written is logged at Warn, and the gate returns the pass without the turn.
func WithReachTurn(inner shuttleengine.Gate, turn ReachTurn) shuttleengine.Gate {
	var mu sync.Mutex
	failing := 0
	return func() (shuttleengine.GateResult, error) {
		rerun := turn.Rerun.On()
		result, err := inner()
		if err != nil {
			return result, err
		}
		mu.Lock()
		defer mu.Unlock()
		if !result.Passed {
			if !rerun && !result.Pending && !result.Terminal {
				failing++
			}
			return result, nil
		}
		findings, listed, err := turn.findings(failing)
		if err != nil {
			return shuttleengine.GateResult{}, err
		}
		if !listed {
			failing = 0
			return result, nil
		}
		if !rerun {
			failing++
		}
		return shuttleengine.GateResult{Findings: findings, WriterOnly: true}, nil
	}
}

// findings returns the findings of a turn after failing failing arrivals, and false when the arrival passes without one.
func (t ReachTurn) findings(failing int) (string, bool, error) {
	pending, found, err := t.Pending()
	if err != nil || !found || failing+1 >= t.Attempts {
		return "", false, err
	}
	lockPath := lock.TrackedFileLockPath(t.ListedPath, ".lock")
	listed, _, err := state.ReadJSON[reachListed](t.ListedPath, lockPath)
	if err != nil {
		t.warnListedFile(err)
		return "", false, nil
	}
	missing := false
	for id, hash := range pending.Functions {
		if recorded, ok := listed.Functions[id]; !ok || recorded != hash {
			missing = true
			break
		}
	}
	if !missing {
		return "", false, nil
	}
	everListed := maps.Clone(listed.Functions)
	if everListed == nil {
		everListed = map[string]string{}
	}
	maps.Copy(everListed, pending.Functions)
	if err := state.WriteJSON(t.ListedPath, lockPath, reachListed{Functions: everListed}); err != nil {
		t.warnListedFile(err)
		return "", false, nil
	}
	return pending.Note + "\n\n" + reachWayForward, true, nil
}

// warnListedFile logs a listed file the turn could not use.
func (t ReachTurn) warnListedFile(cause error) {
	logger.Warn("verifytree: the reach turn cannot use its listed file, so the gate passes without the turn", "path", t.ListedPath, "cause", cause)
}
