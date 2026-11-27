// clone_singlerepo_killedpush_integration_test.go drives --single-repo seeds whose push client is killed mid-push.
// Its tier2 test is kept apart from the tier1 clone tests.
// It shares their helpers in clone_singlerepo_integration_test.go.

package fabricengine_test

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// killPushClientHook returns a receive hook that kills the client `git push` when it is handed an update of ref.
// It sends SIGKILL to the client it finds by walking up from receive-pack through /proc, then exits with exitCode.
// It ignores a deletion of ref.
// It runs the shell lines beforeKill, if any, just before the kill.
// Installed as post-receive, it kills the client after origin applied the update.
// Installed as pre-receive with a non-zero exitCode, it also declines the update, so origin never applies it.
func killPushClientHook(ref string, exitCode int, beforeKill string) string {
	return fmt.Sprintf(`#!/bin/sh
while read old new ref; do
	[ "$ref" = "%s" ] || continue
	case "$new" in *[!0]*) ;; *) continue ;; esac
	pid=$PPID
	while [ "$pid" -gt 1 ]; do
		if tr '\0' ' ' < /proc/$pid/cmdline | grep -q '^git push '; then
			%s
			kill -KILL "$pid"
			exit %d
		fi
		pid=$(awk '/^PPid:/ { print $2 }' /proc/$pid/status)
	done
done
exit 0
`, ref, beforeKill, exitCode)
}

// TestCloneSingleRepo_SeedPushKilledBeforeOriginsAnswer kills the client of one seed push before origin answers it.
// A push origin had applied must be withdrawn, leaving no hub and nothing under origin's records namespace.
// A push origin had not applied may still land,
// so the hub must be kept and the error must name the refs origin may hold.
// A board origin holds at another tip must stay, and the error must name a join.
func TestCloneSingleRepo_SeedPushKilledBeforeOriginsAnswer(t *testing.T) {
	tiergate.Require(t, 2)
	if runtime.GOOS != "linux" {
		t.Skip("the hook finds the pushing client through /proc, which only Linux has")
	}
	t.Parallel()

	const recordsRef = namespaceHeadsPrefix + "main-weft"
	tests := []struct {
		name string
		// hook is the receive hook the kill runs in.
		hook string
		// killedRef is the origin ref whose push is killed.
		killedRef string
		// hookExit is the hook's exit code after the kill; non-zero in a pre-receive hook declines the update.
		hookExit int
		// beforeKill is shell the hook runs on origin just before the kill.
		beforeKill string
		// wantHubKept reports whether the clone must keep its hub.
		wantHubKept bool
		// wantErrorHas are substrings of the clone's error.
		wantErrorHas []string
		// wantNamespace is what origin's records namespace must hold afterwards.
		wantNamespace []string
	}{
		{
			name:      "BoardLanded",
			hook:      "post-receive",
			killedRef: namespaceBoardRef,
			wantErrorHas: []string{
				"the seed push of the board failed",
				"origin took the records branch and, after its push ended without an answer, the board; the seed deleted the board",
				"the records branch was returned to what origin held before",
				"way forward: run the same clone again",
			},
		},
		{
			name:       "BoardMovedToAnotherTipBeforeTheKill",
			hook:       "post-receive",
			killedRef:  namespaceBoardRef,
			beforeKill: `git update-ref ` + namespaceBoardRef + ` "$(git rev-parse refs/heads/main)"`,
			wantErrorHas: []string{
				"origin's board stands at another tip after the board's push ended without an answer",
				"origin now holds another hub's board",
				"run `lym fabric clone <url>` to join that hub",
			},
			wantNamespace: []string{namespaceBoardRef},
		},
		{
			name:      "RecordsBranchLanded",
			hook:      "post-receive",
			killedRef: recordsRef,
			wantErrorHas: []string{
				"the seed push of the records branch failed",
				"origin took the records branch after its push ended without an answer, and the seed withdrew it",
				"way forward: run the same clone again",
			},
		},
		{
			name:        "BoardNotApplied",
			hook:        "pre-receive",
			killedRef:   namespaceBoardRef,
			hookExit:    1,
			wantHubKept: true,
			wantErrorHas: []string{
				"the seed push of the board ended without origin's answer",
				"origin may hold " + namespaceBoardRef + " and " + recordsRef + ";",
				"ls-remote origin " + namespaceBoardRef + " " + recordsRef + "`",
			},
			wantNamespace: []string{recordsRef},
		},
		{
			name:        "RecordsBranchNotApplied",
			hook:        "pre-receive",
			killedRef:   recordsRef,
			hookExit:    1,
			wantHubKept: true,
			wantErrorHas: []string{
				"the seed push of the records branch ended without origin's answer",
				"origin may hold " + recordsRef + ";",
				"ls-remote origin " + recordsRef + "`",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			bare := hubforge.CopyCodeBare(t, "")
			hook := killPushClientHook(tt.killedRef, tt.hookExit, tt.beforeKill)
			if err := os.WriteFile(filepath.Join(bare, "hooks", tt.hook), []byte(hook), 0o755); err != nil {
				t.Fatalf("write the %s hook: %v", tt.hook, err)
			}
			container := t.TempDir()

			_, err := cloneSingleRepo(t, container, bare, "")
			if err == nil {
				t.Fatalf("clone whose push of %s was killed = nil error; want a failure", tt.killedRef)
			}
			for _, want := range tt.wantErrorHas {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("clone error = %q; want it to contain %q", err.Error(), want)
				}
			}
			if !tt.wantHubKept {
				requireEmptyDirectory(t, container)
			} else if entries, readErr := os.ReadDir(container); readErr != nil || len(entries) != 1 {
				t.Errorf("the container holds %v (read error %v); want the one kept hub", entries, readErr)
			} else if kept := "the hub at " + filepath.Join(container, entries[0].Name()) + " is kept"; !strings.Contains(err.Error(), kept) {
				t.Errorf("clone error = %q; want it to contain %q", err.Error(), kept)
			}
			if got := refLines(t, bare, "%(refname)", fabriclayout.RecordsNamespaceRoot); !slices.Equal(got, tt.wantNamespace) {
				t.Errorf("origin's records namespace = %v; want %v", got, tt.wantNamespace)
			}
		})
	}
}
