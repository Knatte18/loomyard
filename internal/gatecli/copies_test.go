// copies_test.go covers `lym gate test --copies`: the lease it takes, the copies it runs and the refusals around it.

package gatecli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestRunTierCopies(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("the tier command is a POSIX shell command")
	}

	tests := []struct {
		name                         string
		poolSlots, tierSlots, copies int
		// oneRed makes exactly one copy exit 1.
		oneRed    bool
		inherited bool
		// guardStep makes the run a guard step of the call.
		guardStep bool
		wantRed   bool
		// wantRuns are the run labels of the started lines, in order.
		wantRuns []string
		// wantClamp is the clamp line the call prints, empty when it prints none.
		wantClamp string
		// wantRefusal is the text of the envelope error the call answers, empty when it runs.
		wantRefusal string
	}{
		{
			name: "TwoCopiesRunInsideOneLease", poolSlots: 2, tierSlots: 1, copies: 2,
			wantRuns: []string{"tier0 copy 1", "tier0 copy 2"},
		},
		{
			name: "CopiesAboveThePoolAreClamped", poolSlots: 2, tierSlots: 1, copies: 3,
			wantRuns: []string{"tier0 copy 1", "tier0 copy 2"}, wantClamp: copiesClampLine(3, 2, "tier0"),
		},
		{
			name: "TierWithAllSlotsRunsOneCopy", poolSlots: 2, tierSlots: 0, copies: 2,
			wantRuns: []string{"tier0 copy 1"}, wantClamp: copiesClampLine(2, 1, "tier0"),
		},
		{
			name: "RedCopyMakesTheCallRed", poolSlots: 2, tierSlots: 1, copies: 2, oneRed: true, wantRed: true,
			wantRuns: []string{"tier0 copy 1", "tier0 copy 2"},
		},
		{
			name: "InheritedSlotRefusesCopies", poolSlots: 2, tierSlots: 1, copies: 2, inherited: true,
			wantRefusal: "way forward: run the call outside a held gate slot",
		},
		{
			name: "GuardStepRunsOnceWhateverTheCopies", poolSlots: 2, tierSlots: 1, copies: 2, guardStep: true,
			wantRuns: []string{"guard tier0"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			command := "exit 0"
			if tt.oneRed {
				// A noclobber redirect creates the file with O_EXCL, so exactly one of the concurrent copies wins;
				// the mkdir of a coreutils rewrite can succeed for two callers at once.
				command = fmt.Sprintf("if (set -C; : > %q) 2>/dev/null; then exit 1; fi", filepath.Join(dir, "first"))
			}
			pool := &gateslot.Pool{
				Dir:    t.TempDir(),
				Limits: func() (gateslot.Limits, error) { return gateslot.Limits{Slots: tt.poolSlots}, nil },
				Poll:   time.Millisecond,
			}
			var out, stderr bytes.Buffer
			exe := &execution{
				ctx: context.Background(),
				out: &out,
				request: testRequest{
					root: dir, stderr: &stderr, copies: tt.copies, timeoutUnit: 30 * time.Second, pipeWaitDelay: defaultPipeWaitDelay,
				},
				location:  &lymcwd.Location{HubPath: t.TempDir(), WorktreeName: "work"},
				pool:      pool,
				inherited: tt.inherited,
				env:       []string{"PATH=" + os.Getenv("PATH")},
				runLogDir: t.TempDir(),
				callID:    "copies",
			}
			run := tierRun{
				number:      0,
				tier:        gateslot.Tier{Command: command, Counter: counterGoJSON, TimeoutMin: 1, Slots: tt.tierSlots},
				invocations: []invocation{{dir: dir}},
				guardStep:   tt.guardStep,
			}

			end := exe.runWithSlots(run, false, func(env []string, _ time.Time) tierResult {
				return runTier(exe.ctx, io.Discard, run, env, exe.request)
			})

			if tt.wantRefusal != "" {
				if !end.halt || end.code != 1 || !strings.Contains(out.String(), tt.wantRefusal) {
					t.Errorf("runWithSlots() = %+v, out = %q; want a halt with exit 1 and an error holding %q", end, out.String(), tt.wantRefusal)
				}
				return
			}
			if (end.code != 0) != tt.wantRed {
				t.Errorf("runWithSlots() exit = %d; want red %v", end.code, tt.wantRed)
			}
			var gotRuns []string
			for _, started := range gateslot.ParseStartedLines(stderr.String()) {
				if started.Started != 0 {
					t.Errorf("started line of %q counts %d test binaries; want 0 for a command that starts none", started.Run, started.Started)
				}
				gotRuns = append(gotRuns, started.Run)
			}
			if !reflect.DeepEqual(gotRuns, tt.wantRuns) {
				t.Errorf("started lines name %q; want %q", gotRuns, tt.wantRuns)
			}
			if clamp := strings.Contains(stderr.String(), "gate test: --copies asked for"); clamp != (tt.wantClamp != "") || (clamp && !strings.Contains(stderr.String(), tt.wantClamp)) {
				t.Errorf("stderr = %q; want clamp line %q", stderr.String(), tt.wantClamp)
			}
			if tt.oneRed {
				if len(exe.failed) != 1 || !strings.HasPrefix(exe.failed[0].label, "tier0 copy ") {
					t.Errorf("failed runs = %+v; want exactly one labelled as a copy of tier0", exe.failed)
				}
			}
			if holders, err := pool.Holders(); err != nil || len(holders) != 0 {
				t.Errorf("Holders() = %v, %v after the call; want no slot held", holders, err)
			}
		})
	}

	keyTests := []struct {
		name        string
		tiers       []int
		wantRefusal string
	}{
		{name: "KeyNamingTwoTiersIsRefused", tiers: []int{0, 1}, wantRefusal: "set the key to one tier below tier3 with lym config gate --set from the prime"},
		{name: "KeyNamingTheBilledTierIsRefused", tiers: []int{3}, wantRefusal: "set the key to one tier below tier3 with lym config gate --set from the prime"},
		{name: "KeyNamingOneLowerTierPasses", tiers: []int{1}},
	}
	for _, tt := range keyTests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg := gateslot.Config{
				Tiers: map[string]gateslot.Tier{"tier0": {}, "tier1": {}, "tier3": {}},
				Gates: map[string]gateslot.GateKey{"live": {Tiers: tt.tiers, Scope: "diff"}},
			}
			got := gateKeyRefusal(cfg, testRequest{gate: "live", copies: 2})
			if tt.wantRefusal == "" {
				if got != "" {
					t.Errorf("gateKeyRefusal() = %q; want none", got)
				}
				return
			}
			if !strings.Contains(got, `"live"`) || !strings.Contains(got, "way forward: "+tt.wantRefusal) {
				t.Errorf("gateKeyRefusal() = %q; want the key and %q", got, "way forward: "+tt.wantRefusal)
			}
		})
	}
}

