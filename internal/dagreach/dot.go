package dagreach

// The DOT reader, ported from src/dagreach/readers/dot.py. Same grammar, same
// error locations, same warnings - the point of the spike is that the JSON
// reports come out identical.

import (
	"fmt"
	"strings"
	"unicode"
)

type ParseError struct {
	Message string
	Source  string
	Line    int
	Column  int
}

func (e *ParseError) Error() string {
	location := e.Source
	if location == "" {
		location = "<input>"
	}
	if e.Line > 0 {
		location = fmt.Sprintf("%s:%d", location, e.Line)
		if e.Column > 0 {
			location = fmt.Sprintf("%s:%d", location, e.Column)
		}
	}
	return location + ": " + e.Message
}

var keywords = map[string]bool{
	"strict": true, "graph": true, "digraph": true,
	"node": true, "edge": true, "subgraph": true,
}

const punctuation = "{}[];,=:+"

type token struct {
	kind   string // ID | PUNCT | EDGEOP | EOF
	value  string
	line   int
	column int
	quoted bool
}

func (t token) keyword() string {
	if t.kind == "ID" && !t.quoted && keywords[strings.ToLower(t.value)] {
		return strings.ToLower(t.value)
	}
	return ""
}

type lexer struct {
	text   []rune
	source string
	pos    int
	line   int
	column int
}

func newLexer(text, source string) *lexer {
	return &lexer{text: []rune(text), source: source, line: 1, column: 1}
}

func (l *lexer) errorAt(message string, line, column int) error {
	return &ParseError{Message: message, Source: l.source, Line: line, Column: column}
}

func (l *lexer) fail(message string) error { return l.errorAt(message, l.line, l.column) }

func (l *lexer) advance(count int) string {
	var built strings.Builder
	for i := 0; i < count && l.pos < len(l.text); i++ {
		char := l.text[l.pos]
		built.WriteRune(char)
		if char == '\n' {
			l.line++
			l.column = 1
		} else {
			l.column++
		}
		l.pos++
	}
	return built.String()
}

func (l *lexer) peek(offset int) rune {
	index := l.pos + offset
	if index < 0 || index >= len(l.text) {
		return 0
	}
	return l.text[index]
}

func (l *lexer) atLineStart() bool {
	index := l.pos - 1
	for index >= 0 && (l.text[index] == ' ' || l.text[index] == '\t') {
		index--
	}
	return index < 0 || l.text[index] == '\n'
}

func (l *lexer) skipToEndOfLine() {
	for l.pos < len(l.text) && l.peek(0) != '\n' {
		l.advance(1)
	}
}

func (l *lexer) skipTrivia() error {
	for l.pos < len(l.text) {
		char := l.peek(0)
		switch {
		case char == ' ' || char == '\t' || char == '\r' || char == '\n':
			l.advance(1)
		case char == '/' && l.peek(1) == '/':
			l.skipToEndOfLine()
		case char == '#' && l.atLineStart():
			l.skipToEndOfLine()
		case char == '/' && l.peek(1) == '*':
			startLine, startColumn := l.line, l.column
			l.advance(2)
			for l.pos < len(l.text) && !(l.peek(0) == '*' && l.peek(1) == '/') {
				l.advance(1)
			}
			if l.pos >= len(l.text) {
				return l.errorAt("unterminated block comment", startLine, startColumn)
			}
			l.advance(2)
		default:
			return nil
		}
	}
	return nil
}

func isNameStart(char rune) bool {
	return unicode.IsLetter(char) || char == '_' || char >= 128
}

func isNameChar(char rune) bool { return isNameStart(char) || unicode.IsDigit(char) }

func (l *lexer) readQuoted() (string, error) {
	startLine, startColumn := l.line, l.column
	l.advance(1)
	var built strings.Builder
	for {
		if l.pos >= len(l.text) {
			return "", l.errorAt("unterminated quoted string", startLine, startColumn)
		}
		char := l.peek(0)
		if char == '\\' {
			following := l.peek(1)
			if following == '\n' {
				l.advance(2)
				continue
			}
			if following == '"' {
				l.advance(2)
				built.WriteRune('"')
				continue
			}
			l.advance(2)
			built.WriteRune('\\')
			built.WriteRune(following)
			continue
		}
		if char == '"' {
			l.advance(1)
			return built.String(), nil
		}
		built.WriteString(l.advance(1))
	}
}

