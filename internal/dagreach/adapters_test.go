package dagreach

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	terraformFixture = "testdata/terraform.dot"
	manifestFixture  = "testdata/dbt-manifest.json"
	sbomFixture      = "testdata/sbom.cdx.json"
)

func load(t *testing.T, path string, options LoadOptions) *Graph {
	t.Helper()
	graph, err := LoadGraph(path, options)
	if err != nil {
		t.Fatalf("load %s: %v", path, err)
	}
	return graph
}

func fixtureText(t *testing.T, path string) string {
	t.Helper()
	text, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(text)
}

func TestEveryProfileDeclaresWhatItReadsAndWhichWay(t *testing.T) {
	for _, profile := range Profiles() {
		if profile.ProducedBy == "" || profile.Summary == "" {
			t.Errorf("%s: incomplete description", profile.Name)
		}
		if !knownSemantics(profile.EdgeSemantics) {
			t.Errorf("%s: semantics = %q", profile.Name, profile.EdgeSemantics)
		}
	}
}

func TestProducersAreRecognisedFromTheFile(t *testing.T) {
	cases := map[string]string{
		terraformFixture: "terraform",
		manifestFixture:  "dbt",
		sbomFixture:      "cyclonedx",
	}
	for path, want := range cases {
		profile := DetectProfile(fixtureText(t, path))
		if profile == nil || profile.Name != want {
			t.Errorf("%s detected as %v, wanted %s", path, profile, want)
		}
	}
}

func TestAnOrdinaryGraphIsRecognisedByNobody(t *testing.T) {
	if DetectProfile("digraph { a -> b }") != nil {
		t.Error("a plain graph should match no profile")
	}
	generic, _ := GetProfile("generic")
	if generic.Detect("digraph { a -> b }") {
		t.Error("generic must never claim a file")
	}
}

func TestDetectionIsAnnouncedRatherThanSilent(t *testing.T) {
	graph := load(t, terraformFixture, LoadOptions{})
	if graph.Profile != "terraform" || !hasWarning(graph, "recognised from the file itself") {
		t.Errorf("profile=%q warnings=%v", graph.Profile, graph.Warnings)
	}
}

func TestAnExplicitProfileIsNotAnnounced(t *testing.T) {
	graph := load(t, terraformFixture, LoadOptions{Profile: "terraform"})
	if hasWarning(graph, "recognised from the file") {
		t.Errorf("warnings = %v", graph.Warnings)
	}
}

func TestAnExplicitSemanticsOverridesTheProfileAndSaysSo(t *testing.T) {
	graph := load(t, terraformFixture, LoadOptions{Profile: "terraform", EdgeSemantics: "feeds"})
	if graph.EdgeSemantics != "feeds" || !hasWarning(graph, "overrides the terraform profile") {
		t.Errorf("semantics=%q warnings=%v", graph.EdgeSemantics, graph.Warnings)
	}
}

func TestFormatIsIgnoredByAProfileThatKnowsBetter(t *testing.T) {
	graph := load(t, manifestFixture, LoadOptions{Profile: "dbt", Format: "jgf"})
	if !hasWarning(graph, "--format jgf was ignored") {
		t.Errorf("warnings = %v", graph.Warnings)
	}
}

func TestTerraformIdentifiersLoseTheirDecoration(t *testing.T) {
	cases := map[string]string{
		"[root] aws_vpc.main (expand)":                           "aws_vpc.main",
		`[root] provider["registry.terraform.io/hashicorp/aws"]`: `provider["registry.terraform.io/hashicorp/aws"]`,
		"[root] module.net.aws_subnet.a (close)":                 "module.net.aws_subnet.a",
		"aws_vpc.main":                                           "aws_vpc.main",
	}
	for raw, want := range cases {
		if got := NormaliseTerraform(raw); got != want {
			t.Errorf("normalise(%q) = %q, wanted %q", raw, got, want)
		}
	}
}

