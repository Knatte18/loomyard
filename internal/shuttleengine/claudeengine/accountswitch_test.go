// accountswitch_test.go tests how a usage limit is recorded against the launched account and moves the worktree's entry.

package claudeengine

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

const (
	// switchTwoAccounts lists the accounts a and b under ROOT.
	switchTwoAccounts = "accounts:\n  - {name: a, config_dir: ROOT/a, weight: 1}\n  - {name: b, config_dir: ROOT/b, weight: 1}\n"
	// switchLimitWithReset is the message of a usage limit that states its reset, written at switchLimitWrittenAt.
	switchLimitWithReset = "You've hit your weekly limit · resets 2pm (Europe/Oslo)"
	// switchLimitWithoutReset is the message of a usage limit that states no reset.
	switchLimitWithoutReset = "You've hit your limit"
	// switchLimitWrittenAt is the time the switch tests' limit entry was written.
	switchLimitWrittenAt = "2026-10-28T11:03:42.000Z"
)

// switchRow is one case of TestClaude_SwitchAccount.
type switchRow struct {
	name string
	// machine is the machine-layer claude.yaml; ROOT stands for the temp directory of the fake config directories.
	machine  string
	worktree shuttleengine.Worktree
	// launched names the account the run launched under; empty makes it a launch on a directory no account lists.
	launched string
	file     accountFile
	// blocked maps an account to the time a stored limit blocks it until.
	blocked map[string]time.Time
	// message is the usage-limit text of the turn end.
	message string
	// calls is how many sessions of the worktree hit the limit; zero means one.
	calls int
	// wantRelaunch is the result each call must give.
	wantRelaunch bool
	// check judges the state after the calls; nil judges nothing more.
	check func(t *testing.T, after switchState)
}

// switchState is what a switch left behind.
type switchState struct {
	engine   *Claude
	worktree shuttleengine.Worktree
	file     accountFile
	usage    map[string]usageRecord
	now      time.Time
}

// switchRig is a hub with two logged-in accounts and a run launched under one of them.
type switchRig struct {
	root, accountDir, runDir string
	engine                   *Claude
	turnEnd                  shuttleengine.Event
	now                      time.Time
	worktree                 shuttleengine.Worktree
}

// newSwitchRig builds the rig row describes.
// It seeds the hub's claude.yaml, the account file and usage, and the run's account.json.
// It writes a transcript whose newest turn end is a usage limit with row's message.
func newSwitchRig(t *testing.T, row switchRow) *switchRig {
	t.Helper()
	root, accountDir, runDir := t.TempDir(), t.TempDir(), t.TempDir()
	for _, name := range []string{"a", "b"} {
		writeClaudeLayer(t, filepath.Join(root, name, credentialsFileName), "{}")
	}
	worktree := row.worktree
	worktree.AccountDir = accountDir
	engine, _ := engineWithAccounts(t, strings.ReplaceAll(row.machine, "ROOT", root), worktree)
	now := time.Date(2026, 10, 28, 10, 0, 0, 0, time.UTC)
	engine.now = func() time.Time { return now }

	if err := updateAccountFile(accountDir, func(accountFile) (accountFile, error) { return row.file, nil }); err != nil {
		t.Fatalf("seed the account file: %v", err)
	}
	for account, until := range row.blocked {
		if err := recordBlocked(accountDir, account, until); err != nil {
			t.Fatalf("seed the block of account %s: %v", account, err)
		}
	}
	launched := launchedAccount{ConfigDir: filepath.Join(root, "other")}
	if row.launched != "" {
		launched = launchedAccount{ConfigDir: filepath.Join(root, row.launched), Account: row.launched}
	}
	if err := writeLaunchedAccount(runDir, launched); err != nil {
		t.Fatalf("write the run's account record: %v", err)
	}

	message := row.message
	if message == "" {
		message = switchLimitWithReset
	}
	entry := fmt.Sprintf(`{"type":"assistant","timestamp":%q,"isApiErrorMessage":true,"error":"rate_limit","message":{"stop_reason":"stop_sequence","content":[{"type":"text","text":%q}]}}`, switchLimitWrittenAt, message)
	transcript := filepath.Join(root, "transcript.jsonl")
	writeClaudeLayer(t, transcript, entry+"\n")
	raw, err := json.Marshal(map[string]string{"transcript_path": transcript})
	if err != nil {
		t.Fatalf("encode the turn end: %v", err)
	}
	return &switchRig{root: root, accountDir: accountDir, runDir: runDir, engine: engine, turnEnd: shuttleengine.Event{Kind: shuttleengine.EventStop, Raw: raw}, now: now, worktree: worktree}
}

// state reads the account file and the usage records the switches left.
func (rig *switchRig) state(t *testing.T) switchState {
	t.Helper()
	file, err := readAccountFile(rig.accountDir)
	if err != nil {
		t.Fatalf("read the account file: %v", err)
	}
	claude, err := LoadConfig(rig.engine.dirs)
	if err != nil {
		t.Fatalf("load claude.yaml: %v", err)
	}
	usage, err := readUsage(rig.accountDir, claude.Accounts)
	if err != nil {
		t.Fatalf("read the usage records: %v", err)
	}
	return switchState{engine: rig.engine, worktree: rig.worktree, file: file, usage: usage, now: rig.now}
}

