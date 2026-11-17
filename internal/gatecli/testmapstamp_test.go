// testmapstamp_test.go covers the environment stamp a coverage map build computes.
// It also covers the side-by-side count and slot ask of the map run.

package gatecli

import (
	"reflect"
	"testing"

	"github.com/Knatte18/loomyard/internal/envconfig"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/testcover"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestStampEnvironment(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	toolchain := testcover.EnvironmentStamp{GoVersion: "go1.26.8", GOOS: "linux", GOARCH: "amd64"}
	baseInputs := func() (map[int]gateslot.Tier, envconfig.Config) {
		tiers := map[int]gateslot.Tier{0: {Command: "go test {args}", Env: map[string]string{"MODE": "fast"}, Parallel: 2}}
		return tiers, envconfig.Config{Vars: map[string]string{"HUB": "one"}}
	}
	baseTiers, baseHubEnv := baseInputs()
	base := stampEnvironment(toolchain, baseTiers, baseHubEnv)

	tests := []struct {
		name   string
		change func(tiers map[int]gateslot.Tier, hubEnv *envconfig.Config)
		// sameTiers and sameHubEnv say whether the change leaves that digest as it was.
		sameTiers, sameHubEnv bool
	}{
		{name: "UnchangedInputs", change: func(map[int]gateslot.Tier, *envconfig.Config) {}, sameTiers: true, sameHubEnv: true},
		{name: "ChangedTierCommand", sameHubEnv: true, change: func(tiers map[int]gateslot.Tier, _ *envconfig.Config) {
			tier := tiers[0]
			tier.Command = "go test -short {args}"
			tiers[0] = tier
		}},
		{name: "ChangedTierEnvValue", sameHubEnv: true, change: func(tiers map[int]gateslot.Tier, _ *envconfig.Config) {
			tiers[0].Env["MODE"] = "slow"
		}},
		{name: "ChangedTierParallel", sameTiers: true, sameHubEnv: true, change: func(tiers map[int]gateslot.Tier, _ *envconfig.Config) {
			tier := tiers[0]
			tier.Parallel = 8
			tiers[0] = tier
		}},
		{name: "ChangedHubEnvVariable", sameTiers: true, change: func(_ map[int]gateslot.Tier, hubEnv *envconfig.Config) {
			hubEnv.Vars["HUB"] = "two"
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tiers, hubEnv := baseInputs()
			tt.change(tiers, &hubEnv)
			got := stampEnvironment(toolchain, tiers, hubEnv)
			if (got.TiersDigest == base.TiersDigest) != tt.sameTiers {
				t.Errorf("stampEnvironment() TiersDigest equal to the base = %t; want %t", got.TiersDigest == base.TiersDigest, tt.sameTiers)
			}
			if (got.HubEnvDigest == base.HubEnvDigest) != tt.sameHubEnv {
				t.Errorf("stampEnvironment() HubEnvDigest equal to the base = %t; want %t", got.HubEnvDigest == base.HubEnvDigest, tt.sameHubEnv)
			}
			if got.GoVersion != toolchain.GoVersion || got.GOOS != toolchain.GOOS || got.GOARCH != toolchain.GOARCH {
				t.Errorf("stampEnvironment() = %+v; want the toolchain's version and platform kept", got)
			}
		})
	}
}

func TestMapSideBySide(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const testMemory = 128 << 20
	tests := []struct {
		name string
		tier gateslot.Tier
		// cappedTo is the most runs the stub memory clamp admits; 0 admits every run.
		cappedTo int
		want     int
	}{
		{"NoMemoryMBGivesTheCappedParallel", gateslot.Tier{Parallel: 4}, 3, 3},
		{"MemoryMBOfTwiceTheTestCapGivesTwo", gateslot.Tier{Parallel: 4, MemoryMB: 256}, 0, 2},
		{"MemoryMBEqualToTheTestCapGivesOne", gateslot.Tier{Parallel: 4, MemoryMB: 128}, 0, 1},
		{"ACappedCountBelowTheMemoryQuotientWins", gateslot.Tier{Parallel: 4, MemoryMB: 1024}, 3, 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			capped := func(parallel int, limit int64) int {
				if limit != testMemory {
					t.Errorf("capped limit = %d; want the test memory %d", limit, int64(testMemory))
				}
				if tt.cappedTo > 0 {
					return min(parallel, tt.cappedTo)
				}
				return parallel
			}
			if got := mapSideBySide(tt.tier, testMemory, capped); got != tt.want {
				t.Errorf("mapSideBySide(%+v) = %d; want %d", tt.tier, got, tt.want)
			}
		})
	}
}

func TestMapSlotAsk(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name  string
		tiers map[int]gateslot.Tier
		want  int
	}{
		{"Tier0Alone", map[int]gateslot.Tier{0: {Slots: 2}}, 2},
		{"TheLargerOfTwoCounts", map[int]gateslot.Tier{0: {Slots: 1}, 1: {Slots: 2}}, 2},
		{"Tier1AskingEverySlot", map[int]gateslot.Tier{0: {Slots: 1}, 1: {Slots: 0}}, 0},
		{"Tier0AskingEverySlotBesideACount", map[int]gateslot.Tier{0: {Slots: 0}, 1: {Slots: 3}}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := mapSlotAsk(tt.tiers); got != tt.want {
				t.Errorf("mapSlotAsk(%+v) = %d; want %d", tt.tiers, got, tt.want)
			}
		})
	}
}

func TestMappedTierConfigs(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tier0 := gateslot.Tier{Command: "go test {args}"}
	cfg := gateslot.Config{Tiers: map[string]gateslot.Tier{"tier0": tier0, "tier2": {Command: "dotnet test"}}}
	want := map[int]gateslot.Tier{0: tier0}
	if got := mappedTierConfigs(cfg); !reflect.DeepEqual(got, want) {
		t.Errorf("mappedTierConfigs() = %v; want only the declared mapped tier %v", got, want)
	}
}
