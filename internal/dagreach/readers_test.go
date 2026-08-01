package dagreach

import (
	"strings"
	"testing"
)

func mustParseJGF(t *testing.T, text string) *Graph {
	t.Helper()
	graph, err := ParseJGF(text, "")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	return graph
}

func TestJGFNodesAsAList(t *testing.T) {
	graph := mustParseJGF(t, `{"graph": {"label": "etl",
		"nodes": [{"id": "a", "metadata": {"duration": 12}}, {"id": "b"}],
		"edges": [{"source": "a", "target": "b"}]}}`)
	if graph.Name != "etl" {
		t.Errorf("name = %q", graph.Name)
	}
	assertEqual(t, graph.Nodes(), []string{"a", "b"}, "nodes")
	if value, _ := durationOf(graph.Node("a").Attrs); value != 12 {
		t.Errorf("duration = %v", value)
	}
	assertEqual(t, edgeKeys(graph), []string{"a->b"}, "edges")
}

func TestJGFNodesKeyedByIDKeepDocumentOrder(t *testing.T) {
	graph := mustParseJGF(t, `{"graph": {"nodes": {
		"gateway": {"metadata": {"group": "edge"}},
		"auth": {"metadata": {"status": "ready"}},
		"billing": {}}}}`)
	assertEqual(t, graph.Nodes(), []string{"gateway", "auth", "billing"}, "declaration order")
	if textAttr(graph.Node("auth").Attrs, StatusKey) != "ready" {
		t.Errorf("status = %v", graph.Node("auth").Attrs)
	}
}

func TestJGFBareDocumentIsAcceptedButFlagged(t *testing.T) {
	graph := mustParseJGF(t, `{"nodes": [{"id": "a"}, {"id": "b"}], "edges": []}`)
	assertEqual(t, graph.Nodes(), []string{"a", "b"}, "nodes")
	if !hasWarning(graph, "no 'graph' envelope") {
		t.Errorf("warnings = %v", graph.Warnings)
	}
}

func TestJGFNonStandardSpellingsAreAcceptedButFlagged(t *testing.T) {
	graph := mustParseJGF(t, `{"graph": {"links": [{"from": "a", "to": "b"}]}}`)
	assertEqual(t, edgeKeys(graph), []string{"a->b"}, "edges")
	if !hasWarning(graph, "'links'") || !hasWarning(graph, "'from'") {
		t.Errorf("warnings = %v", graph.Warnings)
	}
}

func TestJGFEdgesToUndeclaredNodesAreKeptAndFlagged(t *testing.T) {
	graph := mustParseJGF(t,
		`{"graph": {"nodes": [{"id": "a"}], "edges": [{"source": "a", "target": "ghost"}]}}`)
	if !graph.HasNode("ghost") || !hasWarning(graph, "undeclared node 'ghost'") {
		t.Errorf("nodes=%v warnings=%v", graph.Nodes(), graph.Warnings)
	}
}

func TestJGFContainerMetadataIsPreservedAsJSON(t *testing.T) {
	graph := mustParseJGF(t, `{"graph": {"nodes": [{"id": "a", "metadata": {"owners": ["team"]}}]}}`)
	if got := graph.Node("a").Attrs["owners"]; got != `["team"]` {
		t.Errorf("owners = %q", got)
	}
}

func TestJGFSeveralGraphsReadsTheFirstAndSaysSo(t *testing.T) {
	graph := mustParseJGF(t, `{"graphs": [{"nodes": [{"id": "a"}]}, {"nodes": [{"id": "z"}]}]}`)
	assertEqual(t, graph.Nodes(), []string{"a"}, "nodes")
	if !hasWarning(graph, "only the first one") {
		t.Errorf("warnings = %v", graph.Warnings)
	}
}

