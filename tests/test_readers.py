from pathlib import Path

import pytest

from dagreach.errors import DagreachError, UnknownFormatError
from dagreach.profile import duration_of, group_of, status_of, summarize
from dagreach.readers import detect_format, read, read_text

FIXTURES = Path(__file__).parent / "fixtures"


@pytest.mark.parametrize(
    ("text", "hint", "expected"),
    [
        ("digraph { a }", None, "dot"),
        ("  strict graph g { }", None, "dot"),
        ("// comment\n/* block */\ndigraph { }", None, "dot"),
        ('{"graph": {}}', None, "jgf"),
        ("digraph { a }", "plan.dot", "dot"),
        ('{"graph": {}}', "plan.json", "jgf"),
        ("anything at all", "plan.gv", "dot"),
    ],
)
def test_format_detection(text, hint, expected):
    assert detect_format(text, hint) == expected


def test_undetectable_input_asks_for_the_flag():
    with pytest.raises(UnknownFormatError) as exc:
        detect_format("just some prose")
    assert "--format" in str(exc.value)


def test_explicit_format_overrides_detection():
    graph = read_text('{"graph": {"nodes": [{"id": "a"}]}}', format="jgf", hint="misnamed.dot")
    assert graph.format == "jgf"
    assert sorted(graph.nodes) == ["a"]


def test_missing_file_is_a_clean_error():
    with pytest.raises(DagreachError) as exc:
        read(FIXTURES / "does-not-exist.dot")
    assert "no such file" in str(exc.value)


def test_reads_stdin(monkeypatch, capsys):
    monkeypatch.setattr("sys.stdin", __import__("io").StringIO("digraph { a -> b }"))
    graph = read("-")
    assert graph.source == "<stdin>"
    assert graph.edge_count == 1


def test_terraform_fixture():
    graph = read(FIXTURES / "terraform.dot")
    assert graph.format == "dot"
    assert graph.node_count == 5
    assert graph.edge_count == 5
    assert '[root] provider["registry.terraform.io/hashicorp/aws"]' in graph.nodes
    assert graph.nodes["[root] aws_instance.web (expand)"].attrs["label"] == "aws_instance.web"
    assert graph.attrs["compound"] == "true"


def test_bazel_factored_nodes_keep_their_literal_separator():
    graph = read(FIXTURES / "bazel.dot")
    assert graph.node_count == 4
    assert graph.edge_count == 4
    assert "//src:resources\\n//src:assets" in graph.nodes
    assert graph.nodes["//src:app"].attrs["shape"] == "box"


def test_messy_fixture_survives_every_corner():
    graph = read(FIXTURES / "messy.dot")
    assert sorted(graph.nodes) == [
        "42",
        "build",
        'deploy "prod"',
        "long_name",
        "monitor",
        "smoke_a",
        "smoke_b",
        "test",
    ]
    keys = [edge.key for edge in graph.edges]
    assert ("build", "test") in keys
    assert keys.count(("build", "test")) == 1  # strict collapsed the duplicate
    assert ("monitor", "smoke_a") in keys and ("monitor", "smoke_b") in keys
    assert len(graph.self_loops()) == 1
    assert graph.nodes["long_name"].attrs["label"] == "<<b>concatenated</b>>"
    assert group_of(graph.nodes["build"]) == "core"
    # An explicit duration wins over weight, and the edge default applies elsewhere.
    long_name_edge = next(edge for edge in graph.edges if edge.source == "long_name")
    assert duration_of(long_name_edge) == 0.5
    build_test_edge = next(edge for edge in graph.edges if edge.key == ("build", "test"))
    assert duration_of(build_test_edge) == 2.5


def test_pipeline_jgf_fixture():
    graph = read(FIXTURES / "pipeline.jgf.json")
    assert graph.format == "jgf"
    assert graph.name == "etl_daily"
    assert graph.node_count == 5
    assert graph.edge_count == 4
    assert graph.attrs["schedule"] == "0 2 * * *"

    summary = summarize(graph)
    assert summary.nodes_with_duration == 4
    assert summary.statuses == {"success": 2, "running": 1, "pending": 2}
    assert set(summary.groups) == {"extract", "transform", "load"}
    assert status_of(graph.nodes["transform_orders"]) == "running"


def test_keyed_nodes_fixture():
    graph = read(FIXTURES / "keyed_nodes.jgf.json")
    assert graph.name == "services"
    assert graph.node_count == 3
    assert graph.nodes["gateway"].attrs["label"] == "API gateway"
    assert duration_of(graph.nodes["auth"]) == 30.0
