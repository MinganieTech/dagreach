package dagreach

import (
	"fmt"
	"strings"
	"testing"
)

const (
	diamond = "digraph { a -> b; a -> c; b -> d; c -> d }"
	chain   = "digraph { a -> b -> c }"
)

func reachOf(t *testing.T, text string, seeds []string, direction string) []string {
	t.Helper()
	graph := mustParseDOT(t, text)
	return Reachable(graph, seeds, direction, BuildAdjacency(graph))
}

func TestReachableExcludesTheSeeds(t *testing.T) {
	assertEqual(t, reachOf(t, diamond, []string{"a"}, "down"), []string{"b", "c", "d"}, "down")
	assertEqual(t, reachOf(t, diamond, []string{"d"}, "up"), []string{"a", "b", "c"}, "up")
	assertEqual(t, reachOf(t, diamond, []string{"b"}, "down"), []string{"d"}, "down from b")
	assertEqual(t, reachOf(t, diamond, []string{"a"}, "up"), []string{}, "up from a")
}

func TestReachableFollowsDeclarationOrder(t *testing.T) {
	assertEqual(t, reachOf(t, "digraph { z -> m; m -> a; z -> a }", []string{"z"}, "down"),
		[]string{"m", "a"}, "declaration order")
}

func TestLevelsGroupNodesByEarliestPosition(t *testing.T) {
	graph := mustParseDOT(t, diamond)
	levels, ok := TopologicalLevels(graph, BuildAdjacency(graph))
	if !ok || len(levels) != 3 {
		t.Fatalf("levels = %v (ok=%v)", levels, ok)
	}
	assertEqual(t, levels[0], []string{"a"}, "level 0")
	assertEqual(t, levels[1], []string{"b", "c"}, "level 1")
	assertEqual(t, levels[2], []string{"d"}, "level 2")
}

func TestLevelsAreRefusedWhenACycleRemains(t *testing.T) {
	graph := mustParseDOT(t, "digraph { a -> b -> c -> a }")
	if _, ok := TopologicalLevels(graph, BuildAdjacency(graph)); ok {
		t.Error("levels should not be computable with a cycle")
	}
}

func TestCycleDetection(t *testing.T) {
	cases := []struct {
		text string
		want string
	}{
		{"digraph { a -> b -> c -> a }", "a,b,c"},
		{"digraph { a -> a }", "a"},
		{"digraph { a -> b; b -> a; c -> d }", "a,b"},
		{diamond, ""},
	}
	for _, testCase := range cases {
		graph := mustParseDOT(t, testCase.text)
		cycles := Cycles(graph, BuildAdjacency(graph))
		rendered := []string{}
		for _, cycle := range cycles {
			rendered = append(rendered, strings.Join(cycle, ","))
		}
		if strings.Join(rendered, "|") != testCase.want {
			t.Errorf("%s: cycles = %v, wanted %q", testCase.text, rendered, testCase.want)
		}
	}
}

func TestArticulationPointsFindTheSinglePointsOfPassage(t *testing.T) {
	points := func(text string) []string {
		graph := mustParseDOT(t, text)
		return ArticulationPoints(graph, BuildAdjacency(graph))
	}
	assertEqual(t, points(chain), []string{"b"}, "chain")
	assertEqual(t, points(diamond), []string{}, "diamond has two routes")
	assertEqual(t, points("digraph { a -> centre; b -> centre; centre -> c; centre -> d; a -> b; c -> d }"),
		[]string{"centre"}, "bow tie")
	assertEqual(t, points("digraph { a -> a }"), []string{}, "a self-loop disconnects nothing")
}

func TestUnweightedLongestPathCountsEdges(t *testing.T) {
	graph := mustParseDOT(t, "digraph { a -> b -> c -> d; a -> d }")
	path, ok := LongestPath(graph, BuildAdjacency(graph), nil)
	if !ok {
		t.Fatal("no path")
	}
	if path.Weighted {
		t.Error("no durations were declared")
	}
	assertEqual(t, path.Nodes, []string{"a", "b", "c", "d"}, "path")
	if path.Label() != "longest path" || !strings.Contains(path.Describe(), "structural") {
		t.Errorf("label=%q describe=%q", path.Label(), path.Describe())
	}
}

