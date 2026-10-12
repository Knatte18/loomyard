// pausehold_test.go pins that `lyx loom start` refuses a covered run before it seeds anything, and holds the fixture that covers a receiver's run for the resume test too.

package loomcli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// coverRun makes c's worktree a task worktree of its hub and records a live hub-wide pause entry, returning the record's path.
func coverRun(t *testing.T, c *loomCLI) string {
	t.Helper()
	gitDir := filepath.Join(c.location.HubPath, "repo", ".git", "worktrees", c.location.WorktreeName)
	gitFile := filepath.Join(c.location.WorktreePath(), ".git")
	if err := os.WriteFile(gitFile, []byte("gitdir: "+gitDir+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	geometry, err := hubgeom.PauseGeometry(c.location)
	if err != nil {
		t.Fatalf("hubgeom.PauseGeometry: %v", err)
	}
	now := time.Now()
	if _, err := pauserecord.Update(geometry.RecordPath, now, func(record *pauserecord.Record) error {
		record.Live = append(record.Live, pauserecord.Entry{Scope: pauserecord.ScopeHub, PausedAt: now, Caller: "EXTERNAL"})
		return nil
	}); err != nil {
		t.Fatalf("pauserecord.Update: %v", err)
	}
	return geometry.RecordPath
}

// TestStartCmd_RefusesWhileAPauseCoversTheRun drives startCmd over a receiver whose hub holds a live pause entry:
// the refusal names `lyx resume` and nothing is seeded, and once the entry is lifted the same call no longer refuses on the pause.
func TestStartCmd_RefusesWhileAPauseCoversTheRun(t *testing.T) {
	t.Parallel()

	c, _ := newTestSpawnAndWaitReceiver(t, &fakeDriverStarter{}, &fakeDriverPaneProbeFull{strandsFn: noStrands})
	recordPath := coverRun(t, c)

	start := func() string {
		ctx, _ := clihelp.NewExitContext(context.Background())
		var out bytes.Buffer
		cmd := c.startCmd()
		cmd.SetContext(ctx)
		cmd.SetOut(&out)
		_ = cmd.RunE(cmd, nil)
		var env resumeEnvelope
		if err := json.Unmarshal(out.Bytes(), &env); err != nil {
			t.Fatalf("decode envelope %q: %v", out.String(), err)
		}
		return env.Error + env.Message
	}

	refusal := start()
	if !strings.Contains(refusal, "lyx resume") {
		t.Errorf("start under a pause answered %q; want it to name %q", refusal, "lyx resume")
	}
	if _, found, err := shedrun.ReadSeed(c.location, seedSlug(c.location.WorktreeName)); err != nil || found {
		t.Errorf("ReadSeed after the refusal = (found=%v, err=%v); want (false, nil), nothing seeded", found, err)
	}

	if _, err := pauserecord.Update(recordPath, time.Now(), func(record *pauserecord.Record) error {
		record.Live = nil
		return nil
	}); err != nil {
		t.Fatalf("pauserecord.Update: %v", err)
	}
	if lifted := start(); strings.Contains(lifted, "lyx resume") {
		t.Errorf("start after the pause was lifted answered %q; want no pause refusal", lifted)
	}
}
