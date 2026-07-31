package dagreach

// The dagreach attribute profile.
//
// dagreach does not define a graph format. It reads DOT and JSON Graph Format,
// and gives a meaning to a handful of attribute names when they happen to be
// there. Everything is optional: a graph with no profile attributes still gets
// every structural answer, only the weighted ones change.
//
// See docs/attribute-profile.md for the normative description.

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// DurationKeys are read in order of precedence.
var DurationKeys = []string{"duration", "weight"}

const (
	// StatusKey holds a free-text lifecycle state.
	StatusKey = "status"
	// GroupKey holds a grouping used for rollups.
	GroupKey = "group"
)

// durationOf returns the declared duration, or false when absent or unreadable.
func durationOf(attrs map[string]string) (float64, bool) {
	for _, key := range DurationKeys {
		raw, ok := attrs[key]
		if !ok {
			continue
		}
		value, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
		if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
			continue
		}
		return value, true
	}
	return 0, false
}

func textAttr(attrs map[string]string, key string) string {
	return strings.TrimSpace(attrs[key])
}

// ProfileSummary is what the profile found in a graph: the basis of `dagreach parse`.
type ProfileSummary struct {
	NodesWithDuration int
	EdgesWithDuration int
	Statuses          *Counter
	Groups            *Counter
	Unreadable        []string
}

func (s *ProfileSummary) UsesDurations() bool {
	return s.NodesWithDuration > 0 || s.EdgesWithDuration > 0
}

// Summarize reports the profile attributes present, and the values that could not be read.
func Summarize(g *Graph) *ProfileSummary {
	summary := &ProfileSummary{
		Statuses:   NewCounter(),
		Groups:     NewCounter(),
		Unreadable: []string{},
	}
	for _, id := range g.Nodes() {
		node := g.Node(id)
		if _, ok := durationOf(node.Attrs); ok {
			summary.NodesWithDuration++
		} else {
			recordUnreadable(node.Attrs, fmt.Sprintf("node '%s'", id), summary)
		}
		if status := textAttr(node.Attrs, StatusKey); status != "" {
			summary.Statuses.Add(status)
		}
		if group := textAttr(node.Attrs, GroupKey); group != "" {
			summary.Groups.Add(group)
		}
	}
	for _, edge := range g.Edges {
		if _, ok := durationOf(edge.Attrs); ok {
			summary.EdgesWithDuration++
		} else {
			recordUnreadable(edge.Attrs,
				fmt.Sprintf("edge '%s' -> '%s'", edge.Source, edge.Target), summary)
		}
	}
	return summary
}

func recordUnreadable(attrs map[string]string, label string, summary *ProfileSummary) {
	for _, key := range DurationKeys {
		raw, present := attrs[key]
		if !present {
			continue
		}
		if _, ok := durationOf(map[string]string{key: raw}); !ok {
			summary.Unreadable = append(summary.Unreadable,
				fmt.Sprintf("%s: %s='%s' is not a number, so it was ignored", label, key, raw))
			return
		}
	}
}
