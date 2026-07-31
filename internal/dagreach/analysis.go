package dagreach

// The analysis core.
//
// Every metric here is defined in docs/metrics.md and computed in pure Go: the
// algorithms needed - traversal, topological order, strongly connected
// components, longest path, articulation points - are a few dozen lines each,
// and a graph library would only start to pay for itself on the heavier
// mathematics dagreach deliberately does not promise yet.
//
// Determinism is a property of the output, not an accident: every traversal
// walks nodes in the order the source declared them, so two runs on the same
// file produce the same lists.

import (
	"fmt"
	"math"
	"sort"
	"strconv"
)

// -- adjacency -------------------------------------------------------------

type Adjacency struct {
	Successors   map[string][]string
	Predecessors map[string][]string
}

func BuildAdjacency(g *Graph) *Adjacency {
	adjacency := &Adjacency{
		Successors:   make(map[string][]string, g.NodeCount()),
		Predecessors: make(map[string][]string, g.NodeCount()),
	}
	for _, node := range g.Nodes() {
		adjacency.Successors[node] = nil
		adjacency.Predecessors[node] = nil
	}
	seen := map[pair]bool{}
	for _, edge := range g.Edges {
		key := pair{edge.Source, edge.Target}
		if seen[key] {
			continue
		}
		seen[key] = true
		adjacency.Successors[edge.Source] = append(adjacency.Successors[edge.Source], edge.Target)
		adjacency.Predecessors[edge.Target] = append(adjacency.Predecessors[edge.Target], edge.Source)
	}
	return adjacency
}

func (a *Adjacency) neighbours(node, direction string) []string {
	if direction == "down" {
		return a.Successors[node]
	}
	return a.Predecessors[node]
}

// -- traversal -------------------------------------------------------------

func Reachable(g *Graph, seeds []string, direction string, adjacency *Adjacency) []string {
	seedSet := map[string]bool{}
	for _, seed := range seeds {
		seedSet[seed] = true
	}
	found := map[string]bool{}
	stack := make([]string, 0, len(seeds))
	for index := len(seeds) - 1; index >= 0; index-- {
		stack = append(stack, seeds[index])
	}
	for len(stack) > 0 {
		node := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		for _, neighbour := range adjacency.neighbours(node, direction) {
			if found[neighbour] || seedSet[neighbour] {
				continue
			}
			found[neighbour] = true
			stack = append(stack, neighbour)
		}
	}
	return inDeclarationOrder(g, found)
}

type WitnessIndex struct {
	CameFrom map[string]string
	Known    map[string]bool
	Distance map[string]int
}

func (w *WitnessIndex) Path(node string) []string {
	if !w.Known[node] {
		return nil
	}
	path := []string{node}
	for {
		parent, ok := w.CameFrom[path[len(path)-1]]
		if !ok {
			break
		}
		path = append(path, parent)
	}
	for left, right := 0, len(path)-1; left < right; left, right = left+1, right-1 {
		path[left], path[right] = path[right], path[left]
	}
	return path
}

func Witnesses(g *Graph, seeds []string, direction string, adjacency *Adjacency) *WitnessIndex {
	index := &WitnessIndex{
		CameFrom: map[string]string{},
		Known:    map[string]bool{},
		Distance: map[string]int{},
	}
	queue := make([]string, 0, len(seeds))
	for _, seed := range seeds {
		if index.Known[seed] {
			continue
		}
		index.Known[seed] = true
		index.Distance[seed] = 0
		queue = append(queue, seed)
	}
	for head := 0; head < len(queue); head++ {
		node := queue[head]
		for _, neighbour := range adjacency.neighbours(node, direction) {
			if index.Known[neighbour] {
				continue
			}
			index.Known[neighbour] = true
			index.CameFrom[neighbour] = node
			index.Distance[neighbour] = index.Distance[node] + 1
			queue = append(queue, neighbour)
		}
	}
	return index
}

func inDeclarationOrder(g *Graph, members map[string]bool) []string {
	ordered := []string{}
	for _, node := range g.Nodes() {
		if members[node] {
			ordered = append(ordered, node)
		}
	}
	return ordered
}

