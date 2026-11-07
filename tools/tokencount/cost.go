// cost.go defines Cost, the estimated price of a tally of messages.
// It also prices one message through the pricer of the engine that read it.

package main

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// Cost is an estimated price in USD.
// It sums the priced messages and counts, per model, the messages that have no price.
type Cost struct {
	USD float64
	// Unpriced counts, per model, the messages with nonzero usage that no price was found for;
	// their tokens are in no USD figure.
	Unpriced map[string]int
}

// messageCost prices one message's usage through the pricer of the engine that read it.
// A message whose usage is all zero costs nothing whatever its model, so a placeholder message is never unpriced.
// A message with usage is unpriced, never free, when the pricer is nil or has no price for the model.
func messageCost(pricer shuttleengine.MessagePricer, model string, u shuttleengine.MessageUsage) Cost {
	if u.Input+u.CacheWrite+u.CacheRead+u.Output == 0 {
		return Cost{}
	}
	if pricer == nil {
		return Cost{Unpriced: map[string]int{model: 1}}
	}
	usd, priced := pricer.PriceMessage(model, u)
	if !priced {
		return Cost{Unpriced: map[string]int{model: 1}}
	}
	return Cost{USD: usd}
}

func (c *Cost) add(o Cost) {
	c.USD += o.USD
	for model, n := range o.Unpriced {
		if c.Unpriced == nil {
			c.Unpriced = map[string]int{}
		}
		c.Unpriced[model] += n
	}
}

// share returns the cost scaled to the fraction f of its USD.
// It keeps every unpriced count, since a share of an unpriced message is still unpriced.
func (c Cost) share(f float64) Cost {
	return Cost{USD: c.USD * f, Unpriced: c.Unpriced}
}

// String renders the cost for a table cell as "$1.23".
// When only unpriced messages hold tokens it reads "unpriced".
// When both priced and unpriced messages hold tokens it reads "$1.23 + unpriced".
func (c Cost) String() string {
	switch {
	case len(c.Unpriced) == 0:
		return fmt.Sprintf("$%.2f", c.USD)
	case c.USD == 0:
		return "unpriced"
	default:
		return fmt.Sprintf("$%.2f + unpriced", c.USD)
	}
}

// precise renders the cost to a tenth of a cent.
// It suits a figure, such as a cost per line, that rounds to nothing at cents.
// When only unpriced messages hold tokens it reads "unpriced".
// When both do it appends " + unpriced" to the figure.
func (c Cost) precise() string {
	switch {
	case len(c.Unpriced) == 0:
		return fmt.Sprintf("$%.3f", c.USD)
	case c.USD == 0:
		return "unpriced"
	default:
		return fmt.Sprintf("$%.3f + unpriced", c.USD)
	}
}
