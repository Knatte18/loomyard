//go:build integration

// shortname_integration_test.go drives `lyx fabric shortname [<shortname>]` against a real hub from hubforge:
// it prints the recorded shortname, records one on a hub whose record was removed, treats the same shortname as a no-op, and refuses a different one.
//
// Package fabriccli_test, sharing the single TestMain in testmain_test.go.

package fabriccli_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabriccli"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/hubforge"
)

// runShortnameVerb runs `fabric shortname <args>` from the hub's prime worktree and decodes the envelope.
func runShortnameVerb(t *testing.T, h *hubforge.Hub, args ...string) (exit int, env map[string]any) {
	t.Helper()
	var out bytes.Buffer
	exit = fabriccli.RunCLIIn(h.PrimeWorktree(), &out, append([]string{"shortname"}, args...))
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("decode shortname envelope: %v\noutput: %s", err, out.String())
	}
	return exit, env
}

// removeShortnameRecord deletes .lyx-shortname from the board and commits the removal, leaving a hub with no shortname.
func removeShortnameRecord(t *testing.T, h *hubforge.Hub) {
	t.Helper()
	board := h.BoardDir()
	if err := os.Remove(filepath.Join(board, fabricengine.ShortnameFileName)); err != nil {
		t.Fatalf("remove %s: %v", fabricengine.ShortnameFileName, err)
	}
	if _, _, err := fabricengine.NewBolt(board).Commit("test: remove the shortname record", fabricengine.SyncOptions{}); err != nil {
		t.Fatalf("commit the removal: %v", err)
	}
}

// boardGit runs git in the board worktree and returns trimmed stdout.
func boardGit(t *testing.T, h *hubforge.Hub, args ...string) string {
	t.Helper()
	out, err := gitexec.Run(args, h.BoardDir())
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return strings.TrimSpace(out)
}

func TestShortnameVerb_PrintsRecordedShortname(t *testing.T) {
	h := hubforge.NewHub(t, ".")

	exit, env := runShortnameVerb(t, h)
	if exit != 0 || env["ok"] != true {
		t.Fatalf("shortname = %d, %v; want exit 0 ok", exit, env)
	}
	if env["shortname"] != hubforge.TestShortname {
		t.Errorf("shortname = %v; want %q", env["shortname"], hubforge.TestShortname)
	}
}

func TestShortnameVerb_RecordsOnAHubWithNoShortname(t *testing.T) {
	h := hubforge.NewHub(t, ".")
	removeShortnameRecord(t, h)

	exit, env := runShortnameVerb(t, h)
	if exit == 0 || !strings.Contains(env["error"].(string), "lyx fabric shortname <shortname>") {
		t.Fatalf("no-arg on a hub with no shortname = %d, %v; want a refusal naming `lyx fabric shortname <shortname>`", exit, env)
	}

	exit, env = runShortnameVerb(t, h, "zz")
	if exit != 0 || env["shortname"] != "zz" || env["partial"] != false {
		t.Fatalf("record = %d, %v; want exit 0 with shortname zz", exit, env)
	}
	if got := boardGit(t, h, "show", "HEAD:"+fabricengine.ShortnameFileName); got != "zz" {
		t.Errorf("%s at weft:main's tip = %q; want zz", fabricengine.ShortnameFileName, got)
	}
}

// A pending board write is not this verb's to commit: the record lands alone and the board change stays uncommitted.
func TestShortnameVerb_CommitsTheRecordAlone(t *testing.T) {
	h := hubforge.NewHub(t, ".")
	removeShortnameRecord(t, h)
	pending := filepath.Join(h.BoardDir(), "pending-board-write.md")
	if err := os.WriteFile(pending, []byte("half-written\n"), 0o644); err != nil {
		t.Fatalf("write pending board file: %v", err)
	}

	if exit, env := runShortnameVerb(t, h, "zz"); exit != 0 {
		t.Fatalf("record = %d, %v; want exit 0", exit, env)
	}
	if got := boardGit(t, h, "show", "--name-only", "--format=", "HEAD"); got != fabricengine.ShortnameFileName {
		t.Errorf("files in the shortname commit = %q; want only %s", got, fabricengine.ShortnameFileName)
	}
	if got := boardGit(t, h, "status", "--porcelain", "--", "pending-board-write.md"); got == "" {
		t.Errorf("the pending board file was committed; want it left uncommitted")
	}
}

func TestShortnameVerb_SameShortnameIsANoOp(t *testing.T) {
	h := hubforge.NewHub(t, ".")
	before := boardGit(t, h, "rev-parse", "HEAD")

	exit, env := runShortnameVerb(t, h, hubforge.TestShortname)
	if exit != 0 || env["shortname"] != hubforge.TestShortname {
		t.Fatalf("same shortname = %d, %v; want exit 0", exit, env)
	}
	if after := boardGit(t, h, "rev-parse", "HEAD"); after != before {
		t.Errorf("weft:main moved from %s to %s; want no new commit", before, after)
	}
}

func TestShortnameVerb_DifferentShortnameRefusesNamingTheRecordedOne(t *testing.T) {
	h := hubforge.NewHub(t, ".")

	exit, env := runShortnameVerb(t, h, "zz")
	if exit == 0 || env["ok"] != false {
		t.Fatalf("different shortname = %d, %v; want a refusal", exit, env)
	}
	if !strings.Contains(env["error"].(string), `"`+hubforge.TestShortname+`"`) {
		t.Errorf("error = %q; want it to name the recorded shortname %q", env["error"], hubforge.TestShortname)
	}
}
