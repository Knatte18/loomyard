// flags.go keeps one flag file per strand holding unread mail in Geometry.FlagDir, which a strand's tool-boundary hook tests before it asks the daemon for mail.

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

// flaggedFilesLocked returns the flag file names the pool asks for: one per strand holding unread mail, and none for a program.
func (s *Store) flaggedFilesLocked() map[string]bool {
	want := map[string]bool{}
	for _, msg := range s.state.messages {
		if msg.Status == StatusUnread && deliverableReceiver(msg.Holder) {
			want[agentname.FileName(msg.Holder)] = true
		}
	}
	return want
}

// syncFlagsLocked creates each flag the pool asks for and the directory lacks, and removes each flag the directory holds and the pool no longer asks for.
// A failed create or removal is logged and left out of the record of what the directory holds, so the next sync tries it again.
// A store told no flag directory keeps no flags.
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

// ReconcileFlags makes Geometry.FlagDir hold exactly the flags the pool asks for, creating the directory and removing every other regular file in it.
// The daemon runs it once at start, so a flag left by an earlier daemon for mail since read, or a flag lost with the directory, is put right.
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
