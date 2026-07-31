package dagreach

import (
	"strings"
	"testing"
)

func mustParseDOT(t *testing.T, text string) *Graph {
	t.Helper()
	graph, err := ParseDOT(text, "")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	return graph
}

func edgeKeys(g *Graph) []string {
	keys := make([]string, 0, len(g.Edges))
	for _, edge := range g.Edges {
		keys = append(keys, edge.Source+"->"+edge.Target)
	}
	return keys
}

func assertEqual(t *testing.T, got, want []string, what string) {
	t.Helper()
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("%s:\n got %v\nwant %v", what, got, want)
	}
}

func TestMinimalDigraph(t *testing.T) {
	graph := mustParseDOT(t, `digraph "g" { a -> b }`)
	if !graph.Directed || graph.Name != "g" {
		t.Errorf("directed=%v name=%q", graph.Directed, graph.Name)
	}
	assertEqual(t, graph.Nodes(), []string{"a", "b"}, "nodes")
	assertEqual(t, edgeKeys(graph), []string{"a->b"}, "edges")
}

func TestEdgeChainExpandsToConsecutivePairs(t *testing.T) {
	graph := mustParseDOT(t, "digraph { a -> b -> c -> d }")
	assertEqual(t, edgeKeys(graph), []string{"a->b", "b->c", "c->d"}, "edges")
}

func TestSubgraphEndpointExpandsToEveryNodeInside(t *testing.T) {
	graph := mustParseDOT(t, "digraph { a -> { b c } -> d }")
	assertEqual(t, edgeKeys(graph), []string{"a->b", "a->c", "b->d", "c->d"}, "edges")
}

func TestDefaultsAreInheritedAndOverridden(t *testing.T) {
	graph := mustParseDOT(t, `
		digraph {
			node [group = "core", status = "pending"]
			edge [duration = "2"]
			a [status = "ready"]
			a -> b
			a -> c [duration = "9"]
		}`)
	if got := graph.Node("a").Attrs["group"]; got != "core" {
		t.Errorf("inherited group = %q", got)
	}
	if got := textAttr(graph.Node("a").Attrs, StatusKey); got != "ready" {
		t.Errorf("explicit status was overwritten by the default: %q", got)
	}
	if got := textAttr(graph.Node("b").Attrs, StatusKey); got != "pending" {
		t.Errorf("default status = %q", got)
	}
	if value, _ := durationOf(graph.Edges[0].Attrs); value != 2 {
		t.Errorf("edge default duration = %v", value)
	}
	if value, _ := durationOf(graph.Edges[1].Attrs); value != 9 {
		t.Errorf("explicit edge duration = %v", value)
	}
}

func TestDefaultsDoNotLeakOutOfASubgraph(t *testing.T) {
	graph := mustParseDOT(t, `
		digraph {
			subgraph cluster_one { node [group = "inner"] a }
			b
		}`)
	if graph.Node("a").Attrs["group"] != "inner" {
		t.Error("the subgraph default did not apply inside")
	}
	if _, present := graph.Node("b").Attrs["group"]; present {
		t.Error("the subgraph default leaked out")
	}
}

func TestPortsAreParsedAndDropped(t *testing.T) {
	graph := mustParseDOT(t, "digraph { a:out:s -> b:in }")
	assertEqual(t, graph.Nodes(), []string{"a", "b"}, "nodes")
	assertEqual(t, edgeKeys(graph), []string{"a->b"}, "edges")
}

func TestQuotedIdentifiersEscapesAndConcatenation(t *testing.T) {
	graph := mustParseDOT(t, "digraph {\n"+
		"  \"say \\\"hi\\\"\" -> \"long\" + \"_name\"\n"+
		"  \"wrapped \\\nline\" -> \"long_name\"\n}")
	for _, wanted := range []string{`say "hi"`, "long_name", "wrapped line"} {
		if !graph.HasNode(wanted) {
			t.Errorf("missing node %q in %v", wanted, graph.Nodes())
		}
	}
}

