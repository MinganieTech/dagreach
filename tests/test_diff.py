import json

import pytest

from dagreach.cli import EXIT_INPUT_ERROR, EXIT_OK, EXIT_POLICY_FAILED, EXIT_USAGE, main
from dagreach.diff import all_pairs_delta, newly_exposed, reach_diff
from dagreach.readers.dot import parse_dot
from dagreach.selectors import parse_selector

BEFORE = """
digraph {
    auth [group = "core"]
    token [group = "core"]
    payments [group = "production"]
    ledger [group = "production"]
    auth -> token
    ledger -> payments
}
"""

AFTER = """
digraph {
    auth [group = "core"]
    token [group = "core"]
    payments [group = "production"]
    ledger [group = "production"]
    auth -> token
    token -> payments
    ledger -> payments
}
"""


def test_reach_delta_is_the_difference_between_two_traversals():
    diff = reach_diff(parse_dot(BEFORE), parse_dot(AFTER), ["auth"])
    assert diff.reached_before == ["auth", "token"]
    assert diff.reached_after == ["auth", "token", "payments"]
    assert diff.added == ["payments"]
    assert diff.removed == []
    assert diff.unchanged is False


def test_lost_reach_is_reported_too():
    diff = reach_diff(parse_dot(AFTER), parse_dot(BEFORE), ["auth"])
    assert diff.added == []
    assert diff.removed == ["payments"]


def test_structural_change_is_reported_alongside():
    diff = reach_diff(parse_dot(BEFORE), parse_dot(AFTER), ["auth"])
    assert diff.edges_added == [("token", "payments")]
    assert diff.edges_removed == []
    assert diff.nodes_added == []


def test_a_seed_that_exists_in_only_one_version_is_flagged():
    diff = reach_diff(parse_dot("digraph { a }"), parse_dot("digraph { a -> b }"), ["b"])
    assert diff.seeds_missing_before == ["b"]
    assert diff.seeds_missing_after == []


def test_a_new_edge_is_named_as_the_reason():
    exposures = newly_exposed(
        parse_dot(BEFORE),
        parse_dot(AFTER),
        reach_diff(parse_dot(BEFORE), parse_dot(AFTER), ["auth"]),
        parse_selector("group=production"),
    )
    assert [exposure.target for exposure in exposures] == ["payments"]
    assert exposures[0].reason == "new-edge"
    assert exposures[0].detail == "new edge token -> payments"
    assert exposures[0].path == ["auth", "token", "payments"]


def test_a_new_node_is_named_as_such():
    before = parse_dot("digraph { auth }")
    after = parse_dot('digraph { auth -> secrets [group = "x"]; secrets [group = "production"] }')
    diff = reach_diff(before, after, ["auth"])
    exposures = newly_exposed(before, after, diff, parse_selector("group=production"))
    assert exposures[0].reason == "new-node"
    assert "did not exist before" in exposures[0].detail


def test_a_reclassified_target_is_caught_without_any_new_edge():
    before = parse_dot('digraph { auth -> store [group = "staging"]; store [group = "staging"] }')
    after = parse_dot('digraph { auth -> store; store [group = "production"] }')
    diff = reach_diff(before, after, ["auth"])
    assert diff.added == []  # nothing new is reachable
    exposures = newly_exposed(before, after, diff, parse_selector("group=production"))
    assert exposures[0].reason == "reclassified"
    assert "'staging' to 'production'" in exposures[0].detail


def test_a_route_opened_upstream_is_still_attributed_to_the_edge_that_opened_it():
    before = parse_dot('digraph { auth; mid -> target [group = "production"] }')
    after = parse_dot('digraph { auth -> mid -> target; target [group = "production"] }')
    diff = reach_diff(before, after, ["auth"])
    exposures = newly_exposed(before, after, diff, parse_selector("group=production"))
    assert exposures[0].target == "target"
    # The route opens because auth -> mid appeared; the rest of the path is old.
    assert exposures[0].reason == "new-edge"
    assert exposures[0].detail == "new edge auth -> mid"


def test_nothing_exposed_when_the_target_was_already_reached():
    diff = reach_diff(parse_dot(AFTER), parse_dot(AFTER), ["auth"])
    assert (
        newly_exposed(parse_dot(AFTER), parse_dot(AFTER), diff, parse_selector("group=production"))
        == []
    )


# -- the opt-in global analysis --------------------------------------------


