package dagreach

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode"
)

func runCLI(args ...string) (int, string, string) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	code := Run(args, stdout, stderr, strings.NewReader(""))
	return code, stdout.String(), stderr.String()
}

func mustJSON(t *testing.T, text string) map[string]any {
	t.Helper()
	var document map[string]any
	if err := json.Unmarshal([]byte(text), &document); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, text)
	}
	return document
}

func write(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestNoArgumentsPrintsHelp(t *testing.T) {
	code, out, _ := runCLI()
	if code != ExitOK || !strings.Contains(out, "dagreach") {
		t.Errorf("code=%d out=%q", code, out)
	}
}

func TestVersionFlag(t *testing.T) {
	code, out, _ := runCLI("--version")
	if code != ExitOK || !strings.Contains(out, Version) {
		t.Errorf("code=%d out=%q", code, out)
	}
}

func TestParseReportsSizeAndProfile(t *testing.T) {
	code, out, _ := runCLI("parse", "testdata/pipeline.jgf.json")
	if code != ExitOK {
		t.Fatalf("code = %d", code)
	}
	for _, fragment := range []string{"jgf", "5 nodes, 4 edges", "durations on 4/5 nodes"} {
		if !strings.Contains(out, fragment) {
			t.Errorf("missing %q in:\n%s", fragment, out)
		}
	}
}

func TestParseJSONReportIsMachineReadable(t *testing.T) {
	code, out, _ := runCLI("parse", "testdata/messy.dot", "--json")
	if code != ExitOK {
		t.Fatalf("code = %d", code)
	}
	report := mustJSON(t, out)
	if report["format"] != "dot" || report["nodes"].(float64) != 8 || report["self_loops"].(float64) != 1 {
		t.Errorf("report = %v", report)
	}
	statuses := report["attributes"].(map[string]any)["statuses"].(map[string]any)
	if statuses["ready"].(float64) != 2 || statuses["blocked"].(float64) != 1 {
		t.Errorf("statuses = %v", statuses)
	}
}

func TestParseReportsABadFileWithoutATraceback(t *testing.T) {
	code, out, errOut := runCLI("parse", "testdata/missing.dot")
	if code != ExitInputError || out != "" || !strings.Contains(errOut, "no such file") {
		t.Errorf("code=%d out=%q err=%q", code, out, errOut)
	}
}

func TestParseReportsASyntaxErrorWithItsLocation(t *testing.T) {
	path := write(t, "broken.dot", "digraph {\n  a -> \n}\n")
	code, _, errOut := runCLI("parse", path)
	if code != ExitInputError || !strings.Contains(errOut, "broken.dot:3") {
		t.Errorf("code=%d err=%q", code, errOut)
	}
}

func TestStatsReportsTheShape(t *testing.T) {
	code, out, _ := runCLI("stats", "testdata/terraform.dot")
	if code != ExitOK {
		t.Fatalf("code = %d", code)
	}
	for _, fragment := range []string{
		"5 nodes, 5 edges, acyclic",
		"depth 4 level(s), width 2 (largest earliest-start generation)",
		"longest path:",
	} {
		if !strings.Contains(out, fragment) {
			t.Errorf("missing %q in:\n%s", fragment, out)
		}
	}
	if strings.Contains(out, "critical path") {
		t.Error("no durations were declared, so nothing is critical")
	}
}

func TestStatsOnACyclicGraphCollapsesAndSaysSo(t *testing.T) {
	path := write(t, "cyclic.dot", "digraph { a -> b -> c -> a; c -> d }")
	code, out, _ := runCLI("stats", path)
	if code != ExitOK {
		t.Fatalf("code = %d", code)
	}
	for _, fragment := range []string{
		"1 cycle(s)", "cycles are collapsed before measuring",
		"cycle: a, b, c", "measured on the condensed graph", "scc(a+2)",
	} {
		if !strings.Contains(out, fragment) {
			t.Errorf("missing %q in:\n%s", fragment, out)
		}
	}
}

func TestImpactReportsReachCostAndGroups(t *testing.T) {
	code, out, _ := runCLI("impact", "testdata/pipeline.jgf.json", "--changed", "transform_orders")
	if code != ExitOK {
		t.Fatalf("code = %d", code)
	}
	for _, fragment := range []string{
		"reaches 3 of 5 nodes (60%)",
		"downstream (2): load_warehouse, notify",
		"upstream (2)",
		"cost of the impacted set: 480 of declared duration",
		"groups touched: transform 1, load 2",
		"articulation point",
	} {
		if !strings.Contains(out, fragment) {
			t.Errorf("missing %q in:\n%s", fragment, out)
		}
	}
}

func TestImpactAcceptsRepeatedAndCommaSeparatedIDs(t *testing.T) {
	code, out, _ := runCLI("impact", "testdata/pipeline.jgf.json",
		"--changed", "extract_orders,extract_customers", "--changed", "extract_orders", "--json")
	if code != ExitOK {
		t.Fatalf("code = %d", code)
	}
	report := mustJSON(t, out)
	changed := report["changed"].([]any)
	if len(changed) != 2 || report["impacted_count"].(float64) != 5 {
		t.Errorf("changed=%v count=%v", changed, report["impacted_count"])
	}
}

func TestImpactOnAnUnknownNodeSuggestsACloseOne(t *testing.T) {
	code, _, errOut := runCLI("impact", "testdata/pipeline.jgf.json", "--changed", "extract_order")
	if code != ExitInputError {
		t.Fatalf("code = %d", code)
	}
	if !strings.Contains(errOut, "no node 'extract_order'") ||
		!strings.Contains(errOut, "did you mean 'extract_orders'") {
		t.Errorf("err = %q", errOut)
	}
}

func TestListsAreTruncatedInTextModeButSaySo(t *testing.T) {
	var built strings.Builder
	built.WriteString("digraph { ")
	for index := 0; index < 25; index++ {
		built.WriteString("root -> leaf" + string(rune('a'+index%26)) + string(rune('0'+index/10)) + ";")
	}
	built.WriteString(" }")
	path := write(t, "wide.dot", built.String())

	code, out, _ := runCLI("impact", path, "--changed", "root", "--limit", "5")
	if code != ExitOK || !strings.Contains(out, "more)") {
		t.Errorf("code=%d out=%q", code, out)
	}

	code, out, _ = runCLI("impact", path, "--changed", "root", "--limit", "0")
	if code != ExitOK || strings.Contains(out, "more)") {
		t.Errorf("limit 0 should show everything: %q", out)
	}

	code, out, _ = runCLI("impact", path, "--changed", "root", "--limit", "5", "--json")
	report := mustJSON(t, out)
	if len(report["downstream"].([]any)) != 25 {
		t.Error("the JSON report must never truncate")
	}
}

func TestExplainShowsAWitnessPathPerReachedNode(t *testing.T) {
	code, out, _ := runCLI("impact", "testdata/pipeline.jgf.json",
		"--changed", "extract_orders", "--explain")
	if code != ExitOK {
		t.Fatalf("code = %d", code)
	}
	for _, fragment := range []string{
		"why (3 of 3 shown):",
		"transform_orders (distance 1): extract_orders -> transform_orders",
		"notify (distance 3): extract_orders -> transform_orders -> load_warehouse -> notify",
	} {
		if !strings.Contains(out, fragment) {
			t.Errorf("missing %q in:\n%s", fragment, out)
		}
	}
}

func TestExplainIsAbsentFromJSONUnlessAsked(t *testing.T) {
	_, out, _ := runCLI("impact", "testdata/pipeline.jgf.json", "--changed", "extract_orders", "--json")
	if _, present := mustJSON(t, out)["explain"]; present {
		t.Error("explain should be opt-in")
	}
	_, out, _ = runCLI("impact", "testdata/pipeline.jgf.json",
		"--changed", "extract_orders", "--json", "--explain")
	explained := mustJSON(t, out)["explain"].(map[string]any)
	entry := explained["load_warehouse"].(map[string]any)
	if entry["distance"].(float64) != 2 || len(entry["path"].([]any)) != 3 {
		t.Errorf("explain = %v", entry)
	}
}

func TestAViolatedPolicyExitsOneAndExplains(t *testing.T) {
	path := write(t, "services.dot", services)
	code, out, _ := runCLI("impact", path, "--changed", "auth", "--fail-if-reaches", "group=production")
	if code != ExitPolicyFailed {
		t.Fatalf("code = %d", code)
	}
	if !strings.Contains(out, "FAIL fail-if-reaches group=production") ||
		!strings.Contains(out, "payments: auth -> token -> payments") {
		t.Errorf("out = %q", out)
	}
}

func TestASatisfiedPolicyExitsZero(t *testing.T) {
	path := write(t, "services.dot", services)
	code, out, _ := runCLI("impact", path, "--changed", "reporting", "--fail-if-reaches", "group=core")
	if code != ExitOK || !strings.Contains(out, "ok   fail-if-reaches group=core") {
		t.Errorf("code=%d out=%q", code, out)
	}
}

func TestABadSelectorIsAUsageErrorNotAPolicyFailure(t *testing.T) {
	path := write(t, "services.dot", services)
	code, _, errOut := runCLI("impact", path, "--changed", "auth", "--fail-if-reaches", "team=core")
	if code != ExitUsage || !strings.Contains(errOut, "unknown selector key") {
		t.Errorf("code=%d err=%q", code, errOut)
	}
}

func TestAnUnknownOptionIsAUsageError(t *testing.T) {
	code, _, errOut := runCLI("stats", "testdata/messy.dot", "--nope")
	if code != ExitUsage || !strings.Contains(errOut, "unknown option '--nope'") {
		t.Errorf("code=%d err=%q", code, errOut)
	}
}

func TestStatsFailOnCycle(t *testing.T) {
	path := write(t, "cyclic.dot", "digraph { a -> b -> a }")
	code, out, _ := runCLI("stats", path, "--fail-on", "cycle")
	if code != ExitPolicyFailed || !strings.Contains(out, "FAIL fail-on-cycle") {
		t.Errorf("code=%d out=%q", code, out)
	}
	if code, _, _ := runCLI("stats", "testdata/terraform.dot", "--fail-on", "cycle"); code != ExitOK {
		t.Errorf("an acyclic graph passes, code = %d", code)
	}
}

func TestDiffReportsTheDelta(t *testing.T) {
	before := write(t, "before.dot", beforeDOT)
	after := write(t, "after.dot", afterDOT)
	code, out, _ := runCLI("diff", before, after, "--changed", "auth")
	if code != ExitOK {
		t.Fatalf("code = %d", code)
	}
	for _, fragment := range []string{
		"reaches 3 nodes, was 2 (+1, -0)",
		"new reach (1): payments",
		"1 edge(s) added, 0 removed",
	} {
		if !strings.Contains(out, fragment) {
			t.Errorf("missing %q in:\n%s", fragment, out)
		}
	}
}

func TestFailOnNewReachExitsOneAndNamesTheEdge(t *testing.T) {
	before := write(t, "before.dot", beforeDOT)
	after := write(t, "after.dot", afterDOT)
	code, out, _ := runCLI("diff", before, after, "--changed", "auth", "--explain",
		"--fail-on-new-reach", "group=production")
	if code != ExitPolicyFailed {
		t.Fatalf("code = %d", code)
	}
	for _, fragment := range []string{
		"payments is now reached",
		"reason: new edge token -> payments",
		"path:   auth -> token -> payments",
		"FAIL fail-on-new-reach group=production",
	} {
		if !strings.Contains(out, fragment) {
			t.Errorf("missing %q in:\n%s", fragment, out)
		}
	}
}

func TestDiffNeedsChangedOrTheGlobalFlag(t *testing.T) {
	before := write(t, "before.dot", beforeDOT)
	code, _, errOut := runCLI("diff", before, before)
	if code != ExitUsage || !strings.Contains(errOut, "needs --changed") {
		t.Errorf("code=%d err=%q", code, errOut)
	}
}

func TestTheGlobalAnalysisIsOptInAndAggregated(t *testing.T) {
	before := write(t, "before.dot", beforeDOT)
	after := write(t, "after.dot", afterDOT)
	code, out, _ := runCLI("diff", before, after, "--all-pairs-reachability-delta")
	if code != ExitOK || !strings.Contains(out, "all-pairs reachability delta: +2 pair(s)") ||
		!strings.Contains(out, "by source, largest first:") {
		t.Errorf("code=%d out=%q", code, out)
	}

	code, out, _ = runCLI("diff", before, after, "--all-pairs-reachability-delta", "--count-only")
	if code != ExitOK || strings.Contains(out, "by source") {
		t.Errorf("--count-only should drop the ranking: %q", out)
	}
}

func TestProfilesAreListedWithTheirDirection(t *testing.T) {
	code, out, _ := runCLI("profiles")
	if code != ExitOK {
		t.Fatalf("code = %d", code)
	}
	for _, fragment := range []string{"terraform graph", "cyclonedx", "depends-on",
		"--edge-semantics overrides it"} {
		if !strings.Contains(out, fragment) {
			t.Errorf("missing %q in:\n%s", fragment, out)
		}
	}
}

func TestTheReportNamesTheProfileItApplied(t *testing.T) {
	_, out, _ := runCLI("parse", sbomFixture)
	if !strings.Contains(out, "edges: cyclonedx profile, source depends on target") {
		t.Errorf("out = %q", out)
	}
}

func TestTheProfileTravelsIntoTheJSONReport(t *testing.T) {
	_, out, _ := runCLI("stats", manifestFixture, "--json")
	if mustJSON(t, out)["profile"] != "dbt" {
		t.Errorf("profile missing from the report")
	}
}

func TestOutputStaysASCIIForLegacyConsoles(t *testing.T) {
	// A Windows console on a legacy code page turns anything else into '?'.
	cases := [][]string{
		{"parse", "testdata/messy.dot"},
		{"stats", "testdata/pipeline.jgf.json"},
		{"impact", "testdata/pipeline.jgf.json", "--changed", "transform_orders", "--explain"},
		{"profiles"},
	}
	for _, args := range cases {
		_, out, errOut := runCLI(args...)
		for _, text := range []string{out, errOut} {
			for _, char := range text {
				if char > unicode.MaxASCII {
					t.Errorf("%v: non-ASCII %q in output", args, char)
					break
				}
			}
		}
	}
}

func TestDegenerateGraphsStillGetAnAnswer(t *testing.T) {
	empty := write(t, "empty.dot", "digraph { }")
	code, out, _ := runCLI("stats", empty)
	if code != ExitOK || !strings.Contains(out, "0 nodes, 0 edges, acyclic") ||
		!strings.Contains(out, "depth 0 level(s), width 0") {
		t.Errorf("an empty graph should answer plainly: code=%d out=%q", code, out)
	}

	loop := write(t, "loop.dot", "digraph { a -> a }")
	code, out, _ = runCLI("stats", loop)
	if code != ExitOK || !strings.Contains(out, "1 cycle(s)") ||
		!strings.Contains(out, "measured on the condensed graph") {
		t.Errorf("a self-loop is a cycle: code=%d out=%q", code, out)
	}
}

func TestMarkdownIsWhatTheActionPosts(t *testing.T) {
	path := write(t, "services.dot", services)
	code, out, _ := runCLI("impact", path, "--changed", "auth",
		"--fail-if-reaches", "group=production", "--explain", "--markdown")
	if code != ExitPolicyFailed {
		t.Fatalf("code = %d", code)
	}
	for _, fragment := range []string{
		"### dagreach",
		"| verdict | policy | detail |",
		"| **FAIL** | `fail-if-reaches group=production` |",
		"- `payments` &larr; `auth -> token -> payments`",
		"<details>",
		"```text",
	} {
		if !strings.Contains(out, fragment) {
			t.Errorf("missing %q in:\n%s", fragment, out)
		}
	}
	// The headline is the text report's first line, so the two cannot disagree.
	_, text, _ := runCLI("impact", path, "--changed", "auth",
		"--fail-if-reaches", "group=production", "--explain")
	headline := strings.SplitN(text, "\n", 2)[0]
	if !strings.Contains(out, headline) {
		t.Errorf("markdown headline differs from the text report:\n%s", headline)
	}
}

func TestMarkdownWithoutPoliciesHasNoVerdictTable(t *testing.T) {
	code, out, _ := runCLI("stats", "testdata/terraform.dot", "--markdown")
	if code != ExitOK || strings.Contains(out, "| verdict |") {
		t.Errorf("code=%d out=%q", code, out)
	}
	if !strings.Contains(out, "### dagreach") || !strings.Contains(out, "<details>") {
		t.Errorf("out = %q", out)
	}
}

func TestJSONAndMarkdownAreMutuallyExclusive(t *testing.T) {
	code, _, errOut := runCLI("stats", "testdata/terraform.dot", "--json", "--markdown")
	if code != ExitUsage || !strings.Contains(errOut, "pick one") {
		t.Errorf("code=%d err=%q", code, errOut)
	}
}

func TestMarkdownStaysASCII(t *testing.T) {
	path := write(t, "services.dot", services)
	_, out, _ := runCLI("impact", path, "--changed", "auth",
		"--fail-if-reaches", "group=production", "--markdown")
	for _, char := range out {
		if char > unicode.MaxASCII {
			t.Fatalf("non-ASCII %q in the markdown report", char)
		}
	}
}
