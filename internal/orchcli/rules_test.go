// rules_test.go covers the orch rules entry's store change and the refusals writeOrchRules raises before it reaches the store, a cwd outside a hub included.

package orchcli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/heddlecli"
	"github.com/Knatte18/loomyard/internal/heddleengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestWriteOrchRules_ReportsTheStoreOutcome(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "draft.md"), []byte("rule"), 0o644); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name        string
		warning     string
		storeErr    error
		wantCode    int
		wantWarning string
		wantError   string
	}{
		{name: "SyncWarningReachesTheEnvelope", warning: "sync did not start", wantWarning: "sync did not start"},
		{name: "CleanSaveCarriesNoWarning"},
		{name: "StoreErrorIsRefused", storeErr: errors.New("store refuses"), wantCode: 1, wantError: "store refuses"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var out bytes.Buffer
			changeStore := func(string, func(*heddleengine.Store) error) (string, error) { return tt.warning, tt.storeErr }
			if code := writeOrchRules(&out, dir, "draft.md", changeStore); code != tt.wantCode {
				t.Fatalf("writeOrchRules exit = %d; want %d; output: %s", code, tt.wantCode, out.String())
			}
			var envelope map[string]any
			if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
				t.Fatalf("envelope %q is not JSON: %v", out.String(), err)
			}
			if got, _ := envelope["warning"].(string); got != tt.wantWarning {
				t.Errorf("envelope warning = %q; want %q", got, tt.wantWarning)
			}
			if got, _ := envelope["error"].(string); got != tt.wantError {
				t.Errorf("envelope error = %q; want %q", got, tt.wantError)
			}
		})
	}
}

func TestApplyOrchRules(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	otherText, otherOrder := "house rule", 5
	tests := []struct {
		name string
		// prior is the orch rules entry's text and order before the change; empty text means no entry.
		priorText  string
		priorOrder int
		text       string
		// wantText is the entry's text after the change; empty means no entry.
		wantText  string
		wantOrder int
	}{
		{name: "NewEntry", text: "### Rule\n", wantText: "### Rule\n", wantOrder: orchRulesOrder},
		{name: "ReplaceKeepsOrder", priorText: "old", priorOrder: 7, text: "new", wantText: "new", wantOrder: 7},
		{name: "BlankClears", priorText: "old", priorOrder: 7, text: " \n"},
		{name: "BlankWithNoEntry", text: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var store heddleengine.Store
			if err := store.Set("house", heddleengine.Patch{Text: &otherText, Order: &otherOrder, Targets: []string{"claude-prompt"}}); err != nil {
				t.Fatal(err)
			}
			if tt.priorText != "" {
				prior := heddleengine.Patch{Text: &tt.priorText, Order: &tt.priorOrder, Targets: []string{"claude-prompt", heddleengine.OrchPromptTarget}}
				if err := store.Set(orchRulesSlug, prior); err != nil {
					t.Fatal(err)
				}
			}
			if err := applyOrchRules(&store, tt.text); err != nil {
				t.Fatalf("applyOrchRules(%q) error: %v; want nil", tt.text, err)
			}
			entry, ok := store.Get(orchRulesSlug)
			switch {
			case tt.wantText == "" && ok:
				t.Errorf("applyOrchRules(%q) left entry %+v; want none", tt.text, entry)
			case tt.wantText != "" && !ok:
				t.Errorf("applyOrchRules(%q) left no entry; want text %q", tt.text, tt.wantText)
			case ok:
				if entry.Text != tt.wantText || entry.Order != tt.wantOrder || !slices.Equal(entry.Targets, []string{heddleengine.OrchPromptTarget}) {
					t.Errorf("entry = %+v; want text %q, order %d, targets [%s]", entry, tt.wantText, tt.wantOrder, heddleengine.OrchPromptTarget)
				}
			}
			if house, ok := store.Get("house"); !ok || house.Text != otherText {
				t.Errorf("house entry = %+v, %v; want it unchanged", house, ok)
			}
		})
	}
}

func TestWriteOrchRules_RefusesBeforeTheStore(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "draft.md"), []byte("rule"), 0o644); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name    string
		from    string
		wantErr string
	}{
		{"NoFrom", "", "needs --from"},
		{"UnreadableDraft", "missing.md", filepath.Join(dir, "missing.md")},
		{"CwdOutsideAHub", "draft.md", "run lym heddle from a code worktree of a hub"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var out bytes.Buffer
			if code := writeOrchRules(&out, dir, tt.from, heddlecli.ChangeStore); code != 1 {
				t.Fatalf("writeOrchRules(%q) exit = %d; want 1; output: %s", tt.from, code, out.String())
			}
			var envelope map[string]any
			if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
				t.Fatalf("envelope %q is not JSON: %v", out.String(), err)
			}
			if got, _ := envelope["error"].(string); !strings.Contains(got, tt.wantErr) {
				t.Errorf("envelope error = %q; want it to contain %q", got, tt.wantErr)
			}
		})
	}
}
