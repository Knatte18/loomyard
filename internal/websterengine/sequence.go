// sequence.go asserts that a batch list runs in dependency order, derived from card-level Targets/Uses ref matching.
// internal/batcher owns grouping and order (batches come back in plan card order);
// this file only checks that no batch depends on one that runs after it, so the order is asserted, never derived.

package websterengine

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/planparser"
)

// ErrBatchOrder is the sentinel CheckBatchOrder's error wraps when a batch depends on a batch that runs after it.
var ErrBatchOrder = errors.New("webster: batch order: dependency on a later batch")

// CheckBatchOrder returns an error wrapping ErrBatchOrder that names every dependency edge running from a batch to an earlier one, and nil when every batch follows the batches it depends on.
// Each finding names both batch numbers and the card and ref that make the dependency.
// A cycle between batches always holds such an edge, so it is refused by the same rule.
// The way forward names step: an empty step keeps the manual text naming `lym webster rebaseline --card NN` and `lym webster run --fresh`, and a set step says to fix the plan's card order and take step.
func CheckBatchOrder(batches []batcher.Batch, step string) error {
	edges := deriveEdges(batches)
	var findings []string
	for from, successors := range edges {
		for _, to := range successors {
			if to > from {
				continue
			}
			findings = append(findings, backwardEdgeFinding(batches[from], batches[to]))
		}
	}
	if len(findings) == 0 {
		return nil
	}
	sort.Strings(findings)
	wayForward := "then `lym webster rebaseline --card NN` naming each card you edited, or `lym webster run --fresh` for a run that has not begun a batch"
	if step != "" {
		wayForward = "then " + step
	}
	return fmt.Errorf("%w: %s; way forward: fix the plan's card order so every card follows the cards whose targets it uses, %s",
		ErrBatchOrder, strings.Join(findings, "; "), wayForward)
}

// backwardEdgeFinding describes why the batch that runs first must run after the batch that runs later, naming both batch numbers and the first card pair and ref that make the dependency.
// producer is the later batch in the list, consumer the earlier one.
func backwardEdgeFinding(producer, consumer batcher.Batch) string {
	producerNumber, _ := batchIdentity(producer)
	consumerNumber, _ := batchIdentity(consumer)
	for _, a := range producer.Cards {
		for _, b := range consumer.Cards {
			if ref := firstSharedRef(a, b, true); ref != "" {
				return fmt.Sprintf("batch %d (card %d) uses %q, which batch %d (card %d) targets, but batch %d runs first",
					consumerNumber, b.Number, ref, producerNumber, a.Number, consumerNumber)
			}
			if a.Number < b.Number {
				if ref := firstSharedRef(a, b, false); ref != "" {
					return fmt.Sprintf("batch %d (card %d) and batch %d (card %d) both target %q and card %d comes first, but batch %d runs first",
						producerNumber, a.Number, consumerNumber, b.Number, ref, a.Number, consumerNumber)
				}
			}
		}
	}
	return fmt.Sprintf("batch %d depends on batch %d, which runs later", consumerNumber, producerNumber)
}

// firstSharedRef returns the first ref the two cards share that makes a dependency edge under planparser.DependencyEdge, or "" when there is none.
// With consumerUses it walks the consumer's Uses entries against the producer's Targets; otherwise it walks the producer's Targets against the consumer's.
func firstSharedRef(producer, consumer planparser.Card, consumerUses bool) string {
	left, right := producer.Targets, consumer.Targets
	if consumerUses {
		left, right = consumer.Uses, producer.Targets
	}
	for _, ref := range left {
		ref = strings.TrimSpace(ref)
		if ref == "" || !refsIntersect([]string{ref}, right) {
			continue
		}
		consumerKind := planparser.CardRefKind(consumer, ref)
		if consumerUses {
			consumerKind = planparser.RefUse
		}
		if planparser.DependencyEdge(producer, planparser.CardRefKind(producer, ref), consumer, consumerKind) {
			return ref
		}
	}
	return ""
}

// deriveEdges builds the vertex adjacency list, one entry per input batch index, by comparing every ordered pair of cards drawn from different batches.
// Each vertex's successor list is deduplicated and sorted ascending, which keeps the result deterministic.
//
// An edge i -> j is added when:
//   - batch j's card b has a Uses entry matching batch i's card a's Targets entry (producer before consumer); or
//   - batch i's card a and batch j's card b share a Targets entry and a.Number < b.Number (declared card order settles two writers of the same ref);
//
// and planparser.DependencyEdge holds for that shared ref.
// Two Extends on one ref therefore make no edge unless both cards fence a line for one PATTERN.md entry ID.
//
// A shared entry between two cards' Uses never produces an edge, since a read creates no ordering against another read.
// Refs that are empty or whitespace-only after strings.TrimSpace are ignored on both sides of every comparison.
// Ref classification (symbol-shaped vs. path-shaped) is not consulted: both kinds participate identically.
func deriveEdges(batches []batcher.Batch) [][]int {
	n := len(batches)
	adj := make([][]int, n)
	seen := make([]map[int]bool, n)
	for i := range seen {
		seen[i] = map[int]bool{}
	}

	addEdge := func(from, to int) {
		if from == to {
			return
		}
		if seen[from][to] {
			return
		}
		seen[from][to] = true
		adj[from] = append(adj[from], to)
	}

	for i := range batches {
		for _, a := range batches[i].Cards {
			for j := range batches {
				if i == j {
					continue
				}
				for _, b := range batches[j].Cards {
					if firstSharedRef(a, b, true) != "" {
						addEdge(i, j)
					}
					if a.Number < b.Number && firstSharedRef(a, b, false) != "" {
						addEdge(i, j)
					}
				}
			}
		}
	}

	for i := range adj {
		sort.Ints(adj[i])
	}
	return adj
}

// refsIntersect reports whether left and right share at least one entry, compared by exact string
// equality after trimming whitespace; an entry that trims to empty is ignored on both sides.
func refsIntersect(left, right []string) bool {
	set := make(map[string]bool, len(left))
	for _, ref := range left {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			continue
		}
		set[ref] = true
	}
	if len(set) == 0 {
		return false
	}
	for _, ref := range right {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			continue
		}
		if set[ref] {
			return true
		}
	}
	return false
}
