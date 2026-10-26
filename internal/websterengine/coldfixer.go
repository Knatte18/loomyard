// coldfixer.go implements the bounded wait the cold fixer strands share: a strand that is done once its reply file exists, and dead when its turn ends without one, its timeout passes or its strand is gone.

package websterengine

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// coldFixerStrand is what the wait reads of one cold fixer strand's record.
type coldFixerStrand struct {
	// label names the fixer's verb in warnings, such as "fix-verify".
	label         string
	strandGUID    string
	sessionID     string
	engine        string
	spawnedAt     string
	shuttleRunDir string
	eventsPath    string
	eventsOffset  int64
	replyPath     string
}

// coldFixerWait is one bounded wait's outcome.
type coldFixerWait struct {
	// Status is DigestStatusRunning when the wait elapsed first, else DigestStatusDone or DigestStatusDead.
	Status string
	// DeadReason is set only when Status is DigestStatusDead.
	DeadReason string
	// Reply is the reply file's content, set only when Status is DigestStatusDone.
	Reply string
	// ElapsedS is the seconds since the spawn.
	ElapsedS int
	// Warnings are cleanup failures that change no status.
	Warnings []string
}

// awaitColdFixer blocks up to wait for strand to reach a terminal classification, polling on the shared tick.
// The fixer is done once its reply file exists.
// It is dead when its newest turn ended without one (DeadReasonAsking), when timeout has passed since the spawn (DeadReasonTimeout), or when its strand is gone (DeadReasonDied), checked in that order.
// Every terminal result stops the strand; only done also reads the reply and removes the run dir.
// A failed stop, reply read or removal is a warning.
func awaitColdFixer(engines AuditEngines, reed shuttleengine.ReedOps, stopper StrandStopper, strand coldFixerStrand, timeout, wait time.Duration, clk Clock) (*coldFixerWait, error) {
	spawnedAt, err := time.Parse(time.RFC3339, strand.spawnedAt)
	if err != nil {
		return nil, fmt.Errorf("webster: parse the %s fixer's recorded spawnedAt %q: %w", strand.label, strand.spawnedAt, err)
	}
	engine, err := sessionAuditEngine(engines, strand.sessionID, strand.engine)
	if err != nil {
		return nil, err
	}

	gather := func() (Digest, bool, error) {
		if _, err := os.Stat(strand.replyPath); err == nil {
			return Digest{Status: DigestStatusDone}, true, nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return Digest{}, false, fmt.Errorf("webster: stat %s fixer reply %s: %w", strand.label, strand.replyPath, err)
		}
		turnEnded, err := TurnEndedAfter(strand.eventsPath, strand.eventsOffset, engine, TurnEndRead{Now: clk.Now()})
		if err != nil {
			return Digest{}, false, err
		}
		if turnEnded {
			return Digest{Status: DigestStatusDead, DeadReason: DeadReasonAsking}, true, nil
		}
		if clk.Now().Sub(spawnedAt) > timeout {
			return Digest{Status: DigestStatusDead, DeadReason: DeadReasonTimeout}, true, nil
		}
		live, err := StrandLive(reed, strand.strandGUID)
		if err != nil {
			return Digest{}, false, err
		}
		if !live {
			return Digest{Status: DigestStatusDead, DeadReason: DeadReasonDied}, true, nil
		}
		return Digest{Status: DigestStatusRunning}, false, nil
	}

	digest, err := PollUntilTerminal(gather, wait, clk)
	if err != nil {
		return nil, err
	}
	result := &coldFixerWait{Status: digest.Status, DeadReason: digest.DeadReason, ElapsedS: int(clk.Now().Sub(spawnedAt).Seconds())}
	if digest.Status == DigestStatusRunning {
		return result, nil
	}

	if err := stopper.StopStrand(strand.strandGUID); err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("%s: stop strand %s: %v", strand.label, strand.strandGUID, err))
	}
	if digest.Status == DigestStatusDone {
		reply, err := os.ReadFile(strand.replyPath)
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("%s: read reply %s: %v", strand.label, strand.replyPath, err))
		}
		result.Reply = string(reply)
		if strand.shuttleRunDir != "" {
			if err := os.RemoveAll(strand.shuttleRunDir); err != nil {
				result.Warnings = append(result.Warnings, fmt.Sprintf("%s: remove run dir %s: %v", strand.label, strand.shuttleRunDir, err))
			}
		}
	}
	return result, nil
}
