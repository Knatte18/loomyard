//go:build integration

// pushretry_integration_test.go pins, against a real bare remote that refuses every pushed branch with a remote "cannot lock ref", the ordering the fake push in pushretry_test.go cannot reach: the retry exhausts before the rebase-retry or the ErrPushRejected mapping is consulted.

package gitrepo

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/gitkit"
)

// hookLogPlaceholder stands for the hook log's path in procReceiveRefuseScript until a test substitutes it.
const hookLogPlaceholder = "@HOOK_LOG@"

// procReceiveRefuseScript answers a proc-receive request by appending one line to the log at hookLogPlaceholder and refusing every command with a remote "cannot lock ref".
// It speaks the pkt-line protocol with dd, because git holds the hook's stdin open until the answer is written.
const procReceiveRefuseScript = `#!/bin/sh
echo run >> '` + hookLogPlaceholder + `'
read_packet() {
	length=$(dd bs=1 count=4 2>/dev/null)
	if [ "$length" = "0000" ]; then
		packet=
		return 1
	fi
	packet=$(dd bs=1 count=$((0x$length - 4)) 2>/dev/null)
	return 0
}
write_packet() {
	printf '%04x%s\n' $((${#1} + 5)) "$1"
}
while read_packet; do :; done
write_packet "version=1"
printf '0000'
refs=
while read_packet; do
	set -- $packet
	refs="$refs $3"
done
for ref in $refs; do
	write_packet "ng $ref cannot lock ref '$ref': reference is busy"
done
printf '0000'
`

func TestPlainPushPrimitives_ExhaustedRetryWrapsSentinelAndNeverRebases(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		push func(r *Repo) error
	}{
		{"Push", func(r *Repo) error { return r.Push() }},
		{"PushRebaseFree", func(r *Repo) error { return r.PushRebaseFree() }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			container := t.TempDir()
			bare := filepath.Join(container, "remote.git")
			gitkit.MustRun(t, container, "git", "init", "--bare", "-b", "main", bare)
			clonePath := filepath.Join(container, "clone")
			gitkit.MustRun(t, container, "git", "clone", bare, clonePath)
			gitkit.CommitFile(t, clonePath, "first.txt", "first", "first commit")
			gitkit.Git(t, clonePath, "push", "origin", "HEAD:main")

			hookLog := filepath.Join(container, "hook.log")
			hook := filepath.Join(bare, "hooks", "proc-receive")
			script := strings.ReplaceAll(procReceiveRefuseScript, hookLogPlaceholder, filepath.ToSlash(hookLog))
			if err := os.WriteFile(hook, []byte(script), 0o755); err != nil {
				t.Fatalf("write proc-receive hook: %v", err)
			}
			gitkit.Git(t, bare, "config", "receive.procReceiveRefs", "refs/heads")

			gitkit.CommitFile(t, clonePath, "second.txt", "second", "second commit")
			headBefore := gitkit.RevParse(t, clonePath, "HEAD")

			var sleeps []time.Duration
			repo := New(clonePath)
			repo.pushSleep = func(delay time.Duration) { sleeps = append(sleeps, delay) }

			err := tc.push(repo)

			if !errors.Is(err, ErrPushRetriesExhausted) {
				t.Errorf("err = %v; want it to wrap ErrPushRetriesExhausted", err)
			}
			if errors.Is(err, ErrPushRejected) {
				t.Errorf("err = %v; must not map to ErrPushRejected", err)
			}
			logged, readErr := os.ReadFile(hookLog)
			if readErr != nil {
				t.Fatalf("read hook log: %v", readErr)
			}
			if runs := strings.Count(string(logged), "run"); runs != pushMaxAttempts {
				t.Errorf("hook ran %d times; want %d", runs, pushMaxAttempts)
			}
			wantSleeps := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second}
			if !reflect.DeepEqual(sleeps, wantSleeps) {
				t.Errorf("sleeps = %v; want %v", sleeps, wantSleeps)
			}
			if headAfter := gitkit.RevParse(t, clonePath, "HEAD"); headAfter != headBefore {
				t.Errorf("HEAD moved from %s to %s; the retry must not rebase", headBefore, headAfter)
			}
			for _, rebaseDir := range []string{"rebase-merge", "rebase-apply"} {
				if _, statErr := os.Stat(filepath.Join(clonePath, ".git", rebaseDir)); statErr == nil {
					t.Errorf("a rebase is in progress (%s exists)", rebaseDir)
				}
			}
		})
	}
}
