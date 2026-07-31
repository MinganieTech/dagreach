package dagreach

// Comparing what a change could reach before and after.
//
// The interesting question about two versions of a graph is not "which edges
// moved" - a review already shows that - but which reach relationships became
// possible. One added edge can open thousands of routes; that amplification is
// what a diff should surface.
//
// The default computation is deliberately cheap: two multi-source traversals,
// one per graph.
//
//	R_before = reachable(before, changed)
//	R_after  = reachable(after, changed)
//	added    = R_after - R_before
//	removed  = R_before - R_after
//
// No transitive closure is materialised. The global question - which pairs
// became reachable anywhere in the graph - is a separate, explicitly requested
// analysis, because its result can hold one entry per pair of nodes.

import (
	"fmt"
	"math/big"
	"math/bits"
	"sort"
)

// Reasons a target is newly exposed. The three are exhaustive: a target reached
// now that was not exposed before either did not exist, or was reached over an
// edge that did not exist, or was already reachable and changed classification.
var Reasons = []string{"new-node", "new-edge", "reclassified"}

// Exposure is a target the change reaches now and did not reach before.
type Exposure struct {
	Target string
	Reason string
	Detail string
	Path   []string
}

// ReachDiff is what changed between two graphs, from the point of view of the change.
type ReachDiff struct {
	Seeds              []string
	SeedsMissingBefore []string
	SeedsMissingAfter  []string
	ReachedBefore      []string
	ReachedAfter       []string
	Added              []string
	Removed            []string
	NodesAdded         []string
	NodesRemoved       []string
	EdgesAdded         [][2]string
	EdgesRemoved       [][2]string
	WitnessesAfter     *WitnessIndex
}

func (d *ReachDiff) Unchanged() bool { return len(d.Added) == 0 && len(d.Removed) == 0 }

// DiffReach computes the reach delta from seeds, plus the structural change that
// explains it.
func DiffReach(before, after *Graph, seeds []string) *ReachDiff {
	beforeAdjacency := BuildAdjacency(before)
	afterAdjacency := BuildAdjacency(after)

	reachedBefore, _ := reachFrom(before, seeds, beforeAdjacency)
	reachedAfter, afterIndex := reachFrom(after, seeds, afterAdjacency)

	beforeEdges := edgeSet(before)
	afterEdges := edgeSet(after)

	gained := map[string]bool{}
	for node := range reachedAfter {
		if !reachedBefore[node] {
			gained[node] = true
		}
	}
	lost := map[string]bool{}
	for node := range reachedBefore {
		if !reachedAfter[node] {
			lost[node] = true
		}
	}

	missingBefore, missingAfter := []string{}, []string{}
	for _, seed := range seeds {
		if !before.HasNode(seed) {
			missingBefore = append(missingBefore, seed)
		}
		if !after.HasNode(seed) {
			missingAfter = append(missingAfter, seed)
		}
	}

	nodesAdded := []string{}
	for _, node := range after.Nodes() {
		if !before.HasNode(node) {
			nodesAdded = append(nodesAdded, node)
		}
	}
	nodesRemoved := []string{}
	for _, node := range before.Nodes() {
		if !after.HasNode(node) {
			nodesRemoved = append(nodesRemoved, node)
		}
	}

	edgesAdded := [][2]string{}
	for _, key := range orderedEdges(after) {
		if !beforeEdges[pair{key[0], key[1]}] {
			edgesAdded = append(edgesAdded, key)
		}
	}
	edgesRemoved := [][2]string{}
	for _, key := range orderedEdges(before) {
		if !afterEdges[pair{key[0], key[1]}] {
			edgesRemoved = append(edgesRemoved, key)
		}
	}

	return &ReachDiff{
		Seeds:              seeds,
		SeedsMissingBefore: missingBefore,
		SeedsMissingAfter:  missingAfter,
		ReachedBefore:      inDeclarationOrder(before, reachedBefore),
		ReachedAfter:       inDeclarationOrder(after, reachedAfter),
		Added:              inDeclarationOrder(after, gained),
		Removed:            inDeclarationOrder(before, lost),
		NodesAdded:         nodesAdded,
		NodesRemoved:       nodesRemoved,
		EdgesAdded:         edgesAdded,
		EdgesRemoved:       edgesRemoved,
		WitnessesAfter:     afterIndex,
	}
}