// TestClaude_SwitchAccount pins what a usage limit records and when it moves the worktree's entry.
func TestClaude_SwitchAccount(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	statedReset := time.Date(2026, 10, 28, 13, 0, 0, 0, time.UTC)
	task := shuttleengine.Worktree{Name: "t", Kind: shuttleengine.WorktreeTask}
	beam := shuttleengine.Worktree{Name: "t", Kind: shuttleengine.WorktreeBeam}
	entryOn := func(account string) accountFile {
		return accountFile{Worktrees: map[string]string{"t": account}, Picked: map[string]time.Time{"t": time.Unix(1_000, 0)}}
	}
	// blockedUntil fails unless account's stored block ends at want.
	blockedUntil := func(t *testing.T, after switchState, account string, want time.Time) {
		t.Helper()
		if got := after.usage[account].BlockedUntil; !got.Equal(want) {
			t.Errorf("account %s blocked until %v; want %v", account, got, want)
		}
	}
	// entryIs fails unless the worktree's entry names account.
	entryIs := func(t *testing.T, after switchState, account string) {
		t.Helper()
		if got := after.file.Worktrees["t"]; got != account {
			t.Errorf("worktree t's entry = %q; want %q", got, account)
		}
	}

	tests := []switchRow{
		{
			name: "AHitWithAStatedResetBlocksTheLaunchedAccountAndMovesTheEntry", machine: switchTwoAccounts, worktree: task, launched: "a", file: entryOn("a"), wantRelaunch: true,
			check: func(t *testing.T, after switchState) {
				blockedUntil(t, after, "a", statedReset)
				entryIs(t, after, "b")
				if after.file.Counts["b"] != 1 || !after.file.Picked["t"].Equal(after.now) {
					t.Errorf("counts = %v, picked = %v; want b's count 1 and t picked at %v", after.file.Counts, after.file.Picked, after.now)
				}
				claude, err := LoadConfig(after.engine.dirs)
				if err != nil {
					t.Fatalf("load claude.yaml: %v", err)
				}
				if account, _ := resolveAccount(claude, after.worktree, after.file, after.usage, after.now); account.Name != "b" {
					t.Errorf("the worktree resolves to %q; want b", account.Name)
				}
				if hasHeadroom(claude.Accounts[0], after.usage["a"], after.now) {
					t.Error("account a has headroom before its reset; want none")
				}
				if !hasHeadroom(claude.Accounts[0], after.usage["a"], statedReset) {
					t.Error("account a has no headroom at its reset; want some")
				}
			},
		},
		{
			name: "AHitWithNoStatedResetBlocksForThirtyMinutes", machine: switchTwoAccounts, worktree: task, launched: "a", file: entryOn("a"), message: switchLimitWithoutReset, wantRelaunch: true,
			check: func(t *testing.T, after switchState) { blockedUntil(t, after, "a", after.now.Add(usageLimitFallback)) },
		},
		{
			name: "AnEarlierHitNeverLowersAStoredLaterBlock", machine: switchTwoAccounts, worktree: task, launched: "a", file: entryOn("a"), wantRelaunch: true,
			blocked: map[string]time.Time{"a": statedReset.Add(5 * time.Hour)},
			check:   func(t *testing.T, after switchState) { blockedUntil(t, after, "a", statedReset.Add(5*time.Hour)) },
		},
		{
			name: "NoOtherEligibleAccountLeavesTheEntryAndReportsFalse", machine: switchTwoAccounts, worktree: task, launched: "a", file: entryOn("a"),
			blocked: map[string]time.Time{"b": statedReset},
			check: func(t *testing.T, after switchState) {
				blockedUntil(t, after, "a", statedReset)
				entryIs(t, after, "a")
				if len(after.file.Counts) != 0 {
					t.Errorf("counts = %v; want none raised", after.file.Counts)
				}
			},
		},
		{
			name: "AHubPinRecordsTheLimitAndChangesNothingElse", machine: switchTwoAccounts + "account_pins:\n  hub: a\n", worktree: task, launched: "a", file: entryOn("a"),
			check: func(t *testing.T, after switchState) {
				blockedUntil(t, after, "a", statedReset)
				entryIs(t, after, "a")
			},
		},
		{
			name: "ABeamsPinnedWorktreeGetsAnEntryAndReturnsToThePinOnceItsBlockPasses", machine: switchTwoAccounts + "account_pins:\n  beams: a\n", worktree: beam, launched: "a", wantRelaunch: true,
			check: func(t *testing.T, after switchState) {
				entryIs(t, after, "b")
				claude, err := LoadConfig(after.engine.dirs)
				if err != nil {
					t.Fatalf("load claude.yaml: %v", err)
				}
				for _, tt := range []struct {
					at   time.Time
					want string
				}{{after.now, "b"}, {statedReset.Add(time.Minute), "a"}} {
					if got, _ := resolveAccount(claude, after.worktree, after.file, after.usage, tt.at); got.Name != tt.want {
						t.Errorf("at %v the worktree resolves to %q; want %q", tt.at, got.Name, tt.want)
					}
				}
			},
		},
		{
			name: "AWorktreeWithNoEntryOnAListedDirectorySwitchesToTheOtherAccount", machine: switchTwoAccounts, worktree: task, launched: "a", wantRelaunch: true,
			check: func(t *testing.T, after switchState) { entryIs(t, after, "b") },
		},
		{
			name: "TwoSessionsOfOneWorktreeMakeOneSwitch", machine: switchTwoAccounts, worktree: task, launched: "a", file: entryOn("a"), calls: 2, wantRelaunch: true,
			check: func(t *testing.T, after switchState) {
				entryIs(t, after, "b")
				if !reflect.DeepEqual(after.file.Counts, map[string]float64{"a": 0, "b": 1}) {
					t.Errorf("counts = %v; want b raised once", after.file.Counts)
				}
			},
		},
		{
			name: "AnUnlistedLaunchedDirectoryRecordsNothing", machine: switchTwoAccounts, worktree: task, file: entryOn("a"),
			check: func(t *testing.T, after switchState) {
				for account, record := range after.usage {
					if !record.BlockedUntil.IsZero() {
						t.Errorf("account %s blocked until %v; want no block", account, record.BlockedUntil)
					}
				}
				entryIs(t, after, "a")
			},
		},
		{
			name: "OneAccountOfPositiveWeightRecordsTheLimitAndReportsFalse", machine: "accounts:\n  - {name: a, config_dir: ROOT/a, weight: 1}\n  - {name: b, config_dir: ROOT/b, weight: 0}\n", worktree: task, launched: "a", file: entryOn("a"),
			check: func(t *testing.T, after switchState) {
				blockedUntil(t, after, "a", statedReset)
				entryIs(t, after, "a")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rig := newSwitchRig(t, tt)
			for call := 0; call < max(tt.calls, 1); call++ {
				got, err := rig.engine.SwitchAccount(rig.runDir, rig.turnEnd)
				if err != nil {
					t.Fatalf("SwitchAccount() call %d error: %v", call+1, err)
				}
				if got != tt.wantRelaunch {
					t.Errorf("SwitchAccount() call %d = %v; want %v", call+1, got, tt.wantRelaunch)
				}
			}
			if tt.check != nil {
				tt.check(t, rig.state(t))
			}
		})
	}

	t.Run("AfterASwitchAResumeLaunchesUnderTheNewAccountOnceItsTranscriptIsCarried", func(t *testing.T) {
		t.Parallel()

		rig := newSwitchRig(t, switchRow{machine: switchTwoAccounts, worktree: task, launched: "a", file: entryOn("a")})
		if switched, err := rig.engine.SwitchAccount(rig.runDir, rig.turnEnd); err != nil || !switched {
			t.Fatalf("SwitchAccount() = %v, %v; want true", switched, err)
		}
		workdir := filepath.Join(rig.root, "work")
		writeClaudeLayer(t, filepath.Join(projectDirUnder(filepath.Join(rig.root, "a", projectsDirName), workdir), resumeTestID+".jsonl"), "{}\n")

		resumeDir := t.TempDir()
		launch, err := rig.engine.Prepare(resumeDir, shuttleengine.Spec{Prompt: "carry on", PromptEngine: engineName, ResumeSessionID: resumeTestID}, shuttleengine.Config{})
		if err != nil {
			t.Fatalf("Prepare() error: %v", err)
		}
		wantDir := filepath.Join(rig.root, "b")
		if want := configDirAssignment(wantDir); !strings.Contains(launch.ResumeCmd, want) {
			t.Errorf("Launch.ResumeCmd = %q; want it to hold %q", launch.ResumeCmd, want)
		}
		if got, found, err := readLaunchedAccount(resumeDir); err != nil || !found || got != (launchedAccount{ConfigDir: wantDir, Account: "b"}) {
			t.Errorf("account.json = %+v, found %v, err %v; want account b", got, found, err)
		}
		settings, err := os.ReadFile(filepath.Join(resumeDir, settingsFileName))
		if err != nil {
			t.Fatalf("read settings.json: %v", err)
		}
		if !strings.Contains(string(settings), "claude statusline 'b'") {
			t.Errorf("settings.json = %q; want the status line of account b", settings)
		}

		if _, err := rig.engine.CheckResume(resumeTestID, workdir); !errors.Is(err, shuttleengine.ErrNoTranscript) {
			t.Fatalf("CheckResume() before the carry = %v; want ErrNoTranscript", err)
		}
		if copied, err := rig.engine.CarryTranscript(resumeTestID, workdir); err != nil || !copied {
			t.Fatalf("CarryTranscript() = %v, %v; want true", copied, err)
		}
		if _, err := rig.engine.CheckResume(resumeTestID, workdir); err != nil {
			t.Errorf("CheckResume() after the carry error = %v; want nil", err)
		}
	})
}
