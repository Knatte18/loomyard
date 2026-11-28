// runsettings.go holds Seed-Child's prepare step.
// It lays a board task's run settings over its run profile's block for the task's recipe,
// and it marks each refusal for the Stuck route.

package battencli

import (
	"errors"
	"fmt"

	"github.com/Knatte18/loomyard/internal/battenshed"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/configreg"
	"github.com/Knatte18/loomyard/internal/runprofile"
)

// prepareRunSettings returns the local-layer document of each module for a board task's run settings.
// Each document lays the task's keys over the block its run profile holds for the task's recipe.
// The profile is the task's own, else that recipe's default from the run profiles under dirs;
// with neither, the task's settings stand alone.
// A refusal wraps battenshed.ErrInvalidRunSettings.
// It covers an unknown profile, a profile with no block for the recipe, or a profiles file that does not decode.
// A default held only under the recipe's former name is refused the same way.
// So is a refusal of the profile's or the task's settings.
// A profiles file that cannot be read is returned as it is.
func prepareRunSettings(dirs configengine.Dirs, settings battenshed.RunSettings) (map[string][]byte, error) {
	profiles, err := runprofile.Load(dirs)
	if errors.Is(err, configengine.ErrInvalid) {
		return nil, fmt.Errorf("%w: %s", battenshed.ErrInvalidRunSettings, err.Error())
	}
	if err != nil {
		return nil, err
	}
	recipe := settings.Recipe
	name := settings.Profile
	if name == "" {
		name, err = profiles.Default(recipe)
		if err != nil {
			return nil, fmt.Errorf("%w: %s", battenshed.ErrInvalidRunSettings, err.Error())
		}
	}
	var documents map[string][]byte
	if name == "" {
		documents, err = configreg.PrepareRunSettings(settings.Settings)
	} else {
		profile, lookupErr := profiles.Settings(name, recipe)
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
