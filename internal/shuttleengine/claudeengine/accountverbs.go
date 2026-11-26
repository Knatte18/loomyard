// accountverbs.go implements the operator verbs of shuttleengine.AccountKeeper for the Claude engine:
// setting the told worktree's account, showing the accounts, and recording a status line's usage reading.

package claudeengine

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

// statusLineWindowLengths is the length of each usage window a status line reports.
// A reset further ahead than its window's length cannot be genuine.
var statusLineWindowLengths = map[string]time.Duration{
	"five_hour": 5 * time.Hour,
	"seven_day": 7 * 24 * time.Hour,
}

// readAccountState reads the account file and the usage records of claude's accounts.
// It reads them from the told worktree's account directory.
// An engine told no account directory reads the zero file and no usage.
func (c *Claude) readAccountState(claude Config) (accountFile, map[string]usageRecord, error) {
	dir := c.worktree.AccountDir
	if dir == "" {
		return accountFile{}, map[string]usageRecord{}, nil
	}
	file, err := readAccountFile(dir)
	if err != nil {
		return accountFile{}, nil, fmt.Errorf("claudeengine: read the Claude account file: %w", err)
	}
	usage, err := readUsage(dir, claude.Accounts)
	if err != nil {
		return accountFile{}, nil, fmt.Errorf("claudeengine: %w", err)
	}
	return file, usage, nil
}

// activeAccountNames returns the names of the accounts of claude that take worktrees, in list order.
func activeAccountNames(claude Config) []string {
	var names []string
	for _, account := range claude.Accounts {
		if !account.retired() {
			names = append(names, account.Name)
		}
	}
	return names
}

// listedAccountsText names the accounts a refusal offers in place of the one asked for.
func listedAccountsText(claude Config) string {
	names := activeAccountNames(claude)
	if len(names) == 0 {
		return "no account is listed"
	}
	return "the listed accounts are " + strings.Join(names, ", ")
}

// SetAccount implements shuttleengine.AccountKeeper.
// It writes the told worktree's entry in the account file under one hold of its lock.
// The entry takes effect at the worktree's next launch the engine builds, and a running strand keeps its account.
// It refuses an account the list lacks or has retired, naming the listed ones.
// It refuses under a hub pin, naming the pin.
// Under a beams pin it writes the entry, which takes effect while the pin does not apply.
// An engine told no worktree refuses.
func (c *Claude) SetAccount(name string) error {
	claude, err := loadConfigOrRefuse(c.dirs)
	if err != nil {
		return err
	}
	worktree := c.worktree
	if worktree.Name == "" || worktree.AccountDir == "" {
		return errors.New("claudeengine: no worktree is told, so no account is set for it; way forward: run the verb from a worktree of a hub")
	}
	index := slices.IndexFunc(claude.Accounts, func(account AccountConfig) bool { return account.Name == name })
	switch {
	case index < 0:
		return fmt.Errorf("claudeengine: %s lists no account %q; %s; way forward: pick one of them, or add the account with `lym config claude --layer machine`", keyName(accountsKey), name, listedAccountsText(claude))
	case claude.Accounts[index].retired():
		return fmt.Errorf("claudeengine: account %q is retired with weight 0; %s; way forward: pick one of them", name, listedAccountsText(claude))
	case claude.AccountPins.Hub != "":
		return fmt.Errorf("claudeengine: %s names %q and holds every worktree of the hub to it; way forward: change the pin with `lym config claude --layer machine`", keyName(accountPinsKey+".hub"), claude.AccountPins.Hub)
	}
	written := c.now()
	return updateAccountFile(worktree.AccountDir, func(file accountFile) (accountFile, error) {
		if file.Worktrees == nil {
			file.Worktrees = map[string]string{}
		}
		if file.Picked == nil {
			file.Picked = map[string]time.Time{}
		}
		file.Worktrees[worktree.Name] = name
		file.Picked[worktree.Name] = written
		return file, nil
	})
}

