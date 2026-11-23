// campaign_test.go covers CheckCampaign's refusals and Rotate's expansion.

package hardenshed

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestCheckCampaign(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const resume = `then run "lym loom resume"`
	// validCampaign returns an input that passes, over a worktree holding the package directory pkg.
	validCampaign := func(t *testing.T) CampaignCheck {
		root := t.TempDir()
		if err := os.Mkdir(filepath.Join(root, "pkg"), 0o755); err != nil {
			t.Fatal(err)
		}
		return CampaignCheck{
			Config: Config{
				Packages: []string{"pkg"}, MaxBounces: 7, HardMaxBounces: 10,
				Reviewer: []string{"opus", "fable", "sonnet"}, Fixer: []string{"sonnet", "opus"}, Concurrency: 3,
			},
			WorktreeRoot:  root,
			Live:          LiveKey{Tiers: []int{2}, Tested: []int{2}},
			HoldsTierTest: func(string, []int) bool { return true },
			Route:         func(key string) (string, error) { return "ROUTE(" + key + ")", nil },
		}
	}
	liveWrite := "gates." + LiveGateKey + "={tiers: [<live tier>], scope: diff}"

	tests := []struct {
		name     string
		mutate   func(*CampaignCheck)
		wantSubs []string
	}{
		{name: "PassingCampaign", mutate: func(*CampaignCheck) {}},
		{
			name: "NoPackagePassesAndRunsNoCopiesOrTierTestCheck",
			mutate: func(in *CampaignCheck) {
				in.Config.Packages = nil
				in.Live.Tiers = []int{1, 2}
				in.HoldsTierTest = func(string, []int) bool { return false }
			},
		},
		{
			name:     "PackageDirectoryMissing",
			mutate:   func(in *CampaignCheck) { in.Config.Packages = []string{"pkg", "absent"} },
			wantSubs: []string{`package "absent"`, "ROUTE(packages)"},
		},
		{
			name:   "TemplateLiveKeyPassesAndTheCopiesRunOnItsTier",
			mutate: func(in *CampaignCheck) { in.Live = LiveKey{Tiers: []int{0}, Tested: []int{0}} },
		},
		{
			name:     "LiveKeyNamesTheBilledTier",
			mutate:   func(in *CampaignCheck) { in.Live.Tiers = []int{3} },
			wantSubs: []string{"gates." + LiveGateKey + " names tier3", liveWrite},
		},
		{
			name:     "LiveKeyNamesTwoTiersWhenCopiesRun",
			mutate:   func(in *CampaignCheck) { in.Live.Tiers = []int{1, 2} },
			wantSubs: []string{"gates." + LiveGateKey + " names the tiers [1 2]", liveWrite},
		},
		{
			name:   "LiveKeyNamesTwoTiersWhenTheTestsDriveAnLLM",
			mutate: func(in *CampaignCheck) { in.Live.Tiers = []int{1, 2}; in.Config.LLMDriving = true },
		},
		{
			name:   "LiveKeyNamesTwoTiersWhenConcurrencyIsOne",
			mutate: func(in *CampaignCheck) { in.Live.Tiers = []int{1, 2}; in.Config.Concurrency = 1 },
		},
		{
			name: "PackageHoldsNoTestOfTheLiveTier",
			mutate: func(in *CampaignCheck) {
				in.HoldsTierTest = func(dir string, tiers []int) bool { return !strings.HasSuffix(dir, "pkg") }
			},
			wantSubs: []string{`package "pkg"`, "gates." + LiveGateKey, "ROUTE(packages)", liveWrite},
		},
		{
			name: "PackageTestedOnlyThroughACoveredTier",
			mutate: func(in *CampaignCheck) {
				in.Live.Tested = []int{2, 1}
				in.HoldsTierTest = func(_ string, tiers []int) bool { return slices.Contains(tiers, 1) }
			},
		},
		{
			name:     "ReviewerListEmpty",
			mutate:   func(in *CampaignCheck) { in.Config.Reviewer = nil },
			wantSubs: []string{"reviewer list is empty", "ROUTE(reviewer)"},
		},
		{
			name:     "FixerListEmpty",
			mutate:   func(in *CampaignCheck) { in.Config.Fixer = []string{} },
			wantSubs: []string{"fixer list is empty", "ROUTE(fixer)"},
		},
		{
			name:   "ReviewerListOfOneEntryPasses",
			mutate: func(in *CampaignCheck) { in.Config.Reviewer = []string{"opus"} },
		},
		{
			name:     "ReviewerNeighboursAreOneModel",
			mutate:   func(in *CampaignCheck) { in.Config.Reviewer = []string{"opus", "opus[high]", "sonnet"} },
			wantSubs: []string{`"opus"`, "ROUTE(reviewer)"},
		},
		{
			name:     "ReviewerLastAndFirstAreOneModel",
			mutate:   func(in *CampaignCheck) { in.Config.Reviewer = []string{"opus", "sonnet", "opus"} },
			wantSubs: []string{"last and first", "ROUTE(reviewer)"},
		},
		{
			name:   "ReviewerListOfTwoModelsPasses",
			mutate: func(in *CampaignCheck) { in.Config.Reviewer = []string{"opus", "sonnet"} },
		},
		{
			name:     "SoftCapAboveTheHardCap",
			mutate:   func(in *CampaignCheck) { in.Config.MaxBounces = 11 },
			wantSubs: []string{"max_bounces 11", "hard_max_bounces 10", "ROUTE(max_bounces)"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			in := validCampaign(t)
			tt.mutate(&in)

			err := CheckCampaign(in)

			if tt.wantSubs == nil {
				if err != nil {
					t.Errorf("CheckCampaign() = %v; want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("CheckCampaign() = nil; want a refusal holding %q", tt.wantSubs)
			}
			for _, want := range append(tt.wantSubs, "way forward: ", resume) {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("CheckCampaign() error = %q; want it to hold %q", err, want)
				}
			}
		})
	}
}

func TestRotate(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name   string
		list   []string
		rounds int
		want   []string
	}{
		{"NineRoundsOfThreeEntries", []string{"a", "b", "c"}, 9, []string{"a", "b", "c", "a", "b", "c", "a", "b", "c"}},
		{"TenRoundsWrapAgain", []string{"a", "b", "c"}, 10, []string{"a", "b", "c", "a", "b", "c", "a", "b", "c", "a"}},
		{"OneEntryRepeats", []string{"a"}, 3, []string{"a", "a", "a"}},
		{"EmptyListStaysEmpty", []string{}, 5, []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := Rotate(tt.list, tt.rounds); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Rotate(%v, %d) = %v; want %v", tt.list, tt.rounds, got, tt.want)
			}
		})
	}
}