func (l *lexer) readHTML() (string, error) {
	startLine, startColumn := l.line, l.column
	depth := 0
	var built strings.Builder
	for {
		if l.pos >= len(l.text) {
			return "", l.errorAt("unterminated HTML string", startLine, startColumn)
		}
		char := l.peek(0)
		built.WriteString(l.advance(1))
		if char == '<' {
			depth++
		} else if char == '>' {
			depth--
			if depth == 0 {
				return built.String(), nil
			}
		}
	}
}

func (l *lexer) readNumeral() (string, error) {
	var built strings.Builder
	if l.peek(0) == '-' {
		built.WriteString(l.advance(1))
	}
	seenDot, seenDigit := false, false
	for l.pos < len(l.text) {
		char := l.peek(0)
		if unicode.IsDigit(char) {
			seenDigit = true
			built.WriteString(l.advance(1))
		} else if char == '.' && !seenDot {
			seenDot = true
			built.WriteString(l.advance(1))
		} else {
			break
		}
	}
	if !seenDigit {
		return "", l.fail(fmt.Sprintf("expected a number, found '%s'", built.String()))
	}
	return built.String(), nil
}

// absorbConcatenation joins `"a" + "b"`, as DOT allows.
func (l *lexer) absorbConcatenation(value string) (string, error) {
	for {
		savedPos, savedLine, savedColumn := l.pos, l.line, l.column
		if err := l.skipTrivia(); err != nil {
			return "", err
		}
		if l.peek(0) != '+' {
			l.pos, l.line, l.column = savedPos, savedLine, savedColumn
			return value, nil
		}
		l.advance(1)
		if err := l.skipTrivia(); err != nil {
			return "", err
		}
		if l.peek(0) != '"' {
			l.pos, l.line, l.column = savedPos, savedLine, savedColumn
			return value, nil
		}
		more, err := l.readQuoted()
		if err != nil {
			return "", err
		}
		value += more
	}
}

func (l *lexer) tokens() ([]token, error) {
	var tokens []token
	for {
		if err := l.skipTrivia(); err != nil {
			return nil, err
		}
		line, column := l.line, l.column
		if l.pos >= len(l.text) {
			return append(tokens, token{kind: "EOF", line: line, column: column}), nil
		}
		char := l.peek(0)

		switch {
		case char == '-' && (l.peek(1) == '>' || l.peek(1) == '-'):
			tokens = append(tokens, token{kind: "EDGEOP", value: l.advance(2), line: line, column: column})
		case char == '"':
			value, err := l.readQuoted()
			if err != nil {
				return nil, err
			}
			value, err = l.absorbConcatenation(value)
			if err != nil {
				return nil, err
			}
			tokens = append(tokens, token{kind: "ID", value: value, line: line, column: column, quoted: true})
		case char == '<':
			value, err := l.readHTML()
			if err != nil {
				return nil, err
			}
			tokens = append(tokens, token{kind: "ID", value: value, line: line, column: column, quoted: true})
		case unicode.IsDigit(char) || (char == '-' && (unicode.IsDigit(l.peek(1)) || l.peek(1) == '.')) ||
			(char == '.' && unicode.IsDigit(l.peek(1))):
			value, err := l.readNumeral()
			if err != nil {
				return nil, err
			}
			tokens = append(tokens, token{kind: "ID", value: value, line: line, column: column})
		case isNameStart(char):
			var built strings.Builder
			built.WriteString(l.advance(1))
			for l.pos < len(l.text) && isNameChar(l.peek(0)) {
				built.WriteString(l.advance(1))
			}
			tokens = append(tokens, token{kind: "ID", value: built.String(), line: line, column: column})
		case strings.ContainsRune(punctuation, char):
			tokens = append(tokens, token{kind: "PUNCT", value: l.advance(1), line: line, column: column})
		default:
			return nil, l.fail(fmt.Sprintf("unexpected character '%c'", char))
		}
	}
}

type parser struct {
	tokens []token
	source string
	index  int
	graph  *Graph
	edgeOp string
	strict bool
}

func (p *parser) current() token { return p.tokens[p.index] }

func (p *parser) failAt(message string, at token) error {
	return &ParseError{Message: message, Source: p.source, Line: at.line, Column: at.column}
}

func (p *parser) fail(message string) error { return p.failAt(message, p.current()) }

func (p *parser) take() token {
	current := p.tokens[p.index]
	if current.kind != "EOF" {
		p.index++
	}
	return current
}

