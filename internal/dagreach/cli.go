package dagreach

// The command line.
//
// Argument wiring only: reading lives in loading.go, the metrics in analysis.go
// and reports.go, the CI decision in policy.go, and the output shapes in
// render.go, reportjson.go and renderhtml.go.
//
// Flags may appear before or after the file arguments, because that is how the
// documented examples read.

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
)

// Exit codes are part of the public contract (CI depends on them).
const (
	ExitOK            = 0
	ExitPolicyFailed  = 1
	ExitUsage         = 2
	ExitPolicyUnknown = 3
	ExitInputError    = 4
)

// DefaultLimit is how many items a text list shows before it says how many it hid.
const DefaultLimit = 10

// Version is the release this build claims to be. It has one home, and the
// release workflow refuses a tag that disagrees with it.
const Version = "0.1.0-rc.3"

type options struct {
	positional     []string
	profile        string
	format         string
	edgeSemantics  string
	json           bool
	markdown       bool
	html           bool
	explain        bool
	limit          int
	changed        []string
	failIfReaches  []string
	failOnNewReach []string
	failOn         []string
	maxImpacted    *int
	allPairs       bool
	argv           []string
	countOnly      bool
	// seen records every flag the command line carried, so a flag the command
	// does not read can be refused instead of dropped.
	seen []string
}

// valued lists the flags that take a value; everything else is a switch.
var valued = map[string]bool{
	"--profile": true, "--format": true, "--edge-semantics": true, "--limit": true,
	"--changed": true, "--fail-if-reaches": true, "--fail-on-new-reach": true,
	"--fail-on": true, "--max-impacted": true,
}

// commandFlags is what each command actually reads.
//
// A flag a command does not read used to be accepted and then quietly dropped,
// so `diff --fail-if-reaches group=production` exited 0 with the policy never
// evaluated - a gate that passes because nobody ran it. Anything not listed here
// for the command in hand is a usage error, which is the whole point: dagreach
// refuses to look like a clean run it did not perform.
// Each list is complete, output flags included: `profiles` lists what dagreach
// knows rather than reporting on a graph, so a markdown or HTML rendering of it
// would be a shape nobody asked for.
var commandFlags = map[string][]string{
	"profiles": {"--json"},
	"parse":    {"--profile", "--format", "--edge-semantics", "--json", "--markdown", "--html"},
	"stats": {"--profile", "--format", "--edge-semantics", "--limit", "--fail-on",
		"--json", "--markdown", "--html"},
	"impact": {"--profile", "--format", "--edge-semantics", "--limit", "--changed",
		"--explain", "--fail-if-reaches", "--max-impacted", "--fail-on",
		"--json", "--markdown", "--html"},
	"diff": {"--profile", "--format", "--edge-semantics", "--limit", "--changed",
		"--explain", "--fail-on-new-reach", "--all-pairs-reachability-delta", "--count-only",
		"--json", "--markdown", "--html"},
}

// checkFlags refuses a flag this command does not read, and a flag that only
// means something beside another one.
func checkFlags(command string, parsed *options) error {
	known, ok := commandFlags[command]
	if !ok {
		return nil
	}
	allowed := map[string]bool{}
	for _, flag := range known {
		allowed[flag] = true
	}
	for _, flag := range parsed.seen {
		if !allowed[flag] {
			return fmt.Errorf("%s does not read %s; it reads %s",
				command, flag, strings.Join(known, ", "))
		}
	}
	if contains(parsed.seen, "--count-only") && !parsed.allPairs {
		return fmt.Errorf(
			"--count-only shortens the all-pairs comparison, so it needs " +
				"--all-pairs-reachability-delta")
	}
	return nil
}

