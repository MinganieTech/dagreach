import json

import pytest

from dagreach.errors import ParseError
from dagreach.profile import duration_of, group_of, status_of
from dagreach.readers.jgf import parse_jgf


def test_nodes_as_a_list():
    graph = parse_jgf(
        json.dumps(
            {
                "graph": {
                    "label": "etl",
                    "nodes": [{"id": "a", "metadata": {"duration": 12}}, {"id": "b"}],
                    "edges": [{"source": "a", "target": "b"}],
                }
            }
        )
    )
    assert graph.name == "etl"
    assert sorted(graph.nodes) == ["a", "b"]
    assert duration_of(graph.nodes["a"]) == 12.0
    assert [edge.key for edge in graph.edges] == [("a", "b")]


def test_nodes_keyed_by_id():
    graph = parse_jgf(
        json.dumps(
            {
                "graph": {
                    "nodes": {"a": {"metadata": {"status": "ready", "group": "core"}}, "b": {}},
                    "edges": [{"source": "a", "target": "b"}],
                }
            }
        )
    )
    assert status_of(graph.nodes["a"]) == "ready"
    assert group_of(graph.nodes["a"]) == "core"


def test_bare_nodes_and_edges_document_is_accepted_but_flagged():
    graph = parse_jgf(json.dumps({"nodes": [{"id": "a"}, {"id": "b"}], "edges": []}))
    assert sorted(graph.nodes) == ["a", "b"]
    assert any("no 'graph' envelope" in warning for warning in graph.warnings)


def test_non_standard_spellings_are_accepted_but_flagged():
    graph = parse_jgf(json.dumps({"graph": {"links": [{"from": "a", "to": "b"}]}}))
    assert [edge.key for edge in graph.edges] == [("a", "b")]
    assert any("'links'" in warning for warning in graph.warnings)
    assert any("'from'" in warning for warning in graph.warnings)


def test_edges_to_undeclared_nodes_are_kept_and_flagged():
    graph = parse_jgf(
        json.dumps(
            {"graph": {"nodes": [{"id": "a"}], "edges": [{"source": "a", "target": "ghost"}]}}
        )
    )
    assert graph.has_node("ghost")
    assert any("undeclared node 'ghost'" in warning for warning in graph.warnings)


def test_container_metadata_is_preserved_as_json():
    graph = parse_jgf(
        json.dumps({"graph": {"nodes": [{"id": "a", "metadata": {"owners": ["team"]}}]}})
    )
    assert graph.nodes["a"].attrs["owners"] == '["team"]'


def test_several_graphs_reads_the_first_and_says_so():
    graph = parse_jgf(json.dumps({"graphs": [{"nodes": [{"id": "a"}]}, {"nodes": [{"id": "z"}]}]}))
    assert sorted(graph.nodes) == ["a"]
    assert any("only the first one" in warning for warning in graph.warnings)


def test_undirected_graph_is_read_but_flagged():
    graph = parse_jgf(
        json.dumps({"graph": {"directed": False, "edges": [{"source": "a", "target": "b"}]}})
    )
    assert graph.directed is False
    assert any("undirected" in warning for warning in graph.warnings)


def test_invalid_json_reports_the_line():
    with pytest.raises(ParseError) as exc:
        parse_jgf('{\n  "graph": {,}\n}', source="bad.json")
    assert exc.value.line == 2
    assert "invalid JSON" in exc.value.message


@pytest.mark.parametrize(
    ("document", "fragment"),
    [
        ({"whatever": 1}, "expected a 'graph', 'graphs'"),
        ({"graph": {"nodes": [{"label": "no id"}]}}, "has no 'id'"),
        ({"graph": {"edges": [{"source": "a"}]}}, "has no target"),
        ({"graph": {"nodes": "not a container"}}, "'nodes' must be"),
        ({"graph": {"edges": {}}}, "'edges' must be an array"),
        ({"graphs": []}, "non-empty array"),
    ],
)
def test_structural_errors_are_explicit(document, fragment):
    with pytest.raises(ParseError) as exc:
        parse_jgf(json.dumps(document), source="bad.json")
    assert fragment in exc.value.message