func (p *parser) atPunct(value string) bool {
	return p.current().kind == "PUNCT" && p.current().value == value
}

func describe(t token) string {
	if t.kind == "EOF" {
		return "end of input"
	}
	return "'" + t.value + "'"
}

func (p *parser) expectPunct(value string) error {
	if !p.atPunct(value) {
		return p.fail(fmt.Sprintf("expected '%s', found %s", value, describe(p.current())))
	}
	p.take()
	return nil
}

func (p *parser) expectID(what string) (token, error) {
	if p.current().kind != "ID" {
		return token{}, p.fail(fmt.Sprintf("expected %s, found %s", what, describe(p.current())))
	}
	return p.take(), nil
}

func ParseDOT(text, source string) (*Graph, error) {
	tokens, err := newLexer(text, source).tokens()
	if err != nil {
		return nil, err
	}
	p := &parser{tokens: tokens, source: source, graph: NewGraph(source), edgeOp: "->"}
	p.graph.Format = "dot"
	if err := p.parse(); err != nil {
		return nil, err
	}
	return p.graph, nil
}

func (p *parser) parse() error {
	if p.current().keyword() == "strict" {
		p.take()
		p.strict = true
	}

	keyword := p.current().keyword()
	if keyword != "graph" && keyword != "digraph" {
		return p.fail(fmt.Sprintf(
			"expected 'graph' or 'digraph' at the top level, found %s", describe(p.current())))
	}
	p.take()
	p.graph.Directed = keyword == "digraph"
	if p.graph.Directed {
		p.edgeOp = "->"
	} else {
		p.edgeOp = "--"
		p.graph.Warn("the input is an undirected graph; dagreach reads every edge as source -> target")
	}

	if p.current().kind == "ID" && !p.atPunct("{") {
		p.graph.Name = p.take().value
	}
	if err := p.expectPunct("{"); err != nil {
		return err
	}
	if err := p.parseStatements(map[string]string{}, map[string]string{}, nil); err != nil {
		return err
	}
	if err := p.expectPunct("}"); err != nil {
		return err
	}
	if p.current().kind != "EOF" {
		return p.fail(fmt.Sprintf("unexpected %s after the closing '}'", describe(p.current())))
	}
	if p.strict {
		p.applyStrict()
	}
	return nil
}

func copyAttrs(attrs map[string]string) map[string]string {
	copied := map[string]string{}
	for key, value := range attrs {
		copied[key] = value
	}
	return copied
}

func (p *parser) parseStatements(nodeDefaults, edgeDefaults map[string]string, collected *[]string) error {
	for !p.atPunct("}") {
		if p.current().kind == "EOF" {
			return p.fail("unexpected end of input, expected '}'")
		}
		if err := p.parseStatement(nodeDefaults, edgeDefaults, collected); err != nil {
			return err
		}
		for p.atPunct(";") {
			p.take()
		}
	}
	return nil
}

func (p *parser) parseStatement(nodeDefaults, edgeDefaults map[string]string, collected *[]string) error {
	keyword := p.current().keyword()

	if keyword == "node" || keyword == "edge" || keyword == "graph" {
		p.take()
		attrs, err := p.parseAttrList()
		if err != nil {
			return err
		}
		target := p.graph.Attrs
		switch keyword {
		case "node":
			target = nodeDefaults
		case "edge":
			target = edgeDefaults
		}
		for key, value := range attrs {
			target[key] = value
		}
		return nil
	}

	if keyword == "subgraph" || p.atPunct("{") {
		endpoints, err := p.parseSubgraph(nodeDefaults, edgeDefaults)
		if err != nil {
			return err
		}
		if collected != nil {
			*collected = append(*collected, endpoints...)
		}
		if p.current().kind == "EDGEOP" {
			return p.parseEdgeChain(endpoints, nodeDefaults, edgeDefaults, collected)
		}
		return nil
	}

	name, err := p.expectID("a node name, 'node', 'edge', 'graph' or 'subgraph'")
	if err != nil {
		return err
	}

	if p.atPunct("=") {
		p.take()
		value, err := p.expectID("an attribute value")
		if err != nil {
			return err
		}
		p.graph.Attrs[name.value] = value.value
		return nil
	}

	nodeID := name.value
	if err := p.skipPort(); err != nil {
		return err
	}

	if p.current().kind == "EDGEOP" {
		return p.parseEdgeChain([]string{nodeID}, nodeDefaults, edgeDefaults, collected)
	}

	p.graph.AddNode(nodeID, copyAttrs(nodeDefaults), false)
	if p.atPunct("[") {
		attrs, err := p.parseAttrList()
		if err != nil {
			return err
		}
		p.graph.AddNode(nodeID, attrs, true)
	}
	if collected != nil {
		*collected = append(*collected, nodeID)
	}
	return nil
}

