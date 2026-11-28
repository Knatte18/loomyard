// kscreen.go reads kscreen-doctor's JSON for the screen KDE ranks first.

package kwinplace

import (
	"encoding/json"
	"errors"
	"fmt"
)

// TilingOutput returns the name of the enabled output with the lowest priority in the JSON of `kscreen-doctor -j`.
// No enabled output is an error.
func TilingOutput(kscreenJSON []byte) (string, error) {
	var screens struct {
		Outputs []struct {
			Name     string `json:"name"`
			Enabled  bool   `json:"enabled"`
			Priority int    `json:"priority"`
		} `json:"outputs"`
	}
	if err := json.Unmarshal(kscreenJSON, &screens); err != nil {
		return "", fmt.Errorf("kwinplace: decode kscreen-doctor output: %w", err)
	}
	name, best := "", 0
	for _, output := range screens.Outputs {
		if output.Enabled && (name == "" || output.Priority < best) {
			name, best = output.Name, output.Priority
		}
	}
	if name == "" {
		return "", errors.New("kwinplace: kscreen-doctor reports no enabled output")
	}
	return name, nil
}