def test_all_pairs_delta_counts_ordered_pairs():
    delta = all_pairs_delta(
        parse_dot("digraph { a -> b; c }"), parse_dot("digraph { a -> b -> c }")
    )
    assert delta.added_total == 2  # a->c and b->c
    assert delta.removed_total == 0
    assert delta.added_by_source == {"a": 1, "b": 1}


def test_all_pairs_delta_reports_losses():
    delta = all_pairs_delta(
        parse_dot("digraph { a -> b -> c }"), parse_dot("digraph { a -> b; c }")
    )
    assert delta.added_total == 0
    assert delta.removed_total == 2
    assert delta.removed_by_source == {"a": 1, "b": 1}


def test_all_pairs_delta_handles_cycles():
    delta = all_pairs_delta(parse_dot("digraph { a; b }"), parse_dot("digraph { a -> b -> a }"))
    # inside a cycle everything reaches everything, itself included
    assert delta.added_total == 4


# -- the command line ------------------------------------------------------


def write(tmp_path, name, text):
    path = tmp_path / name
    path.write_text(text, encoding="utf-8")
    return str(path)


@pytest.fixture
def graphs(tmp_path):
    return write(tmp_path, "before.dot", BEFORE), write(tmp_path, "after.dot", AFTER)


def test_diff_reports_the_delta(graphs, capsys):
    before, after = graphs
    assert main(["diff", before, after, "--changed", "auth"]) == EXIT_OK
    out = capsys.readouterr().out
    assert "reaches 3 nodes, was 2 (+1, -0)" in out
    assert "new reach (1): payments" in out
    assert "1 edge(s) added, 0 removed" in out


def test_fail_on_new_reach_exits_one_and_names_the_edge(graphs, capsys):
    before, after = graphs
    code = main(
        [
            "diff",
            before,
            after,
            "--changed",
            "auth",
            "--explain",
            "--fail-on-new-reach",
            "group=production",
        ]
    )
    assert code == EXIT_POLICY_FAILED
    out = capsys.readouterr().out
    assert "payments is now reached" in out
    assert "reason: new edge token -> payments" in out
    assert "path:   auth -> token -> payments" in out
    assert "FAIL fail-on-new-reach group=production" in out


def test_fail_on_new_reach_passes_when_nothing_opened(graphs, capsys):
    before, _ = graphs
    assert (
        main(
            ["diff", before, before, "--changed", "auth", "--fail-on-new-reach", "group=production"]
        )
        == EXIT_OK
    )
    assert "ok   fail-on-new-reach group=production" in capsys.readouterr().out


def test_diff_json_carries_exposures_and_policies(graphs, capsys):
    before, after = graphs
    code = main(
        [
            "diff",
            before,
            after,
            "--changed",
            "auth",
            "--fail-on-new-reach",
            "group=production",
            "--json",
        ]
    )
    assert code == EXIT_POLICY_FAILED
    report = json.loads(capsys.readouterr().out)
    assert report["schema_version"] == 1
    assert report["added_reach"] == ["payments"]
    assert report["edges_added"] == [["token", "payments"]]
    assert report["exposures"][0]["reason"] == "new-edge"
    assert report["policies"][0]["failed"] is True
    assert "all_pairs_reachability_delta" not in report


def test_diff_needs_changed_or_the_global_flag(graphs, capsys):
    before, after = graphs
    assert main(["diff", before, after]) == EXIT_USAGE
    assert "needs --changed" in capsys.readouterr().err


def test_the_global_analysis_is_opt_in_and_aggregated(graphs, capsys):
    before, after = graphs
    assert main(["diff", before, after, "--all-pairs-reachability-delta"]) == EXIT_OK
    out = capsys.readouterr().out
    assert "all-pairs reachability delta: +2 pair(s), -0 pair(s) over 4 source(s)" in out
    assert "by source, largest first:" in out


def test_count_only_drops_the_ranking(graphs, capsys):
    before, after = graphs
    assert (
        main(["diff", before, after, "--all-pairs-reachability-delta", "--count-only"]) == EXIT_OK
    )
    out = capsys.readouterr().out
    assert "all-pairs reachability delta:" in out
    assert "by source" not in out


def test_an_unknown_changed_node_is_an_input_error(graphs, capsys):
    before, after = graphs
    assert main(["diff", before, after, "--changed", "nope"]) == EXIT_INPUT_ERROR
    assert "no node 'nope' in either graph" in capsys.readouterr().err


def test_diff_output_stays_ascii(graphs, capsys):
    before, after = graphs
    assert main(["diff", before, after, "--changed", "auth", "--explain"]) == EXIT_OK
    captured = capsys.readouterr()
    assert captured.out.isascii(), captured.out
