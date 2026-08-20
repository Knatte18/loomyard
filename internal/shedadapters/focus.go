// focus.go implements RoundFocus, the structured next-round directive a segment's Bouncer writes
// and a round producer reads, and readRoundFocus, the fail-safe reader that turns a missing,
// unreadable, or malformed focus file into the zero directive rather than an error.

package shedadapters

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/logger"
)

// burlerEngineLabel is the short engine label BurlerProducer's log lines and error text carry.
const burlerEngineLabel = "burler"

// RoundFocus is the structured, mechanically-parseable next-round directive a segment's Bouncer
// writes and a round producer reads.
// Both fields are optional.
// The round token in the filename this directive is read from names the round the directives are
// for, not the round that produced them -- stated explicitly because getting it wrong fails
// silently: a Bouncer rejecting round N writes the file for round N+1, and the seed call writes the
// file for round 1.
type RoundFocus struct {
	// ExcludeLenses names lenses the next round should skip.
	ExcludeLenses []string `json:"exclude_lenses,omitempty"`
	// Hydrate names absolute paths the next round should hydrate into context.
	Hydrate []string `json:"hydrate,omitempty"`
}

// roundFocusFilename returns the filename shape round-<round>-focus.json for a positive decimal
// round with no leading zeros and no attempt suffix.
func roundFocusFilename(round int) string {
	return fmt.Sprintf("round-%d-focus.json", round)
}

// readRoundFocus reads the focus file for round in runDir and returns the RoundFocus it names,
// identifying itself as name in every warning it logs.
// It is fail-safe end to end and never returns an error: it strictly decodes the file as JSON with
// unknown fields rejected, and returns the zero RoundFocus after a logger.Warn when the file is
// absent, when reading it fails for any other reason, when the JSON is malformed, or when it
// carries an unknown field.
// After a successful decode, readRoundFocus filters Hydrate: an entry that is not absolute, or that
// does not exist on disk, is dropped from the returned slice with a logger.Warn naming it, and the
// surviving entries keep their original order.
// ExcludeLenses is returned verbatim after a successful decode -- whether a named lens is usable is
// decided at application time by the producer and by burlerengine.Profile.validate, not here.
func readRoundFocus(name, runDir string, round int) RoundFocus {
	path := filepath.Join(runDir, roundFocusFilename(round))

	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			logger.Warn("shedadapters: focus file absent", "producer", name, "engine", burlerEngineLabel, "path", path, "reason", "absent")
			return RoundFocus{}
		}
		logger.Warn("shedadapters: focus file unreadable", "producer", name, "engine", burlerEngineLabel, "path", path, "reason", err.Error())
		return RoundFocus{}
	}
	defer f.Close()

	var focus RoundFocus
	dec := json.NewDecoder(f)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&focus); err != nil {
		logger.Warn("shedadapters: focus file malformed", "producer", name, "engine", burlerEngineLabel, "path", path, "reason", err.Error())
		return RoundFocus{}
	}

	kept := make([]string, 0, len(focus.Hydrate))
	for _, entry := range focus.Hydrate {
		if !filepath.IsAbs(entry) {
			logger.Warn("shedadapters: focus file hydrate entry dropped", "producer", name, "engine", burlerEngineLabel, "path", path, "reason", "not absolute", "entry", entry)
			continue
		}
		if _, err := os.Stat(entry); err != nil {
			logger.Warn("shedadapters: focus file hydrate entry dropped", "producer", name, "engine", burlerEngineLabel, "path", path, "reason", "does not exist", "entry", entry)
			continue
		}
		kept = append(kept, entry)
	}
	focus.Hydrate = kept

	return focus
}
