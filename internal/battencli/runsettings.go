// runsettings.go — Seed-Child's prepare step: a board task's run settings laid over its run profile, with each refusal marked for the Stuck route.

package battencli

import (
	"errors"
	"fmt"

	"github.com/Knatte18/loomyard/internal/battenshed"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/configreg"
	"github.com/Knatte18/loomyard/internal/runprofile"
)

// prepareRunSettings returns the local-layer document of each module a board task's run settings and run profile name, the task's keys laid over the profile's.
// The profile is read from the run profiles under dirs only when the task names one.
// An unknown profile, a profiles file that does not decode, or a refusal of the profile's or the task's settings, wraps battenshed.ErrInvalidRunSettings;
// a profiles file that cannot be read is returned as it is.
func prepareRunSettings(dirs configengine.Dirs, settings battenshed.RunSettings) (map[string][]byte, error) {
	var documents map[string][]byte
	var err error
	if settings.Profile == "" {
		documents, err = configreg.PrepareRunSettings(settings.Settings)
	} else {
		profiles, loadErr := runprofile.Load(dirs)
		if errors.Is(loadErr, configengine.ErrInvalid) {
			return nil, fmt.Errorf("%w: %s", battenshed.ErrInvalidRunSettings, loadErr.Error())
		}
		if loadErr != nil {
			return nil, loadErr
		}
		profile, lookupErr := profiles.Lookup(settings.Profile)
		if lookupErr != nil {
			return nil, fmt.Errorf("%w: %s", battenshed.ErrInvalidRunSettings, lookupErr.Error())
		}
		documents, err = configreg.LayerRunSettings(settings.Profile, profile, settings.Settings)
	}
	var refusal *configreg.RunSettingsError
	if errors.As(err, &refusal) {
		return nil, fmt.Errorf("%w: %s", battenshed.ErrInvalidRunSettings, err.Error())
	}
	return documents, err
}
