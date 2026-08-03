package dagreach

// A reader for JSON Graph Format (https://jsongraphformat.info/).
//
// Both published node shapes are accepted - the object keyed by id, and the
// list of objects carrying an `id` - as is the bare {"nodes": [...], "edges":
// [...]} document that tools emit in practice. Anything accepted beyond the
// published specification is recorded as a warning rather than absorbed
// silently.

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ParseJGF parses JSON Graph Format text into a Graph.
func ParseJGF(text, source string) (*Graph, error) {
	document, err := DecodeOrderedJSON(text)
	if err != nil {
		line, column := jsonErrorPosition(text, err)
		return nil, &ParseError{
			Message: "invalid JSON: " + err.Error(),
			Source:  source,
			Line:    line,
			Column:  column,
		}
	}
	object, ok := document.(*Object)
	if !ok {
		return nil, &ParseError{Message: "expected a JSON object at the top level", Source: source}
	}

	graph := NewGraph(source)
	graph.Format = "jgf"
	body, err := selectGraphBody(object, graph, source)
	if err != nil {
		return nil, err
	}

	if name, ok := asText(body.Value("label")); ok {
		graph.Name = name
	} else if name, ok := asText(body.Value("id")); ok {
		graph.Name = name
	}

	graph.Directed = true
	if raw, present := body.Get("directed"); present {
		if value, ok := raw.(bool); ok {
			graph.Directed = value
		} else {
			graph.Warn(fmt.Sprintf(
				"'directed' is not a boolean (%s); assuming a directed graph", renderScalar(raw)))
		}
	}
	if !graph.Directed {
		graph.Warn("the input is an undirected graph; dagreach reads every edge as source -> target")
	}

	metadata := flatten(body.Value("metadata"), graph, "graph metadata")
	for _, key := range metadata.Keys() {
		graph.Attrs[key] = metadata.Text(key)
	}

	if err := readNodes(body.Value("nodes"), graph, source); err != nil {
		return nil, err
	}
	return graph, readEdges(body, graph, source)
}

func jsonErrorPosition(text string, err error) (int, int) {
	offset := int64(0)
	switch typed := err.(type) {
	case *json.SyntaxError:
		offset = typed.Offset
	case *json.UnmarshalTypeError:
		offset = typed.Offset
	}
	line, column := 1, 1
	for index := range text {
		if int64(index) >= offset {
			break
		}
		if text[index] == '\n' {
			line++
			column = 1
		} else {
			column++
		}
	}
	return line, column
}

func selectGraphBody(document *Object, graph *Graph, source string) (*Object, error) {
	var body any
	switch {
	case document.Value("graphs") != nil:
		graphs, ok := document.Value("graphs").([]any)
		if !ok || len(graphs) == 0 {
			return nil, &ParseError{Message: "'graphs' must be a non-empty array", Source: source}
		}
		if len(graphs) > 1 {
			graph.Warn(fmt.Sprintf(
				"the document holds %d graphs; only the first one was read", len(graphs)))
		}
		body = graphs[0]
	case document.Value("graph") != nil:
		body = document.Value("graph")
	default:
		// Without an envelope, the pair of keys is the only evidence that this
		// document is a graph at all - and a lone 'nodes' is not evidence. A dbt
		// manifest carries one, keyed by id exactly like JGF nodes, and used to
		// be read as nodes with no edges: a graph in which nothing reaches
		// anything, so every reach policy passes for want of a single edge.
		// Refusing the file is the only honest answer to a question it cannot
		// answer.
		_, hasNodes := document.Get("nodes")
		hasEdges := hasEdgeList(document)
		if !hasNodes && !hasEdges {
			return nil, &ParseError{
				Message: "expected a 'graph', 'graphs', or a top-level 'nodes'/'edges' object",
				Source:  source,
			}
		}
		if !hasNodes || !hasEdges {
			missing, present := "edges", "nodes"
			if !hasNodes {
				missing, present = "nodes", "edges"
			}
			return nil, &ParseError{
				Message: fmt.Sprintf(
					"this document has a top-level '%s' but no '%s', so nothing says it is a "+
						"graph rather than any object with a '%s' key; a genuinely edgeless "+
						"graph declares \"edges\": [], and a producer's export needs its "+
						"profile (%s)",
					present, missing, present, strings.Join(ProfileOrder, ", ")),
				Source: source,
			}
		}
		graph.Warn("the document has no 'graph' envelope; read as a bare nodes/edges object " +
			"(outside the JSON Graph Format specification)")
		body = document
	}

	object, ok := body.(*Object)
	if !ok {
		return nil, &ParseError{Message: "the graph must be a JSON object", Source: source}
	}
	return object, nil
}

func readNodes(nodes any, graph *Graph, source string) error {
	if nodes == nil {
		return nil
	}

	if keyed, ok := nodes.(*Object); ok {
		for _, id := range keyed.Keys() {
			graph.AddNode(id, nodeAttrs(keyed.Value(id), graph, id), true)
		}
		return nil
	}

	if listed, ok := nodes.([]any); ok {
		for position, payload := range listed {
			object, ok := payload.(*Object)
			if !ok {
				return &ParseError{
					Message: fmt.Sprintf("node #%d must be an object, found %s",
						position, jsonTypeName(payload)),
					Source: source,
				}
			}
			raw, present := object.Get("id")
			if !present {
				return &ParseError{
					Message: fmt.Sprintf("node #%d has no 'id'", position),
					Source:  source,
				}
			}
			id := asString(raw)
			graph.AddNode(id, nodeAttrs(object, graph, id), true)
		}
		return nil
	}

	return &ParseError{
		Message: fmt.Sprintf("'nodes' must be an object or an array, found %s", jsonTypeName(nodes)),
		Source:  source,
	}
}

