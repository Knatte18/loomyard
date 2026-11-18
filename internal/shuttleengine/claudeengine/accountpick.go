// accountpick.go holds the rules that pick an account for a worktree and resolve the account a worktree runs under.
// They are pure functions over the account list, the account file and the usage records.

package claudeengine

import (
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// credentialsFileName is the file a logged-in Claude account's config directory holds.
const credentialsFileName = ".credentials.json"

// hasHeadroom reports whether account may take a new worktree at now.
// It has none while usage says a usage limit blocks it.
// Otherwise it has headroom when every window the account lists has no reading, a reset that has passed,
// or a percentage below the account's threshold for it.
// A window the account does not list is ignored.
func hasHeadroom(account AccountConfig, usage usageRecord, now time.Time) bool {
	return len(blockingResets(account, usage, now)) == 0
}

// blockingResets returns the times at which each reason the account lacks headroom ends.
// A reason is a limit block or a listed window at or above its threshold.
// A limit block ends at BlockedUntil, and a window ends at its reset.
// The result is empty exactly when the account has headroom.
func blockingResets(account AccountConfig, usage usageRecord, now time.Time) []time.Time {
	var resets []time.Time
	if usage.BlockedUntil.After(now) {
		resets = append(resets, usage.BlockedUntil)
	}
	for window, threshold := range account.Windows {
		reading, ok := usage.Windows[window]
		if !ok {
			continue
		}
		reset := time.Unix(reading.ResetsAt, 0)
		if reset.After(now) && reading.UsedPercentage >= float64(threshold) {
			resets = append(resets, reset)
		}
	}
	return resets
}

// loggedIn reports whether account holds a login on the platform goos names.
// On darwin the login is in the keychain, which a stat cannot see, so every account counts.
// Elsewhere the account's config directory holds a credentials file, which the check stats and never reads.
func loggedIn(goos string, account AccountConfig) bool {
	if goos == "darwin" {
		return true
	}
	_, err := os.Stat(filepath.Join(account.ConfigDir, credentialsFileName))
	return err == nil
}

// pickRequest is what one pick decides over.
type pickRequest struct {
	// accounts is the configured account list, in list order.
	accounts []AccountConfig
	// usage maps an account name to its usage record; a name it lacks has no reading.
	usage map[string]usageRecord
	// loggedIn reports whether an account holds a login.
	loggedIn func(AccountConfig) bool
	// exclude names an account the pick never takes, empty for none.
	exclude string
	// fallback lets a pick with no account of headroom take the account whose block ends first.
	fallback bool
	// now is the time headroom is judged at.
	now time.Time
}

// pickAccount picks the account a worktree runs under, over file's counters.
// It returns the file with its counters updated for the pick, the taken account's name and true.
// When no account can be taken it returns file unchanged, the empty name and false.
// It leaves Worktrees and Picked to its caller.
//
// An account of positive weight that is new to the counters, or whose weight changed, starts at a reset count.
// That count is the lowest count over weight among the accounts listed and unchanged, times its own weight.
// With none unchanged it starts at 0.
// The candidates are the logged-in accounts of positive weight but the excluded one.
// Among the candidates with headroom it takes the lowest count over weight, a tie going to the earlier list entry.
// With none having headroom and the fallback on, it takes the candidate whose latest blocking reset is earliest.
// A tie there also goes to the earlier list entry.
func pickAccount(file accountFile, req pickRequest) (accountFile, string, bool) {
	counts := resetCounts(file, req.accounts)

	var candidates []AccountConfig
	for _, account := range req.accounts {
		if account.Weight > 0 && account.Name != req.exclude && req.loggedIn(account) {
			candidates = append(candidates, account)
		}
	}
	if len(candidates) == 0 {
		return file, "", false
	}

	var eligible []AccountConfig
	for _, account := range candidates {
		if hasHeadroom(account, req.usage[account.Name], req.now) {
			eligible = append(eligible, account)
		}
	}
	var taken AccountConfig
	switch {
	case len(eligible) > 0:
		taken = lowestLoad(eligible, counts)
	case req.fallback:
		taken = earliestUnblocked(candidates, req.usage, req.now)
	default:
		return file, "", false
	}

	counts[taken.Name]++
	picked := file
	picked.Counts = counts
	picked.Weights = make(map[string]int, len(req.accounts))
	picked.Listed = make([]string, 0, len(req.accounts))
	for _, account := range req.accounts {
		picked.Weights[account.Name] = account.Weight
		picked.Listed = append(picked.Listed, account.Name)
	}
	return picked, taken.Name, true
}

// resetCounts returns the counters of every positive-weight account of accounts.
// An account listed in file under its current weight keeps its count.
// A new or reweighted one starts at the lowest count over weight among the kept accounts, times its weight.
func resetCounts(file accountFile, accounts []AccountConfig) map[string]float64 {
	counts := make(map[string]float64, len(accounts))
	kept := func(account AccountConfig) bool {
		weight, weighed := file.Weights[account.Name]
		return slices.Contains(file.Listed, account.Name) && weighed && weight == account.Weight
	}
	lowest, anyKept := 0.0, false
	for _, account := range accounts {
		if account.Weight <= 0 || !kept(account) {
			continue
		}
		counts[account.Name] = file.Counts[account.Name]
		if load := counts[account.Name] / float64(account.Weight); !anyKept || load < lowest {
			lowest, anyKept = load, true
		}
	}
	for _, account := range accounts {
		if account.Weight > 0 && !kept(account) {
			counts[account.Name] = lowest * float64(account.Weight)
		}
	}
	return counts
}

// lowestLoad returns the account of candidates with the lowest count over weight, a tie going to the earlier entry.
// candidates is not empty and holds only positive weights.
func lowestLoad(candidates []AccountConfig, counts map[string]float64) AccountConfig {
	best := candidates[0]
	for _, account := range candidates[1:] {
		if counts[account.Name]/float64(account.Weight) < counts[best.Name]/float64(best.Weight) {
			best = account
		}
	}
	return best
}

// earliestUnblocked returns the account of candidates whose latest blocking reset is earliest.
// A tie goes to the earlier entry.
// candidates is not empty and holds only accounts without headroom.
func earliestUnblocked(candidates []AccountConfig, usage map[string]usageRecord, now time.Time) AccountConfig {
	latest := func(account AccountConfig) time.Time {
		var last time.Time
		for _, reset := range blockingResets(account, usage[account.Name], now) {
			if reset.After(last) {
				last = reset
			}
		}
		return last
	}
	best := candidates[0]
	for _, account := range candidates[1:] {
		if latest(account).Before(latest(best)) {
			best = account
		}
	}
	return best
}

// resolveAccount returns the account a launch in worktree runs under, and whether it runs under one.
// A hub pin wins and is never skipped.
// For a prime or beam worktree the beams pin comes next.
// It yields when its account is blocked at now and the worktree has an entry.
// Then comes the worktree's own entry in file.
// An entry naming an account the list lacks, or a retired one, counts as none.
// A zero worktree reaches only the hub pin.
func resolveAccount(claude Config, worktree shuttleengine.Worktree, file accountFile, usage map[string]usageRecord, now time.Time) (AccountConfig, bool) {
	listed := func(name string) (AccountConfig, bool) {
		index := slices.IndexFunc(claude.Accounts, func(account AccountConfig) bool { return account.Name == name })
		if index < 0 || claude.Accounts[index].retired() {
			return AccountConfig{}, false
		}
		return claude.Accounts[index], true
	}

	if claude.AccountPins.Hub != "" {
		return listed(claude.AccountPins.Hub)
	}
	if worktree.Name == "" {
		return AccountConfig{}, false
	}
	entry, hasEntry := listed(file.Worktrees[worktree.Name])
	beamsPinned := claude.AccountPins.Beams != "" && (worktree.Kind == shuttleengine.WorktreePrime || worktree.Kind == shuttleengine.WorktreeBeam)
	if beamsPinned {
		pinned, ok := listed(claude.AccountPins.Beams)
		if ok && !(hasEntry && usage[pinned.Name].BlockedUntil.After(now)) {
			return pinned, true
		}
	}
	return entry, hasEntry
}
