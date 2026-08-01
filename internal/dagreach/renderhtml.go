package dagreach

// The HTML report: one file you can open, keep, print, or attach.
//
// It is deliberately a report and not an explorer. There is no graph drawing,
// no interaction and no script: laying out a graph well needs either a layout
// engine or a lie, and a page that draws 20 000 nodes badly is worse than a page
// that does not draw them at all.
//
// Two properties are load-bearing. The page is **self-contained** - style
// inlined, no fonts, no images, nothing fetched - because a report that phones
// home is not an artifact you can keep. And it is **deterministic**: no date, no
// generated identifiers, so two runs on the same input produce the same bytes
// and a report can be diffed against the last one.
//
// Structure comes from the same place the markdown report gets it: the text
// lines, plus the policy results. One renderer decides what a report says; these
// only decide how it looks.

import (
	"fmt"
	"html"
	"strings"
)

// htmlStyle is the whole stylesheet. Both colour schemes are declared because
// the page will be opened by someone whose preference nobody asked.
const htmlStyle = `:root {
  --bg: #ffffff; --fg: #1a1d21; --muted: #5a6472; --line: #d8dee6;
  --panel: #f6f8fa; --pass: #1a7f37; --fail: #b42318; --unknown: #9a6700;
}
@media (prefers-color-scheme: dark) {
  :root {
    --bg: #14171a; --fg: #e6e9ee; --muted: #9aa4b2; --line: #2c3138;
    --panel: #1b1f24; --pass: #4ac26b; --fail: #f97066; --unknown: #d4a72c;
  }
}
* { box-sizing: border-box; }
body {
  margin: 0; padding: 2rem 1.25rem; background: var(--bg); color: var(--fg);
  font: 16px/1.55 system-ui, -apple-system, Segoe UI, Roboto, sans-serif;
}
main { max-width: 54rem; margin: 0 auto; }
h1 { font-size: 1.1rem; letter-spacing: .04em; text-transform: uppercase; color: var(--muted); margin: 0; }
.headline { font-size: 1.5rem; font-weight: 600; margin: .35rem 0 1rem; overflow-wrap: anywhere; }
dl.facts { display: grid; grid-template-columns: max-content 1fr; gap: .2rem .9rem; margin: 0 0 1.75rem; }
dl.facts dt { color: var(--muted); }
dl.facts dd { margin: 0; overflow-wrap: anywhere; }
.verdict { font-weight: 600; padding: .7rem .9rem; border-radius: 6px; border: 1px solid var(--line); background: var(--panel); }
.verdict.pass { color: var(--pass); }
.verdict.fail { color: var(--fail); }
.verdict.unknown { color: var(--unknown); }
.policy { border: 1px solid var(--line); border-radius: 6px; padding: .8rem .9rem; margin: .8rem 0; }
.policy header { display: flex; gap: .6rem; align-items: baseline; flex-wrap: wrap; }
.chip { font-size: .78rem; font-weight: 700; letter-spacing: .06em; padding: .1rem .45rem; border-radius: 4px; border: 1px solid currentColor; }
.chip.pass { color: var(--pass); }
.chip.fail { color: var(--fail); }
.chip.unknown { color: var(--unknown); }
.subject, code, pre { font-family: ui-monospace, SFMono-Regular, Consolas, monospace; }
.detail { margin: .5rem 0 0; }
ul.witnesses { list-style: none; margin: .6rem 0 0; padding: 0; }
ul.witnesses li { padding: .2rem 0; border-top: 1px solid var(--line); font-size: .92rem; overflow-wrap: anywhere; }
ul.witnesses .target { font-weight: 600; }
ul.witnesses .path { color: var(--muted); font-family: ui-monospace, SFMono-Regular, Consolas, monospace; }
h2 { font-size: .85rem; letter-spacing: .06em; text-transform: uppercase; color: var(--muted); margin: 2rem 0 .5rem; }
pre.report { background: var(--panel); border: 1px solid var(--line); border-radius: 6px; padding: .9rem; overflow-x: auto; font-size: .88rem; margin: 0; }
footer { margin-top: 2rem; padding-top: .9rem; border-top: 1px solid var(--line); color: var(--muted); font-size: .85rem; }
footer code { overflow-wrap: anywhere; }
@media print {
  body { padding: 0; } pre.report { white-space: pre-wrap; }
}`

