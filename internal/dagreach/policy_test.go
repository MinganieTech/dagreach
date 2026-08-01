package dagreach

import (
	"strings"
	"testing"
)

const services = `
digraph {
	auth [group = "core"]
	token [group = "core"]
	payments [group = "production", status = "critical"]
	reporting [group = "analytics"]
	auth -> token -> payments
	reporting -> payments
}`

func TestSelectorsParse(t *testing.T) {
	for _, text := range []string{"group=production", "node=auth", "status=critical"} {
		selector, err := ParseSelector(text)
		if err != nil || selector.String() != text {
			t.Errorf("%q -> %v, %v", text, selector, err)
		}
	}
}

func TestBadSelectorsSayWhatWasExpected(t *testing.T) {
	for _, text := range []string{"production", "team=", "=production", "owner=me"} {
		_, err := ParseSelector(text)
		if err == nil || !strings.Contains(err.Error(), "group") {
			t.Errorf("%q -> %v", text, err)
		}
		if _, isUsage := err.(*UsageError); !isUsage {
			t.Errorf("%q: expected a UsageError, got %T", text, err)
		}
	}
}

func TestSelectorMatchesGroupStatusAndNode(t *testing.T) {
	graph := mustParseDOT(t, services)
	assertEqual(t, Selector{Key: "group", Value: "core"}.Select(graph), []string{"auth", "token"}, "group")
	assertEqual(t, Selector{Key: "status", Value: "critical"}.Select(graph), []string{"payments"}, "status")
	assertEqual(t, Selector{Key: "node", Value: "reporting"}.Select(graph), []string{"reporting"}, "node")
}

func TestFailIfReachesCarriesThePathThatProvesIt(t *testing.T) {
	graph := mustParseDOT(t, services)
	result := FailIfReaches(graph, Impact(graph, []string{"auth"}), Selector{Key: "group", Value: "production"}, 3)
	if !result.Failed() {
		t.Fatal("the change reaches production")
	}
	assertEqual(t, result.Matched, []string{"payments"}, "matched")
	assertEqual(t, result.Witnesses["payments"], []string{"auth", "token", "payments"}, "witness")
}

func TestFailIfReachesPassesWhenNothingMatches(t *testing.T) {
	graph := mustParseDOT(t, services)
	result := FailIfReaches(graph, Impact(graph, []string{"reporting"}), Selector{Key: "group", Value: "core"}, 3)
	if result.Failed() || !strings.Contains(result.Detail, "nothing matching group=core is reached") {
		t.Errorf("result = %+v", result)
	}
}

func TestAChangedNodeThatIsItselfATargetIsFlaggedAsSuch(t *testing.T) {
	graph := mustParseDOT(t, services)
	result := FailIfReaches(graph, Impact(graph, []string{"payments"}), Selector{Key: "group", Value: "production"}, 3)
	if !result.Failed() || !strings.Contains(result.Detail, "they are the changed nodes themselves") {
		t.Errorf("detail = %q", result.Detail)
	}
}

func TestMaxImpactedComparesAgainstTheCeiling(t *testing.T) {
	graph := mustParseDOT(t, services)
	report := Impact(graph, []string{"auth"})
	if !MaxImpacted(report, 2).Failed() || MaxImpacted(report, 3).Failed() {
		t.Error("the ceiling is compared the wrong way")
	}
}

func TestFailOnCycleListsTheCycles(t *testing.T) {
	stats := Analyse(mustParseDOT(t, "digraph { a -> b -> a }"))
	result := FailOnCycle(stats.Cycles, 3)
	if !result.Failed() {
		t.Fatal("there is a cycle")
	}
	assertEqual(t, result.Matched, []string{"a, b"}, "matched")
	if FailOnCycle(nil, 3).Failed() {
		t.Error("an acyclic graph passes")
	}
}

func TestAnyFailedIsTheGate(t *testing.T) {
	graph := mustParseDOT(t, services)
	report := Impact(graph, []string{"reporting"})
	passing := FailIfReaches(graph, report, Selector{Key: "group", Value: "core"}, 3)
	failing := FailIfReaches(graph, report, Selector{Key: "group", Value: "production"}, 3)
	if AnyFailed([]*PolicyResult{passing}) || !AnyFailed([]*PolicyResult{passing, failing}) {
		t.Error("the gate does not follow the verdicts")
	}
}

