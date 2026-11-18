// accountverbs_test.go tests the operator verbs of the account keeper: set, show and the status line reading.
// It runs on a temporary hub and fake account directories.

package claudeengine

import (
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

const (
	verbsTwoAccounts = "accounts:\n  - {name: a, config_dir: ROOT/a, weight: 1}\n  - {name: b, config_dir: ROOT/b, weight: 1}\n"
	verbsRetiredOld  = "  - {name: old, config_dir: ROOT/old, weight: 0}\n"
)

// verbsEngine returns an engine over a temp hub holding machineDocument, told worktree with a fresh account directory.
// The engine's clock reads now.
func verbsEngine(t *testing.T, machineDocument string, worktree shuttleengine.Worktree, now time.Time) (engine *Claude, accountDir string) {
	t.Helper()
	root := t.TempDir()
	accountDir = t.TempDir()
	worktree.AccountDir = accountDir
	engine, _ = engineWithAccounts(t, strings.ReplaceAll(machineDocument, "ROOT", root), worktree)
	engine.now = func() time.Time { return now }
	return engine, accountDir
}

func TestClaude_RecordStatusLine(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	now := time.Unix(3_000_000, 0)
	window := func(percentage float64, resetsIn time.Duration) string {
		return fmt.Sprintf(`{"used_percentage":%g,"resets_at":%d}`, percentage, now.Add(resetsIn).Unix())
	}
	limits := func(five, seven string) string {
		return fmt.Sprintf(`{"rate_limits":{"five_hour":%s,"seven_day":%s}}`, five, seven)
	}
	both := limits(window(40, 2*time.Hour), window(10, 72*time.Hour))
	bothWindows := map[string]usageWindow{
		"five_hour": {UsedPercentage: 40, ResetsAt: now.Add(2 * time.Hour).Unix()},
		"seven_day": {UsedPercentage: 10, ResetsAt: now.Add(72 * time.Hour).Unix()},
	}
	tests := []struct {
		name    string
		account string
		input   string
		// blocked seeds the account's record with a block until an hour after now.
		blocked bool
		wantErr bool
		// wantWindows is the stored record's windows; nil means no record is written.
		wantWindows map[string]usageWindow
		wantLine    string
	}{
		{name: "BothWindowsAreWrittenForAListedName", account: "a", input: both, wantWindows: bothWindows, wantLine: "a five_hour 40% seven_day 10%"},
		{name: "InputWithoutRateLimitsWritesNothing", account: "a", input: `{"model":{"id":"m"}}`, wantLine: "a"},
		{name: "AnUnlistedNameIsRefusedAndWritesNothing", account: "ghost", input: both, wantErr: true},
		{
			name: "APercentageOf101IsDroppedAndTheOtherWindowWritten", account: "a", input: limits(window(101, 2*time.Hour), window(10, 72*time.Hour)),
			wantWindows: map[string]usageWindow{"seven_day": bothWindows["seven_day"]}, wantLine: "a seven_day 10%",
		},
		{
			name: "AResetEightDaysAheadIsDroppedAndTheOtherWindowWritten", account: "a", input: limits(window(40, 2*time.Hour), window(10, 8*24*time.Hour)),
			wantWindows: map[string]usageWindow{"five_hour": bothWindows["five_hour"]}, wantLine: "a five_hour 40%",
		},
		{name: "TheStoredBlockedUntilSurvivesAWrite", account: "a", input: both, blocked: true, wantWindows: bothWindows, wantLine: "a five_hour 40% seven_day 10%"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			engine, accountDir := verbsEngine(t, verbsTwoAccounts, shuttleengine.Worktree{Name: "t", Kind: shuttleengine.WorktreeTask}, now)
			blockedUntil := now.Add(time.Hour)
			if tt.blocked {
				if err := recordBlocked(accountDir, "a", blockedUntil); err != nil {
					t.Fatalf("seed the block: %v", err)
				}
			}

			line, err := engine.RecordStatusLine(tt.account, []byte(tt.input))
			if (err != nil) != tt.wantErr {
				t.Fatalf("RecordStatusLine() error = %v; want error %v", err, tt.wantErr)
			}
			if line != tt.wantLine {
				t.Errorf("RecordStatusLine() = %q; want %q", line, tt.wantLine)
			}

			stored, err := readCacheRecord[usageRecord](usageFilePath(accountDir, "a"))
			if err != nil {
				t.Fatalf("read the stored record: %v", err)
			}
			if tt.wantWindows == nil && !tt.blocked {
				if _, statErr := os.Stat(usageFilePath(accountDir, "a")); !os.IsNotExist(statErr) {
					t.Errorf("usage record stat error = %v; want no record written", statErr)
				}
				return
			}
			if !reflect.DeepEqual(stored.Windows, tt.wantWindows) && !(len(stored.Windows) == 0 && len(tt.wantWindows) == 0) {
				t.Errorf("stored windows = %+v; want %+v", stored.Windows, tt.wantWindows)
			}
			if tt.blocked && !stored.BlockedUntil.Equal(blockedUntil) {
				t.Errorf("stored BlockedUntil = %v; want %v kept", stored.BlockedUntil, blockedUntil)
			}
		})
	}
}

