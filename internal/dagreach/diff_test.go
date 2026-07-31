package dagreach

import (
	"strings"
	"testing"
)

const (
	beforeDOT = `digraph {
		auth [group = "core"] token [group = "core"]
		payments [group = "production"] ledger [group = "production"]
		auth -> token
		ledger -> payments
	}`
	afterDOT = `digraph {
		auth [group = "core"] token [group = "core"]
		payments [group = "production"] ledger [group = "production"]
		auth -> token
		token -> payments
		ledger -> payments
	}`
)

func diffOf(t *testing.T, before, after string, seeds []string) (*Graph, *Graph, *ReachDiff) {
	t.Helper()
	left, right := mustParseDOT(t, before), mustParseDOT(t, after)
	return left, right, DiffReach(left, right, seeds)
}

func TestReachDeltaIsTheDifferenceBetweenTwoTraversals(t *testing.T) {
	_, _, diff := diffOf(t, beforeDOT, afterDOT, []string{"auth"})
	assertEqual(t, diff.ReachedBefore, []string{"auth", "token"}, "before")
	assertEqual(t, diff.ReachedAfter, []string{"auth", "token", "payments"}, "after")
	assertEqual(t, diff.Added, []string{"payments"}, "added")
	assertEqual(t, diff.Removed, []string{}, "removed")
	if diff.Unchanged() {
		t.Error("the reach changed")
	}
}

func TestLostReachIsReportedToo(t *testing.T) {
	_, _, diff := diffOf(t, afterDOT, beforeDOT, []string{"auth"})
	assertEqual(t, diff.Added, []string{}, "added")
	assertEqual(t, diff.Removed, []string{"payments"}, "removed")
}

func TestStructuralChangeIsReportedAlongside(t *testing.T) {
	_, _, diff := diffOf(t, beforeDOT, afterDOT, []string{"auth"})
	if len(diff.EdgesAdded) != 1 || diff.EdgesAdded[0] != [2]string{"token", "payments"} {
		t.Errorf("edges added = %v", diff.EdgesAdded)
	}
	if len(diff.EdgesRemoved) != 0 || len(diff.NodesAdded) != 0 {
		t.Errorf("removed=%v nodesAdded=%v", diff.EdgesRemoved, diff.NodesAdded)
	}
}

func TestASeedThatExistsInOnlyOneVersionIsFlagged(t *testing.T) {
	_, _, diff := diffOf(t, "digraph { a }", "digraph { a -> b }", []string{"b"})
	assertEqual(t, diff.SeedsMissingBefore, []string{"b"}, "missing before")
	assertEqual(t, diff.SeedsMissingAfter, []string{}, "missing after")
}

func TestANewEdgeIsNamedAsTheReason(t *testing.T) {
	before, after, diff := diffOf(t, beforeDOT, afterDOT, []string{"auth"})
	exposures := NewlyExposed(before, after, diff, Selector{"group", "production"})
	if len(exposures) != 1 {
		t.Fatalf("exposures = %v", exposures)
	}
	exposure := exposures[0]
	if exposure.Target != "payments" || exposure.Reason != "new-edge" {
		t.Errorf("target=%s reason=%s", exposure.Target, exposure.Reason)
	}
	if exposure.Detail != "new edge token -> payments" {
		t.Errorf("detail = %q", exposure.Detail)
	}
	assertEqual(t, exposure.Path, []string{"auth", "token", "payments"}, "witness")
}

func TestANewNodeIsNamedAsSuch(t *testing.T) {
	before, after, diff := diffOf(t, "digraph { auth }",
		`digraph { auth -> secrets; secrets [group = "production"] }`, []string{"auth"})
	exposures := NewlyExposed(before, after, diff, Selector{"group", "production"})
	if len(exposures) != 1 || exposures[0].Reason != "new-node" {
		t.Fatalf("exposures = %v", exposures)
	}
	if !strings.Contains(exposures[0].Detail, "did not exist before") {
		t.Errorf("detail = %q", exposures[0].Detail)
	}
}

func TestAReclassifiedTargetIsCaughtWithoutAnyNewEdge(t *testing.T) {
	before, after, diff := diffOf(t,
		`digraph { auth -> store; store [group = "staging"] }`,
		`digraph { auth -> store; store [group = "production"] }`, []string{"auth"})
	assertEqual(t, diff.Added, []string{}, "nothing new is reachable")
	exposures := NewlyExposed(before, after, diff, Selector{"group", "production"})
	if len(exposures) != 1 || exposures[0].Reason != "reclassified" {
		t.Fatalf("exposures = %v", exposures)
	}
	if !strings.Contains(exposures[0].Detail, "'staging' to 'production'") {
		t.Errorf("detail = %q", exposures[0].Detail)
	}
}

func TestNothingExposedWhenTheTargetWasAlreadyReached(t *testing.T) {
	before, after, diff := diffOf(t, afterDOT, afterDOT, []string{"auth"})
	if exposures := NewlyExposed(before, after, diff, Selector{"group", "production"}); len(exposures) != 0 {
		t.Errorf("exposures = %v", exposures)
	}
}

func TestAllPairsDeltaCountsOrderedPairs(t *testing.T) {
	delta := DiffAllPairs(mustParseDOT(t, "digraph { a -> b; c }"),
		mustParseDOT(t, "digraph { a -> b -> c }"))
	if delta.AddedTotal != 2 || delta.RemovedTotal != 0 {
		t.Errorf("added=%d removed=%d", delta.AddedTotal, delta.RemovedTotal)
	}
	if delta.AddedBySource["a"] != 1 || delta.AddedBySource["b"] != 1 {
		t.Errorf("by source = %v", delta.AddedBySource)
	}
}

func TestAllPairsDeltaReportsLosses(t *testing.T) {
	delta := DiffAllPairs(mustParseDOT(t, "digraph { a -> b -> c }"),
		mustParseDOT(t, "digraph { a -> b; c }"))
	if delta.AddedTotal != 0 || delta.RemovedTotal != 2 {
		t.Errorf("added=%d removed=%d", delta.AddedTotal, delta.RemovedTotal)
	}
}

func TestAllPairsDeltaHandlesCycles(t *testing.T) {
	delta := DiffAllPairs(mustParseDOT(t, "digraph { a; b }"),
		mustParseDOT(t, "digraph { a -> b -> a }"))
	// inside a cycle everything reaches everything, itself included
	if delta.AddedTotal != 4 {
		t.Errorf("added = %d", delta.AddedTotal)
	}
}
