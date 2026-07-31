package dagreach

// The JSON reports. Keys are sorted on the way out, so a consumer can diff two
// runs, and every document carries a schema_version.

// ParseJSON is the machine-readable form of `dagreach parse`.
func ParseJSON(g *Graph, summary *ProfileSummary) map[string]any {
	return map[string]any{
		"schema_version":  SchemaVersion,
		"source":          g.Source,
		"format":          g.Format,
		"profile":         g.Profile,
		"name":            optionalString(g.Name),
		"directed":        g.Directed,
		"edge_semantics":  g.EdgeSemantics,
		"nodes":           g.NodeCount(),
		"edges":           g.EdgeCount(),
		"self_loops":      g.SelfLoops(),
		"duplicate_edges": g.DuplicateEdges(),
		"attributes": map[string]any{
			"nodes_with_duration": summary.NodesWithDuration,
			"edges_with_duration": summary.EdgesWithDuration,
			"statuses":            summary.Statuses.Map(),
			"groups":              summary.Groups.Map(),
		},
		"warnings": append(append([]string{}, g.Warnings...), summary.Unreadable...),
	}
}

// StatsJSON is the machine-readable form of `dagreach stats`.
func StatsJSON(g *Graph, stats *GraphStats, policies []*PolicyResult) map[string]any {
	return map[string]any{
		"schema_version":      SchemaVersion,
		"source":              g.Source,
		"profile":             g.Profile,
		"edge_semantics":      g.EdgeSemantics,
		"nodes":               stats.Nodes,
		"edges":               stats.Edges,
		"acyclic":             stats.Acyclic(),
		"cycles":              stats.Cycles,
		"condensed":           stats.Condensed,
		"collapsed_cycles":    stats.CollapsedCycles,
		"depth":               stats.Depth,
		"width":               stats.Width,
		"widest_generation":   stats.WidestLevel,
		"roots":               stats.Roots,
		"leaves":              stats.Leaves,
		"isolated":            stats.Isolated,
		"articulation_points": stats.ArticulationPoints,
		"longest_path":        criticalPathJSON(stats.LongestPath),
		"groups":              stats.Groups.Map(),
		"policies":            policiesJSON(policies),
		"warnings":            append([]string{}, g.Warnings...),
	}
}

// ImpactJSON is the machine-readable form of `dagreach impact`.
func ImpactJSON(
	g *Graph, report *ImpactReport, policies []*PolicyResult, explain bool,
) map[string]any {
	document := map[string]any{
		"schema_version":               SchemaVersion,
		"source":                       g.Source,
		"profile":                      g.Profile,
		"edge_semantics":               g.EdgeSemantics,
		"changed":                      report.Seeds,
		"impacted":                     report.Impacted(),
		"impacted_count":               len(report.Impacted()),
		"total_nodes":                  report.TotalNodes,
		"share":                        roundTo(report.Share(), 4),
		"downstream":                   report.Downstream,
		"upstream":                     report.Upstream,
		"impacted_leaves":              report.ImpactedLeaves,
		"impacted_articulation_points": report.ImpactedArticulationPoints,
		"groups":                       report.Groups.Map(),
		"cost":                         optionalFloat(report.Cost),
		"longest_path":                 criticalPathJSON(report.LongestPath),
		"policies":                     policiesJSON(policies),
		"warnings":                     append([]string{}, g.Warnings...),
	}
	if explain {
		explained := map[string]any{}
		for _, node := range report.Downstream {
			explained[node] = map[string]any{
				"distance": report.Witnesses.Distance[node],
				"path":     report.Witnesses.Path(node),
			}
		}
		document["explain"] = explained
	}
	return document
}

// DiffJSON is the machine-readable form of `dagreach diff`.
func DiffJSON(
	before, after *Graph, diff *ReachDiff, exposures []*Exposure,
	policies []*PolicyResult, allPairs *AllPairsDelta,
) map[string]any {
	edgesAdded := make([][]string, 0, len(diff.EdgesAdded))
	for _, key := range diff.EdgesAdded {
		edgesAdded = append(edgesAdded, []string{key[0], key[1]})
	}
	edgesRemoved := make([][]string, 0, len(diff.EdgesRemoved))
	for _, key := range diff.EdgesRemoved {
		edgesRemoved = append(edgesRemoved, []string{key[0], key[1]})
	}
	exposed := make([]map[string]any, 0, len(exposures))
	for _, exposure := range exposures {
		exposed = append(exposed, map[string]any{
			"target": exposure.Target,
			"reason": exposure.Reason,
			"detail": exposure.Detail,
			"path":   exposure.Path,
		})
	}

	document := map[string]any{
		"schema_version":         SchemaVersion,
		"before":                 before.Source,
		"after":                  after.Source,
		"profile":                after.Profile,
		"edge_semantics":         after.EdgeSemantics,
		"changed":                diff.Seeds,
		"changed_missing_before": diff.SeedsMissingBefore,
		"changed_missing_after":  diff.SeedsMissingAfter,
		"reached_before":         diff.ReachedBefore,
		"reached_after":          diff.ReachedAfter,
		"added_reach":            diff.Added,
		"removed_reach":          diff.Removed,
		"nodes_added":            diff.NodesAdded,
		"nodes_removed":          diff.NodesRemoved,
		"edges_added":            edgesAdded,
		"edges_removed":          edgesRemoved,
		"exposures":              exposed,
		"policies":               policiesJSON(policies),
		"warnings":               append(append([]string{}, before.Warnings...), after.Warnings...),
	}
	if allPairs != nil {
		document["all_pairs_reachability_delta"] = map[string]any{
			"added_pairs":       allPairs.AddedTotal,
			"removed_pairs":     allPairs.RemovedTotal,
			"sources":           allPairs.Sources,
			"added_by_source":   allPairs.AddedBySource,
			"removed_by_source": allPairs.RemovedBySource,
		}
	}
	return document
}

func criticalPathJSON(path *CriticalPath) any {
	if path == nil {
		return nil
	}
	measure := "edges"
	if path.Weighted {
		measure = "duration"
	}
	nodes := path.Nodes
	if nodes == nil {
		nodes = []string{}
	}
	return map[string]any{
		"nodes":    nodes,
		"edges":    path.Edges(),
		"cost":     path.Cost,
		"weighted": path.Weighted,
		"measure":  measure,
	}
}

func policiesJSON(policies []*PolicyResult) []map[string]any {
	documents := make([]map[string]any, 0, len(policies))
	for _, result := range policies {
		matched := result.Matched
		if matched == nil {
			matched = []string{}
		}
		witnesses := result.Witnesses
		if witnesses == nil {
			witnesses = map[string][]string{}
		}
		documents = append(documents, map[string]any{
			"policy":    result.Policy,
			"subject":   result.Subject,
			"failed":    result.Failed,
			"detail":    result.Detail,
			"matched":   matched,
			"witnesses": witnesses,
		})
	}
	return documents
}

func optionalString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func optionalFloat(value *float64) any {
	if value == nil {
		return nil
	}
	return *value
}
