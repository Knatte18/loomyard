// credentials.go holds the machine's credential file and the hub's setup file, and the check that both are complete.
// The credential file holds the bot token, the app-level token and the id of the Slack app they belong to.

package slackengine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/state"
)

// Credentials is the machine's Slack credential file: the two tokens and the app they belong to.
type Credentials struct {
	// BotToken is the bot token (`xoxb-`), which makes the Web API calls.
	// It stays under the key `token`, so an older binary still reads it.
	BotToken string `json:"token"`
	// AppToken is the app-level token (`xapp-`), which opens the Socket Mode connection.
	AppToken string `json:"app_token"`
	// AppID is the id of the Slack app both tokens belong to.
	AppID string `json:"app_id"`
}

// Missing names each token kind the credentials lack, the bot token first.
func (c Credentials) Missing() []string {
	var missing []string
	if c.BotToken == "" {
		missing = append(missing, "bot token (xoxb-)")
	}
	if c.AppToken == "" {
		missing = append(missing, "app-level token (xapp-)")
	}
	return missing
}

// Setup is the hub's Slack setup: the channel the relay reads and posts in and the one operator whose messages it forwards.
type Setup struct {
	// ChannelID is the Slack id of the channel.
	ChannelID string `json:"channel_id"`
	// OperatorID is the Slack user id of the operator.
	OperatorID string `json:"operator_id"`
}

// ReadCredentials returns the credentials stored at path, and zero credentials for a missing file.
func ReadCredentials(path string) (Credentials, error) {
	credentials, _, err := state.ReadJSON[Credentials](path, recordLockPath(path))
	if err != nil {
		return Credentials{}, fmt.Errorf("slack: read the credential file %s: %w", path, err)
	}
	return credentials, nil
}

// WriteCredentials stores credentials at path atomically, creating the directory private to the user.
// The file is private to its user too, as the atomic write creates it.
func WriteCredentials(path string, credentials Credentials) error {
	if err := os.MkdirAll(filepath.Dir(path), privateDirMode); err != nil {
		return fmt.Errorf("slack: create the directory of the credential file %s: %w", path, err)
	}
	if err := state.WriteJSON(path, recordLockPath(path), credentials); err != nil {
		return fmt.Errorf("slack: write the credential file %s: %w", path, err)
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

// RequireConfigured returns the machine's credentials and the setup of the hub in geom.
// It refuses a machine missing either token and a hub with no setup file.
// Each refusal names the `lym slack` verb that writes the missing piece as the way forward.
func RequireConfigured(geom Geometry) (Credentials, Setup, error) {
	credentials, err := ReadCredentials(geom.TokenPath)
	if err != nil {
		return Credentials{}, Setup{}, err
	}
	if missing := credentials.Missing(); len(missing) > 0 {
		return Credentials{}, Setup{}, fmt.Errorf(
			"slack: this machine lacks the %s; way forward: run `lym slack login` from the operator's terminal and give both tokens in one run",
			strings.Join(missing, " and the "))
	}
	setup, found, err := ReadSetup(geom.SetupPath)
	if err != nil {
		return Credentials{}, Setup{}, err
	}
	if !found {
		return Credentials{}, Setup{}, errors.New("slack: the hub's Slack operator is not set; way forward: run `lym slack setup --operator <user-id>` from the operator's terminal")
	}
	return credentials, setup, nil
}
