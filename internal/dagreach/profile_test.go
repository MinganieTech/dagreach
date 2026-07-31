package dagreach

import (
	"math"
	"strings"
	"testing"
)

func TestDurationPrecedenceAndTolerance(t *testing.T) {
	cases := []struct {
		attrs map[string]string
		want  float64
		ok    bool
	}{
		{map[string]string{"duration": "12"}, 12, true},
		{map[string]string{"duration": "0.5"}, 0.5, true},
		{map[string]string{"weight": "7"}, 7, true},
		{map[string]string{"duration": "3", "weight": "9"}, 3, true},     // duration wins
		{map[string]string{"duration": "later", "weight": "9"}, 9, true}, // unreadable falls back
		{map[string]string{}, 0, false},
		{map[string]string{"duration": "soon"}, 0, false},
		{map[string]string{"duration": "nan"}, 0, false},
		{map[string]string{"duration": "inf"}, 0, false},
	}
	for _, testCase := range cases {
		got, ok := durationOf(testCase.attrs)
		if ok != testCase.ok || (ok && math.Abs(got-testCase.want) > 1e-9) {
			t.Errorf("durationOf(%v) = %v, %v; wanted %v, %v",
				testCase.attrs, got, ok, testCase.want, testCase.ok)
		}
	}
}

func TestStatusAndGroupAreTrimmedAndOptional(t *testing.T) {
	if got := textAttr(map[string]string{"status": "  ready "}, StatusKey); got != "ready" {
		t.Errorf("status = %q", got)
	}
	if got := textAttr(map[string]string{"status": "   "}, StatusKey); got != "" {
		t.Errorf("blank status = %q", got)
	}
	if got := textAttr(map[string]string{}, GroupKey); got != "" {
		t.Errorf("missing group = %q", got)
	}
}

func TestSummaryCountsAndReportsUnreadableValues(t *testing.T) {
	graph := mustParseDOT(t, `digraph {
		a [duration = "10", status = "ready", group = "core"]
		b [duration = "soon", status = "ready"]
		a -> b [weight = "2"]
	}`)
	summary := Summarize(graph)
	if summary.NodesWithDuration != 1 || summary.EdgesWithDuration != 1 {
		t.Errorf("nodes=%d edges=%d", summary.NodesWithDuration, summary.EdgesWithDuration)
	}
	if summary.Statuses.Get("ready") != 2 || summary.Groups.Get("core") != 1 {
		t.Errorf("statuses=%v groups=%v", summary.Statuses.Map(), summary.Groups.Map())
	}
	if !summary.UsesDurations() || len(summary.Unreadable) != 1 {
		t.Fatalf("uses=%v unreadable=%v", summary.UsesDurations(), summary.Unreadable)
	}
	if !strings.Contains(summary.Unreadable[0], "duration='soon'") {
		t.Errorf("unreadable = %q", summary.Unreadable[0])
	}
}

func TestAGraphWithoutProfileAttributesIsNotAnError(t *testing.T) {
	summary := Summarize(mustParseDOT(t, "digraph { a -> b }"))
	if summary.UsesDurations() || len(summary.Unreadable) != 0 {
		t.Errorf("uses=%v unreadable=%v", summary.UsesDurations(), summary.Unreadable)
	}
}

func TestCounterKeepsFirstSeenOrder(t *testing.T) {
	counter := NewCounter()
	for _, key := range []string{"zebra", "alpha", "zebra"} {
		counter.Add(key)
	}
	assertEqual(t, counter.Keys(), []string{"zebra", "alpha"}, "order")
	if counter.Get("zebra") != 2 || counter.Len() != 2 {
		t.Errorf("counts = %v", counter.Map())
	}
}
