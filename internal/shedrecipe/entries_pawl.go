// entries_pawl.go implements beamUpEntry, beamAwaitEntry and beamDownEntry, the Constructors of the pawl recipe's Go producers, each built over Env.Pawl.

package shedrecipe

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/pawlengine"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// pawlPathFields are the Paths fields every pawl producer that touches the run's files requires.
var pawlPathFields = []string{"Paths.LandRequest", "Paths.DobbyStopMarker", "Paths.AwaitMarker"}

// requirePawl refuses the first of fields that Env.Pawl leaves unset, naming it in its dotted `Pawl.<Field>` form on behalf of entry.
// A string field must be non-empty and every other field non-nil.
func requirePawl(entry string, deps pawlengine.Deps, fields ...string) error {
	values := map[string]any{
		"Slug":                  deps.Slug,
		"RunID":                 deps.RunID,
		"Paths.LandRequest":     deps.Paths.LandRequest,
		"Paths.DobbyStopMarker": deps.Paths.DobbyStopMarker,
		"Paths.AwaitMarker":     deps.Paths.AwaitMarker,
		"SetStatus":             deps.SetStatus,
		"EntryProducer":         deps.EntryProducer,
		"OwnedElsewhere":        deps.OwnedElsewhere,
		"MarkBeam":              deps.MarkBeam,
		"SpawnPush":             deps.SpawnPush,
		"RegenerateWorkspace":   deps.RegenerateWorkspace,
		"StopDobby":             deps.StopDobby,
		"StartDobby":            deps.StartDobby,
		"DobbyLive":             deps.DobbyLive,
		"Recheck":               deps.Recheck,
		"WriteSnapshot":         deps.WriteSnapshot,
		"Notify":                deps.Notify,
		"Down":                  deps.Down,
		"Now":                   deps.Now,
		"Sleep":                 deps.Sleep,
	}
	for _, field := range fields {
		value, known := values[field]
		if !known {
			panic(fmt.Sprintf("shedrecipe: requirePawl: unknown Pawl field %q", field))
		}
		if text, isString := value.(string); isString {
			if err := requireNonEmpty(entry, "Pawl."+field, text); err != nil {
				return err
			}
			continue
		}
		if err := requireSeam(entry, "Pawl."+field, value); err != nil {
			return err
		}
	}
	return nil
}

// beamUpEntry is the Constructor for the "BeamUp" registry row: it rejects every config key, requires the Env.Pawl fields Beam-Up uses, and returns pawlengine.NewBeamUp over Env.Pawl, wrapping that constructor's own error, a step with no name or function, with this package's error-text prefix.
func beamUpEntry(row RowIdentity, cfg Config, env Env) (shedengine.ShedProducer, error) {
	if err := configRejectUnknown(cfg); err != nil {
		return nil, err
	}
	if err := requirePawl("BeamUp", env.Pawl, append(append([]string{}, pawlPathFields...), "MarkBeam", "SpawnPush", "RegenerateWorkspace")...); err != nil {
		return nil, err
	}
	producer, err := pawlengine.NewBeamUp(row.Name, env.Pawl)
	if err != nil {
		return nil, fmt.Errorf("shedrecipe: BeamUp: Env.Pawl: %w", err)
	}
	return producer, nil
}

// beamAwaitEntry is the Constructor for the "BeamAwait" registry row: it rejects every config key, requires the Env.Pawl fields Beam-Await uses, and returns pawlengine.NewBeamAwait over Env.Pawl.
func beamAwaitEntry(row RowIdentity, cfg Config, env Env) (shedengine.ShedProducer, error) {
	if err := configRejectUnknown(cfg); err != nil {
		return nil, err
	}
	fields := append(append([]string{}, pawlPathFields...),
		"Slug", "RunID", "SetStatus", "EntryProducer", "OwnedElsewhere", "StopDobby", "StartDobby",
		"DobbyLive", "Recheck", "WriteSnapshot", "Notify", "Now", "Sleep")
	if err := requirePawl("BeamAwait", env.Pawl, fields...); err != nil {
		return nil, err
	}
	producer, err := pawlengine.NewBeamAwait(row.Name, env.Pawl)
	if err != nil {
		return nil, fmt.Errorf("shedrecipe: BeamAwait: Env.Pawl: %w", err)
	}
	return producer, nil
}

// beamDownEntry is the Constructor for the "BeamDown" registry row: it rejects every config key, requires Env.Pawl.Down, and returns pawlengine.NewBeamDown over Env.Pawl.
func beamDownEntry(row RowIdentity, cfg Config, env Env) (shedengine.ShedProducer, error) {
	if err := configRejectUnknown(cfg); err != nil {
		return nil, err
	}
	if err := requirePawl("BeamDown", env.Pawl, "Down"); err != nil {
		return nil, err
	}
	producer, err := pawlengine.NewBeamDown(row.Name, env.Pawl)
	if err != nil {
		return nil, fmt.Errorf("shedrecipe: BeamDown: Env.Pawl: %w", err)
	}
	return producer, nil
}
