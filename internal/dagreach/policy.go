package dagreach

// Turning an analysis into a CI decision.
//
// Four flags, not a language. Each one states its verdict, what matched, and
// the path that proves it - a gate that says "no" without saying why is a gate
// teams disable.
//
// A policy that fails sets the exit code to 1. Everything a policy reports is in
// the JSON output too, so a pipeline can act on the detail rather than on the
// exit code alone.

import (
	"fmt"
	"strconv"
	"strings"
)

// PolicyResult is the verdict of one policy, and the evidence behind it.
type PolicyResult struct {
	Policy    string
	Subject   string
	Failed    bool
	Detail    string
	Matched   []string
	Witnesses map[string][]string
}

// AnyFailed is the gate.
func AnyFailed(results []*PolicyResult) bool {
	for _, result := range results {
		if result.Failed {
			return true
		}
	}
	return false
}

// FailIfReaches fails when the change reaches anything the selector matches.
func FailIfReaches(g *Graph, report *ImpactReport, selector Selector, witnessLimit int) *PolicyResult {
	impacted := map[string]bool{}
	for _, node := range report.Impacted() {
		impacted[node] = true
	}
	matched := []string{}
	for _, node := range selector.Select(g) {
		if impacted[node] {
			matched = append(matched, node)
		}
	}

	if len(matched) == 0 {
		return &PolicyResult{
			Policy:  "fail-if-reaches",
			Subject: selector.String(),
			Detail:  fmt.Sprintf("nothing matching %s is reached", selector),
		}
	}

	seeds := map[string]bool{}
	for _, seed := range report.Seeds {
		seeds[seed] = true
	}
	onlySeeds := true
	for _, node := range matched {
		if !seeds[node] {
			onlySeeds = false
			break
		}
	}
	detail := fmt.Sprintf("%d node(s) matching %s are reached", len(matched), selector)
	if onlySeeds {
		detail += " (they are the changed nodes themselves)"
	}

	shown := matched
	if witnessLimit > 0 && len(shown) > witnessLimit {
		shown = shown[:witnessLimit]
	}
	witnesses := map[string][]string{}
	for _, node := range shown {
		if path := report.Witnesses.Path(node); len(path) > 0 {
			witnesses[node] = path
		}
	}

	return &PolicyResult{
		Policy:    "fail-if-reaches",
		Subject:   selector.String(),
		Failed:    true,
		Detail:    detail,
		Matched:   matched,
		Witnesses: witnesses,
	}
}

// MaxImpacted fails when a change touches more of the graph than the team accepts.
func MaxImpacted(report *ImpactReport, ceiling int) *PolicyResult {
	size := len(report.Impacted())
	failed := size > ceiling
	detail := fmt.Sprintf("%d node(s) impacted, within the ceiling of %d", size, ceiling)
	if failed {
		detail = fmt.Sprintf("%d node(s) impacted, ceiling is %d", size, ceiling)
	}
	return &PolicyResult{
		Policy:  "max-impacted",
		Subject: strconv.Itoa(ceiling),
		Failed:  failed,
		Detail:  detail,
	}
}

// FailOnCycle fails when the graph is not the acyclic thing it claims to be.
func FailOnCycle(cycles [][]string, limit int) *PolicyResult {
	if len(cycles) == 0 {
		return &PolicyResult{
			Policy: "fail-on-cycle", Subject: "cycle", Detail: "the graph is acyclic",
		}
	}
	shown := cycles
	if limit > 0 && len(shown) > limit {
		shown = shown[:limit]
	}
	matched := make([]string, 0, len(shown))
	for _, cycle := range shown {
		matched = append(matched, strings.Join(cycle, ", "))
	}
	return &PolicyResult{
		Policy:  "fail-on-cycle",
		Subject: "cycle",
		Failed:  true,
		Detail:  fmt.Sprintf("%d cycle(s) found", len(cycles)),
		Matched: matched,
	}
}

// FailOnNewReach fails when the change reaches a matching target it did not reach
// before. "Did not reach before" is judged on the pair (matches the selector, is
// reached), so a target that was always reachable and has just been reclassified
// counts too. See NewlyExposed.
func FailOnNewReach(exposures []*Exposure, selector Selector) *PolicyResult {
	if len(exposures) == 0 {
		return &PolicyResult{
			Policy:  "fail-on-new-reach",
			Subject: selector.String(),
			Detail:  fmt.Sprintf("nothing matching %s became reachable", selector),
		}
	}
	matched := make([]string, 0, len(exposures))
	witnesses := map[string][]string{}
	for _, exposure := range exposures {
		matched = append(matched, exposure.Target)
		if len(exposure.Path) > 0 {
			witnesses[exposure.Target] = exposure.Path
		}
	}
	return &PolicyResult{
		Policy:    "fail-on-new-reach",
		Subject:   selector.String(),
		Failed:    true,
		Detail:    fmt.Sprintf("%d target(s) matching %s became reachable", len(exposures), selector),
		Matched:   matched,
		Witnesses: witnesses,
	}
}