func TestWeightedCriticalPathFollowsTheSlowestBranch(t *testing.T) {
	graph := mustParseDOT(t, `
		digraph {
			a [duration = "1"]
			fast [duration = "1"]
			slow [duration = "10"]
			z [duration = "1"]
			a -> fast -> z
			a -> slow -> z
		}`)
	path, _ := LongestPath(graph, BuildAdjacency(graph), nil)
	assertEqual(t, path.Nodes, []string{"a", "slow", "z"}, "path")
	if !path.Weighted || path.Cost != 12 || path.Label() != "critical path" {
		t.Errorf("weighted=%v cost=%v label=%q", path.Weighted, path.Cost, path.Label())
	}
	if got := path.Describe(); got != "12 of duration over 2 edge(s)" {
		t.Errorf("describe = %q", got)
	}
}

func TestEdgeDurationsCountTowardsThePath(t *testing.T) {
	graph := mustParseDOT(t, `digraph { a -> b [duration = "5"]; a -> c [duration = "1"] }`)
	path, _ := LongestPath(graph, BuildAdjacency(graph), nil)
	assertEqual(t, path.Nodes, []string{"a", "b"}, "path")
	if path.Cost != 5 {
		t.Errorf("cost = %v", path.Cost)
	}
}

func TestLongestPathCanBeRestrictedToASubset(t *testing.T) {
	graph := mustParseDOT(t, "digraph { a -> b -> c -> d }")
	path, _ := LongestPath(graph, BuildAdjacency(graph), map[string]bool{"b": true, "c": true})
	assertEqual(t, path.Nodes, []string{"b", "c"}, "restricted path")
}

func TestCondensationCollapsesCyclesAndSumsTheirDurations(t *testing.T) {
	graph := mustParseDOT(t, `digraph { a [duration="2"] b [duration="3"] a -> b -> a; b -> c }`)
	condensed, memberOf := Condense(graph, BuildAdjacency(graph))
	if memberOf["a"] != "scc(a+1)" || memberOf["b"] != "scc(a+1)" {
		t.Fatalf("membership = %v", memberOf)
	}
	// Tarjan emits components in reverse topological order, so the sink comes first.
	assertEqual(t, condensed.Nodes(), []string{"c", "scc(a+1)"}, "condensed nodes")
	if got := condensed.Node("scc(a+1)").Attrs["duration"]; got != "5" {
		t.Errorf("collapsed duration = %q, wanted the sum", got)
	}
}

func TestAnalyseReportsTheShapeOfAGraph(t *testing.T) {
	stats := Analyse(mustParseDOT(t, diamond))
	if stats.Nodes != 4 || stats.Edges != 4 || !stats.Acyclic() {
		t.Fatalf("nodes=%d edges=%d acyclic=%v", stats.Nodes, stats.Edges, stats.Acyclic())
	}
	if stats.Depth != 3 || stats.Width != 2 {
		t.Errorf("depth=%d width=%d", stats.Depth, stats.Width)
	}
	assertEqual(t, stats.WidestLevel, []string{"b", "c"}, "widest generation")
	assertEqual(t, stats.Roots, []string{"a"}, "roots")
	assertEqual(t, stats.Leaves, []string{"d"}, "leaves")
	assertEqual(t, stats.Isolated, []string{}, "isolated")
}

func TestAnalyseCondensesCyclesRatherThanGivingUp(t *testing.T) {
	stats := Analyse(mustParseDOT(t, "digraph { a -> b -> c -> a; c -> d }"))
	if stats.Acyclic() || !stats.Condensed {
		t.Fatalf("acyclic=%v condensed=%v", stats.Acyclic(), stats.Condensed)
	}
	assertEqual(t, stats.Cycles[0], []string{"a", "b", "c"}, "cycle")
	assertEqual(t, stats.CollapsedCycles["scc(a+2)"], []string{"a", "b", "c"}, "collapsed")
	if stats.Depth != 2 || stats.Width != 1 {
		t.Errorf("depth=%d width=%d, expected the condensation", stats.Depth, stats.Width)
	}
	assertEqual(t, stats.LongestPath.Nodes, []string{"scc(a+2)", "d"}, "path over the condensation")
}

