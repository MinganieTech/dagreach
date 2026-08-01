package dagreach

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The examples in examples/README.md are executable. Each one runs here against
// the corpus, and its exit code and the lines the documentation quotes are
// checked - so a documented outcome cannot quietly stop being true.
func TestDocumentedExamplesStillHold(t *testing.T) {
	corpus := filepath.Join("..", "..", "examples")
	manifest, err := os.ReadFile(filepath.Join(corpus, "examples.json"))
	if err != nil {
		t.Fatal(err)
	}

	var document struct {
		Examples []struct {
			Name     string   `json:"name"`
			Outcome  string   `json:"outcome"`
			Args     []string `json:"args"`
			Exit     int      `json:"exit"`
			Contains []string `json:"contains"`
		} `json:"examples"`
	}
	if err := json.Unmarshal(manifest, &document); err != nil {
		t.Fatal(err)
	}
	if len(document.Examples) == 0 {
		t.Fatal("the manifest lists no examples")
	}

	for _, example := range document.Examples {
		t.Run(example.Name, func(t *testing.T) {
			args := make([]string, 0, len(example.Args))
			for index, argument := range example.Args {
				// The first two arguments are the command and its file(s); the
				// manifest names them relative to the corpus, as a reader would.
				if index > 0 && !strings.HasPrefix(argument, "-") &&
					(strings.HasSuffix(argument, ".dot") || strings.HasSuffix(argument, ".json")) {
					argument = filepath.Join(corpus, argument)
				}
				args = append(args, argument)
			}

			code, out, errOut := runCLI(args...)
			if code != example.Exit {
				t.Fatalf("%s: exit %d, wanted %d\n%s%s", example.Outcome, code, example.Exit, out, errOut)
			}
			for _, fragment := range example.Contains {
				if !strings.Contains(out, fragment) {
					t.Errorf("%s: missing %q in:\n%s", example.Outcome, fragment, out)
				}
			}
		})
	}
}

// Every example must be described in the README, and every corpus file used by
// at least one example: documentation and corpus drift apart otherwise.
func TestTheCorpusAndTheReadmeAgree(t *testing.T) {
	corpus := filepath.Join("..", "..", "examples")
	readme, err := os.ReadFile(filepath.Join(corpus, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := os.ReadFile(filepath.Join(corpus, "examples.json"))
	if err != nil {
		t.Fatal(err)
	}

	var document struct {
		Examples []struct {
			Outcome string   `json:"outcome"`
			Args    []string `json:"args"`
		} `json:"examples"`
	}
	if err := json.Unmarshal(manifest, &document); err != nil {
		t.Fatal(err)
	}

	used := map[string]bool{}
	for _, example := range document.Examples {
		if !strings.Contains(string(readme), example.Outcome) {
			t.Errorf("the README does not describe the outcome %q", example.Outcome)
		}
		for _, argument := range example.Args {
			if strings.HasSuffix(argument, ".dot") || strings.HasSuffix(argument, ".json") {
				used[argument] = true
			}
		}
	}

	entries, err := os.ReadDir(corpus)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if name == "examples.json" || entry.IsDir() ||
			(!strings.HasSuffix(name, ".dot") && !strings.HasSuffix(name, ".json")) {
			continue
		}
		if !used[name] {
			t.Errorf("%s sits in the corpus and no example uses it", name)
		}
	}
}
