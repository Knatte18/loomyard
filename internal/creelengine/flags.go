// flags.go keeps one flag file per strand that holds unread mail or is owed an echo, in Geometry.FlagDir.
// A strand's tool-boundary hook tests that file before it asks the daemon for mail.

package creelengine

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/logger"
)

// flaggedFilesLocked returns the flag file names the pool asks for.
// It names one file per strand that holds unread mail a claim can hand out or is owed an echo, reserved or not;
// it names none for a program.
func (s *Store) flaggedFilesLocked() map[string]bool {
	want := map[string]bool{}
	for strand := range s.echoes {
		want[agentname.FileName(strand)] = true
	}
	for _, msg := range s.state.messages {
		if msg.Status == StatusUnread && deliverableReceiver(msg.Holder) && !lineOverHookField(msg) {
			want[agentname.FileName(msg.Holder)] = true
		}
	}
	return want
}

// syncFlagsLocked brings the flag directory in line with the flags the pool asks for.
// It creates each missing flag and removes each flag the pool no longer asks for.
// A failed create or removal is logged and not recorded, so the next sync tries it again.
// A store with no flag directory keeps no flags.
func (s *Store) syncFlagsLocked() {
	if s.geom.FlagDir == "" {
		return
	}
	want := s.flaggedFilesLocked()
	for name := range want {
		if s.flags[name] {
			continue
		}
		if err := os.WriteFile(filepath.Join(s.geom.FlagDir, name), nil, 0o644); err != nil {
			logger.Warn("creel: could not set a strand's mail flag", "flag", name, "error", err)
			continue
		}
		s.flags[name] = true
	}
	for name := range s.flags {
		if want[name] {
			continue
		}
		if err := os.Remove(filepath.Join(s.geom.FlagDir, name)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			logger.Warn("creel: could not clear a strand's mail flag", "flag", name, "error", err)
			continue
		}
		delete(s.flags, name)
	}
}

// ReconcileFlags makes Geometry.FlagDir hold exactly the flags the pool asks for.
// It creates the directory and removes every other regular file in it.
// The daemon runs it once at start,
// and it puts right a flag an earlier daemon left behind for mail since read, or a flag lost with the directory.
func (s *Store) ReconcileFlags() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(s.geom.FlagDir, 0o755); err != nil {
		return fmt.Errorf("creel: create the flag directory %s: %w", s.geom.FlagDir, err)
	}
	entries, err := os.ReadDir(s.geom.FlagDir)
	if err != nil {
		return fmt.Errorf("creel: read the flag directory %s: %w", s.geom.FlagDir, err)
	}
	s.flags = map[string]bool{}
	for _, entry := range entries {
		if entry.Type().IsRegular() {
			s.flags[entry.Name()] = true
		}
	}
	s.syncFlagsLocked()
	return nil
}
