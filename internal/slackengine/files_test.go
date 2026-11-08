// files_test.go covers the presence record and the token and setup files over t.TempDir paths.

package slackengine_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/Knatte18/loomyard/internal/slackengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestPresence covers a hub with no record, a change, a repeat of the stored value and a stored value neither presence names.
func TestPresence(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)

	t.Run("MissingFileReadsTerminal", func(t *testing.T) {
		t.Parallel()
		got, err := slackengine.ReadPresence(filepath.Join(t.TempDir(), slackengine.PresenceFileName))
		if err != nil || got != slackengine.PresenceTerminal {
			t.Errorf("ReadPresence(missing) = %q, %v; want %q", got, err, slackengine.PresenceTerminal)
		}
	})

	t.Run("WriteReadsBackAndASecondWriteIsUnchanged", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(t.TempDir(), slackengine.PresenceFileName)
		for i, wantChanged := range []bool{true, false} {
			changed, err := slackengine.WritePresence(path, slackengine.PresenceSlack, now)
			if err != nil || changed != wantChanged {
				t.Fatalf("WritePresence(slack) #%d = %v, %v; want changed %v", i+1, changed, err, wantChanged)
			}
			got, err := slackengine.ReadPresence(path)
			if err != nil || got != slackengine.PresenceSlack {
				t.Errorf("ReadPresence after write #%d = %q, %v; want %q", i+1, got, err, slackengine.PresenceSlack)
			}
		}
	})

	t.Run("UnknownStoredValueIsAnErrorNamingTheFile", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(t.TempDir(), slackengine.PresenceFileName)
		if err := os.WriteFile(path, []byte(`{"presence":"away"}`), 0o600); err != nil {
			t.Fatalf("WriteFile = %v", err)
		}
		if got, err := slackengine.ReadPresence(path); err == nil || !strings.Contains(err.Error(), path) {
			t.Errorf("ReadPresence(unknown value) = %q, %v; want an error naming %s", got, err, path)
		}
		if changed, err := slackengine.WritePresence(path, slackengine.PresenceSlack, now); err == nil || changed {
			t.Errorf("WritePresence over an unknown value = %v, %v; want an error and no change", changed, err)
		}
	})
}

// TestCredentialFiles covers the credential and setup files, their modes and the configured check.
func TestCredentialFiles(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	t.Run("CredentialsReadBackWithPrivateModesAndTheBotTokenUnderToken", func(t *testing.T) {
		t.Parallel()
		dir := filepath.Join(t.TempDir(), "credentials")
		path := filepath.Join(dir, slackengine.TokenFileName)
		want := slackengine.Credentials{BotToken: "xoxb-secret", AppToken: "xapp-secret", AppID: "A1"}
		if err := slackengine.WriteCredentials(path, want); err != nil {
			t.Fatalf("WriteCredentials = %v", err)
		}
		got, err := slackengine.ReadCredentials(path)
		if err != nil || got != want {
			t.Errorf("ReadCredentials = %+v, %v; want %+v", got, err, want)
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile = %v", err)
		}
		var keys map[string]string
		if err := json.Unmarshal(raw, &keys); err != nil || keys["token"] != "xoxb-secret" {
			t.Errorf("credential file = %s (%v); want the bot token under the key token", raw, err)
		}
		if runtime.GOOS != "windows" {
			assertMode(t, path, 0o600)
			assertMode(t, dir, 0o700)
		}
	})

	t.Run("MissingFileReadsZeroCredentialsMissingBothTokens", func(t *testing.T) {
		t.Parallel()
		got, err := slackengine.ReadCredentials(filepath.Join(t.TempDir(), slackengine.TokenFileName))
		if err != nil || got != (slackengine.Credentials{}) {
			t.Errorf("ReadCredentials(missing) = %+v, %v; want zero credentials", got, err)
		}
		if diff := cmp.Diff([]string{"bot token (xoxb-)", "app-level token (xapp-)"}, got.Missing()); diff != "" {
			t.Errorf("Missing() mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("SetupReadsBackWithAPrivateMode", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(t.TempDir(), "slack", slackengine.SetupFileName)
		want := slackengine.Setup{ChannelID: "C123", OperatorID: "U456"}
		if err := slackengine.WriteSetup(path, want); err != nil {
			t.Fatalf("WriteSetup = %v", err)
		}
		got, found, err := slackengine.ReadSetup(path)
		if err != nil || !found || got != want {
			t.Errorf("ReadSetup = %+v, %v, %v; want %+v found", got, found, err, want)
		}
		if runtime.GOOS != "windows" {
			assertMode(t, path, 0o600)
		}
	})

	t.Run("RequireConfiguredNamesTheVerbThatWritesWhatIsMissing", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		geom := slackengine.Geometry{
			TokenPath: filepath.Join(root, "credentials", slackengine.TokenFileName),
			SetupPath: filepath.Join(root, "slack", slackengine.SetupFileName),
		}
		const botKind, appKind = "bot token (xoxb-)", "app-level token (xapp-)"
		tokenRows := []struct {
			name        string
			credentials slackengine.Credentials
			wantNamed   []string
			wantAbsent  []string
		}{
			{"NoTokens", slackengine.Credentials{}, []string{botKind, appKind}, nil},
			{"BotTokenAlone", slackengine.Credentials{BotToken: "xoxb-secret"}, []string{appKind}, []string{botKind}},
			{"AppTokenAlone", slackengine.Credentials{AppToken: "xapp-secret"}, []string{botKind}, []string{appKind}},
		}
		for _, row := range tokenRows {
			if err := slackengine.WriteCredentials(geom.TokenPath, row.credentials); err != nil {
				t.Fatalf("WriteCredentials = %v", err)
			}
			_, _, err := slackengine.RequireConfigured(geom)
			if err == nil || !strings.Contains(err.Error(), "lym slack login") {
				t.Fatalf("RequireConfigured(%s) = %v; want an error naming lym slack login", row.name, err)
			}
			for _, kind := range row.wantNamed {
				if !strings.Contains(err.Error(), kind) {
					t.Errorf("RequireConfigured(%s) = %v; want it to name the %s", row.name, err, kind)
				}
			}
			for _, kind := range row.wantAbsent {
				if strings.Contains(err.Error(), kind) {
					t.Errorf("RequireConfigured(%s) = %v; want it not to name the %s", row.name, err, kind)
				}
			}
		}

		bothTokens := slackengine.Credentials{BotToken: "xoxb-secret", AppToken: "xapp-secret", AppID: "A1"}
		if err := slackengine.WriteCredentials(geom.TokenPath, bothTokens); err != nil {
			t.Fatalf("WriteCredentials = %v", err)
		}
		if _, _, err := slackengine.RequireConfigured(geom); err == nil || !strings.Contains(err.Error(), "lym slack setup --operator") {
			t.Errorf("RequireConfigured(no setup) = %v; want an error naming lym slack setup --operator", err)
		}
		want := slackengine.Setup{ChannelID: "C123", OperatorID: "U456"}
		if err := slackengine.WriteSetup(geom.SetupPath, want); err != nil {
			t.Fatalf("WriteSetup = %v", err)
		}
		credentials, setup, err := slackengine.RequireConfigured(geom)
		if err != nil || credentials != bothTokens || setup != want {
			t.Errorf("RequireConfigured(both) = %+v, %+v, %v; want %+v and %+v", credentials, setup, err, bothTokens, want)
		}
	})
}

// assertMode fails the test when the file or directory at path has permission bits other than want.
func assertMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat(%q) = %v", path, err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Errorf("mode of %q = %o; want %o", path, got, want)
	}
}
