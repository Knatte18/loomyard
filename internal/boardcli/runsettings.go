// runsettings.go — validation of a task payload's run_settings before the store write.

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
)

// prepareRunSettings validates and migrates the run_settings of one task payload in place, before the store write.
// A payload without the field passes untouched.
// The value must be an object of module name to keys; configreg.PrepareRunSettings migrates and validates it,
// and the migrated documents replace the payload's value so the board stores the migrated result.
// A change to the stored value is refused from a session whose strand name carries a task slug, and returns a notice when the stored status says a run is past Seed-Child.
// The slug guard reads outside the board's write lock and guards a mistaken write, not a determined one.
func prepareRunSettings(b *boardengine.Board, fields map[string]any, strandName string) (string, error) {
	value, present := fields["run_settings"]
	if !present {
		return "", nil
	}
	requested, isObject := value.(map[string]any)
	if value != nil && !isObject {
		return "", fmt.Errorf("run_settings must be an object of module name to keys, got %T", value)
	}
	documents, err := configreg.PrepareRunSettings(requested)
	if err != nil {
		return "", err
	}
	migrated := make(map[string]any, len(documents))
	for module, document := range documents {
		var keys map[string]any
		if err := yaml.Unmarshal(document, &keys); err != nil {
			return "", fmt.Errorf("run_settings for module %q: decode migrated keys: %w", module, err)
		}
		migrated[module] = keys
	}
	fields["run_settings"] = migrated

	slug, _ := fields["slug"].(string)
	stored, found, err := b.GetTask(slug)
	if err != nil {
		return "", err
	}
	if !found {
		stored = boardengine.Task{}
	}
	same, err := sameRunSettings(stored.RunSettings, migrated)
	if err != nil {
		return "", err
	}
	if same {
		return "", nil
	}
	if agentname.TaskSlug(strandName) != "" {
		return "", fmt.Errorf("a run_settings change is refused from the task session %s; way forward: the operator or the hub orch makes the change", strandName)
	}
	producer, running := boardengine.RunStatusProducer(stored.Status)
	if running && producer != battenrecipe.NameWorktreeCreate && producer != battenrecipe.NameSeedChild {
		return fmt.Sprintf("the run_settings change of %q applies to the next run's Seed-Child; the running pair takes it through `lyx config <module> --layer local --set` in the pair", slug), nil
	}
	return "", nil
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
