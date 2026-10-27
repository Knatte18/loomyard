// runsettings.go — validation of a task payload's run_settings and profile before the store write.

package boardcli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/battenrecipe"
	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/configreg"
	"github.com/Knatte18/loomyard/internal/runprofile"
)

// prepareRunSettings validates the run_settings and the profile of one task payload before the store write, migrating run_settings in place.
// A payload with neither field passes untouched.
// run_settings must be an object of module name to keys; configreg.PrepareRunSettings migrates and validates it,
// and the migrated documents replace the payload's value so the board stores the migrated result.
// profile must be a string, and a non-empty one must name a run profile loadProfiles returns.
// A change to either stored value is refused from a session whose strand name carries a task slug, and returns a notice when the stored status says a run is past Seed-Child.
// The slug guard reads outside the board's write lock and guards a mistaken write, not a determined one.
func prepareRunSettings(b *boardengine.Board, fields map[string]any, strandName string, loadProfiles func() (runprofile.Profiles, error)) (string, error) {
	settingsValue, settingsPresent := fields["run_settings"]
	profileValue, profilePresent := fields["profile"]
	if !settingsPresent && !profilePresent {
		return "", nil
	}
	var migrated map[string]any
	if settingsPresent {
		var err error
		if migrated, err = migrateRunSettings(settingsValue); err != nil {
			return "", err
		}
		fields["run_settings"] = migrated
	}
	profile, isString := profileValue.(string)
	if profilePresent && !isString {
		return "", fmt.Errorf("profile must be a string naming a run profile, got %T", profileValue)
	}
	if profile != "" {
		profiles, err := loadProfiles()
		if err != nil {
			return "", err
		}
		if _, err := profiles.Lookup(profile); err != nil {
			return "", err
		}
	}

	slug, _ := fields["slug"].(string)
	stored, found, err := b.GetTask(slug)
	if err != nil {
		return "", err
	}
	if !found {
		stored = boardengine.Task{}
	}
	changed := profilePresent && profile != stored.Profile
	if settingsPresent {
		same, err := sameRunSettings(stored.RunSettings, migrated)
		if err != nil {
			return "", err
		}
		changed = changed || !same
	}
	if !changed {
		return "", nil
	}
	if agentname.TaskSlug(strandName) != "" {
		return "", fmt.Errorf("a run_settings or profile change is refused from the task session %s; way forward: the operator or the hub orch makes the change", strandName)
	}
	producer, running := boardengine.RunStatusProducer(stored.Status)
	if running && producer != battenrecipe.NameWorktreeCreate && producer != battenrecipe.NameSeedChild {
		return fmt.Sprintf("the run_settings or profile change of %q applies to the next run's Seed-Child; the running pair takes it through `lym config <module> --layer local --set` in the pair", slug), nil
	}
	return "", nil
}

// migrateRunSettings returns a payload's run_settings value migrated and validated, as module name to keys.
func migrateRunSettings(value any) (map[string]any, error) {
	requested, isObject := value.(map[string]any)
	if value != nil && !isObject {
		return nil, fmt.Errorf("run_settings must be an object of module name to keys, got %T", value)
	}
	documents, err := configreg.PrepareRunSettings(requested)
	if err != nil {
		return nil, err
	}
	migrated := make(map[string]any, len(documents))
	for module, document := range documents {
		var keys map[string]any
		if err := yaml.Unmarshal(document, &keys); err != nil {
			return nil, fmt.Errorf("run_settings for module %q: decode migrated keys: %w", module, err)
		}
		migrated[module] = keys
	}
	return migrated, nil
}

// sameRunSettings reports whether two run settings values are equal as the board stores them, an empty value equal to an absent one.
func sameRunSettings(stored, requested map[string]any) (bool, error) {
	storedJSON, err := json.Marshal(stored)
	if err != nil {
		return false, err
	}
	requestedJSON, err := json.Marshal(requested)
	if err != nil {
		return false, err
	}
	if len(stored) == 0 && len(requested) == 0 {
		return true, nil
	}
	return bytes.Equal(storedJSON, requestedJSON), nil
}

// strandNameFromEnv returns the calling session's strand name.
func strandNameFromEnv() string {
	return os.Getenv(agentname.StrandNameEnv)
}
