// bridgerecords_test.go covers the peers record and the bridge map over t.TempDir paths.

package slackengine_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/Knatte18/loomyard/internal/slackengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestBridgeRecords(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	approvedAt := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	peer := slackengine.Peer{AppID: "A1", BotUserID: "U1", ApprovedAt: approvedAt}
	tests := []struct {
		name string
		// path is the record's file name in a fresh directory.
		path string
		run  func(t *testing.T, path string)
	}{
		{"PeersMissingFileReadsAsNone", slackengine.PeersFileName, func(t *testing.T, path string) {
			peers, err := slackengine.ReadPeers(path)
			if err != nil || len(peers.Machines) != 0 {
				t.Errorf("ReadPeers(missing) = %+v, %v; want no peers", peers, err)
			}
		}},
		{"PeersUpdateWritesTheMachineAndKeepsAKeyTheRecordDoesNotKnow", slackengine.PeersFileName, func(t *testing.T, path string) {
			if err := os.WriteFile(path, []byte(`{"machines":{"osl":{"app_id":"A0"}},"future":{"kept":true}}`), 0o600); err != nil {
				t.Fatalf("WriteFile = %v", err)
			}
			err := slackengine.UpdatePeers(path, func(peers *slackengine.Peers) error {
				peers.Machines["neve"] = peer
				return nil
			})
			if err != nil {
				t.Fatalf("UpdatePeers = %v", err)
			}
			got, err := slackengine.ReadPeers(path)
			want := slackengine.Peers{Machines: map[string]slackengine.Peer{"osl": {AppID: "A0"}, "neve": peer}}
			if err != nil || !cmp.Equal(got, want) {
				t.Errorf("ReadPeers = %+v, %v; want %+v", got, err, want)
			}
			if raw, _ := os.ReadFile(path); !strings.Contains(string(raw), `"future"`) {
				t.Errorf("record after the update = %s; want the key it does not know kept", raw)
			}
		}},
		{"BridgeMapMissingFileReadsAsEmpty", slackengine.BridgeMapFileName, func(t *testing.T, path string) {
			bridgeMap, err := slackengine.ReadBridgeMap(path)
			if err != nil || len(bridgeMap.Channels) != 0 {
				t.Errorf("ReadBridgeMap(missing) = %+v, %v; want an empty map", bridgeMap, err)
			}
		}},
		{"BridgeMapUpdateWritesTheEntryAndKeepsAKeyTheRecordDoesNotKnow", slackengine.BridgeMapFileName, func(t *testing.T, path string) {
			if err := os.WriteFile(path, []byte(`{"channels":{"C1":{"name":"old"}},"future":{"kept":true}}`), 0o600); err != nil {
				t.Fatalf("WriteFile = %v", err)
			}
			entry := slackengine.BridgeEntry{Name: "bridge-a--b", LocalOrch: "ly:orch", Peer: "osl::zz:orch", PeerInvited: true, Cursor: "1.0"}
			err := slackengine.UpdateBridgeMap(path, func(bridgeMap *slackengine.BridgeMap) error {
				bridgeMap.Channels["C2"] = entry
				return nil
			})
			if err != nil {
				t.Fatalf("UpdateBridgeMap = %v", err)
			}
			got, err := slackengine.ReadBridgeMap(path)
			want := slackengine.BridgeMap{Channels: map[string]slackengine.BridgeEntry{"C1": {Name: "old"}, "C2": entry}}
			if err != nil || !cmp.Equal(got, want) {
				t.Errorf("ReadBridgeMap = %+v, %v; want %+v", got, err, want)
			}
			if raw, _ := os.ReadFile(path); !strings.Contains(string(raw), `"future"`) {
				t.Errorf("record after the update = %s; want the key it does not know kept", raw)
			}
		}},
		{"ChannelOfFindsTheOpenEntryOfAPairAndSkipsAnArchivedOne", slackengine.BridgeMapFileName, func(t *testing.T, _ string) {
			bridgeMap := slackengine.BridgeMap{Channels: map[string]slackengine.BridgeEntry{
				"C1": {LocalOrch: "ly:orch", Peer: "osl::zz:orch", Archived: true},
				"C2": {LocalOrch: "ly:orch", Peer: "osl::zz:orch"},
				"C3": {LocalOrch: "ly:orch", Peer: "osl::yy:orch"},
				"C4": {LocalOrch: "ly:pay:orch", Peer: "osl::zz:orch"},
			}}
			if id, ok := bridgeMap.ChannelOf("ly:orch", "osl::zz:orch"); !ok || id != "C2" {
				t.Errorf("ChannelOf(ly:orch, osl::zz:orch) = %q, %v; want the open C2", id, ok)
			}
			bridgeMap.Channels["C2"] = slackengine.BridgeEntry{LocalOrch: "ly:orch", Peer: "osl::zz:orch", Archived: true}
			if id, ok := bridgeMap.ChannelOf("ly:orch", "osl::zz:orch"); ok {
				t.Errorf("ChannelOf over only archived entries = %q, true; want none", id)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			tt.run(t, filepath.Join(dir, tt.path))
		})
	}
}