// Run executes one command line and returns its exit code.
func Run(args []string, stdout, stderr io.Writer, stdin io.Reader) int {
	if len(args) == 0 {
		fmt.Fprint(stdout, usage())
		return ExitOK
	}
	if args[0] == "--version" || args[0] == "-V" {
		fmt.Fprintf(stdout, "dagreach %s\n", Version)
		return ExitOK
	}
	if args[0] == "--help" || args[0] == "-h" || args[0] == "help" {
		fmt.Fprint(stdout, usage())
		return ExitOK
	}

	command := args[0]
	parsed, err := parseOptions(args[1:])
	if err != nil {
		fmt.Fprintf(stderr, "dagreach: %v\n", err)
		return ExitUsage
	}
	if err := checkFlags(command, parsed); err != nil {
		fmt.Fprintf(stderr, "dagreach: %v\n", err)
		return ExitUsage
	}
	// Kept so the HTML report can say how to produce itself again; a file
	// outlives the terminal that made it.
	parsed.argv = append([]string{"dagreach"}, args...)

	switch command {
	case "profiles":
		if len(parsed.positional) > 0 {
			fmt.Fprintln(stderr, "dagreach: profiles takes no arguments")
			return ExitUsage
		}
		if parsed.json {
			encoded, _ := json.MarshalIndent(ProfilesJSON(), "", "  ")
			fmt.Fprintln(stdout, string(encoded))
			return ExitOK
		}
		for _, line := range ProfilesText() {
			fmt.Fprintln(stdout, line)
		}
		return ExitOK
	case "parse", "stats", "impact":
		return runSingle(command, parsed, stdout, stderr, stdin)
	case "diff":
		return runDiff(parsed, stdout, stderr, stdin)
	}
	fmt.Fprintf(stderr, "dagreach: unknown command '%s'\n", command)
	return ExitUsage
}

func parseOptions(args []string) (*options, error) {
	parsed := &options{limit: DefaultLimit}
	for index := 0; index < len(args); index++ {
		argument := args[index]
		if !strings.HasPrefix(argument, "--") {
			parsed.positional = append(parsed.positional, argument)
			continue
		}

		name, value, hasInline := strings.Cut(argument, "=")
		if !contains(parsed.seen, name) {
			parsed.seen = append(parsed.seen, name)
		}
		switch {
		case valued[name] && !hasInline:
			index++
			if index >= len(args) {
				return nil, fmt.Errorf("option %s needs a value", name)
			}
			value = args[index]
		case !valued[name] && hasInline:
			// `--json=false` used to turn JSON on, because a switch never looked
			// at the value it was handed. A switch is present or absent.
			return nil, fmt.Errorf("%s is a switch and takes no value; "+
				"pass %s to turn it on, or leave it out", name, name)
		}

		switch name {
		case "--profile":
			if _, ok := GetProfile(value); !ok {
				return nil, fmt.Errorf("unknown profile '%s'; expected one of %s",
					value, strings.Join(ProfileOrder, ", "))
			}
			parsed.profile = value
		case "--format":
			if value != "dot" && value != "jgf" {
				return nil, fmt.Errorf("unknown format '%s'; expected one of %s",
					value, strings.Join(Formats, ", "))
			}
			parsed.format = value
		case "--edge-semantics":
			if !knownSemantics(value) {
				return nil, fmt.Errorf("unknown edge semantics '%s'; expected one of %s",
					value, strings.Join(EdgeSemantics, ", "))
			}
			parsed.edgeSemantics = value
		case "--json":
			parsed.json = true
		case "--markdown":
			parsed.markdown = true
		case "--html":
			parsed.html = true
		case "--explain":
			parsed.explain = true
		case "--limit":
			limit, err := strconv.Atoi(value)
			if err != nil {
				return nil, fmt.Errorf("--limit needs a whole number, found '%s'", value)
			}
			if limit < 0 {
				return nil, fmt.Errorf(
					"--limit cannot be negative; 0 shows everything, found '%s'", value)
			}
			parsed.limit = limit
		case "--changed":
			parsed.changed = append(parsed.changed, value)
		case "--fail-if-reaches":
			parsed.failIfReaches = append(parsed.failIfReaches, value)
		case "--fail-on-new-reach":
			parsed.failOnNewReach = append(parsed.failOnNewReach, value)
		case "--fail-on":
			if value != "cycle" {
				return nil, fmt.Errorf("unknown --fail-on condition '%s'; expected cycle", value)
			}
			parsed.failOn = append(parsed.failOn, value)
		case "--max-impacted":
			ceiling, err := strconv.Atoi(value)
			if err != nil {
				return nil, fmt.Errorf("--max-impacted needs a whole number, found '%s'", value)
			}
			if ceiling < 0 {
				// A negative ceiling fails every run, including one that touches
				// nothing, which is a gate nobody meant to write.
				return nil, fmt.Errorf(
					"--max-impacted cannot be negative; 0 refuses any impact at all, found '%s'",
					value)
			}
			parsed.maxImpacted = &ceiling
		case "--all-pairs-reachability-delta":
			parsed.allPairs = true
		case "--count-only":
			parsed.countOnly = true
		default:
			return nil, fmt.Errorf("unknown option '%s'", name)
		}
	}
	if chosen := outputModes(parsed); len(chosen) > 1 {
		return nil, fmt.Errorf("%s ask for different outputs; pick one",
			strings.Join(chosen, " and "))
	}
	return parsed, nil
}