// HTMLReport renders one report as a standalone page.
//
// `command` is the argv the report came from, printed so the page says how to
// produce it again - the one thing a file that outlives its terminal cannot
// otherwise tell you.
func HTMLReport(g *Graph, command []string, body []string, policies []*PolicyResult, limit int) []string {
	headline := "nothing to report"
	rest := []string{}
	if len(body) > 0 {
		headline, rest = body[0], body[1:]
	}

	lines := []string{
		"<!doctype html>",
		`<html lang="en">`,
		"<head>",
		`<meta charset="utf-8">`,
		`<meta name="viewport" content="width=device-width, initial-scale=1">`,
		"<title>dagreach " + escape(sourceName(g)) + "</title>",
		"<style>",
		htmlStyle,
		"</style>",
		"</head>",
		"<body>",
		"<main>",
		"<h1>dagreach</h1>",
		`<p class="headline">` + escape(headline) + "</p>",
	}
	lines = append(lines, htmlFacts(g)...)
	lines = append(lines, htmlPolicies(policies, limit)...)

	if len(rest) > 0 {
		lines = append(lines, "<h2>Full report</h2>", `<pre class="report">`)
		for _, line := range rest {
			lines = append(lines, escape(line))
		}
		lines = append(lines, "</pre>")
	}

	lines = append(lines,
		"<footer>",
		"<p>produced by <code>"+escape(strings.Join(command, " "))+"</code></p>",
		"<p>every number is defined in docs/metrics.md. "+
			"This page is a report, not a graph explorer.</p>",
		"</footer>",
		"</main>",
		"</body>",
		"</html>",
	)
	return lines
}

func htmlFacts(g *Graph) []string {
	// The edge orientation is not repeated here: it is the second line of every
	// report, and it belongs next to the numbers it changes the meaning of.
	facts := [][2]string{{"source", sourceName(g)}}
	if g.Profile != "" {
		facts = append(facts, [2]string{"profile", g.Profile})
	}
	facts = append(facts, [2]string{"dagreach", Version})

	lines := []string{`<dl class="facts">`}
	for _, fact := range facts {
		lines = append(lines,
			"<dt>"+escape(fact[0])+"</dt><dd>"+escape(fact[1])+"</dd>")
	}
	return append(lines, "</dl>")
}

func htmlPolicies(policies []*PolicyResult, limit int) []string {
	if len(policies) == 0 {
		return nil
	}
	outcome := Outcome(policies)
	lines := []string{
		fmt.Sprintf(`<p class="verdict %s">%s</p>`, outcome, escape(verdictSentence(outcome))),
		"<h2>Policies</h2>",
	}

	for _, result := range policies {
		lines = append(lines,
			`<section class="policy">`,
			"<header>"+
				fmt.Sprintf(`<span class="chip %s">%s</span>`,
					result.Verdict, escape(strings.ToUpper(result.Verdict)))+
				`<span class="subject">`+escape(result.Policy+" "+result.Subject)+"</span>"+
				"</header>",
			`<p class="detail">`+escape(result.Detail)+"</p>",
		)
		lines = append(lines, htmlWitnesses(result, limit)...)
		lines = append(lines, "</section>")
	}
	return lines
}

// htmlWitnesses lists what a failing policy matched, each with the path that
// proves it. Only a failure has something to prove: a pass matched nothing, and
// an unsettled policy could not look.
func htmlWitnesses(result *PolicyResult, limit int) []string {
	if !result.Failed() || len(result.Matched) == 0 {
		return nil
	}
	shown := result.Matched
	if limit > 0 && len(shown) > limit {
		shown = shown[:limit]
	}

	lines := []string{`<ul class="witnesses">`}
	for _, node := range shown {
		entry := `<li><span class="target">` + escape(node) + "</span>"
		if witness, ok := result.Witnesses[node]; ok && len(witness) > 0 {
			// Each step is escaped, then joined with an arrow that is markup:
			// the separator is ours, every node identifier is theirs.
			steps := make([]string, 0, len(witness))
			for _, step := range witness {
				steps = append(steps, escape(step))
			}
			entry += ` <span class="path">` + strings.Join(steps, " &rarr; ") + "</span>"
		}
		lines = append(lines, entry+"</li>")
	}
	if len(result.Matched) > len(shown) {
		lines = append(lines, fmt.Sprintf("<li>(+%d more)</li>", len(result.Matched)-len(shown)))
	}
	return append(lines, "</ul>")
}

func verdictSentence(outcome string) string {
	switch outcome {
	case VerdictFail:
		return "FAIL - at least one policy was violated."
	case VerdictUnknown:
		return "UNKNOWN - at least one policy could not be settled by this graph."
	}
	return "PASS - every policy was satisfied."
}

// escape is the only way user text enters the page. Node identifiers come from
// somebody's file, so a node called <script> has to arrive as five characters
// and not as a tag - which is also why nothing here writes user text into an
// attribute or into script.
func escape(text string) string { return html.EscapeString(text) }
