import pytest

from dagreach.analysis import (
    analyse,
    articulation_points,
    critical_path,
    impact,
    reachable,
    strongly_connected_cycles,
    topological_levels,
)
from dagreach.readers.dot import parse_dot


def graph_of(text: str):
    return parse_dot(text)


DIAMOND = "digraph { a -> b; a -> c; b -> d; c -> d }"
CHAIN = "digraph { a -> b -> c }"


def test_reachable_down_and_up_exclude_the_seeds():
    graph = graph_of(DIAMOND)
    assert reachable(graph, ["a"], "down") == ["b", "c", "d"]
    assert reachable(graph, ["d"], "up") == ["a", "b", "c"]
    assert reachable(graph, ["b"], "down") == ["d"]
    assert reachable(graph, ["a"], "up") == []


def test_reachable_follows_declaration_order():
    graph = graph_of("digraph { z -> m; m -> a; z -> a }")
    assert reachable(graph, ["z"], "down") == ["m", "a"]


def test_levels_group_nodes_by_earliest_position():
    assert topological_levels(graph_of(DIAMOND)) == [["a"], ["b", "c"], ["d"]]
    assert topological_levels(graph_of(CHAIN)) == [["a"], ["b"], ["c"]]


def test_levels_are_none_when_a_cycle_remains():
    assert topological_levels(graph_of("digraph { a -> b -> c -> a }")) is None


@pytest.mark.parametrize(
    ("text", "expected"),
    [
        ("digraph { a -> b -> c -> a }", [["a", "b", "c"]]),
        ("digraph { a -> a }", [["a"]]),
        ("digraph { a -> b; b -> a; c -> d }", [["a", "b"]]),
        (DIAMOND, []),
    ],
)
def test_cycle_detection(text, expected):
    assert strongly_connected_cycles(graph_of(text)) == expected


def test_articulation_points_find_the_single_points_of_passage():
    assert articulation_points(graph_of(CHAIN)) == ["b"]
    # A diamond has two independent paths, so nothing is indispensable.
    assert articulation_points(graph_of(DIAMOND)) == []
    # A bow tie: the centre holds both halves together.
    bow_tie = "digraph { a -> centre; b -> centre; centre -> c; centre -> d; a -> b; c -> d }"
    assert articulation_points(graph_of(bow_tie)) == ["centre"]


def test_a_self_loop_never_makes_a_node_an_articulation_point():
    assert articulation_points(graph_of("digraph { a -> a }")) == []


def test_unweighted_critical_path_counts_edges():
    path = critical_path(graph_of("digraph { a -> b -> c -> d; a -> d }"))
    assert path is not None
    assert path.weighted is False
    assert path.nodes == ["a", "b", "c", "d"]
    assert path.edges == 3
    assert path.label == "longest path"
    assert "structural (no durations declared)" in path.describe()


def test_weighted_critical_path_follows_the_slowest_branch():
    graph = graph_of(
        """
        digraph {
            a [duration = "1"]
            fast [duration = "1"]
            slow [duration = "10"]
            z [duration = "1"]
            a -> fast -> z
            a -> slow -> z
        }
        """
    )
    path = critical_path(graph)
    assert path is not None
    assert path.weighted is True
    assert path.nodes == ["a", "slow", "z"]
    assert path.cost == 12.0
    assert "12 of duration over 2 edge(s)" in path.describe()


def test_edge_durations_count_towards_the_path():
    graph = graph_of('digraph { a -> b [duration = "5"]; a -> c [duration = "1"] }')
    path = critical_path(graph)
    assert path is not None
    assert path.nodes == ["a", "b"]
    assert path.cost == 5.0


def test_critical_path_can_be_restricted_to_a_subset():
    graph = graph_of("digraph { a -> b -> c -> d }")
    path = critical_path(graph, within={"b", "c"})
    assert path is not None
    assert path.nodes == ["b", "c"]


def test_critical_path_is_none_when_a_cycle_remains():
    assert critical_path(graph_of("digraph { a -> b -> a }")) is None