// outputModes names the output flags that were asked for. They are mutually
// exclusive, and the error has to say which two collided rather than list all
// three at a reader who only passed one of them.
func outputModes(parsed *options) []string {
	chosen := []string{}
	for _, mode := range []struct {
		flag string
		on   bool
	}{{"--json", parsed.json}, {"--markdown", parsed.markdown}, {"--html", parsed.html}} {
		if mode.on {
			chosen = append(chosen, mode.flag)
		}
	}
	return chosen
}

// inBody decides whether the text body should carry the policy block. Every mode
// but HTML wants it there; the HTML page renders the verdicts as its own section
// above the report, and printing the same three sentences twice on one screen is
// how a page teaches its reader to skim.
func inBody(parsed *options, policies []*PolicyResult) []*PolicyResult {
	if parsed.html {
		return nil
	}
	return policies
}

func runSingle(command string, parsed *options, stdout, stderr io.Writer, stdin io.Reader) int {
	if len(parsed.positional) != 1 {
		fmt.Fprintf(stderr, "dagreach: %s needs exactly one FILE\n", command)
		return ExitUsage
	}

	graph, err := LoadGraph(parsed.positional[0], LoadOptions{
		Profile: parsed.profile, Format: parsed.format,
		EdgeSemantics: parsed.edgeSemantics, Stdin: stdin,
	})
	if err != nil {
		fmt.Fprintf(stderr, "dagreach: %v\n", err)
		if _, usageProblem := err.(*UsageError); usageProblem {
			return ExitUsage
		}
		return ExitInputError
	}

	switch command {
	case "parse":
		summary := Summarize(graph)
		emit(stdout, parsed, graph, ParseJSON(graph, summary), ParseText(graph, summary), nil)
		return ExitOK
	case "stats":
		stats := Analyse(graph)
		policies := []*PolicyResult{}
		if contains(parsed.failOn, "cycle") {
			policies = append(policies, FailOnCycle(stats.Cycles, 3))
		}
		emit(stdout, parsed, graph, StatsJSON(graph, stats, policies),
			StatsText(graph, stats, parsed.limit, inBody(parsed, policies)), policies)
		return exitFor(policies)
	}

	seeds := splitIDs(parsed.changed)
	if len(seeds) == 0 {
		fmt.Fprintln(stderr, "dagreach: --changed needs at least one node id")
		return ExitUsage
	}
	unknown := []string{}
	for _, seed := range seeds {
		if !graph.HasNode(seed) {
			unknown = append(unknown, seed)
		}
	}
	if len(unknown) > 0 {
		for _, seed := range unknown {
			fmt.Fprintf(stderr, "dagreach: no node '%s' in %s%s\n",
				seed, graph.Source, suggest(seed, graph))
		}
		return ExitInputError
	}

	selectors, err := parseSelectors(parsed.failIfReaches)
	if err != nil {
		fmt.Fprintf(stderr, "dagreach: %v\n", err)
		return ExitUsage
	}

	// A selector naming a node that does not exist is a typo, not a policy that
	// passes - the same treatment --changed already gets.
	for _, selector := range selectors {
		if selector.ByIdentifier() && !graph.HasNode(selector.Value) {
			fmt.Fprintf(stderr, "dagreach: no node '%s' in %s%s\n",
				selector.Value, graph.Source, suggest(selector.Value, graph))
			return ExitInputError
		}
	}

	report := Impact(graph, seeds)
	policies := []*PolicyResult{}
	for _, selector := range selectors {
		policies = append(policies, FailIfReaches(graph, report, selector, 3))
	}
	if parsed.maxImpacted != nil {
		policies = append(policies, MaxImpacted(report, *parsed.maxImpacted))
	}
	if contains(parsed.failOn, "cycle") {
		policies = append(policies, FailOnCycle(Analyse(graph).Cycles, 3))
	}

	emit(stdout, parsed, graph, ImpactJSON(graph, report, policies, parsed.explain),
		ImpactText(graph, report, parsed.limit, inBody(parsed, policies), parsed.explain), policies)
	return exitFor(policies)
}

