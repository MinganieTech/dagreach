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

// The three verdicts a policy can reach. A policy the data cannot settle is
// neither satisfied nor violated, and saying "pass" in that case is the one
// answer a gate must never give.
const (
	VerdictPass    = "pass"
	VerdictFail    = "fail"
	VerdictUnknown = "unknown"
)

// PolicyResult is the verdict of one policy, and the evidence behind it.
type PolicyResult struct {
	Policy    string
	Subject   string
	Verdict   string
	Detail    string
	Matched   []string
	Witnesses map[string][]string
}

// Failed reports a proven violation, and only that.
func (r *PolicyResult) Failed() bool { return r.Verdict == VerdictFail }

// Unknown reports that the analysis could not settle the policy.
func (r *PolicyResult) Unknown() bool { return r.Verdict == VerdictUnknown }

// Outcome is the gate: a proven violation outranks an unsettled one, because it
// is the stronger statement. Everything else passes.
func Outcome(results []*PolicyResult) string {
	outcome := VerdictPass
	for _, result := range results {
		if result.Failed() {
			return VerdictFail
		}
		if result.Unknown() {
			outcome = VerdictUnknown
		}
	}
	return outcome
}

// AnyFailed reports whether a proven violation is among the results.
func AnyFailed(results []*PolicyResult) bool { return Outcome(results) == VerdictFail }

// FailIfReaches fails when the change reaches anything the selector matches.
func FailIfReaches(g *Graph, report *ImpactReport, selector Selector, witnessLimit int) *PolicyResult {
	if !selector.Declared(g) {
		return undeterminable("fail-if-reaches", selector, g)
	}
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
			Verdict: VerdictPass,
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
		Verdict:   VerdictFail,
		Detail:    detail,
		Matched:   matched,
		Witnesses: witnesses,
	}
}

// undeterminable is the verdict for a selector this graph cannot answer: the
// attribute it reads is declared by no node, so "nothing matched" would be a
// statement about the file rather than about the change.
func undeterminable(policy string, selector Selector, g *Graph) *PolicyResult {
	return &PolicyResult{
		Policy:  policy,
		Subject: selector.String(),
		Verdict: VerdictUnknown,
		Detail: fmt.Sprintf(
			"no node in this graph declares '%s', so nothing can match %s and the policy "+
				"cannot be settled by this file", selector.Key, selector),
	}
}

// MaxImpacted fails when a change touches more of the graph than the team accepts.
func MaxImpacted(report *ImpactReport, ceiling int) *PolicyResult {
	size := len(report.Impacted())
	verdict := VerdictPass
	failed := size > ceiling
	if failed {
		verdict = VerdictFail
	}
	detail := fmt.Sprintf("%d node(s) impacted, within the ceiling of %d", size, ceiling)
	if failed {
		detail = fmt.Sprintf("%d node(s) impacted, ceiling is %d", size, ceiling)
	}
	return &PolicyResult{
		Policy:  "max-impacted",
		Subject: strconv.Itoa(ceiling),
		Verdict: verdict,
		Detail:  detail,
	}
}

// FailOnCycle fails when the graph is not the acyclic thing it claims to be.
func FailOnCycle(cycles [][]string, limit int) *PolicyResult {
	if len(cycles) == 0 {
		return &PolicyResult{
			Policy: "fail-on-cycle", Subject: "cycle",
			Verdict: VerdictPass, Detail: "the graph is acyclic",
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
		Verdict: VerdictFail,
		Detail:  fmt.Sprintf("%d cycle(s) found", len(cycles)),
		Matched: matched,
	}
}

// FailOnNewReach fails when the change reaches a matching target it did not reach
// before. "Did not reach before" is judged on the pair (matches the selector, is
// reached), so a target that was always reachable and has just been reclassified
// counts too. See NewlyExposed.
func FailOnNewReach(exposures []*Exposure, selector Selector, before, after *Graph) *PolicyResult {
	if !selector.Declared(before) && !selector.Declared(after) {
		return undeterminable("fail-on-new-reach", selector, after)
	}
	if len(exposures) == 0 {
		return &PolicyResult{
			Policy:  "fail-on-new-reach",
			Subject: selector.String(),
			Verdict: VerdictPass,
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
		Verdict:   VerdictFail,
		Detail:    fmt.Sprintf("%d target(s) matching %s became reachable", len(exposures), selector),
		Matched:   matched,
		Witnesses: witnesses,
	}
}
