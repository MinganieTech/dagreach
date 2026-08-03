package dagreach

// One door for every input: pick a profile, read, orient.
//
// The order matters and is the same for every command. The profile decides how
// to read the file and which way its edges point; an explicit flag always wins
// over the profile; and whatever ends up applied is stated in the report rather
// than assumed by the reader.

import (
	"fmt"
	"io"
)

// LoadOptions are the three things a caller can force.
type LoadOptions struct {
	Profile       string
	Format        string
	EdgeSemantics string
	Stdin         io.Reader
}

// LoadGraph reads path through a profile - named, detected, or generic - and orients it.
func LoadGraph(path string, options LoadOptions) (*Graph, error) {
	text, source, err := ReadSource(path, options.Stdin)
	if err != nil {
		return nil, err
	}

	chosen, detected, err := resolveProfile(text, options.Profile)
	if err != nil {
		return nil, err
	}

	var graph *Graph
	if chosen.Name == "generic" {
		graph, err = ReadText(text, source, options.Format, path)
	} else {
		graph, err = chosen.Load(text, source)
	}
	if err != nil {
		return nil, err
	}
	graph.Profile = chosen.Name

	if options.Format != "" && chosen.Name != "generic" {
		graph.Warn(fmt.Sprintf(
			"--format %s was ignored: the %s profile knows the format it reads",
			options.Format, chosen.Name))
	}
	if detected {
		graph.Warn(fmt.Sprintf(
			"read with the %s profile, recognised from the file itself; "+
				"pass --profile to choose explicitly", chosen.Name))
	}

	semantics := options.EdgeSemantics
	if semantics == "" {
		semantics = chosen.EdgeSemantics
	}
	if semantics == "" {
		semantics = DefaultSemantics
	}
	if !knownSemantics(semantics) {
		return nil, &UsageError{Message: fmt.Sprintf("unknown edge semantics '%s'", semantics)}
	}
	graph = Orient(graph, semantics)

	if options.EdgeSemantics != "" && chosen.Name != "generic" &&
		options.EdgeSemantics != chosen.EdgeSemantics {
		graph.Warn(fmt.Sprintf(
			"--edge-semantics %s overrides the %s profile, which declares %s",
			options.EdgeSemantics, chosen.Name, chosen.EdgeSemantics))
	}
	if chosen.Name == "generic" {
		WarnIfOrientationIsSuspect(graph, options.EdgeSemantics)
	}
	// Nodes and no edges is a legitimate answer to `parse` and a trap for
	// everything else: nothing reaches anything, so every reach policy passes
	// without judging a thing. Said once here rather than in each reader,
	// because every reader can produce one.
	if graph.NodeCount() > 0 && graph.EdgeCount() == 0 {
		graph.Warn("this graph has no edges, so nothing reaches anything and every " +
			"reach policy passes without judging anything")
	}
	return graph, nil
}

func resolveProfile(text, requested string) (*Profile, bool, error) {
	if requested != "" {
		profile, ok := GetProfile(requested)
		if !ok {
			return nil, false, &UsageError{Message: fmt.Sprintf("unknown profile '%s'", requested)}
		}
		return profile, false, nil
	}
	if found := DetectProfile(text); found != nil {
		return found, true, nil
	}
	generic, _ := GetProfile("generic")
	return generic, false, nil
}
