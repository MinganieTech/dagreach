import pytest

from dagreach.model import Edge, Graph, Node
from dagreach.profile import duration_of, group_of, status_of, summarize


@pytest.mark.parametrize(
    ("attrs", "expected"),
    [
        ({"duration": "12"}, 12.0),
        ({"duration": "0.5"}, 0.5),
        ({"weight": "7"}, 7.0),
        ({"duration": "3", "weight": "9"}, 3.0),  # duration wins
        ({"duration": "later", "weight": "9"}, 9.0),  # unreadable duration falls back
        ({}, None),
        ({"duration": "soon"}, None),
        ({"duration": "nan"}, None),
        ({"duration": "inf"}, None),
    ],
)
def test_duration_precedence_and_tolerance(attrs, expected):
    assert duration_of(Node("n", attrs)) == expected


def test_status_and_group_are_trimmed_and_optional():
    assert status_of(Node("n", {"status": "  ready "})) == "ready"
    assert status_of(Node("n", {"status": "   "})) is None
    assert group_of(Node("n", {})) is None


def test_summary_counts_and_reports_unreadable_values():
    graph = Graph()
    graph.nodes["a"] = Node("a", {"duration": "10", "status": "ready", "group": "core"})
    graph.nodes["b"] = Node("b", {"duration": "soon", "status": "ready"})
    graph.edges.append(Edge("a", "b", {"weight": "2"}))

    summary = summarize(graph)
    assert summary.nodes_with_duration == 1
    assert summary.edges_with_duration == 1
    assert summary.statuses == {"ready": 2}
    assert summary.groups == {"core": 1}
    assert summary.uses_durations is True
    assert len(summary.unreadable) == 1
    assert "duration='soon'" in summary.unreadable[0]


def test_a_graph_without_profile_attributes_is_not_an_error():
    graph = Graph()
    graph.add_edge("a", "b")
    summary = summarize(graph)
    assert summary.uses_durations is False
    assert summary.unreadable == []
