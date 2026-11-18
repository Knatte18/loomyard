// accountkeeper.go implements shuttleengine.AccountKeeper for the Claude engine: the pick of a new worktree's account.

package claudeengine

import (
	"fmt"
	"runtime"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// Compile-time proof that Claude can pick a worktree's account.
var _ shuttleengine.AccountKeeper = (*Claude)(nil)

// PickAccount picks the account the told worktree runs under, when the worktree is made on this machine.
// It loads claude.yaml and returns its error.
// It does nothing when fewer than two accounts have a positive weight.
// It does nothing when the worktree is the prime or has no name.
// It does nothing under a hub pin, and under a beams pin for a beam.
// Otherwise it picks under one hold of the account file's lock, with the pick's fallback on.
// It drops the entry of every worktree the hub no longer holds.
// It keeps the entry when an earlier pick wrote it after this call began.
// So two picks at once leave one entry and one count.
// With keepEntry set it also keeps an entry the file already holds.
// With no account to take it removes the worktree's entry, and the worktree runs as it would without accounts.
// A failure to read the hub's worktree names refuses the pick and writes nothing.
func (c *Claude) PickAccount(keepEntry bool) error {
	claude, err := loadConfigOrRefuse(c.dirs)
	if err != nil {
		return err
	}
	if !c.picksAccounts(claude) {
		return nil
	}
	worktree := c.worktree
	noted := c.now()
	return updateAccountFile(worktree.AccountDir, func(file accountFile) (accountFile, error) {
		names, err := worktree.Names()
		if err != nil {
			return accountFile{}, fmt.Errorf("read the hub's worktree names: %w", err)
		}
		file = dropAbsentWorktrees(file, names)
		if file.Picked[worktree.Name].After(noted) {
			return file, nil
		}
		if _, held := file.Worktrees[worktree.Name]; keepEntry && held {
			return file, nil
		}
		usage, err := readUsage(worktree.AccountDir, claude.Accounts)
		if err != nil {
			return accountFile{}, err
		}
		written := c.now()
		picked, account, ok := pickAccount(file, pickRequest{
			accounts: claude.Accounts,
			usage:    usage,
			loggedIn: func(account AccountConfig) bool { return loggedIn(runtime.GOOS, account) },
			fallback: true,
			now:      written,
		})
		if !ok {
			logger.Info("claudeengine: no Claude account can take the worktree, so it has no entry", "worktree", worktree.Name)
			delete(file.Worktrees, worktree.Name)
			delete(file.Picked, worktree.Name)
			return file, nil
		}
		if picked.Worktrees == nil {
			picked.Worktrees = map[string]string{}
		}
		if picked.Picked == nil {
			picked.Picked = map[string]time.Time{}
		}
		picked.Worktrees[worktree.Name] = account
		picked.Picked[worktree.Name] = written
		return picked, nil
	})
}

// picksAccounts reports whether the told worktree takes part in the spread of accounts under claude.
func (c *Claude) picksAccounts(claude Config) bool {
	positive := 0
	for _, account := range claude.Accounts {
		if account.Weight > 0 {
			positive++
		}
	}
	worktree := c.worktree
	switch {
	case positive < 2, worktree.Name == "", worktree.Kind == shuttleengine.WorktreePrime:
		return false
	case claude.AccountPins.Hub != "":
		return false
	case claude.AccountPins.Beams != "" && worktree.Kind == shuttleengine.WorktreeBeam:
		return false
	}
	return true
}

// dropAbsentWorktrees returns file without the entry of any worktree names does not list.
func dropAbsentWorktrees(file accountFile, names []string) accountFile {
	present := make(map[string]bool, len(names))
	for _, name := range names {
		present[name] = true
	}
	for name := range file.Worktrees {
		if !present[name] {
			delete(file.Worktrees, name)
		}
	}
	for name := range file.Picked {
		if !present[name] {
			delete(file.Picked, name)
		}
	}
	return file
}