// -- components ------------------------------------------------------------

func StronglyConnectedComponents(g *Graph, adjacency *Adjacency) [][]string {
	indexOf := map[string]int{}
	lowOf := map[string]int{}
	onStack := map[string]bool{}
	declaredAt := map[string]int{}
	for position, node := range g.Nodes() {
		declaredAt[node] = position
	}
	var componentStack []string
	counter := 0
	components := [][]string{}

	type frame struct {
		node  string
		child int
	}

	for _, root := range g.Nodes() {
		if _, seen := indexOf[root]; seen {
			continue
		}
		work := []frame{{root, 0}}
		for len(work) > 0 {
			top := &work[len(work)-1]
			if top.child == 0 {
				indexOf[top.node] = counter
				lowOf[top.node] = counter
				counter++
				componentStack = append(componentStack, top.node)
				onStack[top.node] = true
			}
			successors := adjacency.Successors[top.node]
			if top.child < len(successors) {
				child := successors[top.child]
				top.child++
				if _, seen := indexOf[child]; !seen {
					work = append(work, frame{child, 0})
				} else if onStack[child] {
					if indexOf[child] < lowOf[top.node] {
						lowOf[top.node] = indexOf[child]
					}
				}
				continue
			}

			node := top.node
			work = work[:len(work)-1]
			if len(work) > 0 {
				parent := work[len(work)-1].node
				if lowOf[node] < lowOf[parent] {
					lowOf[parent] = lowOf[node]
				}
			}
			if lowOf[node] == indexOf[node] {
				var component []string
				for {
					member := componentStack[len(componentStack)-1]
					componentStack = componentStack[:len(componentStack)-1]
					onStack[member] = false
					component = append(component, member)
					if member == node {
						break
					}
				}
				sortByDeclaration(component, declaredAt)
				components = append(components, component)
			}
		}
	}
	return components
}

func sortByDeclaration(nodes []string, declaredAt map[string]int) {
	sort.Slice(nodes, func(left, right int) bool {
		return declaredAt[nodes[left]] < declaredAt[nodes[right]]
	})
}

func Cycles(g *Graph, adjacency *Adjacency) [][]string {
	cycles := [][]string{}
	for _, component := range StronglyConnectedComponents(g, adjacency) {
		if len(component) > 1 {
			cycles = append(cycles, component)
		}
	}
	for _, edge := range g.Edges {
		if edge.Source == edge.Target {
			cycles = append(cycles, []string{edge.Source})
		}
	}
	return cycles
}

// -- levels ----------------------------------------------------------------

func TopologicalLevels(g *Graph, adjacency *Adjacency) ([][]string, bool) {
	remaining := map[string]int{}
	for _, node := range g.Nodes() {
		remaining[node] = len(adjacency.Predecessors[node])
	}
	levelOf := map[string]int{}
	frontier := []string{}
	for _, node := range g.Nodes() {
		if remaining[node] == 0 {
			frontier = append(frontier, node)
			levelOf[node] = 0
		}
	}
	settled := 0
	for len(frontier) > 0 {
		next := []string{}
		for _, node := range frontier {
			settled++
			level := levelOf[node]
			for _, successor := range adjacency.Successors[node] {
				if level+1 > levelOf[successor] {
					levelOf[successor] = level + 1
				}
				remaining[successor]--
				if remaining[successor] == 0 {
					next = append(next, successor)
				}
			}
		}
		frontier = next
	}
	if settled != len(remaining) {
		return nil, false
	}
	depth := 0
	for _, level := range levelOf {
		if level+1 > depth {
			depth = level + 1
		}
	}
	levels := make([][]string, depth)
	for index := range levels {
		levels[index] = []string{}
	}
	for _, node := range g.Nodes() {
		levels[levelOf[node]] = append(levels[levelOf[node]], node)
	}
	return levels, true
}

// -- articulation points ---------------------------------------------------

