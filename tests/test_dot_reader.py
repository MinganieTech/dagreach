import pytest

from dagreach.errors import ParseError
from dagreach.profile import duration_of, status_of
from dagreach.readers.dot import parse_dot


def edge_keys(graph):
    return [edge.key for edge in graph.edges]


def test_minimal_digraph():
    graph = parse_dot('digraph "g" { a -> b }')
    assert graph.directed is True
    assert graph.name == "g"
    assert sorted(graph.nodes) == ["a", "b"]
    assert edge_keys(graph) == [("a", "b")]


def test_edge_chain_expands_to_consecutive_pairs():
    graph = parse_dot("digraph { a -> b -> c -> d }")
    assert edge_keys(graph) == [("a", "b"), ("b", "c"), ("c", "d")]


def test_subgraph_endpoint_expands_to_every_node_inside():
    graph = parse_dot("digraph { a -> { b c } -> d }")
    assert edge_keys(graph) == [("a", "b"), ("a", "c"), ("b", "d"), ("c", "d")]


def test_defaults_are_inherited_and_overridden():
    graph = parse_dot(
        """
        digraph {
            node [group = "core", status = "pending"]
            edge [duration = "2"]
            a [status = "ready"]
            a -> b
            a -> c [duration = "9"]
        }
        """
    )
    assert graph.nodes["a"].attrs["group"] == "core"
    assert status_of(graph.nodes["a"]) == "ready"
    assert status_of(graph.nodes["b"]) == "pending"
    assert duration_of(graph.edges[0]) == 2.0
    assert duration_of(graph.edges[1]) == 9.0


def test_defaults_do_not_leak_out_of_a_subgraph():
    graph = parse_dot(
        """
        digraph {
            subgraph cluster_one { node [group = "inner"] a }
            b
        }
        """
    )
    assert graph.nodes["a"].attrs["group"] == "inner"
    assert "group" not in graph.nodes["b"].attrs


def test_ports_are_parsed_and_dropped():
    graph = parse_dot("digraph { a:out:s -> b:in }")
    assert sorted(graph.nodes) == ["a", "b"]
    assert edge_keys(graph) == [("a", "b")]


def test_quoted_identifiers_escapes_and_concatenation():
    graph = parse_dot(
        r"""digraph {
            "say \"hi\"" -> "long" + "_name"
            "wrapped \
line" -> "long_name"
        }"""
    )
    assert 'say "hi"' in graph.nodes
    assert "long_name" in graph.nodes
    assert "wrapped line" in graph.nodes


def test_html_labels_and_all_three_comment_styles():
    graph = parse_dot(
        """
        # a preprocessor line
        digraph {
            // a line comment
            /* a block comment */
            a [label = <<b>bold</b>>]
            a -> b
        }
        """
    )
    assert graph.nodes["a"].attrs["label"] == "<<b>bold</b>>"
    assert edge_keys(graph) == [("a", "b")]


def test_strict_collapses_parallel_edges_and_says_so():
    graph = parse_dot("strict digraph { a -> b; a -> b; b -> c }")
    assert edge_keys(graph) == [("a", "b"), ("b", "c")]
    assert any("collapsed" in warning for warning in graph.warnings)


def test_non_strict_keeps_parallel_edges():
    graph = parse_dot("digraph { a -> b; a -> b }")
    assert graph.edge_count == 2
    assert graph.duplicate_edges() == [("a", "b")]


def test_graph_attributes_are_kept():
    graph = parse_dot('digraph { rankdir = LR; graph [bgcolor = "white"]; a }')
    assert graph.attrs["rankdir"] == "LR"
    assert graph.attrs["bgcolor"] == "white"


def test_undirected_graph_is_read_but_flagged():
    graph = parse_dot("graph { a -- b }")
    assert graph.directed is False
    assert edge_keys(graph) == [("a", "b")]
    assert any("undirected" in warning for warning in graph.warnings)


def test_wrong_edge_operator_points_at_the_line():
    with pytest.raises(ParseError) as exc:
        parse_dot("digraph {\n  a -- b\n}", source="bad.dot")
    assert exc.value.line == 2
    assert "only valid in a graph" in exc.value.message
    assert str(exc.value).startswith("bad.dot:2:")


@pytest.mark.parametrize(
    ("text", "fragment"),
    [
        ("digraph { a -> b", "expected '}'"),
        ("digraph { a [ shape = box }", "expected ']'"),
        ("subgraph { a }", "expected 'graph' or 'digraph'"),
        ('digraph { "unterminated }', "unterminated quoted string"),
        ("digraph { /* open", "unterminated block comment"),
        ("digraph { a } trailing", "after the closing"),
    ],
)
def test_syntax_errors_are_reported_with_a_location(text, fragment):
    with pytest.raises(ParseError) as exc:
        parse_dot(text, source="bad.dot")
    assert fragment in exc.value.message
    assert exc.value.line is not None
