// resume.go implements the resume sequence and the status read.

package pauseengine

import (
	"errors"
	"fmt"

	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// ResumeRequest asks for a resume.
type ResumeRequest struct {
	// Slug is the pair to resume; empty resumes every live entry.
	Slug string
}

// ResumeReport is what a resume did.
type ResumeReport struct {
	// Resumed holds the scope of each entry lifted.
	Resumed []string `json:"resumed"`
	// Moved holds the sessions handed to another live entry that still covers them, which get no line.
	Moved []string `json:"moved"`
	// Lines holds the strands given the resume line.
	Lines []string `json:"lines"`
	// NoLine holds the sessions given no line, each with the reason.
	NoLine []SessionNote `json:"no_line"`
	// OrchLeftToWatcher reports that a resumed entry owned the orch's session, which the orch watcher gives its line.
	OrchLeftToWatcher bool `json:"orch_left_to_watcher"`
}

// StatusReport is what a pause currently holds.
type StatusReport struct {
	// Live holds the live entries.
	Live []pauserecord.Entry `json:"live"`
	// NetOps holds the network operations in flight.
	NetOps []NetOpReport `json:"net_ops"`
	// Holds holds the parent locks the covered runs hold or wait in.
	Holds holdReport `json:"holds"`
}

// Resume lifts live entries and gives each session they interrupted one line.
// A slug with no entry of its own that a live hub entry covers is refused, naming `lyx resume`; resuming nothing succeeds.
func Resume(deps Deps, req ResumeRequest) (ResumeReport, error) {
	report := ResumeReport{Resumed: []string{}, Moved: []string{}, Lines: []string{}, NoLine: []SessionNote{}}
	now := deps.Now()
	var resumed []pauserecord.Entry
	_, err := pauserecord.Update(deps.RecordPath, now, func(record *pauserecord.Record) error {
		var targets, kept []pauserecord.Entry
		for _, live := range record.Live {
			if req.Slug == "" || live.Scope == req.Slug {
				targets = append(targets, live)
			} else {
				kept = append(kept, live)
			}
		}
		if len(targets) == 0 {
			if _, covered := record.Covering(req.Slug); req.Slug != "" && covered {
				return fmt.Errorf("pause: %q has no pause of its own, and a hub-wide pause covers it; way forward: run \"lyx resume\" to lift the hub pause", req.Slug)
			}
			return nil
		}
		record.Live = kept
		for _, target := range targets {
			target.ResumedAt = now
			var owned []pauserecord.Session
			for _, held := range target.Sessions {
				if !moveToCoveringEntry(record, held) {
					owned = append(owned, held)
					continue
				}
				report.Moved = append(report.Moved, held.Strand)
			}
			target.Sessions = owned
			record.Resumed = append(record.Resumed, target)
			resumed = append(resumed, target)
			report.Resumed = append(report.Resumed, target.Scope)
		}
		return nil
	})
	if err != nil {
		return report, err
	}

	for _, entry := range resumed {
		for _, held := range entry.Sessions {
			if held.Orch {
				report.OrchLeftToWatcher = true
				continue
			}
			if reason := sendResumeLine(deps, held); reason != "" {
				report.NoLine = append(report.NoLine, SessionNote{held.Strand, held.Slug, reason})
				continue
			}
			report.Lines = append(report.Lines, held.Strand)
		}
	}
	return report, nil
}

// moveToCoveringEntry hands held to the oldest live entry of record that covers its pair, and reports whether one did.
func moveToCoveringEntry(record *pauserecord.Record, held pauserecord.Session) bool {
	cover, covered := record.Covering(held.Slug)
	if !covered {
		return false
	}
	for i := range record.Live {
		if sameEntry(record.Live[i], cover) {
			record.Live[i].Sessions = append(record.Live[i].Sessions, held)
			return true
		}
	}
	return false
}

// sendResumeLine gives held the resume line once and returns why it did not, empty when it did.
func sendResumeLine(deps Deps, held pauserecord.Session) string {
	var pair Pair
	for _, candidate := range deps.Pairs {
		if candidate.Slug() == held.Slug {
			pair = candidate
		}
	}
	if pair == nil {
		return "the pair is gone"
	}
	live, err := pair.Live(held.GUID)
	if err != nil {
		return "liveness reading failed: " + err.Error()
	}
	if !live {
		return "the strand is not live"
	}
	busy, err := pair.Busy(held.GUID)
	if err != nil {
		return "busy reading failed: " + err.Error()
	}
	if busy {
		return "the session reads busy"
	}
	switch err := pair.Send(held.GUID, pauserecord.ResumeLine); {
	case errors.Is(err, shuttleengine.ErrSessionBusy):
		return "the input box holds a draft"
	case err != nil:
		return "the send failed: " + err.Error()
	}
	return ""
}

// Status reads the live entries and the holds, changing nothing.
func Status(deps Deps) (StatusReport, error) {
	record, err := pauserecord.Read(deps.RecordPath)
	if err != nil {
		return StatusReport{}, err
	}
	report := StatusReport{Live: append([]pauserecord.Entry{}, record.Live...)}
	if report.NetOps, err = netOpReports(deps, func(string) bool { return true }); err != nil {
		return report, err
	}
	report.Holds, err = readHolds(deps, record)
	return report, err
}