func TestJGFStructuralErrorsAreExplicit(t *testing.T) {
	cases := []struct{ text, fragment string }{
		{`{"whatever": 1}`, "expected a 'graph', 'graphs'"},
		{`{"graph": {"nodes": [{"label": "no id"}]}}`, "has no 'id'"},
		{`{"graph": {"edges": [{"source": "a"}]}}`, "has no target"},
		{`{"graph": {"nodes": "not a container"}}`, "'nodes' must be"},
		{`{"graph": {"edges": {}}}`, "'edges' must be an array"},
		{`{"graphs": []}`, "non-empty array"},
	}
	for _, testCase := range cases {
		_, err := ParseJGF(testCase.text, "bad.json")
		if err == nil || !strings.Contains(err.Error(), testCase.fragment) {
			t.Errorf("%s: error = %v, wanted %q", testCase.text, err, testCase.fragment)
		}
	}
}

func TestFormatDetection(t *testing.T) {
	cases := []struct{ text, hint, want string }{
		{"digraph { a }", "", "dot"},
		{"  strict graph g { }", "", "dot"},
		{"// comment\n/* block */\ndigraph { }", "", "dot"},
		{`{"graph": {}}`, "", "jgf"},
		{"digraph { a }", "plan.dot", "dot"},
		{`{"graph": {}}`, "plan.json", "jgf"},
		{"anything at all", "plan.gv", "dot"},
	}
	for _, testCase := range cases {
		got, err := DetectFormat(testCase.text, testCase.hint)
		if err != nil || got != testCase.want {
			t.Errorf("detect(%q, %q) = %q, %v", testCase.text, testCase.hint, got, err)
		}
	}
}

func TestUndetectableInputAsksForTheFlag(t *testing.T) {
	_, err := DetectFormat("just some prose", "")
	if err == nil || !strings.Contains(err.Error(), "--format") {
		t.Errorf("error = %v", err)
	}
}

func TestExplicitFormatOverridesDetection(t *testing.T) {
	graph, err := ReadText(`{"graph": {"nodes": [{"id": "a"}]}}`, "", "jgf", "misnamed.dot")
	if err != nil {
		t.Fatal(err)
	}
	if graph.Format != "jgf" {
		t.Errorf("format = %q", graph.Format)
	}
}

func TestMissingFileIsACleanError(t *testing.T) {
	_, _, err := ReadSource("testdata/does-not-exist.dot", nil)
	if err == nil || !strings.Contains(err.Error(), "no such file") {
		t.Errorf("error = %v", err)
	}
	if _, isInput := err.(*InputError); !isInput {
		t.Errorf("expected an InputError, got %T", err)
	}
}

func TestReadsStdin(t *testing.T) {
	text, source, err := ReadSource("-", strings.NewReader("digraph { a -> b }"))
	if err != nil || source != "<stdin>" || !strings.Contains(text, "a -> b") {
		t.Errorf("text=%q source=%q err=%v", text, source, err)
	}
}

func TestADocumentIsOneValueAndThenTheEndOfTheFile(t *testing.T) {
	// Two manifests concatenated by a broken pipeline used to be read as the
	// first one, and the report would then describe a graph nobody has.
	for _, text := range []string{
		`{"a": 1} {"b": 2}`,
		`{"a": 1} trailing junk`,
		`{"a": 1}]`,
	} {
		if _, err := DecodeOrderedJSON(text); err == nil {
			t.Errorf("%q was accepted", text)
		} else if !strings.Contains(err.Error(), "after the end of the document") {
			t.Errorf("%q -> %v", text, err)
		}
	}
	for _, text := range []string{`{"a": 1}`, "{\"a\": 1}\n\n", "  [1, 2]  "} {
		if _, err := DecodeOrderedJSON(text); err != nil {
			t.Errorf("%q -> %v", text, err)
		}
	}
}

func TestAKeyDeclaredTwiceMakesTheDocumentAmbiguous(t *testing.T) {
	// JSON says nothing about which one wins, so two readers of the same file
	// can describe two different graphs.
	if _, err := DecodeOrderedJSON(`{"id": "a", "id": "b"}`); err == nil {
		t.Error("a repeated key was accepted")
	} else if !strings.Contains(err.Error(), "declared twice") {
		t.Errorf("err = %v", err)
	}
	if _, err := DecodeOrderedJSON(`{"a": {"id": 1}, "b": {"id": 2}}`); err != nil {
		t.Errorf("the same key in two different objects is ordinary JSON: %v", err)
	}
}