// ShowAccounts implements shuttleengine.AccountKeeper.
// It returns one line per fact, joined by newlines.
// The facts are the told worktree's resolved account and the two pins.
// For each listed account they are its weight, whether it has headroom and when a limit blocks it until.
// Under each account come its stored windows, each with the used percentage, the account's threshold and the reset time.
func (c *Claude) ShowAccounts() (string, error) {
	claude, err := loadConfigOrRefuse(c.dirs)
	if err != nil {
		return "", err
	}
	file, usage, err := c.readAccountState(claude)
	if err != nil {
		return "", err
	}
	now := c.now()

	resolved := "none"
	if account, ok := resolveAccount(claude, c.worktree, file, usage, now); ok {
		resolved = account.Name
	}
	worktree := "no worktree is told"
	if c.worktree.Name != "" {
		worktree = fmt.Sprintf("worktree %s (%s)", c.worktree.Name, c.worktree.Kind)
	}
	pin := func(name string) string {
		if name == "" {
			return "none"
		}
		return name
	}
	lines := []string{
		fmt.Sprintf("%s: resolved account %s", worktree, resolved),
		"hub pin: " + pin(claude.AccountPins.Hub),
		"beams pin: " + pin(claude.AccountPins.Beams),
	}
	for _, account := range claude.Accounts {
		record := usage[account.Name]
		headroom := "headroom"
		if !hasHeadroom(account, record, now) {
			headroom = "no headroom"
		}
		blocked := "none"
		if !record.BlockedUntil.IsZero() {
			blocked = record.BlockedUntil.UTC().Format(time.RFC3339)
		}
		lines = append(lines, fmt.Sprintf("%s: weight %d, %s, blocked until %s", account.Name, account.Weight, headroom, blocked))
		for _, window := range accountWindows {
			reading, stored := record.Windows[window]
			if !stored {
				continue
			}
			threshold := "no threshold"
			if listed, ok := account.Windows[window]; ok {
				threshold = fmt.Sprintf("threshold %d%%", listed)
			}
			lines = append(lines, fmt.Sprintf("  %s: %g%% used, %s, resets %s", window, reading.UsedPercentage, threshold, time.Unix(reading.ResetsAt, 0).UTC().Format(time.RFC3339)))
		}
	}
	return strings.Join(lines, "\n"), nil
}

// RecordStatusLine implements shuttleengine.AccountKeeper.
// It merges the usage windows of the status line payload input into the usage record of the listed account name.
// The record's blocked-until time is kept.
// It returns one short line naming the account and each window's percentage.
// It refuses a name no listed account has, and writes nothing.
// A payload with no readable window writes nothing and returns the name alone.
func (c *Claude) RecordStatusLine(name string, input []byte) (string, error) {
	claude, err := loadConfigOrRefuse(c.dirs)
	if err != nil {
		return "", err
	}
	if !slices.ContainsFunc(claude.Accounts, func(account AccountConfig) bool { return account.Name == name }) {
		var listed []string
		for _, account := range claude.Accounts {
			listed = append(listed, account.Name)
		}
		return "", fmt.Errorf("claudeengine: %s lists no account %q, so nothing is recorded; the listed names are %s; way forward: run the status line with a listed account's name", keyName(accountsKey), name, strings.Join(listed, ", "))
	}
	dir := c.worktree.AccountDir
	if dir == "" {
		return "", errors.New("claudeengine: no worktree is told, so no usage is recorded; way forward: run the verb from a worktree of a hub")
	}
	record, ok := parseStatusLine(input, c.now())
	if ok {
		if err := writeUsage(dir, name, record); err != nil {
			return "", fmt.Errorf("claudeengine: record the usage of account %q: %w", name, err)
		}
	}
	line := name
	for _, window := range accountWindows {
		if reading, kept := record.Windows[window]; kept {
			line += fmt.Sprintf(" %s %g%%", window, reading.UsedPercentage)
		}
	}
	return line, nil
}

// parseStatusLine reads the usage windows of a status line payload.
// It takes `rate_limits.five_hour` and `rate_limits.seven_day`, each with its `used_percentage` and `resets_at`.
// It drops a window that lacks either value.
// It drops a window whose percentage lies outside 0 to 100.
// It drops a window whose reset lies further ahead of now than the window's length.
// It reports false when no window is left, and for a payload that does not decode.
func parseStatusLine(input []byte, now time.Time) (usageRecord, bool) {
	var payload struct {
		RateLimits map[string]struct {
			UsedPercentage *float64 `json:"used_percentage"`
			ResetsAt       *int64   `json:"resets_at"`
		} `json:"rate_limits"`
	}
	if err := json.Unmarshal(input, &payload); err != nil {
		return usageRecord{}, false
	}
	record := usageRecord{Windows: map[string]usageWindow{}}
	for window, length := range statusLineWindowLengths {
		reading, ok := payload.RateLimits[window]
		if !ok || reading.UsedPercentage == nil || reading.ResetsAt == nil {
			continue
		}
		if *reading.UsedPercentage < 0 || *reading.UsedPercentage > 100 {
			continue
		}
		if time.Unix(*reading.ResetsAt, 0).After(now.Add(length)) {
			continue
		}
		record.Windows[window] = usageWindow{UsedPercentage: *reading.UsedPercentage, ResetsAt: *reading.ResetsAt}
	}
	return record, len(record.Windows) > 0
}
