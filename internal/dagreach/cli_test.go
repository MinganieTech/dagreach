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
	// The headline is the text report's first line, escaped for markdown and
	// otherwise untouched, so the two modes cannot disagree about what happened.
	_, text, _ := runCLI("impact", path, "--changed", "auth",
		"--fail-if-reaches", "group=production", "--explain")
	headline := strings.SplitN(strings.ReplaceAll(text, "\r\n", "\n"), "\n", 2)[0]
	if !strings.Contains(out, mdText(headline)) {
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

func TestAnUndeterminablePolicyBlocksWithItsOwnCode(t *testing.T) {
	path := write(t, "risky.dot", risky)
	code, out, _ := runCLI("impact", path, "--changed", "spec",
		"--fail-if-reaches", "attr:environment=production")
	if code != ExitPolicyUnknown {
		t.Fatalf("code = %d, wanted %d", code, ExitPolicyUnknown)
	}
	if !strings.Contains(out, "UNKNOWN fail-if-reaches attr:environment=production") ||
		!strings.Contains(out, "cannot be settled by this file") {
		t.Errorf("out = %q", out)
	}
}

func TestSelectingOnAnyAttributeFromTheCommandLine(t *testing.T) {
	path := write(t, "risky.dot", risky)
	code, out, _ := runCLI("impact", path, "--changed", "spec",
		"--fail-if-reaches", "attr:risk=critical", "--explain")
	if code != ExitPolicyFailed {
		t.Fatalf("code = %d", code)
	}
	if !strings.Contains(out, "FAIL fail-if-reaches attr:risk=critical") ||
		!strings.Contains(out, "api: spec -> schema -> api") {
		t.Errorf("out = %q", out)
	}
}

func TestATypedSelectorKeyIsAUsageErrorNotASilentPass(t *testing.T) {
	path := write(t, "risky.dot", risky)
	code, _, errOut := runCLI("impact", path, "--changed", "spec", "--fail-if-reaches", "risk=critical")
	if code != ExitUsage || !strings.Contains(errOut, "attr:risk=critical") {
		t.Errorf("code=%d err=%q", code, errOut)
	}
}

func TestASelectorNamingAMissingNodeIsAnInputError(t *testing.T) {
	path := write(t, "risky.dot", risky)
	code, _, errOut := runCLI("impact", path, "--changed", "spec", "--fail-if-reaches", "node=ap")
	if code != ExitInputError || !strings.Contains(errOut, "no node 'ap'") ||
		!strings.Contains(errOut, "did you mean") {
		t.Errorf("code=%d err=%q", code, errOut)
	}
}

func TestMarkdownCarriesTheThirdVerdict(t *testing.T) {
	path := write(t, "risky.dot", risky)
	_, out, _ := runCLI("impact", path, "--changed", "spec",
		"--fail-if-reaches", "attr:environment=production", "--markdown")
	if !strings.Contains(out, "| **UNKNOWN** | `fail-if-reaches attr:environment=production` |") {
		t.Errorf("out = %q", out)
	}
}

func TestStatsRanksTheNodesWithTheMostBehindThem(t *testing.T) {
	path := write(t, "spine.dot", "digraph { a -> b -> c -> d; b -> e }")
	code, out, _ := runCLI("stats", path, "--limit", "2")
	if code != ExitOK {
		t.Fatalf("code = %d", code)
	}
	for _, fragment := range []string{
		"most reaching (3 of 5 nodes reach anything):",
		"  a reaches 4 (80%)",
		"  b reaches 3 (60%)",
		"  (+1 more)",
	} {
		if !strings.Contains(out, fragment) {
			t.Errorf("missing %q in:\n%s", fragment, out)
		}
	}
	if strings.Contains(out, "c reaches") {
		t.Error("--limit 2 shows two entries")
	}
}

func TestStatsSaysNothingAboutRankingWhenNothingReachesAnything(t *testing.T) {
	path := write(t, "dust.dot", "digraph { a; b }")
	_, out, _ := runCLI("stats", path)
	if strings.Contains(out, "most reaching") {
		t.Errorf("an edgeless graph has no ranking to report:\n%s", out)
	}
}

// -- the HTML report -------------------------------------------------------

func htmlOf(t *testing.T, args ...string) string {
	t.Helper()
	if !contains(args, "--html") {
		args = append(args, "--html")
	}
	code, out, errOut := runCLI(args...)
	if code == ExitUsage || code == ExitInputError {
		t.Fatalf("code = %d: %s", code, errOut)
	}
	if !strings.HasPrefix(out, "<!doctype html>") {
		t.Fatalf("not an HTML document:\n%s", out)
	}
	return out
}

func TestHTMLIsSelfContained(t *testing.T) {
	out := htmlOf(t, "stats", "testdata/terraform.dot")
	// A report that fetches anything is not an artifact you can keep, and a
	// report that runs anything is not one you can safely open.
	for _, forbidden := range []string{"<script", "http://", "https://", "src=", "@import", "url("} {
		if strings.Contains(out, forbidden) {
			t.Errorf("the page reaches outside itself: %q", forbidden)
		}
	}
}

func TestHTMLEscapesWhatCameFromTheFile(t *testing.T) {
	path := write(t, "hostile.dot", `digraph { "<script>alert(1)</script>" -> "b&c" }`)
	out := htmlOf(t, "impact", path, "--changed", "<script>alert(1)</script>")
	if strings.Contains(out, "<script>") {
		t.Fatal("a node identifier was written to the page as markup")
	}
	for _, want := range []string{"&lt;script&gt;alert(1)&lt;/script&gt;", "b&amp;c"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q", want)
		}
	}
}

