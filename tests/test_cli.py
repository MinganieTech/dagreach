import json
import subprocess
import sys
from pathlib import Path

import pytest

from dagreach import __version__
from dagreach.cli import EXIT_INPUT_ERROR, EXIT_OK, main

FIXTURES = Path(__file__).parent / "fixtures"


def test_no_command_prints_help(capsys):
    assert main([]) == EXIT_OK
    assert "dagreach" in capsys.readouterr().out


def test_version_flag_exits_zero_and_prints_version(capsys):
    with pytest.raises(SystemExit) as exc:
        main(["--version"])
    assert exc.value.code == EXIT_OK
    assert __version__ in capsys.readouterr().out


def test_module_entry_point_runs():
    result = subprocess.run(
        [sys.executable, "-m", "dagreach", "--version"],
        capture_output=True,
        text=True,
        check=False,
    )
    assert result.returncode == EXIT_OK
    assert __version__ in result.stdout


def test_parse_reports_size_and_profile(capsys):
    assert main(["parse", str(FIXTURES / "pipeline.jgf.json")]) == EXIT_OK
    out = capsys.readouterr().out
    assert "jgf" in out and "5 nodes, 4 edges" in out
    assert "durations on 4/5 nodes" in out


def test_parse_json_report_is_machine_readable(capsys):
    assert main(["parse", str(FIXTURES / "messy.dot"), "--json"]) == EXIT_OK
    report = json.loads(capsys.readouterr().out)
    assert report["format"] == "dot"
    assert report["nodes"] == 8
    assert report["self_loops"] == 1
    assert report["profile"]["statuses"] == {"ready": 2, "blocked": 1}
    assert any("collapsed" in warning for warning in report["warnings"])


def test_parse_surfaces_warnings_in_text_mode(capsys):
    assert main(["parse", str(FIXTURES / "messy.dot")]) == EXIT_OK
    out = capsys.readouterr().out
    assert "warnings (" in out
    assert "self-loop" in out


def test_parse_reports_a_bad_file_without_a_traceback(capsys):
    assert main(["parse", str(FIXTURES / "missing.dot")]) == EXIT_INPUT_ERROR
    captured = capsys.readouterr()
    assert captured.out == ""
    assert "no such file" in captured.err


def test_parse_reports_a_syntax_error_with_its_location(tmp_path, capsys):
    broken = tmp_path / "broken.dot"
    broken.write_text("digraph {\n  a -> \n}\n", encoding="utf-8")
    assert main(["parse", str(broken)]) == EXIT_INPUT_ERROR
    assert "broken.dot:3" in capsys.readouterr().err


def test_parse_accepts_an_explicit_format(tmp_path, capsys):
    misnamed = tmp_path / "graph.dot"
    misnamed.write_text('{"graph": {"nodes": [{"id": "a"}]}}', encoding="utf-8")
    assert main(["parse", str(misnamed), "--format", "jgf"]) == EXIT_OK
    assert "1 nodes" in capsys.readouterr().out


def test_stats_reports_the_shape(capsys):
    assert main(["stats", str(FIXTURES / "terraform.dot")]) == EXIT_OK
    out = capsys.readouterr().out
    assert "5 nodes, 5 edges, acyclic" in out
    assert "depth 4 level(s), width 2 (largest earliest-start generation)" in out
    assert "longest path:" in out  # no durations here, so it is structural
    assert "critical path" not in out


def test_stats_json_carries_every_metric(capsys):
    assert main(["stats", str(FIXTURES / "pipeline.jgf.json"), "--json"]) == EXIT_OK
    report = json.loads(capsys.readouterr().out)
    assert report["acyclic"] is True
    assert report["roots"] == ["extract_orders", "extract_customers"]
    assert report["leaves"] == ["notify"]
    assert report["schema_version"] == 1
    assert report["edge_semantics"] == "feeds"
    assert report["longest_path"]["weighted"] is True
    assert report["longest_path"]["measure"] == "duration"
    assert report["longest_path"]["nodes"][0].startswith("extract_")
    assert report["articulation_points"] == ["transform_orders", "load_warehouse"]


def test_stats_on_a_cyclic_graph_collapses_and_says_so(tmp_path, capsys):
    cyclic = tmp_path / "cyclic.dot"
    cyclic.write_text("digraph { a -> b -> c -> a; c -> d }", encoding="utf-8")
    assert main(["stats", str(cyclic)]) == EXIT_OK
    out = capsys.readouterr().out
    assert "1 cycle(s)" in out
    assert "cycles are collapsed before measuring" in out
    assert "cycle: a, b, c" in out
    assert "measured on the condensed graph" in out
    assert "scc(a+2)" in out


