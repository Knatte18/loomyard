// manifest_test.go covers the launch manifest record: its file, its verdict and its comparison.
// The copy a launch keeps is covered with the runner, in run_test.go.

package shuttleengine

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestManifest_Record(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	delivered := ManifestItem{Channel: ChannelGuard, Key: "question-deny", Delivery: DeliveryDelivered}
	native := ManifestItem{Channel: ChannelInstructionSource, Key: "/home/user/CLAUDE.md", Delivery: DeliveryNative}
	omitted := ManifestItem{Channel: ChannelPluginContext, Key: "plug/hook", Delivery: DeliveryOmitted, Reason: "the hook ran no command"}
	keyedToOne := ManifestItem{Channel: ChannelStandingPassage, Key: "claude-only", Engine: "claude", Delivery: DeliveryDelivered}
	settings := ManifestItem{Channel: ChannelSettings, Key: "settings", Engine: "claude", Delivery: DeliveryDelivered, Detail: "812 bytes"}

	t.Run("RoundTripKeepsTheVerdictOfTheOmittedItems", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name        string
			items       []ManifestItem
			wantVerdict []ManifestItem
		}{
			{"SomeOmitted", []ManifestItem{delivered, omitted, native}, []ManifestItem{omitted}},
			{"NoneOmitted", []ManifestItem{delivered, native}, nil},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				runDir := t.TempDir()
				written := Manifest{Engine: "claude", Role: "writer", PromptEngine: "claude", Items: tt.items}
				if err := WriteManifest(runDir, written); err != nil {
					t.Fatalf("WriteManifest() error: %v", err)
				}
				got, found, err := ReadManifest(filepath.Join(runDir, ManifestFileName))
				want := written
				want.Verdict = tt.wantVerdict
				if err != nil || !found || !reflect.DeepEqual(got, want) {
					t.Errorf("ReadManifest() = %+v, %v, %v; want %+v, true, nil", got, found, err, want)
				}
				if got := written.Omitted(); !reflect.DeepEqual(got, tt.wantVerdict) {
					t.Errorf("Omitted() = %+v; want exactly the omitted items %+v", got, tt.wantVerdict)
				}
			})
		}
	})

	t.Run("ARecordHoldingAnUnknownKeyStillReads", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(t.TempDir(), ManifestFileName)
		record := `{"engine": "codex", "role": "writer", "promptEngine": "codex", "items": [], "verdict": [], "addedLater": {"a": 1}}`
		if err := os.WriteFile(path, []byte(record), 0o644); err != nil {
			t.Fatal(err)
		}
		got, found, err := ReadManifest(path)
		if err != nil || !found || got.Engine != "codex" || got.Role != "writer" || got.PromptEngine != "codex" {
			t.Errorf("ReadManifest() = %+v, %v, %v; want the known fields and no error", got, found, err)
		}
	})

	t.Run("AnAbsentFileReadsNotFound", func(t *testing.T) {
		t.Parallel()
		got, found, err := ReadManifest(filepath.Join(t.TempDir(), ManifestFileName))
		if err != nil || found || !reflect.DeepEqual(got, Manifest{}) {
			t.Errorf("ReadManifest() = %+v, %v, %v; want the zero manifest, not found, nil", got, found, err)
		}
	})

	t.Run("MissingItems", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name string
			from []ManifestItem
			in   []ManifestItem
			want []ManifestItem
		}{
			{"AComparedItemAbsentFromInIsMissing", []ManifestItem{delivered, native}, []ManifestItem{native}, []ManifestItem{delivered}},
			{"AnItemHeldUnderAnotherDeliveryIsPresent", []ManifestItem{native}, []ManifestItem{{Channel: native.Channel, Key: native.Key, Delivery: DeliveryOmitted}}, nil},
			{"TheSameKeyUnderAnotherChannelIsMissing", []ManifestItem{delivered}, []ManifestItem{{Channel: ChannelNotice, Key: delivered.Key, Delivery: DeliveryDelivered}}, []ManifestItem{delivered}},
			{"AnItemKeyedToOneEngineIsSkipped", []ManifestItem{keyedToOne}, nil, nil},
			{"ASettingsItemIsSkipped", []ManifestItem{settings}, nil, nil},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				got := MissingItems(Manifest{Items: tt.from}, Manifest{Items: tt.in})
				if !reflect.DeepEqual(got, tt.want) {
					t.Errorf("MissingItems() = %+v; want %+v", got, tt.want)
				}
			})
		}
	})
}
