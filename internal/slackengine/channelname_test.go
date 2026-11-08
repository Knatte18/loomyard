// channelname_test.go covers the names of an orch's channel and of a retired channel.

package slackengine_test

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/slackengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// cutName returns the name a name over 80 characters is cut to: 73 characters, a dash and six hex digits of its SHA-256.
func cutName(name string) string {
	sum := sha256.Sum256([]byte(name))
	return name[:73] + "-" + hex.EncodeToString(sum[:])[:6]
}

func TestChannelName(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	day := time.Date(2026, 11, 8, 23, 0, 0, 0, time.UTC)
	// Each shortname below joins "-neve-prime", which is 11 characters, to a whole name of 80 and of 81 characters.
	shortname80 := strings.Repeat("a", 69)
	shortname81 := strings.Repeat("a", 70)
	base79 := strings.Repeat("b", 79)

	tests := []struct {
		name string
		got  string
		want string
	}{
		{"PrimeForAnEmptySlug", slackengine.ChannelName("ly", "neve", ""), "ly-neve-prime"},
		{"BeamSlug", slackengine.ChannelName("ly", "neve", "payments"), "ly-neve-payments"},
		{"BeamSlugPrimeIsDistinct", slackengine.ChannelName("ly", "neve", "prime"), "ly-neve-prime_beam"},
		{"MachineOutsideTheGrammarBecomesDashes", slackengine.ChannelName("ly", "neve+box 1", "payments"), "ly-neve-box-1-payments"},
		{"MachineIsLowercased", slackengine.ChannelName("ly", "NEVE", ""), "ly-neve-prime"},
		{"EightyCharactersPassUnchanged", slackengine.ChannelName(shortname80, "neve", ""), shortname80 + "-neve-prime"},
		{
			"EightyOneCharactersEndInTheHashOfTheWholeName",
			slackengine.ChannelName(shortname81, "neve", ""),
			cutName(shortname81 + "-neve-prime"),
		},
		{"ArchivedFirstTryCarriesTheDate", slackengine.ArchivedName("ly-neve-payments", day, 1), "ly-neve-payments-2026-11-08"},
		{"ArchivedSecondTryAddsTwo", slackengine.ArchivedName("ly-neve-payments", day, 2), "ly-neve-payments-2026-11-08-2"},
		{"ArchivedThirdTryAddsThree", slackengine.ArchivedName("ly-neve-payments", day, 3), "ly-neve-payments-2026-11-08-3"},
		{"ArchivedLongBaseIsCutToEighty", slackengine.ArchivedName(base79, day, 1), cutName(base79 + "-2026-11-08")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if tt.got != tt.want {
				t.Errorf("name = %q; want %q", tt.got, tt.want)
			}
			if len(tt.got) > 80 {
				t.Errorf("name %q is %d characters; want at most 80", tt.got, len(tt.got))
			}
		})
	}
}
