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
	assertEqual(t, Selector{"group", "core"}.Select(graph), []string{"auth", "token"}, "group")
	assertEqual(t, Selector{"status", "critical"}.Select(graph), []string{"payments"}, "status")
	assertEqual(t, Selector{"node", "reporting"}.Select(graph), []string{"reporting"}, "node")
}

func TestFailIfReachesCarriesThePathThatProvesIt(t *testing.T) {
	graph := mustParseDOT(t, services)
	result := FailIfReaches(graph, Impact(graph, []string{"auth"}), Selector{"group", "production"}, 3)
	if !result.Failed {
		t.Fatal("the change reaches production")
	}
	assertEqual(t, result.Matched, []string{"payments"}, "matched")
	assertEqual(t, result.Witnesses["payments"], []string{"auth", "token", "payments"}, "witness")
}

func TestFailIfReachesPassesWhenNothingMatches(t *testing.T) {
	graph := mustParseDOT(t, services)
	result := FailIfReaches(graph, Impact(graph, []string{"reporting"}), Selector{"group", "core"}, 3)
	if result.Failed || !strings.Contains(result.Detail, "nothing matching group=core is reached") {
		t.Errorf("result = %+v", result)
	}
}

func TestAChangedNodeThatIsItselfATargetIsFlaggedAsSuch(t *testing.T) {
	graph := mustParseDOT(t, services)
	result := FailIfReaches(graph, Impact(graph, []string{"payments"}), Selector{"group", "production"}, 3)
	if !result.Failed || !strings.Contains(result.Detail, "they are the changed nodes themselves") {
		t.Errorf("detail = %q", result.Detail)
	}
}

func TestMaxImpactedComparesAgainstTheCeiling(t *testing.T) {
	graph := mustParseDOT(t, services)
	report := Impact(graph, []string{"auth"})
	if !MaxImpacted(report, 2).Failed || MaxImpacted(report, 3).Failed {
		t.Error("the ceiling is compared the wrong way")
	}
}

func TestFailOnCycleListsTheCycles(t *testing.T) {
	stats := Analyse(mustParseDOT(t, "digraph { a -> b -> a }"))
	result := FailOnCycle(stats.Cycles, 3)
	if !result.Failed {
		t.Fatal("there is a cycle")
	}
	assertEqual(t, result.Matched, []string{"a, b"}, "matched")
	if FailOnCycle(nil, 3).Failed {
		t.Error("an acyclic graph passes")
	}
}

func TestAnyFailedIsTheGate(t *testing.T) {
	graph := mustParseDOT(t, services)
	report := Impact(graph, []string{"reporting"})
	passing := FailIfReaches(graph, report, Selector{"group", "core"}, 3)
	failing := FailIfReaches(graph, report, Selector{"group", "production"}, 3)
	if AnyFailed([]*PolicyResult{passing}) || !AnyFailed([]*PolicyResult{passing, failing}) {
		t.Error("the gate does not follow the verdicts")
	}
}

func TestFailOnNewReachNamesEveryExposure(t *testing.T) {
	before, after, diff := diffOf(t, beforeDOT, afterDOT, []string{"auth"})
	selector := Selector{"group", "production"}
	exposures := NewlyExposed(before, after, diff, selector)
	result := FailOnNewReach(exposures, selector)
	if !result.Failed {
		t.Fatal("payments became reachable")
	}
	assertEqual(t, result.Matched, []string{"payments"}, "matched")
	if FailOnNewReach(nil, selector).Failed {
		t.Error("nothing exposed means nothing to fail")
	}
}