func (p *parser) parseSubgraph(nodeDefaults, edgeDefaults map[string]string) ([]string, error) {
	if p.current().keyword() == "subgraph" {
		p.take()
		if p.current().kind == "ID" {
			p.take()
		}
	}
	if err := p.expectPunct("{"); err != nil {
		return nil, err
	}
	var collected []string
	if err := p.parseStatements(copyAttrs(nodeDefaults), copyAttrs(edgeDefaults), &collected); err != nil {
		return nil, err
	}
	if err := p.expectPunct("}"); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var unique []string
	for _, node := range collected {
		if !seen[node] {
			seen[node] = true
			unique = append(unique, node)
		}
	}
	return unique, nil
}

func (p *parser) parseEdgeChain(left []string, nodeDefaults, edgeDefaults map[string]string, collected *[]string) error {
	groups := [][]string{left}

	for p.current().kind == "EDGEOP" {
		operator := p.take()
		if operator.value != p.edgeOp {
			expected := "graph"
			if operator.value == "->" {
				expected = "digraph"
			}
			return p.failAt(fmt.Sprintf("'%s' is only valid in a %s; this graph uses '%s'",
				operator.value, expected, p.edgeOp), operator)
		}
		if p.current().keyword() == "subgraph" || p.atPunct("{") {
			endpoints, err := p.parseSubgraph(nodeDefaults, edgeDefaults)
			if err != nil {
				return err
			}
			groups = append(groups, endpoints)
			continue
		}
		target, err := p.expectID("a node name after the edge operator")
		if err != nil {
			return err
		}
		if err := p.skipPort(); err != nil {
			return err
		}
		groups = append(groups, []string{target.value})
	}

	attrs := copyAttrs(edgeDefaults)
	if p.atPunct("[") {
		explicit, err := p.parseAttrList()
		if err != nil {
			return err
		}
		for key, value := range explicit {
			attrs[key] = value
		}
	}

	for _, group := range groups {
		for _, nodeID := range group {
			p.graph.AddNode(nodeID, copyAttrs(nodeDefaults), false)
			if collected != nil {
				*collected = append(*collected, nodeID)
			}
		}
	}
	for index := 0; index+1 < len(groups); index++ {
		for _, source := range groups[index] {
			for _, target := range groups[index+1] {
				p.graph.AddEdge(source, target, attrs)
			}
		}
	}
	return nil
}

func (p *parser) parseAttrList() (map[string]string, error) {
	attrs := map[string]string{}
	for p.atPunct("[") {
		p.take()
		for !p.atPunct("]") {
			if p.current().kind == "EOF" {
				return nil, p.fail("unexpected end of input, expected ']'")
			}
			if p.atPunct("}") {
				return nil, p.fail("expected ']' to close the attribute list, found '}'")
			}
			key, err := p.expectID("an attribute name")
			if err != nil {
				return nil, err
			}
			if p.atPunct("=") {
				p.take()
				value, err := p.expectID("an attribute value")
				if err != nil {
					return nil, err
				}
				attrs[key.value] = value.value
			} else {
				attrs[key.value] = "true"
			}
			for p.atPunct(",") || p.atPunct(";") {
				p.take()
			}
		}
		if err := p.expectPunct("]"); err != nil {
			return nil, err
		}
	}
	return attrs, nil
}

func (p *parser) skipPort() error {
	for p.atPunct(":") {
		p.take()
		if _, err := p.expectID("a port name after ':'"); err != nil {
			return err
		}
	}
	return nil
}

func (p *parser) applyStrict() {
	kept := make([]*Edge, 0, len(p.graph.Edges))
	seen := map[pair]bool{}
	dropped := 0
	for _, edge := range p.graph.Edges {
		key := pair{edge.Source, edge.Target}
		if seen[key] {
			dropped++
			continue
		}
		seen[key] = true
		kept = append(kept, edge)
	}
	if dropped > 0 {
		p.graph.Edges = kept
		p.graph.Warn(fmt.Sprintf(
			"the graph is declared 'strict': %d parallel edge(s) were collapsed", dropped))
	}
}
