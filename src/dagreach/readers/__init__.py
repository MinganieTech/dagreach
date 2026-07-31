"""Reading a graph from a file, a stream, or standard input."""

from __future__ import annotations

import re
import sys
from pathlib import Path

from dagreach.errors import DagreachError, UnknownFormatError
from dagreach.model import Graph
from dagreach.readers.dot import parse_dot
from dagreach.readers.jgf import parse_jgf

FORMATS = ("dot", "jgf")

_DOT_EXTENSIONS = {".dot", ".gv"}
_JGF_EXTENSIONS = {".json", ".jgf"}
_DOT_HEAD = re.compile(r"^\s*(strict\s+)?(di)?graph\b", re.IGNORECASE)

__all__ = [
    "FORMATS",
    "detect_format",
    "parse_dot",
    "parse_jgf",
    "read",
    "read_source",
    "read_text",
]


def read_source(path: str | Path) -> tuple[str, str]:
    """The text of `path` (or standard input when it is `-`), and a name for it."""
    if str(path) == "-":
        return sys.stdin.read(), "<stdin>"
    file_path = Path(path)
    try:
        return file_path.read_text(encoding="utf-8"), str(file_path)
    except FileNotFoundError as exc:
        raise DagreachError(f"{file_path}: no such file") from exc
    except IsADirectoryError as exc:
        raise DagreachError(f"{file_path}: is a directory, not a graph") from exc
    except UnicodeDecodeError as exc:
        raise DagreachError(f"{file_path}: not UTF-8 text") from exc


def read(path: str | Path, *, format: str | None = None) -> Graph:
    """Read a graph from `path`, or from standard input when `path` is `-`."""
    text, source = read_source(path)
    return read_text(text, source=source, format=format, hint=path)


def read_text(
    text: str,
    *,
    source: str | None = None,
    format: str | None = None,
    hint: str | Path | None = None,
) -> Graph:
    """Parse graph text. `hint` is a path used only to guess the format."""
    resolved = format or detect_format(text, hint)
    if resolved == "dot":
        graph = parse_dot(text, source=source)
    elif resolved == "jgf":
        graph = parse_jgf(text, source=source)
    else:
        raise DagreachError(f"unknown format {resolved!r}; expected one of {', '.join(FORMATS)}")
    graph.format = resolved
    return graph


def detect_format(text: str, hint: str | Path | None = None) -> str:
    """Guess the input format from the file name first, then from the content."""
    if hint is not None and str(hint) != "-":
        suffix = Path(hint).suffix.lower()
        if suffix in _DOT_EXTENSIONS:
            return "dot"
        if suffix in _JGF_EXTENSIONS:
            return "jgf"

    stripped = _strip_leading_trivia(text)
    if _DOT_HEAD.match(stripped):
        return "dot"
    if stripped.startswith("{") or stripped.startswith("["):
        return "jgf"
    raise UnknownFormatError(
        "could not tell whether this input is DOT or JSON Graph Format; pass --format"
    )


def _strip_leading_trivia(text: str) -> str:
    """Drop leading whitespace and comments so detection sees the first real token."""
    index = 0
    length = len(text)
    while index < length:
        char = text[index]
        if char in " \t\r\n":
            index += 1
        elif text.startswith("//", index) or text.startswith("#", index):
            end = text.find("\n", index)
            index = length if end == -1 else end + 1
        elif text.startswith("/*", index):
            end = text.find("*/", index + 2)
            index = length if end == -1 else end + 2
        else:
            break
    return text[index:]
