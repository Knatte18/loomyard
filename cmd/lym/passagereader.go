// passagereader.go adapts heddle's stored entries into the standing text a shuttle session is launched with.

package main

import (
	"github.com/Knatte18/loomyard/internal/heddleengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// readHeddleStanding returns the standing text heddle stores for a session of engine in strand role role:
// the passages appended to its system prompt and the entries rendered into its engine's instruction file.
// Both keep heddle's render order.
func readHeddleStanding(boltDir, engine, role string) (shuttleengine.Standing, error) {
	renderer := heddleengine.Renderer{}
	passages, err := renderer.Passages(boltDir, engine, role)
	if err != nil {
		return shuttleengine.Standing{}, err
	}
	fileEntries, err := renderer.FileEntries(boltDir, engine)
	if err != nil {
		return shuttleengine.Standing{}, err
	}
	toShuttle := func(entries []heddleengine.Passage) []shuttleengine.Passage {
		converted := make([]shuttleengine.Passage, len(entries))
		for i, entry := range entries {
			converted[i] = shuttleengine.Passage(entry)
		}
		return converted
	}
	return shuttleengine.Standing{Passages: toShuttle(passages), FileEntries: toShuttle(fileEntries)}, nil
}
