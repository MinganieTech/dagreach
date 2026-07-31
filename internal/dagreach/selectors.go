package dagreach

// Selecting nodes for a policy.
//
// A deliberately small grammar - key=value, three keys - rather than an
// expression language. A policy engine is something a project grows into once
// the primitives have proven themselves, not something it starts with.

import (
	"fmt"
	"strings"
)

// SelectorKeys are the three things a policy can select on.
var SelectorKeys = []string{"group", "status", "node"}

// Selector matches nodes by one attribute.
type Selector struct {
	Key   string
	Value string
}

func (s Selector) String() string { return s.Key + "=" + s.Value }

// Matches reports whether a node satisfies the selector.
func (s Selector) Matches(g *Graph, id string) bool {
	if s.Key == "node" {
		return id == s.Value
	}
	node := g.Node(id)
	if node == nil {
		return false
	}
	if s.Key == "group" {
		return textAttr(node.Attrs, GroupKey) == s.Value
	}
	return textAttr(node.Attrs, StatusKey) == s.Value
}

// Select returns every matching node, in declaration order.
func (s Selector) Select(g *Graph) []string {
	matched := []string{}
	for _, id := range g.Nodes() {
		if s.Matches(g, id) {
			matched = append(matched, id)
		}
	}
	return matched
}

// ParseSelector reads KEY=VALUE, or explains what was expected.
func ParseSelector(text string) (Selector, error) {
	key, value, found := strings.Cut(text, "=")
	key, value = strings.TrimSpace(key), strings.TrimSpace(value)
	if !found || key == "" || value == "" {
		return Selector{}, &UsageError{Message: fmt.Sprintf(
			"cannot read the selector '%s'; expected KEY=VALUE with KEY one of %s",
			text, strings.Join(SelectorKeys, ", "))}
	}
	for _, known := range SelectorKeys {
		if key == known {
			return Selector{Key: key, Value: value}, nil
		}
	}
	return Selector{}, &UsageError{Message: fmt.Sprintf(
		"unknown selector key '%s'; expected one of %s", key, strings.Join(SelectorKeys, ", "))}
}

// UsageError is a command-line mistake: it must never look like a policy failure.
type UsageError struct{ Message string }

func (e *UsageError) Error() string { return e.Message }

// InputError is an unreadable input: also never a policy failure.
type InputError struct{ Message string }

func (e *InputError) Error() string { return e.Message }
