// contractevidence.go decides whether a suspect path that is one of the run's two contract files, outcome.yaml and summary.md, is cleared by evidence.
// It is the one evidence function every site that classifies a suspect path consults before checkSuspectPaths,
// since those two paths lie outside the tracked tree and would otherwise be unverifiable, leaving the reset-to-start route as the only way forward.

package websterengine

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/summaryparser"
)

// contractFilePaths lists the run's two contract files: outcome.yaml, then summary.md.
func contractFilePaths(geom Geometry) []string {
	return []string{OutcomePath(geom.WebsterDir), summaryparser.Path(geom.WebsterDir)}
}

// contractCanon returns path's canonical spelling and whether it resolves to one of the run's contract files.
// The error is a link-resolution failure.
func contractCanon(geom Geometry, path string) (canon string, contract bool, err error) {
	canon, err = canonicalPath(resolveWritePath(geom.WorktreeRoot, path))
	if err != nil {
		return "", false, err
	}
	for _, c := range contractFilePaths(geom) {
		want, err := canonicalPath(c)
		if err != nil {
			return "", false, err
		}
		if canon == want {
			return canon, true, nil
		}
	}
	return canon, false, nil
}

// contractFileStatus reports whether path is one of this run's contract files, and whether the audit finding on it is cleared.
// A contract path is cleared when the file is absent, or when the latest successful Master write to it is later than every fork write to it.
// A Master write whose result failed is not evidence.
// Every other path is no contract path and never cleared here.
// The error is a link-resolution or stat failure.
func contractFileStatus(geom Geometry, writes RunWrites, path string) (contract, cleared bool, err error) {
	canon, contract, err := contractCanon(geom, path)
	if err != nil || !contract {
		return false, false, err
	}
	if _, statErr := os.Stat(canon); statErr != nil {
		if errors.Is(statErr, os.ErrNotExist) {
			return true, true, nil
		}
		return false, false, fmt.Errorf("websterengine: stat contract file %s: %w", canon, statErr)
	}
	lastMaster, found := latestSucceededWrite(geom, writes.Master, canon)
	if !found {
		return true, false, nil
	}
	for _, ev := range writes.Forks {
		evCanon, err := canonicalPath(resolveWritePath(geom.WorktreeRoot, ev.Path))
		if err != nil {
			return false, false, err
		}
		if evCanon == canon && !lastMaster.After(ev.At) {
			return true, false, nil
		}
	}
	return true, true, nil
}

// latestSucceededWrite returns the time of the latest successful event in events whose path canonicalizes to canon.
func latestSucceededWrite(geom Geometry, events []shuttleengine.WriteEvent, canon string) (latest time.Time, found bool) {
	for _, ev := range events {
		if !ev.Succeeded {
			continue
		}
		evCanon, err := canonicalPath(resolveWritePath(geom.WorktreeRoot, ev.Path))
		if err != nil || evCanon != canon {
			continue
		}
		if !found || ev.At.After(latest) {
			latest, found = ev.At, true
		}
	}
	return latest, found
}

// contractSplit sorts suspect paths by what the contract-file evidence says about them.
type contractSplit struct {
	// Cleared holds the contract paths the evidence clears, Absent the subset whose file is absent.
	Cleared, Absent []string
	// Uncleared holds the contract paths a fork wrote last.
	Uncleared []string
	// Rest holds every path that is no contract file, in input order.
	Rest []string
}

// splitContractPaths sorts paths into contractSplit's groups, each in input order.
// The error is contractFileStatus's.
func splitContractPaths(geom Geometry, writes RunWrites, paths []string) (contractSplit, error) {
	var s contractSplit
	for _, p := range paths {
		contract, cleared, err := contractFileStatus(geom, writes, p)
		if err != nil {
			return contractSplit{}, err
		}
		switch {
		case !contract:
			s.Rest = append(s.Rest, p)
		case !cleared:
			s.Uncleared = append(s.Uncleared, p)
		default:
			s.Cleared = append(s.Cleared, p)
			canon, _, err := contractCanon(geom, p)
			if err != nil {
				return contractSplit{}, err
			}
			if _, err := os.Stat(canon); errors.Is(err, os.ErrNotExist) {
				s.Absent = append(s.Absent, p)
			}
		}
	}
	return s, nil
}

// contractWritesFor loads the run's write history only when one of paths is a contract file, since no other path needs it.
// A nil engine yields an empty history, under which a present contract file is never cleared.
// The error is a link-resolution or audit failure.
func contractWritesFor(engine shuttleengine.Engine, st *State, geom Geometry, paths []string) (RunWrites, error) {
	if engine == nil {
		return RunWrites{}, nil
	}
	for _, p := range paths {
		if _, contract, err := contractCanon(geom, p); err != nil {
			return RunWrites{}, err
		} else if contract {
			return loadRunWrites(engine, st, geom.WorktreeRoot)
		}
	}
	return RunWrites{}, nil
}

// contractDeleteClause is the delete route for contract paths a fork wrote last: remove the files, then re-run through rerun.
func contractDeleteClause(paths []string, rerun string) string {
	return fmt.Sprintf("a fork wrote %s after Master's last write; %s", strings.Join(paths, ", "), wayForwardSteps("rm "+strings.Join(paths, " "), rerun))
}
