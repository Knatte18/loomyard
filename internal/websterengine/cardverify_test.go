//go:build integration

// cardverify_test.go exercises rerunCardVerifies against real shell commands.

package websterengine

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/planparser"
)

func verifyCard(n int, slug, command string) planparser.Card {
	return planparser.Card{Number: n, Slug: slug, Verify: command, HasVerify: command != ""}
}

func TestRerunCardVerifies_AllPass(t *testing.T) {
	cards := []planparser.Card{verifyCard(1, "a", "true"), verifyCard(2, "b", "")}
	if got := rerunCardVerifies(cards, t.TempDir(), 0); len(got) != 0 {
		t.Fatalf("failures = %v, want none", got)
	}
}

func TestRerunCardVerifies_NonZeroExit(t *testing.T) {
	cards := []planparser.Card{verifyCard(1, "a", "true"), verifyCard(2, "b", "exit 3")}
	got := rerunCardVerifies(cards, t.TempDir(), 0)
	if len(got) != 1 || got[0] != "card 02-b verify exit 3 exited 3" {
		t.Fatalf("failures = %v", got)
	}
}

func TestRerunCardVerifies_Timeout(t *testing.T) {
	cards := []planparser.Card{verifyCard(4, "slow", "sleep 5")}
	got := rerunCardVerifies(cards, t.TempDir(), 200*time.Millisecond)
	if len(got) != 1 || !strings.Contains(got[0], "timed out after 200ms") {
		t.Fatalf("failures = %v", got)
	}
}

func TestRerunCardVerifies_MissingDir(t *testing.T) {
	cards := []planparser.Card{verifyCard(5, "gone", "true")}
	got := rerunCardVerifies(cards, filepath.Join(t.TempDir(), "nope"), 0)
	if len(got) != 1 || !strings.Contains(got[0], "card 05-gone verify true ") {
		t.Fatalf("failures = %v", got)
	}
}
