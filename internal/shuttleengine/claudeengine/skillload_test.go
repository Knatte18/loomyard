package claudeengine

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

func stopEventNaming(t *testing.T, transcriptPath string) shuttleengine.Event {
	t.Helper()
	raw, err := json.Marshal(map[string]string{"transcript_path": transcriptPath})
	if err != nil {
		t.Fatal(err)
	}
	return shuttleengine.Event{Kind: shuttleengine.EventStop, Raw: raw}
}

func TestClassifySkillLoad(t *testing.T) {
	t.Parallel()
	mixed := filepath.Join("testdata", "skillload-mixed.jsonl")
	noListing := filepath.Join("testdata", "skillload-no-listing.jsonl")
	requested := []string{"a", "b", "c", "ghost", "err"}

	tests := []struct {
		name    string
		turnEnd shuttleengine.Event
		skills  []string
		want    shuttleengine.SkillLoadReport
	}{
		{
			name:    "classifies loaded, unknown and missing from the load turn only",
			turnEnd: stopEventNaming(t, mixed),
			skills:  requested,
			want: shuttleengine.SkillLoadReport{
				Verified: true,
				Loaded:   []string{"a", "b"},
				Unknown:  []string{"ghost", "err"},
				Missing:  []string{"c"},
			},
		},
		{
			name:    "no skill listing is unverified, never unknown by absence",
			turnEnd: stopEventNaming(t, noListing),
			skills:  requested,
		},
		{
			name:    "a load message naming a different list is unverified",
			turnEnd: stopEventNaming(t, mixed),
			skills:  []string{"a", "b"},
		},
		{
			name:    "an unreadable transcript is unverified",
			turnEnd: stopEventNaming(t, filepath.Join(t.TempDir(), "absent.jsonl")),
			skills:  requested,
		},
		{
			name:    "a Stop payload with no transcript_path is unverified",
			turnEnd: shuttleengine.Event{Kind: shuttleengine.EventStop, Raw: []byte(`{}`)},
			skills:  requested,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := (&Claude{}).ClassifySkillLoad(tt.turnEnd, tt.skills)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ClassifySkillLoad() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestSkillLoadMessage_IsOneSlashlessLineNamingEverySkillInOrder(t *testing.T) {
	t.Parallel()
	skills := []string{"scribe:prose", "ly:board", "scribe:testing"}
	message := (&Claude{}).SkillLoadMessage(skills)

	if strings.Contains(message, "\n") {
		t.Errorf("message spans several lines: %q", message)
	}
	if strings.HasPrefix(message, "/") {
		t.Errorf("message starts with a slash: %q", message)
	}
	at := -1
	for _, skill := range skills {
		next := strings.Index(message[at+1:], skill)
		if next < 0 {
			t.Fatalf("message %q does not name %q after the previous skill", message, skill)
		}
		at += 1 + next
	}
}
