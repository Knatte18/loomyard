//go:build tier0

// keepwarm_test.go verifies the keep-warm resolver answers a strand's role cap from claude.yaml and falls back to the await cap when nothing resolves.

package shuttleengines

import (
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/shuttlefake"
)

// TestKeepWarm pins the resolver's answers: the template's caps and break-even, a role entry for a numbered or prime strand, the fallback for every unresolvable input, and a zero cap from an engine that keeps nothing warm.
func TestKeepWarm(t *testing.T) {
	t.Parallel()

	fallback := func(a KeepWarmAnswer) bool {
		return a.Fallback != nil && a.Cap == shuttleengine.DefaultAwaitCap && a.LongCap == shuttleengine.DefaultAwaitCap && a.MaxReruns == 0
	}
	tests := []struct {
		name string
		// claudeLocal and shuttleLocal are the local layer documents written before the call, empty for none.
		claudeLocal  string
		shuttleLocal string
		strand       string
		want         KeepWarmAnswer
		wantFallback bool
	}{
		{name: "template", strand: "sh:slug:implementer", want: KeepWarmAnswer{Cap: 250 * time.Second, LongCap: 55 * time.Minute, MaxReruns: 10}},
		{name: "numbered_role_takes_the_base_entry", claudeLocal: "keep_warm_cap_roles:\n  implementer: 120s\n", strand: "sh:slug:implementer-2", want: KeepWarmAnswer{Cap: 120 * time.Second, LongCap: 55 * time.Minute, MaxReruns: 10}},
		{name: "prime_strand_takes_the_entry", claudeLocal: "keep_warm_cap_roles:\n  implementer: 120s\n", strand: "sh:implementer", want: KeepWarmAnswer{Cap: 120 * time.Second, LongCap: 55 * time.Minute, MaxReruns: 10}},
		{name: "unmatched_suffix_is_its_own_role", claudeLocal: "keep_warm_cap_roles:\n  implementer: 120s\n", strand: "sh:slug:implementer-x", want: KeepWarmAnswer{Cap: 250 * time.Second, LongCap: 55 * time.Minute, MaxReruns: 10}},
		{name: "empty_strand", strand: "", wantFallback: true},
		{name: "unparsable_strand", strand: "not a strand", wantFallback: true},
		{name: "invalid_claude_layer", claudeLocal: "binary: /x\n", strand: "sh:slug:implementer", wantFallback: true},
		{name: "unregistered_engine", shuttleLocal: "engine: bogus\n", strand: "sh:slug:implementer", wantFallback: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			anchor := t.TempDir()
			if tt.claudeLocal != "" {
				writeLocalLayer(t, anchor, "claude", tt.claudeLocal)
			}
			if tt.shuttleLocal != "" {
				writeLocalLayer(t, anchor, "shuttle", tt.shuttleLocal)
			}
			got := KeepWarm(configengine.Dirs{Anchor: anchor}, tt.strand)
			if tt.wantFallback {
				if !fallback(got) {
					t.Errorf("KeepWarm(%q) = %+v; want the fallback with a cause", tt.strand, got)
				}
				return
			}
			if got != tt.want {
				t.Errorf("KeepWarm(%q) = %+v; want %+v", tt.strand, got, tt.want)
			}
		})
	}

	t.Run("an_engine_keeping_nothing_warm_answers_zero", func(t *testing.T) {
		t.Parallel()

		constructors := map[string]constructor{
			"claude": func(shuttleengine.Config, configengine.Dirs) shuttleengine.Engine { return &shuttlefake.Engine{} },
		}
		got := keepWarmWith(configengine.Dirs{Anchor: t.TempDir()}, "sh:slug:implementer", constructors)
		if got.Fallback != nil || got.Cap != 0 {
			t.Errorf("keepWarmWith over a fake engine = %+v; want a zero Cap and no fallback", got)
		}
	})
}