func reachFrom(g *Graph, seeds []string, adjacency *Adjacency) (map[string]bool, *WitnessIndex) {
	present := []string{}
	for _, seed := range seeds {
		if g.HasNode(seed) {
			present = append(present, seed)
		}
	}
	index := Witnesses(g, present, "down", adjacency)
	return index.Known, index
}

func edgeSet(g *Graph) map[pair]bool {
	set := map[pair]bool{}
	for _, edge := range g.Edges {
		set[pair{edge.Source, edge.Target}] = true
	}
	return set
}

func orderedEdges(g *Graph) [][2]string {
	seen := map[pair]bool{}
	ordered := [][2]string{}
	for _, edge := range g.Edges {
		key := pair{edge.Source, edge.Target}
		if seen[key] {
			continue
		}
		seen[key] = true
		ordered = append(ordered, [2]string{edge.Source, edge.Target})
	}
	return ordered
}

// NewlyExposed lists the targets the change reaches now and did not reach before.
//
// A target counts as exposed when it matches the selector and is reached in
// `after`, while it did not both match and get reached in `before`. Stated on the
// pair rather than on reachability alone, this also catches the target that was
// always reachable and has just been reclassified - no new edge required, and the
// reason says which of the two happened.
func NewlyExposed(before, after *Graph, diff *ReachDiff, selector Selector) []*Exposure {
	reachedBefore := map[string]bool{}
	for _, node := range diff.ReachedBefore {
		reachedBefore[node] = true
	}
	reachedAfter := map[string]bool{}
	for _, node := range diff.ReachedAfter {
		reachedAfter[node] = true
	}
	exposedBefore := map[string]bool{}
	for _, node := range selector.Select(before) {
		if reachedBefore[node] {
			exposedBefore[node] = true
		}
	}
	beforeEdges := edgeSet(before)

	exposures := []*Exposure{}
	for _, target := range selector.Select(after) {
		if !reachedAfter[target] || exposedBefore[target] {
			continue
		}
		path := diff.WitnessesAfter.Path(target)
		exposures = append(exposures, explainExposure(before, target, path, beforeEdges, selector))
	}
	return exposures
}

func explainExposure(
	before *Graph, target string, path []string, beforeEdges map[pair]bool, selector Selector,
) *Exposure {
	if !before.HasNode(target) {
		return &Exposure{Target: target, Reason: "new-node",
			Detail: fmt.Sprintf("'%s' did not exist before", target), Path: path}
	}

	for index := 0; index+1 < len(path); index++ {
		key := pair{path[index], path[index+1]}
		if !beforeEdges[key] {
			return &Exposure{Target: target, Reason: "new-edge",
				Detail: fmt.Sprintf("new edge %s -> %s", key.Source, key.Target), Path: path}
		}
	}

	// Nothing on the path is new and the target existed, so it was reachable before;
	// since it was not exposed then, its classification is what changed.
	was := "unset"
	if value, present := before.Node(target).Attrs[selector.Key]; present {
		was = value
	}
	return &Exposure{Target: target, Reason: "reclassified",
		Detail: fmt.Sprintf("already reachable, and %s changed from '%s' to '%s'",
			selector.Key, was, selector.Value),
		Path: path}
}

// -- the opt-in global analysis --------------------------------------------

// AllPairsDelta is how many ordered pairs became - or stopped being - reachable.
type AllPairsDelta struct {
	AddedTotal      int
	RemovedTotal    int
	AddedBySource   map[string]int
	RemovedBySource map[string]int
	Sources         int
}

