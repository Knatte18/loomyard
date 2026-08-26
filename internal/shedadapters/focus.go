// focus.go implements RoundFocus, the next-round directive a segment's Bouncer writes and its
// BurlerRound row reads, and readRoundFocus, the fail-safe reader that turns a missing, unreadable,
// or malformed focus file into the zero directive rather than an error.
// The file it reads is the one the Bouncer writes -- round-<N>-focus.md, the YAML-frontmatter shape
// bouncerfiles.go renders and the two bouncer stencils instruct the judge to produce. See
// readRoundFocus for why that had to be said out loud.

package shedadapters

import (
	"os"

	"github.com/Knatte18/loomyard/internal/logger"
)

// burlerEngineLabel is the short engine label BurlerProducer's log lines and error text carry.
const burlerEngineLabel = "burler"

// RoundFocus is the next-round directive a segment's Bouncer writes and its BurlerRound row reads.
// Both fields are optional, and the zero value means "no directive", which every failure path
// degrades to.
// The round token in the filename this directive is read from names the round the directives are
// for, not the round that produced them -- stated explicitly because getting it wrong fails
// silently: a Bouncer rejecting round N writes the file for round N+1, and the seed call writes the
// file for round 1.
type RoundFocus struct {
	// ExcludeLenses names lenses the next round should skip, carried verbatim from the focus file's
	// exclude_lenses list.
	ExcludeLenses []string
	// Hydrate names absolute paths the next round should hydrate into context. It carries the focus
	// file's own path, and only when that file actually says something -- see readRoundFocus.
	Hydrate []string
}

// readRoundFocus reads round's focus file in runDir and returns the RoundFocus it names,
// identifying itself as name in every warning it logs.
//
// It reads focusPath(runDir, round) -- round-<N>-focus.md -- through parseFocus, the same path
// builder and the same parser the Bouncer writes that file with. That agreement is the whole
// substance of this function and is stated here because it was once absent: this reader used to open
// a round-<N>-focus.json path and strictly decode JSON, a filename, a serialization, and a field set
// the writer never produced, so the read failed on every call in production and the judge's entire
// targeting channel was dead. The seed pass spent a real LLM spawn and a permanent unit of the
// segment's bounce budget writing a file its only consumer could not read.
//
// It is fail-safe end to end and never returns an error: an absent file, a read failure, or a file
// parseFocus rejects each yields the zero RoundFocus after a logger.Warn.
//
// Hydrate carries the focus file's own path, and carries it only when the file has a directive to
// deliver -- a non-empty focus list or non-empty prose. That is how the judge's targeting reaches
// the fixer round: BurlerProducer appends Hydrate onto the profile's PriorReviews, so the round
// reads the directive verbatim alongside the prior rounds' reports. An APPROVED judge still writes a
// focus file (its third output file is unconditional, so the run classifies complete), and that file
// carries two empty lists and no prose; hydrating it would hand the next round an empty document
// asserting nothing, so an empty directive stays empty.
func readRoundFocus(name, runDir string, round int) RoundFocus {
	path := focusPath(runDir, round)

	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			logger.Warn("shedadapters: focus file absent", "producer", name, "engine", burlerEngineLabel, "path", path, "reason", "absent")
			return RoundFocus{}
		}
		logger.Warn("shedadapters: focus file unreadable", "producer", name, "engine", burlerEngineLabel, "path", path, "reason", err.Error())
		return RoundFocus{}
	}

	parsed, err := parseFocus(content)
	if err != nil {
		logger.Warn("shedadapters: focus file malformed", "producer", name, "engine", burlerEngineLabel, "path", path, "reason", err.Error())
		return RoundFocus{}
	}

	focus := RoundFocus{ExcludeLenses: parsed.ExcludeLenses}
	if len(parsed.Focus) > 0 || parsed.Prose != "" {
		focus.Hydrate = []string{path}
	}
	return focus
}
