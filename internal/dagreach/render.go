package dagreach

// Turning analysis results into the two output shapes: text and JSON.
//
// Text is for a human reading a terminal or a CI log; JSON is the contract other
// tools consume, and it carries a schema_version so they can rely on it. They
// report the same facts. Long lists are truncated in text mode only, and a
// truncation always says how many items it hid - a silent cap would read as
// "that is all there is".
//
// Printed text stays ASCII: a Windows console on a legacy code page renders
// anything else as '?'.

import (
	"fmt"
	"sort"
	"strings"
)

// SchemaVersion is bumped when the JSON shape changes in a way a consumer could notice.
const SchemaVersion = 1

var orientationText = map[string]string{
	"feeds":      "source feeds target, so impact follows edges forward",
	"depends-on": "source depends on target, so impact follows edges backward",
}

// Listing renders a list, hiding the tail beyond limit but never in silence.
func Listing(items []string, limit int) string {
	if len(items) == 0 {
		return "none"
	}
	if limit <= 0 || len(items) <= limit {
		return strings.Join(items, ", ")
	}
	return strings.Join(items[:limit], ", ") + fmt.Sprintf(" (+%d more)", len(items)-limit)
}

func pathLine(nodes []string, limit int) string {
	if limit > 0 && len(nodes) > limit {
		return strings.Join(nodes[:limit], " -> ") + fmt.Sprintf(" (+%d more)", len(nodes)-limit)
	}
	return strings.Join(nodes, " -> ")
}

func edgeSemanticsLine(g *Graph) string {
	profile := ""
	if g.Profile != "" && g.Profile != "generic" {
		profile = g.Profile + " profile, "
	}
	return "edges: " + profile + orientationText[g.EdgeSemantics]
}

func countsLine(counter *Counter) string {
	parts := make([]string, 0, counter.Len())
	for _, key := range counter.Keys() {
		parts = append(parts, fmt.Sprintf("%s %d", key, counter.Get(key)))
	}
	return strings.Join(parts, ", ")
}

// ProfilesText is the `dagreach profiles` listing.
func ProfilesText() []string {
	lines := []string{"profiles:"}
	width := 0
	for _, profile := range Profiles() {
		if len(profile.Name) > width {
			width = len(profile.Name)
		}
	}
	for _, profile := range Profiles() {
		lines = append(lines,
			fmt.Sprintf("  %-*s  reads %s", width, profile.Name, profile.ProducedBy),
			fmt.Sprintf("  %s  edges: %s; %s",
				strings.Repeat(" ", width), profile.EdgeSemantics, profile.Summary))
	}
	return append(lines, "",
		"A profile is detected from the file when --profile is omitted, and the report",
		"always says which one was applied. --edge-semantics overrides it.")
}

// -- parse -----------------------------------------------------------------

// ParseText renders what the reader understood.
func ParseText(g *Graph, summary *ProfileSummary) []string {
	orientation := "directed"
	if !g.Directed {
		orientation = "undirected"
	}
	name := ""
	if g.Name != "" {
		name = fmt.Sprintf(" '%s'", g.Name)
	}
	lines := []string{
		fmt.Sprintf("%s: %s%s, %s, %d nodes, %d edges",
			sourceName(g), g.Format, name, orientation, g.NodeCount(), g.EdgeCount()),
		edgeSemanticsLine(g),
		fmt.Sprintf("profile: durations on %d/%d nodes and %d/%d edges, "+
			"%d status value(s), %d group(s)",
			summary.NodesWithDuration, g.NodeCount(),
			summary.EdgesWithDuration, g.EdgeCount(),
			summary.Statuses.Len(), summary.Groups.Len()),
	}

	oddities := []string{}
	if loops := g.SelfLoops(); loops > 0 {
		oddities = append(oddities, fmt.Sprintf("%d self-loop(s)", loops))
	}
	if duplicates := g.DuplicateEdges(); duplicates > 0 {
		oddities = append(oddities, fmt.Sprintf("%d duplicated edge(s)", duplicates))
	}
	if len(oddities) > 0 {
		lines = append(lines, "structure: "+strings.Join(oddities, ", "))
	}

	return append(lines, warningLines(append(append([]string{}, g.Warnings...), summary.Unreadable...))...)
}

// -- stats -----------------------------------------------------------------

