package dagreach

// What an edge means, which decides which way "reaches" runs.
//
// Exports disagree, and the disagreement is invisible in the file:
//
//	terraform graph    "aws_instance.web" -> "aws_subnet.main"   source DEPENDS ON target
//	an Airflow export  extract -> transform                      source FEEDS target
//
// Read the first as if it were the second and every impact answer comes out
// exactly backwards, which is worse than no answer at all. dagreach therefore
// never guesses in silence: the orientation applied is stated in every report,
// and a file whose shape contradicts the orientation in force raises a warning.

import (
	"fmt"
	"strings"
)

// EdgeSemantics are the two conventions dagreach knows.
//
//	feeds      = source feeds target (follow edges forward to find what is affected)
//	depends-on = source depends on target (follow them backward)
var EdgeSemantics = []string{"feeds", "depends-on"}

// DefaultSemantics is what applies when nothing says otherwise.
const DefaultSemantics = "feeds"

func knownSemantics(name string) bool {
	for _, known := range EdgeSemantics {
		if known == name {
			return true
		}
	}
	return false
}

// LooksLikeDependencyExport names the producer when the file carries a
// recognisable dependency-first signature, or returns "" rather than guess.
func LooksLikeDependencyExport(g *Graph) string {
	_, hasCompound := g.Attrs["compound"]
	_, hasNewrank := g.Attrs["newrank"]
	if !hasCompound || !hasNewrank { // both, as terraform emits them
		return ""
	}
	for _, node := range g.Nodes() {
		if strings.HasPrefix(node, "[root] ") {
			return "terraform graph"
		}
	}
	return ""
}

// WarnIfOrientationIsSuspect warns when the file looks like a dependency export
// but nobody said so.
func WarnIfOrientationIsSuspect(g *Graph, declared string) {
	if declared == "depends-on" {
		return
	}
	producer := LooksLikeDependencyExport(g)
	if producer == "" {
		return
	}
	g.Warn(fmt.Sprintf(
		"this file looks like %s output, where an edge means 'source depends on target', "+
			"but it was read as '%s'; pass --edge-semantics depends-on if impact comes out backwards",
		producer, g.EdgeSemantics))
}