// DiffAllPairs compares reachability over every ordered pair of nodes.
//
// Aggregated by source on purpose: the full answer holds one entry per pair, so a
// graph of any size produces a result no one reads. Reachable sets are built as
// bitsets in reverse topological order, which keeps the work proportional to the
// edges rather than to the pairs - but the size of the answer is the reason this
// analysis is opt-in, not its speed.
func DiffAllPairs(before, after *Graph) *AllPairsDelta {
	universe := append([]string{}, after.Nodes()...)
	for _, node := range before.Nodes() {
		if !after.HasNode(node) {
			universe = append(universe, node)
		}
	}
	bitOf := map[string]int{}
	for index, node := range universe {
		bitOf[node] = index
	}

	beforeSets := reachableBitsets(before, bitOf)
	afterSets := reachableBitsets(after, bitOf)

	delta := &AllPairsDelta{
		AddedBySource:   map[string]int{},
		RemovedBySource: map[string]int{},
		Sources:         len(universe),
	}
	zero := new(big.Int)
	for _, node := range universe {
		beforeBits, ok := beforeSets[node]
		if !ok {
			beforeBits = zero
		}
		afterBits, ok := afterSets[node]
		if !ok {
			afterBits = zero
		}
		added := bitCount(new(big.Int).AndNot(afterBits, beforeBits))
		removed := bitCount(new(big.Int).AndNot(beforeBits, afterBits))
		if added > 0 {
			delta.AddedBySource[node] = added
			delta.AddedTotal += added
		}
		if removed > 0 {
			delta.RemovedBySource[node] = removed
			delta.RemovedTotal += removed
		}
	}
	return delta
}

func bitCount(value *big.Int) int {
	count := 0
	for _, word := range value.Bits() {
		count += bits.OnesCount(uint(word))
	}
	return count
}

// reachableBitsets gives one bitset per node: everything it reaches, itself excluded
// unless it sits in a cycle.
func reachableBitsets(g *Graph, bitOf map[string]int) map[string]*big.Int {
	working, memberOf := g, map[string]string{}
	for _, node := range g.Nodes() {
		memberOf[node] = node
	}
	adjacency := BuildAdjacency(g)
	if len(Cycles(g, adjacency)) > 0 {
		working, memberOf = Condense(g, adjacency)
		adjacency = BuildAdjacency(working)
	}

	members := map[string][]string{}
	for _, node := range g.Nodes() {
		component := memberOf[node]
		members[component] = append(members[component], node)
	}

	order := reverseTopologicalOrder(working, adjacency)
	reachOf := map[string]*big.Int{}
	for _, node := range order {
		bits := new(big.Int)
		for _, successor := range adjacency.Successors[node] {
			bits.Or(bits, reachOf[successor])
			for _, member := range members[successor] {
				bits.SetBit(bits, bitOf[member], 1)
			}
		}
		reachOf[node] = bits
	}

	expanded := map[string]*big.Int{}
	componentNames := make([]string, 0, len(members))
	for name := range members {
		componentNames = append(componentNames, name)
	}
	sort.Strings(componentNames)
	for _, component := range componentNames {
		componentMembers := members[component]
		bits := new(big.Int).Set(reachOf[component])
		if len(componentMembers) > 1 {
			// Inside a cycle everything reaches everything, itself included.
			for _, member := range componentMembers {
				bits.SetBit(bits, bitOf[member], 1)
			}
		}
		for _, member := range componentMembers {
			expanded[member] = bits
		}
	}
	return expanded
}

func reverseTopologicalOrder(g *Graph, adjacency *Adjacency) []string {
	remaining := map[string]int{}
	for _, node := range g.Nodes() {
		remaining[node] = len(adjacency.Successors[node])
	}
	order := []string{}
	frontier := []string{}
	for _, node := range g.Nodes() {
		if remaining[node] == 0 {
			frontier = append(frontier, node)
		}
	}
	for len(frontier) > 0 {
		next := []string{}
		for _, node := range frontier {
			order = append(order, node)
			for _, predecessor := range adjacency.Predecessors[node] {
				remaining[predecessor]--
				if remaining[predecessor] == 0 {
					next = append(next, predecessor)
				}
			}
		}
		frontier = next
	}
	return order
}