// StatsText renders the shape of a graph.
func StatsText(g *Graph, stats *GraphStats, limit int, policies []*PolicyResult) []string {
	shape := "acyclic"
	if !stats.Acyclic() {
		shape = fmt.Sprintf("%d cycle(s)", len(stats.Cycles))
	}
	lines := []string{
		fmt.Sprintf("%s: %d nodes, %d edges, %s", sourceName(g), stats.Nodes, stats.Edges, shape),
		edgeSemanticsLine(g),
	}

	if !stats.Acyclic() {
		lines = append(lines,
			"cycles are collapsed before measuring depth, width and the longest path; "+
				"reachability is unaffected")
		shown := stats.Cycles
		if limit > 0 && len(shown) > limit {
			shown = shown[:limit]
		}
		for _, cycle := range shown {
			lines = append(lines, "  cycle: "+Listing(cycle, limit))
		}
		if limit > 0 && len(stats.Cycles) > limit {
			lines = append(lines,
				fmt.Sprintf("  (+%d more cycle(s))", len(stats.Cycles)-limit))
		}
	}

	measured := ""
	if stats.Condensed {
		measured = " (measured on the condensed graph)"
	}
	lines = append(lines, fmt.Sprintf(
		"shape%s: depth %d level(s), width %d (largest earliest-start generation), "+
			"%d root(s), %d leaf/leaves",
		measured, stats.Depth, stats.Width, len(stats.Roots), len(stats.Leaves)))

	if stats.LongestPath != nil && len(stats.LongestPath.Nodes) > 0 {
		lines = append(lines,
			fmt.Sprintf("%s: %s", stats.LongestPath.Label(), stats.LongestPath.Describe()),
			"  "+pathLine(stats.LongestPath.Nodes, limit))
	}
	if len(stats.WidestLevel) > 0 {
		lines = append(lines, "widest generation: "+Listing(stats.WidestLevel, limit))
	}

	lines = append(lines, fmt.Sprintf("articulation points (%d, undirected reading): %s",
		len(stats.ArticulationPoints), Listing(stats.ArticulationPoints, limit)))
	if len(stats.Isolated) > 0 {
		lines = append(lines, fmt.Sprintf("isolated (%d): %s",
			len(stats.Isolated), Listing(stats.Isolated, limit)))
	}
	if stats.Groups.Len() > 0 {
		lines = append(lines, "groups: "+countsLine(stats.Groups))
	}

	lines = append(lines, policyLines(policies, limit)...)
	return append(lines, warningLines(g.Warnings)...)
}

// -- impact ----------------------------------------------------------------

// ImpactText renders what a change reaches.
func ImpactText(
	g *Graph, report *ImpactReport, limit int, policies []*PolicyResult, explain bool,
) []string {
	impacted := report.Impacted()
	lines := []string{
		fmt.Sprintf("%s: %s reaches %d of %d nodes (%d%%)",
			sourceName(g), Listing(report.Seeds, limit), len(impacted), report.TotalNodes,
			int(roundTo(report.Share()*100, 0))),
		edgeSemanticsLine(g),
		fmt.Sprintf("downstream (%d): %s", len(report.Downstream), Listing(report.Downstream, limit)),
	}
	if len(report.Upstream) > 0 {
		lines = append(lines, fmt.Sprintf("upstream (%d), what the change depends on: %s",
			len(report.Upstream), Listing(report.Upstream, limit)))
	}
	if len(report.ImpactedLeaves) > 0 {
		lines = append(lines, fmt.Sprintf("impacted leaves (%d): %s",
			len(report.ImpactedLeaves), Listing(report.ImpactedLeaves, limit)))
	}
	if report.Cost != nil {
		lines = append(lines, fmt.Sprintf("cost of the impacted set: %s of declared duration",
			formatNumber(*report.Cost)))
	}
	if report.LongestPath != nil && len(report.LongestPath.Nodes) > 0 {
		lines = append(lines,
			fmt.Sprintf("%s within the impacted set: %s",
				report.LongestPath.Label(), report.LongestPath.Describe()),
			"  "+pathLine(report.LongestPath.Nodes, limit))
	}
	if report.Groups.Len() > 0 {
		lines = append(lines, "groups touched: "+countsLine(report.Groups))
	}
	if len(report.ImpactedArticulationPoints) > 0 {
		lines = append(lines, "note: "+Listing(report.ImpactedArticulationPoints, limit)+
			" is an articulation point: everything behind it depends on it alone")
	}

	if explain {
		lines = append(lines, explainLines(report, limit)...)
	}
	lines = append(lines, policyLines(policies, limit)...)
	return append(lines, warningLines(g.Warnings)...)
}

