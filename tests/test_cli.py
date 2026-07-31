import subprocess
import sys

import pytest

from dagreach import __version__
from dagreach.cli import EXIT_NOT_IMPLEMENTED, EXIT_OK, main


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
