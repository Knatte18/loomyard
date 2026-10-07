// remover.go declares the strand remover a burler round stops one of its two halves with: a told seam over reed's strand table.

package burlerengine

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// StrandRemover stops a half of a round by its strand guid.
// RemoveStrandIfLive removes the strand when reed still reports it live and is a no-op otherwise;
// it kills any live strand it is handed, so a caller passes only the guid of a half's own run.
// A failed liveness probe and a failed removal are both returned, so a caller never proceeds beside a strand it could not stop.
type StrandRemover interface {
	RemoveStrandIfLive(guid string) error
}

// reedStrandRemover is the StrandRemover backed by reed's strand table.
type reedStrandRemover struct {
	reed shuttleengine.ReedOps
}

// NewReedStrandRemover returns the StrandRemover that probes and removes strands through reed.
func NewReedStrandRemover(reed shuttleengine.ReedOps) StrandRemover {
	return reedStrandRemover{reed: reed}
}

// RemoveStrandIfLive removes guid's strand when reed reports it live, logging the removal because it kills a real agent process.
// A probe that could not answer has not shown the strand dead, so it returns an error rather than guessing.
func (r reedStrandRemover) RemoveStrandIfLive(guid string) error {
	status, err := r.reed.Status()
	if err != nil {
		return fmt.Errorf("burler: probe strand %s before stopping it: %w", guid, err)
	}
	live := false
	for _, s := range status.Strands {
		if s.GUID == guid {
			live = s.Live
			break
		}
	}
	if !live {
		return nil
	}
	logger.Warn("burler: stopping a live half of a round", "strandGUID", guid)
	if _, err := r.reed.RemoveStrand(guid, false); err != nil {
		return fmt.Errorf("burler: remove strand %s: %w", guid, err)
	}
	return nil
}