func TestTerraformGroupsByResourceKind(t *testing.T) {
	cases := map[string]string{
		"aws_vpc.main":        "aws_vpc",
		"data.aws_ami.ubuntu": "data",
		`provider["registry.terraform.io/hashicorp/aws"]`: "provider",
	}
	for node, want := range cases {
		if got := TerraformKind(node); got != want {
			t.Errorf("kind(%q) = %q, wanted %q", node, got, want)
		}
	}
}

func TestTerraformImpactRunsTheRightWayWithoutAnyFlag(t *testing.T) {
	graph := load(t, terraformFixture, LoadOptions{Profile: "terraform"})
	if graph.EdgeSemantics != "depends-on" {
		t.Fatalf("semantics = %q", graph.EdgeSemantics)
	}
	report := Impact(graph, []string{"aws_vpc.main"})
	assertEqual(t, report.Downstream,
		[]string{"aws_instance.web", "aws_security_group.web", "aws_subnet.main"},
		"what depends on the VPC")
	if got := graph.Node("aws_vpc.main").Attrs["terraform_id"]; got != "[root] aws_vpc.main (expand)" {
		t.Errorf("terraform_id = %q", got)
	}
	if got := graph.Node("aws_vpc.main").Attrs[GroupKey]; got != "aws_vpc" {
		t.Errorf("group = %q", got)
	}
}

func TestTerraformKeepsFullIdentifiersWhenStrippingWouldCollide(t *testing.T) {
	path := filepath.Join(t.TempDir(), "collide.dot")
	os.WriteFile(path, []byte(`digraph { compound = "true"
		newrank = "true"
		"[root] aws_vpc.main (expand)" -> "[root] aws_vpc.main (close)" }`), 0o600)

	graph := load(t, path, LoadOptions{Profile: "terraform"})
	if !graph.HasNode("[root] aws_vpc.main (expand)") || !hasWarning(graph, "would collide") {
		t.Errorf("nodes=%v warnings=%v", graph.Nodes(), graph.Warnings)
	}
}

func TestDBTReadsModelsSourcesTestsAndExposures(t *testing.T) {
	graph := load(t, manifestFixture, LoadOptions{Profile: "dbt"})
	if graph.Name != "acme_analytics" || graph.EdgeSemantics != "feeds" || graph.NodeCount() != 5 {
		t.Fatalf("name=%q semantics=%q nodes=%d", graph.Name, graph.EdgeSemantics, graph.NodeCount())
	}
	if graph.Attrs["dbt_version"] != "1.9.2" {
		t.Errorf("attrs = %v", graph.Attrs)
	}
	if got := graph.Node("source.acme_analytics.shop.orders").Attrs[GroupKey]; got != "source" {
		t.Errorf("source group = %q", got)
	}
	model := graph.Node("model.acme_analytics.fct_orders")
	if model.Attrs[GroupKey] != "model" || model.Attrs["materialized"] != "table" ||
		model.Attrs["tags"] != "marts,production" {
		t.Errorf("model attrs = %v", model.Attrs)
	}
}

func TestDBTImpactRunsDownstreamOfASource(t *testing.T) {
	graph := load(t, manifestFixture, LoadOptions{Profile: "dbt"})
	report := Impact(graph, []string{"source.acme_analytics.shop.orders"})
	assertEqual(t, report.Downstream, []string{
		"model.acme_analytics.stg_orders",
		"model.acme_analytics.fct_orders",
		"test.acme_analytics.not_null_fct_orders_id",
		"exposure.acme_analytics.revenue_dashboard",
	}, "downstream of a source")
}

