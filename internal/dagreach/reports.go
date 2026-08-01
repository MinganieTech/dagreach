package dagreach

// The two analyses a command reports on: the shape of a graph, and what a
// change reaches in it.

import "math"

// GraphStats is everything `dagreach stats` reports.
type GraphStats struct {
	Nodes              int
	Edges              int
	Roots              []string
	Leaves             []string
	Isolated           []string
	Cycles             [][]string
	Depth              int
	Width              int
	WidestLevel        []string
	ArticulationPoints []string
	Ranking            []NodeReach
	RankingSkipped     bool
	LongestPath        *CriticalPath
	Groups             *Counter
	Condensed          bool
	CollapsedCycles    map[string][]string
}

func (s *GraphStats) Acyclic() bool { return len(s.Cycles) == 0 }

// Analyse measures a graph. Depth, width and the longest path need an acyclic
// graph, so when cycles remain they are measured on the condensation - and the
// report says so.
func Analyse(g *Graph) *GraphStats {
	adjacency := BuildAdjacency(g)
	cycles := Cycles(g, adjacency)

	work, workAdjacency := g, adjacency
	collapsed := map[string][]string{}
	if len(cycles) > 0 {
		condensed, memberOf := Condense(g, adjacency)
		work, workAdjacency = condensed, BuildAdjacency(condensed)
		for _, member := range g.Nodes() {
			if name := memberOf[member]; name != member {
				collapsed[name] = append(collapsed[name], member)
			}
		}
	}

	levels, acyclicWork := TopologicalLevels(work, workAdjacency)
	roots, leaves, isolated := []string{}, []string{}, []string{}
	for _, node := range g.Nodes() {
		noPredecessors := len(adjacency.Predecessors[node]) == 0
		noSuccessors := len(adjacency.Successors[node]) == 0
		if noPredecessors {
			roots = append(roots, node)
			if noSuccessors {
				isolated = append(isolated, node)
			}
		}
		if noSuccessors {
			leaves = append(leaves, node)
		}
	}

	// The condensation is acyclic by construction, so both are always measurable.
	widest := []string{}
	for _, level := range levels {
		if len(level) > len(widest) {
			widest = level
		}
	}
	if !acyclicWork {
		panic("the condensation of a graph cannot hold a cycle")
	}

	groups := NewCounter()
	for _, node := range g.Nodes() {
		if group := textAttr(g.Node(node).Attrs, GroupKey); group != "" {
			groups.Add(group)
		}
	}

	path, _ := LongestPath(work, workAdjacency, nil)

	// The ranking holds one reachable set per node, so its memory grows with the
	// square of the graph: 157 MB at 20k nodes, 2.7 GB at 100k. Past the ceiling
	// it is skipped and said to be skipped, rather than quietly eating the
	// machine or quietly disappearing.
	ranking, skipped := []NodeReach{}, g.NodeCount() > RankingCeiling
	if !skipped {
		ranking = ReachRanking(g)
	}

	return &GraphStats{
		Nodes:              g.NodeCount(),
		Edges:              g.EdgeCount(),
		Roots:              roots,
		Leaves:             leaves,
		Isolated:           isolated,
		Cycles:             cycles,
		Depth:              len(levels),
		Width:              len(widest),
		WidestLevel:        widest,
		ArticulationPoints: ArticulationPoints(g, adjacency),
		Ranking:            ranking,
		RankingSkipped:     skipped,
		LongestPath:        path,
		Groups:             groups,
		Condensed:          len(cycles) > 0,
		CollapsedCycles:    collapsed,
	}
}

// ImpactReport is everything `dagreach impact` reports.
type ImpactReport struct {
	Seeds                      []string
	Downstream                 []string
	Upstream                   []string
	ImpactedLeaves             []string
	ImpactedArticulationPoints []string
	Groups                     *Counter
	Cost                       *float64
	LongestPath                *CriticalPath
	TotalNodes                 int
	Witnesses                  *WitnessIndex
}

// Impacted is the seeds plus everything below them: what a change actually touches.
func (r *ImpactReport) Impacted() []string {
	impacted := make([]string, 0, len(r.Seeds)+len(r.Downstream))
	impacted = append(impacted, r.Seeds...)
	return append(impacted, r.Downstream...)
}

func (r *ImpactReport) Share() float64 {
	if r.TotalNodes == 0 {
		return 0
	}
	return float64(len(r.Impacted())) / float64(r.TotalNodes)
}

// Impact reports what changing `seeds` reaches.
func Impact(g *Graph, seeds []string) *ImpactReport {
	adjacency := BuildAdjacency(g)
	seedSet := map[string]bool{}
	for _, seed := range seeds {
		seedSet[seed] = true
	}
	ordered := inDeclarationOrder(g, seedSet)

	index := Witnesses(g, ordered, "down", adjacency)
	downstream := []string{}
	impacted := map[string]bool{}
	for _, node := range g.Nodes() {
		if index.Known[node] {
			impacted[node] = true
			if !seedSet[node] {
				downstream = append(downstream, node)
			}
		}
	}

	var cost *float64
	if UsesDurations(g) {
		total := 0.0
		for _, node := range g.Nodes() {
			if !impacted[node] {
				continue
			}
			if value, ok := durationOf(g.Node(node).Attrs); ok {
				total += value
			}
		}
		cost = &total
	}

	groups := NewCounter()
	leaves := []string{}
	for _, node := range g.Nodes() {
		if !impacted[node] {
			continue
		}
		if group := textAttr(g.Node(node).Attrs, GroupKey); group != "" {
			groups.Add(group)
		}
		if len(adjacency.Successors[node]) == 0 {
			leaves = append(leaves, node)
		}
	}

	cutVertices := map[string]bool{}
	for _, node := range ArticulationPoints(g, adjacency) {
		cutVertices[node] = true
	}
	seedCuts := []string{}
	for _, seed := range ordered {
		if cutVertices[seed] {
			seedCuts = append(seedCuts, seed)
		}
	}

	path, _ := LongestPath(g, adjacency, impacted)

	return &ImpactReport{
		Seeds:                      ordered,
		Downstream:                 downstream,
		Upstream:                   Reachable(g, ordered, "up", adjacency),
		ImpactedLeaves:             leaves,
		ImpactedArticulationPoints: seedCuts,
		Groups:                     groups,
		Cost:                       cost,
		LongestPath:                path,
		TotalNodes:                 g.NodeCount(),
		Witnesses:                  index,
	}
}

func roundTo(value float64, places int) float64 {
	scale := math.Pow(10, float64(places))
	return math.Round(value*scale) / scale
}