func ArticulationPoints(g *Graph, adjacency *Adjacency) []string {
	neighbours := map[string][]string{}
	for _, node := range g.Nodes() {
		seen := map[string]bool{node: true}
		list := []string{}
		for _, group := range [][]string{adjacency.Successors[node], adjacency.Predecessors[node]} {
			for _, other := range group {
				if !seen[other] {
					seen[other] = true
					list = append(list, other)
				}
			}
		}
		neighbours[node] = list
	}

	discovery := map[string]int{}
	low := map[string]int{}
	parent := map[string]string{}
	hasParent := map[string]bool{}
	counter := 0
	found := map[string]bool{}

	type frame struct {
		node  string
		child int
	}

	for _, root := range g.Nodes() {
		if _, seen := discovery[root]; seen {
			continue
		}
		rootChildren := 0
		work := []frame{{root, 0}}
		hasParent[root] = false
		for len(work) > 0 {
			top := &work[len(work)-1]
			if top.child == 0 {
				discovery[top.node] = counter
				low[top.node] = counter
				counter++
			}
			if top.child < len(neighbours[top.node]) {
				child := neighbours[top.node][top.child]
				top.child++
				if hasParent[top.node] && parent[top.node] == child {
					continue
				}
				if _, seen := discovery[child]; seen {
					if discovery[child] < low[top.node] {
						low[top.node] = discovery[child]
					}
					continue
				}
				parent[child] = top.node
				hasParent[child] = true
				if top.node == root {
					rootChildren++
				}
				work = append(work, frame{child, 0})
				continue
			}

			node := top.node
			work = work[:len(work)-1]
			if len(work) > 0 {
				ancestor := work[len(work)-1].node
				if low[node] < low[ancestor] {
					low[ancestor] = low[node]
				}
				if ancestor != root && low[node] >= discovery[ancestor] {
					found[ancestor] = true
				}
			}
		}
		if rootChildren > 1 {
			found[root] = true
		}
	}
	return inDeclarationOrder(g, found)
}

// -- condensation ----------------------------------------------------------

func condensedName(component []string) string {
	if len(component) == 1 {
		return component[0]
	}
	return fmt.Sprintf("scc(%s+%d)", component[0], len(component)-1)
}

func Condense(g *Graph, adjacency *Adjacency) (*Graph, map[string]string) {
	components := StronglyConnectedComponents(g, adjacency)
	memberOf := map[string]string{}
	condensed := NewGraph(g.Source)
	condensed.Name = g.Name
	condensed.Directed = g.Directed
	condensed.Format = g.Format
	condensed.EdgeSemantics = g.EdgeSemantics

	for _, component := range components {
		name := condensedName(component)
		for _, member := range component {
			memberOf[member] = name
		}
		attrs := map[string]string{}
		total, any := 0.0, false
		groups := map[string]bool{}
		for _, member := range component {
			if value, ok := durationOf(g.Node(member).Attrs); ok {
				total += value
				any = true
			}
			groups[textAttr(g.Node(member).Attrs, "group")] = true
		}
		if any {
			attrs["duration"] = formatNumber(total)
		}
		if len(groups) == 1 {
			for only := range groups {
				if only != "" {
					attrs["group"] = only
				}
			}
		}
		condensed.AddNode(name, attrs, true)
	}

	seen := map[pair]bool{}
	for _, edge := range g.Edges {
		source, target := memberOf[edge.Source], memberOf[edge.Target]
		key := pair{source, target}
		if source == target || seen[key] {
			continue
		}
		seen[key] = true
		condensed.AddEdge(source, target, edge.Attrs)
	}
	return condensed, memberOf
}

// -- longest path ----------------------------------------------------------

type CriticalPath struct {
	Nodes    []string
	Cost     float64
	Weighted bool
}

func (c *CriticalPath) Edges() int {
	if len(c.Nodes) == 0 {
		return 0
	}
	return len(c.Nodes) - 1
}

// Label distinguishes the two measures: weighted paths are critical, unweighted
// ones are only the longest.
func (c *CriticalPath) Label() string {
	if c.Weighted {
		return "critical path"
	}
	return "longest path"
}

// Describe states the measurement and never presents edges as a duration.
func (c *CriticalPath) Describe() string {
	if len(c.Nodes) == 0 {
		return "no path"
	}
	if c.Weighted {
		return fmt.Sprintf("%s of duration over %d edge(s)", formatNumber(c.Cost), c.Edges())
	}
	return fmt.Sprintf("%d edge(s), structural (no durations declared)", c.Edges())
}

