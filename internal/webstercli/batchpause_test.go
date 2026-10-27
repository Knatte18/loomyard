//go:build tier1

// batchpause_test.go covers the pause refusal begin-batch, recover-batch, fix-verify and fix-plan make before they read or change any state, and the lookup of Master's strand it claims.

package webstercli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/spf13/cobra"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
)

const undecodableRecord = "{not json"

// TestBatchVerbs_RefuseUnderAPause drives the batch verbs under a live hub entry with Master's strand lookup stubbed.
// It does not run in parallel: envMasterStrand's part sets an environment variable, and the logger is process-global.
func TestBatchVerbs_RefuseUnderAPause(t *testing.T) {
	const (
		masterName = "ly:pair:master"
		masterGUID = "guid-master"
	)
	claimedMaster := []pauserecord.Session{{Strand: masterName, GUID: masterGUID, Slug: "pair"}}
	okStrand := func(string) func() (string, string, error) {
		return func() (string, string, error) { return masterName, masterGUID, nil }
	}
	failingStrand := func(string) func() (string, string, error) {
		return func() (string, string, error) { return masterName, "", errors.New("no such strand") }
	}
	rewriteRecord := func(t *testing.T, path string, mutate func(*pauserecord.Record)) {
		t.Helper()
		if _, err := pauserecord.Update(path, time.Now(), func(record *pauserecord.Record) error {
			mutate(record)
			return nil
		}); err != nil {
			t.Fatalf("Update() = %v", err)
		}
	}
	refusedGoingOn := func(verb string) []string {
		return []string{"a hub-wide pause holds this pair, so " + verb + " starts no new batch; way forward: end your turn now; a running batch finishes, and once the resume line arrives run \"lym webster " + verb + " 1\" again"}
	}
	refusedNoLine := func(verb string) []string {
		return []string{"a hub-wide pause holds this pair, so " + verb + " starts no new batch, and no resume line will come; way forward: end your turn now, then run \"lym webster " + verb + " 1\" again once \"lym tender status\" shows no pause"}
	}

	tests := []struct {
		name         string
		verb         string
		undecodable  bool
		masterStrand func(recordPath string) func() (string, string, error)
		wantExit     int
		wantText     []string
		wantSessions []pauserecord.Session
		wantWarn     string
		wantRecord   string
		stateKept    bool
	}{
		{name: "begin-batch claims Master and refuses", verb: "begin-batch", masterStrand: okStrand, wantExit: 1, wantText: refusedGoingOn("begin-batch"), wantSessions: claimedMaster, stateKept: true},
		{name: "recover-batch claims Master and refuses", verb: "recover-batch", masterStrand: okStrand, wantExit: 1, wantText: refusedGoingOn("recover-batch"), wantSessions: claimedMaster, stateKept: true},
		{
			name: "fix-verify claims Master and refuses", verb: "fix-verify", masterStrand: okStrand, wantExit: 1, wantSessions: claimedMaster, stateKept: true,
			wantText: []string{"a hub-wide pause holds this pair, so fix-verify spawns no cold fixer; way forward: end your turn now; once the resume line arrives run \"lym webster fix-verify\" again"},
		},
		{
			name: "fix-plan claims Master and refuses", verb: "fix-plan", masterStrand: okStrand, wantExit: 1, wantSessions: claimedMaster, stateKept: true,
			wantText: []string{"a hub-wide pause holds this pair, so fix-plan spawns no plan fixer; way forward: end your turn now; once the resume line arrives run \"lym webster fix-plan\" again"},
		},
		{name: "begin-batch refuses without a claim when Master's strand is unknown", verb: "begin-batch", masterStrand: failingStrand, wantExit: 1, wantText: refusedNoLine("begin-batch"), wantWarn: "Master could not be claimed", stateKept: true},
		{name: "recover-batch refuses without a claim when Master's strand is unknown", verb: "recover-batch", masterStrand: failingStrand, wantExit: 1, wantText: refusedNoLine("recover-batch"), wantWarn: "Master could not be claimed", stateKept: true},
		{
			name: "begin-batch whose record turns undecodable before the claim refuses without a claim", verb: "begin-batch",
			masterStrand: func(path string) func() (string, string, error) {
				return func() (string, string, error) {
					if err := os.WriteFile(path, []byte(undecodableRecord), 0o644); err != nil {
						t.Errorf("WriteFile(%q) = %v", path, err)
					}
					return masterName, masterGUID, nil
				}
			},
			wantExit: 1, wantText: refusedNoLine("begin-batch"), wantWarn: "Master could not be claimed", wantRecord: undecodableRecord, stateKept: true,
		},
		{
			name: "begin-batch whose entry is lifted before the claim goes on", verb: "begin-batch",
			masterStrand: func(path string) func() (string, string, error) {
				return func() (string, string, error) {
					rewriteRecord(t, path, func(record *pauserecord.Record) {
						record.Resumed, record.Live = record.Live, nil
					})
					return masterName, masterGUID, nil
				}
			},
			wantExit: 0, wantText: []string{`"batch":"01-only"`},
		},
		{
			name: "begin-batch over an undecodable record goes on and warns once", verb: "begin-batch", undecodable: true,
			masterStrand: func(string) func() (string, string, error) {
				return func() (string, string, error) {
					t.Error("masterStrand was called over a record that read as uncovered")
					return "", "", errors.New("unreachable")
				}
			},
			wantExit: 0, wantText: []string{`"batch":"01-only"`}, wantWarn: "the pause record could not be read", wantRecord: undecodableRecord,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("FABRIC_SKIP_GIT", "1")
			logs := logcapture.Capture(t)
			fx := newVerbsFixture(t)
			fx.initState(t)
			recordPath := filepath.Join(t.TempDir(), pauserecord.RecordFileName)
			fx.CLI.pause = pauserecord.Geometry{RecordPath: recordPath, Slug: "pair"}
			fx.Runner.SetPause(fx.CLI.pause)
			fx.CLI.masterStrand = tt.masterStrand(recordPath)
			if tt.undecodable {
				if err := os.WriteFile(recordPath, []byte(undecodableRecord), 0o644); err != nil {
					t.Fatalf("WriteFile(%q) = %v", recordPath, err)
				}
			} else {
				rewriteRecord(t, recordPath, func(record *pauserecord.Record) {
					record.Live = []pauserecord.Entry{{Scope: pauserecord.ScopeHub, PausedAt: time.Now().Add(-time.Minute)}}
				})
			}
			statePath := filepath.Join(fx.CLI.geom.WebsterDir, "state.json")
			stateBefore, err := os.ReadFile(statePath)
			if err != nil {
				t.Fatalf("ReadFile(%q) = %v", statePath, err)
			}

			cmd, args := fx.CLI.beginBatchCmd(), []string{"1"}
			switch tt.verb {
			case "recover-batch":
				cmd = fx.CLI.recoverBatchCmd()
			case "fix-verify":
				cmd, args = fx.CLI.fixVerifyCmd(), nil
			case "fix-plan":
				cmd, args = fx.CLI.fixPlanCmd(), nil
			}
			var out strings.Builder
			exitCode := clihelp.Execute(cmd, &out, args)

			if exitCode != tt.wantExit {
				t.Fatalf("%s %v = %d; want %d, output: %s", tt.verb, args, exitCode, tt.wantExit, out.String())
			}
			body := out.String()
			var envelope struct {
				Error string `json:"error"`
			}
			if json.Unmarshal([]byte(body), &envelope) == nil && envelope.Error != "" {
				body = envelope.Error
			}
			for _, want := range tt.wantText {
				if !strings.Contains(body, want) {
					t.Errorf("output missing %q; got %q", want, body)
				}
			}
			if strings.Contains(body, "lym tender resume") {
				t.Errorf("output %q names `lym tender resume`, which would lift the operator's pause", body)
			}
			if tt.wantRecord != "" {
				if got, err := os.ReadFile(recordPath); err != nil || string(got) != tt.wantRecord {
					t.Errorf("pause record = %q, %v; want it left as %q", got, err, tt.wantRecord)
				}
			} else if tt.wantExit != 0 {
				record, err := pauserecord.Read(recordPath)
				if err != nil {
					t.Fatalf("Read() = %v", err)
				}
				var sessions []pauserecord.Session
				for _, entry := range record.Live {
					sessions = append(sessions, entry.Sessions...)
				}
				if diff := cmp.Diff(tt.wantSessions, sessions); diff != "" {
					t.Errorf("recorded sessions mismatch (-want +got):\n%s", diff)
				}
			}
			if tt.stateKept {
				if stateAfter, err := os.ReadFile(statePath); err != nil || !bytes.Equal(stateBefore, stateAfter) {
					t.Errorf("state.json changed by a refused %s: %v", tt.verb, err)
				}
			}
			wantWarns := 0
			if tt.wantWarn != "" {
				wantWarns = 1
			}
			if got := strings.Count(logs.String(), "Master could not be claimed") + strings.Count(logs.String(), "the pause record could not be read"); got != wantWarns {
				t.Errorf("%d warnings (want %d containing %q); logs: %s", got, wantWarns, tt.wantWarn, logs.String())
			}
			if tt.wantWarn != "" && !strings.Contains(logs.String(), tt.wantWarn) {
				t.Errorf("logs lack %q; got %s", tt.wantWarn, logs.String())
			}
			if tt.wantWarn == "Master could not be claimed" && !strings.Contains(logs.String(), masterName) {
				t.Errorf("the warning does not name the strand %s; got %s", masterName, logs.String())
			}
		})
	}

	t.Run("the other verbs are not refused by the pause", func(t *testing.T) {
		t.Setenv("FABRIC_SKIP_GIT", "1")
		fx := newVerbsFixture(t)
		fx.initState(t)
		recordPath := filepath.Join(t.TempDir(), pauserecord.RecordFileName)
		fx.CLI.pause = pauserecord.Geometry{RecordPath: recordPath, Slug: "pair"}
		fx.CLI.masterStrand = func() (string, string, error) {
			t.Error("masterStrand was called by a verb that takes no pause check")
			return "", "", errors.New("unreachable")
		}
		rewriteRecord(t, recordPath, func(record *pauserecord.Record) {
			record.Live = []pauserecord.Entry{{Scope: pauserecord.ScopeHub, PausedAt: time.Now().Add(-time.Minute)}}
		})
		verbs := []struct {
			name string
			cmd  *cobra.Command
			args []string
		}{
			{"record-batch", fx.CLI.recordBatchCmd(), []string{"1"}},
			{"await-batch", fx.CLI.awaitBatchCmd(), []string{"1", "--wait", "1ms"}},
			{"status", fx.CLI.statusCmd(), nil},
		}
		for _, verb := range verbs {
			var out strings.Builder
			clihelp.Execute(verb.cmd, &out, verb.args)
			if strings.Contains(out.String(), "holds this pair") {
				t.Errorf("%s was refused by the pause: %s", verb.name, out.String())
			}
		}
	})

	t.Run("envMasterStrand reads the strand-name variable and resolves its guid", func(t *testing.T) {
		var resolved string
		lookup := envMasterStrand(func(name string) (string, error) {
			resolved = name
			return "guid-for-" + name, nil
		})

		t.Setenv(agentname.StrandNameEnv, "")
		if _, _, err := lookup(); err == nil || !strings.Contains(err.Error(), agentname.StrandNameEnv) {
			t.Errorf("lookup() with the variable empty = %v; want an error naming %s", err, agentname.StrandNameEnv)
		}

		t.Setenv(agentname.StrandNameEnv, masterName)
		name, guid, err := lookup()
		if err != nil || name != masterName || guid != "guid-for-"+masterName || resolved != masterName {
			t.Errorf("lookup() = (%q, %q, %v) after resolving %q; want (%q, %q, nil)", name, guid, err, resolved, masterName, "guid-for-"+masterName)
		}

		failing := errors.New("no such strand")
		if _, _, err := envMasterStrand(func(string) (string, error) { return "", failing })(); !errors.Is(err, failing) {
			t.Errorf("lookup() whose resolve fails = %v; want %v", err, failing)
		}
	})
}
