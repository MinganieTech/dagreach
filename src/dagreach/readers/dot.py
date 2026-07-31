"""A reader for the Graphviz DOT language.

Hand-written rather than delegated to a library for two reasons: dagreach ships
with no runtime dependencies, and error messages must point at a line and a
column, which is what makes a malformed export debuggable.

The grammar implemented is the one published at https://graphviz.org/doc/info/lang.html.
Layout attributes are kept verbatim as node/edge attributes; ports and compass
points are parsed and dropped, since they address a drawing, not a dependency.
"""

from __future__ import annotations

from dataclasses import dataclass

from dagreach.errors import ParseError
from dagreach.model import Graph

_KEYWORDS = {"strict", "graph", "digraph", "node", "edge", "subgraph"}
_PUNCT = set("{}[];,=:+")


@dataclass(frozen=True, slots=True)
class Token:
    kind: str  # "ID" | "PUNCT" | "EDGEOP" | "EOF"
    value: str
    line: int
    column: int
    quoted: bool = False

    @property
    def keyword(self) -> str | None:
        """The lowercase keyword this token stands for, if it is an unquoted one."""
        if self.kind == "ID" and not self.quoted and self.value.lower() in _KEYWORDS:
            return self.value.lower()
        return None


class _Lexer:
    def __init__(self, text: str, source: str | None) -> None:
        self.text = text
        self.source = source
        self.pos = 0
        self.line = 1
        self.column = 1

    # -- low level -------------------------------------------------------

    def _error(
        self, message: str, line: int | None = None, column: int | None = None
    ) -> ParseError:
        return ParseError(
            message,
            source=self.source,
            line=self.line if line is None else line,
            column=self.column if column is None else column,
        )

    def _advance(self, count: int = 1) -> str:
        chunk = self.text[self.pos : self.pos + count]
        for char in chunk:
            if char == "\n":
                self.line += 1
                self.column = 1
            else:
                self.column += 1
        self.pos += count
        return chunk

    def _peek(self, offset: int = 0) -> str:
        index = self.pos + offset
        return self.text[index] if index < len(self.text) else ""

    def _at_line_start(self) -> bool:
        index = self.pos - 1
        while index >= 0 and self.text[index] in " \t":
            index -= 1
        return index < 0 or self.text[index] == "\n"

    def _skip_trivia(self) -> None:
        while self.pos < len(self.text):
            char = self._peek()
            if char in " \t\r\n":
                self._advance()
            elif char == "/" and self._peek(1) == "/":
                self._skip_to_end_of_line()
            elif char == "#" and self._at_line_start():
                self._skip_to_end_of_line()
            elif char == "/" and self._peek(1) == "*":
                start_line, start_column = self.line, self.column
                self._advance(2)
                while self.pos < len(self.text) and not (
                    self._peek() == "*" and self._peek(1) == "/"
                ):
                    self._advance()
                if self.pos >= len(self.text):
                    raise self._error("unterminated block comment", start_line, start_column)
                self._advance(2)
            else:
                return

    def _skip_to_end_of_line(self) -> None:
        while self.pos < len(self.text) and self._peek() != "\n":
            self._advance()

    # -- identifiers -----------------------------------------------------

    @staticmethod
    def _is_name_start(char: str) -> bool:
        return char.isalpha() or char == "_" or ord(char) >= 128

    @staticmethod
    def _is_name_char(char: str) -> bool:
        return _Lexer._is_name_start(char) or char.isdigit()

    def _read_quoted(self) -> str:
        start_line, start_column = self.line, self.column
        self._advance()  # opening quote
        chars: list[str] = []
        while True:
            if self.pos >= len(self.text):
                raise self._error("unterminated quoted string", start_line, start_column)
            char = self._peek()
            if char == "\\":
                following = self._peek(1)
                if following == "\n":  # line continuation: both characters vanish
                    self._advance(2)
                    continue
                if following == '"':
                    self._advance(2)
                    chars.append('"')
                    continue
                self._advance(2)
                chars.append("\\" + following)
                continue
            if char == '"':
                self._advance()
                return "".join(chars)
            chars.append(self._advance())

    def _read_html(self) -> str:
        start_line, start_column = self.line, self.column
        depth = 0
        chars: list[str] = []
        while True:
            if self.pos >= len(self.text):
                raise self._error("unterminated HTML string", start_line, start_column)
            char = self._advance()
            chars.append(char)
            if char == "<":
                depth += 1
            elif char == ">":
                depth -= 1
                if depth == 0:
                    return "".join(chars)

    def _read_numeral(self) -> str:
        chars: list[str] = []
        if self._peek() == "-":
            chars.append(self._advance())
        seen_dot = False
        seen_digit = False
        while self.pos < len(self.text):
            char = self._peek()
            if char.isdigit():
                seen_digit = True
                chars.append(self._advance())
            elif char == "." and not seen_dot:
                seen_dot = True
                chars.append(self._advance())
            else:
                break
        if not seen_digit:
            raise self._error(f"expected a number, found {''.join(chars)!r}")
        return "".join(chars)

    # -- token stream ----------------------------------------------------

    def tokens(self) -> list[Token]:
        tokens: list[Token] = []
        while True:
            self._skip_trivia()
            line, column = self.line, self.column
            if self.pos >= len(self.text):
                tokens.append(Token("EOF", "", line, column))
                return tokens

            char = self._peek()

            if char == "-" and self._peek(1) in {">", "-"}:
                tokens.append(Token("EDGEOP", self._advance(2), line, column))
                continue
            if char == '"':
                value = self._read_quoted()
                value = self._absorb_concatenation(value)
                tokens.append(Token("ID", value, line, column, quoted=True))
                continue
            if char == "<":
                tokens.append(Token("ID", self._read_html(), line, column, quoted=True))
                continue
            if char.isdigit() or (
                char == "-" and (self._peek(1).isdigit() or self._peek(1) == ".")
            ):
                tokens.append(Token("ID", self._read_numeral(), line, column))
                continue
            if char == "." and self._peek(1).isdigit():
                tokens.append(Token("ID", self._read_numeral(), line, column))
                continue
            if self._is_name_start(char):
                chars = [self._advance()]
                while self.pos < len(self.text) and self._is_name_char(self._peek()):
                    chars.append(self._advance())
                tokens.append(Token("ID", "".join(chars), line, column))
                continue
            if char in _PUNCT:
                tokens.append(Token("PUNCT", self._advance(), line, column))
                continue

            raise self._error(f"unexpected character {char!r}")

    def _absorb_concatenation(self, value: str) -> str:
        """DOT joins adjacent quoted strings written as `"a" + "b"`."""
        while True:
            saved = (self.pos, self.line, self.column)
            self._skip_trivia()
            if self._peek() != "+":
                self.pos, self.line, self.column = saved
                return value
            self._advance()
            self._skip_trivia()
            if self._peek() != '"':
                self.pos, self.line, self.column = saved
                return value
            value += self._read_quoted()


