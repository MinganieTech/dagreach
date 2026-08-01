# The HTML report

`--html` writes one standalone page: the verdict, the policies with the paths that prove them, and
the full report underneath. Every command accepts it.

```bash
dagreach impact infra.dot --changed aws_vpc.main \
  --fail-if-reaches group=aws_db_instance --explain --html > report.html
```

## What it is for

The other three modes each have a home: text for a terminal, `--json` for a pipeline, `--markdown`
for a pull-request comment. HTML is for the report that has to **outlive the run that produced it**
— a build artifact you keep, a page a CI server publishes, something you print or attach.

That is a narrower job than the other three, and worth saying plainly: if the report is being read
inside a pull request, `--markdown` is the better answer.

## Two properties it is built around

**Self-contained.** The style is inlined; there are no fonts, no images, no scripts, and nothing is
fetched. A report that phones home is not an artifact you can keep, and a report that runs something
is not one you can safely open. A test refuses `<script`, `src=`, `url(`, `@import` and any URL in
the output.

**Deterministic.** No date, no generated identifiers. Two runs on the same input produce the same
bytes, so this week's report diffs against last week's and the difference is the graph. The page
carries the exact command that produced it instead of a timestamp — the one thing a file that has
outlived its terminal cannot otherwise tell you.

## What it shows

| Part | Content |
|---|---|
| Headline | the first line of the report: what reaches what, and how much |
| Facts | source, profile, dagreach version |
| Verdict | `PASS` / `FAIL` / `UNKNOWN`, in the colour of the outcome, when policies ran |
| Policies | one card each: verdict chip, policy and subject, detail, and every match with the path that proves it |
| Full report | the same text the terminal prints, minus the policy block the cards replaced |

The witness path is the reason this page is worth more than a screenshot of the terminal: a reviewer
sees `aws_vpc.main → aws_subnet.private → aws_db_instance.orders` and can argue with it.

Identifiers come from somebody's file, so everything from the graph is HTML-escaped on the way in. A
node called `<script>` arrives as eight characters.

The page follows the reader's light or dark preference, wraps long identifiers, and prints without a
horizontal scrollbar. `--limit` applies as everywhere else.

## What it is not

**It is not a graph explorer.** No drawing, no interaction, no layout. Laying out a graph well needs
a layout engine or a lie, and a page that draws 20 000 nodes badly is worse than a page that does
not draw them. An explorer stays [out of scope for 1.0](../README.md#roadmap).

**It is not a second source of truth.** One renderer decides what a report says; `--html` only
decides how it looks. If the page and `--json` ever disagree, the page is wrong.