func explainLines(report *ImpactReport, limit int) []string {
	reached := report.Downstream
	if len(reached) == 0 {
		return []string{"why: nothing downstream to explain"}
	}
	shown := reached
	if limit > 0 && len(shown) > limit {
		shown = shown[:limit]
	}
	header := "why:"
	if limit > 0 {
		header = fmt.Sprintf("why (%d of %d shown):", len(shown), len(reached))
	}
	lines := []string{header}
	for _, node := range shown {
		lines = append(lines, fmt.Sprintf("  %s (distance %d): %s",
			node, report.Witnesses.Distance[node], strings.Join(report.Witnesses.Path(node), " -> ")))
	}
	return lines
}

// -- diff ------------------------------------------------------------------

// DiffText renders the reach delta between two graphs.
func DiffText(
	before, after *Graph, diff *ReachDiff, exposures []*Exposure, policies []*PolicyResult,
	limit int, explain bool, allPairs *AllPairsDelta, countOnly bool,
) []string {
	lines := []string{
		fmt.Sprintf("%s -> %s: %s reaches %d nodes, was %d (+%d, -%d)",
			sourceName(before), sourceName(after), Listing(diff.Seeds, limit),
			len(diff.ReachedAfter), len(diff.ReachedBefore), len(diff.Added), len(diff.Removed)),
		edgeSemanticsLine(after),
	}
	if len(diff.SeedsMissingBefore) > 0 {
		lines = append(lines, fmt.Sprintf("new in this version (%d): %s",
			len(diff.SeedsMissingBefore), Listing(diff.SeedsMissingBefore, limit)))
	}
	if len(diff.SeedsMissingAfter) > 0 {
		lines = append(lines, fmt.Sprintf("gone in this version (%d): %s",
			len(diff.SeedsMissingAfter), Listing(diff.SeedsMissingAfter, limit)))
	}
	lines = append(lines, fmt.Sprintf("new reach (%d): %s",
		len(diff.Added), Listing(diff.Added, limit)))
	if len(diff.Removed) > 0 {
		lines = append(lines, fmt.Sprintf("lost reach (%d): %s",
			len(diff.Removed), Listing(diff.Removed, limit)))
	}
	lines = append(lines, fmt.Sprintf(
		"structure: %d node(s) added, %d removed, %d edge(s) added, %d removed",
		len(diff.NodesAdded), len(diff.NodesRemoved), len(diff.EdgesAdded), len(diff.EdgesRemoved)))

	if explain {
		lines = append(lines, exposureLines(exposures, limit)...)
	}
	if allPairs != nil {
		lines = append(lines, allPairsLines(allPairs, limit, countOnly)...)
	}
	lines = append(lines, policyLines(policies, limit)...)
	return append(lines,
		warningLines(append(append([]string{}, before.Warnings...), after.Warnings...))...)
}

func exposureLines(exposures []*Exposure, limit int) []string {
	if len(exposures) == 0 {
		return []string{"why: no target became newly reachable"}
	}
	shown := exposures
	if limit > 0 && len(shown) > limit {
		shown = shown[:limit]
	}
	lines := []string{fmt.Sprintf("why (%d of %d shown):", len(shown), len(exposures))}
	for _, exposure := range shown {
		lines = append(lines,
			fmt.Sprintf("  %s is now reached", exposure.Target),
			"    reason: "+exposure.Detail)
		if len(exposure.Path) > 0 {
			lines = append(lines, "    path:   "+pathLine(exposure.Path, limit))
		}
	}
	return lines
}

func allPairsLines(delta *AllPairsDelta, limit int, countOnly bool) []string {
	lines := []string{fmt.Sprintf(
		"all-pairs reachability delta: +%d pair(s), -%d pair(s) over %d source(s)",
		delta.AddedTotal, delta.RemovedTotal, delta.Sources)}
	if countOnly {
		return lines
	}
	lines = append(lines, "  by source, largest first: "+bySource(delta.AddedBySource, limit, "+"))
	if len(delta.RemovedBySource) > 0 {
		lines = append(lines, "  lost, largest first:     "+bySource(delta.RemovedBySource, limit, "-"))
	}
	return lines
}

