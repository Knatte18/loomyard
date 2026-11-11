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

	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/slackengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestForwarding covers a hub with no forwarding file and slack buttons written on and off.
// It also covers a repeat of the stored state.
func TestForwarding(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	forwardingPath := func(t *testing.T) string {
		t.Helper()
		return filepath.Join(t.TempDir(), "slack", slackengine.ForwardingFileName)
	}

	t.Run("MissingFileHoldsNoOrchAndNoButtonIsOn", func(t *testing.T) {
		t.Parallel()
		path := forwardingPath(t)
		if orchs, err := slackengine.ReadForwarding(path); err != nil || len(orchs) != 0 {
			t.Errorf("ReadForwarding(missing) = %v, %v; want none", orchs, err)
		}
		if on, err := slackengine.ForwardingOn(path, "ly:orch"); err != nil || on {
			t.Errorf("ForwardingOn(missing) = %v, %v; want off", on, err)
		}
	})

	t.Run("WritesReadBackSortedWithAnOffOrchLeftOutAndARepeatIsUnchanged", func(t *testing.T) {
		t.Parallel()
		path := forwardingPath(t)
		steps := []struct {
			orch        string
			on          bool
			wantChanged bool
			wantOn      []string
		}{
			{"ly:payments:orch", true, true, []string{"ly:payments:orch"}},
			{"ly:orch", true, true, []string{"ly:orch", "ly:payments:orch"}},
			{"ly:orch", true, false, []string{"ly:orch", "ly:payments:orch"}},
			{"ly:payments:orch", false, true, []string{"ly:orch"}},
			{"ly:payments:orch", false, false, []string{"ly:orch"}},
			{"ly:other:orch", false, false, []string{"ly:orch"}},
		}
		for i, step := range steps {
			changed, err := slackengine.WriteForwarding(path, step.orch, step.on, now)
			if err != nil || changed != step.wantChanged {
				t.Fatalf("WriteForwarding #%d (%s, %v) = %v, %v; want changed %v", i+1, step.orch, step.on, changed, err, step.wantChanged)
			}
			orchs, err := slackengine.ReadForwarding(path)
			if err != nil {
				t.Fatalf("ReadForwarding after write #%d = %v", i+1, err)
			}
			if diff := cmp.Diff(step.wantOn, orchs); diff != "" {
				t.Errorf("ReadForwarding after write #%d mismatch (-want +got):\n%s", i+1, diff)
			}
			if on, err := slackengine.ForwardingOn(path, step.orch); err != nil || on != step.on {
				t.Errorf("ForwardingOn(%s) after write #%d = %v, %v; want %v", step.orch, i+1, on, err, step.on)
			}
		}
	})

	t.Run("UndecodableFileIsAnErrorNamingIt", func(t *testing.T) {
		t.Parallel()
		path := forwardingPath(t)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatalf("MkdirAll = %v", err)
		}
		if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
			t.Fatalf("WriteFile = %v", err)
		}
		if _, err := slackengine.ReadForwarding(path); err == nil || !strings.Contains(err.Error(), path) {
			t.Errorf("ReadForwarding({) = %v; want an error naming %s", err, path)
		}
		if on, err := slackengine.ForwardingOn(path, "ly:orch"); err == nil || on {
			t.Errorf("ForwardingOn({) = %v, %v; want an error and off", on, err)
		}
	})
}

// TestConnected covers a machine whose socket lock is held, one whose lock is free and one with no credential directory.
func TestConnected(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	t.Run("MissingDirectoryReadsDisconnected", func(t *testing.T) {
		t.Parallel()
		geom := newGeometry(t)
		if connected, err := slackengine.Connected(geom.MachinePaths()); err != nil || connected {
			t.Errorf("Connected(no directory) = %v, %v; want disconnected", connected, err)
		}
	})

	t.Run("HeldLockReadsConnectedAndFreeLockReadsDisconnectedAndStaysFree", func(t *testing.T) {
		t.Parallel()
		geom := newGeometry(t)
		if err := os.MkdirAll(filepath.Dir(geom.SocketLockPath), 0o700); err != nil {
			t.Fatalf("MkdirAll = %v", err)
		}
		held, acquired, err := lock.TryAcquireWriteLock(geom.SocketLockPath)
		if err != nil || !acquired {
			t.Fatalf("TryAcquireWriteLock = %v, %v; want the lock", acquired, err)
		}
		if connected, err := slackengine.Connected(geom.MachinePaths()); err != nil || !connected {
			t.Errorf("Connected(lock held) = %v, %v; want connected", connected, err)
		}
		if err := held.Release(); err != nil {
			t.Fatalf("Release = %v", err)
		}
		if connected, err := slackengine.Connected(geom.MachinePaths()); err != nil || connected {
			t.Errorf("Connected(lock free) = %v, %v; want disconnected", connected, err)
		}
		again, acquired, err := lock.TryAcquireWriteLock(geom.SocketLockPath)
		if err != nil || !acquired {
			t.Fatalf("TryAcquireWriteLock after Connected = %v, %v; want the lock left free", acquired, err)
		}
		if err := again.Release(); err != nil {
			t.Errorf("Release = %v", err)
		}
	})
}

// TestPresence covers a machine with no record, a change and a repeat of the stored value.
// It also covers a stored value neither presence names.
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
		if err := slackengine.SetOperator(path, "U789"); err != nil {
			t.Fatalf("SetOperator = %v", err)
		}
		wantKept := slackengine.Setup{ChannelID: "C123", OperatorID: "U789"}
		if got, found, err := slackengine.ReadSetup(path); err != nil || !found || got != wantKept {
			t.Errorf("ReadSetup after SetOperator = %+v, %v, %v; want %+v with the channel kept", got, found, err, wantKept)
		}
		olderPath := filepath.Join(t.TempDir(), "slack", slackengine.SetupFileName)
		if err := os.MkdirAll(filepath.Dir(olderPath), 0o700); err != nil {
			t.Fatalf("MkdirAll = %v", err)
		}
		if err := os.WriteFile(olderPath, []byte(`{"channel_id":"C9","newer_key":"kept"}`), 0o600); err != nil {
			t.Fatalf("WriteFile = %v", err)
		}
		if err := slackengine.SetOperator(olderPath, "U1"); err != nil {
			t.Fatalf("SetOperator(older file) = %v", err)
		}
		raw, err := os.ReadFile(olderPath)
		if err != nil {
			t.Fatalf("ReadFile = %v", err)
		}
		var keys map[string]string
		wantKeys := map[string]string{"channel_id": "C9", "newer_key": "kept", "operator_id": "U1"}
		if err := json.Unmarshal(raw, &keys); err != nil || !cmp.Equal(keys, wantKeys) {
			t.Errorf("setup file after SetOperator = %s (%v); want the keys %v", raw, err, wantKeys)
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
