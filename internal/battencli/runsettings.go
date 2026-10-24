// runsettings.go — Seed-Child's prepare step: a board task's run settings laid over its run profile's block for the task's recipe, with each refusal marked for the Stuck route.

package battencli

import (
	"errors"
	"fmt"

	"github.com/Knatte18/loomyard/internal/battenshed"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/configreg"
	"github.com/Knatte18/loomyard/internal/runprofile"
)

// prepareRunSettings returns the local-layer document of each module a board task's run settings and its run profile's block for the task's recipe name, the task's keys laid over the profile's.
// The profile is the task's own, else the recipe's default from the run profiles under dirs; with neither, the task's settings stand alone.
// An unknown profile, a profile holding no block for the recipe, a profiles file that does not decode, or a refusal of the profile's or the task's settings, wraps battenshed.ErrInvalidRunSettings;
// a profiles file that cannot be read is returned as it is.
func prepareRunSettings(dirs configengine.Dirs, settings battenshed.RunSettings) (map[string][]byte, error) {
	profiles, err := runprofile.Load(dirs)
	if errors.Is(err, configengine.ErrInvalid) {
		return nil, fmt.Errorf("%w: %s", battenshed.ErrInvalidRunSettings, err.Error())
	}
	if err != nil {
		return nil, err
	}
	name := settings.Profile
	if name == "" {
		name = profiles.Default(settings.Recipe)
	}
	var documents map[string][]byte
	if name == "" {
		documents, err = configreg.PrepareRunSettings(settings.Settings)
	} else {
		profile, lookupErr := profiles.Settings(name, settings.Recipe)
		if lookupErr != nil {
			return nil, fmt.Errorf("%w: %s", battenshed.ErrInvalidRunSettings, lookupErr.Error())
		}
		documents, err = configreg.LayerRunSettings(name, profile, settings.Settings)
	}
	var refusal *configreg.RunSettingsError
	if errors.As(err, &refusal) {
		return nil, fmt.Errorf("%w: %s", battenshed.ErrInvalidRunSettings, err.Error())
	}
	return documents, err
}
