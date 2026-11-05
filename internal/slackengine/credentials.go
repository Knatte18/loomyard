// credentials.go holds the machine's bot token file and the hub's setup file, and the check that both exist.

package slackengine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/state"
)

// tokenRecord is the token file's content.
type tokenRecord struct {
	Token string `json:"token"`
}

// Setup is the hub's Slack setup: the channel the relay reads and posts in and the one operator whose messages it forwards.
type Setup struct {
	// ChannelID is the Slack id of the channel.
	ChannelID string `json:"channel_id"`
	// OperatorID is the Slack user id of the operator.
	OperatorID string `json:"operator_id"`
}

// ReadToken returns the bot token stored at path.
// A missing file and an empty token report found false.
func ReadToken(path string) (string, bool, error) {
	record, found, err := state.ReadJSON[tokenRecord](path, recordLockPath(path))
	if err != nil {
		return "", false, fmt.Errorf("slack: read the token file %s: %w", path, err)
	}
	if !found || record.Token == "" {
		return "", false, nil
	}
	return record.Token, true, nil
}

// WriteToken stores token at path atomically, creating the directory private to the user.
// The file is private to its user too, as the atomic write creates it.
func WriteToken(path, token string) error {
	if err := os.MkdirAll(filepath.Dir(path), privateDirMode); err != nil {
		return fmt.Errorf("slack: create the directory of the token file %s: %w", path, err)
	}
	if err := state.WriteJSON(path, recordLockPath(path), tokenRecord{Token: token}); err != nil {
		return fmt.Errorf("slack: write the token file %s: %w", path, err)
	}
	return nil
}

// ReadSetup returns the setup stored at path, and reports found false for a missing file.
func ReadSetup(path string) (Setup, bool, error) {
	setup, found, err := state.ReadJSON[Setup](path, recordLockPath(path))
	if err != nil {
		return Setup{}, false, fmt.Errorf("slack: read the setup file %s: %w", path, err)
	}
	return setup, found, nil
}

// WriteSetup stores setup at path atomically, creating the directory private to the user.
func WriteSetup(path string, setup Setup) error {
	if err := os.MkdirAll(filepath.Dir(path), privateDirMode); err != nil {
		return fmt.Errorf("slack: create the directory of the setup file %s: %w", path, err)
	}
	if err := state.WriteJSON(path, recordLockPath(path), setup); err != nil {
		return fmt.Errorf("slack: write the setup file %s: %w", path, err)
	}
	return nil
}

// RequireConfigured returns the bot token and the setup of the hub in geom.
// It refuses a machine with no token and a hub with no setup file, each with the `lym slack` verb that writes it as the way forward.
func RequireConfigured(geom Geometry) (string, Setup, error) {
	token, found, err := ReadToken(geom.TokenPath)
	if err != nil {
		return "", Setup{}, err
	}
	if !found {
		return "", Setup{}, errors.New("slack: no Slack token on this machine; way forward: run `lym slack login` from the operator's terminal")
	}
	setup, found, err := ReadSetup(geom.SetupPath)
	if err != nil {
		return "", Setup{}, err
	}
	if !found {
		return "", Setup{}, errors.New("slack: the hub's Slack channel and operator are not set; way forward: run `lym slack setup --channel <id> --operator <user-id>` from the operator's terminal")
	}
	return token, setup, nil
}
