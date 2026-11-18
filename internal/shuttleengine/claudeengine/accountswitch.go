// accountswitch.go implements shuttleengine.AccountSwitcher for the Claude engine:
// the record of a usage limit against an account, and the move of the worktree to another account.

package claudeengine

import (
	"fmt"
	"runtime"
	"time"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// Compile-time proof that Claude can switch a worktree's account at a usage limit.
var _ shuttleengine.AccountSwitcher = (*Claude)(nil)

// usageLimitFallback is how long a usage limit whose message states no reset blocks its account.
const usageLimitFallback = 30 * time.Minute

// SwitchAccount implements shuttleengine.AccountSwitcher.
// It reads the account the run in runDir launched under.
// A session on an unlisted directory records nothing and reports false.
// It blocks that account until the reset the turn end's usage limit states.
// When the limit states none, the block lasts usageLimitFallback from now.
// A block already stored for a later time stays.
// With fewer than two accounts of positive weight it then reports false.
// Otherwise it decides under one hold of the account file's lock.
// Under a hub pin it changes nothing and reports false.
// The worktree may already resolve to another account with headroom, as after a sibling session's switch.
// Then it writes nothing and reports true.
// Otherwise it takes the best logged-in account but the launched one, with the pick's fallback off.
// It writes that account as the worktree's entry and reports true.
// With none eligible it changes nothing and reports false.
// Under a beams pin the switch writes the worktree's entry only,
// and the pinned account's block makes the resolution take that entry until the block passes.
func (c *Claude) SwitchAccount(runDir string, turnEnd shuttleengine.Event) (bool, error) {
	claude, err := loadConfigOrRefuse(c.dirs)
	if err != nil {
		return false, err
	}
	launched, recorded, err := readLaunchedAccount(runDir)
	if err != nil {
		return false, fmt.Errorf("claudeengine: read the account the run in %s launched under: %w", runDir, err)
	}
	worktree := c.worktree
	if !recorded || launched.Account == "" || worktree.AccountDir == "" {
		return false, nil
	}

	until := c.TurnEndActivity(turnEnd).UsageLimit.ResetsAt
	if until.IsZero() {
		until = c.now().Add(usageLimitFallback)
	}
	if err := recordBlocked(worktree.AccountDir, launched.Account, until); err != nil {
		return false, fmt.Errorf("claudeengine: record the usage limit of account %q: %w", launched.Account, err)
	}
	if worktree.Name == "" || positiveWeightAccounts(claude) < 2 {
		return false, nil
	}

	switched := false
	err = updateAccountFile(worktree.AccountDir, func(file accountFile) (accountFile, error) {
		if claude.AccountPins.Hub != "" {
			return file, nil
		}
		usage, err := readUsage(worktree.AccountDir, claude.Accounts)
		if err != nil {
			return accountFile{}, err
		}
		now := c.now()
		if resolved, ok := resolveAccount(claude, worktree, file, usage, now); ok && resolved.Name != launched.Account && hasHeadroom(resolved, usage[resolved.Name], now) {
			switched = true
			return file, nil
		}
		picked, account, ok := pickAccount(file, pickRequest{
			accounts: claude.Accounts,
			usage:    usage,
			loggedIn: func(account AccountConfig) bool { return loggedIn(runtime.GOOS, account) },
			exclude:  launched.Account,
			now:      now,
		})
		if !ok {
			return file, nil
		}
		if picked.Worktrees == nil {
			picked.Worktrees = map[string]string{}
		}
		if picked.Picked == nil {
			picked.Picked = map[string]time.Time{}
		}
		picked.Worktrees[worktree.Name] = account
		picked.Picked[worktree.Name] = now
		switched = true
		return picked, nil
	})
	if err != nil {
		return false, fmt.Errorf("claudeengine: switch the account of worktree %q: %w", worktree.Name, err)
	}
	return switched, nil
}

// positiveWeightAccounts returns how many accounts of claude take worktrees.
func positiveWeightAccounts(claude Config) int {
	positive := 0
	for _, account := range claude.Accounts {
		if account.Weight > 0 {
			positive++
		}
	}
	return positive
}