func TestGreenForCall(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	green := &greenRun{}
	tests := []struct {
		name   string
		copies int
		want   *greenRun
	}{
		{"CallWithoutCopiesKeepsItsGreenRecord", 0, green},
		{"CopiesCallGetsNone", 2, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := greenForCall(green, testRequest{copies: tt.copies}); got != tt.want {
				t.Errorf("greenForCall(green, copies %d) = %p; want %p", tt.copies, got, tt.want)
			}
		})
	}
}

func TestMergeCopies(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	launch := gateslot.PackageLaunch{}
	merged := mergeCopies([]tierResult{
		{record: gateslot.RunRecord{Launches: []gateslot.PackageLaunch{launch}}},
		{record: gateslot.RunRecord{Launches: []gateslot.PackageLaunch{launch, launch}, ExitCode: 1}},
	})

	if got := len(merged.record.Launches); got != 3 {
		t.Errorf("merged launches = %d; want the copies' 1 and 2 summed to 3", got)
	}
	if merged.record.ExitCode != 1 || merged.record.Outcome != "failed" || len(merged.copies) != 2 {
		t.Errorf("merged record = exit %d, outcome %q, %d copies; want the first failing exit 1, failed, and both copies kept", merged.record.ExitCode, merged.record.Outcome, len(merged.copies))
	}
}

// TestRunTierCopies_BuildsThePrebuiltLymOnce is not parallel:
// it writes and then executes a stand-in go binary.
// A fork in a parallel test could hold the script's write handle open.
func TestRunTierCopies_BuildsThePrebuiltLymOnce(t *testing.T) {
	tiergate.Require(t, 0)
	if runtime.GOOS == "windows" {
		t.Skip("the tier command and the stand-in go binary are POSIX shell scripts")
	}

	dir := t.TempDir()
	builds := filepath.Join(dir, "builds")
	goBinary := filepath.Join(dir, "go")
	// The build's arguments are `build -C <root> -p <n> -trimpath -o <bin> ./cmd/lym`, so $8 is the binary's path.
	script := fmt.Sprintf("#!/bin/sh\necho built >> %q\n: > \"$8\"\n", builds)
	if err := os.WriteFile(goBinary, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	// A root without cmd/lym builds nothing.
	if err := os.MkdirAll(filepath.Join(dir, "cmd", "lym"), 0o755); err != nil {
		t.Fatal(err)
	}
	run := tierRun{
		number: 0,
		tier: gateslot.Tier{
			Command: fmt.Sprintf(`test -e "$%s"`, gateslot.PrebuiltLymEnv), Counter: counterGoJSON, TimeoutMin: 1, PrebuiltLym: true,
		},
		invocations: []invocation{{dir: dir}},
	}
	request := testRequest{
		root: dir, goBinary: goBinary, stderr: io.Discard, copiesRun: 2, timeoutUnit: 30 * time.Second, pipeWaitDelay: defaultPipeWaitDelay,
	}

	result := runTierCopies(context.Background(), run, []string{"PATH=" + os.Getenv("PATH")}, request)

	if result.refusal != "" || result.record.ExitCode != 0 || len(result.copies) != 2 {
		t.Fatalf("runTierCopies() = refusal %q, exit %d, %d copies, log %q; want two green copies that saw the prebuilt lym", result.refusal, result.record.ExitCode, len(result.copies), result.log)
	}
	if content, err := os.ReadFile(builds); err != nil || strings.Count(string(content), "built") != 1 {
		t.Errorf("builds record = %q, %v; want the one build both copies share", content, err)
	}
}