class _Parser:
    def __init__(self, tokens: list[Token], source: str | None) -> None:
        self.tokens = tokens
        self.source = source
        self.index = 0
        self.graph = Graph(source=source)
        self.edge_op = "->"
        self.strict = False

    # -- token helpers ---------------------------------------------------

    @property
    def current(self) -> Token:
        return self.tokens[self.index]

    def _error(self, message: str, token: Token | None = None) -> ParseError:
        token = token or self.current
        return ParseError(message, source=self.source, line=token.line, column=token.column)

    def _take(self) -> Token:
        token = self.tokens[self.index]
        if token.kind != "EOF":
            self.index += 1
        return token

    def _at_punct(self, value: str) -> bool:
        return self.current.kind == "PUNCT" and self.current.value == value

    def _expect_punct(self, value: str) -> Token:
        if not self._at_punct(value):
            raise self._error(f"expected {value!r}, found {self._describe(self.current)}")
        return self._take()

    def _expect_id(self, what: str) -> Token:
        if self.current.kind != "ID":
            raise self._error(f"expected {what}, found {self._describe(self.current)}")
        return self._take()

    @staticmethod
    def _describe(token: Token) -> str:
        if token.kind == "EOF":
            return "end of input"
        return repr(token.value)

    # -- grammar ---------------------------------------------------------

    def parse(self) -> Graph:
        if self.current.keyword == "strict":
            self._take()
            self.strict = True
        else:
            self.strict = False

        keyword = self.current.keyword
        if keyword not in {"graph", "digraph"}:
            found = self._describe(self.current)
            raise self._error(f"expected 'graph' or 'digraph' at the top level, found {found}")
        self._take()
        self.graph.directed = keyword == "digraph"
        self.edge_op = "->" if self.graph.directed else "--"
        if not self.graph.directed:
            self.graph.warn(
                "the input is an undirected graph; dagreach reads every edge as source -> target"
            )

        if self.current.kind == "ID" and not self._at_punct("{"):
            self.graph.name = self._take().value

        self._expect_punct("{")
        self._parse_statements(
            node_defaults={},
            edge_defaults={},
            collected=None,
        )
        self._expect_punct("}")

        if self.current.kind != "EOF":
            raise self._error(f"unexpected {self._describe(self.current)} after the closing '}}'")

        if self.strict:
            self._apply_strict()
        return self.graph

    def _parse_statements(
        self,
        *,
        node_defaults: dict[str, str],
        edge_defaults: dict[str, str],
        collected: list[str] | None,
    ) -> None:
        """Parse statements until the matching '}'.

        `collected` accumulates the nodes declared in the current subgraph so an
        edge whose endpoint is a subgraph can expand to every node inside it.
        """
        while not self._at_punct("}"):
            if self.current.kind == "EOF":
                raise self._error("unexpected end of input, expected '}'")
            self._parse_statement(
                node_defaults=node_defaults,
                edge_defaults=edge_defaults,
                collected=collected,
            )
            while self._at_punct(";"):
                self._take()

    def _parse_statement(
        self,
        *,
        node_defaults: dict[str, str],
        edge_defaults: dict[str, str],
        collected: list[str] | None,
    ) -> None:
        keyword = self.current.keyword

        if keyword in {"node", "edge", "graph"}:
            self._take()
            attrs = self._parse_attr_list()
            if keyword == "node":
                node_defaults.update(attrs)
            elif keyword == "edge":
                edge_defaults.update(attrs)
            else:
                self.graph.attrs.update(attrs)
            return

        if keyword == "subgraph" or self._at_punct("{"):
            endpoints = self._parse_subgraph(node_defaults, edge_defaults)
            if collected is not None:
                collected.extend(endpoints)
            if self.current.kind == "EDGEOP":
                self._parse_edge_chain(endpoints, node_defaults, edge_defaults, collected)
            return

        name_token = self._expect_id("a node name, 'node', 'edge', 'graph' or 'subgraph'")

        # A bare `key = value` statement sets a graph attribute.
        if self._at_punct("="):
            self._take()
            value = self._expect_id("an attribute value")
            self.graph.attrs[name_token.value] = value.value
            return

        node_id = name_token.value
        self._skip_port()

        if self.current.kind == "EDGEOP":
            self._parse_edge_chain([node_id], node_defaults, edge_defaults, collected)
            return

        self.graph.add_node(node_id, dict(node_defaults), override=False)
        if self._at_punct("["):
            self.graph.add_node(node_id, self._parse_attr_list())
        if collected is not None:
            collected.append(node_id)

    def _parse_subgraph(
        self, node_defaults: dict[str, str], edge_defaults: dict[str, str]
    ) -> list[str]:
        if self.current.keyword == "subgraph":
            self._take()
            if self.current.kind == "ID":
                self._take()  # the subgraph name is not part of the dependency graph
        self._expect_punct("{")
        collected: list[str] = []
        self._parse_statements(
            node_defaults=dict(node_defaults),
            edge_defaults=dict(edge_defaults),
            collected=collected,
        )
        self._expect_punct("}")
        # Preserve declaration order while removing repeats.
        return list(dict.fromkeys(collected))

    def _parse_edge_chain(
        self,
        left: list[str],
        node_defaults: dict[str, str],
        edge_defaults: dict[str, str],
        collected: list[str] | None,
    ) -> None:
        groups: list[list[str]] = [left]

        while self.current.kind == "EDGEOP":
            operator = self._take()
            if operator.value != self.edge_op:
                expected = "digraph" if operator.value == "->" else "graph"
                raise self._error(
                    f"'{operator.value}' is only valid in a {expected}; "
                    f"this graph uses '{self.edge_op}'",
                    operator,
                )
            if self.current.keyword == "subgraph" or self._at_punct("{"):
                groups.append(self._parse_subgraph(node_defaults, edge_defaults))
            else:
                target = self._expect_id("a node name after the edge operator")
                self._skip_port()
                groups.append([target.value])

        attrs = dict(edge_defaults)
        if self._at_punct("["):
            attrs.update(self._parse_attr_list())

        for group in groups:
            for node_id in group:
                self.graph.add_node(node_id, dict(node_defaults), override=False)
                if collected is not None:
                    collected.append(node_id)

        for source_group, target_group in zip(groups, groups[1:], strict=False):
            for source in source_group:
                for target in target_group:
                    self.graph.add_edge(source, target, dict(attrs))

    def _parse_attr_list(self) -> dict[str, str]:
        attrs: dict[str, str] = {}
        while self._at_punct("["):
            self._take()
            while not self._at_punct("]"):
                if self.current.kind == "EOF":
                    raise self._error("unexpected end of input, expected ']'")
                if self._at_punct("}"):
                    raise self._error("expected ']' to close the attribute list, found '}'")
                key = self._expect_id("an attribute name")
                if self._at_punct("="):
                    self._take()
                    value = self._expect_id("an attribute value")
                    attrs[key.value] = value.value
                else:
                    attrs[key.value] = "true"
                while self._at_punct(",") or self._at_punct(";"):
                    self._take()
            self._expect_punct("]")
        return attrs

    def _skip_port(self) -> None:
        """Consume `:port` and `:port:compass`, which address a drawing, not a node."""
        while self._at_punct(":"):
            self._take()
            self._expect_id("a port name after ':'")

    def _apply_strict(self) -> None:
        """`strict` collapses parallel edges; the first one seen wins."""
        kept: list = []
        seen: set[tuple[str, str]] = set()
        dropped = 0
        for edge in self.graph.edges:
            if edge.key in seen:
                dropped += 1
                continue
            seen.add(edge.key)
            kept.append(edge)
        if dropped:
            self.graph.edges = kept
            self.graph.warn(
                f"the graph is declared 'strict': {dropped} parallel edge(s) were collapsed"
            )


def parse_dot(text: str, *, source: str | None = None) -> Graph:
    """Parse DOT text into a :class:`~dagreach.model.Graph`."""
    tokens = _Lexer(text, source).tokens()
    return _Parser(tokens, source).parse()
