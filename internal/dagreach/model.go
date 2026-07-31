package dagreach

// The graph model every reader produces and every command consumes.
//
// Deliberately small: identifiers, attributes, and the order things were seen
// in. Declaration order is part of the contract, so nodes keep an explicit
// order alongside the lookup map. No adjacency index and no algorithms live
// here - analysis builds its own structures on top.

type Node struct {
	ID    string
	Attrs map[string]string
}

type Edge struct {
	Source string
	Target string
	Attrs  map[string]string
}

type Graph struct {
	Name          string
	Directed      bool
	Source        string
	Format        string
	Profile       string
	EdgeSemantics string
	order         []string
	nodes         map[string]*Node
	Edges         []*Edge
	Attrs         map[string]string
	Warnings      []string
}

func NewGraph(source string) *Graph {
	return &Graph{
		Directed:      true,
		Source:        source,
		EdgeSemantics: "feeds",
		nodes:         map[string]*Node{},
		Attrs:         map[string]string{},
		Warnings:      []string{},
	}
}

// AddNode declares a node. When override is false the attributes are inherited
// defaults: they fill in what is missing and never overwrite an explicit value.
func (g *Graph) AddNode(id string, attrs map[string]string, override bool) *Node {
	node, ok := g.nodes[id]
	if !ok {
		copied := map[string]string{}
		for key, value := range attrs {
			copied[key] = value
		}
		node = &Node{ID: id, Attrs: copied}
		g.nodes[id] = node
		g.order = append(g.order, id)
		return node
	}
	for key, value := range attrs {
		if override {
			node.Attrs[key] = value
		} else if _, present := node.Attrs[key]; !present {
			node.Attrs[key] = value
		}
	}
	return node
}

func (g *Graph) AddEdge(source, target string, attrs map[string]string) {
	g.AddNode(source, nil, true)
	g.AddNode(target, nil, true)
	copied := map[string]string{}
	for key, value := range attrs {
		copied[key] = value
	}
	g.Edges = append(g.Edges, &Edge{Source: source, Target: target, Attrs: copied})
}

func (g *Graph) Warn(message string) { g.Warnings = append(g.Warnings, message) }

func (g *Graph) Nodes() []string        { return g.order }
func (g *Graph) Node(id string) *Node   { return g.nodes[id] }
func (g *Graph) HasNode(id string) bool { _, ok := g.nodes[id]; return ok }
func (g *Graph) NodeCount() int         { return len(g.order) }
func (g *Graph) EdgeCount() int         { return len(g.Edges) }

func (g *Graph) SelfLoops() int {
	count := 0
	for _, edge := range g.Edges {
		if edge.Source == edge.Target {
			count++
		}
	}
	return count
}

type pair struct{ Source, Target string }

func (g *Graph) DuplicateEdges() int {
	seen := map[pair]bool{}
	duplicated := map[pair]bool{}
	for _, edge := range g.Edges {
		key := pair{edge.Source, edge.Target}
		if seen[key] {
			duplicated[key] = true
		}
		seen[key] = true
	}
	return len(duplicated)
}

// Orient turns a depends-on graph into dagreach's internal orientation.
func Orient(g *Graph, semantics string) *Graph {
	g.EdgeSemantics = semantics
	if semantics == "feeds" {
		return g
	}
	for _, edge := range g.Edges {
		edge.Source, edge.Target = edge.Target, edge.Source
	}
	return g
}