def test_analyse_reports_the_shape_of_a_graph():
    stats = analyse(graph_of(DIAMOND))
    assert (stats.nodes, stats.edges) == (4, 4)
    assert stats.acyclic is True
    assert stats.depth == 3
    assert stats.width == 2
    assert stats.widest_level == ["b", "c"]
    assert stats.roots == ["a"]
    assert stats.leaves == ["d"]
    assert stats.isolated == []


def test_analyse_condenses_cycles_rather_than_giving_up():
    stats = analyse(graph_of("digraph { a -> b -> c -> a; c -> d }"))
    assert stats.acyclic is False
    assert stats.cycles == [["a", "b", "c"]]
    assert stats.condensed is True
    assert stats.collapsed_cycles == {"scc(a+2)": ["a", "b", "c"]}
    # The metrics that need acyclicity are measured on the condensation.
    assert stats.depth == 2
    assert stats.width == 1
    assert stats.critical_path is not None
    assert stats.critical_path.nodes == ["scc(a+2)", "d"]


def test_analyse_counts_groups_and_isolated_nodes():
    stats = analyse(
        graph_of('digraph { a [group = "core"]; b [group = "core"]; c [group = "edge"]; a -> b }')
    )
    assert stats.groups == {"core": 2, "edge": 1}
    assert stats.isolated == ["c"]


def test_impact_reports_what_a_change_reaches():
    graph = graph_of(
        """
        digraph {
            auth [group = "core", duration = "10"]
            billing [group = "core", duration = "5"]
            web [group = "edge", duration = "2"]
            unrelated [group = "edge", duration = "100"]
            db -> auth
            auth -> billing -> web
        }
        """
    )
    report = impact(graph, ["auth"])
    assert report.seeds == ["auth"]
    assert report.downstream == ["billing", "web"]
    assert report.upstream == ["db"]
    assert report.impacted == ["auth", "billing", "web"]
    assert report.impacted_leaves == ["web"]
    assert report.total_nodes == 5
    assert round(report.share, 2) == 0.6
    assert report.cost == 17.0
    assert report.groups == {"core": 2, "edge": 1}
    assert report.critical_path is not None
    assert report.critical_path.nodes == ["auth", "billing", "web"]


def test_impact_flags_a_seed_that_is_an_articulation_point():
    report = impact(graph_of(CHAIN), ["b"])
    assert report.impacted_articulation_points == ["b"]


def test_impact_of_several_seeds_is_their_union_in_declaration_order():
    graph = graph_of("digraph { a -> x; b -> y; c }")
    report = impact(graph, ["b", "a"])
    assert report.seeds == ["a", "b"]
    assert report.downstream == ["x", "y"]


def test_deep_graphs_do_not_overflow_the_stack():
    edges = " ".join(f"n{index} -> n{index + 1};" for index in range(10_000))
    graph = graph_of("digraph { " + edges + " }")
    stats = analyse(graph)
    assert stats.nodes == 10_001
    assert stats.depth == 10_001
    assert stats.width == 1
    assert stats.critical_path is not None
    assert stats.critical_path.edges == 10_000
    assert len(stats.articulation_points) == 9_999  # every node but the two ends


def test_results_are_stable_across_runs():
    graph = graph_of(DIAMOND)
    first, second = analyse(graph), analyse(graph)
    assert first == second


def test_witnesses_give_a_shortest_reason_for_every_reached_node():
    from dagreach.analysis import witnesses

    graph = graph_of("digraph { a -> b -> c -> d; a -> d }")
    index = witnesses(graph, ["a"], "down")
    assert index.distance["d"] == 1
    assert index.path("d") == ["a", "d"]
    assert index.path("c") == ["a", "b", "c"]
    assert index.path("a") == ["a"]
    assert index.path("missing") == []


def test_witnesses_from_several_seeds_pick_the_nearest_one():
    from dagreach.analysis import witnesses

    graph = graph_of("digraph { far -> mid -> target; near -> target }")
    index = witnesses(graph, ["far", "near"], "down")
    assert index.path("target") == ["near", "target"]
    assert index.distance["target"] == 1
