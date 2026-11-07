// pricing.go implements shuttleengine.MessagePricer for Claude.
// It holds the per-model API list-price table and prices one message's usage.

package claudeengine

import "github.com/Knatte18/loomyard/internal/shuttleengine"

var _ shuttleengine.MessagePricer = (*Claude)(nil)

// modelPrice is one model's list price in USD per million tokens.
type modelPrice struct {
	Input float64
	// CacheWrite5m prices a cache write with the default five-minute lifetime.
	CacheWrite5m float64
	// CacheWrite1h prices a cache write with the one-hour lifetime.
	CacheWrite1h float64
	CacheRead    float64
	Output       float64
	// LongPrompt, when set, is the dearer rate card for a message whose prompt exceeds LongPromptAbove tokens.
	LongPrompt      *modelPrice
	LongPromptAbove int
}

// prices is the Claude API first-party list price of every model id the transcripts carry, in USD per million tokens.
// Source: the claude-api skill (Claude Code 2.1.294, models cached 2026-10-06).
// Input and output come from its "Current Models" table.
// Cache reads come from shared/models.md where it names a price, and are 0.1x input elsewhere.
// Cache writes come from shared/prompt-caching.md's "Economics":
// 1.25x input for the five-minute lifetime and 2x for the one-hour one.
// Haiku 5.5 is the one model with a long-prompt rate card.
// Its source is shared/model-migration.md, "Migrating to Claude Haiku 5.5".
// The long-prompt card applies to prompts over 100K tokens and keeps the same cache multipliers.
// The skill measures the prompt with count_tokens, which counts the whole prompt whether cached or not,
// so a message's prompt is its input + cache write + cache read.
var prices = map[string]modelPrice{
	"claude-fable-5-1":  {Input: 10, CacheWrite5m: 12.5, CacheWrite1h: 20, CacheRead: 0.25, Output: 50},
	"claude-fable-5":    {Input: 10, CacheWrite5m: 12.5, CacheWrite1h: 20, CacheRead: 1, Output: 50},
	"claude-opus-5-5":   {Input: 4, CacheWrite5m: 5, CacheWrite1h: 8, CacheRead: 0.20, Output: 20},
	"claude-opus-5":     {Input: 5, CacheWrite5m: 6.25, CacheWrite1h: 10, CacheRead: 0.50, Output: 25},
	"claude-opus-4-8":   {Input: 5, CacheWrite5m: 6.25, CacheWrite1h: 10, CacheRead: 0.50, Output: 25},
	"claude-sonnet-5-5": {Input: 2, CacheWrite5m: 2.5, CacheWrite1h: 4, CacheRead: 0.20, Output: 10},
	"claude-sonnet-5":   {Input: 2, CacheWrite5m: 2.5, CacheWrite1h: 4, CacheRead: 0.20, Output: 10},
	"claude-haiku-5-5": {Input: 0.10, CacheWrite5m: 0.125, CacheWrite1h: 0.20, CacheRead: 0.01, Output: 0.50,
		LongPromptAbove: 100_000, LongPrompt: &modelPrice{Input: 0.50, CacheWrite5m: 0.625, CacheWrite1h: 1, CacheRead: 0.05, Output: 2.50}},
	"claude-haiku-4-5-20251001": {Input: 1, CacheWrite5m: 1.25, CacheWrite1h: 2, CacheRead: 0.10, Output: 5},
}

// PriceMessage prices one message's usage at its model's list price in USD, on the rate card its prompt size selects.
// A message whose usage is all zero costs nothing whatever its model, so a placeholder message is never unpriced.
// The second result reports whether the message was priced; it is false only for a model the price table lacks.
func (c *Claude) PriceMessage(model string, u shuttleengine.MessageUsage) (float64, bool) {
	if u == (shuttleengine.MessageUsage{}) {
		return 0, true
	}
	p, ok := prices[model]
	if !ok {
		return 0, false
	}
	if p.LongPrompt != nil && u.Input+u.CacheWrite+u.CacheRead > p.LongPromptAbove {
		p = *p.LongPrompt
	}
	oneHour := u.CacheWriteOneHour
	fiveMinute := u.CacheWrite - oneHour
	return (float64(u.Input)*p.Input +
		float64(fiveMinute)*p.CacheWrite5m +
		float64(oneHour)*p.CacheWrite1h +
		float64(u.CacheRead)*p.CacheRead +
		float64(u.Output)*p.Output) / 1e6, true
}
