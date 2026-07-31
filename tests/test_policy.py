import json
from pathlib import Path

import pytest

from dagreach.analysis import analyse, impact
from dagreach.cli import EXIT_INPUT_ERROR, EXIT_OK, EXIT_POLICY_FAILED, EXIT_USAGE, main
from dagreach.policy import any_failed, fail_if_reaches, fail_on_cycle, max_impacted
from dagreach.readers.dot import parse_dot
from dagreach.selectors import SelectorError, parse_selector

FIXTURES = Path(__file__).parent / "fixtures"

SERVICES = """
digraph {
    auth [group = "core"]
    token [group = "core"]
    payments [group = "production", status = "critical"]
    reporting [group = "analytics"]
    auth -> token -> payments
    reporting -> payments
}
"""


@pytest.mark.parametrize(
    ("text", "key", "value"),
    [("group=production", "group", "production"), ("node=auth", "node", "auth")],
)
def test_selectors_parse(text, key, value):
    selector = parse_selector(text)
    assert (selector.key, selector.value) == (key, value)
    assert str(selector) == text


@pytest.mark.parametrize("text", ["production", "team=", "=production", "owner=me"])
def test_bad_selectors_say_what_was_expected(text):
    with pytest.raises(SelectorError) as exc:
        parse_selector(text)
    assert "group" in str(exc.value)


def test_selector_matches_group_status_and_node():
    graph = parse_dot(SERVICES)
    assert parse_selector("group=core").select(graph) == ["auth", "token"]
    assert parse_selector("status=critical").select(graph) == ["payments"]
    assert parse_selector("node=reporting").select(graph) == ["reporting"]


def test_fail_if_reaches_carries_the_path_that_proves_it():
    graph = parse_dot(SERVICES)
    report = impact(graph, ["auth"])
    result = fail_if_reaches(graph, report, parse_selector("group=production"))
    assert result.failed is True
    assert result.matched == ["payments"]
    assert result.witnesses["payments"] == ["auth", "token", "payments"]


def test_fail_if_reaches_passes_when_nothing_matches():
    graph = parse_dot(SERVICES)
    report = impact(graph, ["reporting"])
    result = fail_if_reaches(graph, report, parse_selector("group=core"))
    assert result.failed is False
    assert "nothing matching group=core is reached" in result.detail


def test_a_changed_node_that_is_itself_a_target_is_flagged_as_such():
    graph = parse_dot(SERVICES)
    report = impact(graph, ["payments"])
    result = fail_if_reaches(graph, report, parse_selector("group=production"))
    assert result.failed is True
    assert "they are the changed nodes themselves" in result.detail


def test_max_impacted_compares_against_the_ceiling():
    graph = parse_dot(SERVICES)
    report = impact(graph, ["auth"])
    assert max_impacted(report, 2).failed is True
    assert max_impacted(report, 3).failed is False


def test_fail_on_cycle_lists_the_cycles():
    stats = analyse(parse_dot("digraph { a -> b -> a }"))
    result = fail_on_cycle(stats.cycles)
    assert result.failed is True
    assert result.matched == ["a, b"]
    assert fail_on_cycle([]).failed is False


def test_any_failed_is_the_gate():
    graph = parse_dot(SERVICES)
    report = impact(graph, ["reporting"])
    passing = fail_if_reaches(graph, report, parse_selector("group=core"))
    failing = fail_if_reaches(graph, report, parse_selector("group=production"))
    assert any_failed([passing]) is False
    assert any_failed([passing, failing]) is True


# -- the command line ------------------------------------------------------


def write(tmp_path, text=SERVICES):
    path = tmp_path / "services.dot"
    path.write_text(text, encoding="utf-8")
    return str(path)


def test_a_violated_policy_exits_one_and_explains(tmp_path, capsys):
    code = main(
        ["impact", write(tmp_path), "--changed", "auth", "--fail-if-reaches", "group=production"]
    )
    assert code == EXIT_POLICY_FAILED
    out = capsys.readouterr().out
    assert "FAIL fail-if-reaches group=production" in out
    assert "payments: auth -> token -> payments" in out


def test_a_satisfied_policy_exits_zero(tmp_path, capsys):
    code = main(
        [
            "impact",
            write(tmp_path),
            "--changed",
            "reporting",
            "--fail-if-reaches",
            "group=core",
        ]
    )
    assert code == EXIT_OK
    assert "ok   fail-if-reaches group=core" in capsys.readouterr().out


def test_policies_are_reported_in_json_with_their_witnesses(tmp_path, capsys):
    code = main(
        [
            "impact",
            write(tmp_path),
            "--changed",
            "auth",
            "--fail-if-reaches",
            "group=production",
            "--max-impacted",
            "2",
            "--json",
        ]
    )
    assert code == EXIT_POLICY_FAILED
    report = json.loads(capsys.readouterr().out)
    policies = {entry["policy"]: entry for entry in report["policies"]}
    assert policies["fail-if-reaches"]["failed"] is True
    assert policies["fail-if-reaches"]["witnesses"]["payments"] == ["auth", "token", "payments"]
    assert policies["max-impacted"]["failed"] is True


def test_max_impacted_alone_can_pass(tmp_path, capsys):
    assert main(["impact", write(tmp_path), "--changed", "auth", "--max-impacted", "10"]) == EXIT_OK
    assert "ok   max-impacted 10" in capsys.readouterr().out


def test_stats_fail_on_cycle(tmp_path, capsys):
    cyclic = write(tmp_path, "digraph { a -> b -> a }")
    assert main(["stats", cyclic, "--fail-on", "cycle"]) == EXIT_POLICY_FAILED
    assert "FAIL fail-on-cycle" in capsys.readouterr().out
    assert main(["stats", str(FIXTURES / "terraform.dot"), "--fail-on", "cycle"]) == EXIT_OK


def test_a_bad_selector_is_a_usage_error_not_a_policy_failure(tmp_path, capsys):
    code = main(["impact", write(tmp_path), "--changed", "auth", "--fail-if-reaches", "team=core"])
    assert code == EXIT_USAGE
    assert "unknown selector key" in capsys.readouterr().err


def test_an_unknown_changed_node_still_beats_the_policies(tmp_path, capsys):
    code = main(
        ["impact", write(tmp_path), "--changed", "nope", "--fail-if-reaches", "group=production"]
    )
    assert code == EXIT_INPUT_ERROR