// hasEdgeList reports whether an object declares an edge list under either
// spelling. Declared and empty is a graph with no edges; absent is a document
// that never claimed to have any.
func hasEdgeList(object *Object) bool {
	for _, key := range []string{"edges", "links"} {
		if _, present := object.Get(key); present {
			return true
		}
	}
	return false
}

func readEdges(body *Object, graph *Graph, source string) error {
	edges := body.Value("edges")
	if edges == nil {
		edges = body.Value("links")
		if edges != nil {
			graph.Warn("edges were read from 'links' (outside the JSON Graph Format specification)")
		}
	}
	if edges == nil {
		return nil
	}
	listed, ok := edges.([]any)
	if !ok {
		return &ParseError{
			Message: fmt.Sprintf("'edges' must be an array, found %s", jsonTypeName(edges)),
			Source:  source,
		}
	}

	for position, payload := range listed {
		object, ok := payload.(*Object)
		if !ok {
			return &ParseError{
				Message: fmt.Sprintf("edge #%d must be an object, found %s",
					position, jsonTypeName(payload)),
				Source: source,
			}
		}
		sourceID, err := endpoint(object, []string{"source", "from"}, position, "source", graph, source)
		if err != nil {
			return err
		}
		targetID, err := endpoint(object, []string{"target", "to"}, position, "target", graph, source)
		if err != nil {
			return err
		}
		attrs := flatten(object.Value("metadata"), graph, fmt.Sprintf("edge #%d metadata", position))
		for _, key := range []string{"relation", "label", "directed"} {
			if value, ok := asText(object.Value(key)); ok {
				attrs.setDefault(key, value)
			}
		}
		for _, id := range []string{sourceID, targetID} {
			if !graph.HasNode(id) {
				graph.Warn(fmt.Sprintf("edge #%d refers to undeclared node '%s'", position, id))
			}
		}
		graph.AddEdge(sourceID, targetID, attrs.Map())
	}
	return nil
}

func endpoint(
	payload *Object, keys []string, position int, what string, graph *Graph, source string,
) (string, error) {
	for index, key := range keys {
		if value, present := payload.Get(key); present {
			if index > 0 {
				graph.Warn(fmt.Sprintf(
					"edge #%d uses '%s' instead of '%s' "+
						"(outside the JSON Graph Format specification)", position, key, keys[0]))
			}
			return asString(value), nil
		}
	}
	return "", &ParseError{
		Message: fmt.Sprintf("edge #%d has no %s", position, what),
		Source:  source,
	}
}

func nodeAttrs(payload any, graph *Graph, id string) map[string]string {
	if payload == nil {
		return map[string]string{}
	}
	object, ok := payload.(*Object)
	if !ok {
		graph.Warn(fmt.Sprintf("node '%s' has a non-object body; its attributes were ignored", id))
		return map[string]string{}
	}
	attrs := flatten(object.Value("metadata"), graph, fmt.Sprintf("node '%s' metadata", id))
	for _, key := range object.Keys() {
		if key == "metadata" || key == "id" {
			continue
		}
		if value, ok := asText(object.Value(key)); ok {
			attrs.setDefault(key, value)
		}
	}
	return attrs.Map()
}

// attrList keeps attribute insertion order, which decides nothing on its own but
// keeps the two implementations byte-comparable.
type attrList struct {
	keys   []string
	values map[string]string
}

func newAttrList() *attrList { return &attrList{values: map[string]string{}} }

func (a *attrList) set(key, value string) {
	if _, present := a.values[key]; !present {
		a.keys = append(a.keys, key)
	}
	a.values[key] = value
}

func (a *attrList) setDefault(key, value string) {
	if _, present := a.values[key]; !present {
		a.set(key, value)
	}
}

func (a *attrList) Keys() []string         { return a.keys }
func (a *attrList) Text(key string) string { return a.values[key] }

func (a *attrList) Map() map[string]string {
	copied := map[string]string{}
	for key, value := range a.values {
		copied[key] = value
	}
	return copied
}

func flatten(metadata any, graph *Graph, what string) *attrList {
	flattened := newAttrList()
	if metadata == nil {
		return flattened
	}
	object, ok := metadata.(*Object)
	if !ok {
		graph.Warn(fmt.Sprintf("%s is not an object; it was ignored", what))
		return flattened
	}
	for _, key := range object.Keys() {
		value := object.Value(key)
		if text, ok := asText(value); ok {
			flattened.set(key, text)
		} else {
			flattened.set(key, renderValue(value))
		}
	}
	return flattened
}

// asText renders a scalar; containers and nulls are not text.
func asText(value any) (string, bool) {
	switch typed := value.(type) {
	case nil:
		return "", false
	case string:
		return typed, true
	case bool:
		if typed {
			return "true", true
		}
		return "false", true
	case json.Number:
		return typed.String(), true
	}
	return "", false
}

func asString(value any) string {
	if text, ok := asText(value); ok {
		return text
	}
	return renderValue(value)
}

func jsonTypeName(value any) string {
	switch value.(type) {
	case nil:
		return "NoneType"
	case bool:
		return "bool"
	case json.Number:
		return "float"
	case string:
		return "str"
	case []any:
		return "list"
	case *Object:
		return "dict"
	}
	return "object"
}