func TestHTMLIsByteIdenticalBetweenRuns(t *testing.T) {
	// No date and no generated identifiers, so a report can be diffed against
	// the last one and the difference is the graph.
	args := []string{"impact", "testdata/terraform.dot", "--changed", "aws_vpc.main", "--html"}
	_, first, _ := runCLI(args...)
	_, second, _ := runCLI(args...)
	if first != second {
		t.Error("two runs on the same input produced different pages")
	}
}

func TestHTMLShowsTheVerdictAndThePathThatProvesIt(t *testing.T) {
	out := htmlOf(t, "impact", "testdata/terraform.dot", "--changed", "aws_vpc.main",
		"--fail-if-reaches", "group=aws_instance", "--html")
	for _, fragment := range []string{
		`<p class="verdict fail">FAIL - at least one policy was violated.</p>`,
		`<span class="chip fail">FAIL</span>`,
		"aws_vpc.main &rarr; aws_security_group.web &rarr; aws_instance.web",
	} {
		if !strings.Contains(out, fragment) {
			t.Errorf("missing %q in:\n%s", fragment, out)
		}
	}
	if strings.Contains(out, "policies:") {
		t.Error("the text policy block is repeated below the cards that replaced it")
	}
}

func TestHTMLSaysWhenAPolicyCouldNotBeSettled(t *testing.T) {
	out := htmlOf(t, "impact", "testdata/terraform.dot", "--changed", "aws_vpc.main",
		"--fail-if-reaches", "attr:severity=critical", "--html")
	if !strings.Contains(out, `<p class="verdict unknown">`) {
		t.Errorf("the third verdict has no banner:\n%s", out)
	}
}

func TestHTMLSaysHowToProduceItAgain(t *testing.T) {
	out := htmlOf(t, "stats", "testdata/terraform.dot", "--limit", "3", "--html")
	if !strings.Contains(out, "<code>dagreach stats testdata/terraform.dot --limit 3 --html</code>") {
		t.Errorf("the page does not carry its own command:\n%s", out)
	}
}

func TestTwoOutputModesAreRefusedByName(t *testing.T) {
	code, _, errOut := runCLI("stats", "testdata/terraform.dot", "--json", "--html")
	if code != ExitUsage {
		t.Fatalf("code = %d", code)
	}
	if !strings.Contains(errOut, "--json and --html") {
		t.Errorf("the error does not name the two that collided: %s", errOut)
	}
}

func TestMarkdownCannotBeForgedByANodeName(t *testing.T) {
	// Node identifiers are written by whoever opened the pull request. A comment
	// a contributor can forge is a gate a contributor can defeat.
	hostile := "digraph {\n" +
		"  \"a|b\" [group = \"production\"]\n" +
		"  \"tick`|**bold**\" -> \"a|b\"\n" +
		"}"
	path := write(t, "forged.dot", hostile)
	code, out, _ := runCLI("impact", path, "--changed", "tick`|**bold**",
		"--fail-if-reaches", "group=production", "--markdown")
	if code != ExitPolicyFailed {
		t.Fatalf("code = %d: %s", code, out)
	}

	table := []string{}
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "|") {
			table = append(table, line)
		}
	}
	for _, row := range table {
		if cells := strings.Count(row, "|") - strings.Count(row, "\\|"); cells != 4 {
			t.Errorf("a node identifier changed the shape of the table: %q", row)
		}
	}
	if strings.Contains(out, "**bold**") && !strings.Contains(out, "\\*\\*bold\\*\\*") {
		t.Error("a node identifier is rendered as markup rather than as its own name")
	}
}

func TestTheFullReportBlockCannotBeClosedEarly(t *testing.T) {
	path := write(t, "fence.dot", "digraph { \"``` heading\" -> b }")
	_, out, _ := runCLI("impact", path, "--changed", "``` heading", "--markdown")
	opening := ""
	for _, line := range strings.Split(out, "\n") {
		if strings.HasSuffix(strings.TrimSpace(line), "text") && strings.HasPrefix(line, "`") {
			opening = strings.TrimSuffix(strings.TrimSpace(line), "text")
			break
		}
	}
	if len(opening) < 4 {
		t.Fatalf("fence = %q, it must outgrow the backticks the report carries:\n%s", opening, out)
	}
}