// incomparable reports why two graphs cannot be diffed, or "" when they can.
//
// Detection runs per file, so one side can be recognised as a dependency export
// and the other read as `feeds` - and then the two are oriented in opposite
// directions and every delta is backwards. A wrong answer that looks right is
// worse than a refusal, so this refuses. Naming --profile or --edge-semantics
// settles both files at once, which is the way out.
func incomparable(before, after *Graph) string {
	for _, mismatch := range []struct{ what, left, right, remedy string }{
		{"profile", before.Profile, after.Profile,
			"pass --profile to read both files the same way"},
		{"edge semantics", before.EdgeSemantics, after.EdgeSemantics,
			"pass --edge-semantics to orient both files the same way"},
		{"direction", directedness(before), directedness(after),
			"no flag settles this one: the two files disagree about what an edge is"},
	} {
		if mismatch.left == mismatch.right {
			continue
		}
		return fmt.Sprintf(
			"%s and %s do not have the same %s (%s against %s), so a comparison would "+
				"compare two different readings; %s",
			sourceName(before), sourceName(after), mismatch.what,
			mismatch.left, mismatch.right, mismatch.remedy)
	}
	return ""
}

func directedness(g *Graph) string {
	if g.Directed {
		return "directed"
	}
	return "undirected"
}

func runDiff(parsed *options, stdout, stderr io.Writer, stdin io.Reader) int {
	if len(parsed.positional) != 2 {
		fmt.Fprintln(stderr, "dagreach: diff needs BEFORE and AFTER")
		return ExitUsage
	}

	load := LoadOptions{Profile: parsed.profile, Format: parsed.format,
		EdgeSemantics: parsed.edgeSemantics, Stdin: stdin}
	before, err := LoadGraph(parsed.positional[0], load)
	if err != nil {
		fmt.Fprintf(stderr, "dagreach: %v\n", err)
		return ExitInputError
	}
	after, err := LoadGraph(parsed.positional[1], load)
	if err != nil {
		fmt.Fprintf(stderr, "dagreach: %v\n", err)
		return ExitInputError
	}
	if reason := incomparable(before, after); reason != "" {
		fmt.Fprintf(stderr, "dagreach: %s\n", reason)
		return ExitInputError
	}

	seeds := splitIDs(parsed.changed)
	if len(seeds) == 0 && !parsed.allPairs {
		fmt.Fprintln(stderr,
			"dagreach: diff needs --changed, or --all-pairs-reachability-delta for the "+
				"global comparison")
		return ExitUsage
	}
	for _, seed := range seeds {
		if !before.HasNode(seed) && !after.HasNode(seed) {
			fmt.Fprintf(stderr, "dagreach: no node '%s' in either graph%s\n",
				seed, suggest(seed, after))
			return ExitInputError
		}
	}

	selectors, err := parseSelectors(parsed.failOnNewReach)
	if err != nil {
		fmt.Fprintf(stderr, "dagreach: %v\n", err)
		return ExitUsage
	}

	diff := DiffReach(before, after, seeds)
	exposures := []*Exposure{}
	policies := []*PolicyResult{}
	for _, selector := range selectors {
		found := NewlyExposed(before, after, diff, selector)
		exposures = append(exposures, found...)
		policies = append(policies, FailOnNewReach(found, selector, before, after))
	}

	var allPairs *AllPairsDelta
	if parsed.allPairs {
		allPairs = DiffAllPairs(before, after)
	}

	emit(stdout, parsed, after, DiffJSON(before, after, diff, exposures, policies, allPairs),
		DiffText(before, after, diff, exposures, inBody(parsed, policies),
			parsed.limit, parsed.explain, allPairs, parsed.countOnly), policies)
	return exitFor(policies)
}

// emit writes the one report the caller asked for: text for a terminal, JSON for
// a pipeline, markdown for a pull-request comment, HTML for a file that outlives
// both. They state the same facts.
//
// `subject` is the graph the report is about - the `after` graph for a diff -
// and it is only read by the HTML page, which has a header to fill.
func emit(
	stdout io.Writer, parsed *options, subject *Graph, document map[string]any,
	lines []string, policies []*PolicyResult,
) {
	switch {
	case parsed.json:
		encoded, err := json.MarshalIndent(document, "", "  ")
		if err != nil {
			fmt.Fprintln(stdout, "{}")
			return
		}
		fmt.Fprintln(stdout, string(encoded))
	case parsed.markdown:
		for _, line := range MarkdownReport(lines, policies, parsed.limit) {
			fmt.Fprintln(stdout, line)
		}
	case parsed.html:
		for _, line := range HTMLReport(subject, parsed.argv, lines, policies, parsed.limit) {
			fmt.Fprintln(stdout, line)
		}
	default:
		for _, line := range lines {
			fmt.Fprintln(stdout, line)
		}
	}
}

