// accountconfig.go declares claude.yaml's account keys, `accounts` and `account_pins`.
// It also holds the checks that keep them machine-local and valid.

package claudeengine

import (
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/Knatte18/loomyard/internal/configengine"
)

const (
	// accountsKey and accountPinsKey are the claude.yaml keys that hold the machine's Claude accounts and their pins.
	accountsKey    = "accounts"
	accountPinsKey = "account_pins"
)

// accountMachineWayForward is the way forward every account refusal ends with.
const accountMachineWayForward = "set it with `lym config claude --layer machine`"

// accountWindows are the usage windows an account's `windows` map may name.
var accountWindows = []string{"five_hour", "seven_day"}

// accountNamePattern is the shape of an account's name.
var accountNamePattern = regexp.MustCompile(`^[a-z0-9-]+$`)

// AccountConfig is one Claude account logged in on this machine.
type AccountConfig struct {
	// Name identifies the account in the account file, the pins and the verbs.
	Name string `yaml:"name"`
	// ConfigDir is the account's Claude config directory; a leading `~/` is expanded to the home directory.
	ConfigDir string `yaml:"config_dir"`
	// Weight is the account's share of the worktrees picked onto it; 0 retires the account.
	Weight int `yaml:"weight"`
	// Windows maps a usage window name to the used percentage at or above which the account is passed over in that window.
	Windows map[string]int `yaml:"windows"`
}

// retired reports whether the account takes no new worktree, which a weight of 0 says.
func (a AccountConfig) retired() bool {
	return a.Weight == 0
}

// AccountPins names the accounts a pin holds the worktrees of a hub or its beams to; an empty name pins nothing.
type AccountPins struct {
	// Hub is the account every worktree of the hub uses.
	Hub string `yaml:"hub"`
	// Beams is the account every beam uses.
	Beams string `yaml:"beams"`
}

// canonicalConfigDir returns the form two config directories are compared in:
// the cleaned path with its symlinks resolved, and the cleaned path alone when it does not resolve.
func canonicalConfigDir(dir string) string {
	cleaned := filepath.Clean(dir)
	if resolved, err := filepath.EvalSymlinks(cleaned); err == nil {
		return resolved
	}
	return cleaned
}

// refuseTrackedAccountKeys refuses a non-empty account key in the default or worktree layer.
// Those files are tracked, and an account list or pin is a fact of one machine.
func refuseTrackedAccountKeys(dirs configengine.Dirs) error {
	docs, err := configengine.Layers(dirs, ConfigSpec())
	if err != nil {
		return err
	}
	for _, doc := range docs {
		if doc.Layer != configengine.LayerDefault && doc.Layer != configengine.LayerWorktree {
			continue
		}
		var keys struct {
			Accounts    []yaml.Node `yaml:"accounts"`
			AccountPins AccountPins `yaml:"account_pins"`
		}
		if err := yaml.Unmarshal(doc.Bytes, &keys); err != nil {
			return configengine.MarkInvalid(fmt.Errorf("unmarshal claude config file %s: %w", doc.Path, err))
		}
		held := map[string]bool{
			accountsKey:               len(keys.Accounts) > 0,
			accountPinsKey + ".hub":   keys.AccountPins.Hub != "",
			accountPinsKey + ".beams": keys.AccountPins.Beams != "",
		}
		for _, key := range []string{accountsKey, accountPinsKey + ".hub", accountPinsKey + ".beams"} {
			if held[key] {
				return configengine.MarkInvalid(fmt.Errorf(
					"claude config file %s sets %s, which is machine-local and never tracked; remove the key from that file with `lym config claude --layer %s`, and %s",
					doc.Path, key, doc.Layer, accountMachineWayForward))
			}
		}
	}
	return nil
}

// validateAccounts checks the account list and the pins of claude.
// It rewrites each ConfigDir to its expanded, cleaned form.
// home expands a leading `~/` of a config directory.
// Every refusal is marked configengine.ErrInvalid and names the key and the way forward.
func validateAccounts(claude *Config, home string) error {
	refuse := func(format string, args ...any) error {
		return configengine.MarkInvalid(fmt.Errorf(format+"; "+accountMachineWayForward, args...))
	}
	names := map[string]bool{}
	dirs := map[string]string{}
	for i := range claude.Accounts {
		account := &claude.Accounts[i]
		if !accountNamePattern.MatchString(account.Name) {
			return refuse("claude.yaml %s[%d] name %q must match [a-z0-9-]+", accountsKey, i, account.Name)
		}
		if names[account.Name] {
			return refuse("claude.yaml %s name %q is listed twice", accountsKey, account.Name)
		}
		names[account.Name] = true

		dir := account.ConfigDir
		if dir == "~" || strings.HasPrefix(dir, "~/") {
			if home == "" {
				return refuse("claude.yaml %s %q config_dir %q starts with ~/ and the home directory is unresolved", accountsKey, account.Name, account.ConfigDir)
			}
			dir = filepath.Join(home, strings.TrimPrefix(dir, "~"))
		}
		if !filepath.IsAbs(dir) {
			return refuse("claude.yaml %s %q config_dir %q must be an absolute path or start with ~/", accountsKey, account.Name, account.ConfigDir)
		}
		account.ConfigDir = filepath.Clean(dir)
		canonical := canonicalConfigDir(account.ConfigDir)
		if other, taken := dirs[canonical]; taken {
			return refuse("claude.yaml %s %q and %q name the same config_dir %q", accountsKey, other, account.Name, account.ConfigDir)
		}
		dirs[canonical] = account.Name

		if account.Weight < 0 {
			return refuse("claude.yaml %s %q weight %d must not be negative", accountsKey, account.Name, account.Weight)
		}
		windowNames := make([]string, 0, len(account.Windows))
		for window := range account.Windows {
			windowNames = append(windowNames, window)
		}
		sort.Strings(windowNames)
		for _, window := range windowNames {
			if !slices.Contains(accountWindows, window) {
				return refuse("claude.yaml %s %q windows names %q; want one of %s", accountsKey, account.Name, window, strings.Join(accountWindows, ", "))
			}
			if threshold := account.Windows[window]; threshold < 1 || threshold > 100 {
				return refuse("claude.yaml %s %q windows %s threshold %d must be from 1 to 100", accountsKey, account.Name, window, threshold)
			}
		}
	}
	for _, pin := range []struct{ key, name string }{
		{accountPinsKey + ".hub", claude.AccountPins.Hub},
		{accountPinsKey + ".beams", claude.AccountPins.Beams},
	} {
		if pin.name == "" {
			continue
		}
		index := slices.IndexFunc(claude.Accounts, func(a AccountConfig) bool { return a.Name == pin.name })
		if index < 0 {
			return refuse("claude.yaml %s names %q, which %s does not list", pin.key, pin.name, accountsKey)
		}
		if claude.Accounts[index].retired() {
			return refuse("claude.yaml %s names %q, which is retired with weight 0", pin.key, pin.name)
		}
	}
	return nil
}
