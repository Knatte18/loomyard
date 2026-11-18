// accountpick_test.go tests the pick of an account for a worktree: the counters, headroom and the candidates.

package claudeengine

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// pickFixture is a root directory holding the config directories of the accounts a test lists.
type pickFixture struct {
	t    testing.TB
	root string
}

// account returns an account of the given weight and windows whose config directory is under the fixture root.
// A logged-in account holds an empty credentials file, which is all a login check stats.
func (f pickFixture) account(name string, weight int, loggedIn bool, windows map[string]int) AccountConfig {
	f.t.Helper()
	dir := filepath.Join(f.root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		f.t.Fatalf("create the config directory of %s: %v", name, err)
	}
	if loggedIn {
		if err := os.WriteFile(filepath.Join(dir, credentialsFileName), nil, 0o600); err != nil {
			f.t.Fatalf("write the credentials file of %s: %v", name, err)
		}
	}
	return AccountConfig{Name: name, ConfigDir: dir, Weight: weight, Windows: windows}
}

// TestPickAccount pins the pick: the counters it keeps, the accounts it passes over and its fallback.
func TestPickAccount(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	fixture := pickFixture{t: t, root: t.TempDir()}
	now := time.Unix(1_000_000, 0)
	future, past := now.Add(time.Hour), now.Add(-time.Hour)
	five := map[string]int{"five_hour": 80}
	atThreshold := func(reset time.Time, percentage float64) usageRecord {
		return usageRecord{Windows: map[string]usageWindow{"five_hour": {UsedPercentage: percentage, ResetsAt: reset.Unix()}}}
	}
	blocked := func(until time.Time) usageRecord { return usageRecord{BlockedUntil: until} }
	a, b := fixture.account("a", 1, true, five), fixture.account("b", 1, true, nil)
	listedAB := accountFile{Listed: []string{"a", "b"}, Weights: map[string]int{"a": 1, "b": 1}, Counts: map[string]float64{"a": 3, "b": 3}}

	tests := []struct {
		name     string
		accounts []AccountConfig
		file     accountFile
		usage    map[string]usageRecord
		exclude  string
		fallback bool
		wantName string
		// wantCounts is the counters after a pick that takes an account; nil skips the check.
		wantCounts map[string]float64
	}{
		{
			name:     "BothEligibleTakesTheLowerCountOverWeight",
			accounts: []AccountConfig{fixture.account("w2", 2, true, nil), fixture.account("w1", 1, true, nil)},
			file:     accountFile{Listed: []string{"w2", "w1"}, Weights: map[string]int{"w2": 2, "w1": 1}, Counts: map[string]float64{"w2": 1, "w1": 1}},
			wantName: "w2", wantCounts: map[string]float64{"w2": 2, "w1": 1},
		},
		{name: "AWindowAtItsThresholdWithAFutureResetIsPassedOver", accounts: []AccountConfig{a, b}, file: listedAB, usage: map[string]usageRecord{"a": atThreshold(future, 80)}, wantName: "b"},
		{name: "AWindowAtItsThresholdWhoseResetPassedIsTaken", accounts: []AccountConfig{a, b}, file: listedAB, usage: map[string]usageRecord{"a": atThreshold(past, 95)}, wantName: "a"},
		{name: "AWindowBelowItsThresholdIsTaken", accounts: []AccountConfig{a, b}, file: listedAB, usage: map[string]usageRecord{"a": atThreshold(future, 79)}, wantName: "a"},
		{name: "AnAccountListingNoWindowsIsPassedOverWhileBlocked", accounts: []AccountConfig{b, a}, file: listedAB, usage: map[string]usageRecord{"b": blocked(future)}, wantName: "a"},
		{
			name:     "AnAccountWithoutCredentialsAndOneOfWeightZeroAreNeverTaken",
			accounts: []AccountConfig{fixture.account("retired", 0, true, nil), fixture.account("nocredentials", 1, false, nil), b},
			wantName: "b",
		},
		{name: "NoCandidateTakesNothing", accounts: []AccountConfig{fixture.account("retired", 0, true, nil)}},
		{
			name: "EveryCandidateShortTakesTheEarliestLatestBlockingReset", accounts: []AccountConfig{a, b}, fallback: true,
			usage:    map[string]usageRecord{"a": blocked(now.Add(2 * time.Hour)), "b": blocked(future)},
			wantName: "b",
		},
		{
			name: "AnAccountsLatestBlockingResetIsTheLaterOfItsReasons", accounts: []AccountConfig{a, b}, fallback: true,
			usage: map[string]usageRecord{
				"a": {BlockedUntil: future, Windows: atThreshold(now.Add(5*time.Hour), 90).Windows},
				"b": blocked(now.Add(2 * time.Hour)),
			},
			wantName: "b",
		},
		{
			name: "ATieOfBlockingResetsGoesToTheEarlierListEntry", accounts: []AccountConfig{a, b}, fallback: true,
			usage:    map[string]usageRecord{"a": blocked(future), "b": blocked(future)},
			wantName: "a",
		},
		{name: "EveryCandidateShortTakesNothingWithTheFallbackOff", accounts: []AccountConfig{a, b}, usage: map[string]usageRecord{"a": blocked(future), "b": blocked(future)}},
		{name: "TheExcludedAccountIsNeverTaken", accounts: []AccountConfig{a, b}, file: accountFile{Listed: []string{"a", "b"}, Weights: map[string]int{"a": 1, "b": 1}, Counts: map[string]float64{"b": 9}}, exclude: "a", wantName: "b"},
		{
			name:     "AnAddedAccountStartsAtTheLowestUnchangedCountOverWeightTimesItsWeight",
			accounts: []AccountConfig{fixture.account("old", 1, true, nil), fixture.account("new", 2, true, nil)},
			file:     accountFile{Listed: []string{"old"}, Weights: map[string]int{"old": 1}, Counts: map[string]float64{"old": 4}},
			wantName: "old", wantCounts: map[string]float64{"old": 5, "new": 8},
		},
		{
			name:     "AReweightedAccountStartsAtTheLowestUnchangedCountOverWeightTimesItsNewWeight",
			accounts: []AccountConfig{fixture.account("steady", 1, true, nil), fixture.account("heavier", 2, true, nil)},
			file: accountFile{
				Listed: []string{"steady", "heavier"}, Weights: map[string]int{"steady": 1, "heavier": 1},
				Counts: map[string]float64{"steady": 2, "heavier": 7},
			},
			wantName: "steady", wantCounts: map[string]float64{"steady": 3, "heavier": 4},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := pickRequest{
				accounts: tt.accounts, usage: tt.usage, exclude: tt.exclude, fallback: tt.fallback, now: now,
				loggedIn: func(account AccountConfig) bool { return loggedIn("linux", account) },
			}
			picked, name, ok := pickAccount(tt.file, req)
			if name != tt.wantName || ok != (tt.wantName != "") {
				t.Fatalf("pickAccount() = %q, %v; want %q, %v", name, ok, tt.wantName, tt.wantName != "")
			}
			if !ok {
				if !reflect.DeepEqual(picked, tt.file) {
					t.Errorf("pickAccount() file = %+v; want the file it was given, unchanged", picked)
				}
				return
			}
			if tt.wantCounts != nil && !reflect.DeepEqual(picked.Counts, tt.wantCounts) {
				t.Errorf("pickAccount() counts = %v; want %v", picked.Counts, tt.wantCounts)
			}
			wantListed := make([]string, 0, len(tt.accounts))
			wantWeights := map[string]int{}
			for _, account := range tt.accounts {
				wantListed = append(wantListed, account.Name)
				wantWeights[account.Name] = account.Weight
			}
			if !reflect.DeepEqual(picked.Listed, wantListed) || !reflect.DeepEqual(picked.Weights, wantWeights) {
				t.Errorf("pickAccount() listed = %v, weights = %v; want %v, %v", picked.Listed, picked.Weights, wantListed, wantWeights)
			}
		})
	}

	t.Run("SixPicksOverWeightsTwoAndOneGiveFourAndTwoAndNeverTheSecondTwiceInARow", func(t *testing.T) {
		t.Parallel()

		accounts := []AccountConfig{fixture.account("first", 2, true, nil), fixture.account("second", 1, true, nil)}
		req := pickRequest{accounts: accounts, now: now, loggedIn: func(account AccountConfig) bool { return loggedIn("linux", account) }}
		var file accountFile
		var taken []string
		for range 6 {
			var name string
			var ok bool
			if file, name, ok = pickAccount(file, req); !ok {
				t.Fatalf("pickAccount() found no account after %v", taken)
			}
			taken = append(taken, name)
		}
		counts := map[string]int{}
		for i, name := range taken {
			counts[name]++
			if name == "second" && i > 0 && taken[i-1] == "second" {
				t.Errorf("picks %v take the second account twice in a row", taken)
			}
		}
		if counts["first"] != 4 || counts["second"] != 2 {
			t.Errorf("picks %v give %v; want four of the first and two of the second", taken, counts)
		}
	})

	t.Run("DarwinNeedsNoCredentialsFile", func(t *testing.T) {
		t.Parallel()

		bare := fixture.account("keychain", 1, false, nil)
		if !loggedIn("darwin", bare) || loggedIn("linux", bare) {
			t.Errorf("loggedIn(darwin) = %v, loggedIn(linux) = %v for an account with no credentials file; want true and false", loggedIn("darwin", bare), loggedIn("linux", bare))
		}
	})
}