// exitFor maps the gate's outcome onto the contract. UNKNOWN blocks, because a
// policy the data cannot settle must not be indistinguishable from one it
// satisfied - but it never claims a violation.
func exitFor(policies []*PolicyResult) int {
	switch Outcome(policies) {
	case VerdictFail:
		return ExitPolicyFailed
	case VerdictUnknown:
		return ExitPolicyUnknown
	}
	return ExitOK
}

func parseSelectors(texts []string) ([]Selector, error) {
	selectors := make([]Selector, 0, len(texts))
	for _, text := range texts {
		selector, err := ParseSelector(text)
		if err != nil {
			return nil, err
		}
		selectors = append(selectors, selector)
	}
	return selectors, nil
}

func splitIDs(values []string) []string {
	ids := []string{}
	seen := map[string]bool{}
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			part = strings.TrimSpace(part)
			if part != "" && !seen[part] {
				seen[part] = true
				ids = append(ids, part)
			}
		}
	}
	return ids
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

// suggest offers the closest node names, so a typo does not read as an empty graph.
func suggest(wanted string, g *Graph) string {
	type scored struct {
		node  string
		ratio float64
	}
	candidates := []scored{}
	for _, node := range g.Nodes() {
		if ratio := similarity(wanted, node); ratio >= 0.6 {
			candidates = append(candidates, scored{node, ratio})
		}
	}
	if len(candidates) == 0 {
		return ""
	}
	sort.SliceStable(candidates, func(left, right int) bool {
		return candidates[left].ratio > candidates[right].ratio
	})
	if len(candidates) > 3 {
		candidates = candidates[:3]
	}
	names := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		names = append(names, "'"+candidate.node+"'")
	}
	return "; did you mean " + strings.Join(names, ", ") + "?"
}

// similarity is the ratio difflib.SequenceMatcher reports: twice the number of
// matching characters over the combined length.
func similarity(left, right string) float64 {
	if len(left) == 0 && len(right) == 0 {
		return 1
	}
	matches := longestCommonSubsequence(left, right)
	return 2 * float64(matches) / float64(len(left)+len(right))
}

func longestCommonSubsequence(left, right string) int {
	previous := make([]int, len(right)+1)
	current := make([]int, len(right)+1)
	for l := 1; l <= len(left); l++ {
		for r := 1; r <= len(right); r++ {
			if left[l-1] == right[r-1] {
				current[r] = previous[r-1] + 1
			} else if previous[r] >= current[r-1] {
				current[r] = previous[r]
			} else {
				current[r] = current[r-1]
			}
		}
		copy(previous, current)
	}
	return previous[len(right)]
}

func usage() string {
	return strings.Join([]string{
		"dagreach - portable change-impact analysis for dependency graphs.",
		"See what a change can reach, why it reaches it, and whether CI should allow it.",
		"",
		"usage:",
		"  dagreach parse    FILE [--profile P] [--format F] [--edge-semantics S] [--json]",
		"  dagreach stats    FILE [--limit N] [--fail-on cycle] [--json]",
		"  dagreach impact   FILE --changed ID[,ID...] [--explain] [--limit N]",
		"                         [--fail-if-reaches SELECTOR] [--max-impacted N]",
		"                         [--fail-on cycle] [--json]",
		"  dagreach diff     BEFORE AFTER --changed ID[,ID...] [--explain]",
		"                         [--fail-on-new-reach SELECTOR]",
		"                         [--all-pairs-reachability-delta [--count-only]] [--json]",
		"  dagreach profiles [--json]",
		"",
		"output: --json for a pipeline, --markdown for a pull-request comment (what the",
		"        GitHub Action posts), --html for a standalone file; text otherwise.",
		"        A flag the command does not read is a usage error, never a silent no-op",
		"",
		"selectors: group=VALUE, status=VALUE, node=ID, attr:NAME=VALUE",
		"exit codes: 0 ok, 1 a policy failed, 2 usage, 3 a policy could not be settled,",
		"            4 the input could not be read",
		"",
	}, "\n")
}
