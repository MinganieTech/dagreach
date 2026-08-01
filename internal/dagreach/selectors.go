package dagreach

// Selecting nodes for a policy.
//
// Three shorthands - group=, status=, node= - plus `attr:NAME=VALUE` for any
// attribute a profile or a file happens to carry. Still not an expression
// language: one key, one value, exact match, no globs and no regular
// expressions.
//
// The reason `attr:` is explicit rather than implicit is the failure it
// prevents. If any unknown key were silently read as an attribute name, then
// `--fail-if-reaches grup=production` would select nodes carrying an attribute
// called "grup", match nothing, and pass - a typo turning a gate into a rubber
// stamp. With the prefix, an unknown bare key is a usage error, and an `attr:`
// naming something no node declares is undeterminable rather than satisfied.

import (
	"fmt"
	"strings"
)

// SelectorKeys are the shorthands. Anything else needs the `attr:` prefix.
var SelectorKeys = []string{"group", "status", "node"}

// AttributePrefix opens the door to every other attribute.
const AttributePrefix = "attr:"

// Selector matches nodes by one attribute, or by identifier.
type Selector struct {
	Key   string // "node", or an attribute name such as "group", "status", "risk"
	Value string
	// Explicit records that the caller wrote `attr:`, so the selector renders
	// the way it was typed.
	Explicit bool
}

func (s Selector) String() string {
	if s.Explicit {
		return AttributePrefix + s.Key + "=" + s.Value
	}
	return s.Key + "=" + s.Value
}

// ByIdentifier reports whether the selector names a node rather than an attribute.
func (s Selector) ByIdentifier() bool { return s.Key == "node" && !s.Explicit }

// Matches reports whether a node satisfies the selector.
func (s Selector) Matches(g *Graph, id string) bool {
	if s.ByIdentifier() {
		return id == s.Value
	}
	node := g.Node(id)
	if node == nil {
		return false
	}
	return textAttr(node.Attrs, s.Key) == s.Value
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

// Declared reports whether any node in the graph carries the attribute this
// selector reads. A selector over an attribute nobody declares cannot be
// answered by this graph - it is not simply unsatisfied.
func (s Selector) Declared(g *Graph) bool {
	if s.ByIdentifier() {
		return true
	}
	for _, id := range g.Nodes() {
		if textAttr(g.Node(id).Attrs, s.Key) != "" {
			return true
		}
	}
	return false
}

// ParseSelector reads KEY=VALUE or attr:NAME=VALUE, or explains what was expected.
func ParseSelector(text string) (Selector, error) {
	body, explicit := strings.CutPrefix(text, AttributePrefix)
	key, value, found := strings.Cut(body, "=")
	key, value = strings.TrimSpace(key), strings.TrimSpace(value)

	if !found || key == "" || value == "" {
		return Selector{}, &UsageError{Message: fmt.Sprintf(
			"cannot read the selector '%s'; expected KEY=VALUE with KEY one of %s, "+
				"or %sNAME=VALUE for any other attribute",
			text, strings.Join(SelectorKeys, ", "), AttributePrefix)}
	}
	if explicit {
		if key == "node" {
			return Selector{}, &UsageError{Message: fmt.Sprintf(
				"'%snode' is not an attribute; write node=%s to select by identifier",
				AttributePrefix, value)}
		}
		return Selector{Key: key, Value: value, Explicit: true}, nil
	}
	for _, known := range SelectorKeys {
		if key == known {
			return Selector{Key: key, Value: value}, nil
		}
	}
	return Selector{}, &UsageError{Message: fmt.Sprintf(
		"unknown selector key '%s'; expected one of %s, or %s%s=%s to read it as an attribute",
		key, strings.Join(SelectorKeys, ", "), AttributePrefix, key, value)}
}

// UsageError is a command-line mistake: it must never look like a policy failure.
type UsageError struct{ Message string }

func (e *UsageError) Error() string { return e.Message }

// InputError is an unreadable input: also never a policy failure.
type InputError struct{ Message string }

func (e *InputError) Error() string { return e.Message }