def test_impact_reports_reach_cost_and_groups(capsys):
    assert (
        main(["impact", str(FIXTURES / "pipeline.jgf.json"), "--changed", "transform_orders"])
        == EXIT_OK
    )
    out = capsys.readouterr().out
    assert "reaches 3 of 5 nodes (60%)" in out
    assert "downstream (2): load_warehouse, notify" in out
    assert "upstream (2)" in out
    assert "cost of the impacted set: 480 of declared duration" in out
    assert "groups touched: transform 1, load 2" in out
    assert "articulation point" in out


def test_impact_accepts_repeated_and_comma_separated_ids(capsys):
    assert (
        main(
            [
                "impact",
                str(FIXTURES / "pipeline.jgf.json"),
                "--changed",
                "extract_orders,extract_customers",
                "--changed",
                "extract_orders",
                "--json",
            ]
        )
        == EXIT_OK
    )
    report = json.loads(capsys.readouterr().out)
    assert report["changed"] == ["extract_orders", "extract_customers"]
    assert report["impacted_count"] == 5


def test_impact_on_an_unknown_node_suggests_a_close_one(capsys):
    assert (
        main(["impact", str(FIXTURES / "pipeline.jgf.json"), "--changed", "extract_order"])
        == EXIT_INPUT_ERROR
    )
    err = capsys.readouterr().err
    assert "no node 'extract_order'" in err
    assert "did you mean 'extract_orders'" in err


def test_lists_are_truncated_in_text_mode_but_say_so(tmp_path, capsys):
    wide = tmp_path / "wide.dot"
    edges = " ".join(f"root -> leaf{index};" for index in range(25))
    wide.write_text("digraph { " + edges + " }", encoding="utf-8")
    assert main(["impact", str(wide), "--changed", "root", "--limit", "5"]) == EXIT_OK
    out = capsys.readouterr().out
    assert "(+20 more)" in out


def test_limit_zero_shows_everything(tmp_path, capsys):
    wide = tmp_path / "wide.dot"
    edges = " ".join(f"root -> leaf{index};" for index in range(25))
    wide.write_text("digraph { " + edges + " }", encoding="utf-8")
    assert main(["impact", str(wide), "--changed", "root", "--limit", "0"]) == EXIT_OK
    out = capsys.readouterr().out
    assert "more)" not in out
    assert "leaf24" in out


def test_json_report_is_never_truncated(tmp_path, capsys):
    wide = tmp_path / "wide.dot"
    edges = " ".join(f"root -> leaf{index};" for index in range(25))
    wide.write_text("digraph { " + edges + " }", encoding="utf-8")
    assert main(["impact", str(wide), "--changed", "root", "--limit", "5", "--json"]) == EXIT_OK
    report = json.loads(capsys.readouterr().out)
    assert len(report["downstream"]) == 25


@pytest.mark.parametrize(
    "argv",
    [
        ["parse", str(FIXTURES / "messy.dot")],
        ["stats", str(FIXTURES / "pipeline.jgf.json")],
        ["impact", str(FIXTURES / "pipeline.jgf.json"), "--changed", "transform_orders"],
    ],
)
def test_output_stays_ascii_for_legacy_consoles(argv, capsys):
    # A Windows console on a legacy code page turns anything else into '?'.
    assert main(argv) == EXIT_OK
    captured = capsys.readouterr()
    assert captured.out.isascii(), captured.out
    assert captured.err.isascii(), captured.err


def test_explain_shows_a_witness_path_per_reached_node(capsys):
    assert (
        main(
            [
                "impact",
                str(FIXTURES / "pipeline.jgf.json"),
                "--changed",
                "extract_orders",
                "--explain",
            ]
        )
        == EXIT_OK
    )
    out = capsys.readouterr().out
    assert "why (3 of 3 shown):" in out
    assert "transform_orders (distance 1): extract_orders -> transform_orders" in out
    assert (
        "notify (distance 3): extract_orders -> transform_orders -> load_warehouse -> notify" in out
    )


def test_explain_is_absent_from_json_unless_asked(capsys):
    assert (
        main(
            ["impact", str(FIXTURES / "pipeline.jgf.json"), "--changed", "extract_orders", "--json"]
        )
        == EXIT_OK
    )
    assert "explain" not in json.loads(capsys.readouterr().out)

    assert (
        main(
            [
                "impact",
                str(FIXTURES / "pipeline.jgf.json"),
                "--changed",
                "extract_orders",
                "--json",
                "--explain",
            ]
        )
        == EXIT_OK
    )
    report = json.loads(capsys.readouterr().out)
    assert report["explain"]["load_warehouse"]["distance"] == 2
    assert report["explain"]["load_warehouse"]["path"] == [
        "extract_orders",
        "transform_orders",
        "load_warehouse",
    ]
