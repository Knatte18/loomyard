package claudeengine

import (
	"math"
	"testing"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

func TestPriceMessage(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		model      string
		usage      shuttleengine.MessageUsage
		wantUSD    float64
		wantPriced bool
	}{
		{
			"PricedByCacheLifetime", "claude-opus-5-5",
			shuttleengine.MessageUsage{Input: 1e6, CacheWrite: 3e6, CacheWriteOneHour: 1e6, CacheRead: 1e6, Output: 1e6},
			4 + 2*5 + 8 + 0.2 + 20, true,
		},
		{
			"PromptAtLongPromptThreshold_LowerCard", "claude-haiku-5-5",
			shuttleengine.MessageUsage{Input: 10_000, CacheRead: 90_000, Output: 1e6},
			(10_000*0.10 + 90_000*0.01 + 1e6*0.50) / 1e6, true,
		},
		{
			"PromptOverLongPromptThreshold_HigherCard", "claude-haiku-5-5",
			shuttleengine.MessageUsage{Input: 10_000, CacheWrite: 1, CacheRead: 90_000, Output: 1e6},
			(10_000*0.50 + 1*0.625 + 90_000*0.05 + 1e6*2.50) / 1e6, true,
		},
		{"UnpricedModel", "claude-future-9", shuttleengine.MessageUsage{Output: 1}, 0, false},
		{"PlaceholderWithoutUsage", "<synthetic>", shuttleengine.MessageUsage{}, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gotUSD, gotPriced := (&Claude{}).PriceMessage(tt.model, tt.usage)
			if gotPriced != tt.wantPriced || math.Abs(gotUSD-tt.wantUSD) > 1e-9 {
				t.Errorf("PriceMessage(%s) = %v, %v; want %v, %v", tt.model, gotUSD, gotPriced, tt.wantUSD, tt.wantPriced)
			}
		})
	}
}