func TestClaude_SetAccount(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	now := time.Unix(3_000_000, 0)
	base := verbsTwoAccounts + verbsRetiredOld
	task := shuttleengine.Worktree{Name: "t", Kind: shuttleengine.WorktreeTask}
	beam := shuttleengine.Worktree{Name: "t", Kind: shuttleengine.WorktreeBeam}
	tests := []struct {
		name     string
		machine  string
		worktree shuttleengine.Worktree
		account  string
		// wantErr lists the texts the refusal holds; empty means the set succeeds.
		wantErr []string
		// wantEntry is the account the worktree's entry names afterwards, empty for none.
		wantEntry string
		// wantResolved is the account the worktree resolves to afterwards.
		wantResolved string
	}{
		{name: "AnEntryIsWrittenAndResolved", machine: base, worktree: task, account: "b", wantEntry: "b", wantResolved: "b"},
		{name: "AnUnlistedAccountIsRefusedNamingTheListedOnes", machine: base, worktree: task, account: "ghost", wantErr: []string{`"ghost"`, "the listed accounts are a, b"}},
		{name: "ARetiredAccountIsRefused", machine: base, worktree: task, account: "old", wantErr: []string{"retired", "the listed accounts are a, b"}},
		{
			name: "AHubPinRefusesNamingThePin", machine: base + "account_pins:\n  hub: a\n", worktree: task, account: "b",
			wantErr: []string{"account_pins.hub", `"a"`, "lym config claude --layer machine"}, wantResolved: "a",
		},
		{name: "ABeamsPinWritesTheEntryWhichTheLivePinOutranks", machine: base + "account_pins:\n  beams: b\n", worktree: beam, account: "a", wantEntry: "a", wantResolved: "b"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			engine, accountDir := verbsEngine(t, tt.machine, tt.worktree, now)
			err := engine.SetAccount(tt.account)
			for _, want := range tt.wantErr {
				if err == nil || !strings.Contains(err.Error(), want) {
					t.Errorf("SetAccount(%q) error = %v; want it to hold %q", tt.account, err, want)
				}
			}
			if len(tt.wantErr) == 0 && err != nil {
				t.Fatalf("SetAccount(%q) error: %v", tt.account, err)
			}

			file, readErr := readAccountFile(accountDir)
			if readErr != nil {
				t.Fatalf("read the account file: %v", readErr)
			}
			if got := file.Worktrees[tt.worktree.Name]; got != tt.wantEntry {
				t.Errorf("the worktree's entry = %q; want %q", got, tt.wantEntry)
			}
			if tt.wantEntry != "" && !file.Picked[tt.worktree.Name].Equal(now) {
				t.Errorf("the entry's time = %v; want %v", file.Picked[tt.worktree.Name], now)
			}
			if tt.wantResolved != "" {
				claude, loadErr := LoadConfig(engine.dirs)
				if loadErr != nil {
					t.Fatalf("load claude.yaml: %v", loadErr)
				}
				resolved, ok := resolveAccount(claude, engine.worktree, file, nil, now)
				if !ok || resolved.Name != tt.wantResolved {
					t.Errorf("resolveAccount() = %q, %v; want %q", resolved.Name, ok, tt.wantResolved)
				}
			}
		})
	}

	t.Run("ShowAccountsNamesTheResolvedAccountThePinsAndEachAccount", func(t *testing.T) {
		t.Parallel()

		machine := "accounts:\n  - {name: a, config_dir: ROOT/a, weight: 2, windows: {five_hour: 80}}\n  - {name: b, config_dir: ROOT/b, weight: 1}\naccount_pins:\n  beams: b\n"
		engine, accountDir := verbsEngine(t, machine, task, now)
		if err := engine.SetAccount("a"); err != nil {
			t.Fatalf("SetAccount: %v", err)
		}
		reset := now.Add(time.Hour)
		if err := writeUsage(accountDir, "a", usageRecord{Windows: map[string]usageWindow{"five_hour": {UsedPercentage: 90, ResetsAt: reset.Unix()}}}); err != nil {
			t.Fatalf("seed usage: %v", err)
		}
		blocked := now.Add(2 * time.Hour)
		if err := recordBlocked(accountDir, "b", blocked); err != nil {
			t.Fatalf("seed the block: %v", err)
		}

		got, err := engine.ShowAccounts()
		if err != nil {
			t.Fatalf("ShowAccounts() error: %v", err)
		}
		want := strings.Join([]string{
			"worktree t (task): resolved account a",
			"hub pin: none",
			"beams pin: b",
			"a: weight 2, no headroom, blocked until none",
			"  five_hour: 90% used, threshold 80%, resets " + reset.UTC().Format(time.RFC3339),
			"b: weight 1, no headroom, blocked until " + blocked.UTC().Format(time.RFC3339),
		}, "\n")
		if got != want {
			t.Errorf("ShowAccounts() =\n%s\nwant\n%s", got, want)
		}
	})
}