func TestHTMLLabelsAndAllThreeCommentStyles(t *testing.T) {
	graph := mustParseDOT(t, "# a preprocessor line\ndigraph {\n"+
		"  // a line comment\n  /* a block comment */\n"+
		"  a [label = <<b>bold</b>>]\n  a -> b\n}")
	if got := graph.Node("a").Attrs["label"]; got != "<<b>bold</b>>" {
		t.Errorf("html label = %q", got)
	}
	assertEqual(t, edgeKeys(graph), []string{"a->b"}, "edges")
}

func TestStrictCollapsesParallelEdgesAndSaysSo(t *testing.T) {
	graph := mustParseDOT(t, "strict digraph { a -> b; a -> b; b -> c }")
	assertEqual(t, edgeKeys(graph), []string{"a->b", "b->c"}, "edges")
	if !hasWarning(graph, "collapsed") {
		t.Errorf("no warning about the collapse: %v", graph.Warnings)
	}
}

func TestNonStrictKeepsParallelEdges(t *testing.T) {
	graph := mustParseDOT(t, "digraph { a -> b; a -> b }")
	if graph.EdgeCount() != 2 || graph.DuplicateEdges() != 1 {
		t.Errorf("edges=%d duplicates=%d", graph.EdgeCount(), graph.DuplicateEdges())
	}
}

func TestGraphAttributesAreKept(t *testing.T) {
	graph := mustParseDOT(t, `digraph { rankdir = LR; graph [bgcolor = "white"]; a }`)
	if graph.Attrs["rankdir"] != "LR" || graph.Attrs["bgcolor"] != "white" {
		t.Errorf("graph attrs = %v", graph.Attrs)
	}
}

func TestUndirectedGraphIsReadButFlagged(t *testing.T) {
	graph := mustParseDOT(t, "graph { a -- b }")
	if graph.Directed {
		t.Error("the graph should be undirected")
	}
	assertEqual(t, edgeKeys(graph), []string{"a->b"}, "edges")
	if !hasWarning(graph, "undirected") {
		t.Errorf("no warning: %v", graph.Warnings)
	}
}

func TestWrongEdgeOperatorPointsAtTheLine(t *testing.T) {
	_, err := ParseDOT("digraph {\n  a -- b\n}", "bad.dot")
	parseErr, ok := err.(*ParseError)
	if !ok {
		t.Fatalf("expected a ParseError, got %v", err)
	}
	if parseErr.Line != 2 || !strings.Contains(parseErr.Message, "only valid in a graph") {
		t.Errorf("error = %v", err)
	}
	if !strings.HasPrefix(err.Error(), "bad.dot:2:") {
		t.Errorf("location = %v", err)
	}
}

func TestSyntaxErrorsAreReportedWithALocation(t *testing.T) {
	cases := []struct{ text, fragment string }{
		{"digraph { a -> b", "expected '}'"},
		{"digraph { a [ shape = box }", "expected ']'"},
		{"subgraph { a }", "expected 'graph' or 'digraph'"},
		{`digraph { "unterminated }`, "unterminated quoted string"},
		{"digraph { /* open", "unterminated block comment"},
		{"digraph { a } trailing", "after the closing"},
	}
	for _, testCase := range cases {
		_, err := ParseDOT(testCase.text, "bad.dot")
		parseErr, ok := err.(*ParseError)
		if !ok {
			t.Errorf("%q: expected a ParseError, got %v", testCase.text, err)
			continue
		}
		if !strings.Contains(parseErr.Message, testCase.fragment) {
			t.Errorf("%q: message = %q, wanted %q", testCase.text, parseErr.Message, testCase.fragment)
		}
		if parseErr.Line == 0 {
			t.Errorf("%q: no line in %v", testCase.text, err)
		}
	}
}

func hasWarning(g *Graph, fragment string) bool {
	for _, warning := range g.Warnings {
		if strings.Contains(warning, fragment) {
			return true
		}
	}
	return false
}
