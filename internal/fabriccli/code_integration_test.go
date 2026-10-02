//go:build integration

// code_integration_test.go drives `lyx fabric code [<code>]` against a real hub from hubforge:
// it prints the recorded code, records one on a hub whose record was removed, treats the same code as a no-op, and refuses a different one.
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

// runCodeVerb runs `fabric code <args>` from the hub's prime worktree and decodes the envelope.
func runCodeVerb(t *testing.T, h *hubforge.Hub, args ...string) (exit int, env map[string]any) {
	t.Helper()
	var out bytes.Buffer
	exit = fabriccli.RunCLIIn(h.PrimeWorktree(), &out, append([]string{"code"}, args...))
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("decode code envelope: %v\noutput: %s", err, out.String())
	}
	return exit, env
}

// removeCodeRecord deletes .lyx-code from the board and commits the removal, leaving an uncoded hub.
func removeCodeRecord(t *testing.T, h *hubforge.Hub) {
	t.Helper()
	board := h.BoardDir()
	if err := os.Remove(filepath.Join(board, fabricengine.CodeFileName)); err != nil {
		t.Fatalf("remove %s: %v", fabricengine.CodeFileName, err)
	}
	if _, _, err := fabricengine.NewBolt(board).Commit("test: remove the code record", fabricengine.SyncOptions{}); err != nil {
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

func TestCodeVerb_PrintsRecordedCode(t *testing.T) {
	h := hubforge.NewHub(t, ".")

	exit, env := runCodeVerb(t, h)
	if exit != 0 || env["ok"] != true {
		t.Fatalf("code = %d, %v; want exit 0 ok", exit, env)
	}
	if env["code"] != hubforge.TestCode {
		t.Errorf("code = %v; want %q", env["code"], hubforge.TestCode)
	}
}

func TestCodeVerb_RecordsOnAnUncodedHub(t *testing.T) {
	h := hubforge.NewHub(t, ".")
	removeCodeRecord(t, h)

	exit, env := runCodeVerb(t, h)
	if exit == 0 || !strings.Contains(env["error"].(string), "lyx fabric code <code>") {
		t.Fatalf("no-arg on an uncoded hub = %d, %v; want a refusal naming `lyx fabric code <code>`", exit, env)
	}

	exit, env = runCodeVerb(t, h, "zz")
	if exit != 0 || env["code"] != "zz" || env["partial"] != false {
		t.Fatalf("record = %d, %v; want exit 0 with code zz", exit, env)
	}
	if got := boardGit(t, h, "show", "HEAD:"+fabricengine.CodeFileName); got != "zz" {
		t.Errorf("%s at weft:main's tip = %q; want zz", fabricengine.CodeFileName, got)
	}
}

// A pending board write is not this verb's to commit: the record lands alone and the board change stays uncommitted.
func TestCodeVerb_CommitsTheRecordAlone(t *testing.T) {
	h := hubforge.NewHub(t, ".")
	removeCodeRecord(t, h)
	pending := filepath.Join(h.BoardDir(), "pending-board-write.md")
	if err := os.WriteFile(pending, []byte("half-written\n"), 0o644); err != nil {
		t.Fatalf("write pending board file: %v", err)
	}

	if exit, env := runCodeVerb(t, h, "zz"); exit != 0 {
		t.Fatalf("record = %d, %v; want exit 0", exit, env)
	}
	if got := boardGit(t, h, "show", "--name-only", "--format=", "HEAD"); got != fabricengine.CodeFileName {
		t.Errorf("files in the code commit = %q; want only %s", got, fabricengine.CodeFileName)
	}
	if got := boardGit(t, h, "status", "--porcelain", "--", "pending-board-write.md"); got == "" {
		t.Errorf("the pending board file was committed; want it left uncommitted")
	}
}

func TestCodeVerb_SameCodeIsANoOp(t *testing.T) {
	h := hubforge.NewHub(t, ".")
	before := boardGit(t, h, "rev-parse", "HEAD")

	exit, env := runCodeVerb(t, h, hubforge.TestCode)
	if exit != 0 || env["code"] != hubforge.TestCode {
		t.Fatalf("same code = %d, %v; want exit 0", exit, env)
	}
	if after := boardGit(t, h, "rev-parse", "HEAD"); after != before {
		t.Errorf("weft:main moved from %s to %s; want no new commit", before, after)
	}
}

func TestCodeVerb_DifferentCodeRefusesNamingTheRecordedOne(t *testing.T) {
	h := hubforge.NewHub(t, ".")

	exit, env := runCodeVerb(t, h, "zz")
	if exit == 0 || env["ok"] != false {
		t.Fatalf("different code = %d, %v; want a refusal", exit, env)
	}
	if !strings.Contains(env["error"].(string), `"`+hubforge.TestCode+`"`) {
		t.Errorf("error = %q; want it to name the recorded code %q", env["error"], hubforge.TestCode)
	}
}
