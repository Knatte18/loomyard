// beamend.go holds the board write that ends a beam's life without a landing, leaving a plain project.

package boardengine

// EndBeam clears the beam flag of the project slug names, under the write lock.
// It ends the life of a beam whose pair is gone with no landing, which would otherwise keep a flag no write can clear.
// The pawl recipe, the owner machine and the colour that only a beam carries go with the flag.
// The project keeps its status, its order and its members, whose project field is untouched.
// It reports whether it cleared a flag, and it writes nothing for an absent entry or one that is no beam.
func (b *Board) EndBeam(slug string) (bool, error) {
	result, err := b.boardCriticalSection(func(store *Store) (any, error) {
		i, found := store.index(slug)
		if !found || store.tasks[i].Kind != KindProject || !store.tasks[i].Beam {
			return noWrite{result: false}, nil
		}
		entry := &store.tasks[i]
		entry.Beam = false
		entry.Recipe = ""
		entry.OwnerMachine = ""
		entry.BeamColor = ""
		return true, nil
	}, nil)
	if err != nil {
		return false, err
	}
	return result.(bool), nil
}
