from pathlib import Path

import pytest

from dagreach.analysis import impact
from dagreach.cli import EXIT_OK, main
from dagreach.readers import read
from dagreach.readers.dot import parse_dot
from dagreach.semantics import looks_like_dependency_export, orient, warn_if_orientation_is_suspect

FIXTURES = Path(__file__).parent / "fixtures"
VPC = "[root] aws_vpc.main (expand)"
INSTANCE = "[root] aws_instance.web (expand)"


def test_feeds_is_the_internal_orientation_and_changes_nothing():
    graph = orient(parse_dot("digraph { a -> b }"), "feeds")
    assert [edge.key for edge in graph.edges] == [("a", "b")]
    assert graph.edge_semantics == "feeds"


def test_depends_on_is_reversed_once_at_the_door():
    graph = orient(parse_dot("digraph { a -> b }"), "depends-on")
    assert [edge.key for edge in graph.edges] == [("b", "a")]
    assert graph.edge_semantics == "depends-on"


def test_unknown_semantics_is_refused():
    with pytest.raises(ValueError):
        orient(parse_dot("digraph { a }"), "sideways")


def test_terraform_output_is_recognised():
    graph = read(FIXTURES / "terraform.dot")
    assert looks_like_dependency_export(graph) == "terraform graph"
    assert looks_like_dependency_export(parse_dot("digraph { a -> b }")) is None


def test_a_dependency_export_read_as_feeds_raises_a_warning():
    graph = orient(read(FIXTURES / "terraform.dot"), "feeds")
    warn_if_orientation_is_suspect(graph, declared=None)
    assert any("terraform graph" in warning for warning in graph.warnings)
    assert any("--edge-semantics depends-on" in warning for warning in graph.warnings)


def test_declaring_the_semantics_silences_the_warning():
    graph = orient(read(FIXTURES / "terraform.dot"), "depends-on")
    warn_if_orientation_is_suspect(graph, declared="depends-on")
    assert not any("terraform" in warning for warning in graph.warnings)


def test_terraform_impact_runs_the_right_way_round():
    """Changing the VPC must reach what depends on it, not what it depends on."""
    graph = orient(read(FIXTURES / "terraform.dot"), "depends-on")
    report = impact(graph, [VPC])
    assert INSTANCE in report.downstream
    assert "[root] aws_subnet.main (expand)" in report.downstream
    assert "[root] aws_security_group.web (expand)" in report.downstream


def test_reading_that_same_file_as_feeds_answers_the_opposite_question():
    graph = orient(read(FIXTURES / "terraform.dot"), "feeds")
    report = impact(graph, [VPC])
    assert INSTANCE not in report.downstream
    assert INSTANCE in report.upstream


def test_the_applied_orientation_is_always_stated(capsys):
    assert main(["impact", str(FIXTURES / "terraform.dot"), "--changed", VPC]) == EXIT_OK
    out = capsys.readouterr().out
    assert "edges: source feeds target, so impact follows edges forward" in out

    assert (
        main(
            [
                "impact",
                str(FIXTURES / "terraform.dot"),
                "--changed",
                VPC,
                "--edge-semantics",
                "depends-on",
            ]
        )
        == EXIT_OK
    )
    out = capsys.readouterr().out
    assert "edges: source depends on target, so impact follows edges backward" in out
