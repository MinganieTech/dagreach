package dagreach

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"
)

// -update rewrites the golden files. Run it, read the diff, and only then commit:
// the point of these files is that a change to the JSON shape has to be noticed
// by a human, since other tools depend on it.
var update = flag.Bool("update", false, "rewrite the golden JSON reports")

// The JSON report is a public contract, documented in docs/json-report.md. These
// cases pin its shape: every command, with and without policies, so an accidental
// rename or a dropped field fails CI instead of a consumer's pipeline.
var goldenCases = []struct {
	name string
	args []string
}{
	{"parse-terraform", []string{"parse", "testdata/terraform.dot"}},
	{"parse-messy", []string{"parse", "testdata/messy.dot"}},
	{"stats-pipeline", []string{"stats", "testdata/pipeline.jgf.json"}},
	{"stats-cyclic", []string{"stats", "testdata/cyclic.dot", "--fail-on", "cycle"}},
	{"impact-terraform", []string{"impact", "testdata/terraform.dot", "--changed", "aws_vpc.main"}},
	{"impact-sbom-policy", []string{
		"impact", "testdata/sbom.cdx.json", "--changed", "pkg:npm/qs@6.11.0",
		"--explain", "--fail-if-reaches", "group=root", "--max-impacted", "2",
	}},
	{"impact-dbt", []string{
		"impact", "testdata/dbt-manifest.json",
		"--changed", "source.acme_analytics.shop.orders", "--explain",
	}},
	{"diff-new-route", []string{
		"diff", "testdata/gate-before.dot", "testdata/gate-after.dot",
		"--changed", "auth", "--explain", "--fail-on-new-reach", "group=production",
	}},
	{"diff-all-pairs", []string{
		"diff", "testdata/gate-before.dot", "testdata/gate-after.dot",
		"--all-pairs-reachability-delta",
	}},
}

func TestJSONReportsMatchTheirGoldenFiles(t *testing.T) {
	for _, testCase := range goldenCases {
		t.Run(testCase.name, func(t *testing.T) {
			_, out, errOut := runCLI(append(testCase.args, "--json")...)
			if errOut != "" {
				t.Fatalf("stderr: %s", errOut)
			}

			path := filepath.Join("testdata", "golden", testCase.name+".json")
			if *update {
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(out), 0o600); err != nil {
					t.Fatal(err)
				}
				return
			}

			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("%v (run: go test ./internal/dagreach -run Golden -update)", err)
			}
			if out != string(want) {
				t.Errorf("the JSON report changed shape.\n"+
					"If that was intended, rerun with -update, read the diff, and say why in the\n"+
					"commit message - and bump SchemaVersion if a consumer could break.\n\n"+
					"got:\n%s\nwant:\n%s", out, want)
			}
		})
	}
}

func TestEveryReportCarriesItsSchemaVersion(t *testing.T) {
	for _, testCase := range goldenCases {
		_, out, _ := runCLI(append(testCase.args, "--json")...)
		var document map[string]any
		if err := json.Unmarshal([]byte(out), &document); err != nil {
			t.Fatalf("%s: %v", testCase.name, err)
		}
		if version, ok := document["schema_version"].(float64); !ok || int(version) != SchemaVersion {
			t.Errorf("%s: schema_version = %v, wanted %d",
				testCase.name, document["schema_version"], SchemaVersion)
		}
	}
}
