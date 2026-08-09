// anchorread_test.go pins readRecordedAnchor's marker semantics in Tier 1: the stale pre-rename
// marker hard-errors, the renamed marker wins when both exist, and an absent or empty marker is a
// fallback — pure file reads, no git spawn, so the untagged suite stays sensitive to a removed
// guard (the integration-tagged anchor_test.go covers the same guard through the full resolvers,
// but an untagged run never compiles it).

package lyxcwd

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestReadRecordedAnchor_MarkerSemantics(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		staleValue string // content for StaleAnchorFileName, "" for absent
		newValue   string // content for AnchorFileName, "" for absent
		wantAnchor string
		wantFound  bool
		wantErr    error
	}{
		{"AbsentBoth_IsFallback", "", "", "", false, nil},
		{"StaleOnly_HardErrors", "backend\n", "", "", false, ErrStaleAnchorMarker},
		{"NewOnly_Wins", "", "backend\n", "backend", true, nil},
		{"BothPresent_NewWins", "old\n", "backend\n", "backend", true, nil},
		{"EmptyNewMarker_IsAbsentNotEmptySubpath", "", "   \n", "", false, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			hub := t.TempDir()
			board := boardDir(hub)
			if err := os.MkdirAll(board, 0o755); err != nil {
				t.Fatalf("mkdir board: %v", err)
			}
			if tt.staleValue != "" {
				if err := os.WriteFile(filepath.Join(board, StaleAnchorFileName), []byte(tt.staleValue), 0o644); err != nil {
					t.Fatalf("write stale marker: %v", err)
				}
			}
			if tt.newValue != "" {
				if err := os.WriteFile(filepath.Join(board, AnchorFileName), []byte(tt.newValue), 0o644); err != nil {
					t.Fatalf("write marker: %v", err)
				}
			}

			anchor, found, err := readRecordedAnchor(hub)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("readRecordedAnchor() error = %v; want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("readRecordedAnchor() error = %v; want nil", err)
			}
			if found != tt.wantFound || anchor != tt.wantAnchor {
				t.Errorf("readRecordedAnchor() = (%q, %v); want (%q, %v)", anchor, found, tt.wantAnchor, tt.wantFound)
			}
		})
	}
}