func TestImpactReportsWhatAChangeReaches(t *testing.T) {
	graph := mustParseDOT(t, `
		digraph {
			auth [group = "core", duration = "10"]
			billing [group = "core", duration = "5"]
			web [group = "edge", duration = "2"]
			unrelated [group = "edge", duration = "100"]
			db -> auth
			auth -> billing -> web
		}`)
	report := Impact(graph, []string{"auth"})
	assertEqual(t, report.Seeds, []string{"auth"}, "seeds")
	assertEqual(t, report.Downstream, []string{"billing", "web"}, "downstream")
	assertEqual(t, report.Upstream, []string{"db"}, "upstream")
	assertEqual(t, report.ImpactedLeaves, []string{"web"}, "leaves")
	if report.TotalNodes != 5 || fmt.Sprintf("%.2f", report.Share()) != "0.60" {
		t.Errorf("total=%d share=%v", report.TotalNodes, report.Share())
	}
	if *report.Cost != 17 {
		t.Errorf("cost = %v", *report.Cost)
	}
	if report.Groups.Get("core") != 2 || report.Groups.Get("edge") != 1 {
		t.Errorf("groups = %v", report.Groups.Map())
	}
	assertEqual(t, report.LongestPath.Nodes, []string{"auth", "billing", "web"}, "path")
}

func TestImpactFlagsASeedThatIsAnArticulationPoint(t *testing.T) {
	report := Impact(mustParseDOT(t, chain), []string{"b"})
	assertEqual(t, report.ImpactedArticulationPoints, []string{"b"}, "articulation seeds")
}

func TestImpactOfSeveralSeedsIsTheirUnionInDeclarationOrder(t *testing.T) {
	report := Impact(mustParseDOT(t, "digraph { a -> x; b -> y; c }"), []string{"b", "a"})
	assertEqual(t, report.Seeds, []string{"a", "b"}, "seeds")
	assertEqual(t, report.Downstream, []string{"x", "y"}, "downstream")
}

func TestWitnessesGiveAShortestReasonForEveryReachedNode(t *testing.T) {
	graph := mustParseDOT(t, "digraph { a -> b -> c -> d; a -> d }")
	index := Witnesses(graph, []string{"a"}, "down", BuildAdjacency(graph))
	if index.Distance["d"] != 1 {
		t.Errorf("distance to d = %d, expected the short route", index.Distance["d"])
	}
	assertEqual(t, index.Path("d"), []string{"a", "d"}, "witness for d")
	assertEqual(t, index.Path("c"), []string{"a", "b", "c"}, "witness for c")
	assertEqual(t, index.Path("missing"), nil, "unknown node")
}

func TestWitnessesFromSeveralSeedsPickTheNearestOne(t *testing.T) {
	graph := mustParseDOT(t, "digraph { far -> mid -> target; near -> target }")
	index := Witnesses(graph, []string{"far", "near"}, "down", BuildAdjacency(graph))
	assertEqual(t, index.Path("target"), []string{"near", "target"}, "nearest witness")
}

func TestDeepGraphsDoNotOverflowTheStack(t *testing.T) {
	var built strings.Builder
	built.WriteString("digraph { ")
	for index := 0; index < 10000; index++ {
		fmt.Fprintf(&built, "n%d -> n%d;", index, index+1)
	}
	built.WriteString(" }")

	stats := Analyse(mustParseDOT(t, built.String()))
	if stats.Nodes != 10001 || stats.Depth != 10001 || stats.Width != 1 {
		t.Fatalf("nodes=%d depth=%d width=%d", stats.Nodes, stats.Depth, stats.Width)
	}
	if stats.LongestPath.Edges() != 10000 {
		t.Errorf("path edges = %d", stats.LongestPath.Edges())
	}
	if len(stats.ArticulationPoints) != 9999 {
		t.Errorf("articulation points = %d, expected every node but the two ends",
			len(stats.ArticulationPoints))
	}
}