// -- flags a command does not read, and graphs that cannot be compared -------

func TestAPolicyACommandCannotRunIsRefused(t *testing.T) {
	// The failure this prevents: the command used to accept the flag, never
	// evaluate it, and exit 0 - a gate that passes because nobody ran it.
	cases := []struct{ args []string }{
		{[]string{"diff", "testdata/gate-before.dot", "testdata/gate-after.dot",
			"--changed", "auth", "--fail-if-reaches", "group=production"}},
		{[]string{"impact", "testdata/pipeline.jgf.json", "--changed", "extract_orders",
			"--fail-on-new-reach", "group=production"}},
		{[]string{"stats", "testdata/pipeline.jgf.json", "--max-impacted", "1"}},
		{[]string{"parse", "testdata/pipeline.jgf.json", "--fail-if-reaches", "group=x"}},
		{[]string{"parse", "testdata/pipeline.jgf.json", "--limit", "2"}},
		{[]string{"stats", "testdata/pipeline.jgf.json", "--explain"}},
		{[]string{"diff", "testdata/gate-before.dot", "testdata/gate-after.dot",
			"--changed", "auth", "--fail-on", "cycle"}},
	}
	for _, test := range cases {
		code, out, errOut := runCLI(test.args...)
		if code != ExitUsage {
			t.Errorf("%v -> code %d, wanted a usage error\n%s%s", test.args, code, out, errOut)
			continue
		}
		if !strings.Contains(errOut, "does not read") {
			t.Errorf("%v -> %q", test.args, errOut)
		}
	}
}

func TestCountOnlyNeedsTheComparisonItShortens(t *testing.T) {
	code, _, errOut := runCLI("diff", "testdata/gate-before.dot", "testdata/gate-after.dot",
		"--changed", "auth", "--count-only")
	if code != ExitUsage || !strings.Contains(errOut, "--all-pairs-reachability-delta") {
		t.Errorf("code=%d err=%q", code, errOut)
	}
}

func TestTheFlagsACommandDoesReadStillWork(t *testing.T) {
	// The guard must refuse the useless, not the useful. Between them these
	// invocations exercise every flag the parser knows at least once, so a name
	// misspelled in commandFlags shows up here rather than in somebody's CI.
	for _, args := range [][]string{
		{"parse", "testdata/terraform.dot", "--profile", "terraform", "--json"},
		{"parse", "testdata/gate-before.dot", "--format", "dot", "--edge-semantics", "feeds"},
		{"parse", "testdata/pipeline.jgf.json", "--markdown"},
		{"stats", "testdata/pipeline.jgf.json", "--limit", "2", "--fail-on", "cycle", "--html"},
		{"impact", "testdata/pipeline.jgf.json", "--changed", "extract_orders",
			"--explain", "--limit", "3", "--max-impacted", "99", "--fail-on", "cycle",
			"--fail-if-reaches", "group=transform"},
		{"diff", "testdata/gate-before.dot", "testdata/gate-after.dot", "--changed", "auth",
			"--explain", "--fail-on-new-reach", "group=production"},
		{"diff", "testdata/gate-before.dot", "testdata/gate-after.dot",
			"--all-pairs-reachability-delta", "--count-only"},
	} {
		if code, _, errOut := runCLI(args...); code == ExitUsage {
			t.Errorf("%v was refused: %s", args, errOut)
		}
	}
}

func TestTwoGraphsReadDifferentlyAreNotCompared(t *testing.T) {
	// One side detected as a dependency export and the other read as `feeds`
	// means the two are oriented opposite ways, and every delta is backwards.
	code, out, errOut := runCLI("diff", "testdata/terraform.dot", "testdata/gate-after.dot",
		"--changed", "auth")
	if code != ExitInputError {
		t.Fatalf("code = %d\n%s%s", code, out, errOut)
	}
	for _, fragment := range []string{
		"do not have the same profile", "terraform against generic",
		"pass --profile to read both files the same way",
	} {
		if !strings.Contains(errOut, fragment) {
			t.Errorf("missing %q in %q", fragment, errOut)
		}
	}
}

func TestSettlingBothFilesMakesTheComparisonPossibleAgain(t *testing.T) {
	// The refusal has to have a way out, or it is just a wall.
	code, _, errOut := runCLI("diff", "testdata/gate-before.dot", "testdata/gate-after.dot",
		"--changed", "auth", "--edge-semantics", "depends-on")
	if code == ExitInputError {
		t.Errorf("an explicit semantics settles both files: %s", errOut)
	}
}
