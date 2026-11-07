// reinterpret_test.go covers the rewrite of trusted_hubs shortnames to repository names over a scripted resolver.

package creelconfig_test

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/creelconfig"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestRewriteShortnamesToRepositories pins that each shortname becomes its repository name or is dropped with a line,
// that a repository name is kept, that a second run changes nothing and that a malformed file is refused.
func TestRewriteShortnamesToRepositories(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	repositories := map[string]string{"ly": "Knatte18/loomyard", "cs": "Knatte18/cs-tools", "dup": "knatte18/LOOMYARD"}
	resolve := func(shortname string) (string, error) {
		if repository, ok := repositories[shortname]; ok {
			return repository, nil
		}
		return "", fmt.Errorf("no hub with the shortname %q is recorded on this machine", shortname)
	}
	rewrite := creelconfig.RewriteShortnamesToRepositories(resolve)

	tests := []struct {
		name  string
		input string
		want  []string
		// wantLineCount is how many lines the rewrite returns, and wantLines the text they hold between them.
		wantLineCount int
		wantLines     []string
		wantUnmoved   bool
	}{
		{"ShortnamesBecomeRepositories", "trusted_hubs:\n  - ly\n  - cs\n", []string{"Knatte18/loomyard", "Knatte18/cs-tools"}, 2, []string{`"ly" rewritten to "Knatte18/loomyard"`, `"cs" rewritten to "Knatte18/cs-tools"`}, false},
		{"AnUnresolvedShortnameIsDroppedWithItsReason", "trusted_hubs:\n  - ly\n  - gone\n", []string{"Knatte18/loomyard"}, 2, []string{`"gone" dropped`, `no hub with the shortname "gone"`, "lym config creel"}, false},
		{"ARepositoryNameIsKept", "trusted_hubs:\n  - Knatte18/other\n  - ly\n", []string{"Knatte18/other", "Knatte18/loomyard"}, 1, []string{`"ly" rewritten`}, false},
		{"TwoShortnamesOfOneRepositoryAreKeptOnce", "trusted_hubs:\n  - ly\n  - dup\n", []string{"Knatte18/loomyard"}, 2, []string{`"ly" rewritten`, `"dup" rewritten`}, false},
		{"OnlyRepositoryNamesChangeNothing", "trusted_hubs:\n  - Knatte18/loomyard\n", []string{"Knatte18/loomyard"}, 0, nil, true},
		{"AnEmptyListChangesNothing", "trusted_hubs: []\n", []string{}, 0, nil, true},
		{"NoKeyChangesNothing", "# nothing tuned\n", nil, 0, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			out, rewrote, flagged, err := rewrite([]byte(tt.input), "/hub/prime", "/hub/prime")
			if err != nil {
				t.Fatalf("rewrite(%q) error = %v; want nil", tt.input, err)
			}
			if flagged != nil {
				t.Errorf("rewrite(%q) flagged = %v; want none", tt.input, flagged)
			}
			if tt.wantUnmoved && string(out) != tt.input {
				t.Errorf("rewrite(%q) bytes = %q; want them unchanged", tt.input, out)
			}
			var got struct {
				TrustedHubs []string `yaml:"trusted_hubs"`
			}
			if err := yaml.Unmarshal(out, &got); err != nil {
				t.Fatalf("unmarshal the rewritten file %q: %v", out, err)
			}
			if !slices.Equal(got.TrustedHubs, tt.want) {
				t.Errorf("rewrite(%q) trusted_hubs = %v; want %v", tt.input, got.TrustedHubs, tt.want)
			}
			joined := strings.Join(rewrote, "\n")
			if len(rewrote) != tt.wantLineCount {
				t.Errorf("rewrite(%q) lines = %q; want %d lines", tt.input, rewrote, tt.wantLineCount)
			}
			for _, want := range tt.wantLines {
				if !strings.Contains(joined, want) {
					t.Errorf("rewrite(%q) lines = %q; want one containing %q", tt.input, rewrote, want)
				}
			}
			again, rewroteAgain, _, err := rewrite(out, "/hub/prime", "/hub/prime")
			if err != nil || string(again) != string(out) || len(rewroteAgain) != 0 {
				t.Errorf("a second rewrite of %q = (%q, %q, %v); want it unchanged with no line", out, again, rewroteAgain, err)
			}
		})
	}

	t.Run("AMalformedFileIsRefusedAsInvalid", func(t *testing.T) {
		t.Parallel()
		if _, _, _, err := rewrite([]byte("trusted_hubs: [ly\n"), "", ""); !errors.Is(err, configengine.ErrInvalid) {
			t.Errorf("rewrite(malformed) error = %v; want one marked ErrInvalid", err)
		}
	})
}
