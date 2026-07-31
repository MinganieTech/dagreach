import json
import subprocess
import sys
from pathlib import Path

import pytest

from dagreach import __version__
from dagreach.cli import EXIT_INPUT_ERROR, EXIT_NOT_IMPLEMENTED, EXIT_OK, main

FIXTURES = Path(__file__).parent / "fixtures"


def test_no_command_prints_help(capsys):
    assert main([]) == EXIT_OK
    assert "dagreach" in capsys.readouterr().out


@pytest.mark.parametrize("command", ["stats", "impact", "diff"])
def test_planned_commands_are_declared_but_not_implemented(command, capsys):
    assert main([command]) == EXIT_NOT_IMPLEMENTED
    assert "not implemented" in capsys.readouterr().err


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