func UsesDurations(g *Graph) bool {
	for _, node := range g.Nodes() {
		if _, ok := durationOf(g.Node(node).Attrs); ok {
			return true
		}
	}
	for _, edge := range g.Edges {
		if _, ok := durationOf(edge.Attrs); ok {
			return true
		}
	}
	return false
}

func LongestPath(g *Graph, adjacency *Adjacency, within map[string]bool) (*CriticalPath, bool) {
	weighted := UsesDurations(g)
	nodes := []string{}
	included := map[string]bool{}
	for _, node := range g.Nodes() {
		if within == nil || within[node] {
			nodes = append(nodes, node)
			included[node] = true
		}
	}

	edgeCost := map[pair]float64{}
	if weighted {
		for _, edge := range g.Edges {
			if included[edge.Source] && included[edge.Target] {
				cost, _ := durationOf(edge.Attrs)
				key := pair{edge.Source, edge.Target}
				if existing, ok := edgeCost[key]; !ok || cost > existing {
					edgeCost[key] = cost
				}
			}
		}
	}

	order, ok := topologicalOrder(nodes, adjacency, included)
	if !ok {
		return nil, false
	}

	best := map[string]float64{}
	cameFrom := map[string]string{}
	for _, node := range order {
		nodeCost := 0.0
		if weighted {
			if value, ok := durationOf(g.Node(node).Attrs); ok {
				nodeCost = value
			}
		}
		incoming := []string{}
		for _, predecessor := range adjacency.Predecessors[node] {
			if included[predecessor] {
				incoming = append(incoming, predecessor)
			}
		}
		if len(incoming) == 0 {
			best[node] = nodeCost
			continue
		}
		chosen, chosenScore := "", math.Inf(-1)
		for _, predecessor := range incoming {
			step := 1.0
			if weighted {
				step = edgeCost[pair{predecessor, node}]
			}
			score := best[predecessor] + step
			if score > chosenScore {
				chosenScore = score
				chosen = predecessor
			}
		}
		best[node] = chosenScore + nodeCost
		cameFrom[node] = chosen
	}

	if len(best) == 0 {
		return &CriticalPath{Nodes: []string{}, Weighted: weighted}, true
	}

	position := map[string]int{}
	for index, node := range order {
		position[node] = index
	}
	end, endScore, endPosition := "", math.Inf(-1), 0
	for _, node := range order {
		score := best[node]
		if score > endScore || (score == endScore && position[node] < endPosition) {
			end, endScore, endPosition = node, score, position[node]
		}
	}

	path := []string{end}
	for {
		parent, ok := cameFrom[path[len(path)-1]]
		if !ok {
			break
		}
		path = append(path, parent)
	}
	for left, right := 0, len(path)-1; left < right; left, right = left+1, right-1 {
		path[left], path[right] = path[right], path[left]
	}
	return &CriticalPath{Nodes: path, Cost: best[end], Weighted: weighted}, true
}

func topologicalOrder(nodes []string, adjacency *Adjacency, included map[string]bool) ([]string, bool) {
	remaining := map[string]int{}
	for _, node := range nodes {
		count := 0
		for _, predecessor := range adjacency.Predecessors[node] {
			if included[predecessor] {
				count++
			}
		}
		remaining[node] = count
	}
	order := []string{}
	frontier := []string{}
	for _, node := range nodes {
		if remaining[node] == 0 {
			frontier = append(frontier, node)
		}
	}
	for len(frontier) > 0 {
		next := []string{}
		for _, node := range frontier {
			order = append(order, node)
			for _, successor := range adjacency.Successors[node] {
				if !included[successor] {
					continue
				}
				remaining[successor]--
				if remaining[successor] == 0 {
					next = append(next, successor)
				}
			}
		}
		frontier = next
	}
	return order, len(order) == len(nodes)
}

func formatNumber(value float64) string {
	if value == math.Trunc(value) {
		return strconv.FormatInt(int64(value), 10)
	}
	return strconv.FormatFloat(value, 'g', -1, 64)
}