func TestDBTFallsBackToDependsOnWhenTheManifestHasNoChildMap(t *testing.T) {
	text := fixtureText(t, manifestFixture)
	start := strings.Index(text, `  "child_map"`)
	trimmed := text[:start] + "  \"unused\": {}\n}\n"
	path := filepath.Join(t.TempDir(), "manifest.json")
	os.WriteFile(path, []byte(trimmed), 0o600)

	graph := load(t, path, LoadOptions{Profile: "dbt"})
	if !hasWarning(graph, "depends_on.nodes") {
		t.Fatalf("warnings = %v", graph.Warnings)
	}
	report := Impact(graph, []string{"model.acme_analytics.stg_orders"})
	found := false
	for _, node := range report.Downstream {
		if node == "model.acme_analytics.fct_orders" {
			found = true
		}
	}
	if !found {
		t.Errorf("downstream = %v", report.Downstream)
	}
}

func TestCycloneDXReadsComponentsAndDependencies(t *testing.T) {
	graph := load(t, sbomFixture, LoadOptions{Profile: "cyclonedx"})
	if graph.Name != "checkout-service" || graph.EdgeSemantics != "depends-on" || graph.NodeCount() != 4 {
		t.Fatalf("name=%q semantics=%q nodes=%d", graph.Name, graph.EdgeSemantics, graph.NodeCount())
	}
	if graph.Attrs["specVersion"] != "1.6" {
		t.Errorf("attrs = %v", graph.Attrs)
	}
	qs := graph.Node("pkg:npm/qs@6.11.0")
	if qs.Attrs[GroupKey] != "library" || qs.Attrs["version"] != "6.11.0" ||
		qs.Attrs["licenses"] != "BSD-3-Clause" {
		t.Errorf("qs attrs = %v", qs.Attrs)
	}
	if got := graph.Node("pkg:npm/checkout-service@2.4.0").Attrs[GroupKey]; got != "root" {
		t.Errorf("root group = %q", got)
	}
}

func TestCycloneDXAnswersTheSupplyChainQuestion(t *testing.T) {
	graph := load(t, sbomFixture, LoadOptions{Profile: "cyclonedx"})
	report := Impact(graph, []string{"pkg:npm/qs@6.11.0"})
	assertEqual(t, report.Downstream, []string{
		"pkg:npm/checkout-service@2.4.0",
		"pkg:npm/express@4.19.2",
		"pkg:npm/body-parser@1.20.2",
	}, "what depends on a vulnerable library")
}

func TestAnSBOMWithoutRelationshipsIsReadAndFlagged(t *testing.T) {
	path := filepath.Join(t.TempDir(), "flat.cdx.json")
	os.WriteFile(path, []byte(`{"bomFormat": "CycloneDX", "specVersion": "1.6",
		"components": [{"bom-ref": "a", "name": "a", "type": "library"}]}`), 0o600)

	graph := load(t, path, LoadOptions{Profile: "cyclonedx"})
	if graph.NodeCount() != 1 || !hasWarning(graph, "no 'dependencies' array") {
		t.Errorf("nodes=%d warnings=%v", graph.NodeCount(), graph.Warnings)
	}
}

func TestADependencyExportReadAsFeedsRaisesAWarning(t *testing.T) {
	graph := load(t, terraformFixture, LoadOptions{Profile: "generic"})
	if !hasWarning(graph, "terraform graph") || !hasWarning(graph, "--edge-semantics depends-on") {
		t.Errorf("warnings = %v", graph.Warnings)
	}
}

func TestDeclaringTheSemanticsSilencesTheWarning(t *testing.T) {
	graph := load(t, terraformFixture, LoadOptions{Profile: "generic", EdgeSemantics: "depends-on"})
	if hasWarning(graph, "terraform graph") {
		t.Errorf("warnings = %v", graph.Warnings)
	}
}

func TestOrientReversesOnceAtTheDoor(t *testing.T) {
	graph := Orient(mustParseDOT(t, "digraph { a -> b }"), "depends-on")
	assertEqual(t, edgeKeys(graph), []string{"b->a"}, "reversed")
	if graph.EdgeSemantics != "depends-on" {
		t.Errorf("semantics = %q", graph.EdgeSemantics)
	}
}