func bySource(counts map[string]int, limit int, sign string) string {
	if len(counts) == 0 {
		return "none"
	}
	type entry struct {
		node  string
		count int
	}
	ranked := make([]entry, 0, len(counts))
	for node, count := range counts {
		ranked = append(ranked, entry{node, count})
	}
	sort.Slice(ranked, func(left, right int) bool {
		if ranked[left].count != ranked[right].count {
			return ranked[left].count > ranked[right].count
		}
		return ranked[left].node < ranked[right].node
	})
	shown := ranked
	if limit > 0 && len(shown) > limit {
		shown = shown[:limit]
	}
	parts := make([]string, 0, len(shown))
	for _, item := range shown {
		parts = append(parts, fmt.Sprintf("%s %s%d", item.node, sign, item.count))
	}
	rendered := strings.Join(parts, ", ")
	if hidden := len(ranked) - len(shown); hidden > 0 {
		rendered += fmt.Sprintf(" (+%d more source(s))", hidden)
	}
	return rendered
}

// -- shared ----------------------------------------------------------------

func policyLines(policies []*PolicyResult, limit int) []string {
	if len(policies) == 0 {
		return nil
	}
	lines := []string{"policies:"}
	for _, result := range policies {
		verdict := "ok  "
		if result.Failed {
			verdict = "FAIL"
		}
		lines = append(lines, fmt.Sprintf("  %s %s %s: %s",
			verdict, result.Policy, result.Subject, result.Detail))
		shown := result.Matched
		if limit > 0 && len(shown) > limit {
			shown = shown[:limit]
		}
		for _, node := range shown {
			if witness, ok := result.Witnesses[node]; ok && len(witness) > 0 {
				lines = append(lines, fmt.Sprintf("    %s: %s", node, pathLine(witness, limit)))
			} else {
				lines = append(lines, "    "+node)
			}
		}
		if limit > 0 && len(result.Matched) > limit {
			lines = append(lines, fmt.Sprintf("    (+%d more)", len(result.Matched)-limit))
		}
	}
	return lines
}

func warningLines(warnings []string) []string {
	if len(warnings) == 0 {
		return nil
	}
	lines := []string{fmt.Sprintf("warnings (%d):", len(warnings))}
	for _, warning := range warnings {
		lines = append(lines, "  - "+warning)
	}
	return lines
}

func sourceName(g *Graph) string {
	if g.Source == "" {
		return "<input>"
	}
	return g.Source
}

// -- markdown --------------------------------------------------------------

// MarkdownReport renders a report for a pull-request comment.
//
// The first line of the text report is the headline, so the two modes can never
// disagree about what happened; the rest travels inside a collapsed block, and
// the verdicts sit at the top where a reviewer reads them. A third verdict slots
// into this table without changing its shape.
func MarkdownReport(body []string, policies []*PolicyResult, limit int) []string {
	if len(body) == 0 {
		return []string{"### dagreach", "", "_nothing to report._"}
	}

	lines := []string{"### dagreach", "", body[0], ""}

	if len(policies) > 0 {
		lines = append(lines, "| verdict | policy | detail |", "| --- | --- | --- |")
		for _, result := range policies {
			verdict := "PASS"
			if result.Failed {
				verdict = "**FAIL**"
			}
			lines = append(lines, fmt.Sprintf("| %s | `%s %s` | %s |",
				verdict, result.Policy, result.Subject, result.Detail))
		}
		lines = append(lines, "")

		for _, result := range policies {
			if !result.Failed || len(result.Matched) == 0 {
				continue
			}
			shown := result.Matched
			if limit > 0 && len(shown) > limit {
				shown = shown[:limit]
			}
			for _, node := range shown {
				if witness, ok := result.Witnesses[node]; ok && len(witness) > 0 {
					lines = append(lines, fmt.Sprintf("- `%s` &larr; `%s`",
						node, strings.Join(witness, " -> ")))
				} else {
					lines = append(lines, fmt.Sprintf("- `%s`", node))
				}
			}
			if limit > 0 && len(result.Matched) > limit {
				lines = append(lines, fmt.Sprintf("- (+%d more)", len(result.Matched)-limit))
			}
			lines = append(lines, "")
		}
	}

	lines = append(lines, "<details>", "<summary>Full report</summary>", "", "```text")
	lines = append(lines, body[1:]...)
	return append(lines, "```", "", "</details>")
}
