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

// prepareRunSettings validates the run_settings and profile of one task payload before the store write.
// It migrates run_settings in place, so the board stores the migrated value.
// A payload with neither field passes untouched.
// run_settings must be an object of module name to keys, and profile must be a string;
// a non-empty profile must name a run profile that loadProfiles returns.
// A session that may not change either stored value is refused, and the check reads outside the board's write lock:
// it guards a mistaken write, not a determined one.
// It returns a notice when the entry's stored status says a run is past Seed-Child.
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
	if err := refuseRunSettingsCaller(b, strandName, fields, stored); err != nil {
		return "", err
	}
	producer, running := boardengine.RunStatusProducer(stored.Status)
	if running && producer != battenrecipe.NameWorktreeCreate && producer != battenrecipe.NameSeedChild {
		return fmt.Sprintf("the run_settings or profile change of %q applies to the next run's Seed-Child; the running pair takes it through `lym config <module> --layer local --set` in the pair", slug), nil
	}
	return "", nil
}

// refuseRunSettingsCaller refuses a run_settings or profile change on one entry from a session that may not make it.
// A session whose strand name carries no slug, the operator's or the hub orch's, may.
// A beam's orch, whose strand name carries the beam's slug and the orch role, may make the change.
// It may do so only when the entry belongs to that beam both as stored and under the project the payload names;
// when the payload names none, the stored project counts.
// The beam's orch may set the profile of the tasks it files under its beam, but it moves no change past the beam's edge.
// Its refusal names the orch of the beam's parent as the way forward, since that orch decides where the work goes.
// Every other session whose strand name carries a slug is refused.
// stored is the zero entry when the board does not yet hold the entry.
func refuseRunSettingsCaller(b *boardengine.Board, strandName string, fields map[string]any, stored boardengine.Task) error {
	name, err := agentname.Parse(strandName)
	if err != nil || name.Slug == "" {
		return nil
	}
	refusal := fmt.Errorf("a run_settings or profile change is refused from the session %s, which is no orch of the entry's beam; way forward: the operator, the hub orch or the orch of the entry's beam makes the change", strandName)
	if !agentname.MatchesRole(name.Role, agentname.RoleOrch) {
		return refusal
	}
	project := stored.Project
	if value, present := fields["project"]; present {
		project, _ = value.(string)
	}
	links := []string{project}
	if stored.Slug != "" && stored.Project != project {
		links = append(links, stored.Project)
	}
	for _, link := range links {
		beam, err := b.BeamUnder(link)
		if err != nil {
			return err
		}
		if beam != name.Slug {
			return refuseBeamOrchOutsideBeam(b, strandName, name)
		}
	}
	return nil
}

// refuseBeamOrchOutsideBeam returns the refusal of a change a beam's orch makes outside its own beam.
// A beam's orch writes the board only under its beam,
// so the way forward is a wish, what to make and why, sent through creel to the orch of the beam's parent.
// That orch places the work, the prime's orch when the beam's parent is the prime.
func refuseBeamOrchOutsideBeam(b *boardengine.Board, strandName string, name agentname.Name) error {
	parentBeam, err := b.ParentBeamOf(name.Slug)
	if err != nil {
		return err
	}
	parentOrch := agentname.Name{Shortname: name.Shortname, Slug: parentBeam, Role: agentname.RoleOrch}.String()
	return fmt.Errorf("a run_settings or profile change is refused from the session %s: a beam's orch writes only under its own beam %q; "+
		"way forward: pass the wish, what to make and why, to the beam's parent orch with `lym creel post %s --expect-reply \"<wish>\"`, and it decides where the work goes",
		strandName, name.Slug, parentOrch)
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

// sameRunSettings reports whether two run settings values are equal as the board stores them;
// an empty value equals an absent one.
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