func TestFailOnNewReachNamesEveryExposure(t *testing.T) {
	before, after, diff := diffOf(t, beforeDOT, afterDOT, []string{"auth"})
	selector := Selector{Key: "group", Value: "production"}
	exposures := NewlyExposed(before, after, diff, selector)
	result := FailOnNewReach(exposures, selector, before, after)
	if !result.Failed() {
		t.Fatal("payments became reachable")
	}
	assertEqual(t, result.Matched, []string{"payments"}, "matched")
	if FailOnNewReach(nil, selector, before, after).Failed() {
		t.Error("nothing exposed means nothing to fail")
	}
}

// -- selecting on any attribute --------------------------------------------

const risky = `
digraph {
	spec    [group = "design", risk = "low"]
	schema  [group = "backend", risk = "high"]
	api     [group = "backend", risk = "critical"]
	docs    [group = "design", risk = "low"]
	spec -> schema -> api
	spec -> docs
}`

func TestAttrSelectsOnAnyAttribute(t *testing.T) {
	graph := mustParseDOT(t, risky)
	selector, err := ParseSelector("attr:risk=high")
	if err != nil {
		t.Fatal(err)
	}
	if selector.String() != "attr:risk=high" {
		t.Errorf("selector renders as %q", selector)
	}
	assertEqual(t, selector.Select(graph), []string{"schema"}, "risk=high")
}

func TestTheShorthandsAreTheSameThing(t *testing.T) {
	graph := mustParseDOT(t, risky)
	shorthand, _ := ParseSelector("group=backend")
	explicit, _ := ParseSelector("attr:group=backend")
	assertEqual(t, shorthand.Select(graph), explicit.Select(graph), "group, both ways")
	if shorthand.String() == explicit.String() {
		t.Error("each should render the way it was written")
	}
}

func TestAnUnknownBareKeyIsRefusedAndPointsAtAttr(t *testing.T) {
	_, err := ParseSelector("risk=high")
	if err == nil || !strings.Contains(err.Error(), "attr:risk=high") {
		t.Fatalf("error = %v", err)
	}
	if _, isUsage := err.(*UsageError); !isUsage {
		t.Errorf("a typo must be a usage error, got %T", err)
	}
}

func TestAttrNodeIsRefused(t *testing.T) {
	_, err := ParseSelector("attr:node=auth")
	if err == nil || !strings.Contains(err.Error(), "not an attribute") {
		t.Errorf("error = %v", err)
	}
}

func TestASelectorNobodyDeclaresIsUndeterminable(t *testing.T) {
	graph := mustParseDOT(t, risky)
	selector, _ := ParseSelector("attr:environment=production")
	result := FailIfReaches(graph, Impact(graph, []string{"spec"}), selector, 3)

	if !result.Unknown() {
		t.Fatalf("verdict = %q, wanted unknown: nothing in the graph declares 'environment'",
			result.Verdict)
	}
	if result.Failed() {
		t.Error("an unsettled policy is not a proven violation")
	}
	if !strings.Contains(result.Detail, "no node in this graph declares 'environment'") {
		t.Errorf("detail = %q", result.Detail)
	}
}

func TestAnAttributeThatExistsButDoesNotMatchSimplyPasses(t *testing.T) {
	graph := mustParseDOT(t, risky)
	selector, _ := ParseSelector("attr:risk=catastrophic")
	result := FailIfReaches(graph, Impact(graph, []string{"spec"}), selector, 3)
	if result.Verdict != VerdictPass {
		t.Errorf("verdict = %q: the attribute is declared, so a zero match is an answer",
			result.Verdict)
	}
}

func TestAProvenViolationOutranksAnUnsettledOne(t *testing.T) {
	graph := mustParseDOT(t, risky)
	report := Impact(graph, []string{"spec"})
	failing, _ := ParseSelector("attr:risk=critical")
	unsettled, _ := ParseSelector("attr:environment=production")

	results := []*PolicyResult{
		FailIfReaches(graph, report, unsettled, 3),
		FailIfReaches(graph, report, failing, 3),
	}
	if Outcome(results) != VerdictFail {
		t.Errorf("outcome = %q, wanted fail", Outcome(results))
	}
	if Outcome(results[:1]) != VerdictUnknown {
		t.Errorf("outcome = %q, wanted unknown", Outcome(results[:1]))
	}
}
