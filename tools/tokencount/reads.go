// reads.go finds and reads sessions through the shuttle engines: the read set tokencount is handed, and the locating and reading of a worktree's sessions across it.

package main

import (
	"fmt"
	"sort"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// ReadSet is the offline read capabilities of every registered shuttle engine, keyed by engine name.
type ReadSet map[string]shuttleengine.RecordReads

// engines returns, sorted, the names of the engines that can locate and read transcripts and the names of those that cannot.
func (reads ReadSet) engines() (readable, unreadable []string) {
	for name, capabilities := range reads {
		if capabilities.Transcripts != nil {
			readable = append(readable, name)
		} else {
			unreadable = append(unreadable, name)
		}
	}
	sort.Strings(readable)
	sort.Strings(unreadable)
	return readable, unreadable
}

// locatedSession is a session and the name of the engine that located it.
type locatedSession struct {
	engine  string
	session shuttleengine.LocatedSession
}

// locateSessions asks every readable engine, in name order, for the sessions it recorded under worktree.
// Within an engine they are sorted by session ID.
// An engine that fails to locate fails the call, naming the engine and the worktree.
func locateSessions(reads ReadSet, worktree string) ([]locatedSession, error) {
	readable, _ := reads.engines()
	var located []locatedSession
	for _, name := range readable {
		sessions, err := reads[name].Transcripts.LocateSessions(worktree)
		if err != nil {
			return nil, fmt.Errorf("locating sessions of %s through engine %s: %w", worktree, name, err)
		}
		sort.Slice(sessions, func(i, j int) bool { return sessions[i].ID < sessions[j].ID })
		for _, session := range sessions {
			located = append(located, locatedSession{engine: name, session: session})
		}
	}
	return located, nil
}

// readLocated reads a located session through the engine that located it.
// A failure names the session and the engine, and keeps the engine's error in its chain.
func readLocated(reads ReadSet, s locatedSession) (shuttleengine.SessionRead, error) {
	read, err := reads[s.engine].Transcripts.ReadSession(s.session)
	if err != nil {
		return shuttleengine.SessionRead{}, fmt.Errorf("reading session %s of engine %s: %w", s.session.ID, s.engine, err)
	}
	return read, nil
}

// usageOf converts one message's neutral usage into a tally.
func usageOf(u shuttleengine.MessageUsage) Usage {
	return Usage{Input: u.Input, Output: u.Output, CacheCreate: u.CacheWrite, CacheRead: u.CacheRead, CacheCreation: CacheCreation{OneHour: u.CacheWriteOneHour}}
}
