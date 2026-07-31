package dagreach

// Reading a graph from a file, a stream, or standard input.

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Formats are the two graph formats dagreach reads on its own.
var Formats = []string{"dot", "jgf"}

var (
	dotExtensions = map[string]bool{".dot": true, ".gv": true}
	jgfExtensions = map[string]bool{".json": true, ".jgf": true}
	dotHead       = regexp.MustCompile(`(?i)^\s*(strict\s+)?(di)?graph\b`)
)

// ReadSource returns the text of path (or standard input when it is "-"), and a
// name for it.
func ReadSource(path string, stdin io.Reader) (string, string, error) {
	if path == "-" {
		text, err := io.ReadAll(stdin)
		if err != nil {
			return "", "", &InputError{Message: fmt.Sprintf("<stdin>: %v", err)}
		}
		return string(text), "<stdin>", nil
	}
	info, err := os.Stat(path)
	if err == nil && info.IsDir() {
		return "", "", &InputError{Message: path + ": is a directory, not a graph"}
	}
	text, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", "", &InputError{Message: path + ": no such file"}
		}
		return "", "", &InputError{Message: fmt.Sprintf("%s: %v", path, err)}
	}
	return string(text), path, nil
}

// ReadText parses graph text. `hint` is a path used only to guess the format.
func ReadText(text, source, format, hint string) (*Graph, error) {
	resolved := format
	if resolved == "" {
		detected, err := DetectFormat(text, hint)
		if err != nil {
			return nil, err
		}
		resolved = detected
	}

	var graph *Graph
	var err error
	switch resolved {
	case "dot":
		graph, err = ParseDOT(text, source)
	case "jgf":
		graph, err = ParseJGF(text, source)
	default:
		return nil, &UsageError{Message: fmt.Sprintf(
			"unknown format '%s'; expected one of %s", resolved, strings.Join(Formats, ", "))}
	}
	if err != nil {
		return nil, err
	}
	graph.Format = resolved
	return graph, nil
}

// DetectFormat guesses the input format from the file name first, then the content.
func DetectFormat(text, hint string) (string, error) {
	if hint != "" && hint != "-" {
		switch suffix := strings.ToLower(filepath.Ext(hint)); {
		case dotExtensions[suffix]:
			return "dot", nil
		case jgfExtensions[suffix]:
			return "jgf", nil
		}
	}

	stripped := stripLeadingTrivia(text)
	if dotHead.MatchString(stripped) {
		return "dot", nil
	}
	if strings.HasPrefix(stripped, "{") || strings.HasPrefix(stripped, "[") {
		return "jgf", nil
	}
	return "", &InputError{Message: "could not tell whether this input is DOT or JSON Graph " +
		"Format; pass --format"}
}

// stripLeadingTrivia drops whitespace and comments so detection sees the first real token.
func stripLeadingTrivia(text string) string {
	index := 0
	for index < len(text) {
		switch {
		case text[index] == ' ' || text[index] == '\t' || text[index] == '\r' || text[index] == '\n':
			index++
		case strings.HasPrefix(text[index:], "//"), strings.HasPrefix(text[index:], "#"):
			end := strings.IndexByte(text[index:], '\n')
			if end < 0 {
				return ""
			}
			index += end + 1
		case strings.HasPrefix(text[index:], "/*"):
			end := strings.Index(text[index+2:], "*/")
			if end < 0 {
				return ""
			}
			index += end + 4
		default:
			return text[index:]
		}
	}
	return ""
}
